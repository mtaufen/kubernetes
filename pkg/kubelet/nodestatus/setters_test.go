/*
Copyright 2018 The Kubernetes Authors.

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

package nodestatus

import (
	"fmt"
	"net"
	goruntime "runtime"
	"sort"
	"sync"
	"testing"
	"time"

	cadvisorapiv1 "github.com/google/cadvisor/info/v1"

	"k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/client-go/tools/record"
	fakecloud "k8s.io/kubernetes/pkg/cloudprovider/providers/fake"
	kubeletapis "k8s.io/kubernetes/pkg/kubelet/apis"
	"k8s.io/kubernetes/pkg/volume"
	volumetest "k8s.io/kubernetes/pkg/volume/testing"

	"github.com/stretchr/testify/assert"
)

const (
	testKubeletHostname = "127.0.0.1"
)

// TODO(mtaufen): We don't currently test that events are sent (we use a stub fake recorder), but we should.

// TODO(mtaufen): Test cases must cover:
// - all injected getters map to expected fields on the modified node
// - any variations wrt conditional logic applied to getter results
// - remember data dependencies on input node

func TestZeroExtendedResources(t *testing.T) {
	extendedResourceName1 := v1.ResourceName("test.com/resource1")
	extendedResourceName2 := v1.ResourceName("test.com/resource2")

	cases := []struct {
		name         string
		existingNode *v1.Node
		expectedNode *v1.Node
	}{
		{
			name: "no update needed without extended resource",
			existingNode: &v1.Node{
				Status: v1.NodeStatus{
					Capacity: v1.ResourceList{
						v1.ResourceCPU:              *resource.NewMilliQuantity(2000, resource.DecimalSI),
						v1.ResourceMemory:           *resource.NewQuantity(10E9, resource.BinarySI),
						v1.ResourceEphemeralStorage: *resource.NewQuantity(5000, resource.BinarySI),
					},
					Allocatable: v1.ResourceList{
						v1.ResourceCPU:              *resource.NewMilliQuantity(2000, resource.DecimalSI),
						v1.ResourceMemory:           *resource.NewQuantity(10E9, resource.BinarySI),
						v1.ResourceEphemeralStorage: *resource.NewQuantity(5000, resource.BinarySI),
					},
				},
			},
			expectedNode: &v1.Node{
				Status: v1.NodeStatus{
					Capacity: v1.ResourceList{
						v1.ResourceCPU:              *resource.NewMilliQuantity(2000, resource.DecimalSI),
						v1.ResourceMemory:           *resource.NewQuantity(10E9, resource.BinarySI),
						v1.ResourceEphemeralStorage: *resource.NewQuantity(5000, resource.BinarySI),
					},
					Allocatable: v1.ResourceList{
						v1.ResourceCPU:              *resource.NewMilliQuantity(2000, resource.DecimalSI),
						v1.ResourceMemory:           *resource.NewQuantity(10E9, resource.BinarySI),
						v1.ResourceEphemeralStorage: *resource.NewQuantity(5000, resource.BinarySI),
					},
				},
			},
		},
		{
			name: "extended resource capacity is zeroed",
			existingNode: &v1.Node{
				Status: v1.NodeStatus{
					Capacity: v1.ResourceList{
						v1.ResourceCPU:              *resource.NewMilliQuantity(2000, resource.DecimalSI),
						v1.ResourceMemory:           *resource.NewQuantity(10E9, resource.BinarySI),
						v1.ResourceEphemeralStorage: *resource.NewQuantity(5000, resource.BinarySI),
						extendedResourceName1:       *resource.NewQuantity(int64(2), resource.DecimalSI),
						extendedResourceName2:       *resource.NewQuantity(int64(10), resource.DecimalSI),
					},
					Allocatable: v1.ResourceList{
						v1.ResourceCPU:              *resource.NewMilliQuantity(2000, resource.DecimalSI),
						v1.ResourceMemory:           *resource.NewQuantity(10E9, resource.BinarySI),
						v1.ResourceEphemeralStorage: *resource.NewQuantity(5000, resource.BinarySI),
						extendedResourceName1:       *resource.NewQuantity(int64(2), resource.DecimalSI),
						extendedResourceName2:       *resource.NewQuantity(int64(10), resource.DecimalSI),
					},
				},
			},
			expectedNode: &v1.Node{
				Status: v1.NodeStatus{
					Capacity: v1.ResourceList{
						v1.ResourceCPU:              *resource.NewMilliQuantity(2000, resource.DecimalSI),
						v1.ResourceMemory:           *resource.NewQuantity(10E9, resource.BinarySI),
						v1.ResourceEphemeralStorage: *resource.NewQuantity(5000, resource.BinarySI),
						extendedResourceName1:       *resource.NewQuantity(int64(0), resource.DecimalSI),
						extendedResourceName2:       *resource.NewQuantity(int64(0), resource.DecimalSI),
					},
					Allocatable: v1.ResourceList{
						v1.ResourceCPU:              *resource.NewMilliQuantity(2000, resource.DecimalSI),
						v1.ResourceMemory:           *resource.NewQuantity(10E9, resource.BinarySI),
						v1.ResourceEphemeralStorage: *resource.NewQuantity(5000, resource.BinarySI),
						extendedResourceName1:       *resource.NewQuantity(int64(0), resource.DecimalSI),
						extendedResourceName2:       *resource.NewQuantity(int64(0), resource.DecimalSI),
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setter := ZeroExtendedResources()
			if err := setter(tc.existingNode); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assert.Equal(t, tc.expectedNode, tc.existingNode)
		})
	}
}

// TODO(mtaufen): below is ported from the old kubelet_node_status_test.go code, potentially add more test coverage for EnforceDefaultLabels setter in future
func TestEnforceDefaultLabels(t *testing.T) {
	cases := []struct {
		name         string
		initialNode  *v1.Node
		existingNode *v1.Node
		needsUpdate  bool
		finalLabels  map[string]string
	}{
		{
			name: "make sure default labels exist",
			initialNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kubeletapis.LabelHostname:          "new-hostname",
						kubeletapis.LabelZoneFailureDomain: "new-zone-failure-domain",
						kubeletapis.LabelZoneRegion:        "new-zone-region",
						kubeletapis.LabelInstanceType:      "new-instance-type",
						kubeletapis.LabelOS:                "new-os",
						kubeletapis.LabelArch:              "new-arch",
					},
				},
			},
			existingNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
			needsUpdate: true,
			finalLabels: map[string]string{
				kubeletapis.LabelHostname:          "new-hostname",
				kubeletapis.LabelZoneFailureDomain: "new-zone-failure-domain",
				kubeletapis.LabelZoneRegion:        "new-zone-region",
				kubeletapis.LabelInstanceType:      "new-instance-type",
				kubeletapis.LabelOS:                "new-os",
				kubeletapis.LabelArch:              "new-arch",
			},
		},
		{
			name: "make sure default labels are up to date",
			initialNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kubeletapis.LabelHostname:          "new-hostname",
						kubeletapis.LabelZoneFailureDomain: "new-zone-failure-domain",
						kubeletapis.LabelZoneRegion:        "new-zone-region",
						kubeletapis.LabelInstanceType:      "new-instance-type",
						kubeletapis.LabelOS:                "new-os",
						kubeletapis.LabelArch:              "new-arch",
					},
				},
			},
			existingNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kubeletapis.LabelHostname:          "old-hostname",
						kubeletapis.LabelZoneFailureDomain: "old-zone-failure-domain",
						kubeletapis.LabelZoneRegion:        "old-zone-region",
						kubeletapis.LabelInstanceType:      "old-instance-type",
						kubeletapis.LabelOS:                "old-os",
						kubeletapis.LabelArch:              "old-arch",
					},
				},
			},
			needsUpdate: true,
			finalLabels: map[string]string{
				kubeletapis.LabelHostname:          "new-hostname",
				kubeletapis.LabelZoneFailureDomain: "new-zone-failure-domain",
				kubeletapis.LabelZoneRegion:        "new-zone-region",
				kubeletapis.LabelInstanceType:      "new-instance-type",
				kubeletapis.LabelOS:                "new-os",
				kubeletapis.LabelArch:              "new-arch",
			},
		},
		{
			name: "make sure existing labels do not get deleted",
			initialNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kubeletapis.LabelHostname:          "new-hostname",
						kubeletapis.LabelZoneFailureDomain: "new-zone-failure-domain",
						kubeletapis.LabelZoneRegion:        "new-zone-region",
						kubeletapis.LabelInstanceType:      "new-instance-type",
						kubeletapis.LabelOS:                "new-os",
						kubeletapis.LabelArch:              "new-arch",
					},
				},
			},
			existingNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kubeletapis.LabelHostname:          "new-hostname",
						kubeletapis.LabelZoneFailureDomain: "new-zone-failure-domain",
						kubeletapis.LabelZoneRegion:        "new-zone-region",
						kubeletapis.LabelInstanceType:      "new-instance-type",
						kubeletapis.LabelOS:                "new-os",
						kubeletapis.LabelArch:              "new-arch",
						"please-persist":                   "foo",
					},
				},
			},
			needsUpdate: false,
			finalLabels: map[string]string{
				kubeletapis.LabelHostname:          "new-hostname",
				kubeletapis.LabelZoneFailureDomain: "new-zone-failure-domain",
				kubeletapis.LabelZoneRegion:        "new-zone-region",
				kubeletapis.LabelInstanceType:      "new-instance-type",
				kubeletapis.LabelOS:                "new-os",
				kubeletapis.LabelArch:              "new-arch",
				"please-persist":                   "foo",
			},
		},
		{
			name: "make sure existing labels do not get deleted when initial node has no opinion",
			initialNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
			existingNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kubeletapis.LabelHostname:          "new-hostname",
						kubeletapis.LabelZoneFailureDomain: "new-zone-failure-domain",
						kubeletapis.LabelZoneRegion:        "new-zone-region",
						kubeletapis.LabelInstanceType:      "new-instance-type",
						kubeletapis.LabelOS:                "new-os",
						kubeletapis.LabelArch:              "new-arch",
						"please-persist":                   "foo",
					},
				},
			},
			needsUpdate: false,
			finalLabels: map[string]string{
				kubeletapis.LabelHostname:          "new-hostname",
				kubeletapis.LabelZoneFailureDomain: "new-zone-failure-domain",
				kubeletapis.LabelZoneRegion:        "new-zone-region",
				kubeletapis.LabelInstanceType:      "new-instance-type",
				kubeletapis.LabelOS:                "new-os",
				kubeletapis.LabelArch:              "new-arch",
				"please-persist":                   "foo",
			},
		},
		{
			name: "no update needed",
			initialNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kubeletapis.LabelHostname:          "new-hostname",
						kubeletapis.LabelZoneFailureDomain: "new-zone-failure-domain",
						kubeletapis.LabelZoneRegion:        "new-zone-region",
						kubeletapis.LabelInstanceType:      "new-instance-type",
						kubeletapis.LabelOS:                "new-os",
						kubeletapis.LabelArch:              "new-arch",
					},
				},
			},
			existingNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kubeletapis.LabelHostname:          "new-hostname",
						kubeletapis.LabelZoneFailureDomain: "new-zone-failure-domain",
						kubeletapis.LabelZoneRegion:        "new-zone-region",
						kubeletapis.LabelInstanceType:      "new-instance-type",
						kubeletapis.LabelOS:                "new-os",
						kubeletapis.LabelArch:              "new-arch",
					},
				},
			},
			needsUpdate: false,
			finalLabels: map[string]string{
				kubeletapis.LabelHostname:          "new-hostname",
				kubeletapis.LabelZoneFailureDomain: "new-zone-failure-domain",
				kubeletapis.LabelZoneRegion:        "new-zone-region",
				kubeletapis.LabelInstanceType:      "new-instance-type",
				kubeletapis.LabelOS:                "new-os",
				kubeletapis.LabelArch:              "new-arch",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setter := EnforceDefaultLabels(tc.initialNode.Labels)
			if err := setter(tc.existingNode); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assert.Equal(t, tc.finalLabels, tc.existingNode.Labels)
		})
	}
}

func TestVolumeLimits(t *testing.T) {
	// TODO(mtaufen): prior to porting to this package, this test was, in addition to testing the volume limits
	// setter, acting as a checksum on the actual limits returned for various cloud providers. I should ensure
	// that these providers at least have their own test coverage around GetVolumeLimits, and also that the manager
	// has test coverage around its ListVolumePluginWithLimits method.

	const (
		volumeLimitKey = "attachable-volumes-fake-provider"
		volumeLimitVal = 16
	)

	var cases = []struct {
		name           string
		listFunc       func() []volume.VolumePluginWithAttachLimits
		expectedLimits map[string]int64
	}{
		{
			name: "no error getting limits from plugin",
			listFunc: func() []volume.VolumePluginWithAttachLimits {
				return []volume.VolumePluginWithAttachLimits{
					&volumetest.FakeVolumePlugin{
						VolumeLimits: map[string]int64{volumeLimitKey: volumeLimitVal},
						LimitKey:     volumeLimitKey,
					},
				}
			},
			expectedLimits: map[string]int64{volumeLimitKey: volumeLimitVal},
		},
		{
			name: "error getting limits from plugin",
			listFunc: func() []volume.VolumePluginWithAttachLimits {
				return []volume.VolumePluginWithAttachLimits{
					&volumetest.FakeVolumePlugin{
						VolumeLimitsError: fmt.Errorf("fake volume limits error"),
					},
				}
			},
		},
		{
			name: "no plugins",
			listFunc: func() []volume.VolumePluginWithAttachLimits {
				return []volume.VolumePluginWithAttachLimits{}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := &v1.Node{}
			// construct setter
			setter := VolumeLimits(tc.listFunc)
			// call setter on node
			if err := setter(node); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// check for limits the setter should have produced
			for k, v := range tc.expectedLimits {
				expected := resource.NewQuantity(v, resource.DecimalSI)
				// check Allocatable
				if actual, ok := node.Status.Allocatable[v1.ResourceName(k)]; !ok {
					t.Errorf("Node.Status.Allocatable: expected key %s not found", k)
				} else if expected.Cmp(actual) != 0 {
					t.Errorf("Node.Status.Allocatable: key %s, expected limit: %s, got: %s", k, expected.String(), actual.String())
				}
				// check Capacity
				if actual, ok := node.Status.Capacity[v1.ResourceName(k)]; !ok {
					t.Errorf("Node.Status.Capacity: expected key %s not found", k)
				} else if expected.Cmp(actual) != 0 {
					t.Errorf("Node.Status.Capacity: key %s, expected limit: %s, got: %s", k, expected.String(), actual.String())
				}
			}
			// check against extra limits the setter should not have produced
			assert.Equal(t, len(tc.expectedLimits), len(node.Status.Allocatable),
				"Node.Status.Allocatable contained extra limits, expected: %v got: %v",
				tc.expectedLimits, node.Status.Allocatable)
			assert.Equal(t, len(tc.expectedLimits), len(node.Status.Capacity),
				"Node.Status.Capacity contained extra limits, expected: %v got: %v",
				tc.expectedLimits, node.Status.Capacity)
		})
	}
}

// TODO(mtaufen): below is ported from the old kubelet_node_status_test.go code, potentially add more test coverage for NodeAddress setter in future
func TestNodeAddress(t *testing.T) {
	cases := []struct {
		name              string
		nodeIP            net.IP
		nodeAddresses     []v1.NodeAddress
		expectedAddresses []v1.NodeAddress
		shouldError       bool
	}{
		{
			name:   "A single InternalIP",
			nodeIP: net.ParseIP("10.1.1.1"),
			nodeAddresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "10.1.1.1"},
				{Type: v1.NodeHostName, Address: testKubeletHostname},
			},
			expectedAddresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "10.1.1.1"},
				{Type: v1.NodeHostName, Address: testKubeletHostname},
			},
			shouldError: false,
		},
		{
			name:   "NodeIP is external",
			nodeIP: net.ParseIP("55.55.55.55"),
			nodeAddresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "10.1.1.1"},
				{Type: v1.NodeExternalIP, Address: "55.55.55.55"},
				{Type: v1.NodeHostName, Address: testKubeletHostname},
			},
			expectedAddresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "10.1.1.1"},
				{Type: v1.NodeExternalIP, Address: "55.55.55.55"},
				{Type: v1.NodeHostName, Address: testKubeletHostname},
			},
			shouldError: false,
		},
		{
			// Accommodating #45201 and #49202
			name:   "InternalIP and ExternalIP are the same",
			nodeIP: net.ParseIP("55.55.55.55"),
			nodeAddresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "55.55.55.55"},
				{Type: v1.NodeExternalIP, Address: "55.55.55.55"},
				{Type: v1.NodeHostName, Address: testKubeletHostname},
			},
			expectedAddresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "55.55.55.55"},
				{Type: v1.NodeExternalIP, Address: "55.55.55.55"},
				{Type: v1.NodeHostName, Address: testKubeletHostname},
			},
			shouldError: false,
		},
		{
			name:   "An Internal/ExternalIP, an Internal/ExternalDNS",
			nodeIP: net.ParseIP("10.1.1.1"),
			nodeAddresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "10.1.1.1"},
				{Type: v1.NodeExternalIP, Address: "55.55.55.55"},
				{Type: v1.NodeInternalDNS, Address: "ip-10-1-1-1.us-west-2.compute.internal"},
				{Type: v1.NodeExternalDNS, Address: "ec2-55-55-55-55.us-west-2.compute.amazonaws.com"},
				{Type: v1.NodeHostName, Address: testKubeletHostname},
			},
			expectedAddresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "10.1.1.1"},
				{Type: v1.NodeExternalIP, Address: "55.55.55.55"},
				{Type: v1.NodeInternalDNS, Address: "ip-10-1-1-1.us-west-2.compute.internal"},
				{Type: v1.NodeExternalDNS, Address: "ec2-55-55-55-55.us-west-2.compute.amazonaws.com"},
				{Type: v1.NodeHostName, Address: testKubeletHostname},
			},
			shouldError: false,
		},
		{
			name:   "An Internal with multiple internal IPs",
			nodeIP: net.ParseIP("10.1.1.1"),
			nodeAddresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "10.1.1.1"},
				{Type: v1.NodeInternalIP, Address: "10.2.2.2"},
				{Type: v1.NodeInternalIP, Address: "10.3.3.3"},
				{Type: v1.NodeExternalIP, Address: "55.55.55.55"},
				{Type: v1.NodeHostName, Address: testKubeletHostname},
			},
			expectedAddresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "10.1.1.1"},
				{Type: v1.NodeExternalIP, Address: "55.55.55.55"},
				{Type: v1.NodeHostName, Address: testKubeletHostname},
			},
			shouldError: false,
		},
		{
			name:   "An InternalIP that isn't valid: should error",
			nodeIP: net.ParseIP("10.2.2.2"),
			nodeAddresses: []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: "10.1.1.1"},
				{Type: v1.NodeExternalIP, Address: "55.55.55.55"},
				{Type: v1.NodeHostName, Address: testKubeletHostname},
			},
			expectedAddresses: nil,
			shouldError:       true,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// testCase setup
			existingNode := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: testKubeletHostname, Annotations: make(map[string]string)},
				Spec:       v1.NodeSpec{},
			}

			nodeIP := testCase.nodeIP
			hostname := testKubeletHostname
			nodeName := types.NodeName(testKubeletHostname)
			externalCloudProvider := false
			cloud := &fakecloud.FakeCloud{
				Addresses: testCase.nodeAddresses,
				Err:       nil,
			}
			cloudproviderRequestMux := &sync.Mutex{}
			cloudproviderRequestParallelism := make(chan int, 1)
			cloudproviderRequestSync := make(chan int)
			cloudproviderRequestTimeout := 10 * time.Second
			// TODO(mtaufen): consider using the standard validator here (this is the original test code, but...)
			nodeIPValidator := func(nodeIP net.IP) error {
				return nil
			}

			// construct setter
			setter := NodeAddress(nodeIP,
				nodeIPValidator,
				hostname,
				nodeName,
				externalCloudProvider,
				cloud,
				cloudproviderRequestMux,
				cloudproviderRequestParallelism,
				cloudproviderRequestSync,
				cloudproviderRequestTimeout)

			// call setter on existing node
			err := setter(existingNode)
			if err != nil && !testCase.shouldError {
				t.Fatalf("unexpected error: %v", err)
			} else if err != nil && testCase.shouldError {
				// expected an error, and got one, so just return early here
				return
			}

			// Sort both sets for consistent equality
			sortNodeAddresses(testCase.expectedAddresses)
			sortNodeAddresses(existingNode.Status.Addresses)

			assert.True(t, apiequality.Semantic.DeepEqual(testCase.expectedAddresses, existingNode.Status.Addresses),
				"Diff: %s", diff.ObjectDiff(testCase.expectedAddresses, existingNode.Status.Addresses))
		})
	}
}

func TestMachineInfo(t *testing.T) {
	// TODO(mtaufen): Add a test for this; kubelet_node_status_test.go did not have an independent unit test for setNodeStatusMachineInfo

	const nodeName = "test-node"
	nodeRef := &v1.ObjectReference{
		Kind:      "Node",
		Name:      nodeName,
		UID:       types.UID(nodeName),
		Namespace: "",
	}

	type dprc struct {
		nodeCapacity                  v1.ResourceList
		nodeAllocatable               v1.ResourceList
		inactiveDevicePluginResources []string
	}

	cases := []struct {
		desc string
		// inputs
		node                         *v1.Node
		maxPods                      int
		podsPerCore                  int
		machineInfo                  *cadvisorapiv1.MachineInfo
		machineInfoError             error
		capacity                     v1.ResourceList
		devicePluginResourceCapacity dprc
		nodeAllocatableReservation   v1.ResourceList
		// outputs
		expectNode *v1.Node
	}{
		// {
		// 	desc: "expected fields are set",
		// 	// inputs
		// 	maxPods:                      110,
		// 	podsPerCore:                  0,
		// 	machineInfo:                  &cadvisorapiv1.MachineInfo{},
		// 	capacity:                     v1.ResourceList{},
		// 	devicePluginResourceCapacity: dprc{},
		// 	nodeAllocatableReservation:   v1.ResourceList{},
		// 	// outputs
		// 	expectNode: &v1.Node{},
		// },
		{
			desc: "error getting machine info, but other expected fields are set",
			// inputs
			node:                         &v1.Node{},
			maxPods:                      110,
			podsPerCore:                  0,
			machineInfoError:             fmt.Errorf("foo"),
			capacity:                     v1.ResourceList{},
			devicePluginResourceCapacity: dprc{},
			nodeAllocatableReservation:   v1.ResourceList{},
			// outputs
			expectNode: &v1.Node{
				Status: v1.NodeStatus{
					Capacity: v1.ResourceList{
						// expect capacity=0 when there is an error getting machine info
						v1.ResourceCPU:    *resource.NewMilliQuantity(0, resource.DecimalSI),
						v1.ResourceMemory: resource.MustParse("0Gi"),
						v1.ResourcePods:   *resource.NewQuantity(int64(110), resource.DecimalSI),
					},
					// expect
					Allocatable: v1.ResourceList{},
				},
			},
		},
		// {
		// 	desc: "extended resources are removed from allocatable when no longer present in capacity",
		// },
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			// construct setter
			setter := MachineInfo(&record.FakeRecorder{},
				nodeName,
				nodeRef,
				tc.maxPods,
				tc.podsPerCore,
				func() (*cadvisorapiv1.MachineInfo, error) { return tc.machineInfo, tc.machineInfoError },
				func() v1.ResourceList { return tc.capacity },
				func() (v1.ResourceList, v1.ResourceList, []string) {
					c := tc.devicePluginResourceCapacity
					return c.nodeCapacity, c.nodeAllocatable, c.inactiveDevicePluginResources
				},
				func() v1.ResourceList { return tc.nodeAllocatableReservation })
			// call setter on a fresh node
			if err := setter(tc.node); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// assert that node is as expected
			assert.True(t, apiequality.Semantic.DeepEqual(tc.expectNode, tc.node),
				"Diff: %s", diff.ObjectDiff(tc.expectNode, tc.node))
		})
	}

}

func TestVersionInfo(t *testing.T) {
	// TODO(mtaufen): Add a test for this; kubelet_node_status_test.go did not have an independent unit test for setNodeStatusVersionInfo
}

func TestDaemonEndpoints(t *testing.T) {
	endpoints := &v1.NodeDaemonEndpoints{
		KubeletEndpoint: v1.DaemonEndpoint{
			Port: 10250,
		},
	}
	expect := *endpoints
	node := &v1.Node{}
	setter := DaemonEndpoints(endpoints)
	if err := setter(node); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assert.Equal(t, expect, node.Status.DaemonEndpoints)
}

func TestImages(t *testing.T) {
	// TODO(mtaufen): Add a test for this; kubelet_node_status_test.go did not have an independent unit test for setNodeStatusImages
	// (it definitely has aggregate tests, need to split these up)
}

func TestGoRuntime(t *testing.T) {
	node := &v1.Node{}
	setter := GoRuntime()
	if err := setter(node); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assert.Equal(t, goruntime.GOOS, node.Status.NodeInfo.OperatingSystem)
	assert.Equal(t, goruntime.GOARCH, node.Status.NodeInfo.Architecture)
}

func TestReadyCondition(t *testing.T) {
	// TODO(mtaufen): Add a test for this; kubelet_node_status_test.go did not have an independent unit test for setNodeReadyCondition
}

func TestMemoryPressureCondition(t *testing.T) {
	// TODO(mtaufen): Add a test for this; kubelet_node_status_test.go did not have an independent unit test for setNodeMemoryPressureCondition
}

func TestPIDPressureCondition(t *testing.T) {
	// TODO(mtaufen): Add a test for this; kubelet_node_status_test.go did not have an independent unit test for setNodePIDPressureCondition
}

func TestDiskPressureCondition(t *testing.T) {
	// TODO(mtaufen): Add a test for this; kubelet_node_status_test.go did not have an independent unit test for setNodeDiskPressureCondition

}

func TestOutOfDiskCondition(t *testing.T) {
	// TODO(mtaufen): Add a test for this; kubelet_node_status_test.go did not have an independent unit test for setNodeOODCondition
}

func TestVolumesInUse(t *testing.T) {
	// TODO(mtaufen): Add a test for this; kubelet_node_status_test.go did not have an independent unit test for setNodeVolumesInUseStatus
}

// Helpers:

// sortableNodeAddress is a type for sorting []v1.NodeAddress
type sortableNodeAddress []v1.NodeAddress

func (s sortableNodeAddress) Len() int { return len(s) }
func (s sortableNodeAddress) Less(i, j int) bool {
	return (string(s[i].Type) + s[i].Address) < (string(s[j].Type) + s[j].Address)
}
func (s sortableNodeAddress) Swap(i, j int) { s[j], s[i] = s[i], s[j] }

func sortNodeAddresses(addrs sortableNodeAddress) {
	sort.Sort(addrs)
}
