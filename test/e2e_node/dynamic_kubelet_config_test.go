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
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"

	apiv1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/apis/kubeletconfig"
	controller "k8s.io/kubernetes/pkg/kubelet/kubeletconfig"
	"k8s.io/kubernetes/pkg/kubelet/kubeletconfig/status"

	"k8s.io/kubernetes/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type configStateStatus struct {
	apiv1.NodeConfigStatus

	SkipActive   bool
	SkipAssigned bool
	SkipLkg      bool
}

type configState struct {
	desc               string
	configSource       *apiv1.NodeConfigSource
	configMap          *apiv1.ConfigMap
	expectConfigStatus *configStateStatus
	expectConfig       *kubeletconfig.KubeletConfiguration
	// whether to expect this substring in an error returned from the API server when updating the config source
	apierr string
	// whether the state would cause a config change event as a result of the update to Node.Spec.ConfigSource,
	// assuming that the current source would have also caused a config change event.
	// for example, some malformed references may result in a download failure, in which case the Kubelet
	// does not restart to change config, while an invalid payload will be detected upon restart
	event bool
}

// This test is marked [Disruptive] because the Kubelet restarts several times during this test.
var _ = framework.KubeDescribe("DynamicKubeletConfiguration [Feature:DynamicKubeletConfig] [Serial] [Disruptive]", func() {
	f := framework.NewDefaultFramework("dynamic-kubelet-configuration-test")
	var originalKC *kubeletconfig.KubeletConfiguration
	var originalConfigMap *apiv1.ConfigMap

	// Dummy context to prevent framework's AfterEach from cleaning up before this test's AfterEach can run
	Context("", func() {
		BeforeEach(func() {
			var err error
			if originalConfigMap == nil {
				originalKC, err = getCurrentKubeletConfig()
				framework.ExpectNoError(err)
				originalConfigMap = newKubeletConfigMap("original-values", originalKC)
				originalConfigMap, err = f.ClientSet.CoreV1().ConfigMaps("kube-system").Create(originalConfigMap)
				framework.ExpectNoError(err)
			}
			// make sure Dynamic Kubelet Configuration feature is enabled on the Kubelet we are about to test
			enabled, err := isKubeletConfigEnabled(f)
			framework.ExpectNoError(err)
			if !enabled {
				framework.ExpectNoError(fmt.Errorf("The Dynamic Kubelet Configuration feature is not enabled.\n" +
					"Pass --feature-gates=DynamicKubeletConfig=true to the Kubelet to enable this feature.\n" +
					"For `make test-e2e-node`, you can set `TEST_ARGS='--feature-gates=DynamicKubeletConfig=true'`."))
			}
		})

		AfterEach(func() {
			// Set the config back to the original values before moving on.
			// We care that the values are the same, not where they come from, so it
			// should be fine to reset the values using a remote config, even if they
			// were initially set via the locally provisioned configuration.
			// This is the same strategy several other e2e node tests use.

			source := &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
				UID:              originalConfigMap.UID,
				Namespace:        originalConfigMap.Namespace,
				Name:             originalConfigMap.Name,
				KubeletConfigKey: "kubelet",
			}}
			setAndTestKubeletConfigState(f, setConfigSourceFunc, &configState{desc: "reset to original values",
				configSource: source,
				expectConfigStatus: &configStateStatus{
					NodeConfigStatus: apiv1.NodeConfigStatus{
						Active:   source,
						Assigned: source,
					},
					SkipLkg: true,
				},
				expectConfig: originalKC,
			}, false)
		})

		Context("When changing NodeConfigSources", func() {
			It("the Kubelet should report the appropriate status and configz", func() {
				var err error
				// we base the "correct" configmap off of the current configuration
				correctKC := originalKC.DeepCopy()
				correctConfigMap := newKubeletConfigMap("dynamic-kubelet-config-test-correct", correctKC)
				correctConfigMap, err = f.ClientSet.CoreV1().ConfigMaps("kube-system").Create(correctConfigMap)
				framework.ExpectNoError(err)

				// fail to parse, we insert some bogus stuff into the configMap
				failParseConfigMap := &apiv1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "dynamic-kubelet-config-test-fail-parse"},
					Data: map[string]string{
						"kubelet": "{0xdeadbeef}",
					},
				}
				failParseConfigMap, err = f.ClientSet.CoreV1().ConfigMaps("kube-system").Create(failParseConfigMap)
				framework.ExpectNoError(err)

				// fail to validate, we make a copy and set an invalid KubeAPIQPS on kc before serializing
				invalidKC := correctKC.DeepCopy()

				invalidKC.KubeAPIQPS = -1
				failValidateConfigMap := newKubeletConfigMap("dynamic-kubelet-config-test-fail-validate", invalidKC)
				failValidateConfigMap, err = f.ClientSet.CoreV1().ConfigMaps("kube-system").Create(failValidateConfigMap)
				framework.ExpectNoError(err)

				correctSource := &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
					Namespace:        correctConfigMap.Namespace,
					Name:             correctConfigMap.Name,
					KubeletConfigKey: "kubelet",
				}}
				failParseSource := &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
					Namespace:        failParseConfigMap.Namespace,
					Name:             failParseConfigMap.Name,
					KubeletConfigKey: "kubelet",
				}}
				failValidateSource := &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
					Namespace:        failValidateConfigMap.Namespace,
					Name:             failValidateConfigMap.Name,
					KubeletConfigKey: "kubelet",
				}}

				// Note: since we start with the nil source (resets lkg), and we don't wait longer than the 10-minute internal
				// qualification period before changing it again, we can assume lkg source will be nil in the status
				// for this entire test, which is why we never set SkipLkg=true here.

				states := []configState{
					{
						desc:               "Node.Spec.ConfigSource is nil",
						configSource:       nil,
						expectConfigStatus: &configStateStatus{},
						expectConfig:       nil,
						event:              true,
					},
					{
						desc:         "Node.Spec.ConfigSource has all nil subfields",
						configSource: &apiv1.NodeConfigSource{},
						apierr:       "exactly one reference subfield must be non-nil",
					},
					{
						desc: "Node.Spec.ConfigSource.ConfigMap is missing namespace",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							Name:             "bar",
							KubeletConfigKey: "kubelet",
						}}, // missing Namespace
						apierr: "spec.configSource.configMap.namespace: Required value: namespace must be set in spec",
					},
					{
						desc: "Node.Spec.ConfigSource.ConfigMap is missing name",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							Namespace:        "foo",
							KubeletConfigKey: "kubelet",
						}}, // missing Name
						apierr: "spec.configSource.configMap.name: Required value: name must be set in spec",
					},
					{
						desc: "Node.Spec.ConfigSource.ConfigMap is missing kubeletConfigKey",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							Namespace: "foo",
							Name:      "bar",
						}}, // missing KubeletConfigKey
						apierr: "spec.configSource.configMap.kubeletConfigKey: Required value: kubeletConfigKey must be set in spec",
					},
					{
						desc: "Node.Spec.ConfigSource.ConfigMap.UID is illegally specified",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							UID:              "foo",
							Name:             "bar",
							Namespace:        "baz",
							KubeletConfigKey: "kubelet",
						}},
						apierr: "spec.configSource.configMap.uid: Forbidden: uid must not be set in spec",
					},
					{
						desc: "Node.Spec.ConfigSource.ConfigMap.ResourceVersion is illegally specified",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							Name:             "bar",
							Namespace:        "baz",
							ResourceVersion:  "1",
							KubeletConfigKey: "kubelet",
						}},
						apierr: "spec.configSource.configMap.resourceVersion: Forbidden: resourceVersion must not be set in spec",
					},
					{
						desc: "Node.Spec.ConfigSource.ConfigMap has invalid namespace",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							Name:             "bar",
							Namespace:        "../baz",
							KubeletConfigKey: "kubelet",
						}},
						apierr: "spec.configSource.configMap.namespace: Invalid value",
					},
					{
						desc: "Node.Spec.ConfigSource.ConfigMap has invalid name",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							Name:             "../bar",
							Namespace:        "baz",
							KubeletConfigKey: "kubelet",
						}},
						apierr: "spec.configSource.configMap.name: Invalid value",
					},
					{
						desc: "Node.Spec.ConfigSource.ConfigMap has invalid kubeletConfigKey",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							Name:             "bar",
							Namespace:        "baz",
							KubeletConfigKey: "../qux",
						}},
						apierr: "spec.configSource.configMap.kubeletConfigKey: Invalid value",
					},
					{
						desc:         "correct",
						configSource: correctSource,
						expectConfigStatus: &configStateStatus{
							NodeConfigStatus: apiv1.NodeConfigStatus{
								Active:        correctSource,
								Assigned:      correctSource,
								LastKnownGood: nil,
							},
						},
						expectConfig: correctKC,
						event:        true,
					},
					{
						desc:         "fail-parse",
						configSource: failParseSource,
						expectConfigStatus: &configStateStatus{
							NodeConfigStatus: apiv1.NodeConfigStatus{
								// we expect LastKnownGood to be nil for the duration of these tests,
								// thus we also expect Active to be nil here
								Active:        nil,
								Assigned:      failParseSource,
								LastKnownGood: nil,
								Error:         status.FailLoadError,
							},
						},
						expectConfig: nil,
						event:        true,
					},
					{
						desc:         "fail-validate",
						configSource: failValidateSource,
						expectConfigStatus: &configStateStatus{
							NodeConfigStatus: apiv1.NodeConfigStatus{
								// we expect LastKnownGood to be nil for the duration of these tests,
								// thus we also expect Active to be nil here
								Active:        nil,
								Assigned:      failValidateSource,
								LastKnownGood: nil,
								Error:         status.FailValidateError,
							},
						},
						expectConfig: nil,
						event:        true,
					},
				}

				L := len(states)
				for i := 1; i <= L; i++ { // need one less iteration than the number of states
					testBothDirections(f, setConfigSourceFunc, &states[i-1 : i][0], states[i:L], 0)
				}

			})
		})

		Context("When a remote config becomes the new last-known-good, and then the Kubelet is updated to use a new, bad config", func() {
			It("the Kubelet should report a status and configz indicating that it rolled back to the new last-known-good", func() {
				var err error
				// we base the "lkg" configmap off of the current configuration
				lkgKC := originalKC.DeepCopy()
				lkgConfigMap := newKubeletConfigMap("dynamic-kubelet-config-test-intended-lkg", lkgKC)
				lkgConfigMap, err = f.ClientSet.CoreV1().ConfigMaps("kube-system").Create(lkgConfigMap)
				framework.ExpectNoError(err)

				// bad config map, we insert some bogus stuff into the configMap
				badConfigMap := &apiv1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "dynamic-kubelet-config-test-bad"},
					Data: map[string]string{
						"kubelet": "{0xdeadbeef}",
					},
				}
				badConfigMap, err = f.ClientSet.CoreV1().ConfigMaps("kube-system").Create(badConfigMap)
				framework.ExpectNoError(err)

				lkgSource := &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
					Namespace:        lkgConfigMap.Namespace,
					Name:             lkgConfigMap.Name,
					KubeletConfigKey: "kubelet",
				}}
				badSource := &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
					Namespace:        badConfigMap.Namespace,
					Name:             badConfigMap.Name,
					KubeletConfigKey: "kubelet",
				}}

				states := []configState{
					// intended lkg
					{desc: "intended last-known-good",
						configSource: lkgSource,
						expectConfigStatus: &configStateStatus{
							NodeConfigStatus: apiv1.NodeConfigStatus{
								Active:   lkgSource,
								Assigned: lkgSource,
							},
							SkipLkg: true,
						},
						expectConfig: lkgKC,
						event:        true,
					},

					// bad config
					{desc: "bad config",
						configSource: badSource,
						expectConfigStatus: &configStateStatus{
							NodeConfigStatus: apiv1.NodeConfigStatus{
								Active:        lkgSource,
								Assigned:      badSource,
								LastKnownGood: lkgSource,
								Error:         status.FailLoadError,
							},
						},
						expectConfig: lkgKC,
						event:        true,
					},
				}

				// wait 12 minutes after setting the first config to ensure it has time to pass the trial duration
				testBothDirections(f, setConfigSourceFunc, &states[0], states[1:], 12*time.Minute)
			})
		})

		Context("When a remote config becomes the new last-known-good, and then Node.ConfigSource.ConfigMap.KubeletConfigKey is updated to use a new, bad config", func() {
			It("the Kubelet should report a status and configz indicating that it rolled back to the new last-known-good", func() {
				const badConfigKey = "bad"
				var err error
				// we base the "lkg" configmap off of the current configuration
				lkgKC := originalKC.DeepCopy()
				combinedConfigMap := newKubeletConfigMap("dynamic-kubelet-config-test-combined", lkgKC)
				combinedConfigMap.Data[badConfigKey] = "{0xdeadbeef}"
				combinedConfigMap, err = f.ClientSet.CoreV1().ConfigMaps("kube-system").Create(combinedConfigMap)
				framework.ExpectNoError(err)

				lkgSource := &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
					Namespace:        combinedConfigMap.Namespace,
					Name:             combinedConfigMap.Name,
					KubeletConfigKey: "kubelet",
				}}
				badSource := &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
					Namespace:        combinedConfigMap.Namespace,
					Name:             combinedConfigMap.Name,
					KubeletConfigKey: badConfigKey,
				}}

				states := []configState{
					// intended lkg
					{desc: "intended last-known-good",
						configSource: lkgSource,
						expectConfigStatus: &configStateStatus{
							NodeConfigStatus: apiv1.NodeConfigStatus{
								Active:   lkgSource,
								Assigned: lkgSource,
							},
							SkipLkg: true,
						},
						expectConfig: lkgKC,
						event:        true,
					},

					// bad config
					{desc: "bad config",
						configSource: badSource,
						expectConfigStatus: &configStateStatus{
							NodeConfigStatus: apiv1.NodeConfigStatus{
								Active:        lkgSource,
								Assigned:      badSource,
								LastKnownGood: lkgSource,
								Error:         status.FailLoadError,
							},
						},
						expectConfig: lkgKC,
						event:        true,
					},
				}

				// wait 12 minutes after setting the first config to ensure it has time to pass the trial duration
				testBothDirections(f, setConfigSourceFunc, &states[0], states[1:], 12*time.Minute)
			})
		})

		// This stress test will help turn up resource leaks across kubelet restarts that can, over time,
		// break our ability to dynamically update kubelet config
		Context("When changing the configuration 100 times", func() {
			It("the Kubelet should report the appropriate status and configz", func() {
				var err error

				// we just create two configmaps with the same config but different names and toggle between them
				kc1 := originalKC.DeepCopy()
				cm1 := newKubeletConfigMap("dynamic-kubelet-config-test-cm1", kc1)
				cm1, err = f.ClientSet.CoreV1().ConfigMaps("kube-system").Create(cm1)
				framework.ExpectNoError(err)

				// slightly change the config
				kc2 := kc1.DeepCopy()
				kc2.EventRecordQPS = kc1.EventRecordQPS + 1
				cm2 := newKubeletConfigMap("dynamic-kubelet-config-test-cm2", kc2)
				cm2, err = f.ClientSet.CoreV1().ConfigMaps("kube-system").Create(cm2)
				framework.ExpectNoError(err)

				cm1Source := &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
					Namespace:        cm1.Namespace,
					Name:             cm1.Name,
					KubeletConfigKey: "kubelet",
				}}
				cm2Source := &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
					Namespace:        cm2.Namespace,
					Name:             cm2.Name,
					KubeletConfigKey: "kubelet",
				}}

				states := []configState{
					{desc: "cm1",
						configSource: cm1Source,
						expectConfigStatus: &configStateStatus{
							NodeConfigStatus: apiv1.NodeConfigStatus{
								Active:   cm1Source,
								Assigned: cm1Source,
							},
							SkipLkg: true,
						},
						expectConfig: kc1,
						event:        true,
					},

					{desc: "cm2",
						configSource: cm2Source,
						expectConfigStatus: &configStateStatus{
							NodeConfigStatus: apiv1.NodeConfigStatus{
								Active:   cm2Source,
								Assigned: cm2Source,
							},
							SkipLkg: true,
						},
						expectConfig: kc2,
						event:        true,
					},
				}

				for i := 0; i < 50; i++ { // change the config 101 times (changes 3 times in the first iteration, 2 times in each subsequent iteration)
					testBothDirections(f, setConfigSourceFunc, &states[0], states[1:], 0)
				}
			})
		})

		// Please note: This behavior is tested to ensure implementation correctness. We do not, however, recommend ConfigMap mutations
		// as a usage pattern for dynamic Kubelet config. It is much safer to create a new ConfigMap, and incrementally roll out
		// a new NodeConfigSource that references the new ConfigMap across your Node objects.
		Context("When updating a ConfigMap in place", func() {
			It("the Kubelet should report the appropriate status and configz", func() {
				var err error
				// we base the "correct" configmap off of the current configuration
				correctKC := originalKC.DeepCopy()
				correctConfigMap := newKubeletConfigMap("dynamic-kubelet-config-test-in-place", correctKC)
				correctConfigMap, err = f.ClientSet.CoreV1().ConfigMaps("kube-system").Create(correctConfigMap)
				framework.ExpectNoError(err)

				// we reuse the same name, namespace // TODO(mtaufen): hopefully update works in this case
				failParseConfigMap := correctConfigMap.DeepCopy()
				failParseConfigMap.Data = map[string]string{
					"kubelet": "{0xdeadbeef}",
				}

				// fail to validate, we make a copy and set an invalid KubeAPIQPS on kc before serializing
				invalidKC := correctKC.DeepCopy()
				invalidKC.KubeAPIQPS = -1
				failValidateConfigMap := correctConfigMap.DeepCopy()
				failValidateConfigMap.Data = newKubeletConfigMap("", invalidKC).Data

				// reset the node's last-known-good config by setting the source to nil,
				// this way we know what the last-known-good will be for these tests
				setAndTestKubeletConfigState(f, setConfigSourceFunc,
					&configState{
						desc:         "reset via nil config source",
						configSource: nil,
						expectConfigStatus: &configStateStatus{
							NodeConfigStatus: apiv1.NodeConfigStatus{
								Active:        nil,
								Assigned:      nil,
								LastKnownGood: nil,
							},
						},
					}, false)

				// ensure node config source is set to the config map we will mutate in-place
				source := &apiv1.NodeConfigSource{
					ConfigMap: &apiv1.ConfigMapNodeConfigSource{
						Namespace:        correctConfigMap.Namespace,
						Name:             correctConfigMap.Name,
						KubeletConfigKey: "kubelet",
					},
				}
				setAndTestKubeletConfigState(f, setConfigSourceFunc,
					&configState{
						desc:         "initial state (correct)",
						configSource: source,
						expectConfigStatus: &configStateStatus{
							NodeConfigStatus: apiv1.NodeConfigStatus{
								Active:        source,
								Assigned:      source,
								LastKnownGood: nil,
							},
						},
						expectConfig: correctKC,
					}, false)

				states := []configState{
					{
						desc:         "correct",
						configSource: source,
						expectConfigStatus: &configStateStatus{
							NodeConfigStatus: apiv1.NodeConfigStatus{
								// TODO(mtaufen): will need to compute resource version here, maybe use setConfigMapFunc to do this
								Active:        source,
								Assigned:      source,
								LastKnownGood: nil,
							},
						},
						expectConfig: correctKC,
						event:        true,
					},
					{
						desc:         "fail-parse",
						configSource: source,
						expectConfigStatus: &configStateStatus{
							NodeConfigStatus: apiv1.NodeConfigStatus{
								// TODO(mtaufen): will need to compute resource version here, maybe use setConfigMapFunc to do this
								// we expect LastKnownGood to be nil for the duration of these tests,
								// thus we also expect Active to be nil here
								Active:        nil,
								Assigned:      source,
								LastKnownGood: nil,
								Error:         status.FailLoadError,
							},
						},
						expectConfig: nil,
						event:        true,
					},
					{
						desc:         "fail-validate",
						configSource: source,
						expectConfigStatus: &configStateStatus{
							NodeConfigStatus: apiv1.NodeConfigStatus{
								// TODO(mtaufen): will need to compute resource version here, maybe use setConfigMapFunc to do this
								// we expect LastKnownGood to be nil for the duration of these tests,
								// thus we also expect Active to be nil here
								Active:        nil,
								Assigned:      source,
								LastKnownGood: nil,
								Error:         status.FailValidateError,
							},
						},
						expectConfig: nil,
						event:        true,
					},
				}
				L := len(states)
				for i := 1; i <= L; i++ { // need one less iteration than the number of states
					testBothDirections(f, setConfigMapFunc, &states[i-1 : i][0], states[i:L], 0)
				}

			})
		})

		// // Please note: This behavior is tested to ensure implementation correctness. We do not, however, recommend ConfigMap mutations
		// // as a usage pattern for dynamic Kubelet config. It is much safer to create a new ConfigMap, and incrementally roll out
		// // a new NodeConfigSource that references the new ConfigMap across your Node objects.
		// // TODO(mtaufen): new test
		// Context("When a configmap version becomes the new last-known-good, and then the Kubelet is updated to use a new, bad config", func() {
		// 	It("the Kubelet should report a status and configz indicating that it rolled back to the new last-known-good", func() {
		// 		var err error
		// 		// we base the "lkg" configmap off of the current configuration
		// 		lkgKC := originalKC.DeepCopy()
		// 		lkgConfigMap := newKubeletConfigMap("dynamic-kubelet-config-test-intended-lkg", lkgKC)
		// 		lkgConfigMap, err = f.ClientSet.CoreV1().ConfigMaps("kube-system").Create(lkgConfigMap)
		// 		framework.ExpectNoError(err)

		// 		// bad config map, we insert some bogus stuff into the configMap
		// 		badConfigMap := &apiv1.ConfigMap{
		// 			ObjectMeta: metav1.ObjectMeta{Name: "dynamic-kubelet-config-test-bad"},
		// 			Data: map[string]string{
		// 				"kubelet": "{0xdeadbeef}",
		// 			},
		// 		}
		// 		badConfigMap, err = f.ClientSet.CoreV1().ConfigMaps("kube-system").Create(badConfigMap)
		// 		framework.ExpectNoError(err)

		// 		states := []configState{
		// 			// intended lkg
		// 			{desc: "intended last-known-good",
		// 				configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
		// 					ObjectReference: apiv1.ObjectReference{
		// 						UID:       lkgConfigMap.UID,
		// 						Namespace: lkgConfigMap.Namespace,
		// 						Name:      lkgConfigMap.Name},
		// 					KubeletConfigKey: "kubelet",
		// 				}},
		// 				expectConfigOk: &apiv1.NodeCondition{Type: apiv1.NodeKubeletConfigOk, Status: apiv1.ConditionTrue,
		// 					Message: fmt.Sprintf(status.CurRemoteMessageFmt, configMapAPIPath(lkgConfigMap), lkgConfigMap.UID, lkgConfigMap.ResourceVersion),
		// 					Reason:  status.CurRemoteOkayReason},
		// 				expectConfig: lkgKC,
		// 				event:        true,
		// 			},

		// 			// bad config
		// 			{desc: "bad config",
		// 				configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
		// 					ObjectReference: apiv1.ObjectReference{
		// 						UID:       badConfigMap.UID,
		// 						Namespace: badConfigMap.Namespace,
		// 						Name:      badConfigMap.Name},
		// 					KubeletConfigKey: "kubelet",
		// 				}},
		// 				expectConfigOk: &apiv1.NodeCondition{Type: apiv1.NodeKubeletConfigOk, Status: apiv1.ConditionFalse,
		// 					Message: fmt.Sprintf(status.LkgRemoteMessageFmt, configMapAPIPath(lkgConfigMap), lkgConfigMap.UID, lkgConfigMap.ResourceVersion),
		// 					Reason:  fmt.Sprintf(status.CurFailLoadReasonFmt, configMapAPIPath(badConfigMap), badConfigMap.UID, badConfigMap.ResourceVersion)},
		// 				expectConfig: lkgKC,
		// 				event:        true,
		// 			},
		// 		}

		// 		// wait 12 minutes after setting the first config to ensure it has time to pass the trial duration
		// 		testBothConfigSourceDirections(f, &states[0], states[1:], 15*time.Minute) // TODO(mtaufen): bring this back down to 12 to see if it continues to pass
		// 	})
		// })

		// // Please note: This behavior is tested to ensure implementation correctness. We do not, however, recommend ConfigMap mutations
		// // as a usage pattern for dynamic Kubelet config. It is much safer to create a new ConfigMap, and incrementally roll out
		// // a new NodeConfigSource that references the new ConfigMap across your Node objects.
		// // TODO(mtaufen): new test
		// Context("When deleting and recreating a ConfigMap", func() {
		// 	It("the Kubelet should respect the update and report the appropriate status and configz", func() {
		// 		// TODO(mtaufen): implement
		// 		// want to test delete and recreate with no change and delete and recreate with chagnes (both good and bad)
		// 	})
		// })
	})
})

