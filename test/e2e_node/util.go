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
	"crypto/sha1"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/golang/glog"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/apis/componentconfig"
	v1alpha1 "k8s.io/kubernetes/pkg/apis/componentconfig/v1alpha1"
	"k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/stats"
	// utilconfig "k8s.io/kubernetes/pkg/util/config"
	"k8s.io/kubernetes/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const nodeConfigKey = "node.config"

// TODO(random-liu): Get this automatically from kubelet flag.
var kubeletAddress = flag.String("kubelet-address", "http://127.0.0.1:10255", "Host and port of the kubelet")

var startServices = flag.Bool("start-services", true, "If true, start local node services")
var stopServices = flag.Bool("stop-services", true, "If true, stop local node services after running tests")

func getNodeSummary() (*stats.Summary, error) {
	req, err := http.NewRequest("GET", *kubeletAddress+"/stats/summary", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build http request: %v", err)
	}
	req.Header.Add("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get /stats/summary: %v", err)
	}

	defer resp.Body.Close()
	contentsBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read /stats/summary: %+v", resp)
	}

	decoder := json.NewDecoder(strings.NewReader(string(contentsBytes)))
	summary := stats.Summary{}
	err = decoder.Decode(&summary)
	if err != nil {
		return nil, fmt.Errorf("failed to parse /stats/summary to go struct: %+v", resp)
	}
	return &summary, nil
}

// Returns the current KubeletConfiguration
func getCurrentKubeletConfig() (*componentconfig.KubeletConfiguration, error) {
	resp := pollConfigz(5*time.Minute, 5*time.Second)
	kubeCfg, err := decodeConfigz(resp)
	if err != nil {
		return nil, err
	}
	return kubeCfg, nil
}

// Convenience method to set the evictionHard threshold during the current context.
func tempSetEvictionHard(f *framework.Framework, evictionHard string) {
	tempSetCurrentKubeletConfig(f, func(initialConfig *componentconfig.KubeletConfiguration) {
		initialConfig.EvictionHard = evictionHard
	})
}

// Must be called within a Context. Allows the function to modify the KubeletConfiguration during the BeforeEach of the context.
// The change is reverted in the AfterEach of the context.
func tempSetCurrentKubeletConfig(f *framework.Framework, updateFunction func(initialConfig *componentconfig.KubeletConfiguration)) {
	var oldCfg *componentconfig.KubeletConfiguration
	BeforeEach(func() {
		oldCfg, err := getCurrentKubeletConfig()
		framework.ExpectNoError(err)
		clone, err := api.Scheme.DeepCopy(oldCfg)
		framework.ExpectNoError(err)
		newCfg := clone.(*componentconfig.KubeletConfiguration)
		updateFunction(newCfg)
		setAndCheckKubeletConfiguration(f, newCfg)
	})
	AfterEach(func() {
		if oldCfg != nil {
			setAndCheckKubeletConfiguration(f, oldCfg)
		}
	})
}

func errorNoDynamicConfig(f *framework.Framework) error {
	cfgz, err := getCurrentKubeletConfig()
	if err != nil {
		return fmt.Errorf("could not determine whether 'DynamicKubeletConfig' feature is enabled, err: %v", err)
	}
	if strings.Contains(cfgz.FeatureGates, "DynamicKubeletConfig=true") {
		return nil
	}
	return fmt.Errorf("The Dynamic Kubelet Configuration feature is not enabled.\n" +
		"Pass --feature-gates=DynamicKubeletConfig=true to the Kubelet to enable this feature.\n" +
		"For `make test-e2e-node`, you can set `TEST_ARGS='--feature-gates=DynamicKubeletConfig=true'`.")
}

