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
	"strings"
	"time"

	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/api"
	// "k8s.io/kubernetes/pkg/api/resource"
	"k8s.io/kubernetes/test/e2e/framework"

	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/componentconfig"
	"k8s.io/kubernetes/pkg/apis/componentconfig/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = fmt.Print
var _ = http.Client{}
var _ = api.ResourceRequirements{}
var _ = time.Second

var _ = framework.KubeDescribe("DynamicKubeletConfiguration", func() {
	f := framework.NewDefaultFramework("dynamic-kubelet-configuration-test")

	Context("When a configmap called `kube-<node-name>` is added to the `kube-system` namespace", func() {
		It("The Kubelet on that node should restart to take up the new config", func() {
			// For simplicity, we just update the Kubelet configuration for the node we are currently on.

			// Get a valid configmap by querying the configz endpoint for this node.
			// TODO: Might need to poll to get this one.

			// TODO: Yeah, probably need to poll or wait for the node to be running

			// glog.Infof("f: %v", f)

			// f.Client

			endpoint := fmt.Sprintf("http://127.0.0.1:8080/api/v1/proxy/nodes/%s/configz", framework.TestContext.NodeName)

			client := &http.Client{}
			// req, err := http.NewRequest("GET", "https://localhost:10250/configz", nil)
			req, err := http.NewRequest("GET", endpoint, nil) //v1/proxy/nodes/127.0.0.1:10250/configz
			if err != nil {
				glog.Warningf("Failed to build http request: %v", err)
				// return false
			}
			req.Header.Add("Accept", "application/json")

			// Poll, because the node might not quite be up when the test starts running
			// TODO(mtaufen): @random-liu might make the framework wait for the node to be up,
			//                in which case we might not have to poll.
			var resp *http.Response
			var decoder *json.Decoder
			Eventually(func() bool {
				resp, err = client.Do(req)
				if err != nil {
					glog.Warningf("Failed to get /configz, retrying: %v", err)
					return false
				}
				if resp.StatusCode != 200 {
					glog.Warningf("Response status not 200, retrying: %+v", resp)
					return false
				}
				contentsBytes, err := ioutil.ReadAll(resp.Body)
				// TODO: Probably use expect no error at this point. Maybe re-poll if error before here.
				if err != nil {
					glog.Warningf("Failed to read /configz, retrying: %+v", resp)
					return false
				}
				contents := string(contentsBytes)
				glog.Infof("Contents: %v", contents)
				decoder = json.NewDecoder(strings.NewReader(contents))
				return true
			}, 14*time.Second, 2*time.Second).Should(Equal(true))

			kubeCfgExt := v1alpha1.KubeletConfiguration{}
			err = decoder.Decode(&kubeCfgExt)
			if err != nil {
				glog.Warningf("Failed to parse /configz to go struct: %+v", resp)
				// return false
			}

			glog.Infof("Downloaded this configuration: %+v", kubeCfgExt)

			// TODO: Convert to internal and modify a value
			// Change a safe value e.g. file check frequency.
			kubeCfg := componentconfig.KubeletConfiguration{}
			api.Scheme.Convert(&kubeCfgExt, &kubeCfg)
			// TODO: Might be best to just create a spurrious node label and see if it shows up.
			kubeCfg.FileCheckFrequency = unversioned.Duration{Duration: 22 * time.Second}
			newKubeCfgExt := v1alpha1.KubeletConfiguration{}
			api.Scheme.Convert(&kubeCfg, &newKubeCfgExt)

			glog.Infof("About to post this configuration: %+v", newKubeCfgExt)

			bytes, err := json.Marshal(newKubeCfgExt)
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

			glog.Infof("Configmap I created %+v", cmap) // TODO(mtaufen): Well there's no real output here...

			// TODO(mtaufen): Refactor original polling into a function
			// TODO(mtaufen): Gate this test to gci images so we know we have systemd init. Then make sure the Kubelet will be restarted.

			// TODO(mtaufen): Goes in kubelet.config key of configmap

			// todo make a configmap

			// Use that config to post a new kube-<node-name> configmap to `kube-system` namespace.

			// Poll the configz endpoint to get the new config.

			// TODO: Maybe do health checks for a while to make sure nothing fails.

			// TODO: Try testing with built-in defaults as well.

			// // A pod is guaranteed only when requests and limits are specified for all the containers and they are equal.
			// guaranteed := createMemhogPod(f, "guaranteed-", "guaranteed", api.ResourceRequirements{
			// 	Requests: api.ResourceList{
			// 		"cpu":    resource.MustParse("100m"),
			// 		"memory": resource.MustParse("100Mi"),
			// 	},
			// 	Limits: api.ResourceList{
			// 		"cpu":    resource.MustParse("100m"),
			// 		"memory": resource.MustParse("100Mi"),
			// 	}})

			// // A pod is burstable if limits and requests do not match across all containers.
			// burstable := createMemhogPod(f, "burstable-", "burstable", api.ResourceRequirements{
			// 	Requests: api.ResourceList{
			// 		"cpu":    resource.MustParse("100m"),
			// 		"memory": resource.MustParse("100Mi"),
			// 	}})

			// // A pod is besteffort if none of its containers have specified any requests or limits.
			// besteffort := createMemhogPod(f, "besteffort-", "besteffort", api.ResourceRequirements{})

			// // We poll until timeout or all pods are killed.
			// // Inside the func, we check that all pods are in a valid phase with
			// // respect to the eviction order of best effort, then burstable, then guaranteed.
			// By("Polling the Status.Phase of each pod and checking for violations of the eviction order.")
			// Eventually(func() bool {

			// 	gteed, gtErr := f.Client.Pods(f.Namespace.Name).Get(guaranteed.Name)
			// 	framework.ExpectNoError(gtErr, fmt.Sprintf("getting pod %s", guaranteed.Name))
			// 	gteedPh := gteed.Status.Phase

			// 	burst, buErr := f.Client.Pods(f.Namespace.Name).Get(burstable.Name)
			// 	framework.ExpectNoError(buErr, fmt.Sprintf("getting pod %s", burstable.Name))
			// 	burstPh := burst.Status.Phase

			// 	best, beErr := f.Client.Pods(f.Namespace.Name).Get(besteffort.Name)
			// 	framework.ExpectNoError(beErr, fmt.Sprintf("getting pod %s", besteffort.Name))
			// 	bestPh := best.Status.Phase

			// 	glog.Infof("Pod phase: guaranteed: %v, burstable: %v, besteffort: %v", gteedPh, burstPh, bestPh)

			// 	if bestPh == api.PodRunning {
			// 		Expect(burstPh).NotTo(Equal(api.PodFailed), "Burstable pod failed before best effort pod")
			// 		Expect(gteedPh).NotTo(Equal(api.PodFailed), "Guaranteed pod failed before best effort pod")
			// 	} else if burstPh == api.PodRunning {
			// 		Expect(gteedPh).NotTo(Equal(api.PodFailed), "Guaranteed pod failed before burstable pod")
			// 	}

			// 	// When both besteffort and burstable have been evicted, return true, else false
			// 	if bestPh == api.PodFailed && burstPh == api.PodFailed {
			// 		return true
			// 	}
			// 	return false

			// }, 60*time.Minute, 5*time.Second).Should(Equal(true))

		})
	})

})

