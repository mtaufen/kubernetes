/*
Copyright 2014 The Kubernetes Authors.

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

// The kubelet binary is responsible for maintaining a set of containers on a particular host VM.
// It syncs data from both configuration file(s) as well as from a quorum of etcd servers.
// It then queries Docker to see what is currently running.  It synchronizes the configuration data,
// with the running set of containers by starting or stopping Docker containers.
package main

import (
	"fmt"
	"os"

	"github.com/golang/glog"
	"github.com/spf13/pflag"

	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/apiserver/pkg/util/flag"
	"k8s.io/apiserver/pkg/util/logs"
	"k8s.io/kubernetes/cmd/kubelet/app"
	"k8s.io/kubernetes/cmd/kubelet/app/options"
	"k8s.io/kubernetes/pkg/version/verflag"

	_ "k8s.io/kubernetes/pkg/client/metrics/prometheus" // for client metric registration
	_ "k8s.io/kubernetes/pkg/version/prometheus"        // for version metric registration
)

func parseFlagSet(fs *pflag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	fs.VisitAll(func(flag *pflag.Flag) {
		glog.V(2).Infof("FLAG: --%s=%q", flag.Name, flag.Value)
	})
	return nil
}

func die(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

// newFlagSetWithGlobals constructs a new pflag.FlagSet with global flags registered
// on it. Pass fake=true to get a FlagSet where global names are registered, but parsing
// args into the FlagSet will not mutate any values.
func newFlagSetWithGlobals(fake bool) *pflag.FlagSet {
	fs := pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
	// set the normalize func, similar to k8s.io/apiserver/pkg/util/flag/flags.go:InitFlags
	fs.SetNormalizeFunc(flag.WordSepNormalizeFunc)
	// explicitly add flags from libs that register global flags
	options.AddGlobalFlags(fs, fake)
	return fs
}

func main() {
	cliArgs := os.Args[1:]
	fs := newFlagSetWithGlobals(false)

	// register kubelet flags
	kubeletFlags := options.NewKubeletFlags()
	kubeletFlags.AddFlags(fs)

	// register kubelet config flags
	defaultConfig, err := options.NewKubeletConfiguration()
	if err != nil {
		die(err)
	}
	options.AddKubeletConfigFlags(fs, defaultConfig)

	// parse flags
	if err := parseFlagSet(fs, cliArgs); err != nil {
		die(err)
	}

	// initialize logging and defer flush
	logs.InitLogs()
	defer logs.FlushLogs()

	// short-circuit on verflag
	verflag.PrintAndExitIfRequested()

	// TODO(mtaufen): won't need this this once dynamic config is GA
	// set feature gates so we can check if dynamic config is enabled
	if err := utilfeature.DefaultFeatureGate.SetFromMap(defaultConfig.FeatureGates); err != nil {
		die(err)
	}
	// validate the initial KubeletFlags, to make sure the dynamic-config-related flags aren't used unless the feature gate is on
	if err := options.ValidateKubeletFlags(kubeletFlags); err != nil {
		die(err)
	}
	// bootstrap the kubelet config controller, app.BootstrapKubeletConfigController will check
	// feature gates and only turn on relevant parts of the controller
	kubeletConfig, kubeletConfigController, err := app.BootstrapKubeletConfigController(
		defaultConfig, kubeletFlags.KubeletConfigFile, kubeletFlags.DynamicConfigDir)
	if err != nil {
		die(err)
	}

	// If the config returned from the controller is a different object than defaultConfig,
	// we must enforce flag precedence by re-parsing the command line into the new object.
	// This is necessary to preserve backwards-compatibility across binary upgrades.
	// See issue #56171 for more details.
	if kubeletConfig != defaultConfig {
		// we use a throwaway kubeletFlags and a fake global flagset to avoid double-parses,
		// as some Set implementations accumulate values from multiple flag invocations
		fs := newFlagSetWithGlobals(true)

		// register throwaway KubeletFlags
		tmp := options.NewKubeletFlags()
		tmp.AddFlags(fs)

		// register new KubeletConfiguration
		options.AddKubeletConfigFlags(fs, kubeletConfig)

		// re-parse flags
		if err := parseFlagSet(fs, cliArgs); err != nil {
			die(err)
		}
	}

	// construct a KubeletServer from kubeletFlags and kubeletConfig
	kubeletServer := &options.KubeletServer{
		KubeletFlags:         *kubeletFlags,
		KubeletConfiguration: *kubeletConfig,
	}

	// use kubeletServer to construct the default KubeletDeps
	kubeletDeps, err := app.UnsecuredDependencies(kubeletServer)
	if err != nil {
		die(err)
	}

	// add the kubelet config controller to kubeletDeps
	kubeletDeps.KubeletConfigController = kubeletConfigController

	// start the experimental docker shim, if enabled
	if kubeletFlags.ExperimentalDockershim {
		if err := app.RunDockershim(kubeletFlags, kubeletConfig); err != nil {
			die(err)
		}
	}

	// run the kubelet
	if err := app.Run(kubeletServer, kubeletDeps); err != nil {
		die(err)
	}
}
