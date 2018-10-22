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
	"testing"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	cloudprovider "k8s.io/cloud-provider"
	fakecloud "k8s.io/kubernetes/pkg/cloudprovider/providers/fake"
	"k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/testutil"
	schedulerapi "k8s.io/kubernetes/pkg/scheduler/api"
)

func TestIt(t *testing.T) {

}

/*


import (
	"context"
	"strings"
	"testing"
	"time"

	coordv1beta1 "k8s.io/api/coordination/v1beta1"
	"k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/wait"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	utilfeaturetesting "k8s.io/apiserver/pkg/util/feature/testing"
	"k8s.io/client-go/informers"
	coordinformers "k8s.io/client-go/informers/coordination/v1beta1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	extensionsinformers "k8s.io/client-go/informers/extensions/v1beta1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	testcore "k8s.io/client-go/testing"
	cloudprovider "k8s.io/cloud-provider"
	fakecloud "k8s.io/kubernetes/pkg/cloudprovider/providers/fake"
	"k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/nodelifecycle/scheduler"
	"k8s.io/kubernetes/pkg/controller/testutil"
	nodeutil "k8s.io/kubernetes/pkg/controller/util/node"
	"k8s.io/kubernetes/pkg/features"
	kubeletapis "k8s.io/kubernetes/pkg/kubelet/apis"
	schedulerapi "k8s.io/kubernetes/pkg/scheduler/api"
	"k8s.io/kubernetes/pkg/util/node"
	taintutils "k8s.io/kubernetes/pkg/util/taints"
	"k8s.io/utils/pointer"
)

*/

// TODO(mtaufen): remove unused constants
const (
	// testNodeMonitorGracePeriod = 40 * time.Second
	// testNodeStartupGracePeriod = 60 * time.Second
	testNodeMonitorPeriod = 5 * time.Second
	// testRateLimiterQPS         = float32(10000)
	// testLargeClusterThreshold  = 20
	// testUnhealthyThreshold     = float32(0.55)
)

func alwaysReady() bool { return true }

type nodeLifecycleController struct {
	*Controller
	nodeInformer coreinformers.NodeInformer
}

func (nc *nodeLifecycleController) syncNodeStore(fakeNodeHandler *testutil.FakeNodeHandler) error {
	nodes, err := fakeNodeHandler.List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	newElems := make([]interface{}, 0, len(nodes.Items))
	for i := range nodes.Items {
		newElems = append(newElems, &nodes.Items[i])
	}
	return nc.nodeInformer.Informer().GetStore().Replace(newElems, "newRV")
}

func newFakeController(cloud cloudprovider.Interface, kubeClient clientset.Interface, nodeMonitorPeriod time.Duration) *nodeLifecycleController {
	nodeInformer := informers.NewSharedInformerFactory(kubeClient, controller.NoResyncPeriodFunc()).Core().V1().Nodes()
	c := NewCloudNodeLifecycleController(cloud, kubeClient, nodeInformer, nodeMonitorPeriod)
	c.nodeInformerSynced = alwaysReady
	// TODO(mtaufen): refactor to hopefully get rid of this wrapper
	return &nodeLifecycleController{c, nodeInformer}
}

func TestCloudProviderNodeShutdown(t *testing.T) {
	testCases := []struct {
		testName string
		node     *v1.Node
		shutdown bool
	}{
		{
			testName: "node shutdowned add taint",
			shutdown: true,
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node0",
					// TODO(mtaufen): can get rid of timestamps; they don't matter to this test now
					CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				Spec: v1.NodeSpec{
					ProviderID: "node0",
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   v1.NodeReady,
							Status: v1.ConditionUnknown,
							// TODO(mtaufen): can get rid of timestamps; they don't matter to this test now
							LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						},
					},
				},
			},
		},
		{
			testName: "node started after shutdown remove taint",
			shutdown: false,
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node0",
					// TODO(mtaufen): can get rid of timestamps; they don't matter to this test now
					CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				Spec: v1.NodeSpec{
					ProviderID: "node0",
					Taints: []v1.Taint{
						{
							Key:    schedulerapi.TaintNodeShutdown,
							Effect: v1.TaintEffectNoSchedule,
						},
					},
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   v1.NodeReady,
							Status: v1.ConditionTrue,
							// TODO(mtaufen): can get rid of timestamps; they don't matter to this test now
							LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			// TODO(mtaufen): finish making this work
			fakeCloud := &fakecloud.FakeCloud{
				ExistsByProviderID: true,
				NodeShutdown:       tc.shutdown,
			}
			fakeClient := &testutil.FakeNodeHandler{
				Existing:  []*v1.Node{tc.node},
				Clientset: fake.NewSimpleClientset(),
			}
			nodeController := newFakeController(fakeCloud, fakeClient, testNodeMonitorPeriod)
			// TODO(mtaufen): better if we can inject this via a constructor?
			nodeController.recorder = testutil.NewFakeRecorder()

			if err := nodeController.syncNodeStore(fakeClient); err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// TODO(mtaufen): call updateNodes() here instead of monitorNodeHealth,
			// also if we move the taint call into a goroutine, we will have to wait
			// for the update later in this test.
			// TODO(mtaufen): refactor updateNodes to also return errors, so we can test for them?
			nodeController.updateNodes()

			if len(fakeClient.UpdatedNodes) != 1 {
				t.Fatalf("Node was not updated")
			}
			if tc.shutdown {
				if len(fakeClient.UpdatedNodes[0].Spec.Taints) != 1 {
					t.Errorf("Node Taint was not added")
				}
				if fakeClient.UpdatedNodes[0].Spec.Taints[0].Key != "node.cloudprovider.kubernetes.io/shutdown" {
					t.Errorf("Node Taint key is not correct")
				}
			} else {
				if len(fakeClient.UpdatedNodes[0].Spec.Taints) != 0 {
					t.Errorf("Node Taint was not removed after node is back in ready state")
				}
			}
		})
	}

}

