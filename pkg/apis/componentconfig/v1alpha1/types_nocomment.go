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

// Old:
type KubeProxyConfiguration struct {
	metav1.TypeMeta

	BindAddress                    string          `json:"bindAddress"` //gen
	ClusterCIDR                    string          `json:"clusterCIDR"` //gen
	HealthzBindAddress             string          `json:"healthzBindAddress"`
	HealthzPort                    int32           `json:"healthzPort"`                    //gen
	HostnameOverride               string          `json:"hostnameOverride"`               //nogen//can/is this taken from KubeletConfig?
	IPTablesMasqueradeBit          *int32          `json:"iptablesMasqueradeBit"`          //todo
	IPTablesSyncPeriod             metav1.Duration `json:"iptablesSyncPeriodSeconds"`      //gen
	IPTablesMinSyncPeriod          metav1.Duration `json:"iptablesMinSyncPeriodSeconds"`   //gen
	KubeconfigPath                 string          `json:"kubeconfigPath"`                 //gen
	MasqueradeAll                  bool            `json:"masqueradeAll"`                  //gen
	Master                         string          `json:"master"`                         //gen
	OOMScoreAdj                    *int32          `json:"oomScoreAdj"`                    //gen
	Mode                           ProxyMode       `json:"mode"`                           //gen
	PortRange                      string          `json:"portRange"`                      //gen
	ResourceContainer              string          `json:"resourceContainer"`              //gen
	UDPIdleTimeout                 metav1.Duration `json:"udpTimeoutMilliseconds"`         //gen
	ConntrackMax                   int32           `json:"conntrackMax"`                   //gen
	ConntrackMaxPerCore            int32           `json:"conntrackMaxPerCore"`            //gen
	ConntrackMin                   int32           `json:"conntrackMin"`                   //gen
	ConntrackTCPEstablishedTimeout metav1.Duration `json:"conntrackTCPEstablishedTimeout"` //gen
	ConntrackTCPCloseWaitTimeout   metav1.Duration `json:"conntrackTCPCloseWaitTimeout"`   //gen
}

//---
// New from #34727:
// ClientConnectionConfiguration contains details for constructing a client.
type ClientConnectionConfiguration struct { //gen
	// KubeConfigFile is the path to a kubeconfig file.
	KubeConfigFile string `json:"kubeconfig"` //gen
	// AcceptContentTypes defines the Accept header sent by clients when connecting to a server, overriding the
	// default value of 'application/json'. This field will control all connections to the server used by a particular
	// client.
	AcceptContentTypes string `json:"acceptContentTypes"` //gen
	// ContentType is the content type used when sending data to the server from this client.
	ContentType string `json:"contentType"` //gen
	// QPS controls the number of queries per second allowed for this connection.
	QPS float32 `json:"qps"` //gen
	// Burst allows extra queries to accumulate when a client is exceeding its rate.
	Burst int32 `json:"burst"` //gen
}

// KubeProxyIPTablesConfiguration contains iptables-related configuration
// details for the Kubernetes proxy server.
type KubeProxyIPTablesConfiguration struct { //todo
	// masqueradeBit is the bit of the iptables fwmark space to use for SNAT if using
	// the pure iptables proxy mode. Values must be within the range [0, 31].
	MasqueradeBit *int32 `json:"masqueradeBit"` //todo I need to learn more about masquerade
	// masqueradeAll tells kube-proxy to SNAT everything if using the pure iptables proxy mode.
	MasqueradeAll bool `json:"masqueradeAll"` //gen
	// syncPeriod is the period that iptables rules are refreshed (e.g. '5s', '1m',
	// '2h22m').  Must be greater than 0.
	SyncPeriod unversioned.Duration `json:"syncPeriod"` //gen
}

// KubeProxyConntrackConfiguration contains conntrack settings for
// the Kubernetes proxy server.
type KubeProxyConntrackConfiguration struct { //gen
	// max is the maximum number of NAT connections to track (0 to
	// leave as-is).  This takes precedence over conntrackMaxPerCore and conntrackMin.
	Max int32 `json:"max"` //gen
	// maxPerCore is the maximum number of NAT connections to track
	// per CPU core (0 to leave the limit as-is and ignore conntrackMin).
	MaxPerCore int32 `json:"maxPerCore"` //gen
	// min is the minimum value of connect-tracking records to allocate,
	// regardless of conntrackMaxPerCore (set conntrackMaxPerCore=0 to leave the limit as-is).
	Min int32 `json:"min"` //gen
	// tcpEstablishedTimeout is how long an idle TCP connection will be kept open
	// (e.g. '250ms', '2s').  Must be greater than 0.
	TCPEstablishedTimeout unversioned.Duration `json:"tcpEstablishedTimeout"` //gen
}

