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

package remote

import (
	"fmt"
	"time"

	"github.com/golang/glog"
)

// StandaloneRemote contains the specific functions in the standalone node test suite
type StandaloneRemote struct{}

func InitStandaloneRemote() TestSuite {
	return &StandaloneRemote{}
}

// SetupTestPackage sets up the test package with binaries k8s required for node e2e tests
func (n *StandaloneRemote) SetupTestPackage(tardir, systemSpecName string) error {
	return setupDefaultTestPackage(tardir, systemSpecName)
}

// RunTest runs test on the node.
func (n *StandaloneRemote) RunTest(host, workspace, results, imageDesc, junitFilePrefix, testArgs, ginkgoArgs, systemSpecName string, timeout time.Duration) (string, error) {
	// Install the cni plugins and add a basic CNI configuration.
	if err := setupCNI(host, workspace); err != nil {
		return "", err
	}

	// Configure iptables firewall rules
	if err := configureFirewall(host); err != nil {
		return "", err
	}

	// Kill any running node processes
	cleanupNodeProcesses(host)

	testArgs, err := updateOSSpecificKubeletFlags(testArgs, host, workspace)
	if err != nil {
		return "", err
	}

	systemSpecFile := ""
	if systemSpecName != "" {
		systemSpecFile = systemSpecName + ".yaml"
	}

	// TODO(mtaufen): hardcode focusing on [standalone] tests

	// Run the tests
	glog.V(2).Infof("Starting tests on %q", host)
	cmd := getSSHCommand(" && ",
		fmt.Sprintf("cd %s", workspace),
		fmt.Sprintf("timeout -k 30s %fs ./ginkgo %s ./e2e_node.test -- --standalone=true --system-spec-name=%s --system-spec-file=%s --logtostderr --v 4 --node-name=%s --report-dir=%s --report-prefix=%s --image-description=\"%s\" %s",
			timeout.Seconds(), ginkgoArgs, systemSpecName, systemSpecFile, host, results, junitFilePrefix, imageDesc, testArgs),
	)
	return SSH(host, "sh", "-c", cmd)
}
