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
	"context"
	"fmt"
	"math"
	"net"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	cadvisorapiv1 "github.com/google/cadvisor/info/v1"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/tools/record"
	v1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"
	"k8s.io/kubernetes/pkg/cloudprovider"
	"k8s.io/kubernetes/pkg/features"
	kubeletapis "k8s.io/kubernetes/pkg/kubelet/apis"
	"k8s.io/kubernetes/pkg/kubelet/cadvisor"
	"k8s.io/kubernetes/pkg/kubelet/cm"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	"k8s.io/kubernetes/pkg/kubelet/events"
	"k8s.io/kubernetes/pkg/version"
	"k8s.io/kubernetes/pkg/volume"

	"github.com/golang/glog"
)

const (
	// maxNamesPerImageInNodeStatus is max number of names per image stored in
	// the node status.
	maxNamesPerImageInNodeStatus = 5
)

// TODO(mtaufen): I tried to keep these setters as close to the originals as possible,
// but many of them are ENORMOUS and could use significant refactoring.

// Setter modifies the node in-place, and returns an error if the modification failed.
// Setters may partially mutate the node before returning an error.
// If mutations need to be transactional, Setters should ensure as much.
type Setter func(node *v1.Node) error

// ZeroExtendedResource returns a Setter that zeros out the capacity for any "extended resources."
func ZeroExtendedResources() Setter {
	return func(node *v1.Node) error {
		for k := range node.Status.Capacity {
			if v1helper.IsExtendedResourceName(k) {
				node.Status.Capacity[k] = *resource.NewQuantity(int64(0), resource.DecimalSI)
				node.Status.Allocatable[k] = *resource.NewQuantity(int64(0), resource.DecimalSI)
			}
		}
		return nil
	}
}

// TODO(mtaufen): need to figure out if a semantic deepequals is enough to see if an update is required
// TODO(mtaufen): I guess we'd inject the initial node's labels when constructing the setter to make this work... might end up being difficult or leveling violation
// TODO(mtaufen): doc comment on semantic of initialLabels - e.g. "Kubelet is configured to enforce these labels as such" (in this case rename to enforceLabels?)
// TODO(mtaufen): in this refactor, `node` is `existingNode` and `initialLabels` is`initialNode.Labels`
// EnforceDefaultLabels returns a Setter that enforces the initial values for the default set of node labels.
func EnforceDefaultLabels(initialLabels map[string]string) Setter {
	// TODO(mtaufen): consider copying the map for safety
	return func(node *v1.Node) error {
		defaultLabels := []string{
			kubeletapis.LabelHostname,
			kubeletapis.LabelZoneFailureDomain,
			kubeletapis.LabelZoneRegion,
			kubeletapis.LabelInstanceType,
			kubeletapis.LabelOS,
			kubeletapis.LabelArch,
		}
		for _, k := range defaultLabels {
			// If a default label is not found in the set of initial labels,
			// we are careful not to delete it from the node, because this
			// Kubelet is not configured to enforce the label. // TODO(mtaufen): this is my guess as to why, need to double-check
			if _, ok := initialLabels[k]; !ok {
				continue
			}

			// If the node has a different value on a default label than the initial
			// value for the label, we reset the node's label to the initial
			// value, because this Kubelet is configured to enforce the label. // TODO(mtaufen): this is my guess as to why, need to double-check
			//
			// Go's map-key-not-found behavior is unintuitive - it returns the zero value.
			// Thus, we enforce the inital label when it is missing from the node, unless
			// the initial label maps to the zero value.
			// TODO(mtaufen): what happens with initial labels with empty values, would they never actually show up on the node? Is this expected?
			if node.Labels[k] != initialLabels[k] {
				node.Labels[k] = initialLabels[k]
			}

			// If the node has an empty value for a default label, we delete
			// that label from the node. // TODO(mtaufen): need to investigate the reason for having to delete these
			if node.Labels[k] == "" {
				delete(node.Labels, k)
			}
		}
		return nil
	}
}

// TODO(mtaufen): may need to inject a getter for desired state of the annotation, not really sure where I'll inject this setter or what I want to do with it yet
func ControllerManagedAttachDetachAnnotation() Setter {
	return func(node *v1.Node) error {
		// TODO(mtaufen): implement
		return nil
	}
}

