// +build !ignore_autogenerated

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

// This file was autogenerated by deepcopy-gen. Do not edit it manually!

package v1alpha1

import (
	api "k8s.io/kubernetes/pkg/api"
	unversioned "k8s.io/kubernetes/pkg/api/unversioned"
	conversion "k8s.io/kubernetes/pkg/conversion"
)

func init() {
	if err := api.Scheme.AddGeneratedDeepCopyFuncs(
		DeepCopy_v1alpha1_KubeProxyConfiguration,
		DeepCopy_v1alpha1_KubeSchedulerConfiguration,
		DeepCopy_v1alpha1_KubeletConfiguration,
		DeepCopy_v1alpha1_LeaderElectionConfiguration,
	); err != nil {
		// if one of the deep copy functions is malformed, detect it immediately.
		panic(err)
	}
}

func DeepCopy_v1alpha1_KubeProxyConfiguration(in KubeProxyConfiguration, out *KubeProxyConfiguration, c *conversion.Cloner) error {
	if err := unversioned.DeepCopy_unversioned_TypeMeta(in.TypeMeta, &out.TypeMeta, c); err != nil {
		return err
	}
	out.BindAddress = in.BindAddress
	out.ClusterCIDR = in.ClusterCIDR
	out.HealthzBindAddress = in.HealthzBindAddress
	out.HealthzPort = in.HealthzPort
	out.HostnameOverride = in.HostnameOverride
	if in.IPTablesMasqueradeBit != nil {
		in, out := in.IPTablesMasqueradeBit, &out.IPTablesMasqueradeBit
		*out = new(int32)
		**out = *in
	} else {
		out.IPTablesMasqueradeBit = nil
	}
	if err := unversioned.DeepCopy_unversioned_Duration(in.IPTablesSyncPeriod, &out.IPTablesSyncPeriod, c); err != nil {
		return err
	}
	out.KubeconfigPath = in.KubeconfigPath
	out.MasqueradeAll = in.MasqueradeAll
	out.Master = in.Master
	if in.OOMScoreAdj != nil {
		in, out := in.OOMScoreAdj, &out.OOMScoreAdj
		*out = new(int32)
		**out = *in
	} else {
		out.OOMScoreAdj = nil
	}
	out.Mode = in.Mode
	out.PortRange = in.PortRange
	out.ResourceContainer = in.ResourceContainer
	if err := unversioned.DeepCopy_unversioned_Duration(in.UDPIdleTimeout, &out.UDPIdleTimeout, c); err != nil {
		return err
	}
	out.ConntrackMax = in.ConntrackMax
	if err := unversioned.DeepCopy_unversioned_Duration(in.ConntrackTCPEstablishedTimeout, &out.ConntrackTCPEstablishedTimeout, c); err != nil {
		return err
	}
	return nil
}

func DeepCopy_v1alpha1_KubeSchedulerConfiguration(in KubeSchedulerConfiguration, out *KubeSchedulerConfiguration, c *conversion.Cloner) error {
	if err := unversioned.DeepCopy_unversioned_TypeMeta(in.TypeMeta, &out.TypeMeta, c); err != nil {
		return err
	}
	out.Port = in.Port
	out.Address = in.Address
	out.AlgorithmProvider = in.AlgorithmProvider
	out.PolicyConfigFile = in.PolicyConfigFile
	if in.EnableProfiling != nil {
		in, out := in.EnableProfiling, &out.EnableProfiling
		*out = new(bool)
		**out = *in
	} else {
		out.EnableProfiling = nil
	}
	out.ContentType = in.ContentType
	out.KubeAPIQPS = in.KubeAPIQPS
	out.KubeAPIBurst = in.KubeAPIBurst
	out.SchedulerName = in.SchedulerName
	out.HardPodAffinitySymmetricWeight = in.HardPodAffinitySymmetricWeight
	out.FailureDomains = in.FailureDomains
	if err := DeepCopy_v1alpha1_LeaderElectionConfiguration(in.LeaderElection, &out.LeaderElection, c); err != nil {
		return err
	}
	return nil
}

