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
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/resource"
	"k8s.io/kubernetes/pkg/apis/componentconfig"
	"k8s.io/kubernetes/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// Eviction Policy is described here:
// https://github.com/kubernetes/kubernetes/blob/master/docs/proposals/kubelet-eviction.md

// TODO(mtaufen): Roll KubeletConfiguration management from here and the dynamic kubelet configuration
// test into a utility library for node e2e tests.

// Returns the current KubeletConfiguration
func getCurrentKubeletConfig() (*componentconfig.KubeletConfiguration, error) {
	resp := pollConfigz(5*time.Minute, 5*time.Second)
	kubeCfg, err := decodeConfigz(resp)
	if err != nil {
		return nil, err
	}
	return kubeCfg, nil
}

func getCurrentKubeletConfigMap(f *framework.Framework) (*api.ConfigMap, error) {
	return f.Client.ConfigMaps("kube-system").Get(fmt.Sprintf("kubelet-%s", framework.TestContext.NodeName))
}

// Creates or updates the configmap for KubeletConfiguration, waits for the Kubelet to restart
// with the new configuration. Returns an error if the configuration after waiting 40 seconds
// doesn't match what you attempted to set.
func setKubeletConfiguration(f *framework.Framework, kubeCfg *componentconfig.KubeletConfiguration) error {
	const (
		restartGap = 40 * time.Second
	)
	// Check whether a configmap for KubeletConfiguration already exists
	_, err := getCurrentKubeletConfigMap(f)
	if err != nil {
		// TODO(mtaufen): Find the actual error message for resources not existing, and only
		// attempt to create the configmap when it doesn't exist, not when we get some other error.
		_, err := createConfigMap(f, kubeCfg)
		if err != nil {
			return err
		}
	} else {
		// The configmap exists, update it instead of creating it.
		_, err := updateConfigMap(f, kubeCfg)
		if err != nil {
			return err
		}
	}

	// Wait for the Kubelet to restart.
	time.Sleep(restartGap)

	// Retrieve the new config and compare it to the one we attempted to set
	newKubeCfg, err := getCurrentKubeletConfig()
	if err != nil {
		return err
	}

	// TODO(mtaufen): Do a deep comparison here, return error if they don't match.
	if !reflect.DeepEqual(*kubeCfg, *newKubeCfg) {
		return fmt.Errorf("The Kubelet does NOT have the configuration you tried to set for this test! Something went wrong!")
	}
	return nil
}

