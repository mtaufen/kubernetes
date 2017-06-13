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
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
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

	// configOKMux is a mutex on the ConfigOK node condition (including configOKNeedsSync).
	// We must take turns between writing the condition to the configOK variable and syncing
	// the condition to the API server.
	configOKMux sync.Mutex

	// pendingConfigOK; write to this channel to indicate that ConfigOK needs to be synced to the API server
	pendingConfigOK chan bool

	// pendingConfigSource; write to this channel to indicate that the config source needs to be synced from the API server
	pendingConfigSource chan bool

	// informer is the informer that watches the Node object
	informer cache.SharedInformer

	// checkpointSaver saves config source checkpoints to disk
	checkpointStore checkpointStore
}

// NewNodeConfigController constructs a new NodeConfigController object and returns it.
// If the client is nil, dynamic configuration (watching the API server) will not be used.
func NewNodeConfigController(configDir string, defaultConfig *componentconfig.KubeletConfiguration) *NodeConfigController {
	return &NodeConfigController{
		configDir:     configDir,
		defaultConfig: defaultConfig,
		// channels must have capacity at least 1, since we signal with non-blocking writes
		pendingConfigOK:     make(chan bool, 1),
		pendingConfigSource: make(chan bool, 1),
		checkpointStore: &fsCheckpointStore{
			checkpointsDir: filepath.Join(configDir, checkpointsDir),
		},
	}
}

// Bootstrap initiates operation of the NodeConfigController.
// If a valid configuration is found as a result of these operations, that configuration is returned.
// If a valid configuration cannot be found, an error is returned. This indicates the Kubelet should not continue,
// because it could not determine a valid configuration.
// Bootstrap must be called synchronously during Kubelet startup, before any KubeletConfiguration is used.
// If Bootstrap completes successfully, you can optionally call StartSyncLoop to watch the API server for config updates.
func (cc *NodeConfigController) Bootstrap() (finalConfig *componentconfig.KubeletConfiguration, fatalErr error) {
	var curUID string

	// return any controller-level panics as bootstrapping errors
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

	// ALWAYS validate the default and init configs. This makes incorrectly provisioned nodes an error.
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
		cc.informer = newSharedNodeInformer(cc.client, cc.nodeName,
			cc.onAddNodeEvent, cc.onUpdateNodeEvent, cc.onDeleteNodeEvent)

		// start the informer loop
		// Rather than use utilruntime.HandleCrash, which doesn't actually crash in the Kubelet,
		// we use mkHandlePanic to manually call the panic handlers and then crash.
		// We have a better chance of recovering normal operation if we just restart the Kubelet in the event
		// of a Go runtime error.
		go mkHandlePanic(func() {
			infof("starting informer sync loop")
			cc.informer.Run(wait.NeverStop)
		})()

		// start the config source sync loop
		go mkHandlePanic(func() {
			infof("starting config sync loop")
			wait.JitterUntil(mkRecoverControllerPanic(cc.syncConfigSource), 10*time.Second, 0.2, true, wait.NeverStop)
		})()

		// start the ConfigOK condition sync loop
		go mkHandlePanic(func() {
			infof("starting status sync loop")
			wait.JitterUntil(mkRecoverControllerPanic(cc.syncConfigOK), 10*time.Second, 0.2, true, wait.NeverStop)
		})()
	} else {
		errorf("cannot start sync loop with nil client")
	}
}