// VolumeLimits returns a Setter that updates the volume limits on the node.
// The injected listFunc lists volume plugins with limits. Typical usage is
// to inject Kubelet.volumePluginMgr.ListVolumePluginWithLimits.
func VolumeLimits(listFunc func() []volume.VolumePluginWithAttachLimits) Setter {
	return func(node *v1.Node) error {
		if node.Status.Capacity == nil {
			node.Status.Capacity = v1.ResourceList{}
		}
		if node.Status.Allocatable == nil {
			node.Status.Allocatable = v1.ResourceList{}
		}

		pluginWithLimits := listFunc()
		for _, volumePlugin := range pluginWithLimits {
			attachLimits, err := volumePlugin.GetVolumeLimits()
			if err != nil {
				// TODO(mtaufen): consider elevating this to an error, returning from the setter, and logging in the common place (common logging kills locality/debugging though...)
				glog.V(4).Infof("Error getting volume limit for plugin %s", volumePlugin.GetPluginName())
				continue
			}
			for limitKey, value := range attachLimits {
				node.Status.Capacity[v1.ResourceName(limitKey)] = *resource.NewQuantity(value, resource.DecimalSI)
				node.Status.Allocatable[v1.ResourceName(limitKey)] = *resource.NewQuantity(value, resource.DecimalSI)
			}
		}
		return nil
	}
}

