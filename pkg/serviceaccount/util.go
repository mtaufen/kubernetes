/*
Copyright 2014 The Kubernetes Authors.

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
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	apiserverserviceaccount "k8s.io/apiserver/pkg/authentication/serviceaccount"
	"k8s.io/apiserver/pkg/authentication/user"

	jose "gopkg.in/square/go-jose.v2"
)

const (
	// PodNameKey is the key used in a user's "extra" to specify the pod name of
	// the authenticating request.
	PodNameKey = "authentication.kubernetes.io/pod-name"
	// PodUIDKey is the key used in a user's "extra" to specify the pod UID of
	// the authenticating request.
	PodUIDKey = "authentication.kubernetes.io/pod-uid"
)

// UserInfo returns a user.Info interface for the given namespace, service account name and UID
func UserInfo(namespace, name, uid string) user.Info {
	return (&ServiceAccountInfo{
		Name:      name,
		Namespace: namespace,
		UID:       uid,
	}).UserInfo()
}

type ServiceAccountInfo struct {
	Name, Namespace, UID string
	PodName, PodUID      string
}

func (sa *ServiceAccountInfo) UserInfo() user.Info {
	info := &user.DefaultInfo{
		Name:   apiserverserviceaccount.MakeUsername(sa.Namespace, sa.Name),
		UID:    sa.UID,
		Groups: apiserverserviceaccount.MakeGroupNames(sa.Namespace),
	}
	if sa.PodName != "" && sa.PodUID != "" {
		info.Extra = map[string][]string{
			PodNameKey: {sa.PodName},
			PodUIDKey:  {sa.PodUID},
		}
	}
	return info
}

// IsServiceAccountToken returns true if the secret is a valid api token for the service account
func IsServiceAccountToken(secret *v1.Secret, sa *v1.ServiceAccount) bool {
	if secret.Type != v1.SecretTypeServiceAccountToken {
		return false
	}

	name := secret.Annotations[v1.ServiceAccountNameKey]
	uid := secret.Annotations[v1.ServiceAccountUIDKey]
	if name != sa.Name {
		// Name must match
		return false
	}
	if len(uid) > 0 && uid != string(sa.UID) {
		// If UID is specified, it must match
		return false
	}

	return true
}

// PublicJWKSFromKeys constructs a JSONWebKeySet from a list of keys. The key
// set will only contain the public keys associated with the input keys.
func PublicJWKSFromKeys(in []interface{}) (*jose.JSONWebKeySet, errors.Aggregate) {
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
		return nil, fmt.Errorf("JWK was not a public keyset! JWK: %v", jwk)
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
