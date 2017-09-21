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

package e2e_node

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = framework.KubeDescribe("Standalone Kubelet Pod Manifest [standalone]", func() {
	const (
		timeout  = time.Minute * 3
		interval = time.Second
		podName  = "standalone-kubelet-pod-manifest-test"
	)
	Context("when a pod manifest is created in the pod manifest directory", func() {
		// TODO(mtaufen): define an SLO for the time it takes the pod to start
		It("the pod should eventually start running", func() {
			// create the manifest file
			err := createPodManifestFile(&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: podName,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Image:   busyboxImage,
							Name:    podName,
							Command: []string{"sh", "-c", "while true; do echo 'Hello World' ; sleep 10 ; done"},
						}}}})
			framework.ExpectNoError(err)

			// check that the pod is running
			Eventually(func() error {
				return checkStaticPodStarted(podName)
			}, timeout, interval).Should(BeNil())

			// TODO(mtaufen): delete the manifest and see that the pod stops running
			// TODO(mtaufen): clean up the manifest file after the test, if it is still there
		})
	})
})
