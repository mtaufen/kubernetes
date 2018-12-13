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

// Package options contains all of the primary arguments for a kubelet.
package options

import (
	"fmt"
	_ "net/http/pprof"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/pflag"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/apiserver/pkg/util/flag"
	"k8s.io/klog"
	"k8s.io/kubelet/config/v1beta1"
	"k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/features"
	kubeletapis "k8s.io/kubernetes/pkg/kubelet/apis"
	kubeletconfig "k8s.io/kubernetes/pkg/kubelet/apis/config"
	kubeletscheme "k8s.io/kubernetes/pkg/kubelet/apis/config/scheme"
	kubeletconfigvalidation "k8s.io/kubernetes/pkg/kubelet/apis/config/validation"
	"k8s.io/kubernetes/pkg/kubelet/config"
	kubetypes "k8s.io/kubernetes/pkg/kubelet/types"
	"k8s.io/kubernetes/pkg/master/ports"
	utilflag "k8s.io/kubernetes/pkg/util/flag"
	utiltaints "k8s.io/kubernetes/pkg/util/taints"
)

const defaultRootDir = "/var/lib/kubelet"

// A configuration field should go in KubeletFlags instead of KubeletConfiguration if any of these are true:
// - its value will never, or cannot safely be changed during the lifetime of a node
// - its value cannot be safely shared between nodes at the same time (e.g. a hostname)
//   KubeletConfiguration is intended to be shared between nodes
// In general, please try to avoid adding flags or configuration fields,
// we already have a confusingly large amount of them.
type KubeletFlags struct {
	KubeConfig          string
	BootstrapKubeconfig string

	// Insert a probability of random errors during calls to the master.
	ChaosChance float64
	// Crash immediately, rather than eating panics.
	ReallyCrashForTesting bool

	// TODO(mtaufen): It is increasingly looking like nobody actually uses the
	//                Kubelet's runonce mode anymore, so it may be a candidate
	//                for deprecation and removal.
	// If runOnce is true, the Kubelet will check the API server once for pods,
	// run those in addition to the pods specified by static pod files, and exit.
	RunOnce bool

	// enableServer enables the Kubelet's server
	EnableServer bool

	// HostnameOverride is the hostname used to identify the kubelet instead
	// of the actual hostname.
	HostnameOverride string
	// NodeIP is IP address of the node.
	// If set, kubelet will use this IP address for the node.
	NodeIP string

	// This flag, if set, sets the unique id of the instance that an external provider (i.e. cloudprovider)
	// can use to identify a specific node
	ProviderID string

	// Container-runtime-specific options.
	config.ContainerRuntimeOptions

	// certDirectory is the directory where the TLS certs are located (by
	// default /var/run/kubernetes). If tlsCertFile and tlsPrivateKeyFile
	// are provided, this flag will be ignored.
	CertDirectory string

	// cloudProvider is the provider for cloud services.
	// +optional
	CloudProvider string

	// cloudConfigFile is the path to the cloud provider configuration file.
	// +optional
	CloudConfigFile string

	// rootDirectory is the directory path to place kubelet files (volume
	// mounts,etc).
	RootDirectory string

	// The Kubelet will use this directory for checkpointing downloaded configurations and tracking configuration health.
	// The Kubelet will create this directory if it does not already exist.
	// The path may be absolute or relative; relative paths are under the Kubelet's current working directory.
	// Providing this flag enables dynamic kubelet configuration.
	// To use this flag, the DynamicKubeletConfig feature gate must be enabled.
	DynamicConfigDir flag.StringFlag

	// The Kubelet will load its initial configuration from this file.
	// The path may be absolute or relative; relative paths are under the Kubelet's current working directory.
	// Omit this flag to use the combination of built-in default configuration values and flags.
	KubeletConfigFile string

	// registerNode enables automatic registration with the apiserver.
	RegisterNode bool

	// registerWithTaints are an array of taints to add to a node object when
	// the kubelet registers itself. This only takes effect when registerNode
	// is true and upon the initial registration of the node.
	RegisterWithTaints []core.Taint

	// WindowsService should be set to true if kubelet is running as a service on Windows.
	// Its corresponding flag only gets registered in Windows builds.
	WindowsService bool

	// EXPERIMENTAL FLAGS
	// Whitelist of unsafe sysctls or sysctl patterns (ending in *).
	// +optional
	AllowedUnsafeSysctls []string
	// containerized should be set to true if kubelet is running in a container.
	Containerized bool
	// remoteRuntimeEndpoint is the endpoint of remote runtime service
	RemoteRuntimeEndpoint string
	// remoteImageEndpoint is the endpoint of remote image service
	RemoteImageEndpoint string
	// experimentalMounterPath is the path of mounter binary. Leave empty to use the default mount path
	ExperimentalMounterPath string
	// If enabled, the kubelet will integrate with the kernel memcg notification to determine if memory eviction thresholds are crossed rather than polling.
	// +optional
	ExperimentalKernelMemcgNotification bool
	// This flag, if set, enables a check prior to mount operations to verify that the required components
	// (binaries, etc.) to mount the volume are available on the underlying node. If the check is enabled
	// and fails the mount operation fails.
	ExperimentalCheckNodeCapabilitiesBeforeMount bool
	// This flag, if set, will avoid including `EvictionHard` limits while computing Node Allocatable.
	// Refer to [Node Allocatable](https://git.k8s.io/community/contributors/design-proposals/node-allocatable.md) doc for more information.
	ExperimentalNodeAllocatableIgnoreEvictionThreshold bool
	// Node Labels are the node labels to add when registering the node in the cluster
	NodeLabels map[string]string
	// volumePluginDir is the full path of the directory in which to search
	// for additional third party volume plugins
	VolumePluginDir string
	// lockFilePath is the path that kubelet will use to as a lock file.
	// It uses this file as a lock to synchronize with other kubelet processes
	// that may be running.
	LockFilePath string
	// ExitOnLockContention is a flag that signifies to the kubelet that it is running
	// in "bootstrap" mode. This requires that 'LockFilePath' has been set.
	// This will cause the kubelet to listen to inotify events on the lock file,
	// releasing it and exiting when another process tries to open that file.
	ExitOnLockContention bool
	// seccompProfileRoot is the directory path for seccomp profiles.
	SeccompProfileRoot string
	// bootstrapCheckpointPath is the path to the directory containing pod checkpoints to
	// run on restore
	BootstrapCheckpointPath string
	// NodeStatusMaxImages caps the number of images reported in Node.Status.Images.
	// This is an experimental, short-term flag to help with node scalability.
	NodeStatusMaxImages int32

	// DEPRECATED FLAGS
	// minimumGCAge is the minimum age for a finished container before it is
	// garbage collected.
	MinimumGCAge metav1.Duration
	// maxPerPodContainerCount is the maximum number of old instances to
	// retain per container. Each container takes up some disk space.
	MaxPerPodContainerCount int32
	// maxContainerCount is the maximum number of old instances of containers
	// to retain globally. Each container takes up some disk space.
	MaxContainerCount int32
	// masterServiceNamespace is The namespace from which the kubernetes
	// master services should be injected into pods.
	MasterServiceNamespace string
	// registerSchedulable tells the kubelet to register the node as
	// schedulable. Won't have any effect if register-node is false.
	// DEPRECATED: use registerWithTaints instead
	RegisterSchedulable bool
	// nonMasqueradeCIDR configures masquerading: traffic to IPs outside this range will use IP masquerade.
	NonMasqueradeCIDR string
	// This flag, if set, instructs the kubelet to keep volumes from terminated pods mounted to the node.
	// This can be useful for debugging volume related issues.
	KeepTerminatedPodVolumes bool
	// allowPrivileged enables containers to request privileged mode.
	// Defaults to true.
	AllowPrivileged bool
	// hostNetworkSources is a comma-separated list of sources from which the
	// Kubelet allows pods to use of host network. Defaults to "*". Valid
	// options are "file", "http", "api", and "*" (all sources).
	HostNetworkSources []string
	// hostPIDSources is a comma-separated list of sources from which the
	// Kubelet allows pods to use the host pid namespace. Defaults to "*".
	HostPIDSources []string
	// hostIPCSources is a comma-separated list of sources from which the
	// Kubelet allows pods to use the host ipc namespace. Defaults to "*".
	HostIPCSources []string
}

