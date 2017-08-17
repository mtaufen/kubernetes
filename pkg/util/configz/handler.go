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

package configz

import (
	"fmt"
	"net/http"
	"strings"

	restful "github.com/emicklei/go-restful"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	groupParam   = "group"
	versionParam = "version"
	kindParam    = "kind"
)

// mux is an interface that lets you register path handlers
// for example, http.ServeMux implements this interface
type mux interface {
	Handle(string, http.Handler)
}

func InstallHandler(s mux) {
	s.Handle("/configz/", http.HandlerFunc(handler))
}

func handler(w http.ResponseWriter, r *http.Request) {
	if r.URL == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	} else if r.Method != http.MethodGet {
		// only accetp GET requests against /configz
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Header().Set("Allow", http.MethodGet) // w3c rfc2616 sec. 10.4.6
		return
	}

	// TODO(mtaufen): sanitize the GVK strings

	// parse the URL and determine whether we want root, GroupVersion, or GroupVersionKind
	// this is such a terrible way to parse a URL... don't we have something better?

	// get rid of empty strings from leading or trailing slashes
	split := strings.Split(r.URL.Path(), "/")
	parts := []string{}
	for _, s := range split {
		if s != "" {
			parts := append(parts, s)
		}
	}

	switch len(parts) {
	case 1: // /configz
		getRoot(w)
	case 3: // /configz/group/version
		getGroupVersion(w, groupVersionFromParts(parts))
	case 4: // /configz/group/version/kind
		getGroupVersionKind(w, groupVersionKindFromParts(parts))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
	return
}

// obviously be careful to call this with a 3 or more components
func groupVersionFromParts(parts []string) *runtime.GroupVersion {
	return runtime.GroupVersion{Group: parts[1], Version: parts[2]}
}

// obviously be careful to call this with 4 or more components
func groupVersionKindFromParts(parts []string) *runtime.GroupVersionKind {
	return runtime.GroupVersion{Group: parts[1], Version: parts[2], Kind: parts[3]}
}

// get API paths for supported GroupVersions
func getRoot(w http.ResponseWriter) {
	// TODO(mtaufen)
	// example of discovery stuff: https://github.com/kubernetes/kubernetes/blob/master/cmd/kube-controller-manager/app/controllermanager.go#L347-L396
	// http://godoc.org/k8s.io/client-go/discovery
	w.Write([]byte("put root discovery stuff here"))
	return
}

// get API paths for supported Kinds in a given GroupVersion
func getGroupVersion(w http.ResponseWriter, gv *runtime.GroupVersion) {
	w.Write([]byte("put kind discovery stuff here"))
	return
}

// get the concrete object registered with the GroupVersionKind
func getGroupVersionKind(w http.ResponseWriter, gvk *runtime.GroupVersionKind) {
	configsMutex.Lock()
	defer configsMutex.Unlock()

	// look for the object
	gk := gvk.GroupKind()
	config, ok := configs[(&gk).String()]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// ensure that the scheme recognizes the requested group, version, and kind
	if !config.scheme.Recognizes(gvk) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// encode the object and write it to the response
	encoder := config.codecs.EncoderForVersion(config.jsonInfo.Serializer, gvk.GroupVersion())
	json, err := runtime.Encode(encoder, config.obj)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("could not encode object"))
		return
	}
	w.Write(json)
	return
}
