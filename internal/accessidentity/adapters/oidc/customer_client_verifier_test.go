package oidc_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/accessidentity/adapters/oidc"
)

// 首方客户族的令牌由同一类发行方替身签出；受众取本产品客户 API 的资源标识，与操作者族的管理台受众（testAudience）、集成
// 客户端族的受众（clientAudience）都不同（ADR-0139 决定二）。
const (
	customerAudience = "https://api.syn.example/idp-parcel/customers"
	customerSubject  = "SYN-CUSTOMER-CLIENT-01"
)

func newCustomerVerifier(t *testing.T, keySet *keySetDouble, clock *time.Time) *oidc.CustomerClientVerifier {
	t.Helper()
	verifier, err := oidc.NewCustomerClientVerifier(
		oidc.Config{Issuer: testIssuer, JWKSURL: keySet.url(), Audience: customerAudience},
		&http.Client{Timeout: 5 * time.Second},
		func() time.Time { return *clock },
	)
	if err != nil {
		t.Fatalf("NewCustomerClientVerifier: %v", err)
	}
	return verifier
}

func customerClaims() map[string]any {
	claims := clientClaims()
	claims["aud"] = customerAudience
	claims["sub"] = customerSubject
	claims["client_id"] = customerSubject
	return claims
}

func verifyCustomer(verifier *oidc.CustomerClientVerifier, token string, certificate []byte) (accessidentity.VerifiedCustomerClientToken, error) {
	return verifier.VerifyCustomerClientCredential(context.Background(), accessidentity.NewIntegrationClientCredential(token, certificate))
}

// Covers: ADR-0139 决定二「复用集成客户端族的凭据机制」——合格令牌交回（发行方、sub）为首方客户端主体；证书绑定照集成客户端
// 族那一段核，绑了且对得上答已绑定。
func TestQualifiedCustomerTokenYieldsTheCustomerClientSubject(t *testing.T) {
	keySet := newKeySetDouble(t, rsaKeyA())
	now := testNow
	verifier := newCustomerVerifier(t, keySet, &now)

	verified, err := verifyCustomer(verifier, sign(t, rsaKeyA(), rsaKeyA().header(), customerClaims()), nil)
	if err != nil {
		t.Fatalf("qualified customer token: %v", err)
	}
	if verified.Subject().Issuer() != testIssuer || verified.Subject().Subject() != customerSubject || verified.CertificateBound() {
		t.Fatalf("verified = (%q, %q, bound %v)", verified.Subject().Issuer(), verified.Subject().Subject(), verified.CertificateBound())
	}

	certificate := selfSignedCertificate(t, customerSubject)
	claims := customerClaims()
	claims["cnf"] = map[string]any{"x5t#S256": thumbprint(certificate)}
	bound, err := verifyCustomer(verifier, sign(t, rsaKeyA(), rsaKeyA().header(), claims), certificate)
	if err != nil || !bound.CertificateBound() {
		t.Fatalf("certificate-bound token: bound %v, err %v", bound.CertificateBound(), err)
	}
	if _, err := verifyCustomer(verifier, sign(t, rsaKeyA(), rsaKeyA().header(), claims), selfSignedCertificate(t, "SYN-OTHER")); !errors.Is(err, accessidentity.ErrCredentialRejected) {
		t.Fatalf("另一张证书：err = %v, want ErrCredentialRejected", err)
	}
}

// Covers: 票 first-party-api/00 完成判据「他族令牌（操作者、集成客户端）在首方口一律拒」——同一个发行方签的操作者令牌与集成
// 客户端令牌受众都不是首方族那一个，过不了校验期；取不回公钥集答依赖故障。
func TestOtherFamiliesTokensDoNotPassTheCustomerVerifier(t *testing.T) {
	for name, claims := range map[string]map[string]any{
		"operator token for the admin console": qualifiedClaims(),
		"integration client token":             clientClaims(),
	} {
		t.Run(name, func(t *testing.T) {
			keySet := newKeySetDouble(t, rsaKeyA())
			now := testNow
			_, err := verifyCustomer(newCustomerVerifier(t, keySet, &now), sign(t, rsaKeyA(), rsaKeyA().header(), claims), nil)
			assertRejected(t, err)
		})
	}

	keySet := newKeySetDouble(t, rsaKeyA())
	keySet.fail()
	now := testNow
	_, err := verifyCustomer(newCustomerVerifier(t, keySet, &now), sign(t, rsaKeyA(), rsaKeyA().header(), customerClaims()), nil)
	if !errors.Is(err, accessidentity.ErrCredentialVerifierUnavailable) || errors.Is(err, accessidentity.ErrCredentialRejected) {
		t.Fatalf("err = %v, want ErrCredentialVerifierUnavailable and not ErrCredentialRejected", err)
	}
}

func TestCustomerVerifierRefusesIncompleteParameters(t *testing.T) {
	if _, err := oidc.NewCustomerClientVerifier(oidc.Config{Issuer: testIssuer, Audience: customerAudience}, http.DefaultClient, time.Now); !errors.Is(err, oidc.ErrInvalidConfig) {
		t.Fatalf("缺 JWKS：err = %v, want ErrInvalidConfig", err)
	}
}