var _ = framework.KubeDescribe("MemoryEviction [Slow] [Serial] [Disruptive]", func() {
	f := framework.NewDefaultFramework("eviction-test")
	var initialKubeletConfiguration componentconfig.KubeletConfiguration

	BeforeEach(func() {
		const (
			threshold = " memory.available<40%"
		)
		kubeCfg, err := getCurrentKubeletConfig()
		framework.ExpectNoError(err)
		initialKubeletConfiguration = *kubeCfg // Copy into initialKubeletConfiguration

		// If EvictionHard contains a memory eviction threshold, edit it.
		// Otherwise append the desired threshold.
		edited := false
		settings := strings.Split(kubeCfg.EvictionHard, ",")
		// TODO(mtaufen): This assumes that there is only one memory.available threshold in the list.
		//                Need to double-check whether there can be more than one.
		for i, str := range settings {
			if strings.Contains(str, "memory.available") {
				settings[i] = threshold
				edited = true
				break
			}
		}
		if !edited {
			settings = append(settings, threshold)
		}
		kubeCfg.EvictionHard = strings.Join(settings, ",")

		err = setKubeletConfiguration(f, kubeCfg)
		framework.ExpectNoError(err)
	})
	// AfterEach blocks run in deep to shallow order, so the AfterEach in the Context
	// block below will be run prior to this AfterEach.
	AfterEach(func() {
		// TODO(mtaufen): Restore initial kubelet configuration here
		Expect(initialKubeletConfiguration).NotTo(BeNil())
		err := setKubeletConfiguration(f, &initialKubeletConfiguration)
		framework.ExpectNoError(err)
	})

	Context("when there is memory pressure", func() {
		AfterEach(func() {
			// Wait for the memory pressure condition to disappear from the node status before continuing.
			By("waiting for the memory pressure condition on the node to disappear before ending the test.")
			Eventually(func() error {
				nodeList, err := f.Client.Nodes().List(api.ListOptions{})
				if err != nil {
					return fmt.Errorf("tried to get node list but got error: %v", err)
				}
				// Assuming that there is only one node, because this is a node e2e test.
				if len(nodeList.Items) != 1 {
					return fmt.Errorf("expected 1 node, but see %d. List: %v", len(nodeList.Items), nodeList.Items)
				}
				node := nodeList.Items[0]
				_, pressure := api.GetNodeCondition(&node.Status, api.NodeMemoryPressure)
				if pressure != nil && pressure.Status == api.ConditionTrue {
					return fmt.Errorf("node is still reporting memory pressure condition: %s", pressure)
				}
				return nil
			}, 5*time.Minute, 15*time.Second).Should(BeNil())

			// Check available memory after condition disappears, just in case:
			// Wait for available memory to decrease to a reasonable level before ending the test.
			// This helps prevent interference with tests that start immediately after this one.
			By("waiting for available memory to decrease to a reasonable level before ending the test.")
			Eventually(func() error {
				summary, err := getNodeSummary()
				if err != nil {
					return err
				}
				if summary.Node.Memory.AvailableBytes == nil {
					return fmt.Errorf("summary.Node.Memory.AvailableBytes was nil, cannot get memory stats.")
				}
				if summary.Node.Memory.WorkingSetBytes == nil {
					return fmt.Errorf("summary.Node.Memory.WorkingSetBytes was nil, cannot get memory stats.")
				}
				avail := *summary.Node.Memory.AvailableBytes
				wset := *summary.Node.Memory.WorkingSetBytes

				// memory limit = avail + wset
				limit := avail + wset
				halflimit := limit / 2

				// Wait for at least half of memory limit to be available
				if avail >= halflimit {
					return nil
				}
				return fmt.Errorf("current available memory is: %d bytes. Expected at least %d bytes available.", avail, halflimit)
			}, 5*time.Minute, 15*time.Second).Should(BeNil())

			// TODO(mtaufen): 5 minute wait to stop flaky test bleeding while we figure out what is actually going on.
			//                If related to pressure transition period in eviction manager, probably only need to wait
			//                just over 30s becasue that is the transition period set for node e2e tests. But since we
			//                know 5 min works and we don't know if transition period is the problem, wait 5 min for now.
			time.Sleep(5 * time.Minute)

			// Finally, try starting a new pod and wait for it to be scheduled and running.
			// This is the final check to try to prevent interference with subsequent tests.
			podName := "admit-best-effort-pod"
			f.PodClient().CreateSync(&api.Pod{
				ObjectMeta: api.ObjectMeta{
					Name: podName,
				},
				Spec: api.PodSpec{
					RestartPolicy: api.RestartPolicyNever,
					Containers: []api.Container{
						{
							Image: ImageRegistry[pauseImage],
							Name:  podName,
						},
					},
				},
			})
		})

		It("should evict pods in the correct order (besteffort first, then burstable, then guaranteed)", func() {
			By("creating a guaranteed pod, a burstable pod, and a besteffort pod.")

			// A pod is guaranteed only when requests and limits are specified for all the containers and they are equal.
			guaranteed := createMemhogPod(f, "guaranteed-", "guaranteed", api.ResourceRequirements{
				Requests: api.ResourceList{
					"cpu":    resource.MustParse("100m"),
					"memory": resource.MustParse("100Mi"),
				},
				Limits: api.ResourceList{
					"cpu":    resource.MustParse("100m"),
					"memory": resource.MustParse("100Mi"),
				}})

			// A pod is burstable if limits and requests do not match across all containers.
			burstable := createMemhogPod(f, "burstable-", "burstable", api.ResourceRequirements{
				Requests: api.ResourceList{
					"cpu":    resource.MustParse("100m"),
					"memory": resource.MustParse("100Mi"),
				}})

			// A pod is besteffort if none of its containers have specified any requests or limits.
			besteffort := createMemhogPod(f, "besteffort-", "besteffort", api.ResourceRequirements{})

			// We poll until timeout or all pods are killed.
			// Inside the func, we check that all pods are in a valid phase with
			// respect to the eviction order of best effort, then burstable, then guaranteed.
			By("polling the Status.Phase of each pod and checking for violations of the eviction order.")
			Eventually(func() error {

				gteed, gtErr := f.Client.Pods(f.Namespace.Name).Get(guaranteed.Name)
				framework.ExpectNoError(gtErr, fmt.Sprintf("getting pod %s", guaranteed.Name))
				gteedPh := gteed.Status.Phase

				burst, buErr := f.Client.Pods(f.Namespace.Name).Get(burstable.Name)
				framework.ExpectNoError(buErr, fmt.Sprintf("getting pod %s", burstable.Name))
				burstPh := burst.Status.Phase

				best, beErr := f.Client.Pods(f.Namespace.Name).Get(besteffort.Name)
				framework.ExpectNoError(beErr, fmt.Sprintf("getting pod %s", besteffort.Name))
				bestPh := best.Status.Phase

				glog.Infof("pod phase: guaranteed: %v, burstable: %v, besteffort: %v", gteedPh, burstPh, bestPh)

				if bestPh == api.PodRunning {
					Expect(burstPh).NotTo(Equal(api.PodFailed), "burstable pod failed before best effort pod")
					Expect(gteedPh).NotTo(Equal(api.PodFailed), "guaranteed pod failed before best effort pod")
				} else if burstPh == api.PodRunning {
					Expect(gteedPh).NotTo(Equal(api.PodFailed), "guaranteed pod failed before burstable pod")
				}

				// When both besteffort and burstable have been evicted, the test has completed.
				if bestPh == api.PodFailed && burstPh == api.PodFailed {
					return nil
				}
				return fmt.Errorf("besteffort and burstable have not yet both been evicted.")

			}, 60*time.Minute, 5*time.Second).Should(BeNil())

		})
	})
})

