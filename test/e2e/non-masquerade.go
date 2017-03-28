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

package e2e

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	// . "github.com/onsi/gomega"
)

const (
	imageSource = "gcr.io/kube-play-ext/"

	testPodPort  = 8080
	testPodImage = "non-masquerade-test-amd64"
	testPodTag   = ":0.0.1"

	testProxyPort  = 31235 // Firewall rule allows external traffic on ports 30000-32767. I just picked a random one.
	testProxyImage = "non-masquerade-test-proxy-amd64"
	testProxyTag   = ":0.0.1"
)

var (
	testPod = v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "masq-daemon-test",
			Labels: map[string]string{
				"masq-daemon-test": "",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "masq-daemon-test",
					Image: imageSource + testPodImage + testPodTag,
					Args:  []string{"--port", strconv.Itoa(testPodPort)},
					Env: []v1.EnvVar{
						v1.EnvVar{
							Name:      "POD_IP",
							ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "status.podIP"}},
						},
					},
				},
			},
		},
	}

	testProxyPod = v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "masq-daemon-test-proxy",
		},
		Spec: v1.PodSpec{
			HostNetwork: true,
			Containers: []v1.Container{
				{
					Name:  "masq-daemon-test-proxy",
					Image: imageSource + testProxyImage + testProxyTag,
					Args:  []string{"--port", strconv.Itoa(testProxyPort)},
					Ports: []v1.ContainerPort{
						{
							ContainerPort: testProxyPort,
							HostPort:      testProxyPort,
						},
					},
				},
			},
		},
	}
)

// Produces a pod spec that passes nip as NODE_IP env var using downward API
func newTestPod(nodename string, nip string) *v1.Pod {
	pod := testPod
	node_ip := v1.EnvVar{
		Name:  "NODE_IP",
		Value: nip,
	}
	pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, node_ip)
	pod.Spec.NodeName = nodename
	return &pod
}

func newTestProxyPod(nodename string) *v1.Pod {
	pod := testProxyPod
	pod.Spec.NodeName = nodename
	return &pod
}

func getInternalIP(node *v1.Node) (string, error) {
	for _, addr := range node.Status.Addresses {
		if addr.Type == v1.NodeInternalIP {
			return addr.Address, nil
		}
	}
	return "", fmt.Errorf("did not find InternalIP on Node")
}

func getExternalIP(node *v1.Node) (string, error) {
	for _, addr := range node.Status.Addresses {
		if addr.Type == v1.NodeExternalIP {
			return addr.Address, nil
		}
	}
	return "", fmt.Errorf("did not find ExternalIP on Node")
}

func getSchedulable(nodes []v1.Node) (*v1.Node, error) {
	for _, node := range nodes {
		if node.Spec.Unschedulable == false {
			return &node, nil
		}
	}
	return nil, fmt.Errorf("all Nodes were unschedulable")
}

func checknosnatURL(proxy, pip string, ips []string) string {
	// TODO(mtaufen): Make this over https, since it uses external IP?
	return "http://" + proxy + "/checknosnat?target=" + pip + "&ips=" + strings.Join(ips, ",")
}

// TODO(mtaufen): run this test in a small cluster that has a node with a
// PodCIDR in each of the three ranges defined in RFC 1918

