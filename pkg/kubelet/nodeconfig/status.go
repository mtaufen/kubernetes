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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/kubernetes/pkg/api"
	apiv1 "k8s.io/kubernetes/pkg/api/v1"
)

const (
	configOKType = "ConfigOK"

	curDefaultMessage = "using current (default)"
	lkgDefaultMessage = "using last-known-good (default)"

	curInitMessage = "using current (init)"
	lkgInitMessage = "using last-known-good (init)"

	curRemoteMessageFmt = "using current (UID: %q)"
	lkgRemoteMessageFmt = "using last-known-good (UID: %q)"

	curDefaultOKReason = "current is set to the local default, and no init config was provided"
	curInitOKReason    = "current is set to the local default, and an init config was provided"
	curRemoteOKReason  = "passed all checks"

	curFailLoadReasonFmt      = "failed to load current (UID: %q)"
	curFailVerifyReasonFmt    = "failed to verify current (UID: %q)"
	curFailParseReasonFmt     = "failed to parse current (UID: %q)"
	curFailValidateReasonFmt  = "failed to validate current (UID: %q)"
	curFailCrashLoopReasonFmt = "current failed trial period due to crash loop (UID %q)"

	lkgFailLoadReasonFmt     = "failed to load last-known-good (UID: %q)"
	lkgFailVerifyReasonFmt   = "failed to verify last-known-good (UID: %q)"
	lkgFailParseReasonFmt    = "failed to parse last-known-good (UID: %q)"
	lkgFailValidateReasonFmt = "failed to validate last-known-good (UID: %q)"

	emptyMessage = "unknown - message not provided"
	emptyReason  = "unknown - reason not provided"
)

// fatalSyncConfigOK attempts to sync a ConfigOK status describing a fatal error.
// It is typical to call fatalf after fatalSyncConfigOK.
func (cc *NodeConfigController) fatalSyncConfigOK(reason string) {
	cc.setConfigOK("fatal-class error occurred while resolving config", reason, apiv1.ConditionFalse)
	cc.syncConfigOK()
}

// setConfigOK constructs a new ConfigOK NodeCondition and sets it on the NodeConfigController
// this is the function that grabs the lock, so in most situations this is the one that should be used
func (cc *NodeConfigController) setConfigOK(message, reason string, status apiv1.ConditionStatus) {
	cc.configOKMux.Lock()
	defer cc.configOKMux.Unlock()
	cc.unsafe_setConfigOK(message, reason, status)
}

// unsafe_setConfigOK constructs a new ConfigOK NodeCondition and sets it on the NodeConfigController
// it does not grab the configOKMux lock, so you should generally use setConfigOK unless you need to grab the lock
// at a higher level to synchronize additional operations
func (cc *NodeConfigController) unsafe_setConfigOK(message, reason string, status apiv1.ConditionStatus) {
	// We avoid an empty Message, Reason, or Status on the condition. Since we use Patch to update conditions, an empty
	// field might cause a value from a previous condition to leak through, which can be very confusing.
	if len(message) == 0 {
		message = emptyMessage
	}
	if len(reason) == 0 {
		reason = emptyReason
	}
	if len(string(status)) == 0 {
		status = apiv1.ConditionUnknown
	}

	cc.configOK = &apiv1.NodeCondition{
		Message: message,
		Reason:  reason,
		Status:  status,
		Type:    configOKType,
	}

	cc.configOKNeedsSync = true
}

// syncConfigOK attempts to sync `cc.configOK` with the Node object for this Kubelet.
// If syncing fails, an error is logged.
func (cc *NodeConfigController) syncConfigOK() {
	cc.configOKMux.Lock()
	defer cc.configOKMux.Unlock()

	if !cc.configOKNeedsSync {
		return
	} else if cc.client == nil {
		infof("client is nil, skipping ConfigOK sync")
		return
	} else if cc.configOK == nil {
		infof("ConfigOK condition is nil, skipping ConfigOK sync")
		return
	}

	// get the Node so we can check the current condition
	node, err := cc.client.CoreV1().Nodes().Get(cc.nodeName, metav1.GetOptions{})
	if err != nil {
		errorf("could not get Node %q, will not sync ConfigOK condition, error: %v", cc.nodeName, err)
		return
	}

	// set timestamps
	syncTime := metav1.NewTime(time.Now())
	cc.configOK.LastHeartbeatTime = syncTime
	if c := getConfigOK(node.Status.Conditions); c == nil || !configOKEq(c, cc.configOK) {
		// update transition time the first time we create the condition,
		// or if we are semantically changing the condition
		cc.configOK.LastTransitionTime = syncTime
	} else {
		// since the conditions are semantically equal, use lastTransitionTime from the condition currently on the Node
		// we need to do this because the field will always be represented in the patch generated below, and this copy
		// prevents nullifying the field during the patch operation
		cc.configOK.LastTransitionTime = c.LastTransitionTime
	}

	// generate the patch
	mediaType := "application/json"
	info, ok := kuberuntime.SerializerInfoForMediaType(api.Codecs.SupportedMediaTypes(), mediaType)
	if !ok {
		errorf("unsupported media type %q", mediaType)
		return
	}
	versions := api.Registry.EnabledVersionsForGroup(api.GroupName)
	if len(versions) == 0 {
		errorf("no enabled versions for group %q", api.GroupName)
		return
	}
	// the "best" version supposedly comes first in the list returned from apiv1.Registry.EnabledVersionsForGroup
	encoder := api.Codecs.EncoderForVersion(info.Serializer, versions[0])

	before, err := kuberuntime.Encode(encoder, node)
	if err != nil {
		errorf("failed to encode before node, error: %v", err)
		return
	}

	patchConfigOK(node, cc.configOK)
	after, err := kuberuntime.Encode(encoder, node)
	if err != nil {
		errorf("failed to encode after node, error: %v", err)
		return
	}

	// generate the patch
	patch, err := strategicpatch.CreateTwoWayMergePatch(before, after, apiv1.Node{})
	if err != nil {
		errorf("failed to generate patch for updating ConfigOK condition, error: %v", err)
		return
	}

	// patch the remote Node object
	_, err = cc.client.CoreV1().Nodes().PatchStatus(cc.nodeName, patch)
	if err != nil {
		errorf("could not update ConfigOK condition, error: %v", err)
		return
	}

	// if the sync succeeded, unset configOKNeedsSync
	cc.configOKNeedsSync = false
}

// patchConfigOK replaces or adds the ConfigOK condition to the node
func patchConfigOK(node *apiv1.Node, configOK *apiv1.NodeCondition) {
	for i := range node.Status.Conditions {
		if node.Status.Conditions[i].Type == configOKType {
			// edit the condition
			node.Status.Conditions[i] = *configOK
			return
		}
	}
	// append the condition
	node.Status.Conditions = append(node.Status.Conditions, *configOK)
}

// configOKEq returns true if the conditions' messages, reasons, and statuses match, false otherwise.
func configOKEq(a, b *apiv1.NodeCondition) bool {
	return a.Message == b.Message && a.Reason == b.Reason && a.Status == b.Status
}

// getConfigOK returns the first NodeCondition in `cs` with Type == configOKType.
// If no such condition exists, returns nil.
func getConfigOK(cs []apiv1.NodeCondition) *apiv1.NodeCondition {
	for i := range cs {
		if cs[i].Type == configOKType {
			return &cs[i]
		}
	}
	return nil
}
