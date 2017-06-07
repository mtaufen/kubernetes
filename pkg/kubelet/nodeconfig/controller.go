/*
Copyright 2017 The Kubernetes Authors.

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

package nodeconfig

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	apiv1 "k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/apis/componentconfig"
	"k8s.io/kubernetes/pkg/apis/componentconfig/validation"
	clientset "k8s.io/kubernetes/pkg/client/clientset_generated/clientset"
)

// NodeConfigController is the controller which, among other things:
// - loads configuration from disk
// - checkpoints configuration to disk
// - downloads new configuration from the API server
// - validates configuration
// - monitors for potential crash-loops caused by new configurations
// - tracks the last-known-good configuration, and rolls-back to last-known-good when necessary
// For more information, see the proposal: https://github.com/kubernetes/kubernetes/pull/29459
type NodeConfigController struct {
	// configDir is the root directory to use for nodeconfig management
	configDir string

	// defaultConfig is the configuration to use if no initConfig is provided
	defaultConfig *componentconfig.KubeletConfiguration

	// initConfig is the unmarshaled init config, this will be loaded by the NodeConfigController if it exists in the configDir
	initConfig *componentconfig.KubeletConfiguration

	// client is the clientset for talking to the apiserver.
	client clientset.Interface

	// nodeName is the name of the Node object we should monitor for config
	nodeName string

	// configOK is the current ConfigOK node condition, which will be reported in the Node.status.conditions
	configOK *apiv1.NodeCondition

	// configOKNeedsSync indicates whether the local configOK still needs to be pushed to the API server
	configOKNeedsSync bool

	// configOKMux is a mutex on the ConfigOK node condition (including configOKNeedsSync).
	// We must take turns between writing the condition to the configOK variable and syncing
	// the condition to the API server.
	configOKMux sync.Mutex
}

// NewNodeConfigController constructs a new NodeConfigController object and returns it.
// If the client is nil, dynamic configuration (watching the API server) will not be used.
func NewNodeConfigController(configDir string, defaultConfig *componentconfig.KubeletConfiguration) *NodeConfigController {
	return &NodeConfigController{
		configDir:     configDir,
		defaultConfig: defaultConfig,
	}
}

// Bootstrap initiates operation of the NodeConfigController.
// If a valid configuration is found as a result of these operations, that configuration is returned.
// If a valid configuration cannot be found, a fatal error occurs, preventing the Kubelet from continuing with invalid configuration.
// Bootstrap must be called synchronously during Kubelet startup, before any KubeletConfiguration is used.
// If Bootstrap completes successfully, you can optionally call StartSyncLoop to watch the API server for config updates.
func (cc *NodeConfigController) Bootstrap() (finalConfig *componentconfig.KubeletConfiguration, fatalErr error) {
	var curUID string

	// defer updating status until the end of Run
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(runtime.Error); ok {
				panic(r)
			}
			fatalErr = r.(error)
		}
	}()

	infof("starting controller")

	// make sure the node-config-dir is set up properly
	cc.ensureCfgDir()

	// record the kubelet startup time, used for crashloop detection
	cc.recordStartup()

	// ALWAYS validate the default and init configs. This makes incorrectly provisioned nodes a fatal error.
	// These must be valid because they are the foundational last-known-good configs.
	infof("validating combination of defaults and flags")
	if err := validation.ValidateKubeletConfiguration(cc.defaultConfig); err != nil {
		panicf("combination of defaults and flags failed validation, error: %v", err)
	}
	cc.loadInitConfig()
	cc.validateInitConfig()
	// Assert: the default and init configs are both valid

	// determine UID of the current config source, empty string if curSymlink targets default
	curUID = cc.curUID()

	// if curUID indicates the default should be used, return initConfig or defaultConfig
	if len(curUID) == 0 {
		if cc.initConfig != nil {
			cc.setConfigOK(curInitMessage, curInitOKReason, apiv1.ConditionTrue)
			finalConfig = cc.initConfig
			return
		}
		cc.setConfigOK(curDefaultMessage, curDefaultOKReason, apiv1.ConditionTrue)
		finalConfig = cc.defaultConfig
		return
	} // Assert: we will not use the init or default configurations, unless we roll back to lkg; curUID is a real UID

	// check whether the current config is marked bad
	if bad, entry := cc.isBadConfig(curUID); bad {
		infof("current config %q was marked bad for reason %q at time %q", curUID, entry.Reason, entry.Time)
		finalConfig = cc.lkgRollback(entry.Reason, apiv1.ConditionFalse)
		return
	}

	// TODO(mtaufen): consider re-verifying integrity and re-attempting download when a load/verify/parse/validate
	// error happens outside trial period, we already made it past the trial so it's probably filesystem corruption
	// or something else scary

	// load the current config
	toVerify, err := cc.loadCheckpoint(curSymlink)
	if err != nil {
		// TODO(mtaufen): rollback and mark bad for now, but this could reasonably be handled by re-attempting a download,
		// it probably indicates some sort of corruption
		finalConfig = cc.badRollback(curUID, fmt.Sprintf(curFailLoadReasonFmt, curUID), fmt.Sprintf("error: %v", err))
		return
	}

	// verify the integrity of the configuration we just loaded
	toParse, err := toVerify.verify()
	if err != nil {
		finalConfig = cc.badRollback(curUID, fmt.Sprintf(curFailVerifyReasonFmt, curUID), fmt.Sprintf("error: %v", err))
		return
	}

	// parse the configuration we just loaded into a KubeletConfiguration
	cur, err := toParse.parse()
	if err != nil {
		finalConfig = cc.badRollback(curUID, fmt.Sprintf(curFailParseReasonFmt, curUID), fmt.Sprintf("error: %v", err))
		return
	}

	// validate current config
	if err := validation.ValidateKubeletConfiguration(cur); err != nil {
		finalConfig = cc.badRollback(curUID, fmt.Sprintf(curFailValidateReasonFmt, curUID), fmt.Sprintf("error: %v", err))
		return
	}

	// check for crash loops if we're still in the trial period
	if cc.curInTrial(cur.ConfigTrialDuration.Duration) {
		if cc.crashLooping(cur.CrashLoopThreshold) {
			finalConfig = cc.badRollback(curUID, fmt.Sprintf(curFailCrashLoopReasonFmt, curUID), "")
			return
		}
	} else if !cc.curIsLkg() {
		// when the trial period is over, the current config becomes the last-known-good
		cc.setCurAsLkg()
	}

	// update the status to note that we will use the current config
	cc.setConfigOK(fmt.Sprintf(curRemoteMessageFmt, curUID), curRemoteOKReason, apiv1.ConditionTrue)
	finalConfig = cur
	return
}

// StartSyncLoop launches the sync loop that watches the API server for configuration updates
// The `client` passed to StartSyncLoop will replace the controller's current client
func (cc *NodeConfigController) StartSyncLoop(client clientset.Interface, nodeName string) {
	if client != nil {
		cc.client = client
		cc.nodeName = nodeName

		fieldselector := fields.OneTermEqualSelector("metadata.name", nodeName)

		// Add some randomness to resync period, which can help avoid controllers falling into lock-step
		minResyncPeriod := 15 * time.Minute
		factor := rand.Float64() + 1
		resyncPeriod := time.Duration(float64(minResyncPeriod.Nanoseconds()) * factor)

		lw := &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (kuberuntime.Object, error) {
				return cc.client.Core().Nodes().List(metav1.ListOptions{
					FieldSelector: fieldselector.String(),
				})
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return cc.client.Core().Nodes().Watch(metav1.ListOptions{
					FieldSelector:   fieldselector.String(),
					ResourceVersion: options.ResourceVersion,
				})
			},
		}

		handler := cache.ResourceEventHandlerFuncs{
			AddFunc: cc.onWatchNodeEvent,
			UpdateFunc: func(old interface{}, nw interface{}) {
				// TODO(mtaufen): do filtering

				cc.onWatchNodeEvent(nw)
			},
			DeleteFunc: func(obj interface{}) {
				cc.onDeleteNodeEvent(obj)
			},
		}

		informer := cache.NewSharedInformer(lw, &apiv1.Node{}, resyncPeriod)
		informer.AddEventHandler(handler)

		// start the configuration sync loop
		go func() {
			infof("starting sync loop")
			defer func() {
				// Rather than use utilruntime.HandleCrash, which doesn't actually crash in the Kubelet,
				// we manually call the panic handlers and then crash. We have a better chance of recovering
				// normal operation if we just restart the Kubelet in this case.
				if r := recover(); r != nil {
					for _, fn := range utilruntime.PanicHandlers {
						fn(r)
					}
					panic(r)
				}
			}()
			informer.Run(wait.NeverStop)
			return
		}()

		// start the ConfigOK condition sync loop
		// TODO(mtaufen): condition syncing is kept separate from informer event handling, because I'm worried about the
		// possibility of creating a feedback loop between condition updates and Node update events.
		// I have yet to test whether a feedback loop is possible, but it's best to be cautious.
		go wait.JitterUntil(cc.syncConfigOK, 10*time.Second, 0.2, true, wait.NeverStop)
	} else {
		errorf("cannot start sync loop with nil client")
	}
}

// onWatchNodeEvent checks for new config and downloads it if necessary.
// If filesystem issues prevent proper operation of syncNodeConfig, a fatal error occurs.
func (cc *NodeConfigController) onWatchNodeEvent(obj interface{}) {
	defer func() {
		// catch controller-level panics, the actual cause of controller-level
		// panic-class errors will have already been logged by panicf (see log.go)
		if r := recover(); r != nil {
			if _, ok := r.(runtime.Error); ok {
				panic(r)
			}
		}
	}()

	node, ok := obj.(*apiv1.Node)
	if !ok {
		errorf("failed to cast watched object to Node, couldn't handle event")
		return
	}

	// check the Node and download any new config
	if updated, cause, err := cc.syncNodeConfig(node); err != nil {
		errorf("failed to sync node config, error: %v", err)
		// Update the ConfigOK status to reflect that we failed to sync, and so we don't know which configuration
		// the user actually wants us to use. In this case, we just continue using the currently-in-use configuration.
		cc.setConfigOK(cc.configOK.Message, fmt.Sprintf("failed to sync, desired config unclear, cause: %s", cause), apiv1.ConditionUnknown)
		return
	} else if updated {
		// TODO(mtaufen): Consider adding a "currently restarting" node condition for this case
		infof("config updated, Kubelet will restart to begin using new config")
		os.Exit(0)
	}

	// If we get here:
	// - there is no need to restart update the current config
	// - there was no error trying to sync configuration
	// - if, previously, there was an error trying to sync configuration,
	//   we need to restore the ConfigOK condition to an error free reason
	//   and condition e.g. "passed all checks" and ConditionTrue.
	// There are 3 possible ConfigOK conditions that set status to ConditionTrue:
	// --------------------------------------------------------------------------------------------------------------
	// message                 | reason                                                               | status
	// --------------------------------------------------------------------------------------------------------------
	// using current (init)    | current is set to the local default, and an init config was provided | ConditionTrue
	// using current (default) | current is set to the local default, and no init config was provided | ConditionTrue
	// using current (UID: %q) | passed all checks                                                    | ConditionTrue
	// --------------------------------------------------------------------------------------------------------------
	// To properly set the reason, we need to determine where .cur currently points, whether the init config exists,
	// and whether the reason for the current condition is a sync error.

	// since our reason-check relies on cc.configOK we must manually take the lock and use cc.unsafe_setConfigOK instead of cc.setConfigOK
	cc.configOKMux.Lock()
	defer cc.configOKMux.Unlock()
	if strings.Contains(cc.configOK.Reason, "failed to sync, desired config unclear") {
		// determine UID of the current config source, empty string if curSymlink targets default
		curUID := cc.curUID()
		if len(curUID) == 0 {
			if cc.initConfig != nil {
				cc.unsafe_setConfigOK(curInitMessage, curInitOKReason, apiv1.ConditionTrue)
			} else {
				cc.unsafe_setConfigOK(curDefaultMessage, curDefaultOKReason, apiv1.ConditionTrue)
			}
		} else {
			cc.unsafe_setConfigOK(fmt.Sprintf(curRemoteMessageFmt, curUID), curRemoteOKReason, apiv1.ConditionTrue)
		}
	}
}

// onDeleteNodeEvent logs a message if the Node was deleted and may log errors
// if an unexpected DeletedFinalStateUnknown was received.
// We explicitly allow the sync-loop to continue, because it is possible that
// the Kubelet detected a Node with unexpected externalID and is attempting
// to delete and re-create the Node (see pkg/kubelet/kubelet_node_status.go).
func (cc *NodeConfigController) onDeleteNodeEvent(obj interface{}) {
	node, ok := obj.(*apiv1.Node)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			errorf("couldn't cast deleted object to DeletedFinalStateUnknown, object: %+v", obj)
			return
		}
		node, ok = tombstone.Obj.(*apiv1.Node)
		if !ok {
			errorf("received DeletedFinalStateUnknown object but it did not contain a Node, object: %+v", obj)
			return
		}
		infof("Node was deleted (DeletedFinalStateUnknown), sync-loop will continue because the Kubelet might recreate the Node, node: %+v", node)
		return
	}
	infof("Node was deleted, sync-loop will continue because the Kubelet might recreate the Node, node: %+v", node)
}
