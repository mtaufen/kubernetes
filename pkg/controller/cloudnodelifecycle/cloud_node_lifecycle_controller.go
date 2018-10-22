/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cloudnodelifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/glog"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	cloudprovider "k8s.io/cloud-provider"
	v1node "k8s.io/kubernetes/pkg/api/v1/node"
	"k8s.io/kubernetes/pkg/controller"
	nodeutil "k8s.io/kubernetes/pkg/controller/util/node"
	"k8s.io/kubernetes/pkg/util/metrics"
)

// TODO(mtaufen):
// - get the basic tests ported from the old controller working, then refactor
//   to clean them up (currently first ported test is FAILING)
// - ensure moving this out of the main loop of NLC doesn't create a race
// - look for prior test coverage, modify if it exists, add if it does not
//   possible coverage includes:
//     - node_lifecycle_controller_test.go:TestCloudProvicerNoRateLimit
//     - node_lifecycle_controller_test.go:TestCloudProvicerNodeShutdown
//     - node_lifecycle_controller_test.go:TestNodeEventGeneration
//     - don't appear to be any integration tests... (should add one that runs controller-manager w/ NLC turned on, fake cloud)
//   e2e, maybe:
//     - test/e2e/gke_node_pools.go (calls gcloud to do node pool create/delete, might catch leftover nodes)
//     - test/e2e/lifecycle/cluster_upgrade.go (maybe?)
//     - test/e2e/storage/regional_pd.go (maybe... it deletes nodes so one would expect corresponding objects to get deleted by nlc, and maybe that figures in)

// Controller implements Node lifecycle APIs that require calls to cloud providers.
type Controller struct {
	cloud cloudprovider.Interface

	kubeClient clientset.Interface
	recorder   record.EventRecorder

	nodeLister         corelisters.NodeLister
	nodeInformerSynced cache.InformerSynced

	nodeMonitorPeriod time.Duration
}

// TODO(mtaufen): names? NodeHarvester? NodeCollector? NodeCleaner? NodeRemover?
// TODO(cheftako): Eventually migrate this to the CCM's node lifecycle controller
func NewCloudNodeLifecycleController(
	cloud cloudprovider.Interface,
	kubeClient clientset.Interface,
	nodeInformer coreinformers.NodeInformer,
	nodeMonitorPeriod time.Duration,
) *Controller {
	// TODO(mtaufen): I really hate calls to fatal in libraries; but this is what the current node lifecycle controller does...
	// would be better to just return an error from the constructor
	if kubeClient == nil {
		glog.Fatalf("client must not be nil")
	}
	if rateLimiter := kubeClient.CoreV1().RESTClient().GetRateLimiter(); rateLimiter != nil {
		metrics.RegisterMetricAndTrackRateLimiterUsage("cloud_node_lifecycle_controller", rateLimiter)
	}
	return &Controller{
		cloud:      cloud,
		kubeClient: kubeClient,
		// TODO(mtaufen): I'd like to change the component to something like "cloud-node-controller" - but not sure if
		// this would break anyone's event monitoring...
		recorder:           record.NewBroadcaster().NewRecorder(scheme.Scheme, v1.EventSource{Component: "node-controller"}),
		nodeLister:         nodeInformer.Lister(),
		nodeInformerSynced: nodeInformer.Informer().HasSynced,
	}
}

func (c *Controller) Run(stopCh <-chan struct{}) {
	glog.Infof("Starting cloud node controller.")
	defer glog.Infof("Shutting down cloud node controller.")
	// TODO(mtaufen): consider making nil cloud an error in the constructor
	// this controller depends on having a cloud provider to query, so don't
	// even bother running if we don't have one
	if c.cloud == nil {
		glog.Warningf("Started cloud node controller with nil cloud provider; it will do nothing.")
		return
	}
	// TODO(mtaufen): Originally first arg was "taint"? Not very descriptive...
	if !controller.WaitForCacheSync("cloud-node-controller", stopCh, c.nodeInformerSynced) {
		return
	}
	go wait.Until(c.updateNodes, c.nodeMonitorPeriod, stopCh)
	<-stopCh
}

func (c *Controller) updateNodes() {
	nodes, err := c.nodeLister.List(labels.Everything())
	if err != nil {
		glog.Errorf("Error listing nodes: %v", err)
		return
	}
	for _, node := range nodes {
		c.updateNode(node)
	}
}

func (c *Controller) updateNode(node *v1.Node) {
	// Default status to v1.ConditionUnknown (same as the default "fake" status from tryUpdateNodeHealth)
	status := v1.ConditionUnknown
	if _, c := v1node.GetNodeCondition(&node.Status, v1.NodeReady); c != nil {
		status = c.Status
	}

	// Skip healthy nodes.
	if status == v1.ConditionTrue {
		return
	}

	// Check if the node was shut down in the cloud provider. If it was
	// shut down, add a taint instead of deleting it.
	if shutdown, err := nodeutil.ShutdownInCloudProvider(context.TODO(), c.cloud, node); err != nil {
		glog.Errorf("Error determining if node %v shutdown in cloud: %v", node.Name, err)
	} else if shutdown {
		// TODO(mtaufen): shouldn't we launch this in a goroutine too, if we're worried about blocking?
		if err = controller.AddOrUpdateTaintOnNode(c.kubeClient, node.Name, controller.ShutdownTaint); err != nil {
			glog.Errorf("Error patching node taints: %v", err)
		}
		return
	}

	// Check whether the node no longer exists in the cloud provider.
	// If not, immediately delete the corresponding Kubernetes Node object.
	if exists, err := nodeutil.ExistsInCloudProvider(c.cloud, types.NodeName(node.Name)); err != nil {
		glog.Errorf("Error determining if node %v exists in cloud: %v", node.Name, err)
	} else if !exists {
		// TODO(mtaufen): shouldn't the event type (DeletingNode) be a constant in some shared library?
		// Record DeletingNode event - note this also logs the event via glog.V(2).Infof.
		nodeutil.RecordNodeEvent(c.recorder, node.Name, string(node.UID), v1.EventTypeNormal, "DeletingNode",
			fmt.Sprintf("Deleting Node %v because it's not present according to cloud provider", node.Name))

		// Delete the Node object from a goroutine, so we don't block processing the rest of the nodes.
		go func(nodeName string) {
			// Since the node no longer exists in the cloud provider, and ready condition indicates
			// Kubelet is not updating Node status, we can delete the Node object without worrying
			// about the node startup grace period.
			if err := nodeutil.ForcefullyDeleteNode(c.kubeClient, nodeName); err != nil {
				glog.Errorf("Unable to forcefully delete Node %q: %v", nodeName, err)
			}
		}(node.Name)
	}
}
