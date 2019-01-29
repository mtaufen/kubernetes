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

package serviceaccount_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	v1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	serviceaccountcontroller "k8s.io/kubernetes/pkg/controller/serviceaccount"
	"k8s.io/kubernetes/pkg/serviceaccount"
)

func TestTokenGenerateAndValidate(t *testing.T) {
	expectedUserName := "system:serviceaccount:test:my-service-account"
	expectedUserUID := "12345"

	// Related API objects
	serviceAccount := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-service-account",
			UID:       "12345",
			Namespace: "test",
		},
	}
	rsaSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-rsa-secret",
			Namespace: "test",
		},
	}
	ecdsaSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-ecdsa-secret",
			Namespace: "test",
		},
	}

	// Generate the RSA token
	rsaGenerator, err := serviceaccount.JWTTokenGenerator(serviceaccount.LegacyIssuer, getPrivateKey(rsaPrivateKey))
	if err != nil {
		t.Fatalf("error making generator: %v", err)
	}
	rsaToken, err := rsaGenerator.GenerateToken(serviceaccount.LegacyClaims(*serviceAccount, *rsaSecret))
	if err != nil {
		t.Fatalf("error generating token: %v", err)
	}
	if len(rsaToken) == 0 {
		t.Fatalf("no token generated")
	}
	rsaSecret.Data = map[string][]byte{
		"token": []byte(rsaToken),
	}

	// Generate the ECDSA token
	ecdsaGenerator, err := serviceaccount.JWTTokenGenerator(serviceaccount.LegacyIssuer, getPrivateKey(ecdsaPrivateKey))
	if err != nil {
		t.Fatalf("error making generator: %v", err)
	}
	ecdsaToken, err := ecdsaGenerator.GenerateToken(serviceaccount.LegacyClaims(*serviceAccount, *ecdsaSecret))
	if err != nil {
		t.Fatalf("error generating token: %v", err)
	}
	if len(ecdsaToken) == 0 {
		t.Fatalf("no token generated")
	}
	ecdsaSecret.Data = map[string][]byte{
		"token": []byte(ecdsaToken),
	}

	// Generate signer with same keys as RSA signer but different issuer
	badIssuerGenerator, err := serviceaccount.JWTTokenGenerator("foo", getPrivateKey(rsaPrivateKey))
	if err != nil {
		t.Fatalf("error making generator: %v", err)
	}
	badIssuerToken, err := badIssuerGenerator.GenerateToken(serviceaccount.LegacyClaims(*serviceAccount, *rsaSecret))
	if err != nil {
		t.Fatalf("error generating token: %v", err)
	}

	testCases := map[string]struct {
		Client clientset.Interface
		Keys   []interface{}
		Token  string

		ExpectedErr      bool
		ExpectedOK       bool
		ExpectedUserName string
		ExpectedUserUID  string
		ExpectedGroups   []string
	}{
		"no keys": {
			Token:       rsaToken,
			Client:      nil,
			Keys:        []interface{}{},
			ExpectedErr: false,
			ExpectedOK:  false,
		},
		"invalid keys (rsa)": {
			Token:       rsaToken,
			Client:      nil,
			Keys:        []interface{}{getPublicKey(otherPublicKey), getPublicKey(ecdsaPublicKey)},
			ExpectedErr: true,
			ExpectedOK:  false,
		},
		"invalid keys (ecdsa)": {
			Token:       ecdsaToken,
			Client:      nil,
			Keys:        []interface{}{getPublicKey(otherPublicKey), getPublicKey(rsaPublicKey)},
			ExpectedErr: true,
			ExpectedOK:  false,
		},
		"valid key (rsa)": {
			Token:            rsaToken,
			Client:           nil,
			Keys:             []interface{}{getPublicKey(rsaPublicKey)},
			ExpectedErr:      false,
			ExpectedOK:       true,
			ExpectedUserName: expectedUserName,
			ExpectedUserUID:  expectedUserUID,
			ExpectedGroups:   []string{"system:serviceaccounts", "system:serviceaccounts:test"},
		},
		"valid key, invalid issuer (rsa)": {
			Token:       badIssuerToken,
			Client:      nil,
			Keys:        []interface{}{getPublicKey(rsaPublicKey)},
			ExpectedErr: false,
			ExpectedOK:  false,
		},
		"valid key (ecdsa)": {
			Token:            ecdsaToken,
			Client:           nil,
			Keys:             []interface{}{getPublicKey(ecdsaPublicKey)},
			ExpectedErr:      false,
			ExpectedOK:       true,
			ExpectedUserName: expectedUserName,
			ExpectedUserUID:  expectedUserUID,
			ExpectedGroups:   []string{"system:serviceaccounts", "system:serviceaccounts:test"},
		},
		"rotated keys (rsa)": {
			Token:            rsaToken,
			Client:           nil,
			Keys:             []interface{}{getPublicKey(otherPublicKey), getPublicKey(ecdsaPublicKey), getPublicKey(rsaPublicKey)},
			ExpectedErr:      false,
			ExpectedOK:       true,
			ExpectedUserName: expectedUserName,
			ExpectedUserUID:  expectedUserUID,
			ExpectedGroups:   []string{"system:serviceaccounts", "system:serviceaccounts:test"},
		},
		"rotated keys (ecdsa)": {
			Token:            ecdsaToken,
			Client:           nil,
			Keys:             []interface{}{getPublicKey(otherPublicKey), getPublicKey(rsaPublicKey), getPublicKey(ecdsaPublicKey)},
			ExpectedErr:      false,
			ExpectedOK:       true,
			ExpectedUserName: expectedUserName,
			ExpectedUserUID:  expectedUserUID,
			ExpectedGroups:   []string{"system:serviceaccounts", "system:serviceaccounts:test"},
		},
		"valid lookup": {
			Token:            rsaToken,
			Client:           fake.NewSimpleClientset(serviceAccount, rsaSecret, ecdsaSecret),
			Keys:             []interface{}{getPublicKey(rsaPublicKey)},
			ExpectedErr:      false,
			ExpectedOK:       true,
			ExpectedUserName: expectedUserName,
			ExpectedUserUID:  expectedUserUID,
			ExpectedGroups:   []string{"system:serviceaccounts", "system:serviceaccounts:test"},
		},
		"invalid secret lookup": {
			Token:       rsaToken,
			Client:      fake.NewSimpleClientset(serviceAccount),
			Keys:        []interface{}{getPublicKey(rsaPublicKey)},
			ExpectedErr: true,
			ExpectedOK:  false,
		},
		"invalid serviceaccount lookup": {
			Token:       rsaToken,
			Client:      fake.NewSimpleClientset(rsaSecret, ecdsaSecret),
			Keys:        []interface{}{getPublicKey(rsaPublicKey)},
			ExpectedErr: true,
			ExpectedOK:  false,
		},
	}

	for k, tc := range testCases {
		auds := authenticator.Audiences{"api"}
		getter := serviceaccountcontroller.NewGetterFromClient(
			tc.Client,
			v1listers.NewSecretLister(newIndexer(func(namespace, name string) (interface{}, error) {
				return tc.Client.CoreV1().Secrets(namespace).Get(name, metav1.GetOptions{})
			})),
			v1listers.NewServiceAccountLister(newIndexer(func(namespace, name string) (interface{}, error) {
				return tc.Client.CoreV1().ServiceAccounts(namespace).Get(name, metav1.GetOptions{})
			})),
			v1listers.NewPodLister(newIndexer(func(namespace, name string) (interface{}, error) {
				return tc.Client.CoreV1().Pods(namespace).Get(name, metav1.GetOptions{})
			})),
		)
		authn := serviceaccount.JWTTokenAuthenticator(serviceaccount.LegacyIssuer, tc.Keys, auds, serviceaccount.NewLegacyValidator(tc.Client != nil, getter))

		// An invalid, non-JWT token should always fail
		ctx := authenticator.WithAudiences(context.Background(), auds)
		if _, ok, err := authn.AuthenticateToken(ctx, "invalid token"); err != nil || ok {
			t.Errorf("%s: Expected err=nil, ok=false for non-JWT token", k)
			continue
		}

		resp, ok, err := authn.AuthenticateToken(ctx, tc.Token)
		if (err != nil) != tc.ExpectedErr {
			t.Errorf("%s: Expected error=%v, got %v", k, tc.ExpectedErr, err)
			continue
		}

		if ok != tc.ExpectedOK {
			t.Errorf("%s: Expected ok=%v, got %v", k, tc.ExpectedOK, ok)
			continue
		}

		if err != nil || !ok {
			continue
		}

		if resp.User.GetName() != tc.ExpectedUserName {
			t.Errorf("%s: Expected username=%v, got %v", k, tc.ExpectedUserName, resp.User.GetName())
			continue
		}
		if resp.User.GetUID() != tc.ExpectedUserUID {
			t.Errorf("%s: Expected userUID=%v, got %v", k, tc.ExpectedUserUID, resp.User.GetUID())
			continue
		}
		if !reflect.DeepEqual(resp.User.GetGroups(), tc.ExpectedGroups) {
			t.Errorf("%s: Expected groups=%v, got %v", k, tc.ExpectedGroups, resp.User.GetGroups())
			continue
		}
	}
}

func newIndexer(get func(namespace, name string) (interface{}, error)) cache.Indexer {
	return &fakeIndexer{get: get}
}

type fakeIndexer struct {
	cache.Indexer
	get func(namespace, name string) (interface{}, error)
}

func (f *fakeIndexer) GetByKey(key string) (interface{}, bool, error) {
	parts := strings.SplitN(key, "/", 2)
	namespace := parts[0]
	name := ""
	if len(parts) == 2 {
		name = parts[1]
	}
	obj, err := f.get(namespace, name)
	return obj, err == nil, err
}
