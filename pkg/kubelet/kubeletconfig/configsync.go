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

package kubeletconfig

import (
	"fmt"
	"os"
	"time"

	"github.com/golang/glog"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/kubelet/kubeletconfig/checkpoint"
	"k8s.io/kubernetes/pkg/kubelet/kubeletconfig/status"
	utillog "k8s.io/kubernetes/pkg/kubelet/kubeletconfig/util/log"
)

const (
	// KubeletConfigChangedEventReason identifies an event as a change of Kubelet configuration
	KubeletConfigChangedEventReason = "KubeletConfigChanged"
	// EventMessageFmt is the message format for Kubelet config change events
	EventMessageFmt = "Kubelet will restart to use: %s"
	// LocalConfigMessage is the text to apply to EventMessageFmt when the Kubelet has been configured to use its local config (init or defaults)
	LocalConfigMessage = "local config"
	// RemoteConfigMessageFmt is the text to apply to EventMessageFmt when the Kubelet has been configured to use its remote config
	// we use KubeletConfigKey instead of KubeletFilename so that this matches the external API for ConfigMap sources, rather than internal interfaces.
	RemoteConfigMessageFmt = "%s, UID: %s, ResourceVersion: %s, KubeletConfigKey: %s"
)

// pokeConfiSourceWorker tells the worker thread that syncs config sources that work needs to be done
func (cc *Controller) pokeConfigSourceWorker() {
	select {
	case cc.pendingConfigSource <- true:
	default:
	}
}

// syncConfigSource checks if work needs to be done to use a new configuration, and does that work if necessary
func (cc *Controller) syncConfigSource(client clientset.Interface, eventClient v1core.EventsGetter, nodeName string) {
	select {
	case <-cc.pendingConfigSource:
	default:
		// no work to be done, return
		return
	}

	// if the sync fails, we want to retry
	var syncerr error
	defer func() {
		if syncerr != nil {
			utillog.Errorf(syncerr.Error())
			cc.pokeConfigSourceWorker()
		}
	}()

	node, err := latestNode(cc.nodeInformer.GetStore(), nodeName)
	if err != nil {
		cc.configOk.SetFailSyncCondition(status.FailSyncReasonInformer)
		syncerr = fmt.Errorf("%s, error: %v", status.FailSyncReasonInformer, err)
		return
	}

	// check the Node and download any new config
	if updated, cur, reason, err := cc.doSyncConfigSource(client, node); err != nil {
		cc.configOk.SetFailSyncCondition(reason)
		syncerr = fmt.Errorf("%s, error: %v", reason, err)
		return
	} else if updated {
		msg := LocalConfigMessage
		if cur != nil {
			msg = fmt.Sprintf(RemoteConfigMessageFmt, cur.APIPath(), cur.UID(), cur.ResourceVersion(), cur.KubeletFilename())
		}
		// we directly log and send the event, instead of using the event recorder,
		// because the event recorder won't flush its queue before we exit (we'd lose the event)
		event := eventf(nodeName, apiv1.EventTypeNormal, KubeletConfigChangedEventReason, EventMessageFmt, msg)
		glog.V(3).Infof("Event(%#v): type: '%v' reason: '%v' %v", event.InvolvedObject, event.Type, event.Reason, event.Message)
		if _, err := eventClient.Events(apiv1.NamespaceDefault).Create(event); err != nil {
			utillog.Errorf("failed to send event, error: %v", err)
		}
		os.Exit(0)
	}

	// If we get here:
	// - there is no need to restart to update the current config
	// - there was no error trying to sync configuration
	// - if, previously, there was an error trying to sync configuration, we need to clear that error from the condition
	cc.configOk.ClearFailSyncCondition()
}

// doSyncConfigSource checkpoints and sets the store's current config to the new config or resets config,
// depending on the `source`, and returns whether the current config in the checkpoint store was updated as a result
func (cc *Controller) doSyncConfigSource(client clientset.Interface, node *apiv1.Node) (bool, checkpoint.RemoteConfigSource, string, error) {
	if node.Spec.ConfigSource == nil {
		utillog.Infof("Node.Spec.ConfigSource is empty, will reset current and last-known-good to defaults")
		updated, reason, err := cc.resetConfig()
		if err != nil {
			return false, nil, reason, err
		}
		return updated, nil, "", nil
	}

	// if the NodeConfigSource is non-nil, download the config
	utillog.Infof("Node.Spec.ConfigSource is non-empty, will checkpoint source and update config if necessary")
	source := checkpoint.NewRemoteConfigSource(node.Spec.ConfigSource)
	reason, err := cc.checkpointConfigSource(client, source)
	if err != nil {
		return false, nil, reason, err
	}
	// Note: cc.checkpointConfigSource calls source.Download internally, so the uid and resourceVersion will be up to date here
	updated, reason, err := cc.setCurrentConfig(source)
	if err != nil {
		return false, nil, reason, err
	}
	return updated, source, "", nil
}