// KubeProxyConfiguration contains everything necessary to configure the
// Kubernetes proxy server.
type KubeProxyConfiguration struct {
	unversioned.TypeMeta

	// featureGates is a comma-separated list of key=value pairs that control
	// which alpha/beta features are enabled.
	//
	// TODO this really should be a map but that requires refactoring all
	// components to use config files because local-up-cluster.sh only supports
	// the --feature-gates flag right now, which is comma-separated key=value
	// pairs.
	FeatureGates string `json:"featureGates"` //gen
	// bindAddress is the IP address for the proxy server to serve on (set to 0.0.0.0
	// for all interfaces)
	BindAddress string `json:"bindAddress"` //gen
	// healthzBindAddress is the IP address and port for the health check server to serve on,
	// defaulting to 127.0.0.1:10249 (set to 0.0.0.0 for all interfaces)
	HealthzBindAddress string `json:"healthzBindAddress"` //gen
	// clusterCIDR is the CIDR range of the pods in the cluster. It is used to
	// bridge traffic coming from outside of the cluster. If not provided,
	// no off-cluster bridging will be performed.
	ClusterCIDR string `json:"clusterCIDR"` //gen
	// hostnameOverride, if non-empty, will be used as the identity instead of the actual hostname.
	HostnameOverride string `json:"hostnameOverride"` //nogen can this be taken from KubeletConfig?
	// clientConnection specifies the kubeconfig file and client connection
	// settings for the proxy server to use when communicating with the apiserver.
	ClientConnection ClientConnectionConfiguration `json:"clientConnection"` //gen
	// iptables contains iptables-related configuration options.
	IPTables KubeProxyIPTablesConfiguration `json:"iptables"` //todo - depends on decision wrt masquerade bits
	// oomScoreAdj is the oom-score-adj value for kube-proxy process. Values must be within
	// the range [-1000, 1000]
	OOMScoreAdj *int32 `json:"oomScoreAdj"` //gen
	// mode specifies which proxy mode to use: 'userspace' (older) or 'iptables'
	// (faster). If blank, look at the Node object on the Kubernetes API and
	// respect the '"+ExperimentalProxyModeAnnotation+"' annotation if provided.
	// Otherwise use the best-available proxy (currently iptables). If the
	// iptables proxy is selected, regardless of how, but the system's kernel or
	// iptables versions are insufficient, this always falls back to the userspace
	// proxy.
	Mode ProxyMode `json:"mode"` //gen
	// portRange is the range of host ports (beginPort-endPort, inclusive) that may be consumed
	// in order to proxy service traffic. If unspecified (0-0) then ports will be randomly chosen.
	PortRange string `json:"portRange"` //gen
	// resourceContainer is the absolute name of the resource-only container to create and run
	// the Kube-proxy in (Default: /kube-proxy).
	ResourceContainer string `json:"resourceContainer"` //gen?
	// udpIdleTimeout is how long an idle UDP connection will be kept open (e.g. '250ms', '2s').
	// Must be greater than 0. Only applicable for proxyMode=userspace.
	UDPIdleTimeout unversioned.Duration `json:"udpTimeoutMilliseconds"` //gen
	// conntrack contains conntrack-related configuration options.
	Conntrack KubeProxyConntrackConfiguration `json:"conntrack"` //gen
	// configSyncPeriod is how often configuration from the apiserver is
	// refreshed.  Must be greater than 0.
	ConfigSyncPeriod unversioned.Duration `json:"configSyncPeriod"` //gen
}

//---

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

	Port                           int                         `json:"port"`                           //gen
	Address                        string                      `json:"address"`                        //todo but I think gen
	AlgorithmProvider              string                      `json:"algorithmProvider"`              //gen
	PolicyConfigFile               string                      `json:"policyConfigFile"`               //gen
	EnableProfiling                *bool                       `json:"enableProfiling"`                //gen
	EnableContentionProfiling      bool                        `json:"enableContentionProfiling"`      //gen
	ContentType                    string                      `json:"contentType"`                    //gen
	KubeAPIQPS                     float32                     `json:"kubeAPIQPS"`                     //gen
	KubeAPIBurst                   int                         `json:"kubeAPIBurst"`                   //gen
	SchedulerName                  string                      `json:"schedulerName"`                  //nogen
	HardPodAffinitySymmetricWeight int                         `json:"hardPodAffinitySymmetricWeight"` //gen
	FailureDomains                 string                      `json:"failureDomains"`
	LeaderElection                 LeaderElectionConfiguration `json:"leaderElection"` //gen
}

