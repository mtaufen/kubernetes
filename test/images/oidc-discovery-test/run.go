package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	oidc "github.com/coreos/go-oidc"
	"golang.org/x/oauth2"
	"gopkg.in/square/go-jose.v2/jwt"
	"k8s.io/client-go/rest"
)

var (
	tokenPath = flag.String("token-path", "", "path to read service account token from")
	audience  = flag.String("audience", "", "audience to check on received token")
)

func main() {
	flag.Parse()

	ctx, err := clientContext()
	if err != nil {
		log.Fatal(err)
	}

	raw, err := gettoken()
	if err != nil {
		log.Fatal(err)
	}
	log.Print("OK: Got token")
	tok, err := jwt.ParseSigned(raw)
	if err != nil {
		log.Fatal(err)
	}
	var unsafeClaims claims
	if err := tok.UnsafeClaimsWithoutVerification(&unsafeClaims); err != nil {
		log.Fatal(err)
	}
	log.Printf("OK: got issuer %s", unsafeClaims.Issuer)
	log.Printf("(Full, not-validated claims: %+v)", unsafeClaims)

	iss, err := oidc.NewProvider(ctx, unsafeClaims.Issuer)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("OK: Constructed OIDC provider for issuer %v", unsafeClaims.Issuer)

	validTok, err := iss.Verifier(&oidc.Config{ClientID: *audience}).Verify(ctx, raw)
	if err != nil {
		log.Fatal(err)
	}
	log.Print("OK: Validated signature on JWT")

	var safeClaims claims
	if err := validTok.Claims(&safeClaims); err != nil {
		log.Fatal(err)
	}
	log.Print("OK: Got valid claims from token!")
	log.Printf("Full, validated claims: \n%#v", &safeClaims)
}

type kubeName struct {
	Name string `json:"name"`
	UID  string `json:"uid"`
}

type kubeClaims struct {
	Namespace      string   `json:"namespace"`
	ServiceAccount kubeName `json:"serviceaccount"`
}

type claims struct {
	jwt.Claims

	Kubernetes kubeClaims `json:"kubernetes.io"`
}

func (k *claims) String() string {
	return fmt.Sprintf("%s/%s for %s", k.Kubernetes.Namespace, k.Kubernetes.ServiceAccount.Name, k.Audience)
}

func gettoken() (string, error) {
	b, err := ioutil.ReadFile(*tokenPath)
	return string(b), err
}

func clientContext() (context.Context, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	rt, err := rest.TransportFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("could not get roundtripper: %v", err)
	}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{
		Transport: rt,
	})
	return ctx, err
}