// checkpointConfigSource downloads and checkpoints the object referred to by `source` if the checkpoint does not already exist,
// if a failure occurs, returns a sanitized failure reason and an error
func (cc *Controller) checkpointConfigSource(client clientset.Interface, source checkpoint.RemoteConfigSource) (string, error) {
	// TODO(mtaufen): it would be nice if we could check the payload's metadata before downloading the whole payload
	//                we can try pulling the latest configmap out of the local store, and seeing if it matches the
	//                source specification.
	// download source
	payload, reason, err := source.Download(client)
	if err != nil {
		return reason, fmt.Errorf("%s, error: %v", reason, err)
	}

	// check whether we need to save
	// Note: source has correct uid and resourceVersion after calling Download
	if ok, err := cc.checkpointStore.Exists(source); err != nil {
		reason := fmt.Sprintf(status.FailSyncReasonCheckpointExistenceFmt, source.APIPath(), payload.UID(), payload.ResourceVersion())
		return reason, fmt.Errorf("%s, error: %v", reason, err)
	} else if ok {
		utillog.Infof("checkpoint already exists for object %s with UID %s and ResourceVersion %s", source.APIPath(), payload.UID(), payload.ResourceVersion())
		return "", nil
	}

	// save
	if err = cc.checkpointStore.Save(payload); err != nil {
		reason := fmt.Sprintf(status.FailSyncReasonSaveCheckpointFmt, source.APIPath(), payload.UID(), payload.ResourceVersion())
		return reason, fmt.Errorf("%s, error: %v", reason, err)
	}

	return "", nil
}

// setCurrentConfig sets the current checkpoint config in the store
// returns whether the current config changed as a result, or a sanitized failure reason and an error.
func (cc *Controller) setCurrentConfig(source checkpoint.RemoteConfigSource) (bool, string, error) {
	failReason := func(s checkpoint.RemoteConfigSource) string {
		if source == nil {
			return status.FailSyncReasonSetCurrentLocal
		}
		return fmt.Sprintf(status.FailSyncReasonSetCurrentRemoteFmt, source.APIPath(), source.UID(), source.ResourceVersion())
	}
	current, err := cc.checkpointStore.Current()
	if err != nil {
		return false, failReason(source), err
	}
	if err := cc.checkpointStore.SetCurrent(source); err != nil {
		return false, failReason(source), err
	}
	return !checkpoint.EqualRemoteConfigSources(current, source), "", nil
}

// resetConfig resets the current and last-known-good checkpoints in the checkpoint store to their default values and
// returns whether the current checkpoint changed as a result, or a sanitized failure reason and an error.
func (cc *Controller) resetConfig() (bool, string, error) {
	updated, err := cc.checkpointStore.Reset()
	if err != nil {
		return false, status.FailSyncReasonReset, err
	}
	return updated, "", nil
}

// latestNode returns the most recent Node with `nodeName` from `store`
func latestNode(store cache.Store, nodeName string) (*apiv1.Node, error) {
	obj, ok, err := store.GetByKey(nodeName)
	if err != nil {
		err := fmt.Errorf("failed to retrieve Node %q from informer's store, error: %v", nodeName, err)
		utillog.Errorf(err.Error())
		return nil, err
	} else if !ok {
		err := fmt.Errorf("Node %q does not exist in the informer's store, can't sync config source", nodeName)
		utillog.Errorf(err.Error())
		return nil, err
	}
	node, ok := obj.(*apiv1.Node)
	if !ok {
		err := fmt.Errorf("failed to cast object from informer's store to Node, can't sync config source for Node %q", nodeName)
		utillog.Errorf(err.Error())
		return nil, err
	}
	return node, nil
}

// eventf constructs and returns an event containing a formatted message
// similar to k8s.io/client-go/tools/record/event.go
func eventf(nodeName, eventType, reason, messageFmt string, args ...interface{}) *apiv1.Event {
	return makeEvent(nodeName, eventType, reason, fmt.Sprintf(messageFmt, args...))
}

// makeEvent constructs an event
// similar to makeEvent in k8s.io/client-go/tools/record/event.go
func makeEvent(nodeName, eventtype, reason, message string) *apiv1.Event {
	const componentKubelet = "kubelet"
	// NOTE(mtaufen): This is consistent with pkg/kubelet/kubelet.go. Even though setting the node
	// name as the UID looks strange, it appears to be conventional for events sent by the Kubelet.
	ref := apiv1.ObjectReference{
		Kind:      "Node",
		Name:      nodeName,
		UID:       types.UID(nodeName),
		Namespace: "",
	}

	t := metav1.Time{Time: time.Now()}
	namespace := ref.Namespace
	if namespace == "" {
		namespace = metav1.NamespaceDefault
	}
	return &apiv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%v.%x", ref.Name, t.UnixNano()),
			Namespace: namespace,
		},
		InvolvedObject: ref,
		Reason:         reason,
		Message:        message,
		FirstTimestamp: t,
		LastTimestamp:  t,
		Count:          1,
		Type:           eventtype,
		Source:         apiv1.EventSource{Component: componentKubelet, Host: string(nodeName)},
	}
}
