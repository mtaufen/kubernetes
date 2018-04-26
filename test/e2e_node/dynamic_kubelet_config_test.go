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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/apis/kubeletconfig"
	controller "k8s.io/kubernetes/pkg/kubelet/kubeletconfig"
	"k8s.io/kubernetes/pkg/kubelet/kubeletconfig/status"

	"k8s.io/kubernetes/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type configState struct {
	desc           string
	configSource   *apiv1.NodeConfigSource
	configMap      *apiv1.ConfigMap
	expectConfigOk *configOkState
	expectConfig   *kubeletconfig.KubeletConfiguration
	// whether to expect this substring in an error returned from the API server when updating the config source
	apierr string
	// whether the state would cause a config change event as a result of the update to Node.Spec.ConfigSource,
	// assuming that the current source would have also caused a config change event.
	// for example, some malformed references may result in a download failure, in which case the Kubelet
	// does not restart to change config, while an invalid payload will be detected upon restart
	event bool
}

type configOkState struct {
	status           apiv1.ConditionStatus
	messageFragments []string
	reasonFragments  []string
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
			// TODO(mtaufen): given difference in defaults between flags and config, we should
			// move to simply recording and resetting to the initial NodeConfigSource.
			setAndTestKubeletConfigSourceState(f, &configState{desc: "reset to original values",
				configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
					ObjectReference: apiv1.ObjectReference{
						UID:       originalConfigMap.UID,
						Namespace: originalConfigMap.Namespace,
						Name:      originalConfigMap.Name},
					KubeletConfigKey: "kubelet",
				}},
				expectConfigOk: &configOkState{
					status:           apiv1.ConditionTrue,
					messageFragments: []string{fmt.Sprintf(status.CurRemoteMessageFmt, configMapAPIPath(originalConfigMap), originalConfigMap.UID, originalConfigMap.ResourceVersion)},
					reasonFragments:  []string{status.CurRemoteOkayReason},
				},
				expectConfig: originalKC,
			}, false)
		})

		Context("When setting new NodeConfigSources that cause transitions between KubeletConfigOk conditions", func() {
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

				states := []configState{
					{
						desc:         "Node.Spec.ConfigSource is nil",
						configSource: nil,
						expectConfigOk: &configOkState{
							status:           apiv1.ConditionTrue,
							messageFragments: []string{status.CurLocalMessage},
							reasonFragments:  []string{status.CurLocalOkayReason},
						},
						expectConfig: nil,
						event:        true,
					},
					// Node.Spec.ConfigSource has all nil subfields
					{desc: "Node.Spec.ConfigSource has all nil subfields",
						configSource: &apiv1.NodeConfigSource{},
						apierr:       "exactly one reference subfield must be non-nil",
					},
					// Node.Spec.ConfigSource.ConfigMap is partial
					{desc: "Node.Spec.ConfigSource.ConfigMap is partial",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							ObjectReference: apiv1.ObjectReference{
								UID:  "foo",
								Name: "bar"},
							KubeletConfigKey: "kubelet",
						}}, // missing Namespace
						apierr: "name, namespace, UID, and KubeletConfigKey must all be non-empty",
					},
					{desc: "Node.Spec.ConfigSource.ConfigMap has invalid namespace",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							ObjectReference: apiv1.ObjectReference{
								UID:       "foo",
								Name:      "bar",
								Namespace: "../baz",
							},
							KubeletConfigKey: "kubelet",
						}},
						apierr: "spec.configSource.configMap.namespace: Invalid value",
					},
					{desc: "Node.Spec.ConfigSource.ConfigMap has invalid name",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							ObjectReference: apiv1.ObjectReference{
								UID:       "foo",
								Name:      "../bar",
								Namespace: "baz",
							},
							KubeletConfigKey: "kubelet",
						}},
						apierr: "spec.configSource.configMap.name: Invalid value",
					},
					{desc: "Node.Spec.ConfigSource.ConfigMap has invalid kubeletConfigKey",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							ObjectReference: apiv1.ObjectReference{
								UID:       "foo",
								Name:      "bar",
								Namespace: "baz",
							},
							KubeletConfigKey: "../qux",
						}},
						apierr: "spec.configSource.configMap.kubeletConfigKey: Invalid value",
					},
					{
						desc: "correct",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							ObjectReference: apiv1.ObjectReference{
								UID:       correctConfigMap.UID,
								Namespace: correctConfigMap.Namespace,
								Name:      correctConfigMap.Name},
							KubeletConfigKey: "kubelet",
						}},
						configMap: correctConfigMap,
						expectConfigOk: &configOkState{
							status:           apiv1.ConditionTrue,
							messageFragments: []string{fmt.Sprintf(status.CurRemoteMessageFmt, configMapAPIPath(correctConfigMap), correctConfigMap.UID, correctConfigMap.ResourceVersion)},
							reasonFragments:  []string{status.CurRemoteOkayReason},
						},
						expectConfig: correctKC,
						event:        true,
					},
					{
						desc: "kubeletConfigKey is not a file in the ConfigMap",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							ObjectReference: apiv1.ObjectReference{
								UID:       correctConfigMap.UID,
								Namespace: correctConfigMap.Namespace,
								Name:      correctConfigMap.Name},
							KubeletConfigKey: "foo",
						}},
						configMap: correctConfigMap,
						// TODO(mtaufen): make sure this expectation is correct
						expectConfigOk: &configOkState{
							status:           apiv1.ConditionFalse,
							messageFragments: []string{fmt.Sprintf(status.CurRemoteMessageFmt, configMapAPIPath(correctConfigMap), correctConfigMap.UID, correctConfigMap.ResourceVersion)},
							reasonFragments:  []string{status.CurRemoteOkayReason},
						},
						expectConfig: correctKC,
						event:        true,
					},
					{
						desc: "fail-parse",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							ObjectReference: apiv1.ObjectReference{
								UID:       failParseConfigMap.UID,
								Namespace: failParseConfigMap.Namespace,
								Name:      failParseConfigMap.Name},
							KubeletConfigKey: "kubelet",
						}},
						configMap: failParseConfigMap,
						expectConfigOk: &configOkState{
							status:           apiv1.ConditionFalse,
							messageFragments: []string{status.LkgLocalMessage},
							reasonFragments:  []string{fmt.Sprintf(status.CurFailLoadReasonFmt, configMapAPIPath(failParseConfigMap), failParseConfigMap.UID, failParseConfigMap.ResourceVersion)},
						},
						expectConfig: nil,
						event:        true,
					},
					{
						desc: "fail-validate",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							ObjectReference: apiv1.ObjectReference{
								UID:       failValidateConfigMap.UID,
								Namespace: failValidateConfigMap.Namespace,
								Name:      failValidateConfigMap.Name},
							KubeletConfigKey: "kubelet",
						}},
						configMap: failValidateConfigMap,
						expectConfigOk: &configOkState{
							status:           apiv1.ConditionFalse,
							messageFragments: []string{status.LkgLocalMessage},
							reasonFragments:  []string{fmt.Sprintf(status.CurFailValidateReasonFmt, configMapAPIPath(failValidateConfigMap), failValidateConfigMap.UID, failValidateConfigMap.ResourceVersion)},
						},
						expectConfig: nil,
						event:        true,
					},
				}

				L := len(states)
				for i := 1; i <= L; i++ { // need one less iteration than the number of states
					testBothConfigSourceDirections(f, &states[i-1 : i][0], states[i:L], 0)
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

				states := []configState{
					// intended lkg
					{desc: "intended last-known-good",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							ObjectReference: apiv1.ObjectReference{
								UID:       lkgConfigMap.UID,
								Namespace: lkgConfigMap.Namespace,
								Name:      lkgConfigMap.Name},
							KubeletConfigKey: "kubelet",
						}},
						configMap: lkgConfigMap,
						expectConfigOk: &configOkState{
							status:           apiv1.ConditionTrue,
							messageFragments: []string{fmt.Sprintf(status.CurRemoteMessageFmt, configMapAPIPath(lkgConfigMap), lkgConfigMap.UID, lkgConfigMap.ResourceVersion)},
							reasonFragments:  []string{status.CurRemoteOkayReason},
						},
						expectConfig: lkgKC,
						event:        true,
					},

					// bad config
					{desc: "bad config",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							ObjectReference: apiv1.ObjectReference{
								UID:       badConfigMap.UID,
								Namespace: badConfigMap.Namespace,
								Name:      badConfigMap.Name},
							KubeletConfigKey: "kubelet",
						}},
						configMap: badConfigMap,
						expectConfigOk: &configOkState{
							status:           apiv1.ConditionFalse,
							messageFragments: []string{fmt.Sprintf(status.LkgRemoteMessageFmt, configMapAPIPath(lkgConfigMap), lkgConfigMap.UID, lkgConfigMap.ResourceVersion)},
							reasonFragments:  []string{fmt.Sprintf(status.CurFailLoadReasonFmt, configMapAPIPath(badConfigMap), badConfigMap.UID, badConfigMap.ResourceVersion)},
						},
						expectConfig: lkgKC,
						event:        true,
					},
				}

				// wait 12 minutes after setting the first config to ensure it has time to pass the trial duration
				testBothConfigSourceDirections(f, &states[0], states[1:], 15*time.Minute) // TODO(mtaufen): bring this back down to 12 to see if it continues to pass
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

				states := []configState{
					// intended lkg
					{desc: "intended last-known-good",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							ObjectReference: apiv1.ObjectReference{
								UID:       combinedConfigMap.UID,
								Namespace: combinedConfigMap.Namespace,
								Name:      combinedConfigMap.Name},
							KubeletConfigKey: "kubelet",
						}},
						configMap: combinedConfigMap,
						expectConfigOk: &configOkState{
							status:           apiv1.ConditionTrue,
							messageFragments: []string{fmt.Sprintf(status.CurRemoteMessageFmt, configMapAPIPath(combinedConfigMap), combinedConfigMap.UID, combinedConfigMap.ResourceVersion)},
							reasonFragments:  []string{status.CurRemoteOkayReason},
						},
						expectConfig: lkgKC,
						event:        true,
					},

					// bad config, we update the source to point at the "bad" config key in the same ConfigMap
					{desc: "bad config",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							ObjectReference: apiv1.ObjectReference{
								UID:       combinedConfigMap.UID,
								Namespace: combinedConfigMap.Namespace,
								Name:      combinedConfigMap.Name},
							KubeletConfigKey: badConfigKey,
						}},
						configMap: combinedConfigMap,
						expectConfigOk: &configOkState{
							status:           apiv1.ConditionFalse,
							messageFragments: []string{fmt.Sprintf(status.LkgRemoteMessageFmt, configMapAPIPath(combinedConfigMap), combinedConfigMap.UID, combinedConfigMap.ResourceVersion)},
							reasonFragments:  []string{fmt.Sprintf(status.CurFailLoadReasonFmt, configMapAPIPath(combinedConfigMap), combinedConfigMap.UID, combinedConfigMap.ResourceVersion)},
						},
						expectConfig: lkgKC,
						event:        true,
					},
				}

				// wait 12 minutes after setting the first config to ensure it has time to pass the trial duration
				testBothConfigSourceDirections(f, &states[0], states[1:], 12*time.Minute)
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

				states := []configState{
					{desc: "cm1",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							ObjectReference: apiv1.ObjectReference{
								UID:       cm1.UID,
								Namespace: cm1.Namespace,
								Name:      cm1.Name},
							KubeletConfigKey: "kubelet",
						}},
						expectConfigOk: &configOkState{
							status:           apiv1.ConditionTrue,
							messageFragments: []string{fmt.Sprintf(status.CurRemoteMessageFmt, configMapAPIPath(cm1), cm1.UID, cm1.ResourceVersion)},
							reasonFragments:  []string{status.CurRemoteOkayReason},
						},
						expectConfig: kc1,
						event:        true,
					},

					{desc: "cm2",
						configSource: &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
							ObjectReference: apiv1.ObjectReference{
								UID:       cm2.UID,
								Namespace: cm2.Namespace,
								Name:      cm2.Name,
							},
							KubeletConfigKey: "kubelet",
						}},
						expectConfigOk: &configOkState{
							status:           apiv1.ConditionTrue,
							messageFragments: []string{fmt.Sprintf(status.CurRemoteMessageFmt, configMapAPIPath(cm2), cm2.UID, cm2.ResourceVersion)},
							reasonFragments:  []string{status.CurRemoteOkayReason},
						},
						expectConfig: kc2,
						event:        true,
					},
				}

				for i := 0; i < 50; i++ { // change the config 101 times (changes 3 times in the first iteration, 2 times in each subsequent iteration)
					testBothConfigSourceDirections(f, &states[0], states[1:], 0)
				}
			})
		})

		// Please note: This behavior is tested to ensure implementation correctness. We do not, however, recommend ConfigMap mutations
		// as a usage pattern for dynamic Kubelet config. It is much safer to create a new ConfigMap, and incrementally roll out
		// a new NodeConfigSource that references the new ConfigMap across your Node objects.
		Context("When updating a ConfigMap in place such that it causes transitions between KubeletConfigOk conditions", func() {
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

				// ensure node config source is set to the config map we will mutate in-place
				source := &apiv1.NodeConfigSource{ConfigMap: &apiv1.ConfigMapNodeConfigSource{
					ObjectReference: apiv1.ObjectReference{
						UID:       correctConfigMap.UID,
						Namespace: correctConfigMap.Namespace,
						Name:      correctConfigMap.Name},
					KubeletConfigKey: "kubelet",
				}}
				setAndTestKubeletConfigSourceState(f, &configState{
					desc:         "initial state (correct)",
					configSource: source,
					expectConfigOk: &configOkState{
						status:           apiv1.ConditionTrue,
						messageFragments: []string{fmt.Sprintf(status.CurRemoteMessageFmt, configMapAPIPath(correctConfigMap), correctConfigMap.UID, correctConfigMap.ResourceVersion)},
						reasonFragments:  []string{status.CurRemoteOkayReason},
					},
					expectConfig: correctKC,
					event:        true,
				}, false)

				states := []configState{
					{
						desc:         "correct",
						configSource: source,
						configMap:    correctConfigMap,
						expectConfigOk: &configOkState{
							status: apiv1.ConditionTrue,
							// we don't know the exact ResourceVersion since we're updating in place // TODO(mtaufen): the updater helper could detect this, maybe update test to automatically populate expected RV (could just add another message fragment)
							messageFragments: []string{fmt.Sprintf("using current: %s, UID: %s, ResourceVersion:", configMapAPIPath(correctConfigMap), correctConfigMap.UID)},
							reasonFragments:  []string{status.CurRemoteOkayReason},
						},
						expectConfig: correctKC,
						event:        true,
					},
					{
						desc:         "fail-parse",
						configSource: source,
						configMap:    failParseConfigMap,
						expectConfigOk: &configOkState{
							status:           apiv1.ConditionFalse,
							messageFragments: []string{status.LkgLocalMessage},
							reasonFragments:  []string{fmt.Sprintf("failed to load current: %s, UID: %s, ResourceVersion:", configMapAPIPath(failParseConfigMap), failParseConfigMap.UID)},
						},
						expectConfig: nil,
						event:        true,
					},
					{
						desc:         "fail-validate",
						configSource: source,
						configMap:    failValidateConfigMap,
						expectConfigOk: &configOkState{
							status:           apiv1.ConditionFalse,
							messageFragments: []string{status.LkgLocalMessage},
							reasonFragments:  []string{fmt.Sprintf("failed to validate current: %s, UID: %s, ResourceVersion:", configMapAPIPath(failValidateConfigMap), failValidateConfigMap.UID)},
						},
						expectConfig: nil,
						event:        true,
					},
				}
				L := len(states)
				for i := 1; i <= L; i++ { // need one less iteration than the number of states
					testBothConfigMapDirections(f, &states[i-1 : i][0], states[i:L], 0)
				}

			})
		})

		// // Please note: This behavior is tested to ensure implementation correctness. We do not, however, recommend ConfigMap mutations
		// // as a usage pattern for dynamic Kubelet config. It is much safer to create a new ConfigMap, and incrementally roll out
		// // a new NodeConfigSource that references the new ConfigMap across your Node objects.
		// // TODO(mtaufen): new test
		// Context("When a remote config becomes the new last-known-good, and then the Kubelet is updated to use a new, bad config", func() {
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