// testBothDirections tests the state change represented by each edge, where each state is a vertex,
// and there are edges in each direction between first and each of the states.
func testBothDirections(f *framework.Framework, fn func(f *framework.Framework, state *configState) error, first *configState, states []configState, waitAfterFirst time.Duration) {
	// set to first and check that everything got set up properly
	By(fmt.Sprintf("setting initial state %q", first.desc))
	// we don't always expect an event here, because setting "first" might not represent
	// a change from the current configuration
	setAndTestKubeletConfigState(f, fn, first, false)

	// TODO(mtaufen): would be better to let setAndTest make this a wait between setting and testing,
	// so we can easily test if lkg shows up in status
	time.Sleep(waitAfterFirst)

	// for each state, set to that state, check expectations, then reset to first and check again
	for i := range states {
		By(fmt.Sprintf("from %q to %q", fn, first.desc, states[i].desc))
		// from first -> states[i], states[i].event fully describes whether we should get a config change event
		setAndTestKubeletConfigState(f, fn, &states[i], states[i].event)

		By(fmt.Sprintf("back to %q from %q", first.desc, states[i].desc))
		// whether first -> states[i] should have produced a config change event partially determines whether states[i] -> first should produce an event
		setAndTestKubeletConfigState(f, fn, first, first.event && states[i].event)
	}
}