// Queries the API server for a Kubelet configuration for the node described by framework.TestContext.NodeName
func getCurrentNodeConfigMap(f *framework.Framework) (*v1.ConfigMap, error) {
	// Find the Node object associated with this Kubelet
	node, err := f.ClientSet.Core().Nodes().Get(framework.TestContext.NodeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	// Check for the node config annotation, if it exists grab the referenced ConfigMap from kube-system
	if cmName, exists := node.Annotations[nodeConfigKey]; exists {
		return f.ClientSet.Core().ConfigMaps("kube-system").Get(cmName, metav1.GetOptions{})
	}
	return nil, fmt.Errorf("Node.Annotations did not contain key %q", nodeConfigKey)
}

// Creates or updates the configmap for KubeletConfiguration, waits for the Kubelet to restart
// with the new configuration. The test will fail if anything goes wrong here.
func setAndCheckKubeletConfiguration(f *framework.Framework, newKubeCfg *componentconfig.KubeletConfiguration) {
	const restartGap = 30 * time.Second

	// Make sure dynamic configuration feature is enabled on the Kubelet we are about to reconfigure
	framework.ExpectNoError(errorNoDynamicConfig(f))

	// Construct the new Node ConfigMap to wrap the KubeletConfiguration
	cm := makeNodeConfigMap(newKubeCfg)

	// If a ConfigMap with the same name already exists, we assume its Data is the same as well,
	// even though it theoretically could be a hash collision.
	// We don't really care about that here because this is for tests, which will already
	// detect a mismatch between intended data and actual data.
	_, err := f.ClientSet.Core().ConfigMaps("kube-system").Get(cm.Name, metav1.GetOptions{})
	if k8serr.IsNotFound(err) {
		_, err := f.ClientSet.Core().ConfigMaps("kube-system").Create(cm)
		framework.ExpectNoError(err)
	} else if err != nil {
		framework.ExpectNoError(err)
	}

	// Update the Node's config annotation to point at the desired ConfigMap
	node, err := f.ClientSet.Core().Nodes().Get(framework.TestContext.NodeName, metav1.GetOptions{})
	framework.ExpectNoError(err)
	node.Annotations[nodeConfigKey] = cm.Name
	_, err = f.ClientSet.Core().Nodes().Update(cm)
	framework.ExpectNoError(err)

	// Wait for the Kubelet to restart.
	time.Sleep(restartGap)

	// Retrieve the current Kubelet config and compare it to the one we attempted to set via Node config
	curKubeCfg, err := getCurrentKubeletConfig()
	framework.ExpectNoError(err)

	// Error if the desired config is not in use by now
	if !reflect.DeepEqual(*newKubeCfg, *curKubeCfg) {
		framework.ExpectNoError(fmt.Errorf("either the Kubelet did not restart or it did not present the modified configuration via /configz after restarting."))
	}
}

// Causes the test to fail, or returns a status 200 response from the /configz endpoint
func pollConfigz(timeout time.Duration, pollInterval time.Duration) *http.Response {
	endpoint := fmt.Sprintf("http://127.0.0.1:8080/api/v1/proxy/nodes/%s/configz", framework.TestContext.NodeName)
	client := &http.Client{}
	req, err := http.NewRequest("GET", endpoint, nil)
	framework.ExpectNoError(err)
	req.Header.Add("Accept", "application/json")

	var resp *http.Response
	Eventually(func() bool {
		resp, err = client.Do(req)
		if err != nil {
			glog.Errorf("Failed to get /configz, retrying. Error: %v", err)
			return false
		}
		if resp.StatusCode != 200 {
			glog.Errorf("/configz response status not 200, retrying. Response was: %+v", resp)
			return false
		}
		return true
	}, timeout, pollInterval).Should(Equal(true))
	return resp
}

// Decodes the http response from /configz and returns a componentconfig.KubeletConfiguration (internal type).
func decodeConfigz(resp *http.Response) (*componentconfig.KubeletConfiguration, error) {
	// This hack because /configz reports the following structure:
	// {"componentconfig": {the JSON representation of v1alpha1.KubeletConfiguration}}
	type configzWrapper struct {
		ComponentConfig v1alpha1.KubeletConfiguration `json:"componentconfig"`
	}

	configz := configzWrapper{}
	kubeCfg := componentconfig.KubeletConfiguration{}

	contentsBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(contentsBytes, &configz)
	if err != nil {
		return nil, err
	}

	err = api.Scheme.Convert(&configz.ComponentConfig, &kubeCfg, nil)
	if err != nil {
		return nil, err
	}

	return &kubeCfg, nil
}

// Appends the hash of the JSON serialization of the ConfigMap's Data field to its name
func appendHashToName(cm *v1.ConfigMap) *v1.ConfigMap {
	b, err := json.Marshal(cm.Data)
	framework.ExpectNoError(err)
	hash := fmt.Sprintf("%x", sha1.Sum(b))
	cm.Name = fmt.Sprintf("%s-%s", cm.Name, hash)
	return cm
}

// Constructs a Node ConfigMap that contains the specified KubeletConfiguration. The ConfigMap's name will contain a hash of its data.
func makeNodeConfigMap(kubeCfg *componentconfig.KubeletConfiguration) *v1.ConfigMap {
	kubeCfgExt := v1alpha1.KubeletConfiguration{}
	api.Scheme.Convert(kubeCfg, &kubeCfgExt, nil)

	bytes, err := json.Marshal(kubeCfgExt)
	framework.ExpectNoError(err)

	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-config",
		},
		Data: map[string]string{
			"kubelet": string(bytes),
		},
	}
	return appendHashToName(cm)
}

func logPodEvents(f *framework.Framework) {
	framework.Logf("Summary of pod events during the test:")
	err := framework.ListNamespaceEvents(f.ClientSet, f.Namespace.Name)
	framework.ExpectNoError(err)
}

func logNodeEvents(f *framework.Framework) {
	framework.Logf("Summary of node events during the test:")
	err := framework.ListNamespaceEvents(f.ClientSet, "")
	framework.ExpectNoError(err)
}