// NewKubeletFlags will create a new KubeletFlags with default values
func NewKubeletFlags() *KubeletFlags {
	remoteRuntimeEndpoint := ""
	if runtime.GOOS == "linux" {
		remoteRuntimeEndpoint = "unix:///var/run/dockershim.sock"
	} else if runtime.GOOS == "windows" {
		remoteRuntimeEndpoint = "npipe:////./pipe/dockershim"
	}

	return &KubeletFlags{
		EnableServer:                        true,
		ContainerRuntimeOptions:             *NewContainerRuntimeOptions(),
		CertDirectory:                       "/var/lib/kubelet/pki",
		RootDirectory:                       defaultRootDir,
		MasterServiceNamespace:              metav1.NamespaceDefault,
		MaxContainerCount:                   -1,
		MaxPerPodContainerCount:             1,
		MinimumGCAge:                        metav1.Duration{Duration: 0},
		NonMasqueradeCIDR:                   "10.0.0.0/8",
		RegisterSchedulable:                 true,
		ExperimentalKernelMemcgNotification: false,
		RemoteRuntimeEndpoint:               remoteRuntimeEndpoint,
		NodeLabels:                          make(map[string]string),
		VolumePluginDir:                     "/usr/libexec/kubernetes/kubelet-plugins/volume/exec/",
		RegisterNode:                        true,
		SeccompProfileRoot:                  filepath.Join(defaultRootDir, "seccomp"),
		HostNetworkSources:                  []string{kubetypes.AllSource},
		HostPIDSources:                      []string{kubetypes.AllSource},
		HostIPCSources:                      []string{kubetypes.AllSource},
		// TODO(#58010:v1.13.0): Remove --allow-privileged, it is deprecated
		AllowPrivileged: true,
		// prior to the introduction of this flag, there was a hardcoded cap of 50 images
		NodeStatusMaxImages: 50,
	}
}

func ValidateKubeletFlags(f *KubeletFlags) error {
	// ensure that nobody sets DynamicConfigDir if the dynamic config feature gate is turned off
	if f.DynamicConfigDir.Provided() && !utilfeature.DefaultFeatureGate.Enabled(features.DynamicKubeletConfig) {
		return fmt.Errorf("the DynamicKubeletConfig feature gate must be enabled in order to use the --dynamic-config-dir flag")
	}
	if f.NodeStatusMaxImages < -1 {
		return fmt.Errorf("invalid configuration: NodeStatusMaxImages (--node-status-max-images) must be -1 or greater")
	}

	unknownLabels := sets.NewString()
	for k := range f.NodeLabels {
		if isKubernetesLabel(k) && !kubeletapis.IsKubeletLabel(k) {
			unknownLabels.Insert(k)
		}
	}
	if len(unknownLabels) > 0 {
		// TODO(liggitt): in 1.15, return an error
		klog.Warningf("unknown 'kubernetes.io' or 'k8s.io' labels specified with --node-labels: %v", unknownLabels.List())
		klog.Warningf("in 1.15, --node-labels in the 'kubernetes.io' namespace must begin with an allowed prefix (%s) or be in the specifically allowed set (%s)", strings.Join(kubeletapis.KubeletLabelNamespaces(), ", "), strings.Join(kubeletapis.KubeletLabels(), ", "))
	}

	return nil
}

func isKubernetesLabel(key string) bool {
	namespace := getLabelNamespace(key)
	if namespace == "kubernetes.io" || strings.HasSuffix(namespace, ".kubernetes.io") {
		return true
	}
	if namespace == "k8s.io" || strings.HasSuffix(namespace, ".k8s.io") {
		return true
	}
	return false
}

func getLabelNamespace(key string) string {
	if parts := strings.SplitN(key, "/", 2); len(parts) == 2 {
		return parts[0]
	}
	return ""
}

// NewKubeletConfiguration will create a new KubeletConfiguration with default values
func NewKubeletConfiguration() (*kubeletconfig.KubeletConfiguration, error) {
	scheme, _, err := kubeletscheme.NewSchemeAndCodecs()
	if err != nil {
		panic(err.Error())
	}
	versioned := &v1beta1.KubeletConfiguration{}
	scheme.Default(versioned)
	config := &kubeletconfig.KubeletConfiguration{}
	if err := scheme.Convert(versioned, config, nil); err != nil {
		panic(err.Error())
	}
	applyLegacyDefaults(config)
	return config
}