/*

// TestCloudProviderNoRateLimit tests that monitorNodes() immediately deletes
// pods and the node when kubelet has not reported, and the cloudprovider says
// the node is gone.
func TestCloudProviderNoRateLimit(t *testing.T) {
	fnh := &testutil.FakeNodeHandler{
		Existing: []*v1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:               v1.NodeReady,
							Status:             v1.ConditionUnknown,
							LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						},
					},
				},
			},
		},
		Clientset:      fake.NewSimpleClientset(&v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0"), *testutil.NewPod("pod1", "node0")}}),
		DeleteWaitChan: make(chan struct{}),
	}
	nodeController, _ := newNodeLifecycleControllerFromClient(
		nil,
		fnh,
		10*time.Minute,
		testRateLimiterQPS,
		testRateLimiterQPS,
		testLargeClusterThreshold,
		testUnhealthyThreshold,
		testNodeMonitorGracePeriod,
		testNodeStartupGracePeriod,
		testNodeMonitorPeriod,
		false)
	nodeController.cloud = &fakecloud.FakeCloud{}
	nodeController.now = func() metav1.Time { return metav1.Date(2016, 1, 1, 12, 0, 0, 0, time.UTC) }
	nodeController.recorder = testutil.NewFakeRecorder()
	nodeController.nodeExistsInCloudProvider = func(nodeName types.NodeName) (bool, error) {
		return false, nil
	}
	nodeController.nodeShutdownInCloudProvider = func(ctx context.Context, node *v1.Node) (bool, error) {
		return false, nil
	}
	// monitorNodeHealth should allow this node to be immediately deleted
	if err := nodeController.syncNodeStore(fnh); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := nodeController.monitorNodeHealth(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	select {
	case <-fnh.DeleteWaitChan:
	case <-time.After(wait.ForeverTestTimeout):
		t.Errorf("Timed out waiting %v for node to be deleted", wait.ForeverTestTimeout)
	}
	if len(fnh.DeletedNodes) != 1 || fnh.DeletedNodes[0].Name != "node0" {
		t.Errorf("Node was not deleted")
	}
	if nodeOnQueue := nodeController.zonePodEvictor[""].Remove("node0"); nodeOnQueue {
		t.Errorf("Node was queued for eviction. Should have been immediately deleted.")
	}
}

*/

// TODO(mtaufen): for this one, strip down to just the events generated by the new controller
/*

func TestNodeEventGeneration(t *testing.T) {
	fakeNow := metav1.Date(2016, 9, 10, 12, 0, 0, 0, time.UTC)
	fakeNodeHandler := &testutil.FakeNodeHandler{
		Existing: []*v1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					UID:               "1234567890",
					CreationTimestamp: metav1.Date(2015, 8, 10, 0, 0, 0, 0, time.UTC),
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:               v1.NodeReady,
							Status:             v1.ConditionUnknown,
							LastHeartbeatTime:  metav1.Date(2015, 8, 10, 0, 0, 0, 0, time.UTC),
							LastTransitionTime: metav1.Date(2015, 8, 10, 0, 0, 0, 0, time.UTC),
						},
					},
				},
			},
		},
		Clientset: fake.NewSimpleClientset(&v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}}),
	}

	nodeController, _ := newNodeLifecycleControllerFromClient(
		nil,
		fakeNodeHandler,
		5*time.Minute,
		testRateLimiterQPS,
		testRateLimiterQPS,
		testLargeClusterThreshold,
		testUnhealthyThreshold,
		testNodeMonitorGracePeriod,
		testNodeStartupGracePeriod,
		testNodeMonitorPeriod,
		false)
	nodeController.cloud = &fakecloud.FakeCloud{}
	nodeController.nodeExistsInCloudProvider = func(nodeName types.NodeName) (bool, error) {
		return false, nil
	}
	nodeController.nodeShutdownInCloudProvider = func(ctx context.Context, node *v1.Node) (bool, error) {
		return false, nil
	}
	nodeController.now = func() metav1.Time { return fakeNow }
	fakeRecorder := testutil.NewFakeRecorder()
	nodeController.recorder = fakeRecorder
	if err := nodeController.syncNodeStore(fakeNodeHandler); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := nodeController.monitorNodeHealth(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(fakeRecorder.Events) != 2 {
		t.Fatalf("unexpected events, got %v, expected %v: %+v", len(fakeRecorder.Events), 2, fakeRecorder.Events)
	}
	if fakeRecorder.Events[0].Reason != "RegisteredNode" || fakeRecorder.Events[1].Reason != "DeletingNode" {
		var reasons []string
		for _, event := range fakeRecorder.Events {
			reasons = append(reasons, event.Reason)
		}
		t.Fatalf("unexpected events generation: %v", strings.Join(reasons, ","))
	}
	for _, event := range fakeRecorder.Events {
		involvedObject := event.InvolvedObject
		actualUID := string(involvedObject.UID)
		if actualUID != "1234567890" {
			t.Fatalf("unexpected event uid: %v", actualUID)
		}
	}
}
*/
