package oidc_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/accessidentity/adapters/oidc"
)

func newVerifier(t *testing.T, keySet *keySetDouble, clock *time.Time) *oidc.Verifier {
	t.Helper()
	verifier, err := oidc.NewVerifier(
		oidc.Config{Issuer: testIssuer, JWKSURL: keySet.url(), Audience: testAudience},
		&http.Client{Timeout: 5 * time.Second},
		func() time.Time { return *clock },
	)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	return verifier
}

func verify(verifier *oidc.Verifier, token string) (accessidentity.OperatorSubject, error) {
	return verifier.VerifyOperatorCredential(context.Background(), accessidentity.NewOperatorCredential(token))
}

func TestQualifiedTokenYieldsTheOperatorSubject(t *testing.T) {
	keySet := newKeySetDouble(t, rsaKeyA())
	now := testNow
	verifier := newVerifier(t, keySet, &now)

	subject, err := verify(verifier, sign(t, rsaKeyA(), rsaKeyA().header(), qualifiedClaims()))
	if err != nil {
		t.Fatalf("qualified token: %v", err)
	}
	if subject.Issuer() != testIssuer || subject.Subject() != testSubject {
		t.Fatalf("subject = (%q, %q), want (%q, %q)", subject.Issuer(), subject.Subject(), testIssuer, testSubject)
	}
}

func assertRejected(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, accessidentity.ErrCredentialRejected) {
		t.Fatalf("err = %v, want ErrCredentialRejected", err)
	}
}

func TestTokenSignedByAKeyTheIssuerDidNotPublishIsRejected(t *testing.T) {
	keySet := newKeySetDouble(t, rsaKeyA())
	now := testNow
	verifier := newVerifier(t, keySet, &now)

	_, err := verify(verifier, sign(t, rsaKeyB(), rsaKeyA().header(), qualifiedClaims()))
	assertRejected(t, err)
}

func TestTokenIssuedForAnotherDeploymentIsRejected(t *testing.T) {
	cases := map[string]func(map[string]any){
		"iss is another issuer":         func(c map[string]any) { c["iss"] = "https://id.syn.example/other" },
		"iss differs by trailing slash": func(c map[string]any) { c["iss"] = testIssuer + "/" },
		"aud is another audience":       func(c map[string]any) { c["aud"] = "another-client" },
		"aud list lacks this audience":  func(c map[string]any) { c["aud"] = []string{"another-client", "third-client"} },
		"aud is absent":                 func(c map[string]any) { delete(c, "aud") },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			keySet := newKeySetDouble(t, rsaKeyA())
			now := testNow
			verifier := newVerifier(t, keySet, &now)
			claims := qualifiedClaims()
			mutate(claims)

			_, err := verify(verifier, sign(t, rsaKeyA(), rsaKeyA().header(), claims))
			assertRejected(t, err)
		})
	}
}

func TestTokenOutsideItsValidityWindowIsRejected(t *testing.T) {
	cases := map[string]func(map[string]any){
		"exp has passed":        func(c map[string]any) { c["exp"] = testNow.Add(-time.Second).Unix() },
		"exp is now":            func(c map[string]any) { c["exp"] = testNow.Unix() },
		"exp is absent":         func(c map[string]any) { delete(c, "exp") },
		"nbf is still to come":  func(c map[string]any) { c["nbf"] = testNow.Add(10 * time.Minute).Unix() },
		"exp is not a number":   func(c map[string]any) { c["exp"] = "tomorrow" },
		"sub is absent":         func(c map[string]any) { delete(c, "sub") },
		"sub is blank":          func(c map[string]any) { c["sub"] = "  " },
		"iss is absent":         func(c map[string]any) { delete(c, "iss") },
		"claims are not object": nil,
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			keySet := newKeySetDouble(t, rsaKeyA())
			now := testNow
			verifier := newVerifier(t, keySet, &now)
			var token string
			if mutate == nil {
				token = signRawClaims(t, rsaKeyA(), []byte(`["not", "an", "object"]`))
			} else {
				claims := qualifiedClaims()
				mutate(claims)
				token = sign(t, rsaKeyA(), rsaKeyA().header(), claims)
			}

			_, err := verify(verifier, token)
			assertRejected(t, err)
		})
	}
}