func DeepCopy_v1alpha1_KubeletConfiguration(in KubeletConfiguration, out *KubeletConfiguration, c *conversion.Cloner) error {
	if err := unversioned.DeepCopy_unversioned_TypeMeta(in.TypeMeta, &out.TypeMeta, c); err != nil {
		return err
	}
	out.Config = in.Config
	if err := unversioned.DeepCopy_unversioned_Duration(in.SyncFrequency, &out.SyncFrequency, c); err != nil {
		return err
	}
	if err := unversioned.DeepCopy_unversioned_Duration(in.FileCheckFrequency, &out.FileCheckFrequency, c); err != nil {
		return err
	}
	if err := unversioned.DeepCopy_unversioned_Duration(in.HTTPCheckFrequency, &out.HTTPCheckFrequency, c); err != nil {
		return err
	}
	out.ManifestURL = in.ManifestURL
	out.ManifestURLHeader = in.ManifestURLHeader
	if in.EnableServer != nil {
		in, out := in.EnableServer, &out.EnableServer
		*out = new(bool)
		**out = *in
	} else {
		out.EnableServer = nil
	}
	out.Address = in.Address
	out.Port = in.Port
	out.ReadOnlyPort = in.ReadOnlyPort
	out.TLSCertFile = in.TLSCertFile
	out.TLSPrivateKeyFile = in.TLSPrivateKeyFile
	out.CertDirectory = in.CertDirectory
	out.HostnameOverride = in.HostnameOverride
	out.PodInfraContainerImage = in.PodInfraContainerImage
	out.DockerEndpoint = in.DockerEndpoint
	out.RootDirectory = in.RootDirectory
	if in.AllowPrivileged != nil {
		in, out := in.AllowPrivileged, &out.AllowPrivileged
		*out = new(bool)
		**out = *in
	} else {
		out.AllowPrivileged = nil
	}
	if in.HostNetworkSources != nil {
		in, out := in.HostNetworkSources, &out.HostNetworkSources
		*out = make([]string, len(in))
		copy(*out, in)
	} else {
		out.HostNetworkSources = nil
	}
	if in.HostPIDSources != nil {
		in, out := in.HostPIDSources, &out.HostPIDSources
		*out = make([]string, len(in))
		copy(*out, in)
	} else {
		out.HostPIDSources = nil
	}
	if in.HostIPCSources != nil {
		in, out := in.HostIPCSources, &out.HostIPCSources
		*out = make([]string, len(in))
		copy(*out, in)
	} else {
		out.HostIPCSources = nil
	}
	out.RegistryPullQPS = in.RegistryPullQPS
	out.RegistryBurst = in.RegistryBurst
	out.EventRecordQPS = in.EventRecordQPS
	out.EventBurst = in.EventBurst
	if in.EnableDebuggingHandlers != nil {
		in, out := in.EnableDebuggingHandlers, &out.EnableDebuggingHandlers
		*out = new(bool)
		**out = *in
	} else {
		out.EnableDebuggingHandlers = nil
	}
	if err := unversioned.DeepCopy_unversioned_Duration(in.MinimumGCAge, &out.MinimumGCAge, c); err != nil {
		return err
	}
	out.MaxPerPodContainerCount = in.MaxPerPodContainerCount
	if in.MaxContainerCount != nil {
		in, out := in.MaxContainerCount, &out.MaxContainerCount
		*out = new(int64)
		**out = *in
	} else {
		out.MaxContainerCount = nil
	}
	out.CAdvisorPort = in.CAdvisorPort
	out.HealthzPort = in.HealthzPort
	out.HealthzBindAddress = in.HealthzBindAddress
	out.OOMScoreAdj = in.OOMScoreAdj
	if in.RegisterNode != nil {
		in, out := in.RegisterNode, &out.RegisterNode
		*out = new(bool)
		**out = *in
	} else {
		out.RegisterNode = nil
	}
	out.ClusterDomain = in.ClusterDomain
	out.MasterServiceNamespace = in.MasterServiceNamespace
	out.ClusterDNS = in.ClusterDNS
	if err := unversioned.DeepCopy_unversioned_Duration(in.StreamingConnectionIdleTimeout, &out.StreamingConnectionIdleTimeout, c); err != nil {
		return err
	}
	if err := unversioned.DeepCopy_unversioned_Duration(in.NodeStatusUpdateFrequency, &out.NodeStatusUpdateFrequency, c); err != nil {
		return err
	}
	if err := unversioned.DeepCopy_unversioned_Duration(in.ImageMinimumGCAge, &out.ImageMinimumGCAge, c); err != nil {
		return err
	}
	out.ImageGCHighThresholdPercent = in.ImageGCHighThresholdPercent
	out.ImageGCLowThresholdPercent = in.ImageGCLowThresholdPercent
	out.LowDiskSpaceThresholdMB = in.LowDiskSpaceThresholdMB
	if err := unversioned.DeepCopy_unversioned_Duration(in.VolumeStatsAggPeriod, &out.VolumeStatsAggPeriod, c); err != nil {
		return err
	}
	out.NetworkPluginName = in.NetworkPluginName
	out.NetworkPluginDir = in.NetworkPluginDir
	out.VolumePluginDir = in.VolumePluginDir
	out.CloudProvider = in.CloudProvider
	out.CloudConfigFile = in.CloudConfigFile
	out.KubeletCgroups = in.KubeletCgroups
	out.RuntimeCgroups = in.RuntimeCgroups
	out.SystemCgroups = in.SystemCgroups
	out.CgroupRoot = in.CgroupRoot
	out.ContainerRuntime = in.ContainerRuntime
	out.RktPath = in.RktPath
	if in.LockFilePath != nil {
		in, out := in.LockFilePath, &out.LockFilePath
		*out = new(string)
		**out = *in
	} else {
		out.LockFilePath = nil
	}
	out.RktStage1Image = in.RktStage1Image
	if in.ConfigureCBR0 != nil {
		in, out := in.ConfigureCBR0, &out.ConfigureCBR0
		*out = new(bool)
		**out = *in
	} else {
		out.ConfigureCBR0 = nil
	}
	out.HairpinMode = in.HairpinMode
	out.MaxPods = in.MaxPods
	out.DockerExecHandlerName = in.DockerExecHandlerName
	out.PodCIDR = in.PodCIDR
	out.ResolverConfig = in.ResolverConfig
	if in.CPUCFSQuota != nil {
		in, out := in.CPUCFSQuota, &out.CPUCFSQuota
		*out = new(bool)
		**out = *in
	} else {
		out.CPUCFSQuota = nil
	}
	if in.Containerized != nil {
		in, out := in.Containerized, &out.Containerized
		*out = new(bool)
		**out = *in
	} else {
		out.Containerized = nil
	}
	out.MaxOpenFiles = in.MaxOpenFiles
	if in.ReconcileCIDR != nil {
		in, out := in.ReconcileCIDR, &out.ReconcileCIDR
		*out = new(bool)
		**out = *in
	} else {
		out.ReconcileCIDR = nil
	}
	if in.RegisterSchedulable != nil {
		in, out := in.RegisterSchedulable, &out.RegisterSchedulable
		*out = new(bool)
		**out = *in
	} else {
		out.RegisterSchedulable = nil
	}
	out.KubeAPIQPS = in.KubeAPIQPS
	out.KubeAPIBurst = in.KubeAPIBurst
	if in.SerializeImagePulls != nil {
		in, out := in.SerializeImagePulls, &out.SerializeImagePulls
		*out = new(bool)
		**out = *in
	} else {
		out.SerializeImagePulls = nil
	}
	if in.ExperimentalFlannelOverlay != nil {
		in, out := in.ExperimentalFlannelOverlay, &out.ExperimentalFlannelOverlay
		*out = new(bool)
		**out = *in
	} else {
		out.ExperimentalFlannelOverlay = nil
	}
	if err := unversioned.DeepCopy_unversioned_Duration(in.OutOfDiskTransitionFrequency, &out.OutOfDiskTransitionFrequency, c); err != nil {
		return err
	}
	out.NodeIP = in.NodeIP
	if in.NodeLabels != nil {
		in, out := in.NodeLabels, &out.NodeLabels
		*out = make(map[string]string)
		for key, val := range in {
			(*out)[key] = val
		}
	} else {
		out.NodeLabels = nil
	}
	out.NonMasqueradeCIDR = in.NonMasqueradeCIDR
	out.EnableCustomMetrics = in.EnableCustomMetrics
	return nil
}

func DeepCopy_v1alpha1_LeaderElectionConfiguration(in LeaderElectionConfiguration, out *LeaderElectionConfiguration, c *conversion.Cloner) error {
	if in.LeaderElect != nil {
		in, out := in.LeaderElect, &out.LeaderElect
		*out = new(bool)
		**out = *in
	} else {
		out.LeaderElect = nil
	}
	if err := unversioned.DeepCopy_unversioned_Duration(in.LeaseDuration, &out.LeaseDuration, c); err != nil {
		return err
	}
	if err := unversioned.DeepCopy_unversioned_Duration(in.RenewDeadline, &out.RenewDeadline, c); err != nil {
		return err
	}
	if err := unversioned.DeepCopy_unversioned_Duration(in.RetryPeriod, &out.RetryPeriod, c); err != nil {
		return err
	}
	return nil
}
