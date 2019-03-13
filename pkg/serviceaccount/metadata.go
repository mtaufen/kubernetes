package serviceaccount

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	restful "github.com/emicklei/go-restful"
	jose "gopkg.in/square/go-jose.v2"
)

const (
	// OpenIDConfigPath is the URL path at which the API server serves
	// an OIDC Provider Configuration Information document, corresponding
	// to the Kubernetes Service Account key issuer.
	// https://openid.net/specs/openid-connect-discovery-1_0.html
	OpenIDConfigPath = "/.well-known/openid-configuration"
	// JwksPath is the URL path at which the API server serves a JWKS
	// containing the public keys that may be used to sign Kubernetes
	// Service Account keys
	JwksPath = "/serviceaccountkeys/v1/jwks.json"

	// UseSigning is the JWK "key use" field value for "signature" (as opposed to "encryption").
	UseSigning = "sig"
)

// IssuerMetadataServer serves issuer metadata for the KSA token issuer.
//
// It implmements a minimal subset of
// https://openid.net/specs/openid-connect-discovery-1_0.html#ProviderMetadata.
type IssuerMetadataServer interface {
	// Install configures the Container to route the appropriate requests
	// to the IssuerMetadataServer.
	Install(c *restful.Container)

	// SetErrorHandler sets a function to call when this server encounters an error.
	SetErrorHandler(func(error))
}

// NewServer creates a new IssuerMetadataServer.
func NewServer(iss string, keys []interface{}) IssuerMetadataServer {
	return &issuerServer{
		metadata: issuerMetadata{
			Issuer:  iss,
			JwksURI: JwksPath,
		},
		keys: keys,
	}
}

// issuerMetadata provides a subset of OIDC provider metadata:
// https://openid.net/specs/openid-connect-discovery-1_0.html#ProviderMetadata
type issuerMetadata struct {
	Issuer  string `json:"issuer"`   // REQUIRED in OIDC
	JwksURI string `json:"jwks_uri"` // REQUIRED in OIDC
}

// issuerServer is an HTTP server for metadata of the KSA token issuer.
type issuerServer struct {
	errorHandler func(error)
	metadata     issuerMetadata
	keys         []interface{}
}

// Install adds this server to the request router c.
func (s *issuerServer) Install(c *restful.Container) {
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
	jwks.Path(JwksPath).Route(
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

func (s *issuerServer) serveConfiguration(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// TODO: Set cache header appropriately

	enc := json.NewEncoder(w)
	enc.SetIndent("", "\t")
	// We can't stop writing the body and switch the header; but we can record when an error occured.
	err := enc.Encode(s.metadata)
	if err != nil && s.errorHandler != nil {
		s.errorHandler(ResponseError{
			URL: *req.URL,
			Err: err,
		})
	}
}

func (s *issuerServer) getJwks() *jose.JSONWebKeySet {
	// Decode keys into a JWKS.
	var keys jose.JSONWebKeySet
	for i, key := range s.keys {
		var pubkey *jose.JSONWebKey
		var err error

		switch k := key.(type) {
		case interface {
			Public() crypto.PublicKey
		}:
			// This is a private key. Get its public key
			pubkey, err = jwkFromPubkey(k.Public())
		case rsa.PublicKey:
			pubkey, err = jwkFromPubkey(&k)
		case *rsa.PublicKey:
			pubkey, err = jwkFromPubkey(k)
		case ecdsa.PublicKey:
			pubkey, err = jwkFromPubkey(&k)
		case *ecdsa.PublicKey:
			pubkey, err = jwkFromPubkey(k)
		default:
			err = KeyExcludedError{
				Key:    k,
				Reason: "must be (*)rsa.PublicKey or (*)ecdsa.PublicKey",
			}
		}
		if err != nil {
			s.handleError(err)
			continue
		}

		if !pubkey.Valid() {
			s.handleError(KeyExcludedError{
				Key:    pubkey,
				Reason: fmt.Sprintf("configured key #%d not valid", i),
			})
			continue
		}
		keys.Keys = append(keys.Keys, *pubkey)
	}
	return &keys
}

func (s *issuerServer) serveKeys(w http.ResponseWriter, req *http.Request) {
	// Per RFC7517 : https://tools.ietf.org/html/rfc7517#section-8.5.1
	w.Header().Set("Content-Type", "application/jwk-set+json")
	// TODO: Set cache header

	enc := json.NewEncoder(w)
	enc.SetIndent("", "\t")

	err := enc.Encode(s.getJwks())

	if err != nil {
		s.handleError(ResponseError{
			URL: *req.URL,
			Err: err,
		})
	}
}

func jwkFromPubkey(k crypto.PublicKey) (*jose.JSONWebKey, error) {
	alg, err := algorithmForPublicKey(k)
	if err != nil {
		return nil, KeyExcludedError{
			Key:    k,
			Reason: "must be *rsa.PublicKey, *ecdsa.PublicKey, or jose.OpaqueSigner",
		}
	}
	return &jose.JSONWebKey{
		Key:       k,
		Algorithm: string(alg),
		Use:       UseSigning,
	}, nil
}

func serveError(w http.ResponseWriter, code int) {
	http.Error(w, http.StatusText(code), code)
}

func (s *issuerServer) SetErrorHandler(f func(error)) {
	s.errorHandler = f
}

func (s *issuerServer) handleError(err error) {
	if err == nil {
		return
	}
	if s.errorHandler != nil {
		s.errorHandler(err)
	}
}

// ResponseError indicates there was an error responding to a request.
type ResponseError struct {
	URL url.URL
	Err error
}

func (e ResponseError) Error() string {
	return fmt.Sprintf("error responding for %v: %v", e.URL, e.Err)
}

// KeyExcludedError indicates a configured key was excluded from the JWKS.
type KeyExcludedError struct {
	Key    interface{}
	Reason string
}

func (ke KeyExcludedError) Error() string {
	return fmt.Sprintf("could not serve key of type %T: %v", ke.Key, ke.Reason)
}