// HairpinMode denotes how the kubelet should configure networking to handle
// hairpin packets.
type HairpinMode string

// Enum settings for different ways to handle hairpin packets.
const (
	HairpinVeth       = "hairpin-veth"
	PromiscuousBridge = "promiscuous-bridge"
	HairpinNone       = "none"
)

// LeaderElectionConfiguration defines the configuration of leader election
// clients for components that can run with leader election enabled.
type LeaderElectionConfiguration struct {
	LeaderElect   *bool           `json:"leaderElect"`   //gen
	LeaseDuration metav1.Duration `json:"leaseDuration"` //gen
	RenewDeadline metav1.Duration `json:"renewDeadline"` //gen
	RetryPeriod   metav1.Duration `json:"retryPeriod"`   //gen
}

type KubeletConfiguration struct {
	metav1.TypeMeta

	PodManifestPath                              string                `json:"podManifestPath"`                                        //gen
	SyncFrequency                                metav1.Duration       `json:"syncFrequency"`                                          //gen
	FileCheckFrequency                           metav1.Duration       `json:"fileCheckFrequency"`                                     //gen
	HTTPCheckFrequency                           metav1.Duration       `json:"httpCheckFrequency"`                                     //gen
	ManifestURL                                  string                `json:"manifestURL"`                                            //todo
	ManifestURLHeader                            string                `json:"manifestURLHeader"`                                      //gen
	EnableServer                                 *bool                 `json:"enableServer"`                                           //gen
	Address                                      string                `json:"address"`                                                //gen
	Port                                         int32                 `json:"port"`                                                   //gen
	ReadOnlyPort                                 int32                 `json:"readOnlyPort"`                                           //gen
	TLSCertFile                                  string                `json:"tlsCertFile"`                                            //gen
	TLSPrivateKeyFile                            string                `json:"tlsPrivateKeyFile"`                                      //gen
	CertDirectory                                string                `json:"certDirectory"`                                          //gen
	Authentication                               KubeletAuthentication `json:"authentication"`                                         //todo
	Authorization                                KubeletAuthorization  `json:"authorization"`                                          //todo
	HostnameOverride                             string                `json:"hostnameOverride"`                                       //nogen
	PodInfraContainerImage                       string                `json:"podInfraContainerImage"`                                 //gen
	DockerEndpoint                               string                `json:"dockerEndpoint"`                                         //gen
	RootDirectory                                string                `json:"rootDirectory"`                                          //gen
	SeccompProfileRoot                           string                `json:"seccompProfileRoot"`                                     //gen
	AllowPrivileged                              *bool                 `json:"allowPrivileged"`                                        //gen
	HostNetworkSources                           []string              `json:"hostNetworkSources"`                                     //gen
	HostPIDSources                               []string              `json:"hostPIDSources"`                                         //todo
	HostIPCSources                               []string              `json:"hostIPCSources"`                                         //todo
	RegistryPullQPS                              *int32                `json:"registryPullQPS"`                                        //gen
	RegistryBurst                                int32                 `json:"registryBurst"`                                          //gen
	EventRecordQPS                               *int32                `json:"eventRecordQPS"`                                         //gen
	EventBurst                                   int32                 `json:"eventBurst"`                                             //gen
	EnableDebuggingHandlers                      *bool                 `json:"enableDebuggingHandlers"`                                //gen
	MinimumGCAge                                 metav1.Duration       `json:"minimumGCAge"`                                           //gen
	MaxPerPodContainerCount                      int32                 `json:"maxPerPodContainerCount"`                                //gen
	MaxContainerCount                            *int32                `json:"maxContainerCount"`                                      //gen
	CAdvisorPort                                 int32                 `json:"cAdvisorPort"`                                           //gen - probable cross-component dep
	HealthzPort                                  int32                 `json:"healthzPort"`                                            //gen - probable cross-component dep
	HealthzBindAddress                           string                `json:"healthzBindAddress"`                                     //gen
	OOMScoreAdj                                  *int32                `json:"oomScoreAdj"`                                            //gen
	RegisterNode                                 *bool                 `json:"registerNode"`                                           //gen
	ClusterDomain                                string                `json:"clusterDomain"`                                          //gen - probable cross-component dep
	MasterServiceNamespace                       string                `json:"masterServiceNamespace"`                                 //gen
	ClusterDNS                                   string                `json:"clusterDNS"`                                             //gen - probable cross-component dep
	StreamingConnectionIdleTimeout               metav1.Duration       `json:"streamingConnectionIdleTimeout"`                         //gen
	NodeStatusUpdateFrequency                    metav1.Duration       `json:"nodeStatusUpdateFrequency"`                              //gen - cross-component dep
	ImageMinimumGCAge                            metav1.Duration       `json:"imageMinimumGCAge"`                                      //gen
	ImageGCHighThresholdPercent                  *int32                `json:"imageGCHighThresholdPercent"`                            //gen
	ImageGCLowThresholdPercent                   *int32                `json:"imageGCLowThresholdPercent"`                             //gen
	LowDiskSpaceThresholdMB                      int32                 `json:"lowDiskSpaceThresholdMB"`                                //gen
	VolumeStatsAggPeriod                         metav1.Duration       `json:"volumeStatsAggPeriod"`                                   //gen
	NetworkPluginName                            string                `json:"networkPluginName"`                                      //gen
	NetworkPluginDir                             string                `json:"networkPluginDir"`                                       //gen
	CNIConfDir                                   string                `json:"cniConfDir"`                                             //gen
	CNIBinDir                                    string                `json:"cniBinDir"`                                              //gen
	NetworkPluginMTU                             int32                 `json:"networkPluginMTU"`                                       //todo
	VolumePluginDir                              string                `json:"volumePluginDir"`                                        //gen
	CloudProvider                                string                `json:"cloudProvider"`                                          //gen
	CloudConfigFile                              string                `json:"cloudConfigFile"`                                        //gen
	KubeletCgroups                               string                `json:"kubeletCgroups"`                                         //todo
	RuntimeCgroups                               string                `json:"runtimeCgroups"`                                         //todo
	SystemCgroups                                string                `json:"systemCgroups"`                                          //todo
	CgroupRoot                                   string                `json:"cgroupRoot"`                                             //todo - but probably gen
	ExperimentalCgroupsPerQOS                    *bool                 `json:"experimentalCgroupsPerQOS,omitempty"`                    //gen - but this may depend on values of other possibly nogen cgroups flags
	CgroupDriver                                 string                `json:"cgroupDriver,omitempty"`                                 //gen
	ContainerRuntime                             string                `json:"containerRuntime"`                                       //gen
	RemoteRuntimeEndpoint                        string                `json:"remoteRuntimeEndpoint"`                                  //todo
	RemoteImageEndpoint                          string                `json:"remoteImageEndpoint"`                                    //todo
	RuntimeRequestTimeout                        metav1.Duration       `json:"runtimeRequestTimeout"`                                  //gen
	RktPath                                      string                `json:"rktPath"`                                                //gen
	ExperimentalMounterPath                      string                `json:"experimentalMounterPath,omitempty"`                      //gen
	RktAPIEndpoint                               string                `json:"rktAPIEndpoint"`                                         //gen
	RktStage1Image                               string                `json:"rktStage1Image"`                                         //gen
	LockFilePath                                 *string               `json:"lockFilePath"`                                           //gen
	ExitOnLockContention                         bool                  `json:"exitOnLockContention"`                                   //gen
	HairpinMode                                  string                `json:"hairpinMode"`                                            //gen
	BabysitDaemons                               bool                  `json:"babysitDaemons"`                                         //todo - maybe gen? or should this depend on local discovery?
	MaxPods                                      int32                 `json:"maxPods"`                                                //gen
	NvidiaGPUs                                   int32                 `json:"nvidiaGPUs"`                                             //todo - this is weird - technically it's not nogen, but it is really something that should be supported based on feature discovery...
	DockerExecHandlerName                        string                `json:"dockerExecHandlerName"`                                  //gen
	PodCIDR                                      string                `json:"podCIDR"`                                                //gen -- actually kind of irrelevant given that it's a standalone-only flag, thus forced homogeneous in cluster mode
	ResolverConfig                               string                `json:"resolvConf"`                                             //gen (todo: is this a path to a file?)
	CPUCFSQuota                                  *bool                 `json:"cpuCFSQuota"`                                            //gen
	Containerized                                *bool                 `json:"containerized"`                                          //gen -- todo: but does anyone actually use this?
	MaxOpenFiles                                 int64                 `json:"maxOpenFiles"`                                           //gen
	ReconcileCIDR                                *bool                 `json:"reconcileCIDR"`                                          //gen
	RegisterSchedulable                          *bool                 `json:"registerSchedulable"`                                    //gen -- also deprecated it might be time to drop this
	RegisterWithTaints                           []v1.Taint            `json:"registerWithTaints"`                                     //gen
	ContentType                                  string                `json:"contentType"`                                            //gen
	KubeAPIQPS                                   *int32                `json:"kubeAPIQPS"`                                             //gen
	KubeAPIBurst                                 int32                 `json:"kubeAPIBurst"`                                           //gen
	SerializeImagePulls                          *bool                 `json:"serializeImagePulls"`                                    //gen -- note concerns that depend on docker version
	OutOfDiskTransitionFrequency                 metav1.Duration       `json:"outOfDiskTransitionFrequency"`                           //gen
	NodeIP                                       string                `json:"nodeIP"`                                                 //nogen
	NodeLabels                                   map[string]string     `json:"nodeLabels"`                                             //todo -- idkfs but could be gen or nogen depending on the label
	NonMasqueradeCIDR                            string                `json:"nonMasqueradeCIDR"`                                      //gen
	EnableCustomMetrics                          bool                  `json:"enableCustomMetrics"`                                    //gen
	EvictionHard                                 *string               `json:"evictionHard"`                                           //gen
	EvictionSoft                                 string                `json:"evictionSoft"`                                           //gen
	EvictionSoftGracePeriod                      string                `json:"evictionSoftGracePeriod"`                                //gen
	EvictionPressureTransitionPeriod             metav1.Duration       `json:"evictionPressureTransitionPeriod"`                       //gen
	EvictionMaxPodGracePeriod                    int32                 `json:"evictionMaxPodGracePeriod"`                              //gen
	EvictionMinimumReclaim                       string                `json:"evictionMinimumReclaim"`                                 //gen
	PodsPerCore                                  int32                 `json:"podsPerCore"`                                            //gen
	EnableControllerAttachDetach                 *bool                 `json:"enableControllerAttachDetach"`                           //gen
	SystemReserved                               map[string]string     `json:"systemReserved"`                                         //gen
	KubeReserved                                 map[string]string     `json:"kubeReserved"`                                           //gen
	ProtectKernelDefaults                        bool                  `json:"protectKernelDefaults"`                                  //gen
	MakeIPTablesUtilChains                       *bool                 `json:"makeIPTablesUtilChains"`                                 //gen
	IPTablesMasqueradeBit                        *int32                `json:"iptablesMasqueradeBit"`                                  //nogen -- cross-component dependency
	IPTablesDropBit                              *int32                `json:"iptablesDropBit"`                                        //nogen -- cross-node dependency? what does it mean by "must be different from other mark bits"?
	AllowedUnsafeSysctls                         []string              `json:"allowedUnsafeSysctls,omitempty"`                         //gen
	FeatureGates                                 string                `json:"featureGates,omitempty"`                                 //gen -- potential cross-component dependency (which was the point of how we implemented FeatureGates)
	EnableCRI                                    bool                  `json:"enableCRI,omitempty"`                                    //gen
	ExperimentalFailSwapOn                       bool                  `json:"experimentalFailSwapOn,omitempty"`                       //gen
	ExperimentalCheckNodeCapabilitiesBeforeMount bool                  `json:"ExperimentalCheckNodeCapabilitiesBeforeMount,omitempty"` //gen
}