// testBothConfigSourceDirections tests the state change represented by each edge, where each state is a vertex,
// and there are edges in each direction between first and each of the states.
func testBothConfigSourceDirections(f *framework.Framework, first *configState, states []configState, waitAfterFirst time.Duration) {
	// set to first and check that everything got set up properly
	By(fmt.Sprintf("setting configSource to state %q", first.desc))
	// we don't always expect an event here, because setting "first" might not represent
	// a change from the current configuration
	setAndTestKubeletConfigSourceState(f, first, false)

	time.Sleep(waitAfterFirst)

	// for each state, set to that state, check condition and configz, then reset to first and check again
	for i := range states {
		By(fmt.Sprintf("from %q to %q", first.desc, states[i].desc))
		// from first -> states[i], states[i].event fully describes whether we should get a config change event
		setAndTestKubeletConfigSourceState(f, &states[i], states[i].event)

		By(fmt.Sprintf("back to %q from %q", first.desc, states[i].desc))
		// whether first -> states[i] should have produced a config change event partially determines whether states[i] -> first should produce an event
		setAndTestKubeletConfigSourceState(f, first, first.event && states[i].event)
	}
}

// testBothConfigMapDirections tests the state change represented by each edge, where each state is a vertex,
// and there are edges in each direction between first and each of the states.
// A ConfigMap must exist
func testBothConfigMapDirections(f *framework.Framework, first *configState, states []configState, waitAfterFirst time.Duration) {
	// set to first and check that everything got set up properly
	By(fmt.Sprintf("setting ConfigMap to state %q", first.desc))
	// we don't always expect an event here, because setting "first" might not represent
	// a change from the current configuration
	setAndTestKubeletConfigMapState(f, first, false)

	time.Sleep(waitAfterFirst)

	// for each state, set to that state, check condition and configz, then reset to first and check again
	for i := range states {
		By(fmt.Sprintf("from %q to %q", first.desc, states[i].desc))
		// from first -> states[i], states[i].event fully describes whether we should get a config change event
		setAndTestKubeletConfigMapState(f, &states[i], states[i].event)

		By(fmt.Sprintf("back to %q from %q", first.desc, states[i].desc))
		// whether first -> states[i] should have produced a config change event partially determines whether states[i] -> first should produce an event
		setAndTestKubeletConfigMapState(f, first, first.event && states[i].event)
	}
}