// setAndTestKubeletConfigState tests that, after performing fn, the node spec, status, configz, and latest event match
// the expectations described by state.
func setAndTestKubeletConfigState(f *framework.Framework, fn func(f *framework.Framework, state *configState) error, state *configState, expectEvent bool) {
	// set the desired state, retry a few times in case we are competing with other editors
	Eventually(func() error {
		if err := fn(f, state); err != nil {
			if len(state.apierr) == 0 {
				return fmt.Errorf("case %s: expect nil error but got %q", state.desc, err.Error())
			} else if !strings.Contains(err.Error(), state.apierr) {
				return fmt.Errorf("case %s: expect error to contain %q but got %q", state.desc, state.apierr, err.Error())
			}
		} else if len(state.apierr) > 0 {
			return fmt.Errorf("case %s: expect error to contain %q but got nil error", state.desc, state.apierr)
		}
		return nil
	}, time.Minute, time.Second).Should(BeNil())
	// skip further checks if we expected an API error
	if len(state.apierr) > 0 {
		return
	}
	// check config source
	checkNodeConfigSource(f, state.desc, state.configSource)
	// check status
	checkConfigStatus(f, state.desc, state.expectConfigStatus)
	// check expectConfig
	if state.expectConfig != nil {
		checkConfig(f, state.desc, state.expectConfig)
	}
	// check that an event was sent for the config change
	if expectEvent {
		checkEvent(f, state.desc, state.configSource)
	}
}

