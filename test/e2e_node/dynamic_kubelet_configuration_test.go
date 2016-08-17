/*
Copyright 2016 The Kubernetes Authors.

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

package e2e_node

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	// "strconv"
	// "strings"
	"time"

	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/api"
	// "k8s.io/kubernetes/pkg/api/resource"
	"k8s.io/kubernetes/test/e2e/framework"

	// "k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/componentconfig"
	"k8s.io/kubernetes/pkg/apis/componentconfig/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = fmt.Print
var _ = http.Client{}
var _ = api.ResourceRequirements{}
var _ = time.Second

// This test is marked [Disruptive] because the Kubelet temporarily goes down as part of of this test.
var _ = framework.KubeDescribe("DynamicKubeletConfiguration [Disruptive]", func() {
	f := framework.NewDefaultFramework("dynamic-kubelet-configuration-test")

	Context("When a configmap called `kube-<node-name>` is added to the `kube-system` namespace", func() {
		It("The Kubelet on that node should restart to take up the new config", func() {
			// Get a valid configmap by querying the configz endpoint for this node.

			// TODO: Yeah, probably need to poll or wait for the node to be running

			// Poll, because the node might not quite be up when the test starts running
			// TODO(mtaufen): @random-liu might make the framework wait for the node to be up,
			//                in which case we might not have to poll.

			resp := pollConfigz(2*time.Minute, 5*time.Second)
			glog.Infof("Response was: %+v", resp)
			kubeCfg, err := decodeConfigz(resp)
			if err != nil {
				glog.Fatalf("Failed to decode response from /configz, err: %v", err)
			}
			glog.Infof("And the first one came out like this: %+v", *kubeCfg)

			// TODO: Might be best to just create a spurrious node label and see if it shows up.
			// Change a safe value e.g. file check frequency. And make sure we're actually providing a new value
			oldFileCheckFrequency := kubeCfg.FileCheckFrequency.Duration
			newFileCheckFrequency := 11 * time.Second
			if kubeCfg.FileCheckFrequency.Duration == newFileCheckFrequency {
				newFileCheckFrequency = 10 * time.Second
			}
			kubeCfg.FileCheckFrequency.Duration = newFileCheckFrequency

			// Use the new config to create a new kube-<node-name> configmap to `kube-system` namespace.
			_, err = createConfigMap(f, kubeCfg)
			if err != nil {
				glog.Fatalf("Failed to create configmap, err: %v", err)
			}

			// Wait 15 seconds to give the kubelet time to restart and take up the new config
			time.Sleep(15 * time.Second)

			// Poll the configz endpoint to get the new config.
			resp = pollConfigz(2*time.Minute, 5*time.Second)
			kubeCfg, err = decodeConfigz(resp)
			if err != nil {
				glog.Fatalf("Failed to decode response from /configz, err: %v", err)
			}
			glog.Infof("And the second one came out like this: %+v", *kubeCfg)

			// We expect to see the value that we just set in the new config.
			Expect(kubeCfg.FileCheckFrequency.Duration).To(Equal(newFileCheckFrequency))

			// Set anything we changed back to what it used to be before the test finishes.
			kubeCfg.FileCheckFrequency.Duration = oldFileCheckFrequency
			_, err = updateConfigMap(f, kubeCfg)
			if err != nil {
				glog.Fatalf("Failed to update configmap, err: %v", err)
			}

			// TODO: Maybe do health checks for a while to make sure nothing fails.

			// TODO(mtaufen): Gate this test to gci images so we know we have systemd init. Then make sure the Kubelet will be restarted.
		})
	})
})

// This function either causes the test to fail, or it returns a status 200 response.
func pollConfigz(timeout time.Duration, pollInterval time.Duration) *http.Response {
	endpoint := fmt.Sprintf("http://127.0.0.1:8080/api/v1/proxy/nodes/%s/configz", framework.TestContext.NodeName)
	client := &http.Client{}
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		glog.Fatalf("Failed to build http request: %v", err)
	}
	req.Header.Add("Accept", "application/json")

	var resp *http.Response
	Eventually(func() bool {
		resp, err = client.Do(req)
		if err != nil {
			glog.Warningf("Failed to get /configz, retrying. Error was: %v", err)
			return false
		}
		if resp.StatusCode != 200 {
			glog.Warningf("Response status not 200, retrying. Response was: %+v", resp)
			return false
		}
		return true
	}, timeout, pollInterval).Should(Equal(true))
	return resp
}

// Decodes the http response and returns a componentconfig.KubeletConfiguration (internal type)
// Causes the test to fail if it encounters a problem.
func decodeConfigz(resp *http.Response) (*componentconfig.KubeletConfiguration, error) {

	// TODO(mtaufen): Clean up this hacky decoding.

	// This hack because /configz reports {"componentconfig": {the JSON representation of v1alpha1.KubeletConfiguration}}
	type configzWrapper struct {
		ComponentConfig v1alpha1.KubeletConfiguration `json:"componentconfig"`
	}

	configz := configzWrapper{}
	kubeCfg := componentconfig.KubeletConfiguration{}

	contentsBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	contents := string(contentsBytes)
	glog.Infof("Contents: %v", contents)
	// decoder := json.NewDecoder(strings.NewReader(contents))
	// err = decoder.Decode(&configz) // TODO(mtaufen): This decoder ain't working properly. "Downloaded this..." is reporting nil and 0s.....
	//                 I think this might be because json coming down looks like Contents: {"componentconfig":{  and we want what's in here!

	err = json.Unmarshal(contentsBytes, &configz)
	if err != nil {
		return nil, err
	}

	glog.Infof("Downloaded this configz: %+v", configz)

	err = api.Scheme.Convert(&configz.ComponentConfig, &kubeCfg)
	if err != nil {
		return nil, err
	}
	glog.Infof("Converted to internal type with this result: %+v", kubeCfg)

	return &kubeCfg, nil
}

// Hand this function the internal type and it will convert it to the external type and
// create it in the "kube-system" namespace for you
func createConfigMap(f *framework.Framework, kubeCfg *componentconfig.KubeletConfiguration) (*api.ConfigMap, error) {
	kubeCfgExt := v1alpha1.KubeletConfiguration{}
	api.Scheme.Convert(kubeCfg, &kubeCfgExt)
	glog.Infof("About to create this configuration: %+v", kubeCfgExt)
	bytes, err := json.Marshal(kubeCfgExt)
	if err != nil {
		glog.Fatalf("Error converting KubeletConfiguration to JSON: %v", err)
	}
	glog.Infof("JSON output: %+v", string(bytes))
	cmap, err := f.Client.ConfigMaps("kube-system").Create(&api.ConfigMap{
		ObjectMeta: api.ObjectMeta{
			Name: fmt.Sprintf("kubelet-%s", framework.TestContext.NodeName),
		},
		Data: map[string]string{
			"kubelet.config": string(bytes),
		},
	})
	if err != nil {
		return nil, err
	}
	return cmap, nil
}

func updateConfigMap(f *framework.Framework, kubeCfg *componentconfig.KubeletConfiguration) (*api.ConfigMap, error) {
	kubeCfgExt := v1alpha1.KubeletConfiguration{}
	api.Scheme.Convert(kubeCfg, &kubeCfgExt)
	glog.Infof("About to update this configuration: %+v", kubeCfgExt)
	bytes, err := json.Marshal(kubeCfgExt)
	if err != nil {
		glog.Fatalf("Error converting KubeletConfiguration to JSON: %v", err)
	}
	glog.Infof("JSON output: %+v", string(bytes))
	cmap, err := f.Client.ConfigMaps("kube-system").Update(&api.ConfigMap{
		ObjectMeta: api.ObjectMeta{
			Name: fmt.Sprintf("kubelet-%s", framework.TestContext.NodeName),
		},
		Data: map[string]string{
			"kubelet.config": string(bytes),
		},
	})
	if err != nil {
		return nil, err
	}
	return cmap, nil
}
