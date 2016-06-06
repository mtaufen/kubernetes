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
	"k8s.io/kubernetes/pkg/api/errors"
	"k8s.io/kubernetes/pkg/api/resource"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/util/wait"
	"k8s.io/kubernetes/pkg/watch"
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

	// TODO: Make sure all these pods run on the same node!

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
				Expect(err).To(BeNil(), fmt.Sprintf("Error creating guaranteed Pod %v", err))
				framework.ExpectNoError(f.WaitForPodRunning(guaranteedPod.Name))
			}

			// A pod is burstable if limits and requests do not match across all containers.
			burstablePod := &api.Pod{
				ObjectMeta: api.ObjectMeta{
					GenerateName: "burstable-",
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
				framework.ExpectNoError(f.WaitForPodRunning(burstablePod.Name))
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
				framework.ExpectNoError(f.WaitForPodRunning(besteffortPod.Name))
			}

			// TOOD: Watch for eviction on the node (watch status of each pod and detect failed status,
			// then depending on the pod, query status of other two pods, and check that against
			// failure modes described below).

			// Watch is a method on PodInterface i.e. our podClient
			// this returns a watch.Interface and I think you have to pass
			// that to something else to use it.

			// gw, gwErr := podClient.Watch(api.SingleObject(api.ObjectMeta{Name: guaranteedPod.ObjectMeta.Name}))
			glog.Info("made the pods, about to wait for eviction")
			//event, err := watch.Until(5*time.Minute, gw, podFailed)

			// glog.Infof("%v %v %v %v", gw, gwErr, event, err)

			// Eventually polls, so you can do e.g.
			// Eventually(func string {...}, ...).Should(Equal("stuff"))
			// So I suppose we should wait for them all to be killed?
			// and if they aren't all dead, it keeps polling, and we'll make the
			// test fail from inside the eventually. How do we force failure from inside
			// the eventually? panic? or can we use Should?

			// We poll until timeout or all pods are dead
			// The func returns true when all pods are dead,
			// false otherwise.
			// Inside the func, we check that no failure modes are present
			// for the combined pod statuses.
			Eventually(func() bool {

				gteed, gtErr := f.Client.Pods(f.Namespace.Name).Get(guaranteedPod.Name)
				framework.ExpectNoError(gtErr, fmt.Sprintf("getting pod %s", guaranteedPod.Name))
				gteedPh := gteed.Status.Phase

				burst, buErr := f.Client.Pods(f.Namespace.Name).Get(burstablePod.Name)
				framework.ExpectNoError(buErr, fmt.Sprintf("getting pod %s", burstablePod.Name))
				burstPh := burst.Status.Phase

				best, beErr := f.Client.Pods(f.Namespace.Name).Get(besteffortPod.Name)
				framework.ExpectNoError(beErr, fmt.Sprintf("getting pod %s", besteffortPod.Name))
				bestPh := best.Status.Phase

				// Check failure modes:
				/*

					Test ends when everything is dead

					We watch for something to die

					When something dies, we get the status of the pods

					Each failure mode is a violation of the order in which
					the pods should be evicted.

					The following are the possible test failure modes:

					101
					011
					010
					001

					besteffort: alive
					burstable:  dead
					guaranteed: alive

					besteffort: alive
					burstable:  alive
					guaranteed: dead

					besteffort: dead
					burstable:  alive
					guaranteed: dead

					besteffort: alive
					burstable:  dead
					guaranteed: dead

				*/

				if bestPh == api.PodRunning {
					// TODO: Need to check "not failed" rather than "running" in case it's pending
					Expect(burstPh).NotTo(Equal(api.PodFailed), "Burstable pod failed before best effort pod")
					Expect(gteedPh).NotTo(Equal(api.PodFailed), "Guaranteed pod failed before best effort pod")
				} else if burstPh == api.PodRunning {
					Expect(gteedPh).NotTo(Equal(api.PodFailed), "Guaranteed pod failed before burstable pod")
				}

				// Otherwise pods are in a valid state wrt memory eviction,
				// or some are still pending, or some are in a weird state that
				// I'm not testing for yet.

				// When all the pods are evicted, return true, else false
				if bestPh == api.PodFailed && burstPh == api.PodFailed && gteedPh == api.PodFailed {
					return true
				}
				return false

			}, 5*time.Minute, 4*time.Second).Should(Equal(true))

			glog.Info("done waiting for eviction")
			// Eventually(func() )

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


			*/

		})
	})

	Context("When there is disk pressure", func() {

		It("should evict pods in the correct order", func() {

		})

	})

})

func pollEvictionOrder(best string, burst, string, gteed string) {

}

// Condition func TODO: See if anyone's implemented this somewhere else so I can reuse
// might want to check kubernetes/pkg/client/unversioned/conditions.go for examples
func podFailed(event watch.Event) (bool, error) {
	glog.Infof("event: %v", event)
	switch event.Type {
	case watch.Deleted:
		return false, errors.NewNotFound(unversioned.GroupResource{Resource: "pods"}, "")
	}
	switch t := event.Object.(type) {
	case *api.Pod:
		switch t.Status.Phase {
		case api.PodFailed:
			return true, nil
		default:
			glog.Infof("phase: %v", t.Status.Phase)
		}
	}
	return false, nil
}

// TODO: There is also a WaitForPodTerminated function that could be useful
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
