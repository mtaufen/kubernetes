/*
Copyright 2015 The Kubernetes Authors.

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

package configz

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/api"

	// install the KubeletConfig APIs
	_ "k8s.io/kubernetes/pkg/kubelet/apis/kubeletconfig/install"

	// hacky source for some test data, ideally we'd have a fake scheme with multiple supported versions
	"k8s.io/kubernetes/pkg/kubelet/apis/kubeletconfig"
	kubeletconfigv1alpha1 "k8s.io/kubernetes/pkg/kubelet/apis/kubeletconfig/v1alpha1"
)

func newKubeletConfigEncoder(t *testing.T) runtime.Encoder {
	// encode to json
	mediaType := "application/json"
	info, ok := runtime.SerializerInfoForMediaType(api.Codecs.SupportedMediaTypes(), mediaType)
	if !ok {
		t.Fatalf("unsupported media type %q", mediaType)
	}

	versions := api.Registry.EnabledVersionsForGroup(kubeletconfig.GroupName)
	if len(versions) == 0 {
		t.Fatalf("no enabled versions for group %q", kubeletconfig.GroupName)
	}

	// the "best" version supposedly comes first in the list returned from api.Registry.EnabledVersionsForGroup
	return api.Codecs.EncoderForVersion(info.Serializer, versions[0])
}

func newDefaultAndEncodedKubeletConfiguration(t *testing.T) (*kubeletconfigv1alpha1.KubeletConfiguration, string) {
	versioned := &kubeletconfigv1alpha1.KubeletConfiguration{}
	api.Scheme.Default(versioned)

	encoder := newKubeletConfigEncoder(t)
	data, err := runtime.Encode(encoder, versioned)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// round trip so that the decoded object actually has its kind and apiVersion fields set
	// TODO(mtaufen): I don't think the kubelet sets these internally today, and that will be a problem for configz
	if _, _, err := api.Codecs.UniversalDecoder().Decode(data, nil, versioned); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	return versioned, string(data)
}

// TODO(mtaufen): test the handler

type schemeObject struct {
	scheme *runtime.Scheme
	obj    runtime.Object
}

func TestRegister(t *testing.T) {

}

func TestUnregister(t *testing.T) {

}

func TestHandler(t *testing.T) {

	// Test:
	// get root
	// get group version
	// get invalid paths /configz/unregistered/thing, /configz/thing/and/more/stuff/than/we/support, etc.
	// unsupported http methods (make sure it returns an allow header)
	// that content type is application/json

	// get some objects to register for testing
	defaultKC, defaultKCEncoded := newDefaultAndEncodedKubeletConfiguration(t)

	// we only allow GET requests
	headerAllowGET := map[string][]string{"Allow": []string{"GET"}}

	// test cases
	cases := []struct {
		method       string
		path         string
		expectCode   int
		expectHeader map[string][]string
		expectBody   string
		// optional scheme, object pairs to register
		register []schemeObject
	}{
		// only support GET requests, return correct "Allow" header
		{http.MethodHead, "/configz", http.StatusMethodNotAllowed, headerAllowGET, "", nil},
		{http.MethodPost, "/configz", http.StatusMethodNotAllowed, headerAllowGET, "", nil},
		{http.MethodPut, "/configz", http.StatusMethodNotAllowed, headerAllowGET, "", nil},
		{http.MethodPatch, "/configz", http.StatusMethodNotAllowed, headerAllowGET, "", nil},
		{http.MethodDelete, "/configz", http.StatusMethodNotAllowed, headerAllowGET, "", nil},
		{http.MethodConnect, "/configz", http.StatusMethodNotAllowed, headerAllowGET, "", nil},
		{http.MethodOptions, "/configz", http.StatusMethodNotAllowed, headerAllowGET, "", nil},
		{http.MethodTrace, "/configz", http.StatusMethodNotAllowed, headerAllowGET, "", nil},

		// API discovery
		{http.MethodGet, "/configz", http.StatusOK, nil, "TODO(mtaufen): discovery stuff", nil},
		{http.MethodGet, "/configz/kubeletconfig/v1alpha1", http.StatusOK, nil, "TODO(mtaufen): discovery stuff", nil},

		// get the serialized object back
		// TODO(mtaufen): should we make kind case-insensitive?
		{http.MethodGet, "/configz/kubeletconfig/v1alpha1/KubeletConfiguration", http.StatusOK, nil, defaultKCEncoded, []schemeObject{
			{api.Scheme, defaultKC},
		}},

		// path that goes past concrete object
		{http.MethodGet, "/configz/kubeletconfig/v1alpha1/KubeletConfiguration/foo", http.StatusNotFound, nil, "", nil},
	}

	for _, c := range cases {
		testCase := fmt.Sprintf("%s %s", c.method, c.path)

		// construct response writer and request
		w := httptest.NewRecorder()
		req := httptest.NewRequest(c.method, c.path, nil)

		// register
		if c.register != nil {
			for _, r := range c.register {
				if err := Register(r.scheme, r.obj); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		}
		// invoke handler
		handler(w, req)
		// unregister
		if c.register != nil {
			for _, r := range c.register {
				Unregister(KeyForObject(r.obj))
			}
		}

		// expected response code
		if w.Code != c.expectCode {
			t.Errorf("case %q, expect code %d but got %d", testCase, c.expectCode, w.Code)
			continue
		}

		// expected headers (checks for subset of header keys, but covers values for each key)
		if c.expectHeader != nil {
			h := map[string][]string(w.Header())
			for expectK, expectV := range c.expectHeader {
				v, ok := h[expectK]
				if !ok {
					t.Errorf("case %q, expected header %q not found", testCase, expectK)
					continue
				}
				// sort both
				sort.Strings(expectV)
				sort.Strings(v)
				// compare
				if !reflect.DeepEqual(expectV, v) {
					t.Errorf("case %q, expected header %q with values %v, but got %v", testCase, expectK, expectV, v)
					continue
				}
			}
		}

		// expected response body
		data, err := ioutil.ReadAll(w.Body)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			continue
		}
		body := string(data)
		if body != c.expectBody {
			t.Errorf("case %q, expect body:\n%s\nbut got:\n%s", testCase, c.expectBody, body)
			continue
		}
	}

}
