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
	"testing"

	utiltest "k8s.io/kubernetes/pkg/kubelet/kubeletconfig/util/test"
)

func TestEncodeMapStringString(t *testing.T) {
	cases := []struct {
		desc   string
		m      map[string]string
		expect string
	}{
		// empty map
		{"empty map", map[string]string{}, ""},
		// one key
		{"one key", map[string]string{"one": ""}, "one:,"},
		// three keys (tests sorting order)
		{"three keys", map[string]string{"two": "2", "one": "", "three": "3"}, "one:,three:3,two:2,"},
	}
	for _, c := range cases {
		s := EncodeMapStringString(c.m)
		if s != c.expect {
			t.Errorf("case %q, expect %q from encode %#v, but got %q", c.desc, c.expect, c.m, s)
		}
	}
}

func TestHash(t *testing.T) {
	// hash the empty string for each alg constant and be sure we get the correct result,
	// this makes sure the correct alg is used for each constant
	cases := []struct {
		alg  string
		hash string
		err  string
	}{
		// sha256
		{Sha256Alg, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", ""},
		// unsupported algorithm
		{"bogus", "", "not supported"},
	}

	for _, c := range cases {
		h, err := hash(c.alg, "")
		if utiltest.SkipRest(t, c.alg, err, c.err) {
			continue
		}
		if h != c.hash {
			t.Errorf("case %q, expected hash %q but got %q", c.alg, c.hash, h)
		}
	}
}

func TestMapStringStringHash(t *testing.T) {
	cases := []struct {
		desc string
		m    map[string]string
		alg  string
		hash string
		err  string
	}{
		// empty map
		{"empty map", map[string]string{}, Sha256Alg, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", ""},
		// one key
		{"one key", map[string]string{"one": ""}, Sha256Alg, "695ceb80f5aa4640897ecc2287dde413dfb595d5f545452bc58bbef5a012fdab", ""},
		// two keys
		{"two keys", map[string]string{"one": "", "two": "2"}, Sha256Alg, "2bff03d6249c8a9dc9a1436d087c124741361ccfac6615b81b67afcff5c42431", ""},
		// unsupported alg
		{"unsupported alg", map[string]string{}, "bogus", "", "not supported"},
	}

	for _, c := range cases {
		h, err := MapStringStringHash(c.alg, c.m)
		if utiltest.SkipRest(t, c.desc, err, c.err) {
			continue
		}
		if h != c.hash {
			t.Errorf("case %q, expect hash %q but got %q", c.desc, c.hash, h)
		}
	}
}
