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

package status

import (
	"fmt"
	"strings"
	"sync"
	"time"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset"
	utilequal "k8s.io/kubernetes/pkg/kubelet/kubeletconfig/util/equal"
	utillog "k8s.io/kubernetes/pkg/kubelet/kubeletconfig/util/log"
)

const (
	configOKType = "ConfigOK"

	CurDefaultMessage = "using current (default)"
	LkgDefaultMessage = "using last-known-good (default)"

	CurInitMessage = "using current (init)"
	LkgInitMessage = "using last-known-good (init)"

	CurRemoteMessageFmt = "using current (UID: %q)"
	LkgRemoteMessageFmt = "using last-known-good (UID: %q)"

	CurDefaultOKReason = "current is set to the local default, and no init config was provided"
	CurInitOKReason    = "current is set to the local default, and an init config was provided"
	CurRemoteOKReason  = "passed all checks"

	CurFailLoadReasonFmt      = "failed to load current (UID: %q)"
	CurFailVerifyReasonFmt    = "failed to verify current (UID: %q)"
	CurFailParseReasonFmt     = "failed to parse current (UID: %q)"
	CurFailValidateReasonFmt  = "failed to validate current (UID: %q)"
	CurFailCrashLoopReasonFmt = "current failed trial period due to crash loop (UID %q)"

	LkgFailLoadReasonFmt     = "failed to load last-known-good (UID: %q)"
	LkgFailVerifyReasonFmt   = "failed to verify last-known-good (UID: %q)"
	LkgFailParseReasonFmt    = "failed to parse last-known-good (UID: %q)"
	LkgFailValidateReasonFmt = "failed to validate last-known-good (UID: %q)"

	emptyMessage = "unknown - message not provided"
	emptyReason  = "unknown - reason not provided"
)

// ConfigOKCondition represents a ConfigOK NodeCondition
type ConfigOKCondition interface {
	// Set sets the Message, Reason, and Status of the condition
	Set(message, reason string, status apiv1.ConditionStatus)
	// SetFailedSyncCondition sets the condition for when syncing Kubelet config fails
	SetFailedSyncCondition(reason string)
	// ClearFailedSyncCondition resets ConfigOKCondition to the correct condition for successfully syncing the kubelet config
	ClearFailedSyncCondition(current string, lastKnownGood string, currentBadReason string, initConfig bool)
	// Sync patches the current condition into the Node identified by `nodeName`
	Sync(client clientset.Interface, nodeName string)
}

// configOKCondition implements ConfigOKCondition
type configOKCondition struct {
	// conditionMux is a mutex on the condition, alternate between setting and syncing the condition
	conditionMux sync.Mutex
	// condition is the current ConfigOK node condition, which will be reported in the Node.status.conditions
	condition *apiv1.NodeCondition
	// pendingCondition; write to this channel to indicate that ConfigOK needs to be synced to the API server
	pendingCondition chan bool
}

// NewConfigOKCondition returns a new ConfigOKCondition
func NewConfigOKCondition() ConfigOKCondition {
	return &configOKCondition{
		// channels must have capacity at least 1, since we signal with non-blocking writes
		pendingCondition: make(chan bool, 1),
	}
}

// unsafe_set sets the current state of the condition
// it does not grab the conditionMux lock, so you should generally use setConfigOK unless you need to grab the lock
// at a higher level to synchronize additional operations
func (c *configOKCondition) unsafe_set(message, reason string, status apiv1.ConditionStatus) {
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

	c.condition = &apiv1.NodeCondition{
		Message: message,
		Reason:  reason,
		Status:  status,
		Type:    configOKType,
	}

	c.pokeSyncWorker()
}

func (c *configOKCondition) Set(message, reason string, status apiv1.ConditionStatus) {
	c.conditionMux.Lock()
	defer c.conditionMux.Unlock()
	c.unsafe_set(message, reason, status)
}

// SetFailedSyncCondition updates the ConfigOK status to reflect that we failed to sync to the latest config because we couldn't figure out what
// config to use (e.g. due to a malformed reference, a download failure, etc)
func (c *configOKCondition) SetFailedSyncCondition(reason string) {
	c.Set(c.condition.Message, fmt.Sprintf("failed to sync, desired config unclear, reason: %s", reason), apiv1.ConditionUnknown)
}