// NodeAddress does all the complicated stuff it takes to get the address on the node... // TODO(mtaufen): give this a real, informative comment
func NodeAddress(nodeIP net.IP, // typically Kubelet.nodeIP
	validateNodeIPFunc func(net.IP) error, // typically Kubelet.nodeIPValidator
	hostname string, // typically Kubelet.hostname
	nodeName types.NodeName, // typically Kubelet.nodeName
	externalCloudProvider bool, // typically Kubelet.externalCloudProvider
	cloud cloudprovider.Interface, // typically Kubelet.cloud
	cloudproviderRequestMux *sync.Mutex, // typically Kubelet.cloudproviderRequestMux
	cloudproviderRequestParallelism chan int, // typically Kubelet.cloudproviderRequestParallelism
	cloudproviderRequestSync chan int, // typically Kubelet.cloudproviderRequestSync
	cloudproviderRequestTimeout time.Duration, // typically Kubelet.cloudproviderRequestTimeout
) Setter {
	return func(node *v1.Node) error {
		if nodeIP != nil {
			// TODO(mtaufen): consider lifting node IP validation to before the setter is generated...
			if err := validateNodeIPFunc(nodeIP); err != nil {
				return fmt.Errorf("failed to validate nodeIP: %v", err)
			}
			glog.V(2).Infof("Using node IP: %q", nodeIP.String())
		}

		if externalCloudProvider {
			if nodeIP != nil {
				if node.ObjectMeta.Annotations == nil {
					node.ObjectMeta.Annotations = make(map[string]string)
				}
				node.ObjectMeta.Annotations[kubeletapis.AnnotationProvidedIPAddr] = nodeIP.String()
			}
			// We rely on the external cloud provider to supply the addresses.
			return nil
		}
		if cloud != nil {
			instances, ok := cloud.Instances()
			if !ok {
				return fmt.Errorf("failed to get instances from cloud provider")
			}
			// TODO(roberthbailey): Can we do this without having credentials to talk
			// to the cloud provider?
			// TODO(justinsb): We can if CurrentNodeName() was actually CurrentNode() and returned an interface
			// TODO: If IP addresses couldn't be fetched from the cloud provider, should kubelet fallback on the other methods for getting the IP below?
			var nodeAddresses []v1.NodeAddress
			var err error

			// Make sure the instances.NodeAddresses returns even if the cloud provider API hangs for a long time
			func() {
				cloudproviderRequestMux.Lock()
				if len(cloudproviderRequestParallelism) > 0 {
					cloudproviderRequestMux.Unlock()
					return
				}
				cloudproviderRequestParallelism <- 0
				cloudproviderRequestMux.Unlock()

				go func() {
					nodeAddresses, err = instances.NodeAddresses(context.TODO(), nodeName)

					cloudproviderRequestMux.Lock()
					<-cloudproviderRequestParallelism
					cloudproviderRequestMux.Unlock()

					cloudproviderRequestSync <- 0
				}()
			}()

			select {
			case <-cloudproviderRequestSync:
			case <-time.After(cloudproviderRequestTimeout):
				err = fmt.Errorf("Timeout after %v", cloudproviderRequestTimeout)
			}

			if err != nil {
				return fmt.Errorf("failed to get node address from cloud provider: %v", err)
			}
			if nodeIP != nil {
				enforcedNodeAddresses := []v1.NodeAddress{}

				var nodeIPType v1.NodeAddressType
				for _, nodeAddress := range nodeAddresses {
					if nodeAddress.Address == nodeIP.String() {
						enforcedNodeAddresses = append(enforcedNodeAddresses, v1.NodeAddress{Type: nodeAddress.Type, Address: nodeAddress.Address})
						nodeIPType = nodeAddress.Type
						break
					}
				}
				if len(enforcedNodeAddresses) > 0 {
					for _, nodeAddress := range nodeAddresses {
						if nodeAddress.Type != nodeIPType && nodeAddress.Type != v1.NodeHostName {
							enforcedNodeAddresses = append(enforcedNodeAddresses, v1.NodeAddress{Type: nodeAddress.Type, Address: nodeAddress.Address})
						}
					}

					enforcedNodeAddresses = append(enforcedNodeAddresses, v1.NodeAddress{Type: v1.NodeHostName, Address: hostname})
					node.Status.Addresses = enforcedNodeAddresses
					return nil
				}
				return fmt.Errorf("failed to get node address from cloud provider that matches ip: %v", nodeIP)
			}

			// Only add a NodeHostName address if the cloudprovider did not specify one
			// (we assume the cloudprovider knows best)
			var addressNodeHostName *v1.NodeAddress
			for i := range nodeAddresses {
				if nodeAddresses[i].Type == v1.NodeHostName {
					addressNodeHostName = &nodeAddresses[i]
					break
				}
			}
			if addressNodeHostName == nil {
				hostnameAddress := v1.NodeAddress{Type: v1.NodeHostName, Address: hostname}
				nodeAddresses = append(nodeAddresses, hostnameAddress)
			} else {
				glog.V(2).Infof("Using Node Hostname from cloudprovider: %q", addressNodeHostName.Address)
			}
			node.Status.Addresses = nodeAddresses
		} else {
			var ipAddr net.IP
			var err error

			// 1) Use nodeIP if set
			// 2) If the user has specified an IP to HostnameOverride, use it
			// 3) Lookup the IP from node name by DNS and use the first valid IPv4 address.
			//    If the node does not have a valid IPv4 address, use the first valid IPv6 address.
			// 4) Try to get the IP from the network interface used as default gateway
			if nodeIP != nil {
				ipAddr = nodeIP
			} else if addr := net.ParseIP(hostname); addr != nil {
				ipAddr = addr
			} else {
				var addrs []net.IP
				addrs, _ = net.LookupIP(node.Name)
				for _, addr := range addrs {
					if err = validateNodeIPFunc(addr); err == nil {
						if addr.To4() != nil {
							ipAddr = addr
							break
						}
						if addr.To16() != nil && ipAddr == nil {
							ipAddr = addr
						}
					}
				}

				if ipAddr == nil {
					ipAddr, err = utilnet.ChooseHostInterface()
				}
			}

			if ipAddr == nil {
				// We tried everything we could, but the IP address wasn't fetchable; error out
				return fmt.Errorf("can't get ip address of node %s. error: %v", node.Name, err)
			}
			node.Status.Addresses = []v1.NodeAddress{
				{Type: v1.NodeInternalIP, Address: ipAddr.String()},
				{Type: v1.NodeHostName, Address: hostname},
			}
		}
		return nil
	}
}

