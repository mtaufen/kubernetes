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
	// "fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

const () badConfigsFile = ".bad-configs.json"
	tmpBadConfigsFile = ".tmp_bad-configs.json"
	)

// badConfigEntry represents an entry in the bad-config-tracking file
type badConfigEntry struct {
	Time   string `json:"time"`
	Reason string `json:"reason"`
}

// isBadConfig checks the bad-config-tracking file for an entry for `uid`.
// If the entry exists, returns (true, entry).
// If the entry does not exist, returns (false, empty entry).
// If the bad-config-tracking file cannot be loaded, a panic occurs.
func (cc *NodeConfigController) isBadConfig(uid string) (bool, badConfigEntry) {
	m := cc.loadBadConfigs()
	entry, ok := m[uid]
	if ok {
		return ok, entry
	}
	return false, badConfigEntry{}
}

// markBadConfig makes an entry for `uid` containing the current time and the `reason` in the bad-config-tracking file.
// If a the bad-config-tracking file cannot be loaded or saved, a panic occurs.
func (cc *NodeConfigController) markBadConfig(uid, reason string) {
	// load the file
	m := cc.loadBadConfigs()

	// create the entry
	now := time.Now()
	entry := badConfigEntry{
		Time:   now.Format(time.RFC3339), // use RFC3339 time format
		Reason: reason,
	}
	m[uid] = entry

	// save the file
	cc.saveBadConfigs(m)
}

// loadBadConfigs loads the bad-config-tracking file from disk.
// If loading succeeds, returns a map of UIDs to badConfigEntrys
// If the file is empty, returns an empty map.
// If the file cannot be loaded, a panic occurs.
func (cc *NodeConfigController) loadBadConfigs() map[string]badConfigEntry {
	path := filepath.Join(cc.configDir, badConfigsFile)

	// load the file
	b, err := ioutil.ReadFile(path)
	if err != nil {
		panicf("failed to load bad-config-tracking file %q, error: %v", path, err)
	}

	// parse json into the map
	m := map[string]badConfigEntry{}

	// if the file is empty, just use empty map
	if len(b) == 0 {
		return m
	}

	// otherwise unmarshal the json
	if err := json.Unmarshal(b, &m); err != nil {
		panicf("failed to unmarshal json from bad-config-tracking file %q, error: %v", path, err)
	}
	return m
}

// saveBadConfigs replaces the contents of the bad-config-tracking file with `m`.
// If the file cannot be saved, a panic occurs.
func (cc *NodeConfigController) saveBadConfigs(m map[string]badConfigEntry) {
	path := filepath.Join(cc.configDir, badConfigsFile)
	tmpPath := filepath.Join(cc.configDir, tmpStartupsFile)

	// require that file exist, as ensureFile should be used to create it
	if _, err := os.Stat(path); os.IsNotExist(err) {
		panicf("bad-config-tracking file %q must already exist in order to save it, error: %v", path, err)
	} else if err != nil {
		panicf("failed to stat bad-config-tracking file %q, error: %v", path, err)
	} // Assert: file exists

	// marshal the json
	b, err := json.Marshal(m)
	if err != nil {
		panicf("failed to marshal json for bad-config-tracking file, m: %v, error: %v", m, err)
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
