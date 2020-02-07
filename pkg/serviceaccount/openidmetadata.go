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

package serviceaccount

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/url"

	jose "gopkg.in/square/go-jose.v2"

	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// JWKSPath is the URL path at which the API server serves a JWKS
	// containing the public keys that may be used to sign Kubernetes
	// Service Account keys.
	JWKSPath = "/openid/v1/jwks"
)

// OpenIDMetadata contains the pre-rendered responses for OIDC discovery endpoints.
type OpenIDMetadata struct {
	MetadataJSON     []byte
	PublicKeysetJSON []byte
}

// OpenIDMetadataBuilder constructs JSON metadata responses for OIDC discovery endpoints.
type OpenIDMetadataBuilder interface {
	OpenIDMetadata(defaultExternalAddress string) (*OpenIDMetadata, error)
}

type metadataBuilder struct {
	issuerURL string
	jwksURI   string
	pubKeys   []interface{}
}

// NewOpenIDMetadataBuilder constructs a builder that can produce pre-rendered
// JSON responses for the OIDC discovery endpoints.
func NewOpenIDMetadataBuilder(issuerURL, jwksURI string, pubKeys []interface{}) OpenIDMetadataBuilder {
	return &metadataBuilder{
		issuerURL: issuerURL,
		jwksURI:   jwksURI,
		pubKeys:   pubKeys,
	}
}

// OpenIDMetadata returns the pre-rendered JSON responses for the OIDC discovery
// endpoints, or an error if they could not be constructed. Callers should note
// that this function may perform additional validation on inputs that is not
// backwards-compatible with all command-line validation. The recommendation is
// to log the error and skip installing the OIDC discovery endpoints.
func (b *metadataBuilder) OpenIDMetadata(defaultExternalAddress string) (*OpenIDMetadata, error) {
	// Ensure the issuer URL is https, per the OIDC spec (this is the additional
	// validation the doc comment warns about).
	issuerURL, err := url.Parse(b.issuerURL)
	if err != nil {
		return nil, err
	}
	if issuerURL.Scheme != "https" {
		return nil, fmt.Errorf("issuer URL must use https scheme, got: %s", b.issuerURL)
	}

	// Either use the provided JWKS URI or default to ExternalAddress plus
	// the JWKS path.
	jwksURI := b.jwksURI
	if b.jwksURI == "" {
		const msg = "attempted to build jwks_uri from external " +
			"address %s, but could not construct a valid URL. Error: %v"

		if defaultExternalAddress == "" {
			return nil, fmt.Errorf(msg, defaultExternalAddress,
				fmt.Errorf("empty address"))
		}

		u := &url.URL{
			Scheme: "https",
			Host:   defaultExternalAddress,
			Path:   JWKSPath,
		}
		jwksURI = u.String()

		// TODO(mtaufen): I think we can probably expect ExternalAddress is
		// at most just host + port and skip the sanity check, but want to be
		// careful until that is confirmed.

		// Sanity check that the jwksURI we produced is the valid URL we expect.
		// This is just in case ExternalAddress came in as something weird,
		// like a scheme + host + port, instead of just host + port.
		parsed, err := url.Parse(jwksURI)
		if err != nil {
			return nil, fmt.Errorf(msg, defaultExternalAddress, err)
		} else if u.Scheme != parsed.Scheme ||
			u.Host != parsed.Host ||
			u.Path != parsed.Path {
			return nil, fmt.Errorf(msg, defaultExternalAddress,
				fmt.Errorf("got %v, expected %v", parsed, u))
		}
	} else {
		// Double-check that jwksURI is an https URL
		if u, err := url.Parse(b.jwksURI); err != nil {
			return nil, err
		} else if u.Scheme != "https" {
			return nil, fmt.Errorf("jwksURI requires https scheme, parsed as: %v", u.String())
		}
	}

	// Metadata and keys are expected to only change across restarts at present,
	// so we just marshal immediately and serve the cached JSON bytes.
	metadataJSON, err := openIDMetadataJSON(b.issuerURL, jwksURI, b.pubKeys)
	if err != nil {
		return nil, fmt.Errorf("could not marshal issuer discovery JSON, error: %v", err)
	}

	keysetJSON, err := openIDKeysetJSON(b.pubKeys)
	if err != nil {
		return nil, fmt.Errorf("could not marshal issuer keys JSON, error: %v", err)
	}

	return &OpenIDMetadata{
		MetadataJSON:     metadataJSON,
		PublicKeysetJSON: keysetJSON,
	}, nil
}

