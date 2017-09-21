/*
Copyright 2016 The Kubernetes Authors.

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

package remote

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"k8s.io/kubernetes/test/e2e_node/builder"

	"github.com/golang/glog"
)

// utils.go contains functions used across test suites.

const (
	cniRelease          = "0799f5732f2a11b329d9e3d51b9c8f2e3759f2ff"
	cniDirectory        = "cni" // The CNI tarball creates the "bin" directory under "cni".
	cniConfDirectory    = "cni/net.d"
	cniURL              = "https://storage.googleapis.com/kubernetes-release/network-plugins/cni-" + cniRelease + ".tar.gz"
	localCOSMounterPath = "cluster/gce/gci/mounter/mounter"
	systemSpecPath      = "test/e2e_node/system/specs"
)

const cniConfig = `{
  "name": "mynet",
  "type": "bridge",
  "bridge": "mynet0",
  "isDefaultGateway": true,
  "forceAddress": false,
  "ipMasq": true,
  "hairpinMode": true,
  "ipam": {
    "type": "host-local",
    "subnet": "10.10.0.0/16"
  }
}
`

// Install the cni plugin and add basic bridge configuration to the
// configuration directory.
func setupCNI(host, workspace string) error {
	glog.V(2).Infof("Install CNI on %q", host)
	cniPath := filepath.Join(workspace, cniDirectory)
	cmd := getSSHCommand(" ; ",
		fmt.Sprintf("mkdir -p %s", cniPath),
		fmt.Sprintf("wget -O - %s | tar -xz -C %s", cniURL, cniPath),
	)
	if output, err := SSH(host, "sh", "-c", cmd); err != nil {
		return fmt.Errorf("failed to install cni plugin on %q: %v output: %q", host, err, output)
	}

	// The added CNI network config is not needed for kubenet. It is only
	// used when testing the CNI network plugin, but is added in both cases
	// for consistency and simplicity.
	glog.V(2).Infof("Adding CNI configuration on %q", host)
	cniConfigPath := filepath.Join(workspace, cniConfDirectory)
	cmd = getSSHCommand(" ; ",
		fmt.Sprintf("mkdir -p %s", cniConfigPath),
		fmt.Sprintf("echo %s > %s", quote(cniConfig), filepath.Join(cniConfigPath, "mynet.conf")),
	)
	if output, err := SSH(host, "sh", "-c", cmd); err != nil {
		return fmt.Errorf("failed to write cni configuration on %q: %v output: %q", host, err, output)
	}
	return nil
}

// configureFirewall configures iptable firewall rules.
func configureFirewall(host string) error {
	glog.V(2).Infof("Configure iptables firewall rules on %q", host)
	// TODO: consider calling bootstrap script to configure host based on OS
	output, err := SSH(host, "iptables", "-L", "INPUT")
	if err != nil {
		return fmt.Errorf("failed to get iptables INPUT on %q: %v output: %q", host, err, output)
	}
	if strings.Contains(output, "Chain INPUT (policy DROP)") {
		cmd := getSSHCommand("&&",
			"(iptables -C INPUT -w -p TCP -j ACCEPT || iptables -A INPUT -w -p TCP -j ACCEPT)",
			"(iptables -C INPUT -w -p UDP -j ACCEPT || iptables -A INPUT -w -p UDP -j ACCEPT)",
			"(iptables -C INPUT -w -p ICMP -j ACCEPT || iptables -A INPUT -w -p ICMP -j ACCEPT)")
		output, err := SSH(host, "sh", "-c", cmd)
		if err != nil {
			return fmt.Errorf("failed to configured firewall on %q: %v output: %v", host, err, output)
		}
	}
	output, err = SSH(host, "iptables", "-L", "FORWARD")
	if err != nil {
		return fmt.Errorf("failed to get iptables FORWARD on %q: %v output: %q", host, err, output)
	}
	if strings.Contains(output, "Chain FORWARD (policy DROP)") {
		cmd := getSSHCommand("&&",
			"(iptables -C FORWARD -w -p TCP -j ACCEPT || iptables -A FORWARD -w -p TCP -j ACCEPT)",
			"(iptables -C FORWARD -w -p UDP -j ACCEPT || iptables -A FORWARD -w -p UDP -j ACCEPT)",
			"(iptables -C FORWARD -w -p ICMP -j ACCEPT || iptables -A FORWARD -w -p ICMP -j ACCEPT)")
		output, err = SSH(host, "sh", "-c", cmd)
		if err != nil {
			return fmt.Errorf("failed to configured firewall on %q: %v output: %v", host, err, output)
		}
	}
	return nil
}

// cleanupNodeProcesses kills all running node processes may conflict with the test.
func cleanupNodeProcesses(host string) {
	glog.V(2).Infof("Killing any existing node processes on %q", host)
	cmd := getSSHCommand(" ; ",
		"pkill kubelet",
		"pkill kube-apiserver",
		"pkill etcd",
		"pkill e2e_node.test",
	)
	// No need to log an error if pkill fails since pkill will fail if the commands are not running.
	// If we are unable to stop existing running k8s processes, we should see messages in the kubelet/apiserver/etcd
	// logs about failing to bind the required ports.
	SSH(host, "sh", "-c", cmd)
}

// Quotes a shell literal so it can be nested within another shell scope.
func quote(s string) string {
	return fmt.Sprintf("'\"'\"'%s'\"'\"'", s)
}

// dest is relative to the root of the tar
func tarAddFile(tar, source, dest string) error {
	dir := filepath.Dir(dest)
	tardir := filepath.Join(tar, dir)
	tardest := filepath.Join(tar, dest)

	out, err := exec.Command("mkdir", "-p", tardir).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create archive bin subdir %q, was dest for file %q. Err: %v. Output:\n%s", tardir, source, err, out)
	}
	out, err = exec.Command("cp", source, tardest).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to copy file %q to the archive bin subdir %q. Err: %v. Output:\n%s", source, tardir, err, out)
	}
	return nil
}

// Includes the GCI/COS mounter artifacts in the deployed tarball
func tarAddCOSMounter(tar string) error {
	k8sDir, err := builder.GetK8sRootDir()
	if err != nil {
		return fmt.Errorf("Could not find K8s root dir! Err: %v", err)
	}

	source := filepath.Join(k8sDir, localCOSMounterPath)

	// Require the GCI/COS mounter script, we want to make sure the remote test runner stays up to date if the mounter file moves
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("Could not find GCI/COS mounter script at %q! If this script has been (re)moved, please update the e2e node remote test runner accordingly! Err: %v", source, err)
	}

	tarAddFile(tar, source, localCOSMounterPath)
	return nil
}

// prependCOSMounterFlag prepends the flag for setting the GCI mounter path to
// args and returns the result.
func prependCOSMounterFlag(args, host, workspace string) (string, error) {
	// If we are testing on a GCI/COS node, we chmod 544 the mounter and specify a different mounter path in the test args.
	// We do this here because the local var `workspace` tells us which /tmp/node-e2e-%d is relevant to the current test run.

	// Determine if the GCI/COS mounter script exists locally.
	k8sDir, err := builder.GetK8sRootDir()
	if err != nil {
		return args, fmt.Errorf("could not find K8s root dir! Err: %v", err)
	}
	source := filepath.Join(k8sDir, localCOSMounterPath)

	// Require the GCI/COS mounter script, we want to make sure the remote test runner stays up to date if the mounter file moves
	if _, err = os.Stat(source); err != nil {
		return args, fmt.Errorf("could not find GCI/COS mounter script at %q! If this script has been (re)moved, please update the e2e node remote test runner accordingly! Err: %v", source, err)
	}

	glog.V(2).Infof("GCI/COS node and GCI/COS mounter both detected, modifying --experimental-mounter-path accordingly")
	// Note this implicitly requires the script to be where we expect in the tarball, so if that location changes the error
	// here will tell us to update the remote test runner.
	mounterPath := filepath.Join(workspace, localCOSMounterPath)
	output, err := SSH(host, "sh", "-c", fmt.Sprintf("'chmod 544 %s'", mounterPath))
	if err != nil {
		return args, fmt.Errorf("unabled to chmod 544 GCI/COS mounter script. Err: %v, Output:\n%s", err, output)
	}
	// Insert args at beginning of test args, so any values from command line take precedence
	args = fmt.Sprintf("--kubelet-flags=--experimental-mounter-path=%s ", mounterPath) + args
	return args, nil
}

// prependMemcgNotificationFlag prepends the flag for enabling memcg
// notification to args and returns the result.
func prependMemcgNotificationFlag(args string) string {
	return "--kubelet-flags=--experimental-kernel-memcg-notification=true " + args
}

// updateOSSpecificKubeletFlags updates the Kubelet args with OS specific
// settings.
func updateOSSpecificKubeletFlags(args, host, workspace string) (string, error) {
	output, err := SSH(host, "cat", "/etc/os-release")
	if err != nil {
		return "", fmt.Errorf("issue detecting node's OS via node's /etc/os-release. Err: %v, Output:\n%s", err, output)
	}
	switch {
	case strings.Contains(output, "ID=gci"), strings.Contains(output, "ID=cos"):
		args = prependMemcgNotificationFlag(args)
		return prependCOSMounterFlag(args, host, workspace)
	case strings.Contains(output, "ID=ubuntu"):
		return prependMemcgNotificationFlag(args), nil
	}
	return args, nil
}

// setupDefaultTestPackage sets up the test package with binaries k8s required for e2e node or standalone node tests
func setupDefaultTestPackage(tardir, systemSpecName string) error {
	// Build the executables
	if err := builder.BuildGo(); err != nil {
		return fmt.Errorf("failed to build the depedencies: %v", err)
	}

	// Make sure we can find the newly built binaries
	buildOutputDir, err := builder.GetK8sBuildOutputDir()
	if err != nil {
		return fmt.Errorf("failed to locate kubernetes build output directory: %v", err)
	}

	rootDir, err := builder.GetK8sRootDir()
	if err != nil {
		return fmt.Errorf("failed to locate kubernetes root directory: %v", err)
	}

	// Copy binaries
	requiredBins := []string{"kubelet", "e2e_node.test", "ginkgo"}
	for _, bin := range requiredBins {
		source := filepath.Join(buildOutputDir, bin)
		if _, err := os.Stat(source); err != nil {
			return fmt.Errorf("failed to locate test binary %s: %v", bin, err)
		}
		out, err := exec.Command("cp", source, filepath.Join(tardir, bin)).CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to copy %q: %v Output: %q", bin, err, out)
		}
	}

	if systemSpecName != "" {
		// Copy system spec file
		source := filepath.Join(rootDir, systemSpecPath, systemSpecName+".yaml")
		if _, err := os.Stat(source); err != nil {
			return fmt.Errorf("failed to locate system spec %q: %v", source, err)
		}
		out, err := exec.Command("cp", source, tardir).CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to copy system spec %q: %v, output: %q", source, err, out)
		}
	}

	// Include the GCI/COS mounter artifacts in the deployed tarball
	err = tarAddCOSMounter(tardir)
	if err != nil {
		return err
	}
	return nil
}