// applyLegacyDefaults applies legacy default values to the KubeletConfiguration in order to
// preserve the command line API. This is used to construct the baseline default KubeletConfiguration
// before the first round of flag parsing.
func applyLegacyDefaults(kc *kubeletconfig.KubeletConfiguration) {
	// --anonymous-auth
	kc.Authentication.Anonymous.Enabled = true
	// --authentication-token-webhook
	kc.Authentication.Webhook.Enabled = false
	// --authorization-mode
	kc.Authorization.Mode = kubeletconfig.KubeletAuthorizationModeAlwaysAllow
	// --read-only-port
	kc.ReadOnlyPort = ports.KubeletReadOnlyPort
}

// KubeletServer encapsulates all of the parameters necessary for starting up
// a kubelet. These can either be set via command line or directly.
type KubeletServer struct {
	KubeletFlags
	kubeletconfig.KubeletConfiguration
}

// NewKubeletServer will create a new KubeletServer with default values.
func NewKubeletServer() (*KubeletServer, error) {
	config, err := NewKubeletConfiguration()
	if err != nil {
		return nil, err
	}
	return &KubeletServer{
		KubeletFlags:         *NewKubeletFlags(),
		KubeletConfiguration: *config,
	}, nil
}

// validateKubeletServer validates configuration of KubeletServer and returns an error if the input configuration is invalid
func ValidateKubeletServer(s *KubeletServer) error {
	// please add any KubeletConfiguration validation to the kubeletconfigvalidation.ValidateKubeletConfiguration function
	if err := kubeletconfigvalidation.ValidateKubeletConfiguration(&s.KubeletConfiguration); err != nil {
		return err
	}
	if err := ValidateKubeletFlags(&s.KubeletFlags); err != nil {
		return err
	}
	return nil
}

// AddFlags adds flags for a specific KubeletServer to the specified FlagSet
func (s *KubeletServer) AddFlags(fs *pflag.FlagSet) {
	s.KubeletFlags.AddFlags(fs)
	AddKubeletConfigFlags(fs, &s.KubeletConfiguration)
}

