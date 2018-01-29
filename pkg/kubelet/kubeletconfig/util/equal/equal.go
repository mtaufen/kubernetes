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

package equal

import apiv1 "k8s.io/api/core/v1"

// ConfigSourceEq returns true if the two config sources are semantically equivalent in the context of dynamic config
func ConfigSourceEq(a, b *apiv1.NodeConfigSource) bool {
	if a == b {
		return true
	} else if a == nil || b == nil {
		// not equal, and one is nil
		return false
	}
	// check equality of config source subifelds
	if a.ConfigMap != b.ConfigMap {
		return ConfigMapNodeConfigSourceEq(a.ConfigMap, b.ConfigMap)
	}
	// all internal subfields of the config source are equal
	return true
}

// ConfigMapNodeConfigSourceEq returns true if the two config map references are semantically equivalent in the context of dynamic config
func ConfigMapNodeConfigSourceEq(a, b *apiv1.ConfigMapNodeConfigSource) bool {
	if a == b {
		return true
	} else if a == nil || b == nil {
		// not equal, and one is nil
		return false
	}
	// TODO(mtaufen): ensure changing the kubelet config key triggers a Kubelet restart
	return a.UID == b.UID &&
		a.Namespace == b.Namespace &&
		a.Name == b.Name &&
		a.KubeletConfigKey == b.KubeletConfigKey
}

// ConfigOKEq returns true if the two conditions are semantically equivalent in the context of dynamic config
func ConfigOKEq(a, b *apiv1.NodeCondition) bool {
	return a.Message == b.Message && a.Reason == b.Reason && a.Status == b.Status
}