// setConfigSourceFunc updates the configSource specified in state on the Node
func setConfigSourceFunc(f *framework.Framework, state *configState) error {
	return setNodeConfigSource(f, state.configSource)
}

// setConfigMapFunc updates the configMap with the same namespace/name as specified in state to
// contain the same data as in state. It also updates the resourceVersion in any non-nil
// NodeConfigSource.ConfigMap in the expected status to match the resourceVersion of the updated configMap.
func setConfigMapFunc(f *framework.Framework, state *configState) error {
	newConfigMap, err := f.ClientSet.CoreV1().ConfigMaps(state.configMap.Namespace).Update(state.configMap)
	if err != nil {
		return err
	}
	if expect := state.expectConfigStatus; expect != nil {
		if source := expect.Active; source != nil {
			if cm := source.ConfigMap; cm != nil {
				cm.ResourceVersion = newConfigMap.ResourceVersion
			}
		}
		if source := expect.Assigned; source != nil {
			if cm := source.ConfigMap; cm != nil {
				cm.ResourceVersion = newConfigMap.ResourceVersion
			}
		}
		if source := expect.LastKnownGood; source != nil {
			if cm := source.ConfigMap; cm != nil {
				cm.ResourceVersion = newConfigMap.ResourceVersion
			}
		}
	}
	return nil
}