// openIDMetadata provides a minimal subset of OIDC provider metadata:
// https://openid.net/specs/openid-connect-discovery-1_0.html#ProviderMetadata
type openIDMetadata struct {
	Issuer string `json:"issuer"` // REQUIRED in OIDC; meaningful to relying parties.
	// TODO(mtaufen): Since our goal is compatibility for relying parties that
	// need to validate ID tokens, but do not need to initiate login flows,
	// and since we aren't sure what to put in authorization_endpoint yet,
	// we will omit this field until someone files a bug.
	// AuthzEndpoint string   `json:"authorization_endpoint"`                // REQUIRED in OIDC; but useless to relying parties.
	JWKSURI       string   `json:"jwks_uri"`                              // REQUIRED in OIDC; meaningful to relying parties.
	ResponseTypes []string `json:"response_types_supported"`              // REQUIRED in OIDC
	SubjectTypes  []string `json:"subject_types_supported"`               // REQUIRED in OIDC
	SigningAlgs   []string `json:"id_token_signing_alg_values_supported"` // REQUIRED in OIDC
}

// openIDMetadataJSON returns the JSON OIDC Discovery Doc for the service
// account issuer.
func openIDMetadataJSON(iss, jwksURI string, keys []interface{}) ([]byte, error) {
	keyset, errs := publicJWKSFromKeys(keys)
	if errs != nil {
		return nil, errs
	}

	metadata := openIDMetadata{
		Issuer:        iss,
		JWKSURI:       jwksURI,
		ResponseTypes: []string{"id_token"}, // Kubernetes only produces ID tokens
		SubjectTypes:  []string{"public"},   // https://openid.net/specs/openid-connect-core-1_0.html#SubjectIDTypes
		SigningAlgs:   getAlgs(keyset),      // REQUIRED by OIDC
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal service account issuer metadata: %v", err)
	}

	return metadataJSON, nil
}

// openIDKeysetJSON returns the JSON Web Key Set for the service account
// issuer's keys.
func openIDKeysetJSON(keys []interface{}) ([]byte, error) {
	keyset, errs := publicJWKSFromKeys(keys)
	if errs != nil {
		return nil, errs
	}

	keysetJSON, err := json.Marshal(keyset)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal service account issuer JWKS: %v", err)
	}

	return keysetJSON, nil
}

func getAlgs(keys *jose.JSONWebKeySet) []string {
	algs := sets.NewString()
	for _, k := range keys.Keys {
		algs.Insert(k.Algorithm)
	}
	// Note: List returns a sorted slice.
	return algs.List()
}

// publicJWKSFromKeys constructs a JSONWebKeySet from a list of keys. The key
// set will only contain the public keys associated with the input keys.
func publicJWKSFromKeys(in []interface{}) (*jose.JSONWebKeySet, errors.Aggregate) {
	// Decode keys into a JWKS.
	var keys jose.JSONWebKeySet
	var errs []error
	for i, key := range in {
		var pubkey *jose.JSONWebKey
		var err error

		switch k := key.(type) {
		case interface {
			Public() crypto.PublicKey
		}:
			// This is a private key. Get its public key
			pubkey, err = jwkFromPublicKey(k.Public())
		default:
			pubkey, err = jwkFromPublicKey(k)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("error constructing JWK for key #%d: %v", i, err))
			continue
		}

		if !pubkey.Valid() {
			errs = append(errs, fmt.Errorf("key #%d not valid", i))
			continue
		}
		keys.Keys = append(keys.Keys, *pubkey)
	}
	if len(errs) != 0 {
		return nil, errors.NewAggregate(errs)
	}
	return &keys, nil
}

func jwkFromPublicKey(publicKey crypto.PublicKey) (*jose.JSONWebKey, error) {
	alg, err := algorithmFromPublicKey(publicKey)
	if err != nil {
		return nil, err
	}

	keyID, err := keyIDFromPublicKey(publicKey)
	if err != nil {
		return nil, err
	}

	jwk := &jose.JSONWebKey{
		Algorithm: string(alg),
		Key:       publicKey,
		KeyID:     keyID,
		Use:       "sig",
	}

	if !jwk.IsPublic() {
		return nil, fmt.Errorf("JWK was not a public key! JWK: %v", jwk)
	}

	return jwk, nil
}

func algorithmFromPublicKey(publicKey crypto.PublicKey) (jose.SignatureAlgorithm, error) {
	switch pk := publicKey.(type) {
	case *rsa.PublicKey:
		return jose.RS256, nil
	case *ecdsa.PublicKey:
		switch pk.Curve {
		case elliptic.P256():
			return jose.ES256, nil
		case elliptic.P384():
			return jose.ES384, nil
		case elliptic.P521():
			return jose.ES512, nil
		default:
			return "", fmt.Errorf("unknown private key curve, must be 256, 384, or 521")
		}
	case jose.OpaqueSigner:
		return jose.SignatureAlgorithm(pk.Public().Algorithm), nil
	default:
		return "", fmt.Errorf("unknown public key type, must be *rsa.PublicKey, *ecdsa.PublicKey, or jose.OpaqueSigner")
	}
}
