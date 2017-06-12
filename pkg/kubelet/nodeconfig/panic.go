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
	"runtime"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

// mkIgnoreControllerPanics returns a function that calls `fn`. If a non-Go-runtime panic occurs, it is ignored,
// otherwise the panic continues to bubble up the call stack.
func mkRecoverControllerPanic(fn func()) func() {
	return func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(runtime.Error); ok {
					panic(r)
				}
			}
		}()
		// call the function
		fn()
	}
}

// mkHandlePanic returns a function that calls `fn`. If a panic occurs when `fn` is called, the
// panic handlers in `utilruntime` will be called on it, and then the panic will continue to bubble
// up the call stack.
func mkHandlePanic(fn func()) func() {
	return func() {
		defer func() {
			if r := recover(); r != nil {
				for _, fn := range utilruntime.PanicHandlers {
					fn(r)
				}
				panic(r)
			}
		}()
		// call the function
		fn()
	}
}