// setAndTestKubeletConfigSourceState tests that after setting the config source, the KubeletConfigOk condition
// and (if appropriate) configuration exposed via conifgz are as expected.
// The configuration will be converted to the internal type prior to comparison.
func setAndTestKubeletConfigSourceState(f *framework.Framework, state *configState, expectEvent bool) {
	// set the desired state, retry a few times in case we are competing with other editors
	Eventually(func() error {
		if err := setNodeConfigSource(f, state.configSource); err != nil {
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
	// check that config source actually got set to what we expect
	checkNodeConfigSource(f, state.desc, state.configSource)
	// check condition
	checkConfigOkCondition(f, state.desc, state.expectConfigOk)
	// check expectConfig
	if state.expectConfig != nil {
		checkConfig(f, state.desc, state.expectConfig)
	}
	// check that an event was sent for the config change
	if expectEvent {
		checkEvent(f, state.desc, state.configSource, state.configMap)
	}
}

// setAndTestKubeletConfigMapState tests that after setting the config source, the KubeletConfigOk condition
// and (if appropriate) configuration exposed via conifgz are as expected.
// The configuration will be converted to the internal type prior to comparison.
func setAndTestKubeletConfigMapState(f *framework.Framework, state *configState, expectEvent bool) {
	// TODO(mtaufen): may have to pull current configmap, make data change, and then send update

	// update the configmap
	Eventually(func() error {
		cm, err := f.ClientSet.CoreV1().ConfigMaps(state.configMap.Namespace).Update(state.configMap)
		if err != nil {
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
	// check condition
	checkConfigOkCondition(f, state.desc, state.expectConfigOk)
	// check expectConfig
	if state.expectConfig != nil {
		checkConfig(f, state.desc, state.expectConfig)
	}
	// check that an event was sent for the config change
	if expectEvent {
		checkEvent(f, state.desc, state.configSource, state.configMap)
	}
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

// make sure the ConfigOk node condition eventually matches what we expect
func checkConfigOkCondition(f *framework.Framework, desc string, expect *apiv1.NodeCondition) {
	const (
		timeout  = time.Minute
		interval = time.Second
	)
	Eventually(func() error {
		node, err := f.ClientSet.CoreV1().Nodes().Get(framework.TestContext.NodeName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("checkConfigOkCondition: case %s: %v", desc, err)
		}
		actual := getKubeletConfigOkCondition(node.Status.Conditions)
		if actual == nil {
			return fmt.Errorf("checkConfigOkCondition: case %s: ConfigOk condition not found on node %q", desc, framework.TestContext.NodeName)
		}
		if err := expectConfigOk(expect, actual); err != nil {
			return fmt.Errorf("checkConfigOkCondition: case %s: %v", desc, err)
		}
		return nil
	}, timeout, interval).Should(BeNil())
}

// if the actual matches the expect, return nil, else error explaining the mismatch
// if a subfield of the expect is the empty string, that check is skipped
func expectConfigOk(expect *configOkState, actual *apiv1.NodeCondition) error {
	if expect.status != actual.Status {
		return fmt.Errorf("expected condition Status %q but got %q", expect.status, actual.Status)
	}
	if len(expect.messageFragments) > 0 {
		for _, fragment := range expect.messageFragments {
			if !strings.Contains(actual.Message, fragment) {
				return fmt.Errorf("expected condition Message to contain %q but got %q", fragment, actual.Message)
			}
		}
	}
	if len(expect.reasonFragments) > 0 {
		for _, fragment := range expect.reasonFragments {
			if !strings.Contains(actual.Reason, fragment) {
				return fmt.Errorf("expected condition Reason to contain %q but got %q", fragment, actual.Reason)
			}
		}
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
// and that it mentions the config we expect. If a non-nil configSource is provided, it
// is expected that configMap will also be non-nil, and will match the configSource as appropriate.
func checkEvent(f *framework.Framework, desc string, configSource *apiv1.NodeConfigSource, configMap *apiv1.ConfigMap) {
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
		var fragments []string
		if configSource != nil {
			// we ignore the specific resourceversion, since it's hard to get a lock on it
			fragments = append(fragments, fmt.Sprintf(controller.EventMessageFmt, fmt.Sprintf("/api/v1/namespaces/%s/configmaps/%s, UID: %s, ResourceVersion:",
				configMap.Namespace,
				configMap.Name,
				configMap.UID)))
			fragments = append(fragments, fmt.Sprintf(controller.EventMessageFmt, fmt.Sprintf("KubeletConfigKey: %s",
				configSource.ConfigMap.KubeletConfigKey)))
		} else {
			fragments = append(fragments, fmt.Sprintf(controller.EventMessageFmt, controller.LocalConfigMessage))
		}
		for _, fragment := range fragments {
			if !strings.Contains(recent.Message, fragment) {
				return fmt.Errorf("checkEvent: case %s: expected event message to contain %q but got %q", desc, fragment, recent.Message)
			}
		}
		return nil
	}, timeout, interval).Should(BeNil())
}

// constructs the expected SelfLink for a config map
func configMapAPIPath(cm *apiv1.ConfigMap) string {
	return fmt.Sprintf("/api/v1/namespaces/%s/configmaps/%s", cm.Namespace, cm.Name)
}
