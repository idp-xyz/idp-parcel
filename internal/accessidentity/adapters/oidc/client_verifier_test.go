package oidc_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net/http"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/accessidentity/adapters/oidc"
)

// 集成客户端族的令牌由同一类发行方替身签出；受众取本产品 API 的资源标识，与操作者族的管理台受众（testAudience）不同。
const (
	clientAudience = "https://api.syn.example/idp-parcel"
	clientSubject  = "SYN-CLIENT-01"
)

func newClientVerifier(t *testing.T, keySet *keySetDouble, clock *time.Time) *oidc.ClientVerifier {
	t.Helper()
	verifier, err := oidc.NewClientVerifier(
		oidc.Config{Issuer: testIssuer, JWKSURL: keySet.url(), Audience: clientAudience},
		&http.Client{Timeout: 5 * time.Second},
		func() time.Time { return *clock },
	)
	if err != nil {
		t.Fatalf("NewClientVerifier: %v", err)
	}
	return verifier
}

func clientClaims() map[string]any {
	return map[string]any{
		"iss":       testIssuer,
		"aud":       clientAudience,
		"sub":       clientSubject,
		"client_id": clientSubject,
		"iat":       testNow.Add(-time.Minute).Unix(),
		"exp":       testNow.Add(10 * time.Minute).Unix(),
	}
}

func verifyClient(verifier *oidc.ClientVerifier, token string, certificate []byte) (accessidentity.VerifiedClientToken, error) {
	return verifier.VerifyIntegrationClientCredential(context.Background(), accessidentity.NewIntegrationClientCredential(token, certificate))
}

// selfSignedCertificate 造一张客户端证书（DER）。核对只看证书的 DER 摘要，证书链与有效期不归令牌校验管——那是终止
// TLS 的那一层的事。
func selfSignedCertificate(t *testing.T, commonName string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    testNow.Add(-time.Hour),
		NotAfter:     testNow.Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// thumbprint 是 RFC 8705 第 3.1 节的 x5t#S256：证书 DER 的 SHA-256，base64url 不带填充。
func thumbprint(der []byte) string {
	sum := sha256.Sum256(der)
	return b64(sum[:])
}

// Covers: 票面做什么第 2 条「OAuth 2.0 client credentials 令牌校验」——合格令牌交回（发行方、sub）为客户端主体；
// 不带 cnf 的是持有者令牌，答未绑定。
func TestQualifiedClientTokenYieldsTheClientSubject(t *testing.T) {
	keySet := newKeySetDouble(t, rsaKeyA())
	now := testNow
	verifier := newClientVerifier(t, keySet, &now)

	verified, err := verifyClient(verifier, sign(t, rsaKeyA(), rsaKeyA().header(), clientClaims()), nil)
	if err != nil {
		t.Fatalf("qualified client token: %v", err)
	}
	if verified.Subject().Issuer() != testIssuer || verified.Subject().Subject() != clientSubject || verified.CertificateBound() {
		t.Fatalf("verified = (%q, %q, bound %v), want (%q, %q, unbound)",
			verified.Subject().Issuer(), verified.Subject().Subject(), verified.CertificateBound(), testIssuer, clientSubject)
	}
}

// Covers: 票面「方式同 02」——签名、iss、aud、有效期与 sub 的核验与操作者族是同一段；发给管理台的操作者令牌受众不是本产品
// API，交到集成客户端族这一侧不过。
func TestClientTokenIsVerifiedLikeAnOperatorTokenFirst(t *testing.T) {
	cases := map[string]func(t *testing.T) string{
		"token is absent": func(*testing.T) string { return "" },
		"signed by a key the issuer did not publish": func(t *testing.T) string {
			return sign(t, rsaKeyB(), rsaKeyA().header(), clientClaims())
		},
		"issued by another issuer": func(t *testing.T) string {
			claims := clientClaims()
			claims["iss"] = "https://id.syn.example/other"
			return sign(t, rsaKeyA(), rsaKeyA().header(), claims)
		},
		"operator token for the admin console": func(t *testing.T) string {
			return sign(t, rsaKeyA(), rsaKeyA().header(), qualifiedClaims())
		},
		"expired": func(t *testing.T) string {
			claims := clientClaims()
			claims["exp"] = testNow.Add(-time.Second).Unix()
			return sign(t, rsaKeyA(), rsaKeyA().header(), claims)
		},
		"sub is absent": func(t *testing.T) string {
			claims := clientClaims()
			delete(claims, "sub")
			return sign(t, rsaKeyA(), rsaKeyA().header(), claims)
		},
		"alg is HS256 keyed by the published RSA key": func(t *testing.T) string {
			return signHS256WithPublishedRSAKey(t, rsaKeyA(), clientClaims())
		},
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			keySet := newKeySetDouble(t, rsaKeyA())
			now := testNow
			verifier := newClientVerifier(t, keySet, &now)

			_, err := verifyClient(verifier, token(t), nil)
			assertRejected(t, err)
		})
	}
}