// func createMemhogPod(f *framework.Framework, genName string, ctnName string, res api.ResourceRequirements) *api.Pod {
// 	env := []api.EnvVar{
// 		{
// 			Name: "MEMORY_LIMIT",
// 			ValueFrom: &api.EnvVarSource{
// 				ResourceFieldRef: &api.ResourceFieldSelector{
// 					Resource: "limits.memory",
// 				},
// 			},
// 		},
// 	}

// 	// If there is a limit specified, pass 80% of it for -mem-total, otherwise use the downward API
// 	// to pass limits.memory, which will be the total memory available.
// 	// This helps prevent a guaranteed pod from triggering an OOM kill due to it's low memory limit,
// 	// which will cause the test to fail inappropriately.
// 	var memLimit string
// 	if limit, ok := res.Limits["memory"]; ok {
// 		memLimit = strconv.Itoa(int(
// 			float64(limit.Value()) * 0.8))
// 	} else {
// 		memLimit = "$(MEMORY_LIMIT)"
// 	}

// 	pod := &api.Pod{
// 		ObjectMeta: api.ObjectMeta{
// 			GenerateName: genName,
// 		},
// 		Spec: api.PodSpec{
// 			RestartPolicy: api.RestartPolicyNever,
// 			Containers: []api.Container{
// 				{
// 					Name:            ctnName,
// 					Image:           "gcr.io/google-containers/stress:v1",
// 					ImagePullPolicy: "Always",
// 					Env:             env,
// 					// 60 min timeout * 60s / tick per 10s = 360 ticks before timeout => ~11.11Mi/tick
// 					// to fill ~4Gi of memory, so initial ballpark 12Mi/tick.
// 					// We might see flakes due to timeout if the total memory on the nodes increases.
// 					Args:      []string{"-mem-alloc-size", "12Mi", "-mem-alloc-sleep", "10s", "-mem-total", memLimit},
// 					Resources: res,
// 				},
// 			},
// 		},
// 	}
// 	// The generated pod.Name will be on the pod spec returned by CreateSync
// 	pod = f.PodClient().CreateSync(pod)
// 	glog.Infof("pod created with name: %s", pod.Name)
// 	return pod
// }
