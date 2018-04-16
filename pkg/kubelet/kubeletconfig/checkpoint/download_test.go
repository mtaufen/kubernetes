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
	"testing"

	"github.com/davecgh/go-spew/spew"

	apiv1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeclient "k8s.io/client-go/kubernetes/fake"
	utiltest "k8s.io/kubernetes/pkg/kubelet/kubeletconfig/util/test"
)

func TestRemoteConfigMapUID(t *testing.T) {
	const expect = "uid"
	source := NewRemoteConfigSource(&apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
		ObjectReference:  apiv1.ObjectReference{Name: "name", Namespace: "namespace", UID: expect},
		KubeletConfigKey: "kubelet",
	}})
	uid := source.UID()
	if expect != uid {
		t.Errorf("expect %q, but got %q", expect, uid)
	}
}

func TestRemoteConfigMapAPIPath(t *testing.T) {
	const (
		name      = "name"
		namespace = "namespace"
	)
	source := NewRemoteConfigSource(&apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
		ObjectReference:  apiv1.ObjectReference{Name: name, Namespace: namespace, UID: "uid"},
		KubeletConfigKey: "kubelet",
	}})
	expect := fmt.Sprintf(configMapAPIPathFmt, namespace, name)
	path := source.APIPath()

	if expect != path {
		t.Errorf("expect %q, but got %q", expect, path)
	}
}

func TestRemoteConfigMapDownload(t *testing.T) {
	cm := &apiv1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "name",
			Namespace:       "namespace",
			UID:             "uid",
			ResourceVersion: "1",
		}}
	client := fakeclient.NewSimpleClientset(cm)
	payload, err := NewConfigMapPayload(cm)
	if err != nil {
		t.Fatalf("error constructing payload: %v", err)
	}

	cases := []struct {
		desc   string
		source RemoteConfigSource
		expect Payload
		err    string
	}{
		{
			desc: "object doesn't exist",
			source: NewRemoteConfigSource(&apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
				ObjectReference:  apiv1.ObjectReference{Name: "bogus", Namespace: "namespace"},
				KubeletConfigKey: "kubelet",
			}}),
			expect: nil,
			err:    "not found",
		},
		{
			desc: "object exists",
			source: NewRemoteConfigSource(&apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
				ObjectReference:  apiv1.ObjectReference{Name: "name", Namespace: "namespace"},
				KubeletConfigKey: "kubelet",
			}}),
			expect: payload,
			err:    "",
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			payload, _, err := c.source.Download(client)
			utiltest.ExpectError(t, err, c.err)
			if err != nil {
				return
			}
			// downloaded object should match the expected
			if !apiequality.Semantic.DeepEqual(c.expect.object(), payload.object()) {
				t.Errorf("expect Checkpoint %s but got %s", spew.Sdump(c.expect), spew.Sdump(payload))
			}
			// source UID and ResourceVersion should be updated by Download
			if payload.UID() != c.source.UID() {
				t.Errorf("expect UID to be updated by Download to match payload: %s, but got source UID: %s", payload.UID(), c.source.UID())
			}
			if payload.ResourceVersion() != c.source.ResourceVersion() {
				t.Errorf("expect ResourceVersion to be updated by Download to match payload: %s, but got source ResourceVersion: %s", payload.ResourceVersion(), c.source.ResourceVersion())
			}
		})
	}
}

func TestEqualRemoteConfigSources(t *testing.T) {
	cases := []struct {
		desc   string
		a      RemoteConfigSource
		b      RemoteConfigSource
		expect bool
	}{
		{"both nil", nil, nil, true},
		{"a nil", nil, &remoteConfigMap{}, false},
		{"b nil", &remoteConfigMap{}, nil, false},
		{"neither nil, equal", &remoteConfigMap{}, &remoteConfigMap{}, true},
		{
			desc: "neither nil, not equal",
			a: &remoteConfigMap{&apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
				ObjectReference: apiv1.ObjectReference{Name: "a"},
			}}},
			b:      &remoteConfigMap{&apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{KubeletConfigKey: "kubelet"}}},
			expect: false,
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			if EqualRemoteConfigSources(c.a, c.b) != c.expect {
				t.Errorf("expected EqualRemoteConfigSources to return %t, but got %t", c.expect, !c.expect)
			}
		})
	}
}