// Covers: 票面做什么第 2 条「可选 mTLS 绑定」（RFC 8705 第 3 节）——令牌带 cnf 的 x5t#S256 时，只有这次连接出示的正是它
// 绑定的那张证书才过、并答已绑定；换一张证书、没出示证书、摘要不成形都拒。带本校验器验不了的确认方法（如 DPoP 的
// jkt）同样拒：那枚令牌不是持有者令牌，当持有者令牌收就丢了它要求的持有证明。
func TestCertificateBoundClientTokenNeedsTheCertificateItIsBoundTo(t *testing.T) {
	bound := selfSignedCertificate(t, "SYN-CLIENT-01")
	other := selfSignedCertificate(t, "SYN-CLIENT-02")
	withConfirmation := func(cnf any) func(*testing.T) string {
		return func(t *testing.T) string {
			claims := clientClaims()
			claims["cnf"] = cnf
			return sign(t, rsaKeyA(), rsaKeyA().header(), claims)
		}
	}
	boundToken := withConfirmation(map[string]any{"x5t#S256": thumbprint(bound)})

	t.Run("the bound certificate is presented", func(t *testing.T) {
		keySet := newKeySetDouble(t, rsaKeyA())
		now := testNow
		verified, err := verifyClient(newClientVerifier(t, keySet, &now), boundToken(t), bound)
		if err != nil || !verified.CertificateBound() || verified.Subject().Subject() != clientSubject {
			t.Fatalf("bound certificate: bound %v, sub %q, err %v; want a bound verification", verified.CertificateBound(), verified.Subject().Subject(), err)
		}
	})

	refused := map[string]struct {
		token       func(*testing.T) string
		certificate []byte
	}{
		"another certificate is presented":        {token: boundToken, certificate: other},
		"no certificate is presented":             {token: boundToken},
		"thumbprint is not base64url":             {token: withConfirmation(map[string]any{"x5t#S256": "not base64url!"}), certificate: bound},
		"thumbprint is not a SHA-256 digest":      {token: withConfirmation(map[string]any{"x5t#S256": b64([]byte("short"))}), certificate: bound},
		"thumbprint is not a string":              {token: withConfirmation(map[string]any{"x5t#S256": 42}), certificate: bound},
		"confirmation is a DPoP key thumbprint":   {token: withConfirmation(map[string]any{"jkt": thumbprint(bound)}), certificate: bound},
		"confirmation adds a method beside x5t":   {token: withConfirmation(map[string]any{"x5t#S256": thumbprint(bound), "jkt": "x"}), certificate: bound},
		"confirmation names no method":            {token: withConfirmation(map[string]any{}), certificate: bound},
		"confirmation is not a JSON object":       {token: withConfirmation("x5t"), certificate: bound},
		"confirmation is null instead of omitted": {token: withConfirmation(nil), certificate: bound},
	}
	for name, testCase := range refused {
		t.Run(name, func(t *testing.T) {
			keySet := newKeySetDouble(t, rsaKeyA())
			now := testNow
			_, err := verifyClient(newClientVerifier(t, keySet, &now), testCase.token(t), testCase.certificate)
			assertRejected(t, err)
		})
	}
}

// Covers: 取不回发行方公钥集答依赖故障而不是令牌不过，与操作者族同（票 operator-channel/02 完成判据）。
func TestUnreachableKeySetIsADependencyFailureForClientTokens(t *testing.T) {
	keySet := newKeySetDouble(t, rsaKeyA())
	keySet.fail()
	now := testNow
	verifier := newClientVerifier(t, keySet, &now)

	_, err := verifyClient(verifier, sign(t, rsaKeyA(), rsaKeyA().header(), clientClaims()), nil)
	if !errors.Is(err, accessidentity.ErrCredentialVerifierUnavailable) || errors.Is(err, accessidentity.ErrCredentialRejected) {
		t.Fatalf("err = %v, want ErrCredentialVerifierUnavailable and not ErrCredentialRejected", err)
	}
}

func TestClientVerifierIsNotBuiltFromIncompleteParameters(t *testing.T) {
	for name, config := range map[string]oidc.Config{
		"audience absent": {Issuer: testIssuer, JWKSURL: "https://id.syn.example/dex/keys"},
		"JWKS URL absent": {Issuer: testIssuer, Audience: clientAudience},
	} {
		if _, err := oidc.NewClientVerifier(config, &http.Client{}, time.Now); !errors.Is(err, oidc.ErrInvalidConfig) {
			t.Fatalf("%s: err = %v, want ErrInvalidConfig", name, err)
		}
	}
	config := oidc.Config{Issuer: testIssuer, JWKSURL: "https://id.syn.example/dex/keys", Audience: clientAudience}
	if _, err := oidc.NewClientVerifier(config, nil, time.Now); err == nil {
		t.Fatal("client verifier built without an HTTP client")
	}
	if _, err := oidc.NewClientVerifier(config, &http.Client{}, nil); err == nil {
		t.Fatal("client verifier built without a clock")
	}
}