// TODO(mtaufen): change this to just inject a recordEventFunc
func MachineInfo(recorder record.EventRecorder,
	nodeName string,
	nodeRef *v1.ObjectReference,
	maxPods int,
	podsPerCore int,
	machineInfoFunc func() (*cadvisorapiv1.MachineInfo, error), // typically Kubelet.GetCachedMachineInfo
	capacityFunc func() v1.ResourceList, // typically ContainerManager.GetCapacity
	devicePluginResourceCapacityFunc func() (v1.ResourceList, v1.ResourceList, []string), // typically ContainerManager.GetDevicePluginResourceCapacity
	nodeAllocatableReservationFunc func() v1.ResourceList, // typically ContainerManager.GetNodeAllocatableReservation
) Setter {
	return func(node *v1.Node) error {
		// Note: avoid blindly overwriting the capacity in case opaque
		//       resources are being advertised.
		if node.Status.Capacity == nil {
			node.Status.Capacity = v1.ResourceList{}
		}

		var devicePluginAllocatable v1.ResourceList
		var devicePluginCapacity v1.ResourceList
		var removedDevicePlugins []string

		// TODO: Post NotReady if we cannot get MachineInfo from cAdvisor. This needs to start
		// cAdvisor locally, e.g. for test-cmd.sh, and in integration test.
		info, err := machineInfoFunc()
		if err != nil {
			// TODO(roberthbailey): This is required for test-cmd.sh to pass.
			// See if the test should be updated instead.
			node.Status.Capacity[v1.ResourceCPU] = *resource.NewMilliQuantity(0, resource.DecimalSI)
			node.Status.Capacity[v1.ResourceMemory] = resource.MustParse("0Gi")
			node.Status.Capacity[v1.ResourcePods] = *resource.NewQuantity(int64(maxPods), resource.DecimalSI)
			glog.Errorf("Error getting machine info: %v", err)
		} else {
			node.Status.NodeInfo.MachineID = info.MachineID
			node.Status.NodeInfo.SystemUUID = info.SystemUUID

			for rName, rCap := range cadvisor.CapacityFromMachineInfo(info) {
				node.Status.Capacity[rName] = rCap
			}

			if podsPerCore > 0 {
				node.Status.Capacity[v1.ResourcePods] = *resource.NewQuantity(
					int64(math.Min(float64(info.NumCores*podsPerCore), float64(maxPods))), resource.DecimalSI)
			} else {
				node.Status.Capacity[v1.ResourcePods] = *resource.NewQuantity(
					int64(maxPods), resource.DecimalSI)
			}

			if node.Status.NodeInfo.BootID != "" &&
				node.Status.NodeInfo.BootID != info.BootID {
				// TODO: This requires a transaction, either both node status is updated
				// and event is recorded or neither should happen, see issue #6055.
				recorder.Eventf(nodeRef, v1.EventTypeWarning, events.NodeRebooted,
					"Node %s has been rebooted, boot id: %s", nodeName, info.BootID)
			}
			node.Status.NodeInfo.BootID = info.BootID

			if utilfeature.DefaultFeatureGate.Enabled(features.LocalStorageCapacityIsolation) {
				// TODO: all the node resources should use GetCapacity instead of deriving the
				// capacity for every node status request
				initialCapacity := capacityFunc()
				if initialCapacity != nil {
					node.Status.Capacity[v1.ResourceEphemeralStorage] = initialCapacity[v1.ResourceEphemeralStorage]
				}
			}

			devicePluginCapacity, devicePluginAllocatable, removedDevicePlugins = devicePluginResourceCapacityFunc()
			if devicePluginCapacity != nil {
				for k, v := range devicePluginCapacity {
					if old, ok := node.Status.Capacity[k]; !ok || old.Value() != v.Value() {
						glog.V(2).Infof("Update capacity for %s to %d", k, v.Value())
					}
					node.Status.Capacity[k] = v
				}
			}

			for _, removedResource := range removedDevicePlugins {
				glog.V(2).Infof("Set capacity for %s to 0 on device removal", removedResource)
				// Set the capacity of the removed resource to 0 instead of
				// removing the resource from the node status. This is to indicate
				// that the resource is managed by device plugin and had been
				// registered before.
				//
				// This is required to differentiate the device plugin managed
				// resources and the cluster-level resources, which are absent in
				// node status.
				node.Status.Capacity[v1.ResourceName(removedResource)] = *resource.NewQuantity(int64(0), resource.DecimalSI)
			}
		}

		// Set Allocatable.
		if node.Status.Allocatable == nil {
			node.Status.Allocatable = make(v1.ResourceList)
		}
		// Remove extended resources from allocatable that are no longer
		// present in capacity.
		for k := range node.Status.Allocatable {
			_, found := node.Status.Capacity[k]
			if !found && v1helper.IsExtendedResourceName(k) {
				delete(node.Status.Allocatable, k)
			}
		}
		allocatableReservation := nodeAllocatableReservationFunc()
		for k, v := range node.Status.Capacity {
			value := *(v.Copy())
			if res, exists := allocatableReservation[k]; exists {
				value.Sub(res)
			}
			if value.Sign() < 0 {
				// Negative Allocatable resources don't make sense.
				value.Set(0)
			}
			node.Status.Allocatable[k] = value
		}

		if devicePluginAllocatable != nil {
			for k, v := range devicePluginAllocatable {
				if old, ok := node.Status.Allocatable[k]; !ok || old.Value() != v.Value() {
					glog.V(2).Infof("Update allocatable for %s to %d", k, v.Value())
				}
				node.Status.Allocatable[k] = v
			}
		}
		// for every huge page reservation, we need to remove it from allocatable memory
		for k, v := range node.Status.Capacity {
			if v1helper.IsHugePageResourceName(k) {
				allocatableMemory := node.Status.Allocatable[v1.ResourceMemory]
				value := *(v.Copy())
				allocatableMemory.Sub(value)
				if allocatableMemory.Sign() < 0 {
					// Negative Allocatable resources don't make sense.
					allocatableMemory.Set(0)
				}
				node.Status.Allocatable[v1.ResourceMemory] = allocatableMemory
			}
		}
		return nil
	}
}