// AddFlags adds flags for a specific KubeletFlags to the specified FlagSet
func (f *KubeletFlags) AddFlags(mainfs *pflag.FlagSet) {
	fs := pflag.NewFlagSet("", pflag.ExitOnError)
	defer func() {
		// Unhide deprecated flags. We want deprecated flags to show in Kubelet help.
		// We have some hidden flags, but we might as well unhide these when they are deprecated,
		// as silently deprecating and removing (even hidden) things is unkind to people who use them.
		fs.VisitAll(func(f *pflag.Flag) {
			if len(f.Deprecated) > 0 {
				f.Hidden = false
			}
		})
		mainfs.AddFlagSet(fs)
	}()

	f.ContainerRuntimeOptions.AddFlags(fs)
	f.addOSFlags(fs)

	fs.StringVar(&f.KubeletConfigFile, "config", f.KubeletConfigFile, "The Kubelet will load its initial configuration from this file. The path may be absolute or relative; relative paths start at the Kubelet's current working directory. Omit this flag to use the built-in default configuration values. Command-line flags override configuration from this file.")
	fs.StringVar(&f.KubeConfig, "kubeconfig", f.KubeConfig, "Path to a kubeconfig file, specifying how to connect to the API server. Providing --kubeconfig enables API server mode, omitting --kubeconfig enables standalone mode.")

	fs.StringVar(&f.BootstrapKubeconfig, "bootstrap-kubeconfig", f.BootstrapKubeconfig, "Path to a kubeconfig file that will be used to get client certificate for kubelet. "+
		"If the file specified by --kubeconfig does not exist, the bootstrap kubeconfig is used to request a client certificate from the API server. "+
		"On success, a kubeconfig file referencing the generated client certificate and key is written to the path specified by --kubeconfig. "+
		"The client certificate and key file will be stored in the directory pointed by --cert-dir.")

	fs.BoolVar(&f.ReallyCrashForTesting, "really-crash-for-testing", f.ReallyCrashForTesting, "If true, when panics occur crash. Intended for testing.")
	fs.Float64Var(&f.ChaosChance, "chaos-chance", f.ChaosChance, "If > 0.0, introduce random client errors and latency. Intended for testing.")

	fs.BoolVar(&f.RunOnce, "runonce", f.RunOnce, "If true, exit after spawning pods from static pod files or remote urls. Exclusive with --enable-server")
	fs.BoolVar(&f.EnableServer, "enable-server", f.EnableServer, "Enable the Kubelet's server")

	fs.StringVar(&f.HostnameOverride, "hostname-override", f.HostnameOverride, "If non-empty, will use this string as identification instead of the actual hostname. If --cloud-provider is set, the cloud provider determines the name of the node (consult cloud provider documentation to determine if and how the hostname is used).")

	fs.StringVar(&f.NodeIP, "node-ip", f.NodeIP, "IP address of the node. If set, kubelet will use this IP address for the node")

	fs.StringVar(&f.ProviderID, "provider-id", f.ProviderID, "Unique identifier for identifying the node in a machine database, i.e cloudprovider")

	fs.StringVar(&f.CertDirectory, "cert-dir", f.CertDirectory, "The directory where the TLS certs are located. "+
		"If --tls-cert-file and --tls-private-key-file are provided, this flag will be ignored.")

	fs.StringVar(&f.CloudProvider, "cloud-provider", f.CloudProvider, "The provider for cloud services. Specify empty string for running with no cloud provider. If set, the cloud provider determines the name of the node (consult cloud provider documentation to determine if and how the hostname is used).")
	fs.StringVar(&f.CloudConfigFile, "cloud-config", f.CloudConfigFile, "The path to the cloud provider configuration file.  Empty string for no configuration file.")

	fs.StringVar(&f.RootDirectory, "root-dir", f.RootDirectory, "Directory path for managing kubelet files (volume mounts,etc).")

	fs.Var(&f.DynamicConfigDir, "dynamic-config-dir", "The Kubelet will use this directory for checkpointing downloaded configurations and tracking configuration health. The Kubelet will create this directory if it does not already exist. The path may be absolute or relative; relative paths start at the Kubelet's current working directory. Providing this flag enables dynamic Kubelet configuration. The DynamicKubeletConfig feature gate must be enabled to pass this flag; this gate currently defaults to true because the feature is beta.")

	fs.BoolVar(&f.RegisterNode, "register-node", f.RegisterNode, "Register the node with the apiserver. If --kubeconfig is not provided, this flag is irrelevant, as the Kubelet won't have an apiserver to register with.")
	fs.Var(utiltaints.NewTaintsVar(&f.RegisterWithTaints), "register-with-taints", "Register the node with the given list of taints (comma separated \"<key>=<value>:<effect>\"). No-op if register-node is false.")
	fs.BoolVar(&f.Containerized, "containerized", f.Containerized, "Running kubelet in a container.")

	// EXPERIMENTAL FLAGS
	fs.StringVar(&f.ExperimentalMounterPath, "experimental-mounter-path", f.ExperimentalMounterPath, "[Experimental] Path of mounter binary. Leave empty to use the default mount.")
	fs.StringSliceVar(&f.AllowedUnsafeSysctls, "allowed-unsafe-sysctls", f.AllowedUnsafeSysctls, "Comma-separated whitelist of unsafe sysctls or unsafe sysctl patterns (ending in *). Use these at your own risk. Sysctls feature gate is enabled by default.")
	fs.BoolVar(&f.ExperimentalKernelMemcgNotification, "experimental-kernel-memcg-notification", f.ExperimentalKernelMemcgNotification, "If enabled, the kubelet will integrate with the kernel memcg notification to determine if memory eviction thresholds are crossed rather than polling.")
	fs.StringVar(&f.RemoteRuntimeEndpoint, "container-runtime-endpoint", f.RemoteRuntimeEndpoint, "[Experimental] The endpoint of remote runtime service. Currently unix socket and tcp endpoints are supported on Linux, while npipe and tcp endpoints are supported on windows.  Examples:'unix:///var/run/dockershim.sock', 'npipe:////./pipe/dockershim'")
	fs.StringVar(&f.RemoteImageEndpoint, "image-service-endpoint", f.RemoteImageEndpoint, "[Experimental] The endpoint of remote image service. If not specified, it will be the same with container-runtime-endpoint by default. Currently unix socket and tcp endpoints are supported on Linux, while npipe and tcp endpoints are supported on windows.  Examples:'unix:///var/run/dockershim.sock', 'npipe:////./pipe/dockershim'")
	fs.BoolVar(&f.ExperimentalCheckNodeCapabilitiesBeforeMount, "experimental-check-node-capabilities-before-mount", f.ExperimentalCheckNodeCapabilitiesBeforeMount, "[Experimental] if set true, the kubelet will check the underlying node for required components (binaries, etc.) before performing the mount")
	fs.BoolVar(&f.ExperimentalNodeAllocatableIgnoreEvictionThreshold, "experimental-allocatable-ignore-eviction", f.ExperimentalNodeAllocatableIgnoreEvictionThreshold, "When set to 'true', Hard Eviction Thresholds will be ignored while calculating Node Allocatable. See https://kubernetes.io/docs/tasks/administer-cluster/reserve-compute-resources/ for more details. [default=false]")
	bindableNodeLabels := flag.ConfigurationMap(f.NodeLabels)
	fs.Var(&bindableNodeLabels, "node-labels", fmt.Sprintf("<Warning: Alpha feature> Labels to add when registering the node in the cluster.  Labels must be key=value pairs separated by ','. Labels in the 'kubernetes.io' namespace must begin with an allowed prefix (%s) or be in the specifically allowed set (%s)", strings.Join(kubeletapis.KubeletLabelNamespaces(), ", "), strings.Join(kubeletapis.KubeletLabels(), ", ")))
	fs.StringVar(&f.VolumePluginDir, "volume-plugin-dir", f.VolumePluginDir, "The full path of the directory in which to search for additional third party volume plugins")
	fs.StringVar(&f.LockFilePath, "lock-file", f.LockFilePath, "<Warning: Alpha feature> The path to file for kubelet to use as a lock file.")
	fs.BoolVar(&f.ExitOnLockContention, "exit-on-lock-contention", f.ExitOnLockContention, "Whether kubelet should exit upon lock-file contention.")
	fs.StringVar(&f.SeccompProfileRoot, "seccomp-profile-root", f.SeccompProfileRoot, "<Warning: Alpha feature> Directory path for seccomp profiles.")
	fs.StringVar(&f.BootstrapCheckpointPath, "bootstrap-checkpoint-path", f.BootstrapCheckpointPath, "<Warning: Alpha feature> Path to the directory where the checkpoints are stored")
	fs.Int32Var(&f.NodeStatusMaxImages, "node-status-max-images", f.NodeStatusMaxImages, "<Warning: Alpha feature> The maximum number of images to report in Node.Status.Images. If -1 is specified, no cap will be applied.")

	// DEPRECATED FLAGS
	fs.StringVar(&f.BootstrapKubeconfig, "experimental-bootstrap-kubeconfig", f.BootstrapKubeconfig, "")
	fs.MarkDeprecated("experimental-bootstrap-kubeconfig", "Use --bootstrap-kubeconfig")
	fs.DurationVar(&f.MinimumGCAge.Duration, "minimum-container-ttl-duration", f.MinimumGCAge.Duration, "Minimum age for a finished container before it is garbage collected.  Examples: '300ms', '10s' or '2h45m'")
	fs.MarkDeprecated("minimum-container-ttl-duration", "Use --eviction-hard or --eviction-soft instead. Will be removed in a future version.")
	fs.Int32Var(&f.MaxPerPodContainerCount, "maximum-dead-containers-per-container", f.MaxPerPodContainerCount, "Maximum number of old instances to retain per container.  Each container takes up some disk space.")
	fs.MarkDeprecated("maximum-dead-containers-per-container", "Use --eviction-hard or --eviction-soft instead. Will be removed in a future version.")
	fs.Int32Var(&f.MaxContainerCount, "maximum-dead-containers", f.MaxContainerCount, "Maximum number of old instances of containers to retain globally.  Each container takes up some disk space. To disable, set to a negative number.")
	fs.MarkDeprecated("maximum-dead-containers", "Use --eviction-hard or --eviction-soft instead. Will be removed in a future version.")
	fs.StringVar(&f.MasterServiceNamespace, "master-service-namespace", f.MasterServiceNamespace, "The namespace from which the kubernetes master services should be injected into pods")
	fs.MarkDeprecated("master-service-namespace", "This flag will be removed in a future version.")
	fs.BoolVar(&f.RegisterSchedulable, "register-schedulable", f.RegisterSchedulable, "Register the node as schedulable. Won't have any effect if register-node is false.")
	fs.MarkDeprecated("register-schedulable", "will be removed in a future version")
	fs.StringVar(&f.NonMasqueradeCIDR, "non-masquerade-cidr", f.NonMasqueradeCIDR, "Traffic to IPs outside this range will use IP masquerade. Set to '0.0.0.0/0' to never masquerade.")
	fs.MarkDeprecated("non-masquerade-cidr", "will be removed in a future version")
	fs.BoolVar(&f.KeepTerminatedPodVolumes, "keep-terminated-pod-volumes", f.KeepTerminatedPodVolumes, "Keep terminated pod volumes mounted to the node after the pod terminates.  Can be useful for debugging volume related issues.")
	fs.MarkDeprecated("keep-terminated-pod-volumes", "will be removed in a future version")
	// TODO(#58010:v1.13.0): Remove --allow-privileged, it is deprecated
	fs.BoolVar(&f.AllowPrivileged, "allow-privileged", f.AllowPrivileged, "If true, allow containers to request privileged mode. Default: true")
	fs.MarkDeprecated("allow-privileged", "will be removed in a future version")
	// TODO(#58010:v1.12.0): Remove --host-network-sources, it is deprecated
	fs.StringSliceVar(&f.HostNetworkSources, "host-network-sources", f.HostNetworkSources, "Comma-separated list of sources from which the Kubelet allows pods to use of host network.")
	fs.MarkDeprecated("host-network-sources", "will be removed in a future version")
	// TODO(#58010:v1.12.0): Remove --host-pid-sources, it is deprecated
	fs.StringSliceVar(&f.HostPIDSources, "host-pid-sources", f.HostPIDSources, "Comma-separated list of sources from which the Kubelet allows pods to use the host pid namespace.")
	fs.MarkDeprecated("host-pid-sources", "will be removed in a future version")
	// TODO(#58010:v1.12.0): Remove --host-ipc-sources, it is deprecated
	fs.StringSliceVar(&f.HostIPCSources, "host-ipc-sources", f.HostIPCSources, "Comma-separated list of sources from which the Kubelet allows pods to use the host ipc namespace.")
	fs.MarkDeprecated("host-ipc-sources", "will be removed in a future version")

}