// make sure the node's config source matches what we expect, after setting it
func checkNodeConfigSource(f *framework.Framework, desc string, expect *apiv1.NodeConfigSource) {
	const (
		timeout  = time.Minute
		interval = time.Second
	)
	Eventually(func() error {
		node, err := f.ClientSet.CoreV1().Nodes().Get(framework.TestContext.NodeName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("checkNodeConfigSource: case %s: %v", desc, err)
		}
		actual := node.Spec.ConfigSource
		if !reflect.DeepEqual(expect, actual) {
			return fmt.Errorf(spew.Sprintf("checkNodeConfigSource: case %s: expected %#v but got %#v", desc, expect, actual))
		}
		return nil
	}, timeout, interval).Should(BeNil())
}

// make sure the node status eventually matches what we expect
func checkConfigStatus(f *framework.Framework, desc string, expect *configStateStatus) {
	const (
		timeout  = time.Minute
		interval = time.Second
	)
	Eventually(func() error {
		node, err := f.ClientSet.CoreV1().Nodes().Get(framework.TestContext.NodeName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("checkConfigStatus: case %s: %v", desc, err)
		}
		if err := expectConfigStatus(expect, node.Status.Config); err != nil {
			return fmt.Errorf("checkConfigStatus: case %s: %v", desc, err)
		}
		return nil
	}, timeout, interval).Should(BeNil())
}

