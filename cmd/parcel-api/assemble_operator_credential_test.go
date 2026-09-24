package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

const (
	syntheticOperatorIssuer   = "https://id.syn.example/dex"
	syntheticOperatorAudience = "idp-parcel-admin-web"
)

func TestOperatorChannelWithoutIssuerParametersAnswersNotConfigured(t *testing.T) {
	verifier, configured, err := buildOperatorCredentialVerifier(fakeGetenv(nil))
	if err != nil {
		t.Fatalf("unset parameters must not refuse to start: %v", err)
	}
	if configured {
		t.Fatal("unset parameters reported as a configured operator channel")
	}
	_, err = verifier.VerifyOperatorCredential(context.Background(), accessidentity.NewOperatorCredential("any.presented.token"))
	if !errors.Is(err, accessidentity.ErrAccessChannelNotConfigured) {
		t.Fatalf("err = %v, want ErrAccessChannelNotConfigured", err)
	}
}

func TestOperatorChannelWithIncompleteOrMalformedIssuerParametersRefusesToStart(t *testing.T) {
	complete := map[string]string{
		operatorIssuerEnv:   syntheticOperatorIssuer,
		operatorJWKSURLEnv:  "https://id.syn.example/dex/keys",
		operatorAudienceEnv: syntheticOperatorAudience,
	}
	cases := map[string]map[string]string{
		"issuer only":            {operatorIssuerEnv: syntheticOperatorIssuer},
		"JWKS URL only":          {operatorJWKSURLEnv: complete[operatorJWKSURLEnv]},
		"audience only":          {operatorAudienceEnv: syntheticOperatorAudience},
		"issuer without JWKS":    {operatorIssuerEnv: syntheticOperatorIssuer, operatorAudienceEnv: syntheticOperatorAudience},
		"issuer without aud":     {operatorIssuerEnv: syntheticOperatorIssuer, operatorJWKSURLEnv: complete[operatorJWKSURLEnv]},
		"JWKS and aud no issuer": {operatorJWKSURLEnv: complete[operatorJWKSURLEnv], operatorAudienceEnv: syntheticOperatorAudience},
		"issuer is not a URL":    {operatorIssuerEnv: "dex", operatorJWKSURLEnv: complete[operatorJWKSURLEnv], operatorAudienceEnv: syntheticOperatorAudience},
		"JWKS URL is a file":     {operatorIssuerEnv: syntheticOperatorIssuer, operatorJWKSURLEnv: "file:///keys.json", operatorAudienceEnv: syntheticOperatorAudience},
	}
	for name, values := range cases {
		t.Run(name, func(t *testing.T) {
			verifier, _, err := buildOperatorCredentialVerifier(fakeGetenv(values))
			if err == nil {
				t.Fatalf("parameters %v started with verifier %T; want refusal", values, verifier)
			}
		})
	}
}

func TestOperatorChannelWithAllIssuerParametersVerifiesTokensFromThatIssuer(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{"keys": []map[string]any{{
			"kty": "RSA", "kid": "syn-1", "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(jwks.Close)

	verifier, configured, err := buildOperatorCredentialVerifier(fakeGetenv(map[string]string{
		operatorIssuerEnv:   syntheticOperatorIssuer,
		operatorJWKSURLEnv:  jwks.URL,
		operatorAudienceEnv: syntheticOperatorAudience,
	}))
	if err != nil || !configured {
		t.Fatalf("complete parameters: configured=%v err=%v", configured, err)
	}
	token := signOperatorToken(t, key, "syn-1", map[string]any{
		"iss": syntheticOperatorIssuer,
		"aud": syntheticOperatorAudience,
		"sub": "SYN-OPERATOR-01",
		"exp": time.Now().Add(10 * time.Minute).Unix(),
	})
	subject, err := verifier.VerifyOperatorCredential(context.Background(), accessidentity.NewOperatorCredential(token))
	if err != nil {
		t.Fatalf("token from the configured issuer: %v", err)
	}
	if subject.Issuer() != syntheticOperatorIssuer || subject.Subject() != "SYN-OPERATOR-01" {
		t.Fatalf("subject = (%q, %q)", subject.Issuer(), subject.Subject())
	}
}

func signOperatorToken(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	encode := func(value any) string {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	input := encode(map[string]any{"alg": "RS256", "kid": kid, "typ": "JWT"}) + "." + encode(claims)
	digest := sha256.Sum256([]byte(input))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(signature)
}
