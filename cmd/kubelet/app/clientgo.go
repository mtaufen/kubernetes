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

// this file contains parallel methods for getting a client-go clientconfig
// The types are not reasonably compatibly between the client-go and kubernetes restclient.Interface,
// restclient.Config, or typed clients, so this is the simplest solution during the transition
package app

import (
	"errors"
	"net/http"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/kubernetes/cmd/kubelet/app/options"
	"k8s.io/kubernetes/pkg/client/chaosclient"
)

func kubeconfigClientGoConfig(s *options.KubeletServer) (*rest.Config, error) {
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: s.KubeConfig.Value()},
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
}

// createClientConfig creates a client configuration from the command line arguments.
// If --kubeconfig is explicitly set, it will be used. If it is not set but
// --require-kubeconfig=true, we attempt to load the default kubeconfig file.
func createClientGoConfig(s *options.KubeletServer) (*rest.Config, error) {
	// TODO(mtaufen): File an issue to update this when we remove the default kubeconfig path and --require-kubeconfig.
	// If --kubeconfig was not provided, it will have a default path set in cmd/kubelet/app/options/options.go.
	// We only use that default path when --require-kubeconfig=true. The default path is temporary until --require-kubeconfig is removed.
	if s.KubeConfig.Provided() || (s.RequireKubeConfig.Provided() && s.RequireKubeConfig.Value() == true) {
		return kubeconfigClientGoConfig(s)
	} else {
		return nil, errors.New("--kubeconfig was not provided")
	}
}

// createAPIServerClientGoConfig generates a client.Config from command line flags,
// including api-server-list, via createClientConfig and then injects chaos into
// the configuration via addChaosToClientConfig. This func is exported to support
// integration with third party kubelet extensions (e.g. kubernetes-mesos).
func createAPIServerClientGoConfig(s *options.KubeletServer) (*rest.Config, error) {
	clientConfig, err := createClientGoConfig(s)
	if err != nil {
		return nil, err
	}

	clientConfig.ContentType = s.ContentType
	// Override kubeconfig qps/burst settings from flags
	clientConfig.QPS = float32(s.KubeAPIQPS)
	clientConfig.Burst = int(s.KubeAPIBurst)

	addChaosToClientGoConfig(s, clientConfig)
	return clientConfig, nil
}

// addChaosToClientConfig injects random errors into client connections if configured.
func addChaosToClientGoConfig(s *options.KubeletServer, config *rest.Config) {
	if s.ChaosChance != 0.0 {
		config.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
			seed := chaosclient.NewSeed(1)
			// TODO: introduce a standard chaos package with more tunables - this is just a proof of concept
			// TODO: introduce random latency and stalls
			return chaosclient.NewChaosRoundTripper(rt, chaosclient.LogChaos, seed.P(s.ChaosChance, chaosclient.ErrSimulatedConnectionResetByPeer))
		}
	}
}