// ClearFailedSyncCondition resets ConfigOK to the correct condition for the config UIDs
// `current` and `lastKnownGood`, depending on whether current is bad (non-empty `currentBadReason`)
// and whether an init config exists (`initConfig` is true).
func (c *configOKCondition) ClearFailedSyncCondition(current string,
	lastKnownGood string,
	currentBadReason string,
	initConfig bool) {
	// since our reason-check relies on c.condition we must manually take the lock and use c.unsafe_set instead of c.Set
	c.conditionMux.Lock()
	defer c.conditionMux.Unlock()
	if strings.Contains(c.condition.Reason, "failed to sync, desired config unclear") {
		// if we should report a "current is bad, rolled back" state
		if len(currentBadReason) > 0 {
			if len(current) == 0 {
				if initConfig {
					c.unsafe_set(LkgInitMessage, currentBadReason, apiv1.ConditionFalse)
					return
				}
				c.unsafe_set(LkgDefaultMessage, currentBadReason, apiv1.ConditionFalse)
				return
			}
			c.unsafe_set(fmt.Sprintf(LkgRemoteMessageFmt, lastKnownGood), currentBadReason, apiv1.ConditionFalse)
			return
		}
		// if we should report a "current is ok" state
		if len(current) == 0 {
			if initConfig {
				c.unsafe_set(CurInitMessage, CurInitOKReason, apiv1.ConditionTrue)
				return
			}
			c.unsafe_set(CurDefaultMessage, CurDefaultOKReason, apiv1.ConditionTrue)
			return
		}
		c.unsafe_set(fmt.Sprintf(CurRemoteMessageFmt, current), CurRemoteOKReason, apiv1.ConditionTrue)
	}
}

// pokeSyncWorker notes that the ConfigOK condition needs to be synced to the API server
func (c *configOKCondition) pokeSyncWorker() {
	select {
	case c.pendingCondition <- true:
	default:
	}
}

// Sync attempts to sync `c.condition` with the Node object for this Kubelet,
// if syncing fails, an error is logged, and work is queued for retry.
func (c *configOKCondition) Sync(client clientset.Interface, nodeName string) {
	select {
	case <-c.pendingCondition:
	default:
		// no work to be done, return
		return
	}

	// grab the lock
	c.conditionMux.Lock()
	defer c.conditionMux.Unlock()

	// if the sync fails, we want to retry
	var err error
	defer func() {
		if err != nil {
			utillog.Errorf(err.Error())
			c.pokeSyncWorker()
		}
	}()

	if c.condition == nil {
		utillog.Infof("ConfigOK condition is nil, skipping ConfigOK sync")
		return
	}

	// get the Node so we can check the current condition
	node, err := client.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("could not get Node %q, will not sync ConfigOK condition, error: %v", nodeName, err)
		return
	}

	// set timestamps
	syncTime := metav1.NewTime(time.Now())
	c.condition.LastHeartbeatTime = syncTime
	if remote := getConfigOK(node.Status.Conditions); remote == nil || !utilequal.ConfigOKEq(remote, c.condition) {
		// update transition time the first time we create the condition,
		// or if we are semantically changing the condition
		c.condition.LastTransitionTime = syncTime
	} else {
		// since the conditions are semantically equal, use lastTransitionTime from the condition currently on the Node
		// we need to do this because the field will always be represented in the patch generated below, and this copy
		// prevents nullifying the field during the patch operation
		c.condition.LastTransitionTime = remote.LastTransitionTime
	}

	// generate the patch
	mediaType := "application/json"
	info, ok := kuberuntime.SerializerInfoForMediaType(api.Codecs.SupportedMediaTypes(), mediaType)
	if !ok {
		err = fmt.Errorf("unsupported media type %q", mediaType)
		return
	}
	versions := api.Registry.EnabledVersionsForGroup(api.GroupName)
	if len(versions) == 0 {
		err = fmt.Errorf("no enabled versions for group %q", api.GroupName)
		return
	}
	// the "best" version supposedly comes first in the list returned from apiv1.Registry.EnabledVersionsForGroup
	encoder := api.Codecs.EncoderForVersion(info.Serializer, versions[0])

	before, err := kuberuntime.Encode(encoder, node)
	if err != nil {
		err = fmt.Errorf(`failed to encode "before" node while generating patch, error: %v`, err)
		return
	}

	patchConfigOK(node, c.condition)
	after, err := kuberuntime.Encode(encoder, node)
	if err != nil {
		err = fmt.Errorf(`failed to encode "after" node while generating patch, error: %v`, err)
		return
	}

	patch, err := strategicpatch.CreateTwoWayMergePatch(before, after, apiv1.Node{})
	if err != nil {
		err = fmt.Errorf("failed to generate patch for updating ConfigOK condition, error: %v", err)
		return
	}

	// patch the remote Node object
	_, err = client.CoreV1().Nodes().PatchStatus(nodeName, patch)
	if err != nil {
		err = fmt.Errorf("could not update ConfigOK condition, error: %v", err)
		return
	}
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

// getConfigOK returns the first NodeCondition in `cs` with Type == configOKType,
// or if no such condition exists, returns nil.
func getConfigOK(cs []apiv1.NodeCondition) *apiv1.NodeCondition {
	for i := range cs {
		if cs[i].Type == configOKType {
			return &cs[i]
		}
	}
	return nil
}