func VersionInfo(versionInfoFunc func() (*cadvisorapiv1.VersionInfo, error), // typically cadvisor.Interface.VersionInfo
	runtimeTypeFunc func() string, // typically kubecontainer.Runtime.Type
	runtimeVersionFunc func() (kubecontainer.Version, error), // typically kubecontainer.Runtime.Version
) Setter {
	return func(node *v1.Node) error {
		verinfo, err := versionInfoFunc()
		if err != nil {
			// TODO(mtaufen): consider removing this log line, since returned error will be logged?
			glog.Errorf("Error getting version info: %v", err)
			return fmt.Errorf("error getting version info: %v", err)
		}

		node.Status.NodeInfo.KernelVersion = verinfo.KernelVersion
		node.Status.NodeInfo.OSImage = verinfo.ContainerOsVersion

		runtimeVersion := "Unknown"
		if runtimeVer, err := runtimeVersionFunc(); err == nil {
			runtimeVersion = runtimeVer.String()
		}
		node.Status.NodeInfo.ContainerRuntimeVersion = fmt.Sprintf("%s://%s", runtimeTypeFunc(), runtimeVersion)

		node.Status.NodeInfo.KubeletVersion = version.Get().String()
		// TODO: kube-proxy might be different version from kubelet in the future
		node.Status.NodeInfo.KubeProxyVersion = version.Get().String()
		return nil
	}
}

func DaemonEndpoints(daemonEndpoints *v1.NodeDaemonEndpoints) Setter {
	return func(node *v1.Node) error {
		node.Status.DaemonEndpoints = *daemonEndpoints
		return nil
	}
}

func Images(nodeStatusMaxImages int32, imageListFunc func() ([]kubecontainer.Image, error)) Setter {
	return func(node *v1.Node) error {
		// Update image list of this node
		var imagesOnNode []v1.ContainerImage
		containerImages, err := imageListFunc()
		if err != nil {
			// TODO(mtaufen): consider removing this log line, since returned error will be logged?
			glog.Errorf("Error getting image list: %v", err)
			node.Status.Images = imagesOnNode
			return fmt.Errorf("error getting image list: %v", err)
		}
		// sort the images from max to min, and only set top N images into the node status.
		if int(nodeStatusMaxImages) > -1 &&
			int(nodeStatusMaxImages) < len(containerImages) {
			containerImages = containerImages[0:nodeStatusMaxImages]
		}

		for _, image := range containerImages {
			names := append(image.RepoDigests, image.RepoTags...)
			// Report up to maxNamesPerImageInNodeStatus names per image.
			if len(names) > maxNamesPerImageInNodeStatus {
				names = names[0:maxNamesPerImageInNodeStatus]
			}
			imagesOnNode = append(imagesOnNode, v1.ContainerImage{
				Names:     names,
				SizeBytes: image.Size,
			})
		}

		node.Status.Images = imagesOnNode
		return nil
	}
}

func GoRuntime() Setter {
	return func(node *v1.Node) error {
		node.Status.NodeInfo.OperatingSystem = goruntime.GOOS
		node.Status.NodeInfo.Architecture = goruntime.GOARCH
		return nil
	}
}

