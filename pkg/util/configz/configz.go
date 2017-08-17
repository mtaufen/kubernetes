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
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

const (
	jsonMediaType = "application/json"
)

var (
	configsMutex sync.RWMutex
	configs      = map[string]*config{}
)

type config struct {
	scheme   *runtime.Scheme
	codecs   serializer.CodecFactory
	jsonInfo runtime.SerializerInfo
	obj      runtime.Object
}

// Register registers the object for serving via /configz/group/version/kind.
// All versions supported by the scheme can be served.
// To update the object, simply Register it again.
// Register will return an error if the scheme doesn't recognize the object
func Register(scheme *runtime.Scheme, obj runtime.Object) error {
	configsMutex.Lock()
	defer configsMutex.Unlock()

	gvk := obj.GetObjectKind().GroupVersionKind()

	// make sure the scheme supports the object
	if !scheme.Recognizes(gvk) {
		return fmt.Errorf("scheme does not recognize object with GroupVersionKind %#v", gvk)
	}

	codecs := serializer.NewCodecFactory(scheme)

	// get json serializer
	info, ok := runtime.SerializerInfoForMediaType(codecs.SupportedMediaTypes(), jsonMediaType)
	if !ok {
		return fmt.Errorf("%q not supported", jsonMediaType)
	}

	gk := gvk.GroupKind()
	configs[(&gk).String()] = &config{scheme, codecs, info, obj}
	return nil
}

// Unregister deletes the registered object at the specified key.
func Unregister(key string) {
	configsMutex.Lock()
	defer configsMutex.Unlock()
	delete(configs, key)
}