// AddKubeletConfigFlags adds flags for a specific kubeletconfig.KubeletConfiguration to the specified FlagSet
func AddKubeletConfigFlags(mainfs *pflag.FlagSet) func(c *Config) {
	// tmp is a default kubeletconfig.KubeletConfiguration, which we will parse the flags into
	tmp := NewKubeletConfiguration()
	fs := pflag.NewFlagSet("", pflag.ExitOnError)
	fns := map[string]func(c *kubeletconfig.KubeletConfiguration){}

	fs.BoolVar(&tmp.FailSwapOn, "fail-swap-on", tmp.FailSwapOn, "Makes the Kubelet fail to start if swap is enabled on the node. ")
	fns["fail-swap-on"] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.BoolVar(&tmp.FailSwapOn, "experimental-fail-swap-on", tmp.FailSwapOn, "DEPRECATED: please use --fail-swap-on instead.")
	fns["experimental-fail-swap-on"] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.StaticPodPath, "pod-manifest-path", tmp.StaticPodPath, "Path to the directory containing static pod files to run, or the path to a single static pod file. Files starting with dots will be ignored.")
	fns["pod-manifest-path"] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.DurationVar(&tmp.SyncFrequency.Duration, "sync-frequency", tmp.SyncFrequency.Duration, "Max period between synchronizing running containers and config")
	fns["sync-frequency"] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.DurationVar(&tmp.FileCheckFrequency.Duration, "file-check-frequency", tmp.FileCheckFrequency.Duration, "Duration between checking config files for new data")
	fns["file-check-frequency"] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.DurationVar(&tmp.HTTPCheckFrequency.Duration, "http-check-frequency", tmp.HTTPCheckFrequency.Duration, "Duration between checking http for new data")
	fns["http-check-frequency"] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.StaticPodURL, "manifest-url", tmp.StaticPodURL, "URL for accessing additional Pod specifications to run")
	fns["manifest-url"] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Var(flag.NewColonSeparatedMultimapStringString(&tmp.StaticPodURLHeader), "manifest-url-header", "Comma-separated list of HTTP headers to use when accessing the url provided to --manifest-url. Multiple headers with the same name will be added in the same order provided. This flag can be repeatedly invoked. For example: `--manifest-url-header 'a:hello,b:again,c:world' --manifest-url-header 'b:beautiful'`")
	fns["manifest-url-header"] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Var(utilflag.IPVar{Val: &tmp.Address}, "address", "The IP address for the Kubelet to serve on (set to `0.0.0.0` for all IPv4 interfaces and `::` for all IPv6 interfaces)")
	fns["address"] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int32Var(&tmp.Port, "port", tmp.Port, "The port for the Kubelet to serve on.")
	fns["port"] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int32Var(&tmp.ReadOnlyPort, "read-only-port", tmp.ReadOnlyPort, "The read-only port for the Kubelet to serve on with no authentication/authorization (set to 0 to disable)")
	fns["read-only-port"] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	// Authentication
	fs.BoolVar(&tmp.Authentication.Anonymous.Enabled, "anonymous-auth", tmp.Authentication.Anonymous.Enabled, ""+
		"Enables anonymous requests to the Kubelet server. Requests that are not rejected by another "+
		"authentication method are treated as anonymous requests. Anonymous requests have a username "+
		"of system:anonymous, and a group name of system:unauthenticated.")
	fns["anonymous-auth"] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.BoolVar(&tmp.Authentication.Webhook.Enabled, "authentication-token-webhook", tmp.Authentication.Webhook.Enabled, ""+
		"Use the TokenReview API to determine authentication for bearer tokens.")
	fns["authentication-token-webhook"] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.DurationVar(&tmp.Authentication.Webhook.CacheTTL.Duration, "authentication-token-webhook-cache-ttl", tmp.Authentication.Webhook.CacheTTL.Duration, ""+
		"The duration to cache responses from the webhook token authenticator.")
	fns["authentication-token-webhook-cache-ttl"] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.Authentication.X509.ClientCAFile, "client-ca-file", tmp.Authentication.X509.ClientCAFile, ""+
		"If set, any request presenting a client certificate signed by one of the authorities in the client-ca-file "+
		"is authenticated with an identity corresponding to the CommonName of the client certificate.")
	fns["client-ca-file"] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	// Authorization
	fs.StringVar((*string)(&tmp.Authorization.Mode), "authorization-mode", string(c.Authorization.Mode), ""+
		"Authorization mode for Kubelet server. Valid options are AlwaysAllow or Webhook. "+
		"Webhook mode uses the SubjectAccessReview API to determine authorization.")
	fns["authorization-mode"] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.DurationVar(&tmp.Authorization.Webhook.CacheAuthorizedTTL.Duration, "authorization-webhook-cache-authorized-ttl", tmp.Authorization.Webhook.CacheAuthorizedTTL.Duration, ""+
		"The duration to cache 'authorized' responses from the webhook authorizer.")
	fns["authorization-webhook-cache-authorized-ttl"] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.DurationVar(&tmp.Authorization.Webhook.CacheUnauthorizedTTL.Duration, "authorization-webhook-cache-unauthorized-ttl", tmp.Authorization.Webhook.CacheUnauthorizedTTL.Duration, ""+
		"The duration to cache 'unauthorized' responses from the webhook authorizer.")
	fns["authorization-webhook-cache-unauthorized-ttl"] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.TLSCertFile, "tls-cert-file", tmp.TLSCertFile, ""+
		"File containing x509 Certificate used for serving HTTPS (with intermediate certs, if any, concatenated after server cert). "+
		"If --tls-cert-file and --tls-private-key-file are not provided, a self-signed certificate and key "+
		"are generated for the public address and saved to the directory passed to --cert-dir.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.TLSPrivateKeyFile, "tls-private-key-file", tmp.TLSPrivateKeyFile, "File containing x509 private key matching --tls-cert-file.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.BoolVar(&tmp.ServerTLSBootstrap, "rotate-server-certificates", tmp.ServerTLSBootstrap, "Auto-request and rotate the kubelet serving certificates by requesting new certificates from the kube-apiserver when the certificate expiration approaches. Requires the RotateKubeletServerCertificate feature gate to be enabled, and approval of the submitted CertificateSigningRequest objects.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	tlsCipherPossibleValues := flag.TLSCipherPossibleValues()
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringSliceVar(&tmp.TLSCipherSuites, "tls-cipher-suites", tmp.TLSCipherSuites,
		"Comma-separated list of cipher suites for the server. "+
			"If omitted, the default Go cipher suites will be used. "+
			"Possible values: "+strings.Join(tlsCipherPossibleValues, ","))
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	tlsPossibleVersions := flag.TLSPossibleVersions()
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.TLSMinVersion, "tls-min-version", tmp.TLSMinVersion,
		"Minimum TLS version supported. "+
			"Possible values: "+strings.Join(tlsPossibleVersions, ", "))
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.BoolVar(&tmp.RotateCertificates, "rotate-certificates", tmp.RotateCertificates, "<Warning: Beta feature> Auto rotate the kubelet client certificates by requesting new certificates from the kube-apiserver when the certificate expiration approaches.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int32Var(&tmp.RegistryPullQPS, "registry-qps", tmp.RegistryPullQPS, "If > 0, limit registry pull QPS to this value.  If 0, unlimited.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int32Var(&tmp.RegistryBurst, "registry-burst", tmp.RegistryBurst, "Maximum size of a bursty pulls, temporarily allows pulls to burst to this number, while still not exceeding registry-qps. Only used if --registry-qps > 0")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int32Var(&tmp.EventRecordQPS, "event-qps", tmp.EventRecordQPS, "If > 0, limit event creations per second to this value. If 0, unlimited.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int32Var(&tmp.EventBurst, "event-burst", tmp.EventBurst, "Maximum size of a bursty event records, temporarily allows event records to burst to this number, while still not exceeding event-qps. Only used if --event-qps > 0")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.BoolVar(&tmp.EnableDebuggingHandlers, "enable-debugging-handlers", tmp.EnableDebuggingHandlers, "Enables server endpoints for log collection and local running of containers and commands")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.BoolVar(&tmp.EnableContentionProfiling, "contention-profiling", tmp.EnableContentionProfiling, "Enable lock contention profiling, if profiling is enabled")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int32Var(&tmp.HealthzPort, "healthz-port", tmp.HealthzPort, "The port of the localhost healthz endpoint (set to 0 to disable)")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Var(utilflag.IPVar{Val: &tmp.HealthzBindAddress}, "healthz-bind-address", "The IP address for the healthz server to serve on (set to `0.0.0.0` for all IPv4 interfaces and `::` for all IPv6 interfaces)")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int32Var(&tmp.OOMScoreAdj, "oom-score-adj", tmp.OOMScoreAdj, "The oom-score-adj value for kubelet process. Values must be within the range [-1000, 1000]")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.ClusterDomain, "cluster-domain", tmp.ClusterDomain, "Domain for this cluster.  If set, kubelet will configure all containers to search this domain in addition to the host's search domains")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringSliceVar(&tmp.ClusterDNS, "cluster-dns", tmp.ClusterDNS, "Comma-separated list of DNS server IP address.  This value is used for containers DNS server in case of Pods with \"dnsPolicy=ClusterFirst\". Note: all DNS servers appearing in the list MUST serve the same set of records otherwise name resolution within the cluster may not work correctly. There is no guarantee as to which DNS server may be contacted for name resolution.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.DurationVar(&tmp.StreamingConnectionIdleTimeout.Duration, "streaming-connection-idle-timeout", tmp.StreamingConnectionIdleTimeout.Duration, "Maximum time a streaming connection can be idle before the connection is automatically closed. 0 indicates no timeout. Example: '5m'")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.DurationVar(&tmp.NodeStatusUpdateFrequency.Duration, "node-status-update-frequency", tmp.NodeStatusUpdateFrequency.Duration, "Specifies how often kubelet posts node status to master. Note: be cautious when changing the constant, it must work with nodeMonitorGracePeriod in nodecontroller.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.DurationVar(&tmp.ImageMinimumGCAge.Duration, "minimum-image-ttl-duration", tmp.ImageMinimumGCAge.Duration, "Minimum age for an unused image before it is garbage collected.  Examples: '300ms', '10s' or '2h45m'.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int32Var(&tmp.ImageGCHighThresholdPercent, "image-gc-high-threshold", tmp.ImageGCHighThresholdPercent, "The percent of disk usage after which image garbage collection is always run. Values must be within the range [0, 100], To disable image garbage collection, set to 100. ")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int32Var(&tmp.ImageGCLowThresholdPercent, "image-gc-low-threshold", tmp.ImageGCLowThresholdPercent, "The percent of disk usage before which image garbage collection is never run. Lowest disk usage to garbage collect to. Values must be within the range [0, 100] and should not be larger than that of --image-gc-high-threshold.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.DurationVar(&tmp.VolumeStatsAggPeriod.Duration, "volume-stats-agg-period", tmp.VolumeStatsAggPeriod.Duration, "Specifies interval for kubelet to calculate and cache the volume disk usage for all pods and volumes.  To disable volume calculations, set to 0.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Var(flag.NewMapStringBool(&tmp.FeatureGates), "feature-gates", "A set of key=value pairs that describe feature gates for alpha/experimental features. "+
		"Options are:\n"+strings.Join(utilfeature.DefaultFeatureGate.KnownFeatures(), "\n"))
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.KubeletCgroups, "kubelet-cgroups", tmp.KubeletCgroups, "Optional absolute name of cgroups to create and run the Kubelet in.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.SystemCgroups, "system-cgroups", tmp.SystemCgroups, "Optional absolute name of cgroups in which to place all non-kernel processes that are not already inside a cgroup under `/`. Empty for no container. Rolling back the flag requires a reboot.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.BoolVar(&tmp.CgroupsPerQOS, "cgroups-per-qos", tmp.CgroupsPerQOS, "Enable creation of QoS cgroup hierarchy, if true top level QoS and pod cgroups are created.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.CgroupDriver, "cgroup-driver", tmp.CgroupDriver, "Driver that the kubelet uses to manipulate cgroups on the host.  Possible values: 'cgroupfs', 'systemd'")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.CgroupRoot, "cgroup-root", tmp.CgroupRoot, "Optional root cgroup to use for pods. This is handled by the container runtime on a best effort basis. Default: '', which means use the container runtime default.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.CPUManagerPolicy, "cpu-manager-policy", tmp.CPUManagerPolicy, "CPU Manager policy to use. Possible values: 'none', 'static'. Default: 'none'")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.DurationVar(&tmp.CPUManagerReconcilePeriod.Duration, "cpu-manager-reconcile-period", tmp.CPUManagerReconcilePeriod.Duration, "<Warning: Alpha feature> CPU Manager reconciliation period. Examples: '10s', or '1m'. If not supplied, defaults to `NodeStatusUpdateFrequency`")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Var(flag.NewMapStringString(&tmp.QOSReserved), "qos-reserved", "<Warning: Alpha feature> A set of ResourceName=Percentage (e.g. memory=50%) pairs that describe how pod resource requests are reserved at the QoS level. Currently only memory is supported. Requires the QOSReserved feature gate to be enabled.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.DurationVar(&tmp.RuntimeRequestTimeout.Duration, "runtime-request-timeout", tmp.RuntimeRequestTimeout.Duration, "Timeout of all runtime requests except long running request - pull, logs, exec and attach. When timeout exceeded, kubelet will cancel the request, throw out an error and retry later.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.HairpinMode, "hairpin-mode", tmp.HairpinMode, "How should the kubelet setup hairpin NAT. This allows endpoints of a Service to loadbalance back to themselves if they should try to access their own Service. Valid values are \"promiscuous-bridge\", \"hairpin-veth\" and \"none\".")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int32Var(&tmp.MaxPods, "max-pods", tmp.MaxPods, "Number of Pods that can run on this Kubelet.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.PodCIDR, "pod-cidr", tmp.PodCIDR, "The CIDR to use for pod IP addresses, only used in standalone mode.  In cluster mode, this is obtained from the master. For IPv6, the maximum number of IP's allocated is 65536")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int64Var(&tmp.PodPidsLimit, "pod-max-pids", tmp.PodPidsLimit, "<Warning: Alpha feature> Set the maximum number of processes per pod.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.ResolverConfig, "resolv-conf", tmp.ResolverConfig, "Resolver configuration file used as the basis for the container DNS resolution configuration.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.BoolVar(&tmp.CPUCFSQuota, "cpu-cfs-quota", tmp.CPUCFSQuota, "Enable CPU CFS quota enforcement for containers that specify CPU limits")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.DurationVar(&tmp.CPUCFSQuotaPeriod.Duration, "cpu-cfs-quota-period", tmp.CPUCFSQuotaPeriod.Duration, "Sets CPU CFS quota period value, cpu.cfs_period_us, defaults to Linux Kernel default")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.BoolVar(&tmp.EnableControllerAttachDetach, "enable-controller-attach-detach", tmp.EnableControllerAttachDetach, "Enables the Attach/Detach controller to manage attachment/detachment of volumes scheduled to this node, and disables kubelet from executing any attach/detach operations")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.BoolVar(&tmp.MakeIPTablesUtilChains, "make-iptables-util-chains", tmp.MakeIPTablesUtilChains, "If true, kubelet will ensure iptables utility rules are present on host.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int32Var(&tmp.IPTablesMasqueradeBit, "iptables-masquerade-bit", tmp.IPTablesMasqueradeBit, "The bit of the fwmark space to mark packets for SNAT. Must be within the range [0, 31]. Please match this parameter with corresponding parameter in kube-proxy.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int32Var(&tmp.IPTablesDropBit, "iptables-drop-bit", tmp.IPTablesDropBit, "The bit of the fwmark space to mark packets for dropping. Must be within the range [0, 31].")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.ContainerLogMaxSize, "container-log-max-size", tmp.ContainerLogMaxSize, "<Warning: Beta feature> Set the maximum size (e.g. 10Mi) of container log file before it is rotated. This flag can only be used with --container-runtime=remote.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int32Var(&tmp.ContainerLogMaxFiles, "container-log-max-files", tmp.ContainerLogMaxFiles, "<Warning: Beta feature> Set the maximum number of container log files that can be present for a container. The number must be >= 2. This flag can only be used with --container-runtime=remote.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	// Flags intended for testing, not recommended used in production environments.
	fs.Int64Var(&tmp.MaxOpenFiles, "max-open-files", tmp.MaxOpenFiles, "Number of files that can be opened by Kubelet process.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.ContentType, "kube-api-content-type", tmp.ContentType, "Content type of requests sent to apiserver.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int32Var(&tmp.KubeAPIQPS, "kube-api-qps", tmp.KubeAPIQPS, "QPS to use while talking with kubernetes apiserver")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int32Var(&tmp.KubeAPIBurst, "kube-api-burst", tmp.KubeAPIBurst, "Burst to use while talking with kubernetes apiserver")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.BoolVar(&tmp.SerializeImagePulls, "serialize-image-pulls", tmp.SerializeImagePulls, "Pull images one at a time. We recommend *not* changing the default value on nodes that run docker daemon with version < 1.9 or an Aufs storage backend. Issue #10959 has more details.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Var(flag.NewLangleSeparatedMapStringString(&tmp.EvictionHard), "eviction-hard", "A set of eviction thresholds (e.g. memory.available<1Gi) that if met would trigger a pod eviction.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Var(flag.NewLangleSeparatedMapStringString(&tmp.EvictionSoft), "eviction-soft", "A set of eviction thresholds (e.g. memory.available<1.5Gi) that if met over a corresponding grace period would trigger a pod eviction.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Var(flag.NewMapStringString(&tmp.EvictionSoftGracePeriod), "eviction-soft-grace-period", "A set of eviction grace periods (e.g. memory.available=1m30s) that correspond to how long a soft eviction threshold must hold before triggering a pod eviction.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.DurationVar(&tmp.EvictionPressureTransitionPeriod.Duration, "eviction-pressure-transition-period", tmp.EvictionPressureTransitionPeriod.Duration, "Duration for which the kubelet has to wait before transitioning out of an eviction pressure condition.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int32Var(&tmp.EvictionMaxPodGracePeriod, "eviction-max-pod-grace-period", tmp.EvictionMaxPodGracePeriod, "Maximum allowed grace period (in seconds) to use when terminating pods in response to a soft eviction threshold being met.  If negative, defer to pod specified value.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Var(flag.NewMapStringString(&tmp.EvictionMinimumReclaim), "eviction-minimum-reclaim", "A set of minimum reclaims (e.g. imagefs.available=2Gi) that describes the minimum amount of resource the kubelet will reclaim when performing a pod eviction if that resource is under pressure.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Int32Var(&tmp.PodsPerCore, "pods-per-core", tmp.PodsPerCore, "Number of Pods per core that can run on this Kubelet. The total number of Pods on this Kubelet cannot exceed max-pods, so max-pods will be used if this calculation results in a larger number of Pods allowed on the Kubelet. A value of 0 disables this limit.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.BoolVar(&tmp.ProtectKernelDefaults, "protect-kernel-defaults", tmp.ProtectKernelDefaults, "Default kubelet behaviour for kernel tuning. If set, kubelet errors if any of kernel tunables is different than kubelet defaults.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	// Node Allocatable Flags
	fs.Var(flag.NewMapStringString(&tmp.SystemReserved), "system-reserved", "A set of ResourceName=ResourceQuantity (e.g. cpu=200m,memory=500Mi,ephemeral-storage=1Gi) pairs that describe resources reserved for non-kubernetes components. Currently only cpu and memory are supported. See http://kubernetes.io/docs/user-guide/compute-resources for more detail. [default=none]")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.Var(flag.NewMapStringString(&tmp.KubeReserved), "kube-reserved", "A set of ResourceName=ResourceQuantity (e.g. cpu=200m,memory=500Mi,ephemeral-storage=1Gi) pairs that describe resources reserved for kubernetes system components. Currently cpu, memory and local ephemeral storage for root file system are supported. See http://kubernetes.io/docs/user-guide/compute-resources for more detail. [default=none]")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringSliceVar(&tmp.EnforceNodeAllocatable, "enforce-node-allocatable", tmp.EnforceNodeAllocatable, "A comma separated list of levels of node allocatable enforcement to be enforced by kubelet. Acceptable options are 'none', 'pods', 'system-reserved', and 'kube-reserved'. If the latter two options are specified, '--system-reserved-cgroup' and '--kube-reserved-cgroup' must also be set, respectively. If 'none' is specified, no additional options should be set. See https://kubernetes.io/docs/tasks/administer-cluster/reserve-compute-resources/ for more details.")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.SystemReservedCgroup, "system-reserved-cgroup", tmp.SystemReservedCgroup, "Absolute name of the top level cgroup that is used to manage non-kubernetes components for which compute resources were reserved via '--system-reserved' flag. Ex. '/system-reserved'. [default='']")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	fs.StringVar(&tmp.KubeReservedCgroup, "kube-reserved-cgroup", tmp.KubeReservedCgroup, "Absolute name of the top level cgroup that is used to manage kubernetes components for which compute resources were reserved via '--kube-reserved' flag. Ex. '/kube-reserved'. [default='']")
	fns[""] = func(tmp, c *kubeletconfig.KubeletConfiguration) {}

	// TODO(mtaufen): figure out how to wrap this back up in a defer later
	// All KubeletConfiguration flags are now deprecated, and any new flags that point to
	// KubeletConfiguration fields are deprecated-on-creation. When removing flags at the end
	// of their deprecation period, be careful to check that they have *actually* been deprecated
	// members of the KubeletConfiguration for the entire deprecation period:
	// e.g. if a flag was added after this deprecation function, it may not be at the end
	// of its lifetime yet, even if the rest are.
	deprecated := "This parameter should be set via the config file specified by the Kubelet's --config flag. See https://kubernetes.io/docs/tasks/administer-cluster/kubelet-config-file/ for more information."
	fs.VisitAll(func(f *pflag.Flag) {
		f.Deprecated = deprecated
	})
	mainfs.AddFlagSet(fs)

	return mkApply(mainfs, tmp, fns)
}

func mkApply(fs *pflag.FlagSet, tmp *KubeletConfiguration, applyFuncs map[string]func(tmp, c *Config)) {
	return func(c *Config) {
		for name, apply := range applyFuncs {
			// TODO(mtaufen): Consider using fs.Lookup(name) manually if we want to warn
			// on mismatch between applyFuncs and registered flags
			if fs.Changed(name) {
				apply(f, c)
			}
		}
	}
}