// TODO(mtaufen): sometime in the future, consider injecting feature gates to make testing easier
// TODO(mtaufen): unify how I refer to "typically x" in comments (e.g. as a Kubelet subfield, vs. a package and type for an interface)
func ReadyCondition(
	nowFunc func() time.Time, // typically Kubelet.clock.Now
	runtimeErrorsFunc func() []string, // typically Kubelet.runtimeState.runtimeErrors
	networkErrorsFunc func() []string, // typically Kubelet.runtimeState.networkErrors
	validateHostFunc func() error, // typically apparmor.Validator.ValidateHost, might be nil depending on whether there was an appArmorValidator
	cmStatusFunc func() cm.Status, // typically ContainerManager.Status()
	recordEventFunc func(eventType, event string), // typically Kubelet.recordNodeStatusEvent
) Setter {
	return func(node *v1.Node) error {
		// NOTE(aaronlevy): NodeReady condition needs to be the last in the list of node conditions.
		// This is due to an issue with version skewed kubelet and master components.
		// ref: https://github.com/kubernetes/kubernetes/issues/16961
		currentTime := metav1.NewTime(nowFunc())
		newNodeReadyCondition := v1.NodeCondition{
			Type:              v1.NodeReady,
			Status:            v1.ConditionTrue,
			Reason:            "KubeletReady",
			Message:           "kubelet is posting ready status",
			LastHeartbeatTime: currentTime,
		}
		rs := append(runtimeErrorsFunc(), networkErrorsFunc()...)
		requiredCapacities := []v1.ResourceName{v1.ResourceCPU, v1.ResourceMemory, v1.ResourcePods}
		if utilfeature.DefaultFeatureGate.Enabled(features.LocalStorageCapacityIsolation) {
			requiredCapacities = append(requiredCapacities, v1.ResourceEphemeralStorage)
		}
		missingCapacities := []string{}
		for _, resource := range requiredCapacities {
			if _, found := node.Status.Capacity[resource]; !found {
				missingCapacities = append(missingCapacities, string(resource))
			}
		}
		if len(missingCapacities) > 0 {
			rs = append(rs, fmt.Sprintf("Missing node capacity for resources: %s", strings.Join(missingCapacities, ", ")))
		}
		if len(rs) > 0 {
			newNodeReadyCondition = v1.NodeCondition{
				Type:              v1.NodeReady,
				Status:            v1.ConditionFalse,
				Reason:            "KubeletNotReady",
				Message:           strings.Join(rs, ","),
				LastHeartbeatTime: currentTime,
			}
		}
		// Append AppArmor status if it's enabled.
		// TODO(tallclair): This is a temporary message until node feature reporting is added.
		if validateHostFunc != nil && newNodeReadyCondition.Status == v1.ConditionTrue {
			if err := validateHostFunc(); err == nil {
				newNodeReadyCondition.Message = fmt.Sprintf("%s. AppArmor enabled", newNodeReadyCondition.Message)
			}
		}

		// Record any soft requirements that were not met in the container manager.
		status := cmStatusFunc()
		if status.SoftRequirements != nil {
			newNodeReadyCondition.Message = fmt.Sprintf("%s. WARNING: %s", newNodeReadyCondition.Message, status.SoftRequirements.Error())
		}

		readyConditionUpdated := false
		needToRecordEvent := false
		for i := range node.Status.Conditions {
			if node.Status.Conditions[i].Type == v1.NodeReady {
				if node.Status.Conditions[i].Status == newNodeReadyCondition.Status {
					newNodeReadyCondition.LastTransitionTime = node.Status.Conditions[i].LastTransitionTime
				} else {
					newNodeReadyCondition.LastTransitionTime = currentTime
					needToRecordEvent = true
				}
				node.Status.Conditions[i] = newNodeReadyCondition
				readyConditionUpdated = true
				break
			}
		}
		if !readyConditionUpdated {
			newNodeReadyCondition.LastTransitionTime = currentTime
			node.Status.Conditions = append(node.Status.Conditions, newNodeReadyCondition)
		}
		if needToRecordEvent {
			if newNodeReadyCondition.Status == v1.ConditionTrue {
				recordEventFunc(v1.EventTypeNormal, events.NodeReady)
			} else {
				recordEventFunc(v1.EventTypeNormal, events.NodeNotReady)
				glog.Infof("Node became not ready: %+v", newNodeReadyCondition)
			}
		}
		return nil
	}
}

