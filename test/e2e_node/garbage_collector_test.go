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
	// TODO: I just switched from fsouza/go-dockerclient to docker/engine-api/client
	//       So we may need to fiddle with things to get them to work again
	dockerapi "github.com/docker/engine-api/client"
	dockertypes "github.com/docker/engine-api/types"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"k8s.io/kubernetes/pkg/api"
	// "k8s.io/kubernetes/pkg/client/restclient"
	// client "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/util/wait"
	"k8s.io/kubernetes/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// TODO: Make sure this test runs in serial before we
//       submit a PR and run it with the whole suite of e2e tests

var _ = framework.KubeDescribe("GarbageCollect", func() {
	var dockerClient *dockerapi.Client

	// BeforeEach blocks closer to the root of the Describe/Context/It tree run first
	BeforeEach(func() {
		var err error
		dockerClient, err = dockerapi.NewEnvClient()
		Expect(err).To(BeNil(), fmt.Sprintf("Error connecting to docker %v", err))
	})

	Context("when pods are deleted", func() {
		f := NewDefaultFramework("gc-deleted-pod-containers")

		It("it should garbage collect containers for deleted pods", func() {
			Skip("Requires docker permissions") // FIXME TODO: I think this was a Jenkins thing?

			// TODO: Change back to e = 5 and num = 90
			const (
				// The acceptable delta when counting containers.
				epsilon = 0
				// The number of pods to create & delete.
				numPods = 15
			)

			// Get an initial container count
			// We compare the number of running containers to this initial count to determine
			// whether the containers for our pods are running or not. This is one reason the
			// test needs to run in serial with other tests.
			containers, err := dockerClient.ContainerList(context.Background(), dockertypes.ContainerListOptions{All: true})
			Expect(err).To(BeNil(), fmt.Sprintf("Error listing containers %v", err))
			initialContainerCount := len(containers)

			// Start pods
			By("Creating the pods")
			gc_podNames := make([]string, numPods)
			gc_podContainers := []api.Container{getPauseContainer()}
			for i := 0; i < numPods; i++ {
				gc_podNames[i] = fmt.Sprintf("gc-container-pod-%d", i)
				createPod(f, gc_podNames[i], gc_podContainers, nil)
			}

			// Wait for containers to start
			By("Waiting for the containers to start")
			Expect(waitForContainerCount(dockerClient, atLeast(initialContainerCount+numPods))).To(BeNil())

			// Delete pods.
			By("Deleting the pods")
			podClient := f.Client.Pods(f.Namespace.Name)
			for _, podName := range gc_podNames {
				err := podClient.Delete(podName, &api.DeleteOptions{})
				Expect(err).To(BeNil(), fmt.Sprintf("Error deleting Pod %q: %v", podName, err))
			}

			// Wait for containers to be garbage collected.
			By("Waiting for the containers to be garbage collected")
			Expect(waitForContainerCount(dockerClient, atMost(initialContainerCount+epsilon))).To(BeNil())
		})
	})

	Context("when pods with a restart policy die and are restarted", func() {
		f := NewDefaultFramework("gc-dead-containers")

		It("it should garbage collect the dead containers", func() {
			Skip("Just skip this one for now") // TODO: remove this skip

			// TODO: Change back to num = 90
			const (
				// The number of pods to create & delete.
				numPods = 20
			)

			// Now we'll create some pods with a restart policy that crash, and we'll try to make sure
			// that the container count does not monotonically increase (e.g. old, crashed containers get cleaned up)
			// (the restarted pods will be running new containers)

			By("Creating the Every Villain Is Lemons pods") // TODO: Potentially rename evil pods -> death pods
			evil_podNames := make([]string, numPods)
			for i := 0; i < numPods; i++ {
				// TODO: Make these pods crash when they start (run false as a command?)
				// TODO: Any way to test if these crash?
				evil_podNames[i] = fmt.Sprintf("evil-pod-%d", i)
				evil_podContainers := []api.Container{getBusyboxDeathContainer()}
				createUndeadPod(f, evil_podNames[i], evil_podContainers, nil) // There is only one container in podContainers, so there will be one container per pod
			}

			// Now we'll watch the container count and wait until we see it dip.
			// We are trying to detect that old, dead containers get cleaned up.
			// Every time a pod restarts, it does so in a new container, and the old,
			// crashed container stays around until it is garbage collected.
			By("Waiting for the container count to dip. This indicates container GC is happening.")
			Expect(waitForContainerCountDip(dockerClient)).To(BeNil())

			// TODO: need to lower the thresholds for garbage collection in the kubelet
			// this probably happens when the kubelet server is created

			// See this doc, useful: https://github.com/kubernetes/kubernetes/blob/b9cfab87e33ea649bdd13a1bd243c502d76e5d22/docs/admin/garbage-collection.md#L83

			// These thresholds seem to be parameters on the Kubelet server? See func NewKubeletServer in
			// cmd/kubelet/app/options/options.go and also componentconfig.KubeletConfiguration
			// in pkg/apis/componentconfig/types.go

			// TODO: Where does the Kubelet configuration live for end to end tests?
			//       Seems to be in kubernetes/test/e2e_node/e2e_service.go:startKubeletServer
		})

	})

	Context("during high disk pressure", func() {
		f := NewDefaultFramework("gc-images-disk-pressure")
		// TODO:
		It("it should garbage collect images to free up space", func() {

			// Now we'll induce some disk pressure to see if the sample images get cleaned up
			// TODO: I wonder how this will play out on my workstation... maybe I can trick the pod into cleaning up?
			//       What actually does the cleanup? Is it the pod? Some daemon?
			//By("Creating a pod that will induce disk pressure")

			// The sample image is ________________
			// TODO: How to induce disk pressure
			// TODO: How to lower the threshold for image garbage collection (again, this is a)
			//       parameter on the kubelet:
			//         - image-gc-high-threshold: percent of disk usage which triggers image gc
			///          default 90% (Vish thinks we can lower this to 50% for the test)
			//         - image-gc-low-threshold: target percent of disk usage for gc to achieve once triggered
			//           default 50%

			/*
				kubernetes manages lifecycle of all images through imageManager, with the cooperation of cadvisor.
				The policy for garbage collecting images we apply takes two factors into consideration, HighThresholdPercent
				and LowThresholdPercent. Disk usage above the the high threshold will trigger garbage collection, which attempts
				to delete unused images until the low threshold is met. Least recently used images are deleted first.
			*/

			// TODO: Could probably just create one pod with several containers if
			//       I just want those images to be pulled.
			By("Creating and then deleting a pod, so that images for an old pod exist")
			podContainers := []api.Container{getPauseContainer()} // TODO: use several containers with big images
			// for i := 0; i < numPods; i++ {
			name := fmt.Sprintf("gc-img-pod-%d", 0)
			createPod(f, name, podContainers, nil)
			framework.ExpectNoError(f.WaitForPodRunning(name))
			err := f.Client.Pods(f.Namespace.Name).Delete(name, &api.DeleteOptions{})
			Expect(err).To(BeNil(), fmt.Sprintf("Error deleting Pod %q: %v", name, err))
			// }

			// TODO: It might turn out that we need to wait for the containers to start running before we can delete the pod.

			By("Starting a new pod which we will use to create disk pressure")
			createPod(f, "gc-pressure-pod-0",
				[]api.Container{
					api.Container{
						Name:    "busybox",
						Image:   "gcr.io/google_containers/busybox:1.24",
						Command: []string{"/bin/sh", "-c", "stat /firehose/ > /firehose/statfh && du > /firehose/du && df -h > /firehose/dfh"},
						//Command: []string{"stat", "/firehose/", ">", "/firehose/teststat"}, // TODO: Change this to something that writes to the volume
						// could do something like: touch foo && dd if=/dev/zero of=foo bs=500M count=2 (on the volume!)
						// bs is block size and count is number of blocks
						VolumeMounts: []api.VolumeMount{
							{
								Name:      "disk-pressure-volume", // TODO
								MountPath: "/firehose",            // TOOD
							},
						},
					}},
				[]api.Volume{
					{
						Name: "disk-pressure-volume",
						VolumeSource: api.VolumeSource{
							HostPath: &api.HostPathVolumeSource{
								Path: "/tmp/firehose", // TODO: For now, just doing this
							},
						}, // I want a volume with a disk size. How do I put a disk usage restriction on the entire pod? Idk if this is a feature yet. Buddha is working on podlevel resource restrictions.
					},
				})

			By("Bringing a volume into the new pod")
			// TODO: Bring in a volume
			// Good example of how to do this:
			// test/e2e/volumes.go:221

			By("Waiting for the image count to dip. This indicates image GC is happening.")
			Expect(waitForImageCountDip(dockerClient)).To(BeNil())

		})
	})
})

