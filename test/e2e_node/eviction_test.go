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
	"time"

	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/resource"
	"k8s.io/kubernetes/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// TODO: Talk to Phil and get this set up to run in serial before merging PR

// Eviction Policy is described here:
// https://github.com/kubernetes/kubernetes/blob/master/docs/proposals/kubelet-eviction.md

var _ = framework.KubeDescribe("Eviction [Slow] [Serial]", func() {
	f := NewDefaultFramework("eviction-test")

	Context("When there is memory pressure", func() {
		It("It should evict pods in the correct order (besteffort first, then burstable, then guaranteed)", func() {
			By("Creating a guaranteed pod, a burstable pod, and a besteffort pod.")

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
			By("Polling the Status.Phase of each pod and checking for violations of the eviction order.")
			Eventually(func() bool {

				gteed, gtErr := f.Client.Pods(f.Namespace.Name).Get(guaranteed.Name)
				framework.ExpectNoError(gtErr, fmt.Sprintf("getting pod %s", guaranteed.Name))
				gteedPh := gteed.Status.Phase

				burst, buErr := f.Client.Pods(f.Namespace.Name).Get(burstable.Name)
				framework.ExpectNoError(buErr, fmt.Sprintf("getting pod %s", burstable.Name))
				burstPh := burst.Status.Phase

				best, beErr := f.Client.Pods(f.Namespace.Name).Get(besteffort.Name)
				framework.ExpectNoError(beErr, fmt.Sprintf("getting pod %s", besteffort.Name))
				bestPh := best.Status.Phase

				glog.Infof("Pod phase: guaranteed: %v, burstable: %v, besteffort: %v", gteedPh, burstPh, bestPh)

				if bestPh == api.PodRunning {
					Expect(burstPh).NotTo(Equal(api.PodFailed), "Burstable pod failed before best effort pod")
					Expect(gteedPh).NotTo(Equal(api.PodFailed), "Guaranteed pod failed before best effort pod")
				} else if burstPh == api.PodRunning {
					Expect(gteedPh).NotTo(Equal(api.PodFailed), "Guaranteed pod failed before burstable pod")
				}

				// TODO: For now, we end the test when the best and burst are failed,
				//       eventually we will go back to ending when all are failed, but
				//       to fail the gteed we need something to hog memory and be charged
				//       to the host, and we haven't done that yet.
				// When all the pods are evicted, return true, else false
				if bestPh == api.PodFailed && burstPh == api.PodFailed /*&& gteedPh == api.PodFailed*/ {
					return true
				}
				return false

			}, 20*time.Minute, 5*time.Second).Should(Equal(true))

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
		{
			Name: "MEMORY_REQUEST",
			ValueFrom: &api.EnvVarSource{
				ResourceFieldRef: &api.ResourceFieldSelector{
					Resource: "requests.memory",
				},
			},
		},
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
					Image:           "gcr.io/google_containers/ubuntu:14.04",
					ImagePullPolicy: "Always",
					Env:             env,
					Command: []string{"/bin/bash",
						"-c",
						"(( MEM_TICKS = (MEMORY_LIMIT/(1024*1024*20))-2 )); " +
							"TICK_COUNTER=0; " +
							"while [ $TICK_COUNTER -lt $MEM_TICKS ]; " +
							"do " +
							"((TICK_COUNTER = TICK_COUNTER + 1)); " +
							"echo $MEMORY_LIMIT $MEM_TICKS $TICK_COUNTER; " +
							"dd bs=10M count=1 oflag=append conv=notrunc if=/dev/zero of=/memhog-1/hog; " +
							"dd bs=10M count=1 oflag=append conv=notrunc if=/dev/zero of=/memhog-2/hog; " +
							"sleep 10; " +
							"done; " +
							"while true; " +
							"do sleep 10; " +
							"done"},
					Resources: res,
					VolumeMounts: []api.VolumeMount{
						{
							Name:      "memhog-1-volume",
							MountPath: "/memhog-1",
						},
						{
							Name:      "memhog-2-volume",
							MountPath: "/memhog-2",
						},
					},
				},
			},
			Volumes: []api.Volume{
				// An emptyDir mounted in memory is a tmpfs. By default,
				// the capacity of a tmpfs is set to half of the physical
				// RAM. So between these two volumes, we can theoretically
				// hog all of the node's RAM, if we want to.
				{
					Name: "memhog-1-volume",
					VolumeSource: api.VolumeSource{
						EmptyDir: &api.EmptyDirVolumeSource{
							Medium: "Memory",
						},
					},
				},
				{
					Name: "memhog-2-volume",
					VolumeSource: api.VolumeSource{
						EmptyDir: &api.EmptyDirVolumeSource{
							Medium: "Memory",
						},
					},
				},
			},
		},
	}
	var err error
	f.MungePodSpec(pod)
	// The generated pod.Name will be on the pod spec returned by Create
	pod, err = f.PodClient().Create(pod)
	framework.ExpectNoError(err, "Error creating Pod")
	framework.ExpectNoError(f.WaitForPodRunning(pod.Name))
	glog.Infof("pod created with name: %s", pod.Name)
	return pod
}
