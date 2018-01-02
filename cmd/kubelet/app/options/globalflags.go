/*
Copyright 2017 The Kubernetes Authors.

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
	"os"
	"strings"

	"github.com/spf13/pflag"

	flagutil "k8s.io/apiserver/pkg/util/flag"

	// libs that provide registration functions
	"k8s.io/apiserver/pkg/util/logs"
	"k8s.io/kubernetes/pkg/version/verflag"

	// ensure libs have a chance to globally register their flags
	_ "github.com/golang/glog"
	_ "github.com/google/cadvisor/container/common"
	_ "github.com/google/cadvisor/container/containerd"
	_ "github.com/google/cadvisor/container/docker"
	_ "github.com/google/cadvisor/container/raw"
	_ "github.com/google/cadvisor/machine"
	_ "github.com/google/cadvisor/manager"
	_ "github.com/google/cadvisor/storage"
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

// register adds a flag to local that targets the Value associated with the Flag named globalName in global
// If fake is true, will register the flag name, but point it at a value with a noop Set implementation.
func register(global, local *flag.FlagSet, globalName, help string, fake bool) {
	if fake {
		local.Var(flagutil.NoOp{}, strings.Replace(globalName, "_", "-", -1), help)
	} else if f := global.Lookup(globalName); f != nil {
		// we should always use hyphens instead of underscores when registering kubelet flags
		local.Var(f.Value, strings.Replace(globalName, "_", "-", -1), help)
	}
}

// addGlogFlags adds flags from github.com/golang/glog
func addGlogFlags(fs *pflag.FlagSet, fake bool) {
	// lookup flags in global flag set and re-register the values with our flagset
	global := flag.CommandLine
	local := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	register(global, local, "logtostderr", "log to standard error instead of files", fake)
	register(global, local, "alsologtostderr", "log to standard error as well as files", fake)
	register(global, local, "v", "log level for V logs", fake)
	register(global, local, "stderrthreshold", "logs at or above this threshold go to stderr", fake)
	register(global, local, "vmodule", "comma-separated list of pattern=N settings for file-filtered logging", fake)
	register(global, local, "log_backtrace_at", "when logging hits line file:N, emit a stack trace", fake)
	register(global, local, "log_dir", "If non-empty, write log files in this directory", fake)

	fs.AddGoFlagSet(local)
}

// addCadvisorFlags adds flags from cadvisor
func addCadvisorFlags(fs *pflag.FlagSet, fake bool) {
	// lookup flags in global flag set and re-register the values with our flagset
	global := flag.CommandLine
	local := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	mistakes := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// These flags were also implicit from cadvisor, but are actually used by something in the core repo:
	// TODO(mtaufen): This one is stil used by our salt, but for heaven's sake it's even deprecated in cadvisor
	register(global, local, "docker_root", "DEPRECATED: docker root is read from docker info (this is a fallback, default: /var/lib/docker)", fake)
	// e2e node tests rely on this
	register(global, local, "housekeeping_interval", "Interval between container housekeepings", fake)

	// These flags were implicit from cadvisor, and are mistakes that should be registered deprecated:
	const help = "This is a cadvisor flag that was mistakenly registered with the Kubelet. Due to legacy concerns, it will follow the standard CLI deprecation timeline before being removed."

	register(global, mistakes, "application_metrics_count_limit", help, fake)
	register(global, mistakes, "boot_id_file", help, fake)
	register(global, mistakes, "container_hints", help, fake)
	register(global, mistakes, "containerd", help, fake)
	register(global, mistakes, "docker", help, fake)
	register(global, mistakes, "docker_env_metadata_whitelist", help, fake)
	register(global, mistakes, "docker_only", help, fake)
	register(global, mistakes, "docker-tls", help, fake)
	register(global, mistakes, "docker-tls-ca", help, fake)
	register(global, mistakes, "docker-tls-cert", help, fake)
	register(global, mistakes, "docker-tls-key", help, fake)
	register(global, mistakes, "enable_load_reader", help, fake)
	register(global, mistakes, "event_storage_age_limit", help, fake)
	register(global, mistakes, "event_storage_event_limit", help, fake)
	register(global, mistakes, "global_housekeeping_interval", help, fake)
	register(global, mistakes, "log_cadvisor_usage", help, fake)
	register(global, mistakes, "machine_id_file", help, fake)
	register(global, mistakes, "storage_driver_user", help, fake)
	register(global, mistakes, "storage_driver_password", help, fake)
	register(global, mistakes, "storage_driver_host", help, fake)
	register(global, mistakes, "storage_driver_db", help, fake)
	register(global, mistakes, "storage_driver_table", help, fake)
	register(global, mistakes, "storage_driver_secure", help, fake)
	register(global, mistakes, "storage_driver_buffer_duration", help, fake)

	// mark all mistakes flags as deprecated
	temp := pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
	temp.AddGoFlagSet(mistakes)
	temp.VisitAll(func(f *pflag.Flag) {
		f.Deprecated = help
	})

	// finally, add cadvisor flags to the provided flagset
	fs.AddGoFlagSet(local)
	fs.AddFlagSet(temp)
}