func TestMissingOrMalformedTokenIsRejected(t *testing.T) {
	qualified := func(t *testing.T) string { return sign(t, rsaKeyA(), rsaKeyA().header(), qualifiedClaims()) }
	withHeader := func(mutate func(map[string]any)) func(*testing.T) string {
		return func(t *testing.T) string {
			header := rsaKeyA().header()
			mutate(header)
			return sign(t, rsaKeyA(), header, qualifiedClaims())
		}
	}
	cases := map[string]func(*testing.T) string{
		"token is empty":          func(*testing.T) string { return "" },
		"token is blank":          func(*testing.T) string { return "   " },
		"token has two segments":  func(t *testing.T) string { return strings.Join(strings.Split(qualified(t), ".")[:2], ".") },
		"token has four segments": func(t *testing.T) string { return qualified(t) + ".extra" },
		"signature is not base64": func(t *testing.T) string {
			segments := strings.Split(qualified(t), ".")
			return segments[0] + "." + segments[1] + ".!!!"
		},
		"header is not JSON": func(t *testing.T) string {
			segments := strings.Split(qualified(t), ".")
			return b64([]byte("not json")) + "." + segments[1] + "." + segments[2]
		},
		"alg is none":                 withHeader(func(h map[string]any) { h["alg"] = "none" }),
		"alg is absent":               withHeader(func(h map[string]any) { delete(h, "alg") }),
		"alg is not the key's alg":    withHeader(func(h map[string]any) { h["alg"] = "ES256" }),
		"header carries a crit claim": withHeader(func(h map[string]any) { h["crit"] = []string{"exp"} }),
		"alg is HS256 keyed by the published RSA key": func(t *testing.T) string {
			return signHS256WithPublishedRSAKey(t, rsaKeyA(), qualifiedClaims())
		},
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			keySet := newKeySetDouble(t, rsaKeyA())
			now := testNow
			verifier := newVerifier(t, keySet, &now)

			_, err := verify(verifier, token(t))
			assertRejected(t, err)
		})
	}
}

func TestUnreachableKeySetIsADependencyFailureNotARejection(t *testing.T) {
	cases := map[string]func(*keySetDouble){
		"issuer answers 503": func(keySet *keySetDouble) { keySet.fail() },
		"issuer is down":     func(keySet *keySetDouble) { keySet.server.Close() },
		"issuer serves junk": func(keySet *keySetDouble) { keySet.serveJunk() },
	}
	for name, breakIssuer := range cases {
		t.Run(name, func(t *testing.T) {
			keySet := newKeySetDouble(t, rsaKeyA())
			now := testNow
			verifier := newVerifier(t, keySet, &now)
			breakIssuer(keySet)

			_, err := verify(verifier, sign(t, rsaKeyA(), rsaKeyA().header(), qualifiedClaims()))
			if !errors.Is(err, accessidentity.ErrCredentialVerifierUnavailable) {
				t.Fatalf("err = %v, want ErrCredentialVerifierUnavailable", err)
			}
			if errors.Is(err, accessidentity.ErrCredentialRejected) {
				t.Fatalf("err = %v also reads as ErrCredentialRejected; the holder's token is not at fault", err)
			}
		})
	}
}

func TestES256TokenFromAPublishedECKeyIsAccepted(t *testing.T) {
	keySet := newKeySetDouble(t, rsaKeyA(), ecKeyA())
	now := testNow
	verifier := newVerifier(t, keySet, &now)

	subject, err := verify(verifier, sign(t, ecKeyA(), ecKeyA().header(), qualifiedClaims()))
	if err != nil {
		t.Fatalf("ES256 token: %v", err)
	}
	if subject.Subject() != testSubject {
		t.Fatalf("sub = %q, want %q", subject.Subject(), testSubject)
	}
}

func TestKeySetIsFetchedOnceAndReusedAcrossVerifications(t *testing.T) {
	keySet := newKeySetDouble(t, rsaKeyA())
	now := testNow
	verifier := newVerifier(t, keySet, &now)

	for range 3 {
		if _, err := verify(verifier, sign(t, rsaKeyA(), rsaKeyA().header(), qualifiedClaims())); err != nil {
			t.Fatalf("qualified token: %v", err)
		}
	}
	if got := keySet.fetchCount(); got != 1 {
		t.Fatalf("JWKS fetched %d times for three verifications, want 1", got)
	}
}

func TestRotatedKeyIsPickedUpByRefetchingTheKeySet(t *testing.T) {
	keySet := newKeySetDouble(t, rsaKeyA())
	now := testNow
	verifier := newVerifier(t, keySet, &now)
	if _, err := verify(verifier, sign(t, rsaKeyA(), rsaKeyA().header(), qualifiedClaims())); err != nil {
		t.Fatalf("token under the first key: %v", err)
	}

	keySet.publish(rsaKeyB())
	now = testNow.Add(time.Hour)
	claims := qualifiedClaims()
	claims["exp"] = now.Add(10 * time.Minute).Unix()
	if _, err := verify(verifier, sign(t, rsaKeyB(), rsaKeyB().header(), claims)); err != nil {
		t.Fatalf("token under the rotated key: %v", err)
	}
}

func TestUnknownKeyIDDoesNotMakeEveryVerificationHitTheIssuer(t *testing.T) {
	keySet := newKeySetDouble(t, rsaKeyA())
	now := testNow
	verifier := newVerifier(t, keySet, &now)
	stranger := sign(t, rsaKeyB(), rsaKeyB().header(), qualifiedClaims())

	for range 3 {
		_, err := verify(verifier, stranger)
		assertRejected(t, err)
	}
	if got := keySet.fetchCount(); got != 1 {
		t.Fatalf("JWKS fetched %d times for three unknown-kid tokens in a row, want 1", got)
	}

	now = testNow.Add(2 * time.Minute)
	_, err := verify(verifier, stranger)
	assertRejected(t, err)
	if got := keySet.fetchCount(); got != 2 {
		t.Fatalf("JWKS fetched %d times after the refetch interval, want 2", got)
	}
}

