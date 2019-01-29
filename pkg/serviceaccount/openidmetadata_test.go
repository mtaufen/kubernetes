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

package serviceaccount_test

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	restful "github.com/emicklei/go-restful"
	"github.com/google/go-cmp/cmp"
	jose "gopkg.in/square/go-jose.v2"

	"k8s.io/kubernetes/pkg/routes"
	"k8s.io/kubernetes/pkg/serviceaccount"
)

func setupServer(t *testing.T, iss string, keys []interface{}) *httptest.Server {
	t.Helper()

	c := restful.NewContainer()
	s := httptest.NewServer(c)

	// Construct after we start server so key URL can include host
	metadataJSON, err := serviceaccount.OpenIDMetadataJSON(
		iss, s.URL+routes.JWKSPath, keys)
	if err != nil {
		t.Fatalf("could not marshal issuer discovery JSON, error: %v", err)
	}

	keysJSON, err := serviceaccount.OpenIDKeysetJSON(keys)
	if err != nil {
		t.Fatalf("could not marshal issuer keys JSON, error: %v", err)
	}

	srv := routes.NewOpenIDMetadataServer(metadataJSON, keysJSON)
	srv.Install(c)

	return s
}

var defaultKeys = []interface{}{getPublicKey(rsaPublicKey), getPublicKey(ecdsaPublicKey)}

// Configuration is an OIDC configuration, including most but not all required fields.
// https://openid.net/specs/openid-connect-discovery-1_0.html#ProviderMetadata
type Configuration struct {
	Issuer        string   `json:"issuer"`
	JWKSURI       string   `json:"jwks_uri"`
	ResponseTypes []string `json:"response_types_supported"`
	SigningAlgs   []string `json:"id_token_signing_alg_values_supported"`
	SubjectTypes  []string `json:"subject_types_supported"`
}

func TestServeConfiguration(t *testing.T) {
	s := setupServer(t, "my-fake-issuer", defaultKeys)
	defer s.Close()

	want := Configuration{
		Issuer:        "my-fake-issuer",
		JWKSURI:       s.URL + routes.JWKSPath,
		ResponseTypes: []string{"id_token"},
		SubjectTypes:  []string{"public"},
		SigningAlgs:   []string{"ES256", "RS256"},
	}

	reqURL := s.URL + "/.well-known/openid-configuration"

	resp, err := http.Get(reqURL)
	if err != nil {
		t.Fatalf("Get(%s) = %v, %v want: <response>, <nil>", reqURL, resp, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Get(%s) = %v, _ want: %v, _", reqURL, resp.StatusCode, http.StatusOK)
	}

	if got, want := resp.Header.Get("Content-Type"), "application/json"; got != want {
		t.Errorf("Get(%s) Content-Type = %q, _ want: %q, _", reqURL, got, want)
	}
	if got, want := resp.Header.Get("Cache-Control"), "public, max-age=86400"; got != want {
		t.Errorf("Get(%s) Cache-Control = %q, _ want: %q, _", reqURL, got, want)
	}

	var got Configuration
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("Decode(_) = %v, want: <nil>", err)
	}

	if !cmp.Equal(want, got) {
		t.Errorf("unexpected diff in received configuration (-want, +got):%s",
			cmp.Diff(want, got))
	}
}

func TestServeKeys(t *testing.T) {
	wantPubRSA := getPublicKey(rsaPublicKey).(*rsa.PublicKey)
	wantPubECDSA := getPublicKey(ecdsaPublicKey).(*ecdsa.PublicKey)
	var serveKeysTests = []struct {
		Name     string
		Keys     []interface{}
		WantKeys []jose.JSONWebKey
	}{
		{
			Name: "configured public keys",
			Keys: []interface{}{
				getPublicKey(rsaPublicKey),
				getPublicKey(ecdsaPublicKey),
			},
			WantKeys: []jose.JSONWebKey{
				{
					Algorithm:    "RS256",
					Key:          wantPubRSA,
					KeyID:        rsaKeyID,
					Use:          "sig",
					Certificates: []*x509.Certificate{},
				},
				{
					Algorithm:    "ES256",
					Key:          wantPubECDSA,
					KeyID:        ecdsaKeyID,
					Use:          "sig",
					Certificates: []*x509.Certificate{},
				},
			},
		},
		{
			Name: "only publishes public keys",
			Keys: []interface{}{
				getPrivateKey(rsaPrivateKey),
				getPrivateKey(ecdsaPrivateKey),
			},
			WantKeys: []jose.JSONWebKey{
				{
					Algorithm:    "RS256",
					Key:          wantPubRSA,
					KeyID:        rsaKeyID,
					Use:          "sig",
					Certificates: []*x509.Certificate{},
				},
				{
					Algorithm:    "ES256",
					Key:          wantPubECDSA,
					KeyID:        ecdsaKeyID,
					Use:          "sig",
					Certificates: []*x509.Certificate{},
				},
			},
		},
	}

	for _, tt := range serveKeysTests {
		t.Run(tt.Name, func(t *testing.T) {
			s := setupServer(t, "my-fake-issuer", tt.Keys)
			defer s.Close()

			reqURL := s.URL + "/openid/v1/jwks"

			resp, err := http.Get(reqURL)
			if err != nil {
				t.Fatalf("Get(%s) = %v, %v want: <response>, <nil>", reqURL, resp, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Get(%s) = %v, _ want: %v, _", reqURL, resp.StatusCode, http.StatusOK)
			}

			if got, want := resp.Header.Get("Content-Type"), "application/jwk-set+json"; got != want {
				t.Errorf("Get(%s) Content-Type = %q, _ want: %q, _", reqURL, got, want)
			}
			if got, want := resp.Header.Get("Cache-Control"), "public, max-age=86400"; got != want {
				t.Errorf("Get(%s) Cache-Control = %q, _ want: %q, _", reqURL, got, want)
			}

			ks := &jose.JSONWebKeySet{}
			if err := json.NewDecoder(resp.Body).Decode(ks); err != nil {
				t.Fatalf("Decode(_) = %v, want: <nil>", err)
			}

			bigIntComparer := cmp.Comparer(
				func(x, y *big.Int) bool {
					return x.Cmp(y) == 0
				})
			if !cmp.Equal(tt.WantKeys, ks.Keys, bigIntComparer) {
				t.Errorf("unexpected diff in JWKS keys (-want, +got): %v",
					cmp.Diff(tt.WantKeys, ks.Keys, bigIntComparer))
			}
		})
	}
}

func TestURLBoundaries(t *testing.T) {
	s := setupServer(t, "my-fake-issuer", defaultKeys)
	defer s.Close()

	for _, tt := range []struct {
		Name   string
		Path   string
		WantOK bool
	}{
		{"OIDC config path", "/.well-known/openid-configuration", true},
		{"JWKS path", "/openid/v1/jwks", true},
		{"well-known", "/.well-known", false},
		{"subpath", "/openid/v1/jwks/foo", false},
		{"query", "/openid/v1/jwks?format=yaml", true},
		{"fragment", "/openid/v1/jwks#issuer", true},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			resp, err := http.Get(s.URL + tt.Path)
			if err != nil {
				t.Fatal(err)
			}

			if tt.WantOK && (resp.StatusCode != http.StatusOK) {
				t.Errorf("Get(%v)= %v, want %v", tt.Path, resp.StatusCode, http.StatusOK)
			}
			if !tt.WantOK && (resp.StatusCode != http.StatusNotFound) {
				t.Errorf("Get(%v)= %v, want %v", tt.Path, resp.StatusCode, http.StatusNotFound)
			}
		})
	}
}
