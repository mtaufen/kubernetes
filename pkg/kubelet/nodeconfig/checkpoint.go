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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	kuberuntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/api"
	apiv1 "k8s.io/kubernetes/pkg/api/v1"
)

type loadableCheckpoint interface{}

type checkpoint interface {
	// parsable // all checkpoints implement the parsable interface
	// verifiable // all checkpoints implement the verifiable interface
	// save(s checkpointStore) (string, error)
	bytes() ([]byte, error)
	uid() string
}

// checkpointExists returns true if checkpoint for the config source identified by `uid` exists on disk.
// If the existence of a checkpoint cannot be determined due to filesystem issues, a panic occurs.
func (cc *NodeConfigController) checkpointExists(uid string) bool {
	ok, err := cc.dirExists(filepath.Join(checkpointsDir, uid))
	if err != nil {
		panicf("failed to determine whether checkpoint %q exists, error: %v", uid, err)
	}
	return ok
}

// loadCheckpoint loads the checkpoint at `cc.configDir/relPath`, which is expected to be a checkpoint directory.
// If the checkpoint directory does not exist or if data cannot be retrieved from the filesystem,
// a panic will occur.
// If the data cannot be decoded and converted to a supported config source type, returns an error.
// This may indicate a failure to completely save the checkpoint. You may want to attempt a re-download in this scenario.
// If loading succeeds, returns a `verifiable` (see verify.go). This interface can be used to verify the integrity of
// the loaded checkpoint.
func (cc *NodeConfigController) loadCheckpoint(relPath string) (verifiable, error) {
	path := filepath.Join(cc.configDir, relPath)
	infof("loading configuration from %q", path)

	// find the checkpoint file(s)
	files, err := ioutil.ReadDir(path)
	if err != nil {
		panicf("failed to enumerate checkpoint files in dir %q, error: %v", path, err)
	} else if len(files) == 0 {
		return nil, fmt.Errorf("no checkpoint files in dir %q, but there should be at least one", path)
	}

	// TODO(mtaufen): for now, we only have one file per checkpoint (a serialized API object, e.g. a ConfigMap); if this ever changes we will need to extend this
	file := files[0]
	filePath := filepath.Join(path, file.Name())
	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		panicf("failed to read checkpoint file %q, error: %v", filePath, err)
	}

	// decode the checkpoint file
	obj, err := kuberuntime.Decode(api.Codecs.UniversalDecoder(), b)
	if err != nil {
		return nil, fmt.Errorf("failed to decode checkpoint file %q, error: %v", filePath, err)
	}

	// TODO(mtaufen): for now we assume we are trying to load a ConfigMap, but we may need to eventually be generic to the type

	// convert it to the external ConfigMap type, so we're consistently working with the external type outside of the on-disk representation
	cm := &apiv1.ConfigMap{}
	err = api.Scheme.Convert(obj, cm, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to convert decoded object into a v1 ConfigMap, error: %v", err)
	}

	return &verifiableConfigMap{cm: cm}, nil
}

type configMapCheckpoint apiv1.ConfigMap

// TODO(mtaufen): refactor this to pass a byte array to a saver interface
// func (c *configMapCheckpoint) save(saver checkpointStore) (cause string, reterr error) {
// 	data, reterr := c.bytes()
// 	if reterr != nil {
// 		return
// 	}
// 	reterr = saver.save(c.uid(), data)
// 	if reterr != nil {
// 		cause = fmt.Sprintf("failed to save checkpoint for object with UID %q", c.uid())
// 		return
// 	}

// 	return

// 	// TODO(mtaufen): return a loadableCheckpoint
// }

func (c *configMapCheckpoint) bytes() ([]byte, error) {
	cm := (*apiv1.ConfigMap)(c)

	// serialize to json
	mediaType := "application/json"
	info, ok := kuberuntime.SerializerInfoForMediaType(api.Codecs.SupportedMediaTypes(), mediaType)
	if !ok {
		return nil, fmt.Errorf("unsupported media type %q", mediaType)
	}

	versions := api.Registry.EnabledVersionsForGroup(apiv1.GroupName)
	if len(versions) == 0 {
		return nil, fmt.Errorf("no enabled versions for group %q", apiv1.GroupName)
	}

	// the "best" version supposedly comes first in the list returned from api.Registry.EnabledVersionsForGroup
	encoder := api.Codecs.EncoderForVersion(info.Serializer, versions[0])
	data, err := kuberuntime.Encode(encoder, cm)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (c *configMapCheckpoint) uid() string {
	return string(c.ObjectMeta.UID)
}

// While the save() method on the checkpoint interface is for dispatching the proper
// serialization process for the underlying object type, the save() method on checkpointStore
// is for plumbing that serialization to the proper storage location; similar for load().
type checkpointStore interface {
	// save saves the `data` representing a checkpoint to the appropriate location for `uid`.
	// The caller must serialize any objects to bytes before saving.
	save(c checkpoint) error
	// load loads the checkpoint described by `uid` and returns the bytes that represent
	// the checkpoint contents. The caller of load must deserialize the checkpoint.
	// load(uid string) (checkpoint, error)
}

// fsCheckpointSaver is for saving checkpoints to the local filesystem
type fsCheckpointStore struct {
	// checkpointsDir is an absolute path to the directory that checkpoints should be saved in,
	// e.g. cc.configDir/checkpointsDir
	checkpointsDir string
}

// TODO(mtaufen): cause doesnt need to be this low level, elevate cause generation
func (saver *fsCheckpointStore) save(c checkpoint) (reterr error) {
	uid := c.uid()
	data, reterr := c.bytes()
	if reterr != nil {
		return
	}

	uidPath := filepath.Join(saver.checkpointsDir, uid)
	err := os.Mkdir(uidPath, defaultPerm)
	if err != nil {
		reterr = fmt.Errorf("failed to save checkpoint for object with UID %s, err: %v", uid, err)
		return
	}

	// defer cleanup function now that we have something to clean up (we just created a dir)
	defer func() {
		if reterr != nil {
			// clean up the checkpoint dir
			rmerr := os.RemoveAll(uidPath)
			if rmerr != nil {
				reterr = fmt.Errorf("failed to save checkpoint for object with UID %s, error: %v; failed to clean up checkpoint dir, error: %v", uid, reterr, rmerr)
			}
			reterr = fmt.Errorf("failed to save checkpoint for object with UID %s, error: %v", uid, reterr)
		}
	}()

	// TODO(mtaufen): we might need to just make the UID things files instead of dirs, but that's ok it actually makes loading simpler too
	// checkpoint the configmap object we got
	filePath := filepath.Join(uidPath, "refactoring-kludge") // TODO(mtaufen): this is a kludge to carry me through the refactoring, will be gone soon

	// TODO(mtaufen): write to a tmp file and do a mv to make this more atomic; don't want crashes in the middle of
	// saving to corrupt a real checkpoint file
	// save the file
	reterr = ioutil.WriteFile(filePath, data, defaultPerm)
	return
}

// func (loader *fsCheckpointLoader) load(uid string) ([]byte, error) {
// 	return
// }
