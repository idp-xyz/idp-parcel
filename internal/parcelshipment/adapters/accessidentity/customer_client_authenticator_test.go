package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	psaccess "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
)

const (
	customerIssuer  = "https://id.syn.example/customers"
	customerAccount = "SYN-ACCOUNT-01"
)

var customerNow = time.Date(2026, 10, 19, 3, 0, 0, 0, time.UTC)

type customerVerifier struct{ err error }

func (verifier customerVerifier) VerifyCustomerClientCredential(
	_ context.Context,
	credential identity.IntegrationClientCredential,
) (identity.VerifiedCustomerClientToken, error) {
	if verifier.err != nil {
		return identity.VerifiedCustomerClientToken{}, verifier.err
	}
	if credential.Token() == "" {
		return identity.VerifiedCustomerClientToken{}, identity.ErrCredentialRejected
	}
	subject, err := identity.NewCustomerClientSubject(customerIssuer, "SYN-CUSTOMER-CLIENT-01")
	if err != nil {
		return identity.VerifiedCustomerClientToken{}, err
	}
	return identity.NewVerifiedCustomerClientToken(subject, false), nil
}

type customerRegister struct {
	granted bool
	err     error
}

func (register customerRegister) FindCustomerClient(_ context.Context, subject identity.CustomerClientSubject) (identity.CustomerClientStanding, bool, error) {
	if register.err != nil {
		return identity.CustomerClientStanding{}, false, register.err
	}
	reference, err := identity.NewCredentialReference("SYN-CREDENTIAL-REF/customer-01")
	if err != nil {
		return identity.CustomerClientStanding{}, false, err
	}
	binding, err := identity.NewCustomerClientBinding(subject, operatorTenant, customerAccount, reference, false, "SYN-BASIS-BINDING")
	if err != nil {
		return identity.CustomerClientStanding{}, false, err
	}
	var grants []identity.RecordedCustomerClientGrant
	if register.granted {
		interval, err := identity.NewEffectiveInterval(customerNow.Add(-time.Hour), time.Time{})
		if err != nil {
			return identity.CustomerClientStanding{}, false, err
		}
		grant, err := identity.NewCustomerClientGrant(operatorTenant, "SYN-CUSTOMER-GRANT-01", subject,
			identity.CustomerCapabilityShipmentRequestSubmission, interval, "SYN-BASIS-GRANT")
		if err != nil {
			return identity.CustomerClientStanding{}, false, err
		}
		recorded, err := identity.NewRecordedCustomerClientGrant(grant, nil)
		if err != nil {
			return identity.CustomerClientStanding{}, false, err
		}
		grants = append(grants, recorded)
	}
	standing, err := identity.NewCustomerClientStanding(binding, grants)
	return standing, err == nil, err
}

func customerAuthenticator(t *testing.T, verifier customerVerifier, register customerRegister) *psaccess.CustomerClientAuthenticator {
	t.Helper()
	minter, err := identity.NewCustomerClientMinter(verifier, register, identity.UnconfiguredAdmissionScope{}, func() time.Time { return customerNow })
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err := psaccess.NewCustomerClientAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return authenticator
}

func accessRequest(id string) func() (string, error) {
	return func() (string, error) { return id, nil }
}

// Covers: 票 first-party-api/00 完成判据「正例铸出首方信封」——来源身份的四格取自首方信封：租户与客户账户是册上的绑定，来源
// 是客户端主体，来源请求键是接入请求标识。
func TestCustomerClientAuthenticatorTranslatesTheMintedEnvelope(t *testing.T) {
	authenticator := customerAuthenticator(t, customerVerifier{}, customerRegister{granted: true})
	source, err := authenticator.AuthenticateSubmission(context.Background(),
		shipmenthttp.PresentedClientCredential{Token: "presented.customer.token"}, accessRequest(" SYN-ACCESS-REQUEST-0001 "))
	if err != nil {
		t.Fatalf("granted customer client: %v", err)
	}
	if source.TenantID().String() != operatorTenant || source.CustomerAccountID().String() != customerAccount ||
		source.Source().String() != customerIssuer+"#SYN-CUSTOMER-CLIENT-01" || source.RequestKey().String() != "SYN-ACCESS-REQUEST-0001" {
		t.Fatalf("来源身份 = (%s, %s, %s, %s)", source.TenantID(), source.CustomerAccountID(), source.Source(), source.RequestKey())
	}
}

// Covers: ADR-0139 决定四——共享能力的每一格译成 shipmenthttp 的那一格，不合并；接入请求标识缺席答形状错；令牌不过时读载荷
// 的回调一次都不调。
func TestCustomerClientAuthenticatorAnswersEachGrade(t *testing.T) {
	for name, testCase := range map[string]struct {
		verifier customerVerifier
		register customerRegister
		token    string
		id       string
		want     error
	}{
		"issuer unset":      {customerVerifier{err: identity.ErrAccessChannelNotConfigured}, customerRegister{granted: true}, "t", "k", shipmenthttp.ErrAccessChannelNotConfigured},
		"no token":          {customerVerifier{}, customerRegister{granted: true}, "", "k", shipmenthttp.ErrCustomerClientCredentialRejected},
		"key set down":      {customerVerifier{err: identity.ErrCredentialVerifierUnavailable}, customerRegister{granted: true}, "t", "k", shipmenthttp.ErrIdentityDependencyUnavailable},
		"no grant":          {customerVerifier{}, customerRegister{}, "t", "k", shipmenthttp.ErrCustomerClientNotGranted},
		"register down":     {customerVerifier{}, customerRegister{err: errors.New("connection refused")}, "t", "k", shipmenthttp.ErrIdentityDependencyUnavailable},
		"no access request": {customerVerifier{}, customerRegister{granted: true}, "t", "  ", shipmenthttp.ErrMalformedRequest},
	} {
		t.Run(name, func(t *testing.T) {
			authenticator := customerAuthenticator(t, testCase.verifier, testCase.register)
			called := false
			_, err := authenticator.AuthenticateSubmission(context.Background(),
				shipmenthttp.PresentedClientCredential{Token: testCase.token}, func() (string, error) {
					called = true
					return testCase.id, nil
				})
			if !errors.Is(err, testCase.want) {
				t.Fatalf("err = %v, want %v", err, testCase.want)
			}
			if testCase.want != shipmenthttp.ErrMalformedRequest && called {
				t.Fatal("认证没过却读了载荷")
			}
		})
	}

	malformed := errors.New("payload is not JSON")
	authenticator := customerAuthenticator(t, customerVerifier{}, customerRegister{granted: true})
	_, err := authenticator.AuthenticateSubmission(context.Background(), shipmenthttp.PresentedClientCredential{Token: "t"},
		func() (string, error) { return "", malformed })
	if !errors.Is(err, malformed) {
		t.Fatalf("读载荷的错 = %v, want 原样交回", err)
	}
}