type condition struct {
	desc string
	test func(int) bool
}

func atMost(val int) condition {
	return condition{
		desc: fmt.Sprintf("at most %d", val),
		test: func(x int) bool { return x <= val },
	}
}

func atLeast(val int) condition {
	return condition{
		desc: fmt.Sprintf("at least %d", val),
		test: func(x int) bool { return x >= val },
	}
}

// Wait for at least count containers to be running if atleast is true, or at most count containers
// to be running if atleast is false.
func waitForContainerCount(dockerClient *dockerapi.Client, cond condition) error {
	const (
		pollPeriod  = 10 * time.Second
		pollTimeout = 5 * time.Minute
	)
	var count int
	err := wait.PollImmediate(pollPeriod, pollTimeout, func() (bool, error) {
		containers, err := dockerClient.ContainerList(context.Background(), dockertypes.ContainerListOptions{All: true})
		if err != nil {
			glog.Errorf("Error listing containers: %v", err)
			return false, nil // TODO: Is it right to return nil here?
		}
		count = len(containers)
		if cond.test(count) {
			return true, nil
		}
		glog.Infof("Waiting for %s containers, currently %d", cond.desc, count)
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for %s containers: found %d", cond.desc, count)
	}
	glog.Infof("Finished waiting for %s containers: currently %d", cond.desc, count)
	return nil
}

func waitForContainerCountDip(dockerClient *dockerapi.Client) error {
	const (
		pollPeriod  = 3 * time.Second // TODO: Change back to every 10 seconds?
		pollTimeout = 5 * time.Minute
	)
	var oldCount int
	var count int
	err := wait.PollImmediate(pollPeriod, pollTimeout, func() (bool, error) {
		containers, err := dockerClient.ContainerList(context.Background(), dockertypes.ContainerListOptions{All: true})
		if err != nil {
			glog.Errorf("Error listing containers: %v", err)
			return false, nil // TODO: Is it right to return nil here?
		}
		count = len(containers)
		if count >= oldCount {
			oldCount = count
			glog.Infof("Waiting for container count to dip, currently %d", count)
			return false, nil
		}
		glog.Infof("Saw dip in container count, currently %d", count)
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for a dip in the container count")
	}
	return nil
}

func waitForImageCountDip(dockerClient *dockerapi.Client) error {
	const (
		pollPeriod  = 3 * time.Second // TODO: Change to every 10 sec?
		pollTimeout = 5 * time.Minute
	)
	var oldCount int
	var count int
	err := wait.PollImmediate(pollPeriod, pollTimeout, func() (bool, error) {
		images, err := dockerClient.ImageList(context.Background(), dockertypes.ImageListOptions{All: true})
		if err != nil {
			glog.Errorf("Error listing images: %v", err)
			return false, nil
		}
		count = len(images)
		if count >= oldCount {
			oldCount = count
			glog.Infof("Waiting for image count to dip, currently %d", count)
			return false, nil
		}
		glog.Infof("Saw dip in image count, currently %d", count)
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("timed out waiting for a dip in the image count")
	}
	return nil
}

// Does not WaitForPodRunning. The decision to do so is now up to the caller of createPod.
func createPod(f *framework.Framework, podName string, containers []api.Container, volumes []api.Volume) {
	podClient := f.Client.Pods(f.Namespace.Name)
	pod := &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Name: podName,
		},
		Spec: api.PodSpec{
			// Force the Pod to schedule to the node without a scheduler running
			NodeName: *nodeName,
			// Don't restart the Pod since it is expected to exit
			RestartPolicy: api.RestartPolicyNever,
			Containers:    containers,
			Volumes:       volumes,
		},
	}
	_, err := podClient.Create(pod)
	Expect(err).To(BeNil(), fmt.Sprintf("Error creating Pod %v", err))
}

// Same as createPod, except this one has a restart policy. It's undead!
func createUndeadPod(f *framework.Framework, podName string, containers []api.Container, volumes []api.Volume) {
	podClient := f.Client.Pods(f.Namespace.Name)
	pod := &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Name: podName,
		},
		Spec: api.PodSpec{
			// Force the Pod to schedule to the node without a scheduler running
			NodeName: *nodeName,
			// Restart the pod. Always! Is undead. RestartPolicy[Always | OnFailure | Never]
			RestartPolicy: api.RestartPolicyAlways,
			Containers:    containers,
			Volumes:       volumes,
		},
	}
	_, err := podClient.Create(pod)
	Expect(err).To(BeNil(), fmt.Sprintf("Error creating Pod %v", err))
}

func getPauseContainer() api.Container {
	return api.Container{
		Name:  "pause",
		Image: "gcr.io/google_containers/pause:2.0",
	}
}

func getBusyboxDeathContainer() api.Container {
	return api.Container{
		Name:  "busybox",
		Image: "gcr.io/google_containers/busybox:1.24",
		// Run `false` so the status is Exited (1)
		Command: []string{"false"},
	}
}
