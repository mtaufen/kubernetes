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
	"strconv"
	"time"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// PLEASE NOTE: Most of this stuff cloned from test/e2e/volumes.go

// Configuration of one tests. The test consist of:
// - server pod - runs serverImage, exports ports[]
// - client pod - does not need any special configuration
type VolumeTestConfig struct {
	// Prefix of all pods. Typically the test name.
	prefix string
	// Name of container image for the server pod.
	serverImage string
	// Ports to export from the server pod. TCP only.
	serverPorts []int
	// Arguments to pass to the container image.
	serverArgs []string
	// Volumes needed to be mounted to the server container from the host
	// map <host (source) path> -> <container (dst.) path>
	volumes map[string]string
}

// Starts a container specified by config.serverImage and exports all
// config.serverPorts from it. The returned pod should be used to get the server
// IP address and create appropriate VolumeSource.
func startVolumeServer(f *framework.Framework, config VolumeTestConfig, exportText string) *api.Pod {
	podClient := f.PodClient()

	portCount := len(config.serverPorts)
	serverPodPorts := make([]api.ContainerPort, portCount)

	for i := 0; i < portCount; i++ {
		portName := fmt.Sprintf("%s-%d", config.prefix, i)

		serverPodPorts[i] = api.ContainerPort{
			Name:          portName,
			ContainerPort: int32(config.serverPorts[i]),
			Protocol:      api.ProtocolTCP,
		}
	}

	volumeCount := len(config.volumes)
	volumes := make([]api.Volume, volumeCount)
	mounts := make([]api.VolumeMount, volumeCount)

	i := 0
	for src, dst := range config.volumes {
		mountName := fmt.Sprintf("path%d", i)
		volumes[i].Name = mountName
		volumes[i].VolumeSource.HostPath = &api.HostPathVolumeSource{
			Path: src,
		}

		mounts[i].Name = mountName
		mounts[i].ReadOnly = false
		mounts[i].MountPath = dst

		i++
	}

	By(fmt.Sprint("creating ", config.prefix, " server pod"))
	privileged := new(bool)
	*privileged = true
	serverPod := &api.Pod{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: api.ObjectMeta{
			Name: config.prefix + "-server",
			Labels: map[string]string{
				"role": config.prefix + "-server",
			},
		},

		Spec: api.PodSpec{
			Containers: []api.Container{
				{
					Name:  config.prefix + "-server",
					Image: config.serverImage,
					SecurityContext: &api.SecurityContext{
						Privileged: privileged,
					},
					Args:         config.serverArgs,
					Ports:        serverPodPorts,
					VolumeMounts: mounts,
				},
			},
			Volumes: volumes,
		},
	}
	serverPod = podClient.CreateSync(serverPod)

	By("locating the server pod")
	pod, err := podClient.Get(serverPod.Name)
	framework.ExpectNoError(err, "Cannot locate the server pod %v: %v", serverPod.Name, err)

	By("sleeping a bit to give the server time to start")
	time.Sleep(20 * time.Second)

	By("server pod is probably running, inject some text into /export/index.html if provided")
	if exportText != "" {
		_, stderr, err := f.ExecCommandInPodWithFullOutput(serverPod.Name, fmt.Sprintf("echo \"%s\" > /export/index.html"))
		if err != nil {
			framework.Logf("framework.ExecCommandInPodWithFullOutput stderr: %q", stderr)
		}
	}
	return pod
}

// Clean both server and client pods.
func volumeTestCleanup(f *framework.Framework, config VolumeTestConfig) {
	By(fmt.Sprint("cleaning the environment after ", config.prefix))

	defer GinkgoRecover()

	podClient := f.PodClient()

	podClient.DeleteSync(config.prefix+"-client", nil, 5*time.Minute)

	if config.serverImage != "" {
		// See issue #24100.
		// Prevent umount errors by making sure making sure the client pod exits cleanly *before* the volume server pod exits.
		By("sleeping a bit so client can stop and unmount")
		time.Sleep(20 * time.Second)

		podClient.DeleteSync(config.prefix+"-server", nil, 5*time.Minute)
	}
}