func TestCachedKeysKeepVerifyingWhileTheIssuerIsDown(t *testing.T) {
	keySet := newKeySetDouble(t, rsaKeyA())
	now := testNow
	verifier := newVerifier(t, keySet, &now)
	token := sign(t, rsaKeyA(), rsaKeyA().header(), qualifiedClaims())
	if _, err := verify(verifier, token); err != nil {
		t.Fatalf("token while the issuer is up: %v", err)
	}

	keySet.fail()
	now = testNow.Add(5 * time.Minute)
	if _, err := verify(verifier, token); err != nil {
		t.Fatalf("token under a cached key while the issuer is down: %v", err)
	}
}

func TestFailedFetchIsNotRetriedOnEveryVerification(t *testing.T) {
	keySet := newKeySetDouble(t, rsaKeyA())
	keySet.fail()
	now := testNow
	verifier := newVerifier(t, keySet, &now)
	token := sign(t, rsaKeyA(), rsaKeyA().header(), qualifiedClaims())

	for range 3 {
		if _, err := verify(verifier, token); !errors.Is(err, accessidentity.ErrCredentialVerifierUnavailable) {
			t.Fatalf("err = %v, want ErrCredentialVerifierUnavailable", err)
		}
	}
	if got := keySet.fetchCount(); got != 1 {
		t.Fatalf("JWKS fetched %d times while the issuer stays down, want 1", got)
	}
}

func TestTokenWithoutKeyIDNeedsASingleKeySet(t *testing.T) {
	unnamed := func(key signingKey) map[string]any {
		header := key.header()
		delete(header, "kid")
		return header
	}
	t.Run("single key", func(t *testing.T) {
		keySet := newKeySetDouble(t, rsaKeyA())
		now := testNow
		verifier := newVerifier(t, keySet, &now)
		if _, err := verify(verifier, sign(t, rsaKeyA(), unnamed(rsaKeyA()), qualifiedClaims())); err != nil {
			t.Fatalf("kid-less token against a single-key set: %v", err)
		}
	})
	t.Run("several keys", func(t *testing.T) {
		keySet := newKeySetDouble(t, rsaKeyA(), rsaKeyB())
		now := testNow
		verifier := newVerifier(t, keySet, &now)
		_, err := verify(verifier, sign(t, rsaKeyA(), unnamed(rsaKeyA()), qualifiedClaims()))
		assertRejected(t, err)
	})
}

func TestVerifierIsNotBuiltFromIncompleteOrMalformedParameters(t *testing.T) {
	cases := map[string]func(*oidc.Config){
		"issuer absent":                 func(c *oidc.Config) { c.Issuer = "" },
		"JWKS URL absent":               func(c *oidc.Config) { c.JWKSURL = "" },
		"audience absent":               func(c *oidc.Config) { c.Audience = "" },
		"audience blank":                func(c *oidc.Config) { c.Audience = "   " },
		"issuer is not an absolute URL": func(c *oidc.Config) { c.Issuer = "id.syn.example/dex" },
		"issuer carries a query":        func(c *oidc.Config) { c.Issuer = testIssuer + "?tenant=x" },
		"JWKS URL is not http(s)":       func(c *oidc.Config) { c.JWKSURL = "file:///etc/keys.json" },
		"JWKS URL has no host":          func(c *oidc.Config) { c.JWKSURL = "https:///keys" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			config := oidc.Config{Issuer: testIssuer, JWKSURL: "https://id.syn.example/dex/keys", Audience: testAudience}
			mutate(&config)
			if _, err := oidc.NewVerifier(config, &http.Client{}, time.Now); !errors.Is(err, oidc.ErrInvalidConfig) {
				t.Fatalf("err = %v, want ErrInvalidConfig", err)
			}
		})
	}
	config := oidc.Config{Issuer: testIssuer, JWKSURL: "https://id.syn.example/dex/keys", Audience: testAudience}
	if _, err := oidc.NewVerifier(config, nil, time.Now); err == nil {
		t.Fatal("verifier built without an HTTP client")
	}
	if _, err := oidc.NewVerifier(config, &http.Client{}, nil); err == nil {
		t.Fatal("verifier built without a clock")
	}
}

func TestAudienceListContainingThisAudienceIsAccepted(t *testing.T) {
	keySet := newKeySetDouble(t, rsaKeyA())
	now := testNow
	verifier := newVerifier(t, keySet, &now)
	claims := qualifiedClaims()
	claims["aud"] = []string{"another-client", testAudience}

	if _, err := verify(verifier, sign(t, rsaKeyA(), rsaKeyA().header(), claims)); err != nil {
		t.Fatalf("aud list containing %q: %v", testAudience, err)
	}
}
