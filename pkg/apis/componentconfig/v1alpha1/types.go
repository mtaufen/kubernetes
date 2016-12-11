/*
Below, find the componentconfig/v1alpha1/types.go file from 12/07~14:23 PST

I will add a comment immediately after each type noting
whether the field is generalizable or non-generalizable.

Possible scenatios (depends on the kind of component):
- configuration of component is eventually consistent cluster-wide
- configuration might have small differences per group of nodes or even per-node, but can still work under a deployment (kube-proxy)
- configuration needs values unique to a given node


Possible categories:
- gen: value may be set on more than one instance of the object (can be shared via a layer)
- nogen: value must be unique cluster-wide (can never be shared via a layer, thought technically in some cases it may be ok to gen with only the empty value (e.g. HostnameOverride))
- todo: I'm not sure and I'll come back to it

Might be an argument that typically only the Kubelet has nogen fields today.
Maybe.

Is this true?
gen <=> non-identifying information
nogen <=> identifying information
maybe...

And I think we have a third category:
Information that should not be in config at all, but should be discovered from the node

*/

/*
Copyright 2015 The Kubernetes Authors.

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

package v1alpha1

import (
	"k8s.io/kubernetes/pkg/api/v1"
	metav1 "k8s.io/kubernetes/pkg/apis/meta/v1"
)

type KubeProxyConfiguration struct {
	metav1.TypeMeta

	// bindAddress is the IP address for the proxy server to serve on (set to 0.0.0.0
	// for all interfaces)
	BindAddress string `json:"bindAddress"` //gen
	// clusterCIDR is the CIDR range of the pods in the cluster. It is used to
	// bridge traffic coming from outside of the cluster. If not provided,
	// no off-cluster bridging will be performed.
	ClusterCIDR string `json:"clusterCIDR"` //gen
	// healthzBindAddress is the IP address for the health check server to serve on,
	// defaulting to 127.0.0.1 (set to 0.0.0.0 for all interfaces)
	HealthzBindAddress string `json:"healthzBindAddress"`
	// healthzPort is the port to bind the health check server. Use 0 to disable.
	HealthzPort int32 `json:"healthzPort"` //gen
	// hostnameOverride, if non-empty, will be used as the identity instead of the actual hostname.
	HostnameOverride string `json:"hostnameOverride"` //nogen//can/is this taken from KubeletConfig?
	// iptablesMasqueradeBit is the bit of the iptables fwmark space to use for SNAT if using
	// the pure iptables proxy mode. Values must be within the range [0, 31].
	IPTablesMasqueradeBit *int32 `json:"iptablesMasqueradeBit"` //todo
	// iptablesSyncPeriod is the period that iptables rules are refreshed (e.g. '5s', '1m',
	// '2h22m').  Must be greater than 0.
	IPTablesSyncPeriod metav1.Duration `json:"iptablesSyncPeriodSeconds"` //gen
	// iptablesMinSyncPeriod is the minimum period that iptables rules are refreshed (e.g. '5s', '1m',
	// '2h22m').
	IPTablesMinSyncPeriod metav1.Duration `json:"iptablesMinSyncPeriodSeconds"` //gen
	// kubeconfigPath is the path to the kubeconfig file with authorization information (the
	// master location is set by the master flag).
	KubeconfigPath string `json:"kubeconfigPath"` //gen
	// masqueradeAll tells kube-proxy to SNAT everything if using the pure iptables proxy mode.
	MasqueradeAll bool `json:"masqueradeAll"` //gen
	// master is the address of the Kubernetes API server (overrides any value in kubeconfig)
	Master string `json:"master"` //gen
	// oomScoreAdj is the oom-score-adj value for kube-proxy process. Values must be within
	// the range [-1000, 1000]
	OOMScoreAdj *int32 `json:"oomScoreAdj"` //gen
	// mode specifies which proxy mode to use.
	Mode ProxyMode `json:"mode"` //gen
	// portRange is the range of host ports (beginPort-endPort, inclusive) that may be consumed
	// in order to proxy service traffic. If unspecified (0-0) then ports will be randomly chosen.
	PortRange string `json:"portRange"` //gen
	// resourceContainer is the bsolute name of the resource-only container to create and run
	// the Kube-proxy in (Default: /kube-proxy).
	ResourceContainer string `json:"resourceContainer"` //gen
	// udpIdleTimeout is how long an idle UDP connection will be kept open (e.g. '250ms', '2s').
	// Must be greater than 0. Only applicable for proxyMode=userspace.
	UDPIdleTimeout metav1.Duration `json:"udpTimeoutMilliseconds"` //gen
	// conntrackMax is the maximum number of NAT connections to track (0 to
	// leave as-is).  This takes precedence over conntrackMaxPerCore and conntrackMin.
	ConntrackMax int32 `json:"conntrackMax"` //gen
	// conntrackMaxPerCore is the maximum number of NAT connections to track
	// per CPU core (0 to leave the limit as-is and ignore conntrackMin).
	ConntrackMaxPerCore int32 `json:"conntrackMaxPerCore"` //gen
	// conntrackMin is the minimum value of connect-tracking records to allocate,
	// regardless of conntrackMaxPerCore (set conntrackMaxPerCore=0 to leave the limit as-is).
	ConntrackMin int32 `json:"conntrackMin"` //gen
	// conntrackTCPEstablishedTimeout is how long an idle TCP connection
	// will be kept open (e.g. '2s').  Must be greater than 0.
	ConntrackTCPEstablishedTimeout metav1.Duration `json:"conntrackTCPEstablishedTimeout"` //gen
	// conntrackTCPCloseWaitTimeout is how long an idle conntrack entry
	// in CLOSE_WAIT state will remain in the conntrack
	// table. (e.g. '60s'). Must be greater than 0 to set.
	ConntrackTCPCloseWaitTimeout metav1.Duration `json:"conntrackTCPCloseWaitTimeout"` //gen
}

// Currently two modes of proxying are available: 'userspace' (older, stable) or 'iptables'
// (experimental). If blank, look at the Node object on the Kubernetes API and respect the
// 'net.experimental.kubernetes.io/proxy-mode' annotation if provided.  Otherwise use the
// best-available proxy (currently userspace, but may change in future versions).  If the
// iptables proxy is selected, regardless of how, but the system's kernel or iptables
// versions are insufficient, this always falls back to the userspace proxy.
type ProxyMode string

const (
	ProxyModeUserspace ProxyMode = "userspace"
	ProxyModeIPTables  ProxyMode = "iptables"
)

type KubeSchedulerConfiguration struct {
	metav1.TypeMeta

	// port is the port that the scheduler's http service runs on.
	Port int `json:"port"` //gen
	// address is the IP address to serve on.
	Address string `json:"address"` //todo but I think gen
	// algorithmProvider is the scheduling algorithm provider to use.
	AlgorithmProvider string `json:"algorithmProvider"` //gen
	// policyConfigFile is the filepath to the scheduler policy configuration.
	PolicyConfigFile string `json:"policyConfigFile"` //gen
	// enableProfiling enables profiling via web interface.
	EnableProfiling *bool `json:"enableProfiling"` //gen
	// enableContentionProfiling enables lock contention profiling, if enableProfiling is true.
	EnableContentionProfiling bool `json:"enableContentionProfiling"` //gen
	// contentType is contentType of requests sent to apiserver.
	ContentType string `json:"contentType"` //gen
	// kubeAPIQPS is the QPS to use while talking with kubernetes apiserver.
	KubeAPIQPS float32 `json:"kubeAPIQPS"` //gen
	// kubeAPIBurst is the QPS burst to use while talking with kubernetes apiserver.
	KubeAPIBurst int `json:"kubeAPIBurst"` //gen
	// schedulerName is name of the scheduler, used to select which pods
	// will be processed by this scheduler, based on pod's annotation with
	// key 'scheduler.alpha.kubernetes.io/name'.
	SchedulerName string `json:"schedulerName"` //nogen
	// RequiredDuringScheduling affinity is not symmetric, but there is an implicit PreferredDuringScheduling affinity rule
	// corresponding to every RequiredDuringScheduling affinity rule.
	// HardPodAffinitySymmetricWeight represents the weight of implicit PreferredDuringScheduling affinity rule, in the range 0-100.
	HardPodAffinitySymmetricWeight int `json:"hardPodAffinitySymmetricWeight"` //gen
	// Indicate the "all topologies" set for empty topologyKey when it's used for PreferredDuringScheduling pod anti-affinity.
	FailureDomains string `json:"failureDomains"`
	// leaderElection defines the configuration of leader election client.
	LeaderElection LeaderElectionConfiguration `json:"leaderElection"` //gen
}

// HairpinMode denotes how the kubelet should configure networking to handle
// hairpin packets.
type HairpinMode string

// Enum settings for different ways to handle hairpin packets.
const (
	// Set the hairpin flag on the veth of containers in the respective
	// container runtime.
	HairpinVeth = "hairpin-veth"
	// Make the container bridge promiscuous. This will force it to accept
	// hairpin packets, even if the flag isn't set on ports of the bridge.
	PromiscuousBridge = "promiscuous-bridge"
	// Neither of the above. If the kubelet is started in this hairpin mode
	// and kube-proxy is running in iptables mode, hairpin packets will be
	// dropped by the container bridge.
	HairpinNone = "none"
)

// LeaderElectionConfiguration defines the configuration of leader election
// clients for components that can run with leader election enabled.
type LeaderElectionConfiguration struct {
	// leaderElect enables a leader election client to gain leadership
	// before executing the main loop. Enable this when running replicated
	// components for high availability.
	LeaderElect *bool `json:"leaderElect"` //gen
	// leaseDuration is the duration that non-leader candidates will wait
	// after observing a leadership renewal until attempting to acquire
	// leadership of a led but unrenewed leader slot. This is effectively the
	// maximum duration that a leader can be stopped before it is replaced
	// by another candidate. This is only applicable if leader election is
	// enabled.
	LeaseDuration metav1.Duration `json:"leaseDuration"` //gen
	// renewDeadline is the interval between attempts by the acting master to
	// renew a leadership slot before it stops leading. This must be less
	// than or equal to the lease duration. This is only applicable if leader
	// election is enabled.
	RenewDeadline metav1.Duration `json:"renewDeadline"` //gen
	// retryPeriod is the duration the clients should wait between attempting
	// acquisition and renewal of a leadership. This is only applicable if
	// leader election is enabled.
	RetryPeriod metav1.Duration `json:"retryPeriod"` //gen
}

type KubeletConfiguration struct {
	metav1.TypeMeta

	// podManifestPath is the path to the directory containing pod manifests to
	// run, or the path to a single manifest file
	PodManifestPath string `json:"podManifestPath"` //gen
	// syncFrequency is the max period between synchronizing running
	// containers and config
	SyncFrequency metav1.Duration `json:"syncFrequency"` //gen
	// fileCheckFrequency is the duration between checking config files for
	// new data
	FileCheckFrequency metav1.Duration `json:"fileCheckFrequency"` //gen
	// httpCheckFrequency is the duration between checking http for new data
	HTTPCheckFrequency metav1.Duration `json:"httpCheckFrequency"` //gen
	// manifestURL is the URL for accessing the container manifest
	ManifestURL string `json:"manifestURL"` //todo
	// manifestURLHeader is the HTTP header to use when accessing the manifest
	// URL, with the key separated from the value with a ':', as in 'key:value'
	ManifestURLHeader string `json:"manifestURLHeader"` //gen
	// enableServer enables the Kubelet's server
	EnableServer *bool `json:"enableServer"` //gen
	// address is the IP address for the Kubelet to serve on (set to 0.0.0.0
	// for all interfaces)
	Address string `json:"address"` //gen
	// port is the port for the Kubelet to serve on.
	Port int32 `json:"port"` //gen
	// readOnlyPort is the read-only port for the Kubelet to serve on with
	// no authentication/authorization (set to 0 to disable)
	ReadOnlyPort int32 `json:"readOnlyPort"` //gen
	// tlsCertFile is the file containing x509 Certificate for HTTPS.  (CA cert,
	// if any, concatenated after server cert). If tlsCertFile and
	// tlsPrivateKeyFile are not provided, a self-signed certificate
	// and key are generated for the public address and saved to the directory
	// passed to certDir.
	TLSCertFile string `json:"tlsCertFile"` //gen
	// tlsPrivateKeyFile is the ile containing x509 private key matching
	// tlsCertFile.
	TLSPrivateKeyFile string `json:"tlsPrivateKeyFile"` //gen
	// certDirectory is the directory where the TLS certs are located (by
	// default /var/run/kubernetes). If tlsCertFile and tlsPrivateKeyFile
	// are provided, this flag will be ignored.
	CertDirectory string `json:"certDirectory"` //gen
	// authentication specifies how requests to the Kubelet's server are authenticated
	Authentication KubeletAuthentication `json:"authentication"` //todo
	// authorization specifies how requests to the Kubelet's server are authorized
	Authorization KubeletAuthorization `json:"authorization"` //todo
	// hostnameOverride is the hostname used to identify the kubelet instead
	// of the actual hostname.
	HostnameOverride string `json:"hostnameOverride"` //nogen
	// podInfraContainerImage is the image whose network/ipc namespaces
	// containers in each pod will use.
	PodInfraContainerImage string `json:"podInfraContainerImage"` //gen
	// dockerEndpoint is the path to the docker endpoint to communicate with.
	DockerEndpoint string `json:"dockerEndpoint"` //gen
	// rootDirectory is the directory path to place kubelet files (volume
	// mounts,etc).
	RootDirectory string `json:"rootDirectory"` //gen
	// seccompProfileRoot is the directory path for seccomp profiles.
	SeccompProfileRoot string `json:"seccompProfileRoot"` //gen
	// allowPrivileged enables containers to request privileged mode.
	// Defaults to false.
	AllowPrivileged *bool `json:"allowPrivileged"` //gen
	// hostNetworkSources is a comma-separated list of sources from which the
	// Kubelet allows pods to use of host network. Defaults to "*". Valid
	// options are "file", "http", "api", and "*" (all sources).
	HostNetworkSources []string `json:"hostNetworkSources"` //gen
	// hostPIDSources is a comma-separated list of sources from which the
	// Kubelet allows pods to use the host pid namespace. Defaults to "*".
	HostPIDSources []string `json:"hostPIDSources"` //todo
	// hostIPCSources is a comma-separated list of sources from which the
	// Kubelet allows pods to use the host ipc namespace. Defaults to "*".
	HostIPCSources []string `json:"hostIPCSources"` //todo
	// registryPullQPS is the limit of registry pulls per second. If 0,
	// unlimited. Set to 0 for no limit. Defaults to 5.0.
	RegistryPullQPS *int32 `json:"registryPullQPS"` //gen
	// registryBurst is the maximum size of a bursty pulls, temporarily allows
	// pulls to burst to this number, while still not exceeding registryQps.
	// Only used if registryQPS > 0.
	RegistryBurst int32 `json:"registryBurst"` //gen
	// eventRecordQPS is the maximum event creations per second. If 0, there
	// is no limit enforced.
	EventRecordQPS *int32 `json:"eventRecordQPS"` //gen
	// eventBurst is the maximum size of a bursty event records, temporarily
	// allows event records to burst to this number, while still not exceeding
	// event-qps. Only used if eventQps > 0
	EventBurst int32 `json:"eventBurst"` //gen
	// enableDebuggingHandlers enables server endpoints for log collection
	// and local running of containers and commands
	EnableDebuggingHandlers *bool `json:"enableDebuggingHandlers"` //gen
	// minimumGCAge is the minimum age for a finished container before it is
	// garbage collected.
	MinimumGCAge metav1.Duration `json:"minimumGCAge"` //gen
	// maxPerPodContainerCount is the maximum number of old instances to
	// retain per container. Each container takes up some disk space.
	MaxPerPodContainerCount int32 `json:"maxPerPodContainerCount"` //gen
	// maxContainerCount is the maximum number of old instances of containers
	// to retain globally. Each container takes up some disk space.
	MaxContainerCount *int32 `json:"maxContainerCount"` //gen
	// cAdvisorPort is the port of the localhost cAdvisor endpoint
	CAdvisorPort int32 `json:"cAdvisorPort"` //gen - probable cross-component dep
	// healthzPort is the port of the localhost healthz endpoint
	HealthzPort int32 `json:"healthzPort"` //gen - probable cross-component dep
	// healthzBindAddress is the IP address for the healthz server to serve
	// on.
	HealthzBindAddress string `json:"healthzBindAddress"` //gen
	// oomScoreAdj is The oom-score-adj value for kubelet process. Values
	// must be within the range [-1000, 1000].
	OOMScoreAdj *int32 `json:"oomScoreAdj"` //gen
	// registerNode enables automatic registration with the apiserver.
	RegisterNode *bool `json:"registerNode"` //gen
	// clusterDomain is the DNS domain for this cluster. If set, kubelet will
	// configure all containers to search this domain in addition to the
	// host's search domains.
	ClusterDomain string `json:"clusterDomain"` //gen - probable cross-component dep
	// masterServiceNamespace is The namespace from which the kubernetes
	// master services should be injected into pods.
	MasterServiceNamespace string `json:"masterServiceNamespace"` //gen
	// clusterDNS is the IP address for a cluster DNS server.  If set, kubelet
	// will configure all containers to use this for DNS resolution in
	// addition to the host's DNS servers
	ClusterDNS string `json:"clusterDNS"` //gen - probable cross-component dep
	// streamingConnectionIdleTimeout is the maximum time a streaming connection
	// can be idle before the connection is automatically closed.
	StreamingConnectionIdleTimeout metav1.Duration `json:"streamingConnectionIdleTimeout"` //gen
	// nodeStatusUpdateFrequency is the frequency that kubelet posts node
	// status to master. Note: be cautious when changing the constant, it
	// must work with nodeMonitorGracePeriod in nodecontroller.
	NodeStatusUpdateFrequency metav1.Duration `json:"nodeStatusUpdateFrequency"` //gen - cross-component dep
	// imageMinimumGCAge is the minimum age for an unused image before it is
	// garbage collected.
	ImageMinimumGCAge metav1.Duration `json:"imageMinimumGCAge"` //gen
	// imageGCHighThresholdPercent is the percent of disk usage after which
	// image garbage collection is always run. The percent is calculated as
	// this field value out of 100.
	ImageGCHighThresholdPercent *int32 `json:"imageGCHighThresholdPercent"` //gen
	// imageGCLowThresholdPercent is the percent of disk usage before which
	// image garbage collection is never run. Lowest disk usage to garbage
	// collect to. The percent is calculated as this field value out of 100.
	ImageGCLowThresholdPercent *int32 `json:"imageGCLowThresholdPercent"` //gen
	// lowDiskSpaceThresholdMB is the absolute free disk space, in MB, to
	// maintain. When disk space falls below this threshold, new pods would
	// be rejected.
	LowDiskSpaceThresholdMB int32 `json:"lowDiskSpaceThresholdMB"` //gen
	// How frequently to calculate and cache volume disk usage for all pods
	VolumeStatsAggPeriod metav1.Duration `json:"volumeStatsAggPeriod"` //gen
	// networkPluginName is the name of the network plugin to be invoked for
	// various events in kubelet/pod lifecycle
	NetworkPluginName string `json:"networkPluginName"` //gen
	// networkPluginDir is the full path of the directory in which to search
	// for network plugins (and, for backwards-compat, CNI config files)
	NetworkPluginDir string `json:"networkPluginDir"` //gen
	// CNIConfDir is the full path of the directory in which to search for
	// CNI config files
	CNIConfDir string `json:"cniConfDir"` //gen
	// CNIBinDir is the full path of the directory in which to search for
	// CNI plugin binaries
	CNIBinDir string `json:"cniBinDir"` //gen
	// networkPluginMTU is the MTU to be passed to the network plugin,
	// and overrides the default MTU for cases where it cannot be automatically
	// computed (such as IPSEC).
	NetworkPluginMTU int32 `json:"networkPluginMTU"` //todo
	// volumePluginDir is the full path of the directory in which to search
	// for additional third party volume plugins
	VolumePluginDir string `json:"volumePluginDir"` //gen
	// cloudProvider is the provider for cloud services.
	CloudProvider string `json:"cloudProvider"` //gen
	// cloudConfigFile is the path to the cloud provider configuration file.
	CloudConfigFile string `json:"cloudConfigFile"` //gen
	// kubeletCgroups is the absolute name of cgroups to isolate the kubelet in.
	KubeletCgroups string `json:"kubeletCgroups"` //todo
	// runtimeCgroups are cgroups that container runtime is expected to be isolated in.
	RuntimeCgroups string `json:"runtimeCgroups"` //todo
	// systemCgroups is absolute name of cgroups in which to place
	// all non-kernel processes that are not already in a container. Empty
	// for no container. Rolling back the flag requires a reboot.
	SystemCgroups string `json:"systemCgroups"` //todo
	// cgroupRoot is the root cgroup to use for pods. This is handled by the
	// container runtime on a best effort basis.
	CgroupRoot string `json:"cgroupRoot"` //todo - but probably gen
	// Enable QoS based Cgroup hierarchy: top level cgroups for QoS Classes
	// And all Burstable and BestEffort pods are brought up under their
	// specific top level QoS cgroup.
	// +optional
	ExperimentalCgroupsPerQOS *bool `json:"experimentalCgroupsPerQOS,omitempty"` //gen - but this may depend on values of other possibly nogen cgroups flags
	// driver that the kubelet uses to manipulate cgroups on the host (cgroupfs or systemd)
	// +optional
	CgroupDriver string `json:"cgroupDriver,omitempty"` //gen
	// containerRuntime is the container runtime to use.
	ContainerRuntime string `json:"containerRuntime"` //gen
	// remoteRuntimeEndpoint is the endpoint of remote runtime service
	RemoteRuntimeEndpoint string `json:"remoteRuntimeEndpoint"` //todo
	// remoteImageEndpoint is the endpoint of remote image service
	RemoteImageEndpoint string `json:"remoteImageEndpoint"` //todo
	// runtimeRequestTimeout is the timeout for all runtime requests except long running
	// requests - pull, logs, exec and attach.
	RuntimeRequestTimeout metav1.Duration `json:"runtimeRequestTimeout"` //gen
	// rktPath is the  path of rkt binary. Leave empty to use the first rkt in
	// $PATH.
	RktPath string `json:"rktPath"` //gen
	// experimentalMounterPath is the path to mounter binary. If not set, kubelet will attempt to use mount
	// binary that is available via $PATH,
	ExperimentalMounterPath string `json:"experimentalMounterPath,omitempty"` //gen
	// rktApiEndpoint is the endpoint of the rkt API service to communicate with.
	RktAPIEndpoint string `json:"rktAPIEndpoint"` //gen
	// rktStage1Image is the image to use as stage1. Local paths and
	// http/https URLs are supported.
	RktStage1Image string `json:"rktStage1Image"` //gen
	// lockFilePath is the path that kubelet will use to as a lock file.
	// It uses this file as a lock to synchronize with other kubelet processes
	// that may be running.
	LockFilePath *string `json:"lockFilePath"` //gen
	// ExitOnLockContention is a flag that signifies to the kubelet that it is running
	// in "bootstrap" mode. This requires that 'LockFilePath' has been set.
	// This will cause the kubelet to listen to inotify events on the lock file,
	// releasing it and exiting when another process tries to open that file.
	ExitOnLockContention bool `json:"exitOnLockContention"` //gen
	// How should the kubelet configure the container bridge for hairpin packets.
	// Setting this flag allows endpoints in a Service to loadbalance back to
	// themselves if they should try to access their own Service. Values:
	//   "promiscuous-bridge": make the container bridge promiscuous.
	//   "hairpin-veth":       set the hairpin flag on container veth interfaces.
	//   "none":               do nothing.
	// Generally, one must set --hairpin-mode=veth-flag to achieve hairpin NAT,
	// because promiscous-bridge assumes the existence of a container bridge named cbr0.
	HairpinMode string `json:"hairpinMode"` //gen
	// The node has babysitter process monitoring docker and kubelet.
	BabysitDaemons bool `json:"babysitDaemons"` //todo - maybe gen? or should this depend on local discovery?
	// maxPods is the number of pods that can run on this Kubelet.
	MaxPods int32 `json:"maxPods"` //gen
	// nvidiaGPUs is the number of NVIDIA GPU devices on this node.
	NvidiaGPUs int32 `json:"nvidiaGPUs"` //todo - this is weird - technically it's not nogen, but it is really something that should be supported based on feature discovery...
	// dockerExecHandlerName is the handler to use when executing a command
	// in a container. Valid values are 'native' and 'nsenter'. Defaults to
	// 'native'.
	DockerExecHandlerName string `json:"dockerExecHandlerName"` //gen
	// The CIDR to use for pod IP addresses, only used in standalone mode.
	// In cluster mode, this is obtained from the master.
	PodCIDR string `json:"podCIDR"` //gen -- actually kind of irrelevant given that it's a standalone-only flag, thus forced homogeneous in cluster mode
	// ResolverConfig is the resolver configuration file used as the basis
	// for the container DNS resolution configuration."), []
	ResolverConfig string `json:"resolvConf"` //gen (todo: is this a path to a file?)
	// cpuCFSQuota is Enable CPU CFS quota enforcement for containers that
	// specify CPU limits
	CPUCFSQuota *bool `json:"cpuCFSQuota"` //gen
	// containerized should be set to true if kubelet is running in a container.
	Containerized *bool `json:"containerized"` //gen -- todo: but does anyone actually use this?
	// maxOpenFiles is Number of files that can be opened by Kubelet process.
	MaxOpenFiles int64 `json:"maxOpenFiles"` //gen
	// reconcileCIDR is Reconcile node CIDR with the CIDR specified by the
	// API server. Won't have any effect if register-node is false.
	ReconcileCIDR *bool `json:"reconcileCIDR"` //gen
	// registerSchedulable tells the kubelet to register the node as
	// schedulable. Won't have any effect if register-node is false.
	// DEPRECATED: use registerWithTaints instead
	RegisterSchedulable *bool `json:"registerSchedulable"` //gen -- also deprecated it might be time to drop this
	// registerWithTaints are an array of taints to add to a node object when
	// the kubelet registers itself. This only takes effect when registerNode
	// is true and upon the initial registration of the node.
	RegisterWithTaints []v1.Taint `json:"registerWithTaints"` //gen
	// contentType is contentType of requests sent to apiserver.
	ContentType string `json:"contentType"` //gen
	// kubeAPIQPS is the QPS to use while talking with kubernetes apiserver
	KubeAPIQPS *int32 `json:"kubeAPIQPS"` //gen
	// kubeAPIBurst is the burst to allow while talking with kubernetes
	// apiserver
	KubeAPIBurst int32 `json:"kubeAPIBurst"` //gen
	// serializeImagePulls when enabled, tells the Kubelet to pull images one
	// at a time. We recommend *not* changing the default value on nodes that
	// run docker daemon with version  < 1.9 or an Aufs storage backend.
	// Issue #10959 has more details.
	SerializeImagePulls *bool `json:"serializeImagePulls"` //gen -- note concerns that depend on docker version
	// outOfDiskTransitionFrequency is duration for which the kubelet has to
	// wait before transitioning out of out-of-disk node condition status.
	OutOfDiskTransitionFrequency metav1.Duration `json:"outOfDiskTransitionFrequency"` //gen
	// nodeIP is IP address of the node. If set, kubelet will use this IP
	// address for the node.
	NodeIP string `json:"nodeIP"` //nogen
	// nodeLabels to add when registering the node in the cluster.
	NodeLabels map[string]string `json:"nodeLabels"` //todo -- idkfs but could be gen or nogen depending on the label
	// nonMasqueradeCIDR configures masquerading: traffic to IPs outside this range will use IP masquerade.
	NonMasqueradeCIDR string `json:"nonMasqueradeCIDR"` //gen
	// enable gathering custom metrics.
	EnableCustomMetrics bool `json:"enableCustomMetrics"` //gen
	// Comma-delimited list of hard eviction expressions.  For example, 'memory.available<300Mi'.
	EvictionHard *string `json:"evictionHard"` //gen
	// Comma-delimited list of soft eviction expressions.  For example, 'memory.available<300Mi'.
	EvictionSoft string `json:"evictionSoft"` //gen
	// Comma-delimeted list of grace periods for each soft eviction signal.  For example, 'memory.available=30s'.
	EvictionSoftGracePeriod string `json:"evictionSoftGracePeriod"` //gen
	// Duration for which the kubelet has to wait before transitioning out of an eviction pressure condition.
	EvictionPressureTransitionPeriod metav1.Duration `json:"evictionPressureTransitionPeriod"` //gen
	// Maximum allowed grace period (in seconds) to use when terminating pods in response to a soft eviction threshold being met.
	EvictionMaxPodGracePeriod int32 `json:"evictionMaxPodGracePeriod"` //gen
	// Comma-delimited list of minimum reclaims (e.g. imagefs.available=2Gi) that describes the minimum amount of resource the kubelet will reclaim when performing a pod eviction if that resource is under pressure.
	EvictionMinimumReclaim string `json:"evictionMinimumReclaim"` //gen
	// Maximum number of pods per core. Cannot exceed MaxPods
	PodsPerCore int32 `json:"podsPerCore"` //gen
	// enableControllerAttachDetach enables the Attach/Detach controller to
	// manage attachment/detachment of volumes scheduled to this node, and
	// disables kubelet from executing any attach/detach operations
	EnableControllerAttachDetach *bool `json:"enableControllerAttachDetach"` //gen
	// A set of ResourceName=ResourceQuantity (e.g. cpu=200m,memory=150G) pairs
	// that describe resources reserved for non-kubernetes components.
	// Currently only cpu and memory are supported. [default=none]
	// See http://kubernetes.io/docs/user-guide/compute-resources for more detail.
	SystemReserved map[string]string `json:"systemReserved"` //gen
	// A set of ResourceName=ResourceQuantity (e.g. cpu=200m,memory=150G) pairs
	// that describe resources reserved for kubernetes system components.
	// Currently only cpu and memory are supported. [default=none]
	// See http://kubernetes.io/docs/user-guide/compute-resources for more detail.
	KubeReserved map[string]string `json:"kubeReserved"` //gen
	// Default behaviour for kernel tuning
	ProtectKernelDefaults bool `json:"protectKernelDefaults"` //gen
	// If true, Kubelet ensures a set of iptables rules are present on host.
	// These rules will serve as utility rules for various components, e.g. KubeProxy.
	// The rules will be created based on IPTablesMasqueradeBit and IPTablesDropBit.
	MakeIPTablesUtilChains *bool `json:"makeIPTablesUtilChains"` //gen
	// iptablesMasqueradeBit is the bit of the iptables fwmark space to mark for SNAT
	// Values must be within the range [0, 31]. Must be different from other mark bits.
	// Warning: Please match the value of corresponding parameter in kube-proxy
	// TODO: clean up IPTablesMasqueradeBit in kube-proxy
	IPTablesMasqueradeBit *int32 `json:"iptablesMasqueradeBit"` //nogen -- cross-component dependency
	// iptablesDropBit is the bit of the iptables fwmark space to mark for dropping packets.
	// Values must be within the range [0, 31]. Must be different from other mark bits.
	IPTablesDropBit *int32 `json:"iptablesDropBit"` //nogen -- cross-node dependency? what does it mean by "must be different from other mark bits"?
	// Whitelist of unsafe sysctls or sysctl patterns (ending in *). Use these at your own risk.
	// Resource isolation might be lacking and pod might influence each other on the same node.
	// +optional
	AllowedUnsafeSysctls []string `json:"allowedUnsafeSysctls,omitempty"` //gen
	// featureGates is a string of comma-separated key=value pairs that describe feature
	// gates for alpha/experimental features.
	FeatureGates string `json:"featureGates,omitempty"` //gen -- potential cross-component dependency (which was the point of how we implemented FeatureGates)
	// Enable Container Runtime Interface (CRI) integration.
	// +optional
	EnableCRI bool `json:"enableCRI,omitempty"` //gen
	// TODO(#34726:1.8.0): Remove the opt-in for failing when swap is enabled.
	// Tells the Kubelet to fail to start if swap is enabled on the node.
	ExperimentalFailSwapOn bool `json:"experimentalFailSwapOn,omitempty"` //gen
	// This flag, if set, enables a check prior to mount operations to verify that the required components
	// (binaries, etc.) to mount the volume are available on the underlying node. If the check is enabled
	// and fails the mount operation fails.
	ExperimentalCheckNodeCapabilitiesBeforeMount bool `json:"ExperimentalCheckNodeCapabilitiesBeforeMount,omitempty"` //gen
}

type KubeletAuthorizationMode string

const (
	// KubeletAuthorizationModeAlwaysAllow authorizes all authenticated requests
	KubeletAuthorizationModeAlwaysAllow KubeletAuthorizationMode = "AlwaysAllow"
	// KubeletAuthorizationModeWebhook uses the SubjectAccessReview API to determine authorization
	KubeletAuthorizationModeWebhook KubeletAuthorizationMode = "Webhook"
)

type KubeletAuthorization struct {
	// mode is the authorization mode to apply to requests to the kubelet server.
	// Valid values are AlwaysAllow and Webhook.
	// Webhook mode uses the SubjectAccessReview API to determine authorization.
	Mode KubeletAuthorizationMode `json:"mode"`

	// webhook contains settings related to Webhook authorization.
	Webhook KubeletWebhookAuthorization `json:"webhook"`
}

type KubeletWebhookAuthorization struct {
	// cacheAuthorizedTTL is the duration to cache 'authorized' responses from the webhook authorizer.
	CacheAuthorizedTTL metav1.Duration `json:"cacheAuthorizedTTL"`
	// cacheUnauthorizedTTL is the duration to cache 'unauthorized' responses from the webhook authorizer.
	CacheUnauthorizedTTL metav1.Duration `json:"cacheUnauthorizedTTL"`
}

type KubeletAuthentication struct {
	// x509 contains settings related to x509 client certificate authentication
	X509 KubeletX509Authentication `json:"x509"`
	// webhook contains settings related to webhook bearer token authentication
	Webhook KubeletWebhookAuthentication `json:"webhook"`
	// anonymous contains settings related to anonymous authentication
	Anonymous KubeletAnonymousAuthentication `json:"anonymous"`
}

type KubeletX509Authentication struct {
	// clientCAFile is the path to a PEM-encoded certificate bundle. If set, any request presenting a client certificate
	// signed by one of the authorities in the bundle is authenticated with a username corresponding to the CommonName,
	// and groups corresponding to the Organization in the client certificate.
	ClientCAFile string `json:"clientCAFile"`
}

type KubeletWebhookAuthentication struct {
	// enabled allows bearer token authentication backed by the tokenreviews.authentication.k8s.io API
	Enabled *bool `json:"enabled"`
	// cacheTTL enables caching of authentication results
	CacheTTL metav1.Duration `json:"cacheTTL"`
}

type KubeletAnonymousAuthentication struct {
	// enabled allows anonymous requests to the kubelet server.
	// Requests that are not rejected by another authentication method are treated as anonymous requests.
	// Anonymous requests have a username of system:anonymous, and a group name of system:unauthenticated.
	Enabled *bool `json:"enabled"`
}