func expectConfigStatus(expect *configStateStatus, actual *apiv1.NodeConfigStatus) error {
	if expect == nil {
		return fmt.Errorf("expectConfigStatus requires expect to be a non-nil!")
	}
	if actual == nil {
		return fmt.Errorf("expectConfigStatus requires actual to be non-nil!")
	}
	var errs []string
	if !expect.SkipActive && !apiequality.Semantic.DeepEqual(expect.Active, actual.Active) {
		errs = append(errs, spew.Sprintf("expected Active %#v but got %#v", expect.Active, actual.Active))
	}
	if !expect.SkipAssigned && !apiequality.Semantic.DeepEqual(expect.Assigned, actual.Assigned) {
		errs = append(errs, spew.Sprintf("expected Assigned %#v but got %#v", expect.Assigned, actual.Assigned))
	}
	if !expect.SkipLkg && !apiequality.Semantic.DeepEqual(expect.LastKnownGood, actual.LastKnownGood) {
		errs = append(errs, spew.Sprintf("expected LastKnownGood %#v but got %#v", expect.LastKnownGood, actual.LastKnownGood))
	}
	if expect.Error != actual.Error {
		errs = append(errs, fmt.Sprintf("expected Error %q but got %q", expect.Error, actual.Error))
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, ", "))
	}
	return nil
}

