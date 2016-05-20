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

	docker "github.com/fsouza/go-dockerclient"
	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/restclient"
	client "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/util/wait"
	"k8s.io/kubernetes/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// TODO: Make sure this test runs in serial before we
//       submit a PR and run it with the whole suite of e2e tests

var _ = Describe("GarbageCollect", func() {
	var cl *client.Client
	var fr *framework.Framework
	// fr = NewDefaultFramework("garbage-collection")
	var dockerClient *docker.Client

	// Note: apiServerAddress is in test/e2e_node/util.go
	//       and at this time is 127.0.0.1:8080
	cl = client.NewOrDie(&restclient.Config{Host: *apiServerAddress})

	// I'm starting with a new framework and client for each test.
	// Normally if you don't put a client on a framework it makes one for you.
	// But does it make the right kind of client?
	// The framework will make a namespace for you as well. Not quite sure what
	// doing that entails yet. But I'm pretty sure it happens.

	fr = framework.NewFramework("garbage-collection-test",
		framework.FrameworkOptions{ // same options that are given to the default framework
			ClientQPS:   20,
			ClientBurst: 50,
		},
		cl)

	BeforeEach(func() { // TODO: How does this play with the BeforeEach that's built into framework?
		var err error
		dockerClient, err = docker.NewClientFromEnv()
		Expect(err).To(BeNil(), fmt.Sprintf("Error connecting to docker %v", err))
	})

	// It("Should garbage collect containers for deleted pods", func() {
	// 	// Skip("Requires docker permissions") // FIXME TODO: I think this was a Jenkins thing?

	// 	// TODO: Change back to e = 5 and num = 90
	// 	const (
	// 		// The acceptable delta when counting containers.
	// 		epsilon = 0
	// 		// The number of pods to create & delete.
	// 		numPods = 15
	// 	)

	// 	// Get an initial container count
	// 	// We compare the number of running containers to this initial count to determine
	// 	// whether the containers for our pods are running or not. This is one reason the
	// 	// test needs to run in serial with other tests.
	// 	containers, err := dockerClient.ListContainers(docker.ListContainersOptions{All: true})
	// 	Expect(err).To(BeNil(), fmt.Sprintf("Error listing containers %v", err))
	// 	initialContainerCount := len(containers)

	// 	// Start pods
	// 	By("Creating the pods")
	// 	gc_podNames := make([]string, numPods)
	// 	gc_podContainers := []api.Container{getPauseContainer()} // TODO: Modify this to use containers based off the sample images
	// 	for i := 0; i < numPods; i++ {
	// 		gc_podNames[i] = fmt.Sprintf("gc-pod-%d", i)
	// 		createPod(fr, gc_podNames[i], gc_podContainers, nil) // There is only one container in podContainers, so there will be one container per pod
	// 	}

	// 	// Wait for containers to start
	// 	By("Waiting for the containers to start")
	// 	Expect(waitForContainerCount(dockerClient, atLeast(initialContainerCount+numPods))).To(BeNil())

	// 	// Delete pods.
	// 	By("Deleting the pods")
	// 	podClient := fr.Client.Pods(fr.Namespace.Name)
	// 	for _, podName := range gc_podNames {
	// 		err := podClient.Delete(podName, &api.DeleteOptions{})
	// 		Expect(err).To(BeNil(), fmt.Sprintf("Error deleting Pod %q: %v", podName, err))
	// 	}

	// 	// Wait for containers to be garbage collected.
	// 	By("Waiting for the containers to be garbage collected") // TODO: What actually does the garbage collection?
	// 	Expect(waitForContainerCount(dockerClient, atMost(initialContainerCount+epsilon))).To(BeNil())

	// })

	// This is failing... why?
	// The error string is "pod ran to completion"
	It("Should garbage collect dead containers", func() {

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
			createUndeadPod(fr, evil_podNames[i], evil_podContainers, nil) // There is only one container in podContainers, so there will be one container per pod
		}

		// Now we'll watch the container count and wait until we see it dip
		// We are trying to detect that old, dead containers get cleaned up
		// (Every time a pod restarts, it does so in a new container, and the old,
		//  crashed container stays around until it is garbage collected)

		Expect(waitForContainerCountDip(dockerClient)).To(BeNil())

		// TODO: need to lower the thresholds for garbage collection in the kubelet

		// See this doc, useful: https://github.com/kubernetes/kubernetes/blob/b9cfab87e33ea649bdd13a1bd243c502d76e5d22/docs/admin/garbage-collection.md#L83

		// These thresholds seem to be parameters on the Kubelet server? See func NewKubeletServer in
		// cmd/kubelet/app/options/options.go and also componentconfig.KubeletConfiguration
		// in pkg/apis/componentconfig/types.go

		// TODO: Where does the Kubelet configuration live for end to end tests?

		// Now we'll induce some disk pressure to see if the sample images get cleaned up
		// TODO: I wonder how this will play out on my workstation... maybe I can trick the pod into cleaning up?
		//       What actually does the cleanup? Is it the pod? Some daemon?
		//By("Creating a pod that will induce disk pressure")
	})

	// TODO:
	// It("Should garbage collect images during high disk pressure", func() {})
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
func waitForContainerCount(dockerClient *docker.Client, cond condition) error {
	const (
		pollPeriod  = 10 * time.Second
		pollTimeout = 5 * time.Minute
	)
	var count int
	err := wait.PollImmediate(pollPeriod, pollTimeout, func() (bool, error) {
		containers, err := dockerClient.ListContainers(docker.ListContainersOptions{All: true})
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

func waitForContainerCountDip(dockerClient *docker.Client) error {
	const (
		pollPeriod  = 10 * time.Second
		pollTimeout = 5 * time.Minute
	)
	var oldCount int
	var count int
	err := wait.PollImmediate(pollPeriod, pollTimeout, func() (bool, error) {
		containers, err := dockerClient.ListContainers(docker.ListContainersOptions{All: true})
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
