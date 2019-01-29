package serviceaccount_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	restful "github.com/emicklei/go-restful"
	jose "gopkg.in/square/go-jose.v2"
	"k8s.io/kubernetes/pkg/serviceaccount"
)

func setupServer(s serviceaccount.IssuerMetadataServer) *httptest.Server {
	c := restful.NewContainer()
	s.Install(c)
	return httptest.NewServer(c)
}

func TestServeConfiguration(t *testing.T) {
	s := setupServer(serviceaccount.NewServer("my-fake-issuer", nil))
	defer s.Close()

	want := `{
	"issuer": "my-fake-issuer",
	"jwks_uri": "/serviceaccountissuer/v1alpha1/jwks.json"
}
`
	reqURL := s.URL + "/serviceaccountissuer/v1alpha1/metadata.json"

	resp, err := http.Get(reqURL)
	if resp == nil || err != nil {
		t.Errorf("Get(%s) = %v, %v want: <response>, %v", reqURL, resp, err, nil)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Get(%s) = %v, _ want: %v, _", reqURL, resp.StatusCode, http.StatusOK)
	}

	if got, want := resp.Header.Get("Content-Type"), "application/json"; got != want {
		t.Errorf("Get(%s) Content-Type = %q, _ want: %q, _", reqURL, got, want)
	}

	b := bytes.NewBuffer(nil)
	if resp.Body == nil {
		t.Errorf("resp.Body = %v, want io.ReadCloser", resp.Body)
		return // can't evaluate body
	}
	defer resp.Body.Close()

	_, err = b.ReadFrom(resp.Body)
	if err != nil {
		t.Errorf("ReadFrom(_) = _, %v want: _, %v", err, nil)
		return // can't evaluate body
	}

	if got := b.String(); got != want {
		t.Errorf("response differs: got: ---\n%s\n--- want: ---\n%s\n---", got, want)
	}
}

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
			{Key: &rsa.PublicKey{}},
			{Key: &ecdsa.PublicKey{}},
		},
	},
	{
		Name: "only publishes public keys",
		Keys: []interface{}{
			getPrivateKey(rsaPrivateKey),
			getPrivateKey(ecdsaPrivateKey),
		},
		WantKeys: []jose.JSONWebKey{
			{Key: &rsa.PublicKey{}},
			{Key: &ecdsa.PublicKey{}},
		},
	},
}

func TestServeKeys(t *testing.T) {
	for _, tt := range serveKeysTests {
		t.Run(tt.Name, func(t *testing.T) {
			h := serviceaccount.NewServer("my-fake-issuer", tt.Keys)
			var errors []error
			h.SetErrorHandler(func(err error) {
				errors = append(errors, err)
			})

			s := setupServer(h)
			defer s.Close()

			reqURL := s.URL + "/serviceaccountissuer/v1alpha1/jwks.json"

			resp, err := http.Get(reqURL)
			if resp == nil || err != nil {
				t.Errorf("Get(%s) = %v, %v want: <response>, %v", reqURL, resp, err, nil)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("Get(%s) = %v, _ want: %v, _", reqURL, resp.StatusCode, http.StatusOK)
			}
			if got, want := resp.Header.Get("Content-Type"), "application/jwk-set+json"; got != want {
				t.Errorf("Get(%s) Content-Type = %q, _ want: %q, _", reqURL, got, want)
			}
			if resp.Body == nil {
				t.Errorf("resp.Body = %v, want io.ReadCloser", resp.Body)
				return // can't evaluate body
			}
			defer resp.Body.Close()

			if len(errors) != 0 {
				t.Errorf("unexpected errors while serving: got: %v want: <no errors>", errors)
			}

			d := json.NewDecoder(resp.Body)
			ks := &jose.JSONWebKeySet{}
			if err := d.Decode(ks); err != nil {
				t.Errorf("Decode(_) = %v, want: <nil>", err)
				return // can't evaluate keyset
			}

			if got, want := len(ks.Keys), len(tt.WantKeys); got != want {
				t.Errorf("JWKS: wrong number of keys: got: %d want: %d", got, want)
			}

			for i := range tt.WantKeys {
				if i == len(ks.Keys) {
					return
				}

				got, want := fmt.Sprintf("%T", ks.Keys[i].Key), fmt.Sprintf("%T", tt.WantKeys[i].Key)
				if got != want {
					t.Errorf("Keys[%d]: wrong type: got: %q want: %q", i, got, want)
				}
			}

			// TODO - DO NOT SUBMIT - test key set contents
		})
	}
}

func TestDisallowMethods(t *testing.T) {
	var loggedError error
	h := serviceaccount.NewServer("my-fake-issuer", nil)
	h.SetErrorHandler(func(e error) {
		loggedError = e
	})

	s := setupServer(h)
	defer s.Close()

	for _, method := range []string{
		http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace,
	} {
		t.Run(fmt.Sprintf("disallow %s", method), func(t *testing.T) {
			loggedError = nil

			req, err := http.NewRequest(method, s.URL+"/serviceaccountissuer/v1alpha1/metadata.json", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("Do() = %v, %v want: <response>, %v", resp, err, nil)
			}

			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("Do() = %v, _ want: %v, _", resp.StatusCode, http.StatusMethodNotAllowed)
			}

			if loggedError != nil {
				t.Errorf("Do(): server encountered unexpected error: %v", loggedError)
			}
		})
	}
}

func TestUrlBoundaries(t *testing.T) {
	s := setupServer(serviceaccount.NewServer("my-fake-issuer", nil))
	defer s.Close()

	for _, tt := range []struct {
		Name   string
		Path   string
		WantOK bool
	}{
		{"OIDC config path", "/serviceaccountissuer/v1alpha1/metadata.json", true},
		{"JWKS path", "/serviceaccountissuer/v1alpha1/jwks.json", true},
		{"prefix", "/serviceaccountissuer/v1alpha1/metadata", false},
		{"well-known", "/.well-known", false},
		{"subpath", "/serviceaccountissuer/v1alpha1/jwks.json/foo", false},
		{"query", "/serviceaccountissuer/v1alpha1/metadata.json?format=yaml", true},
		{"fragment", "/serviceaccountissuer/v1alpha1/metadata.json#issuer", true},
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
