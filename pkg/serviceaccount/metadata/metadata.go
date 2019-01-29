/*
Copyright 2019 The Kubernetes Authors.

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

package metadata

import (
	"encoding/json"
	"net/http"
	"sort"

	restful "github.com/emicklei/go-restful"
	jose "gopkg.in/square/go-jose.v2"

	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/serviceaccount"
)

const (
	// OpenIDConfigPath is the URL path at which the API server serves
	// an OIDC Provider Configuration Information document, corresponding
	// to the Kubernetes Service Account key issuer.
	// https://openid.net/specs/openid-connect-discovery-1_0.html
	OpenIDConfigPath = "/.well-known/openid-configuration"
	// JWKSPath is the URL path at which the API server serves a JWKS
	// containing the public keys that may be used to sign Kubernetes
	// Service Account keys
	JWKSPath = "/jwks"
)

// IssuerMetadataServer is an HTTP server for metadata of the KSA token issuer.
type IssuerMetadataServer struct {
	metadata issuerMetadata
	keys     *jose.JSONWebKeySet
}

// issuerMetadata provides a minimal subset of OIDC provider metadata:
// https://openid.net/specs/openid-connect-discovery-1_0.html#ProviderMetadata
type issuerMetadata struct {
	Issuer string `json:"issuer"` // REQUIRED in OIDC; meaningful to relying parties.
	// AuthzEndpoint string   `json:"authorization_endpoint"`                // REQUIRED in OIDC; but useless to relying parties.
	JWKSURI       string   `json:"jwks_uri"`                              // REQUIRED in OIDC; meaningful to relying parties.
	ResponseTypes []string `json:"response_types_supported"`              // REQUIRED in OIDC
	SubjectTypes  []string `json:"subject_types_supported"`               // REQUIRED in OIDC
	SigningAlgs   []string `json:"id_token_signing_alg_values_supported"` // REQUIRED in OIDC

}

// NewServer creates a new IssuerMetadataServer.
// The issuer is the OIDC issuer; keys are the keys that may be used to sign
// KSA tokens.
func NewServer(iss, jwksURI string, keys []interface{}) (*IssuerMetadataServer, error) {
	ks, errs := serviceaccount.PublicJWKSFromKeys(keys)
	if errs != nil {
		return nil, errs
	}

	return &IssuerMetadataServer{
		metadata: issuerMetadata{
			Issuer:        iss,
			JWKSURI:       jwksURI,
			ResponseTypes: []string{"id_token"}, // Kubernetes only produces ID tokens
			SubjectTypes:  []string{"public"},   // https://openid.net/specs/openid-connect-core-1_0.html#SubjectIDTypes
			SigningAlgs:   getAlgs(ks),          // REQUIRED by OIDC
		},
		keys: ks,
	}, nil
}

// Install adds this server to the request router c.
func (s *IssuerMetadataServer) Install(c *restful.Container) {
	// Container.Add "will detect duplicate root paths and exit in that case",
	// so we need a root for /.well-known/openid-configuration to avoid conflicts.
	cfg := new(restful.WebService)
	cfg.Path(OpenIDConfigPath).Route(
		cfg.GET("").
			To(fromStandard(s.serveConfiguration)).
			Doc("get serviceaccount issuer OIDC configuration").
			Operation("getServiceAccountIssuerMetadata"))
	c.Add(cfg)
	// ...and another one for the JWKS
	jwks := new(restful.WebService)
	jwks.Path(JWKSPath).Route(
		jwks.GET("").
			To(fromStandard(s.serveKeys)).
			Doc("get serviceaccount issuer keys").
			Operation("getServiceAccountIssuerKeys"))
	c.Add(jwks)
}

// fromStandard provides compatibility between the standard (net/http) handler signature and the restful signature.
func fromStandard(h http.HandlerFunc) restful.RouteFunction {
	return func(req *restful.Request, resp *restful.Response) {
		h(resp, req.Request)
	}
}

func (s *IssuerMetadataServer) serveConfiguration(w http.ResponseWriter, req *http.Request) {
	b, err := json.Marshal(s.metadata)
	if err != nil {
		klog.Errorf("failed to marshal service account issuer metadata: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "max-age=1")
	if _, err := w.Write(b); err != nil {
		klog.Errorf("failed to write service account issuer metadata response: %v", err)
		return
	}
}

func getAlgs(keys *jose.JSONWebKeySet) []string {
	var result []string
	algs := map[string]bool{}
	for _, k := range keys.Keys {
		a := k.Algorithm
		if !algs[a] {
			result = append(result, a)
		}
		algs[a] = true
	}
	sort.Strings(result)
	return result
}

func (s *IssuerMetadataServer) serveKeys(w http.ResponseWriter, req *http.Request) {
	b, err := json.Marshal(s.keys)
	if err != nil {
		klog.Errorf("failed to marshal service account issuer JWKS: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Per RFC7517 : https://tools.ietf.org/html/rfc7517#section-8.5.1
	w.Header().Set("Content-Type", "application/jwk-set+json")
	w.Header().Set("Cache-Control", "max-age=1")
	if _, err := w.Write(b); err != nil {
		klog.Errorf("failed to write service account issuer JWKS response: %v", err)
		return
	}
}
