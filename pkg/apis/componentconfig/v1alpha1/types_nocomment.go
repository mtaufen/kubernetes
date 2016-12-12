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
	KubeConfigFile     string  `json:"kubeconfig"`         //gen
	AcceptContentTypes string  `json:"acceptContentTypes"` //gen
	ContentType        string  `json:"contentType"`        //gen
	QPS                float32 `json:"qps"`                //gen
	Burst              int32   `json:"burst"`              //gen
}

// KubeProxyIPTablesConfiguration contains iptables-related configuration
// details for the Kubernetes proxy server.
type KubeProxyIPTablesConfiguration struct { //todo
	MasqueradeBit *int32               `json:"masqueradeBit"` //todo I need to learn more about masquerade
	MasqueradeAll bool                 `json:"masqueradeAll"` //gen
	SyncPeriod    unversioned.Duration `json:"syncPeriod"`    //gen
}

// KubeProxyConntrackConfiguration contains conntrack settings for
// the Kubernetes proxy server.
type KubeProxyConntrackConfiguration struct { //gen
	Max                   int32                `json:"max"`                   //gen
	MaxPerCore            int32                `json:"maxPerCore"`            //gen
	Min                   int32                `json:"min"`                   //gen
	TCPEstablishedTimeout unversioned.Duration `json:"tcpEstablishedTimeout"` //gen
}

// KubeProxyConfiguration contains everything necessary to configure the
// Kubernetes proxy server.
type KubeProxyConfiguration struct {
	unversioned.TypeMeta

	FeatureGates       string                          `json:"featureGates"`           //gen
	BindAddress        string                          `json:"bindAddress"`            //gen
	HealthzBindAddress string                          `json:"healthzBindAddress"`     //gen
	ClusterCIDR        string                          `json:"clusterCIDR"`            //gen
	HostnameOverride   string                          `json:"hostnameOverride"`       //nogen can this be taken from KubeletConfig?
	ClientConnection   ClientConnectionConfiguration   `json:"clientConnection"`       //gen
	IPTables           KubeProxyIPTablesConfiguration  `json:"iptables"`               //todo - depends on decision wrt masquerade bits
	OOMScoreAdj        *int32                          `json:"oomScoreAdj"`            //gen
	Mode               ProxyMode                       `json:"mode"`                   //gen
	PortRange          string                          `json:"portRange"`              //gen
	ResourceContainer  string                          `json:"resourceContainer"`      //gen?
	UDPIdleTimeout     unversioned.Duration            `json:"udpTimeoutMilliseconds"` //gen
	Conntrack          KubeProxyConntrackConfiguration `json:"conntrack"`              //gen
	ConfigSyncPeriod   unversioned.Duration            `json:"configSyncPeriod"`       //gen
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
	Authentication                               KubeletAuthentication `json:"authentication"`                                         //gen
	Authorization                                KubeletAuthorization  `json:"authorization"`                                          //gen
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

type KubeletAuthorization struct { //gen
	Mode KubeletAuthorizationMode `json:"mode"` //gen

	Webhook KubeletWebhookAuthorization `json:"webhook"` //gen
}

type KubeletWebhookAuthorization struct { //gen
	CacheAuthorizedTTL   metav1.Duration `json:"cacheAuthorizedTTL"`   //gen
	CacheUnauthorizedTTL metav1.Duration `json:"cacheUnauthorizedTTL"` //gen
}

type KubeletAuthentication struct { //gen
	X509      KubeletX509Authentication      `json:"x509"`      //gen
	Webhook   KubeletWebhookAuthentication   `json:"webhook"`   //gen
	Anonymous KubeletAnonymousAuthentication `json:"anonymous"` //gen
}

type KubeletX509Authentication struct { //gen
	ClientCAFile string `json:"clientCAFile"` //gen
}

type KubeletWebhookAuthentication struct { //gen
	Enabled  *bool           `json:"enabled"`  //gen
	CacheTTL metav1.Duration `json:"cacheTTL"` //gen
}

type KubeletAnonymousAuthentication struct { //gen
	Enabled *bool `json:"enabled"` //gen
}
