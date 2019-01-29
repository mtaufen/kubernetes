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

package routes

import (
	"net/http"

	restful "github.com/emicklei/go-restful"

	"k8s.io/klog"
)

// This code is in package routes because many controllers import
// pkg/serviceaccount, but are not allowed by import-boss to depend on
// go-restful. All logic that deals with keys is kept in pkg/serviceaccount,
// and only the rendered JSON is passed into this server.

const (
	// OpenIDConfigPath is the URL path at which the API server serves
	// an OIDC Provider Configuration Information document, corresponding
	// to the Kubernetes Service Account key issuer.
	// https://openid.net/specs/openid-connect-discovery-1_0.html
	OpenIDConfigPath = "/.well-known/openid-configuration"
	// JWKSPath is the URL path at which the API server serves a JWKS
	// containing the public keys that may be used to sign Kubernetes
	// Service Account keys
	JWKSPath = "/openid/v1/jwks"
	// cacheControl is the value of the Cache-Control header. Overrides the
	// global `private, no-cache` setting.
	headerCacheControl = "Cache-Control"
	cacheControl       = "public, max-age=86400" // 1 day

	// mimeJWKS is the content type of the keyset response
	mimeJWKS = "application/jwk-set+json"
)

// OpenIDMetadataServer is an HTTP server for metadata of the KSA token issuer.
type OpenIDMetadataServer struct {
	metadataJSON []byte
	keysetJSON   []byte
}

// NewOpenIDMetadataServer creates a new OpenIDMetadataServer.
// The issuer is the OIDC issuer; keys are the keys that may be used to sign
// KSA tokens.
func NewOpenIDMetadataServer(metadataJSON, keysetJSON []byte) *OpenIDMetadataServer {
	return &OpenIDMetadataServer{
		metadataJSON: metadataJSON,
		keysetJSON:   keysetJSON,
	}
}

// Install adds this server to the request router c.
func (s *OpenIDMetadataServer) Install(c *restful.Container) {
	// Configuration WebService
	// Container.Add "will detect duplicate root paths and exit in that case",
	// so we need a root for /.well-known/openid-configuration to avoid conflicts.
	cfg := new(restful.WebService).
		Produces(restful.MIME_JSON)

	cfg.Path(OpenIDConfigPath).Route(
		cfg.GET("").
			To(fromStandard(s.serveConfiguration)).
			Doc("get service account issuer OpenID configuration, also known as the 'OIDC discovery doc'").
			Operation("getServiceAccountIssuerOpenIDMetadata").
			// Just include the OK, doesn't look like we include Internal Error in our openapi-spec.
			Returns(http.StatusOK, "OK", ""))
	c.Add(cfg)

	// JWKS WebService
	jwks := new(restful.WebService).
		Produces(mimeJWKS)

	jwks.Path(JWKSPath).Route(
		jwks.GET("").
			To(fromStandard(s.serveKeys)).
			Doc("get service account issuer OpenID JSON Web Key Set (contains public token verification keys)").
			Operation("getServiceAccountIssuerOpenIDKeyset").
			// Just include the OK, doesn't look like we include Internal Error in our openapi-spec.
			Returns(http.StatusOK, "OK", ""))
	c.Add(jwks)
}

// fromStandard provides compatibility between the standard (net/http) handler signature and the restful signature.
func fromStandard(h http.HandlerFunc) restful.RouteFunction {
	return func(req *restful.Request, resp *restful.Response) {
		h(resp, req.Request)
	}
}

func (s *OpenIDMetadataServer) serveConfiguration(w http.ResponseWriter, req *http.Request) {
	w.Header().Set(restful.HEADER_ContentType, restful.MIME_JSON)
	w.Header().Set(headerCacheControl, cacheControl)
	if _, err := w.Write(s.metadataJSON); err != nil {
		klog.Errorf("failed to write service account issuer metadata response: %v", err)
		return
	}
}

func (s *OpenIDMetadataServer) serveKeys(w http.ResponseWriter, req *http.Request) {
	// Per RFC7517 : https://tools.ietf.org/html/rfc7517#section-8.5.1
	w.Header().Set(restful.HEADER_ContentType, mimeJWKS)
	w.Header().Set(headerCacheControl, cacheControl)
	if _, err := w.Write(s.keysetJSON); err != nil {
		klog.Errorf("failed to write service account issuer JWKS response: %v", err)
		return
	}
}