// TODO(mtaufen): all of the "pressure" condition setters use roughly the same pattern; see if we can abstract this
func MemoryPressureCondition(nowFunc func() time.Time, // typically Kubelet.clock.Now
	pressureFunc func() bool, // typically Kubelet.evictionManager.IsUnderMemoryPressure
	recordEventFunc func(eventType, event string), // typically Kubelet.recordNodeStatusEvent
) Setter {
	return func(node *v1.Node) error {
		currentTime := metav1.NewTime(nowFunc())
		var condition *v1.NodeCondition

		// Check if NodeMemoryPressure condition already exists and if it does, just pick it up for update.
		for i := range node.Status.Conditions {
			if node.Status.Conditions[i].Type == v1.NodeMemoryPressure {
				condition = &node.Status.Conditions[i]
			}
		}

		newCondition := false
		// If the NodeMemoryPressure condition doesn't exist, create one
		if condition == nil {
			condition = &v1.NodeCondition{
				Type:   v1.NodeMemoryPressure,
				Status: v1.ConditionUnknown,
			}
			// cannot be appended to node.Status.Conditions here because it gets
			// copied to the slice. So if we append to the slice here none of the
			// updates we make below are reflected in the slice.
			newCondition = true
		}

		// Update the heartbeat time
		condition.LastHeartbeatTime = currentTime

		// Note: The conditions below take care of the case when a new NodeMemoryPressure condition is
		// created and as well as the case when the condition already exists. When a new condition
		// is created its status is set to v1.ConditionUnknown which matches either
		// condition.Status != v1.ConditionTrue or
		// condition.Status != v1.ConditionFalse in the conditions below depending on whether
		// the kubelet is under memory pressure or not.
		if pressureFunc() {
			if condition.Status != v1.ConditionTrue {
				condition.Status = v1.ConditionTrue
				condition.Reason = "KubeletHasInsufficientMemory"
				condition.Message = "kubelet has insufficient memory available"
				condition.LastTransitionTime = currentTime
				recordEventFunc(v1.EventTypeNormal, "NodeHasInsufficientMemory")
			}
		} else if condition.Status != v1.ConditionFalse {
			condition.Status = v1.ConditionFalse
			condition.Reason = "KubeletHasSufficientMemory"
			condition.Message = "kubelet has sufficient memory available"
			condition.LastTransitionTime = currentTime
			recordEventFunc(v1.EventTypeNormal, "NodeHasSufficientMemory")
		}

		if newCondition {
			node.Status.Conditions = append(node.Status.Conditions, *condition)
		}
		return nil
	}
}

func PIDPressureCondition(nowFunc func() time.Time, // typically Kubelet.clock.Now
	pressureFunc func() bool, // typically Kubelet.evictionManager.IsUnderPIDPressure
	recordEventFunc func(eventType, event string), // typically Kubelet.recordNodeStatusEvent
) Setter {
	return func(node *v1.Node) error {
		currentTime := metav1.NewTime(nowFunc())
		var condition *v1.NodeCondition

		// Check if NodePIDPressure condition already exists and if it does, just pick it up for update.
		for i := range node.Status.Conditions {
			if node.Status.Conditions[i].Type == v1.NodePIDPressure {
				condition = &node.Status.Conditions[i]
			}
		}

		newCondition := false
		// If the NodePIDPressure condition doesn't exist, create one
		if condition == nil {
			condition = &v1.NodeCondition{
				Type:   v1.NodePIDPressure,
				Status: v1.ConditionUnknown,
			}
			// cannot be appended to node.Status.Conditions here because it gets
			// copied to the slice. So if we append to the slice here none of the
			// updates we make below are reflected in the slice.
			newCondition = true
		}

		// Update the heartbeat time
		condition.LastHeartbeatTime = currentTime

		// Note: The conditions below take care of the case when a new NodePIDPressure condition is
		// created and as well as the case when the condition already exists. When a new condition
		// is created its status is set to v1.ConditionUnknown which matches either
		// condition.Status != v1.ConditionTrue or
		// condition.Status != v1.ConditionFalse in the conditions below depending on whether
		// the kubelet is under PID pressure or not.
		if pressureFunc() {
			if condition.Status != v1.ConditionTrue {
				condition.Status = v1.ConditionTrue
				condition.Reason = "KubeletHasInsufficientPID"
				condition.Message = "kubelet has insufficient PID available"
				condition.LastTransitionTime = currentTime
				recordEventFunc(v1.EventTypeNormal, "NodeHasInsufficientPID")
			}
		} else if condition.Status != v1.ConditionFalse {
			condition.Status = v1.ConditionFalse
			condition.Reason = "KubeletHasSufficientPID"
			condition.Message = "kubelet has sufficient PID available"
			condition.LastTransitionTime = currentTime
			recordEventFunc(v1.EventTypeNormal, "NodeHasSufficientPID")
		}

		if newCondition {
			node.Status.Conditions = append(node.Status.Conditions, *condition)
		}
		return nil
	}
}

