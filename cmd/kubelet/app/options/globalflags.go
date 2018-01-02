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

package options

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"

	flagutil "k8s.io/apiserver/pkg/util/flag"

	// libs that provide registration functions
	"k8s.io/apiserver/pkg/util/logs"
	"k8s.io/kubernetes/pkg/version/verflag"

	// ensure libs have a chance to globally register their flags
	_ "github.com/golang/glog"
	_ "k8s.io/kubernetes/pkg/credentialprovider/azure"
	_ "k8s.io/kubernetes/pkg/credentialprovider/gcp"
)

// AddGlobalFlags explicitly registers flags that libraries (glog, verflag, etc.) register
// against the global flagsets from "flag" and "github.com/spf13/pflag".
// We do this in order to prevent unwanted flags from leaking into the Kubelet's flagset.
// If fake is true, will register the flag name, but point it at a value with a noop Set implementation.
func AddGlobalFlags(fs *pflag.FlagSet, fake bool) {
	addGlogFlags(fs, fake)
	addCadvisorFlags(fs, fake)
	verflag.AddFlags(fs, fake)
	logs.AddFlags(fs, fake)
}

// normalize replaces underscores with hyphens
// we should always use hyphens instead of underscores when registering kubelet flags
func normalize(s string) string {
	return strings.Replace(s, "_", "-", -1)
}

// register adds a flag to local that targets the Value associated with the Flag named globalName in global.
// If fake is true, will register the flag name, but point it at a value with a noop Set implementation.
func register(global *flag.FlagSet, local *pflag.FlagSet, globalName string, fake bool) {
	if fake {
		local.Var(flagutil.NoOp{}, normalize(globalName), "")
	} else if f := global.Lookup(globalName); f != nil {
		f.Name = normalize(f.Name)
		local.AddFlag(pflag.PFlagFromGoFlag(f))
	} else {
		panic(fmt.Sprintf("failed to find flag in global flagset (flag): %s", globalName))
	}
}

// pflagRegister adds a flag to local that targets the Value associated with the Flag named globalName in global.
// If fake is true, will register the flag name, but point it at a value with a noop Set implementation.
func pflagRegister(global, local *pflag.FlagSet, globalName string, fake bool) {
	if fake {
		local.Var(flagutil.NoOp{}, normalize(globalName), "")
	} else if f := global.Lookup(globalName); f != nil {
		f.Name = normalize(f.Name)
		local.AddFlag(f)
	} else {
		panic(fmt.Sprintf("failed to find flag in global flagset (pflag): %s", globalName))
	}
}

// registerDeprecated registers the flag with register, and then marks it deprecated
func registerDeprecated(global *flag.FlagSet, local *pflag.FlagSet, globalName, deprecated string, fake bool) {
	register(global, local, globalName, fake)
	local.Lookup(normalize(globalName)).Deprecated = deprecated
}

// pflagRegisterDeprecated registers the flag with pflagRegister, and then marks it deprecated
func pflagRegisterDeprecated(global, local *pflag.FlagSet, globalName, deprecated string, fake bool) {
	pflagRegister(global, local, globalName, fake)
	local.Lookup(normalize(globalName)).Deprecated = deprecated
}

// addCredentialProviderFlags adds flags from k8s.io/kubernetes/pkg/credentialprovider
// If fake is true, will register the flag names, but point them at values with noop Set implementations.
func addCredentialProviderFlags(fs *pflag.FlagSet, fake bool) {
	// lookup flags in global flag set and re-register the values with our flagset
	global := pflag.CommandLine
	local := pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)

	// Note this is deprecated in the library that provides it, so we just allow that deprecation
	// notice to pass through our registration here.
	pflagRegister(global, local, "google-json-key", fake)
	// TODO(#58034): This is not a static file, so it's not quite as straightforward as --google-json-key.
	// We need to figure out how ACR users can dynamically provide pull credentials before we can deprecate this.
	pflagRegister(global, local, "azure-container-registry-config", fake)

	fs.AddFlagSet(local)
}

// addGlogFlags adds flags from github.com/golang/glog
// If fake is true, will register the flag names, but point them at values with noop Set implementations.
func addGlogFlags(fs *pflag.FlagSet, fake bool) {
	// lookup flags in global flag set and re-register the values with our flagset
	global := flag.CommandLine
	local := pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)

	register(global, local, "logtostderr", fake)
	register(global, local, "alsologtostderr", fake)
	register(global, local, "v", fake)
	register(global, local, "stderrthreshold", fake)
	register(global, local, "vmodule", fake)
	register(global, local, "log_backtrace_at", fake)
	register(global, local, "log_dir", fake)

	fs.AddFlagSet(local)
}
