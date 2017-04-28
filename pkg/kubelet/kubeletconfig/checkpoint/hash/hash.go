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

package hash

import (
	"crypto/sha256"
	"fmt"
	"sort"
)

const (
	Sha256Alg = "sha256"
)

// MapStringStringHash returns a hash of the EncodeMapStringString encoding of `m`, using `alg`,
// the returned hash is a hexidecimal string
func MapStringStringHash(alg string, m map[string]string) (string, error) {
	s := EncodeMapStringString(m)
	return hash(alg, s)
}

// EncodeMapStringString extracts the key-value pairs from `m`, sorts them in byte-alphabetic order by key,
// and encodes them in a string representation. Keys and values are separated with `:` and pairs are separated
// with `,`. If m is non-empty, there is a trailing comma in the pre-hash serialization. If m is empty,
// there is no trailing comma.
func EncodeMapStringString(m map[string]string) string {
	kv := make([][]string, len(m))
	i := 0
	for k, v := range m {
		kv[i] = []string{k, v}
		i++
	}
	// sort based on keys
	sort.Slice(kv, func(i, j int) bool {
		return kv[i][0] < kv[j][0]
	})
	// encode to a string
	s := ""
	for _, p := range kv {
		s = s + p[0] + ":" + p[1] + ","
	}
	return s
}

// hash hashes `data` with `alg` if `alg` is supported
func hash(alg string, data string) (string, error) {
	// take the hash based on alg
	switch alg {
	case Sha256Alg:
		sum := sha256.Sum256([]byte(data))
		return fmt.Sprintf("%x", sum), nil
	default:
		return "", fmt.Errorf("requested hash algorithm %q is not supported", alg)
	}
}