// make sure config exposed on configz matches what we expect
func checkConfig(f *framework.Framework, desc string, expect *kubeletconfig.KubeletConfiguration) {
	const (
		timeout  = time.Minute
		interval = time.Second
	)
	Eventually(func() error {
		actual, err := getCurrentKubeletConfig()
		if err != nil {
			return fmt.Errorf("checkConfig: case %s: %v", desc, err)
		}
		if !reflect.DeepEqual(expect, actual) {
			return fmt.Errorf(spew.Sprintf("checkConfig: case %s: expected %#v but got %#v", desc, expect, actual))
		}
		return nil
	}, timeout, interval).Should(BeNil())
}

// checkEvent makes sure an event was sent marking the Kubelet's restart to use new config,
// and that it mentions the config we expect.
func checkEvent(f *framework.Framework, desc string, expect *apiv1.NodeConfigSource) {
	const (
		timeout  = time.Minute
		interval = time.Second
	)
	Eventually(func() error {
		events, err := f.ClientSet.CoreV1().Events("").List(metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("checkEvent: case %s: %v", desc, err)
		}
		// find config changed event with most recent timestamp
		var recent *apiv1.Event
		for i := range events.Items {
			if events.Items[i].Reason == controller.KubeletConfigChangedEventReason {
				if recent == nil {
					recent = &events.Items[i]
					continue
				}
				// for these events, first and last timestamp are always the same
				if events.Items[i].FirstTimestamp.Time.After(recent.FirstTimestamp.Time) {
					recent = &events.Items[i]
				}
			}
		}

		// we expect at least one config change event
		if recent == nil {
			return fmt.Errorf("checkEvent: case %s: no events found with reason %s", desc, controller.KubeletConfigChangedEventReason)
		}

		// ensure the message is what we expect (including the resource path)
		expectMessage := fmt.Sprintf(controller.EventMessageFmt, controller.LocalConfigMessage)
		if expect != nil {
			if expect.ConfigMap != nil {
				expectMessage = fmt.Sprintf(controller.EventMessageFmt, fmt.Sprintf("/api/v1/namespaces/%s/configmaps/%s", expect.ConfigMap.Namespace, expect.ConfigMap.Name))
			}
		}
		if expectMessage != recent.Message {
			return fmt.Errorf("checkEvent: case %s: expected event message %q but got %q", desc, expectMessage, recent.Message)
		}

		return nil
	}, timeout, interval).Should(BeNil())
}

// constructs the expected SelfLink for a config map
func configMapAPIPath(cm *apiv1.ConfigMap) string {
	return fmt.Sprintf("/api/v1/namespaces/%s/configmaps/%s", cm.Namespace, cm.Name)
}