// Start a client pod using given VolumeSource (exported by startVolumeServer())
// and check that the pod sees the data from the server pod.
func testVolumeClient(f *framework.Framework, config VolumeTestConfig, volume api.VolumeSource, fsGroup *int64, expectedContent string) {
	By(fmt.Sprint("starting ", config.prefix, " client"))
	clientPod := &api.Pod{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: api.ObjectMeta{
			Name: config.prefix + "-client",
			Labels: map[string]string{
				"role": config.prefix + "-client",
			},
		},
		Spec: api.PodSpec{
			Containers: []api.Container{
				{
					Name:       config.prefix + "-client",
					Image:      "gcr.io/google_containers/busybox:1.24",
					WorkingDir: "/opt",
					// An imperative and easily debuggable container which reads vol contents for
					// us to scan in the tests or by eye.
					// We expect that /opt is empty in the minimal containers which we use in this test.
					Command: []string{
						"/bin/sh",
						"-c",
						"while true ; do cat /opt/index.html ; sleep 2 ; ls -altrh /opt/  ; sleep 2 ; done ",
					},
					VolumeMounts: []api.VolumeMount{
						{
							Name:      config.prefix + "-volume",
							MountPath: "/opt/",
						},
					},
				},
			},
			SecurityContext: &api.PodSecurityContext{
				SELinuxOptions: &api.SELinuxOptions{
					Level: "s0:c0,c1",
				},
			},
			Volumes: []api.Volume{
				{
					Name:         config.prefix + "-volume",
					VolumeSource: volume,
				},
			},
		},
	}
	podClient := f.PodClient()

	if fsGroup != nil {
		clientPod.Spec.SecurityContext.FSGroup = fsGroup
	}
	clientPod = podClient.CreateSync(clientPod)

	By("Checking that text file contents are perfect.")
	_, err := framework.LookForString(expectedContent, time.Minute, func() string {
		// DEBUG HACK:
		stdout, stderr, err := f.ExecCommandInPodWithFullOutput(clientPod.Name, "ls /opt/")
		framework.Logf("framework.ExecCommandInPodWithFullOutput stdout: %q", stdout)
		framework.Logf("framework.ExecCommandInPodWithFullOutput stderr: %q", stderr)
		// What we actually want
		stdout, stderr, err = f.ExecCommandInPodWithFullOutput(clientPod.Name, "cat /opt/index.html")
		if err != nil {
			framework.Logf("framework.ExecCommandInPodWithFullOutput stderr: %q", stderr)
		}
		return stdout
	})
	Expect(err).NotTo(HaveOccurred(), "failed: finding the contents of the mounted file.")

	// TODO(mtaufen): Not sure if this works yet.
	if fsGroup != nil {
		By("Checking fsGroup is correct.")
		_, err := framework.LookForString(strconv.Itoa(int(*fsGroup)), time.Minute, func() string {
			stdout, stderr, err := f.ExecCommandInPodWithFullOutput(clientPod.Name, "ls -ld /tmp")
			if err != nil {
				framework.Logf("framework.ExecCommandInPodWithFullOutput stderr: %q", stderr)
			}
			return stdout
		})
		Expect(err).NotTo(HaveOccurred(), "failed: getting the right priviliges in the file %v", int(*fsGroup))
	}
}

