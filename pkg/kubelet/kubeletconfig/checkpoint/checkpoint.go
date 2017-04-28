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
	"regexp"

	apiv1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/componentconfig"
)

// Checkpoint represents a local copy of a config source (payload) object
type Checkpoint interface {
	// UID returns the UID of the config source object behind the Checkpoint
	UID() string
	// Verify returns an error if the hash presented by config source behind the Checkpoint doesn't match the hash of the config source
	Verify() error
	// Parse parses the checkpoint into the internal KubeletConfiguration type
	Parse() (*componentconfig.KubeletConfiguration, error)
	// Encode returns a []byte representation of the config source object behind the Checkpoint
	Encode() ([]byte, error)

	// object returns the underlying checkpointed object. If you want to compare sources for equality, use EqualCheckpoints,
	// which compares the underlying checkpointed objects for semantic API equality.
	object() interface{}
}

// DecodeCheckpoint is a helper for using the apimachinery to decode serialized checkpoints
func DecodeCheckpoint(data []byte) (Checkpoint, error) {
	// decode the checkpoint
	obj, err := runtime.Decode(api.Codecs.UniversalDecoder(), data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode, error: %v", err)
	}

	// TODO(mtaufen): for now we assume we are trying to load a ConfigMap checkpoint, may need to extend this if we allow other checkpoint types

	// convert it to the external ConfigMap type, so we're consistently working with the external type outside of the on-disk representation
	cm := &apiv1.ConfigMap{}
	err = api.Scheme.Convert(obj, cm, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to convert decoded object into a v1 ConfigMap, error: %v", err)
	}
	return NewConfigMapCheckpoint(cm)
}

func EqualCheckpoints(a, b Checkpoint) bool {
	if a != nil && b != nil {
		return apiequality.Semantic.DeepEqual(a.object(), b.object())
	}
	if a == nil && b == nil {
		return true
	}
	return false
}

// capture groups:
// name: config name substring, sans trailing `-`
// alg: algorithm used to produce the hash value
// hash: hash value
// TODO(mtaufen): Move away from using a regex to parse this. It was expedient to write, but generally regex is difficult to maintain.
const algHashRE = `^((?P<name>[a-z0-9.\-]+)-)?(?P<alg>[a-z0-9]+)-(?P<hash>[a-f0-9]+)$`

// parseCheckpointName extracts `name`, `alg`, and `hash` from the name of a config source object
func parseCheckpointName(n string) (name string, alg string, hash string, err error) {
	alg = ""
	hash = ""

	re, err := regexp.Compile(algHashRE)
	if err != nil {
		return
	}

	// run the regexp, zero matches is treated as an error because it means the name is malformed
	groupNames := re.SubexpNames()
	matches := re.FindStringSubmatch(n)
	if len(matches) == 0 {
		err = fmt.Errorf("malformed name %q did not match regexp %q", n, algHashRE)
		return
	}
	// zip names and matches into a map
	namedMatches := map[string]string{}
	for i, match := range matches {
		namedMatches[groupNames[i]] = match
	}

	name = namedMatches["name"]
	alg = namedMatches["alg"]
	hash = namedMatches["hash"]
	return
}
