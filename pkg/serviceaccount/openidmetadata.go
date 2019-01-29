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
	"encoding/json"
	"fmt"

	jose "gopkg.in/square/go-jose.v2"

	"k8s.io/apimachinery/pkg/util/sets"
)

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

// OpenIDMetadataJSON returns the JSON OIDC Discovery Doc for the service
// account issuer.
func OpenIDMetadataJSON(iss, jwksURI string, keys []interface{}) ([]byte, error) {
	keyset, errs := PublicJWKSFromKeys(keys)
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

// OpenIDKeysetJSON returns the JSON Web Key Set for the service account
// issuer's keys.
func OpenIDKeysetJSON(keys []interface{}) ([]byte, error) {
	keyset, errs := PublicJWKSFromKeys(keys)
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
