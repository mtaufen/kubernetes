/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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
	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/resource"
	"k8s.io/kubernetes/pkg/util/wait"
	"k8s.io/kubernetes/test/e2e/framework"
	"time"

	// dockerapi "github.com/docker/engine-api/client"
	// dockertypes "github.com/docker/engine-api/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = framework.KubeDescribe("ResourceEvict", func() {
	// var dockerClient *dockerapi.Client

	// BeforeEach(func() {
	// 	var err error
	// 	dockerClient, err = dockerapi.NewEnvClient()
	// 	Expect(err).To(BeNil(), fmt.Sprintf("Error connecting to docker %v", err))
	// })

	Context("when there is memory pressure", func() {
		f := NewDefaultFramework("resource-eviction-test")

		It("It should evict pods in the correct order", func() {
			glog.Info("about to make the podclient")
			podClient := f.Client.Pods(f.Namespace.Name)
			glog.Info("made the podclient")

			// A pod is guaranteed only when requests and limits are specified for all the containers and they are equal.
			guaranteedPod := &api.Pod{
				ObjectMeta: api.ObjectMeta{
					GenerateName: "guaranteed-",
				},
				Spec: api.PodSpec{
					NodeName:      *nodeName,
					RestartPolicy: api.RestartPolicyNever,
					Containers: []api.Container{
						{
							Name:            "guaranteed",
							Image:           "derekwaynecarr/memhog",
							ImagePullPolicy: "Always",
							Command: []string{"/bin/sh",
								"-c",
								"while true; do memhog -r100 200m; sleep 1; done"},
							Resources: api.ResourceRequirements{
								Requests: api.ResourceList{
									"cpu":    resource.MustParse("100m"),
									"memory": resource.MustParse("100Mi"),
								},
								Limits: api.ResourceList{
									"cpu":    resource.MustParse("100m"),
									"memory": resource.MustParse("100Mi"),
								},
							},
						},
					},
				},
			}
			{
				var err error
				guaranteedPod, err = podClient.Create(guaranteedPod)
				Expect(err).To(BeNil(), fmt.Sprintf("Error creating burstable Pod %v", err))
				// framework.ExpectNoError(f.WaitForPodRunning(guaranteedPod.Name))
			}

			// A pod is burstable if limits and requests do not match across all containers.
			burstablePod := &api.Pod{
				ObjectMeta: api.ObjectMeta{
					GenerateName: "besteffort-",
				},
				Spec: api.PodSpec{
					NodeName:      *nodeName,
					RestartPolicy: api.RestartPolicyNever,
					Containers: []api.Container{
						{
							Name:            "burstable",
							Image:           "derekwaynecarr/memhog",
							ImagePullPolicy: "Always",
							Command: []string{"/bin/sh",
								"-c",
								"while true; do memhog -r100 200m; sleep 1; done"},
							Resources: api.ResourceRequirements{
								Requests: api.ResourceList{
									"cpu":    resource.MustParse("100m"),
									"memory": resource.MustParse("100Mi"),
								},
							},
						},
					},
				},
			}
			{
				var err error
				burstablePod, err = podClient.Create(burstablePod)
				Expect(err).To(BeNil(), fmt.Sprintf("Error creating burstable Pod %v", err))
				// framework.ExpectNoError(f.WaitForPodRunning(burstablePod.Name))
			}

			// A pod is besteffort if none of its containers have specified any requests or limits.
			besteffortPod := &api.Pod{
				ObjectMeta: api.ObjectMeta{
					GenerateName: "besteffort-",
				},
				Spec: api.PodSpec{
					NodeName:      *nodeName,
					RestartPolicy: api.RestartPolicyNever,
					Containers: []api.Container{
						{
							Name:            "besteffort",
							Image:           "derekwaynecarr/memhog",
							ImagePullPolicy: "Always",
							Command: []string{"/bin/sh",
								"-c",
								"while true; do memhog -r100 200m; sleep 1; done"},
						},
					},
				},
			}
			{
				var err error
				besteffortPod, err = podClient.Create(besteffortPod)
				Expect(err).To(BeNil(), fmt.Sprintf("Error creating besteffort Pod %v", err))
				glog.Infof("made the besteffort pod, waiting for it to start running (%s)", besteffortPod.Name)
				// framework.ExpectNoError(f.WaitForPodRunning(besteffortPod.Name))
			}

			// TODO: Need to add a wait in here somewhere so I can watch for eviction
			glog.Info("made the pods, about to wait for eviction")
			waitForPodEviction(guaranteedPod.ObjectMeta.Name, f)
			glog.Info("done waiting for eviction")

			// for {
			// }

			/* Pods should be evicted in this order:
			Best effort, then
			Burstable, then
			Guaranteed

			// See some of Derek's stuff here:
			// https://github.com/derekwaynecarr/kubernetes/tree/examples-eviction/demo/kubelet-eviction

			Is there a related github issue?

			This looks like a related pull request:
			https://github.com/kubernetes/kubernetes/pull/25772
			I think that modified some of the eviction logic.


			Eviction Policy is described here:
			https://github.com/kubernetes/kubernetes/blob/master/docs/proposals/kubelet-eviction.md



			One idea:
			Apply pressure through guaranteed, make sure best effort and burstable get evicted
			Apply pressure through burstable, make sure best effort is evicted but not guaranteed
			(still need to make sure best effort gets evicted before burstable)
			Check if guaranteed keeps applying too much pressure, that it gets evicted?


			*/

		})
	})

	Context("When there is disk pressure", func() {

		It("should evict pods in the correct order", func() {

		})

	})

})

// TODO: Wait for eviction function
func waitForPodEviction(podName string, f *framework.Framework) error {
	const (
		pollPeriod  = 10 * time.Second
		pollTimeout = 5 * time.Minute
	)
	podClient := f.Client.Pods(f.Namespace.Name)
	glog.Infof("Podname waz %s", podName)

	err := wait.PollImmediate(pollPeriod, pollTimeout, func() (bool, error) {
		pod, err := podClient.Get(podName)
		if err != nil {
			glog.Errorf("Error getting pod (%s): %v", podName, err)
			return false, nil
		}
		glog.Infof("This is what I gotz: %v", pod)

		glog.Infof("Waiting for pod (%s) to be evicted", podName)
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for pod (%s) to be evicted", podName)
	}
	glog.Infof("Finished waiting, %s was evicted", podName)
	return nil
}
