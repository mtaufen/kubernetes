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
	"os"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiv1 "k8s.io/kubernetes/pkg/api/v1"
	clientset "k8s.io/kubernetes/pkg/client/clientset_generated/clientset"
)

// pokeConfiSource notes that the Node's ConfiSource needs to be synced from the API server
func (cc *NodeConfigController) pokeConfigSource() {
	select {
	case cc.pendingConfigSource <- true:
	default:
	}
}

// TODO(mtaufen): finish refactoring this
func (cc *NodeConfigController) syncConfigSource() {
	select {
	case <-cc.pendingConfigSource:
	default:
		// no work to be done, return
		return
	}

	// look at the store on the informer to get the latest node
	obj, ok, err := cc.informer.GetStore().GetByKey(cc.nodeName)
	if err != nil {
		errorf("failed to retrieve Node %q from informer's store, error: %v", cc.nodeName, err)
		return
	} else if !ok {
		errorf("Node %q does not exist in the informer's store, can't sync config source", cc.nodeName)
		return
	}
	node, ok := obj.(*apiv1.Node)
	if !ok {
		errorf("failed to cast object from informer's store to Node, can't sync config source", cc.nodeName)
		return
	}

	// check the Node and download any new config
	if updated, cause, err := cc.syncConfigSourceHelper(node); err != nil {
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
	// - there is no need to restart to update the current config
	// - there was no error trying to sync configuration
	// - if, previously, there was an error trying to sync configuration, we need to restore the ConfigOK condition
	//   to an error free reason and condition e.g. "passed all checks" and ConditionTrue.
	// There are 3 possible ConfigOK conditions that set status to ConditionTrue: "using current (init)", "using current (default)", "using current (UID: %q)"

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

// syncConfigSourceHelper downloads and checkpoints the configuration for `node`, if necessary, and also updates the symlinks
// to point to a new configuration if necessary.
// If all operations succeed, returns (bool, nil), where bool indicates whether the current configuration changed. If (true, nil),
// restarting the Kubelet to begin using the new configuration is recommended.
// If downloading fails for a non-fatal reason, an error is returned. See `downloadConfig` and `downloadConfigMap` for fatal reasons.
// If filesystem issues prevent inspecting current configuration or setting symlinks, a panic occurs.
// If an error is returned, a cause is also returned in the second position,
// this is a sanitized version of the error that can be reported in the ConfigOK condition.
func (cc *NodeConfigController) syncConfigSourceHelper(node *apiv1.Node) (bool, string, error) {
	// if the NodeConfigSource is non-nil, download the config
	updatedCfg := false
	src := node.Spec.ConfigSource
	if src != nil {
		infof("Node.Spec.ConfigSource is non-empty, will download config if necessary")

		source, cause, err := newRemoteConfigSource(*node.Spec.ConfigSource)
		if err != nil {
			return false, cause, err
		}

		uid := source.uid()

		// if the checkpoint already exists, skip downloading
		if cc.checkpointExists(uid) {
			infof("checkpoint already exists for object with UID %q, skipping download", uid)
			return false, "", nil
		}

		checkpoint, cause, err := source.download(cc.client)
		if err != nil {
			return false, cause, fmt.Errorf("failed to download config, error: %v", err)
		}

		err = cc.checkpointStore.save(checkpoint)
		if err != nil {
			return false,
				fmt.Sprintf("failed to save checkpoint for object with UID %q", checkpoint.uid()),
				fmt.Errorf("failed to save checkpoint, error: %v", err)
		}

		// if curUID is already correct we can skip updating the symlink
		curUID := cc.curUID()
		if curUID == uid {
			return false, "", nil
		}

		// update curSymlink to point to the new configuration
		cc.setSymlinkUID(curSymlink, uid)
		updatedCfg = true
	} else {
		infof("Node.Spec.ConfigSource is empty, will reset symlinks if necessary")
		// empty config on the node requires both symlinks be reset to the default,
		// we return whether the current configuration changed
		cc.resetSymlink(lkgSymlink)
		updatedCfg = cc.resetSymlink(curSymlink)
	}
	return updatedCfg, "", nil
}

type remoteConfigSource interface {
	// uid returns the UID of the config source object
	uid() string
	// download returns a checkpoint, or a sanitized cause and an error
	download(client clientset.Interface) (checkpoint, string, error)
}

// constructs a new remoteConfigSource from a NodeConfigSource
// returns a sanitized cause and an error if the NodeConfigSource is blatantly invalid
func newRemoteConfigSource(source apiv1.NodeConfigSource) (remoteConfigSource, string, error) {
	// exactly one subfield of the config source must be non-nil, toady ConfigMapRef is the only reference
	if source.ConfigMapRef == nil {
		return nil,
			"invalid NodeConfigSource, exactly one subfield must be non-nil, but all were nil",
			fmt.Errorf("%s, NodeConfigSource was: %+v", cause, source)
	}

	// at this point we know we're using the ConfigMapRef subfield
	ref := source.ConfigMapRef

	// name, namespace, and UID must all be non-empty
	if ref.Name == "" || ref.Namespace == "" || string(ref.UID) == "" {
		return nil,
			"invalid ObjectReference, all of UID, Name, and Namespace must be specified",
			fmt.Errorf("%s, ObjectReference was: %+v", cause, ref)
	}

	cmSource := configMapRef(*ref)
	return &cmSource, "", nil
}

// configMapRef refers to a configuration stored in a ConfigMap on the API server, implements remoteConfigSource
type configMapRef apiv1.ObjectReference

func (ref *configMapRef) uid() string {
	return string(ref.UID)
}

func (ref *configMapRef) download(client clientset.Interface) (checkpoint, string, error) {
	var cause string

	uid := string(ref.UID)

	infof("attempting to download ConfigMap with UID %q", uid)

	// get the ConfigMap via namespace/name, there doesn't seem to be a way to get it by UID
	cm, err := client.CoreV1().ConfigMaps(ref.Namespace).Get(ref.Name, metav1.GetOptions{})
	if err != nil {
		cause = fmt.Sprintf("could not download ConfigMap with name %q from namespace %q", ref.Name, ref.Namespace)
		return nil, cause, fmt.Errorf("%s, error: %v", cause, err)
	}

	// ensure that UID matches the UID on the reference, the ObjectReference must be unambiguous
	if ref.UID != cm.UID {
		cause = fmt.Sprintf("invalid ObjectReference, UID %q does not match UID of downloaded ConfigMap %q", ref.UID, cm.UID)
		return nil, cause, fmt.Errorf("%s", cause)
	}

	infof("successfully downloaded ConfigMap with UID %q", uid)
	ckpt := configMapCheckpoint(*cm)
	return &ckpt, "", nil
}
