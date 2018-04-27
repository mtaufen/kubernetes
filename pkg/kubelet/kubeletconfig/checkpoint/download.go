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

package checkpoint

import (
	"fmt"

	apiv1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	"k8s.io/kubernetes/pkg/kubelet/kubeletconfig/status"
	utilcodec "k8s.io/kubernetes/pkg/kubelet/kubeletconfig/util/codec"
	utillog "k8s.io/kubernetes/pkg/kubelet/kubeletconfig/util/log"
)

// Payload represents a local copy of a config source (payload) object
type Payload interface {
	// UID returns a globally unique (space and time) identifier for the payload.
	UID() string

	// Files returns a map of filenames to file contents.
	Files() map[string]string

	// object returns the underlying checkpointed object.
	object() interface{}
}

// RemoteConfigSource represents a remote config source object that can be downloaded as a Checkpoint
type RemoteConfigSource interface {
	// UID returns a globally unique identifier of the source described by the remote config source object
	UID() string
	// KubeletFilename returns the name of the Kubelet config file as it should appear in the keys of Payload.Files()
	KubeletFilename() string
	// APIPath returns the API path to the remote resource, e.g. its SelfLink
	APIPath() string
	// Download downloads the remote config source object returns a Payload backed by the object,
	// or a sanitized failure reason and error if the download fails
	Download(client clientset.Interface) (Payload, string, error)
	// Encode returns a []byte representation of the NodeConfigSource behind the RemoteConfigSource
	Encode() ([]byte, error)

	// NodeConfigSource returns a copy of the underlying apiv1.NodeConfigSource object.
	// All RemoteConfigSources are expected to be backed by a NodeConfigSource,
	// though the convenience methods on the interface will target the source
	// type that was detected in a call to NewRemoteConfigSource.
	NodeConfigSource() *apiv1.NodeConfigSource
}

// NewRemoteConfigSource constructs a RemoteConfigSource from a v1/NodeConfigSource object
// You should only call this with a non-nil config source.
// Note that the API server validates Node.Spec.ConfigSource.
func NewRemoteConfigSource(source *apiv1.NodeConfigSource) (RemoteConfigSource, string, error) {
	// NOTE: Even though the API server validates the config, we check whether all *known* fields are
	// nil here, so that if a new API server allows a new config source type, old clients can send
	// an error message rather than crashing due to a nil pointer dereference.

	// exactly one reference subfield of the config source must be non-nil, toady ConfigMap is the only reference subfield
	if source.ConfigMap == nil {
		return nil, status.SyncErrorAllNilSubfields, fmt.Errorf("%s, NodeConfigSource was: %#v", status.SyncErrorAllNilSubfields, source)
	}
	return &remoteConfigMap{source}, "", nil
}

// DecodeRemoteConfigSource is a helper for using the apimachinery to decode serialized RemoteConfigSources;
// e.g. the metadata stored by checkpoint/store/fsstore.go
func DecodeRemoteConfigSource(data []byte) (RemoteConfigSource, error) {
	// decode the remote config source
	obj, err := runtime.Decode(legacyscheme.Codecs.UniversalDecoder(), data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode, error: %v", err)
	}

	// for now we assume we are trying to load an apiv1.SerializedNodeConfigSource,
	// this may need to be extended if e.g. a new version of the api is born

	// convert it to the external type, so we're consistently working with the external type
	cs := &apiv1.SerializedNodeConfigSource{}
	err = legacyscheme.Scheme.Convert(obj, cs, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to convert decoded object into a v1 SerializedNodeConfigSource, error: %v", err)
	}
	source, _, err := NewRemoteConfigSource(&cs.Source)
	return source, err
}

// EqualRemoteConfigSources is a helper for comparing remote config sources by
// comparing the underlying API objects for semantic equality.
func EqualRemoteConfigSources(a, b RemoteConfigSource) bool {
	if a != nil && b != nil {
		return apiequality.Semantic.DeepEqual(a.NodeConfigSource(), b.NodeConfigSource())
	}
	return a == b
}

// remoteConfigMap implements RemoteConfigSource for v1/ConfigMap config sources
type remoteConfigMap struct {
	source *apiv1.NodeConfigSource
}

var _ RemoteConfigSource = (*remoteConfigMap)(nil)

func (r *remoteConfigMap) UID() string {
	return string(r.source.ConfigMap.UID)
}

func (r *remoteConfigMap) KubeletFilename() string {
	return r.source.ConfigMap.KubeletConfigKey
}

const configMapAPIPathFmt = "/api/v1/namespaces/%s/configmaps/%s"

func (r *remoteConfigMap) APIPath() string {
	ref := r.source.ConfigMap
	return fmt.Sprintf(configMapAPIPathFmt, ref.Namespace, ref.Name)
}

func (r *remoteConfigMap) Download(client clientset.Interface) (Payload, string, error) {
	var reason string
	uid := string(r.source.ConfigMap.UID)

	utillog.Infof("attempting to download ConfigMap with UID %q", uid)

	// get the ConfigMap via namespace/name, there doesn't seem to be a way to get it by UID
	cm, err := client.CoreV1().ConfigMaps(r.source.ConfigMap.Namespace).Get(r.source.ConfigMap.Name, metav1.GetOptions{})
	if err != nil {
		reason = fmt.Sprintf(status.SyncErrorDownload, r.APIPath())
		return nil, reason, fmt.Errorf("%s, error: %v", reason, err)
	}

	// ensure that UID matches the UID on the source
	if r.source.ConfigMap.UID != cm.UID {
		reason = fmt.Sprintf(status.SyncErrorUIDMismatchFmt, r.source.ConfigMap.UID, r.APIPath(), cm.UID)
		return nil, reason, fmt.Errorf(reason)
	}

	payload, err := NewConfigMapPayload(cm)
	if err != nil {
		reason = fmt.Sprintf("invalid downloaded object")
		return nil, reason, fmt.Errorf("%s, error: %v", reason, err)
	}

	utillog.Infof("successfully downloaded ConfigMap with UID %q", uid)
	return payload, "", nil
}

func (r *remoteConfigMap) Encode() ([]byte, error) {
	encoder, err := utilcodec.NewYAMLEncoder(apiv1.GroupName)
	if err != nil {
		return nil, err
	}
	data, err := runtime.Encode(encoder, &apiv1.SerializedNodeConfigSource{Source: *r.source})
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (r *remoteConfigMap) NodeConfigSource() *apiv1.NodeConfigSource {
	return r.source.DeepCopy()
}
