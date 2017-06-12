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

import apiv1 "k8s.io/kubernetes/pkg/api/v1"

// configSoruceEq returns true if the two config sources are semantically equivalent in the context of dynamic config
func configSourceEq(a, b *apiv1.NodeConfigSource) bool {
	if a == b {
		return true
	} else if a == nil || b == nil {
		// not equal, and one is nil
		return false
	}
	// check equality of config source subifelds
	if a.ConfigMapRef != b.ConfigMapRef {
		return objectRefEq(a.ConfigMapRef, b.ConfigMapRef)
	}
	// all internal subfields of the config soruce are equal
	return true
}

// objectRefEq returns true if the two object references are semantically equivalent in the context of dynamic config
func objectRefEq(a, b *apiv1.ObjectReference) bool {
	if a == b {
		return true
	} else if a == nil || b == nil {
		// not equal, and one is nil
		return false
	}
	return a.UID == b.UID && a.Namespace == b.Namespace && a.Name == b.Name
}

// configOKEq returns true if the two conditions are semantically equivalent in the context of dynamic config
func configOKEq(a, b *apiv1.NodeCondition) bool {
	return a.Message == b.Message && a.Reason == b.Reason && a.Status == b.Status
}