type KubeletAuthorizationMode string

const (
	KubeletAuthorizationModeAlwaysAllow KubeletAuthorizationMode = "AlwaysAllow"
	KubeletAuthorizationModeWebhook     KubeletAuthorizationMode = "Webhook"
)

type KubeletAuthorization struct {
	Mode KubeletAuthorizationMode `json:"mode"`

	Webhook KubeletWebhookAuthorization `json:"webhook"`
}

type KubeletWebhookAuthorization struct {
	CacheAuthorizedTTL   metav1.Duration `json:"cacheAuthorizedTTL"`
	CacheUnauthorizedTTL metav1.Duration `json:"cacheUnauthorizedTTL"`
}

type KubeletAuthentication struct {
	X509      KubeletX509Authentication      `json:"x509"`
	Webhook   KubeletWebhookAuthentication   `json:"webhook"`
	Anonymous KubeletAnonymousAuthentication `json:"anonymous"`
}

type KubeletX509Authentication struct {
	ClientCAFile string `json:"clientCAFile"`
}

type KubeletWebhookAuthentication struct {
	Enabled  *bool           `json:"enabled"`
	CacheTTL metav1.Duration `json:"cacheTTL"`
}

type KubeletAnonymousAuthentication struct {
	Enabled *bool `json:"enabled"`
}