// The masq-daemon configures masquerade settings in iptables.
// This test verifies that packets sent between Pods in the cluster are not
// subject to masquearde SNAT as long as their IPs are in the RFC 1918
// private address space.
var _ = framework.KubeDescribe("Masquerade Daemon [some-cleverly-named-test-label-that-identifies-this-as-needing-its-own-special-snowflake-cluster]", func() {
	f := framework.NewDefaultFramework("masq-daemon-test")
	It("Should be able to send traffic between Pods without SNAT", func() {
		cs := f.ClientSet
		pc := cs.Core().Pods(f.Namespace.Name)
		nc := cs.Core().Nodes()

		By("creating a masq-daemon-test pod for each node")
		nodes, err := nc.List(metav1.ListOptions{})
		framework.ExpectNoError(err)
		if len(nodes.Items) == 0 {
			err = fmt.Errorf("Uh oh, no Nodes in the cluster!")
			framework.ExpectNoError(err)
		}
		for _, node := range nodes.Items {
			// find the Node's internal ip address to feed to the Pod
			// TODO(mtaufen): Since this test will (probably) run in GCP, we can assume there's an internal IP...I think
			// but what about clusters where nodes all have external IPs only?
			inIP, err := getInternalIP(&node)
			framework.ExpectNoError(err)

			// target Pod at Node and feed Pod Node's InternalIP
			pod := newTestPod(node.Name, inIP)
			_, err = pc.Create(pod)
			framework.ExpectNoError(err)
		}

		// In some (most?) scenarios, the test harness doesn't run in the same network as the Pods,
		// which means it can't query Pods using their cluster-internal IPs. To get around this,
		// we create a Pod in a Node's host network, and have that Pod serve on a specific port of that Node.
		// We can then ask this masq-daemon-test-proxy Pod to touch the internal endpoints served by
		// the masq-daemon-test Pods.

		// Find the first schedulable node; masters are marked unschedulable. We don't put the proxy on the master
		// because firewall rules don't allow external traffic to hit ports 30000-32767 on the master, but do
		// allow this on the nodes.
		node, err := getSchedulable(nodes.Items)
		framework.ExpectNoError(err)
		By("creating a masq-daemon-test-proxy Pod on Node " + node.Name + " port " + strconv.Itoa(testProxyPort) +
			" so we can target our masq-daemon-test Pods through that Node's ExternalIP")

		extIP, err := getExternalIP(node)
		framework.ExpectNoError(err)
		proxyNodeIP := extIP + ":" + strconv.Itoa(testProxyPort)

		proxyPod := newTestProxyPod(node.Name)
		_, err = pc.Create(proxyPod)
		framework.ExpectNoError(err)

		By("waiting for all of the masq-daemon-test pods to be scheduled and running")
		err = wait.PollImmediate(10*time.Second, 1*time.Minute, func() (bool, error) {
			pods, err := pc.List(metav1.ListOptions{LabelSelector: "masq-daemon-test"})
			if err != nil {
				return false, err
			}

			// check all pods are running
			for _, pod := range pods.Items {
				if pod.Status.Phase != v1.PodRunning {
					if pod.Status.Phase != v1.PodPending {
						return false, fmt.Errorf("expected pod to be in phase \"Pending\" or \"Running\"")
					}
					return false, nil // pod is still pending
				}
			}
			return true, nil // all pods are running
		})
		framework.ExpectNoError(err)

		By("waiting for the masq-daemon-test-proxy Pod to be scheduled and running")
		err = wait.PollImmediate(10*time.Second, 1*time.Minute, func() (bool, error) {
			pod, err := pc.Get("masq-daemon-test-proxy", metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if pod.Status.Phase != v1.PodRunning {
				if pod.Status.Phase != v1.PodPending {
					return false, fmt.Errorf("expected pod to be in phase \"Pending\" or \"Running\"")
				}
				return false, nil // pod is still pending
			}
			return true, nil // pod is running
		})
		framework.ExpectNoError(err)

		By("sending traffic from each pod to the others and checking that SNAT does not occur")
		pods, err := pc.List(metav1.ListOptions{LabelSelector: "masq-daemon-test"})
		framework.ExpectNoError(err)

		// collect pod IPs, sort for quick find/remove of any given address
		podIPs := []string{}
		for _, pod := range pods.Items {
			ip := pod.Status.PodIP
			podIPs = append(podIPs, ip+":"+strconv.Itoa(testPodPort))
		}

		// hit the /checknosnat endpoint on each Pod, tell each Pod to check all the other Pods
		// this alg is O(n^2) but it doesn't matter because we only run this test on clusters with ~3 nodes
		errs := []string{}
		client := http.Client{
			Timeout: time.Duration(0), //5 * time.Minute,
		}
		for _, pip := range podIPs {
			ips := []string{}
			for _, ip := range podIPs {
				if ip == pip {
					continue
				}
				ips = append(ips, ip)
			}
			// hit /checknosnat on pip, via proxy
			resp, err := client.Get(checknosnatURL(proxyNodeIP, pip, ips))
			framework.ExpectNoError(err)

			// check error code on the response, if 500 record the body, which will describe the error
			if resp.StatusCode == 500 {
				body, err := ioutil.ReadAll(resp.Body)
				framework.ExpectNoError(err)
				errs = append(errs, string(body))
			}
			resp.Body.Close()
		}

		// report the errors all at the end
		if len(errs) > 0 {
			str := strings.Join(errs, "\n")
			err := fmt.Errorf("/checknosnat failed in the following cases:\n%s", str)
			framework.ExpectNoError(err)
		}
	})
})
