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

	restful "github.com/emicklei/go-restful"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	groupParam   = "group"
	versionParam = "version"
	kindParam    = "kind"
)

// services is a subset of the restful.Container interface
type services interface {
	// Add registeres a web service with the restful.Container
	Add(service *restful.WebService) *restful.Container
}

func InstallHandler(s services) {
	ws := new(restful.WebService)
	ws.Path("/configz")
	// routing for /configz (API discovery)
	// ws.Route(ws.GET("").
	// 	To(discoverConfigz).
	// 	Operation("getConfigz"))
	// routing for /configz/group/version/kind
	ws.Route(ws.GET(fmt.Sprintf("/{%s}/{%s}/{%s}", groupParam, versionParam, kindParam)).
		To(getConfigz).
		Operation("getConfigz"))
	s.Add(ws)

	// TODO(mtaufen): API discovery
}

// func discoverConfigz(req *restful.Request, resp *restful.Response) {

// }

func getConfigz(req *restful.Request, resp *restful.Response) {
	configsMutex.Lock()
	defer configsMutex.Unlock()

	gvk := gvkForRequest(req)

	// look for the object
	gk := gvk.GroupKind()
	config, ok := configs[(&gk).String()]
	if !ok {
		resp.WriteError(http.StatusNotFound, errorMessage("config not found"))
		return
	}

	// ensure that the scheme recognizes the requested group, version, and kind
	if !config.scheme.Recognizes(gvk) {
		resp.WriteError(http.StatusNotFound, errorMessage(fmt.Sprintf("GroupVersionKind %#v unsupported", gvk)))
		return
	}

	// encode the object
	encoder := config.codecs.EncoderForVersion(config.jsonInfo.Serializer, gvk.GroupVersion())
	json, err := runtime.Encode(encoder, config.obj)
	if err != nil {
		resp.WriteError(http.StatusInternalServerError, errorMessage("could not encode response"))
		return
	}

	// write 200 response w/ json encoded object
	resp.Write(json)
	return
}

// TODO(mtaufen): API discovery bits

type configzRequestParams struct {
	group   string
	version string
	kind    string
}

func gvkForRequest(req *restful.Request) schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   req.PathParameter(groupParam),
		Version: req.PathParameter(versionParam),
		Kind:    req.PathParameter(kindParam),
	}
}

func errorMessage(msg string) error {
	return fmt.Errorf(`{"message":"%s"}`, msg)
}