func DiskPressureCondition(nowFunc func() time.Time, // typically Kubelet.clock.Now
	pressureFunc func() bool, // typically Kubelet.evictionManager.IsUnderDiskPressure
	recordEventFunc func(eventType, event string), // typically Kubelet.recordNodeStatusEvent
) Setter {
	return func(node *v1.Node) error {
		currentTime := metav1.NewTime(nowFunc())
		var condition *v1.NodeCondition

		// Check if NodeDiskPressure condition already exists and if it does, just pick it up for update.
		for i := range node.Status.Conditions {
			if node.Status.Conditions[i].Type == v1.NodeDiskPressure {
				condition = &node.Status.Conditions[i]
			}
		}

		newCondition := false
		// If the NodeDiskPressure condition doesn't exist, create one
		if condition == nil {
			condition = &v1.NodeCondition{
				Type:   v1.NodeDiskPressure,
				Status: v1.ConditionUnknown,
			}
			// cannot be appended to node.Status.Conditions here because it gets
			// copied to the slice. So if we append to the slice here none of the
			// updates we make below are reflected in the slice.
			newCondition = true
		}

		// Update the heartbeat time
		condition.LastHeartbeatTime = currentTime

		// Note: The conditions below take care of the case when a new NodeDiskPressure condition is
		// created and as well as the case when the condition already exists. When a new condition
		// is created its status is set to v1.ConditionUnknown which matches either
		// condition.Status != v1.ConditionTrue or
		// condition.Status != v1.ConditionFalse in the conditions below depending on whether
		// the kubelet is under disk pressure or not.
		if pressureFunc() {
			if condition.Status != v1.ConditionTrue {
				condition.Status = v1.ConditionTrue
				condition.Reason = "KubeletHasDiskPressure"
				condition.Message = "kubelet has disk pressure"
				condition.LastTransitionTime = currentTime
				recordEventFunc(v1.EventTypeNormal, "NodeHasDiskPressure")
			}
		} else if condition.Status != v1.ConditionFalse {
			condition.Status = v1.ConditionFalse
			condition.Reason = "KubeletHasNoDiskPressure"
			condition.Message = "kubelet has no disk pressure"
			condition.LastTransitionTime = currentTime
			recordEventFunc(v1.EventTypeNormal, "NodeHasNoDiskPressure")
		}

		if newCondition {
			node.Status.Conditions = append(node.Status.Conditions, *condition)
		}
		return nil
	}
}

// TODO(mtaufen): file an issue to remove this condition
// TODO(dashpole): probably remove this condition; it has been (we think, double-check) deprecated for enough releases
func OutOfDiskCondition(nowFunc func() time.Time, // typically Kubelet.clock.Now
	recordEventFunc func(eventType, event string), // typically Kubelet.recordNodeStatusEvent
) Setter {
	return func(node *v1.Node) error {
		currentTime := metav1.NewTime(nowFunc())
		var nodeOODCondition *v1.NodeCondition

		// Check if NodeOutOfDisk condition already exists and if it does, just pick it up for update.
		for i := range node.Status.Conditions {
			if node.Status.Conditions[i].Type == v1.NodeOutOfDisk {
				nodeOODCondition = &node.Status.Conditions[i]
			}
		}

		newOODCondition := nodeOODCondition == nil
		if newOODCondition {
			nodeOODCondition = &v1.NodeCondition{}
		}
		if nodeOODCondition.Status != v1.ConditionFalse {
			nodeOODCondition.Type = v1.NodeOutOfDisk
			nodeOODCondition.Status = v1.ConditionFalse
			nodeOODCondition.Reason = "KubeletHasSufficientDisk"
			nodeOODCondition.Message = "kubelet has sufficient disk space available"
			nodeOODCondition.LastTransitionTime = currentTime
			recordEventFunc(v1.EventTypeNormal, "NodeHasSufficientDisk")
		}

		// Update the heartbeat time irrespective of all the conditions.
		nodeOODCondition.LastHeartbeatTime = currentTime

		if newOODCondition {
			node.Status.Conditions = append(node.Status.Conditions, *nodeOODCondition)
		}
		return nil
	}
}

func VolumesInUse(syncedFunc func() bool, // typically Kubelet.volumeManager.ReconcilerStatesHasBeenSynced
	volumesInUseFunc func() []v1.UniqueVolumeName, // typically Kubelet.volumeManager.GetVolumesInUse
) Setter {
	return func(node *v1.Node) error {
		// Make sure to only update node status after reconciler starts syncing up states
		if syncedFunc() {
			node.Status.VolumesInUse = volumesInUseFunc()
		}
		return nil
	}
}
