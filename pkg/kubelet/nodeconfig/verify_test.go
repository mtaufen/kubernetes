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
	"strings"
	"testing"
)

func TestParseConfigName(t *testing.T) {
	var cases = []struct {
		fullName  string
		errSubstr string // if empty, expect nil err

		name string
		alg  string
		hash string
	}{
		// full name with human-readable identifier
		{"testcfg-sha256-91f42f686726251311d399d86bd01425cea38fbd154ff2104e0555343610c83f", "", "testcfg", "sha256", "91f42f686726251311d399d86bd01425cea38fbd154ff2104e0555343610c83f"},
		// hash only
		{"sha256-91f42f686726251311d399d86bd01425cea38fbd154ff2104e0555343610c83f", "", "", "sha256", "91f42f686726251311d399d86bd01425cea38fbd154ff2104e0555343610c83f"},
		// incorrect leading dash
		{fullName: "-sha256-91f42f686726251311d399d86bd01425cea38fbd154ff2104e0555343610c83f", errSubstr: "did not match"},
		// missing hash
		{fullName: "testcfg-sha256", errSubstr: "did not match"},
		// empty alg
		{fullName: "testcfg--91f42f686726251311d399d86bd01425cea38fbd154ff2104e0555343610c83f", errSubstr: "did not match"},
		// empty hash
		{fullName: "testcfg-sha256-", errSubstr: "did not match"},
		// missing hash and alg
		{fullName: "testcfg", errSubstr: "did not match"},
		{fullName: "testcfg-", errSubstr: "did not match"},
	}

	for _, c := range cases {
		name, alg, hash, err := parseConfigName(c.fullName)
		if err != nil {
			if len(c.errSubstr) == 0 {
				t.Fatalf("expected nil error for case %q, but got %q", c.fullName, err.Error())
			} else if !strings.Contains(err.Error(), c.errSubstr) {
				t.Fatalf("expected error for case %q to contain %q, but got %q", c.fullName, c.errSubstr, err.Error())
			}
		} else if len(c.errSubstr) > 0 {
			t.Fatalf("expected non-nil error containing %q for case %q, but got a nil error", c.errSubstr, c.fullName)
		}
		if name != c.name {
			t.Errorf("expected name %q for case %q, but got %q", c.name, c.fullName, name)
		}
		if alg != c.alg {
			t.Errorf("expected alg %q for case %q, but got %q", c.alg, c.fullName, alg)
		}
		if hash != c.hash {
			t.Errorf("expected hash %q for case %q, but got %q", c.hash, c.fullName, hash)
		}
	}
}