// Don't label this [Feature:Volumes] because we want it to actually run in the test job.
var _ = framework.KubeDescribe("NodeVolumes", func() {
	f := framework.NewDefaultFramework("node-volumes-test")

	// If 'false', the test won't clear its volumes upon completion. Useful for debugging,
	// note that namespace deletion is handled by delete-namespace flag
	clean := true

	////////////////////////////////////////////////////////////////////////
	// NFS
	////////////////////////////////////////////////////////////////////////

	framework.KubeDescribe("NFS", func() {
		It("should be mountable", func() {
			config := VolumeTestConfig{
				prefix:      "nfs",
				serverImage: "eatnumber1/nfs",
				serverPorts: []int{2049, 111, 1067, 1066},
			}

			defer func() {
				if clean {
					volumeTestCleanup(f, config)
				}
			}()
			pod := startVolumeServer(f, config, "Hello from NFS!")
			serverIP := pod.Status.PodIP
			framework.Logf("NFS server IP address: %v", serverIP)

			volume := api.VolumeSource{
				NFS: &api.NFSVolumeSource{
					Server:   serverIP,
					Path:     "/export", // TODO: Should this be /export with eatnumber1's image?
					ReadOnly: true,
				},
			}
			// Must match content of test/images/volumes-tester/nfs/index.html
			testVolumeClient(f, config, volume, nil, "Hello from NFS!")
		})
	})

	////////////////////////////////////////////////////////////////////////
	// Gluster
	////////////////////////////////////////////////////////////////////////

	framework.KubeDescribe("GlusterFS", func() {
		PIt("should be mountable", func() {
			config := VolumeTestConfig{
				prefix:      "gluster",
				serverImage: "gcr.io/google_containers/volume-gluster:0.2",
				serverPorts: []int{24007, 24008, 49152},
			}

			defer func() {
				if clean {
					volumeTestCleanup(f, config)
				}
			}()

			// This is the right way to do it, but today is a day of breaking the rules (10:48am)
			pod := startVolumeServer(f, config, "")
			serverIP := pod.Status.PodIP
			framework.Logf("Gluster server IP address: %v", serverIP)

			// TODO: Make sure gluster is installed on the node first, error if not

			// Hack: just start the container by hand with the ports published and 1:1 mapped to the host:
			// runCmd := exec.Command("sudo", "docker", "run", "-d", "--privileged",
			// 	"-p", "24007:24007",
			// 	"-p", "24008:24008",
			// 	"-p", "49152:49152",
			// 	"gcr.io/google_containers/volume-gluster:0.2")
			// err := runCmd.Run()
			// if err != nil {
			// 	framework.Logf("Error running docker container %+v", runCmd)
			// 	framework.ExpectNoError(err)
			// }

			// Hack: just try to mount it and see if it succeeds
			//       this won't work on GCI because the root filesystem is read only
			// mkdirCmd := exec.Command("sudo", "mkdir", "/my_mnt")
			// err = mkdirCmd.Run()
			// Hack: dont worry about the error just assume its there, so we don't have to restart node across test runs
			// if err != nil {
			// 	framework.Logf("Error running mkdir %+v", mkdirCmd)
			// 	framework.ExpectNoError(err)
			// }
			// remember to umount this when done
			// mountCmd := exec.Command("sudo", "mount", "-t", "glusterfs", "127.0.0.1:test_vol", "/my_mnt")
			// err = mountCmd.Run()
			// if err != nil {
			// 	framework.Logf("Error running mount %+v", mountCmd)
			// 	framework.ExpectNoError(err)
			// }

			// create Endpoints for the server
			endpoints := api.Endpoints{
				TypeMeta: unversioned.TypeMeta{
					Kind:       "Endpoints",
					APIVersion: "v1",
				},
				ObjectMeta: api.ObjectMeta{
					Name: config.prefix + "-server",
				},
				Subsets: []api.EndpointSubset{
					{
						Addresses: []api.EndpointAddress{
							{
								IP: serverIP,
							},
						},
						Ports: []api.EndpointPort{
							{
								Name:     "gluster",
								Port:     24007,
								Protocol: api.ProtocolTCP,
							},
						},
					},
				},
			}

			endClient := f.Client.Endpoints(f.Namespace.Name)

			defer func() {
				if clean {
					endClient.Delete(config.prefix + "-server")
				}
			}()

			if _, err := endClient.Create(&endpoints); err != nil {
				framework.Failf("Failed to create endpoints for Gluster server: %v", err)
			}

			volume := api.VolumeSource{
				Glusterfs: &api.GlusterfsVolumeSource{
					EndpointsName: config.prefix + "-server",
					// 'test_vol' comes from test/images/volumes-tester/gluster/run_gluster.sh
					Path:     "test_vol",
					ReadOnly: true,
				},
			}
			// Must match content of test/images/volumes-tester/gluster/index.html
			testVolumeClient(f, config, volume, nil, "Hello from GlusterFS!")
		})
	})
})