func createMemhogPod(f *framework.Framework, genName string, ctnName string, res api.ResourceRequirements) *api.Pod {
	env := []api.EnvVar{
		{
			Name: "MEMORY_LIMIT",
			ValueFrom: &api.EnvVarSource{
				ResourceFieldRef: &api.ResourceFieldSelector{
					Resource: "limits.memory",
				},
			},
		},
	}

	// If there is a limit specified, pass 80% of it for -mem-total, otherwise use the downward API
	// to pass limits.memory, which will be the total memory available.
	// This helps prevent a guaranteed pod from triggering an OOM kill due to it's low memory limit,
	// which will cause the test to fail inappropriately.
	var memLimit string
	if limit, ok := res.Limits["memory"]; ok {
		memLimit = strconv.Itoa(int(
			float64(limit.Value()) * 0.8))
	} else {
		memLimit = "$(MEMORY_LIMIT)"
	}

	pod := &api.Pod{
		ObjectMeta: api.ObjectMeta{
			GenerateName: genName,
		},
		Spec: api.PodSpec{
			RestartPolicy: api.RestartPolicyNever,
			Containers: []api.Container{
				{
					Name:            ctnName,
					Image:           "gcr.io/google-containers/stress:v1",
					ImagePullPolicy: "Always",
					Env:             env,
					// 60 min timeout * 60s / tick per 10s = 360 ticks before timeout => ~11.11Mi/tick
					// to fill ~4Gi of memory, so initial ballpark 12Mi/tick.
					// We might see flakes due to timeout if the total memory on the nodes increases.
					Args:      []string{"-mem-alloc-size", "12Mi", "-mem-alloc-sleep", "10s", "-mem-total", memLimit},
					Resources: res,
				},
			},
		},
	}
	// The generated pod.Name will be on the pod spec returned by CreateSync
	pod = f.PodClient().CreateSync(pod)
	glog.Infof("pod created with name: %s", pod.Name)
	return pod
}
