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
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"k8s.io/kubernetes/pkg/apis/componentconfig/validation"
)

const (
	startupsFile    = ".startups.json"
	tmpStartupsFile = ".tmp_startups.json"

	// we allow one extra startup to account for the startup necessary to update configuration
	maxStartups = validation.MaxCrashLoopThreshold + 1
)

// recordStartup appends a timestamp to the startups-tracking file to indicate a rough Kubelet startup time.
// If the startups-tracking file cannot be loaded or saved, a panic occurs.
func (cc *NodeConfigController) recordStartup() {
	// load the file
	ls := cc.loadStartups()

	// record current time
	now := time.Now()
	stamp := now.Format(time.RFC3339) // use RFC3339 time format
	ls = append(ls, stamp)

	// rotate the list if necessary
	if len(ls) > maxStartups {
		ls = ls[1:]
	}

	// save the file
	cc.saveStartups(ls)
}

// crashLooping returns true if the number of startup timestamps since the last modification
// of the current config exceeds `threshold`, false otherwise.
// This function assumes that the trial period for a config is still active, if called outside
// the trial period, it may overcount startups.
// If filesystem issues prevent determining a modification time or loading
// the startups-tracking-file, a panic occurs.
func (cc *NodeConfigController) crashLooping(threshold int32) bool {
	// load the startups-tracking file
	ls := cc.loadStartups()

	// determine the last time the current config changed
	modTime := cc.curModTime()

	// count the timestamps in the startups-tracking file that occur after the last change to curSymlink
	// we assume that we are still in the trial period, and since the file is append-only
	// we only need to count the number of timestamps since the modification time
	num := int32(0)
	l := len(ls)
	for i, stamp := range ls {
		t, err := time.Parse(time.RFC3339, stamp)
		if err != nil {
			panicf("failed to parse timestamp while checking for crash loops, error: %v", err)
		}
		if t.After(modTime) {
			num = int32(l - i)
			break
		}
	}

	// >, rather than >=, because we implicitly allow one extra startup for changing config
	return num > threshold
}

// loadStartups loads the startups-tracking file from disk.
// If loading succeeds, returns a string slice of RFC3339 format timestamps.
// If the file is empty, returns an empty slice.
// If the file cannot be loaded, a panic occurs.
func (cc *NodeConfigController) loadStartups() []string {
	path := filepath.Join(cc.configDir, startupsFile)

	// load the file
	b, err := ioutil.ReadFile(path)
	if err != nil {
		panicf("failed to load startups-tracking file %q, error: %v", path, err)
	}

	// parse json into the slice
	ls := []string{}

	// if the file is empty, just return empty slice
	if len(b) == 0 {
		return ls
	}

	// otherwise unmarshal the json
	if err := json.Unmarshal(b, &ls); err != nil {
		panicf("failed to unmarshal json from startups-tracking file %q, error: %v", path, err)
	}
	return ls
}

// saveStartups replaces the contents of the startups-tracking file with `ls`.
// If the file cannot be saved, a panic occurs.
func (cc *NodeConfigController) saveStartups(ls []string) {
	path := filepath.Join(cc.configDir, startupsFile)
	tmpPath := filepath.Join(cc.configDir, tmpStartupsFile)

	// require that startupsFile exist, as ensureFile should be used to create it
	if _, err := os.Stat(path); os.IsNotExist(err) {
		panicf("startups-tracking file %q must already exist in order to save it, error: %v", path, err)
	} else if err != nil {
		panicf("failed to stat startups-tracking file %q, error: %v", path, err)
	} // Assert: file exists

	// marshal the json
	b, err := json.Marshal(ls)
	if err != nil {
		panicf("failed to marshal json for startups-tracking file, ls: %v, error: %v", ls, err)
	}

	// write to a tmp file
	if err := ioutil.WriteFile(tmpPath, b, defaultPerm); err != nil {
		panicf("failed to save to tmp file %q, error: %v", tmpPath, err)
	}

	// atomic rename over the existing file
	if err := os.Rename(tmpPath, path); err != nil {
		panicf("failed to rename tmp file %q over %q, error: %v", tmpPath, path, err)
	}
}
