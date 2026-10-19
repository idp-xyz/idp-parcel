package accessidentity_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	noaccess "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/accessidentity"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
)

// 本文件证送站预报两口的首方认证适配（票 ps-external-mark-relations/09；ADR-0139 决定四）：经真的首方铸造器铸预报两种信封，把
// 四要素译成本上下文的客户身份；共享接入身份能力的各格逐格译成 nodeopshttp 的格。核验方、客户渠道册与准入范围是替身。夹具全为
// 合成（S 级）。

var customerNow = time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC)

const (
	customerIssuer = "https://auth.syn.example/customers"
	customerTenant = "SYN-TENANT-01"
)

type customerVerifier struct{ err error }

func (verifier customerVerifier) VerifyCustomerClientCredential(context.Context, identity.IntegrationClientCredential) (identity.VerifiedCustomerClientToken, error) {
	if verifier.err != nil {
		return identity.VerifiedCustomerClientToken{}, verifier.err
	}
	subject, err := identity.NewCustomerClientSubject(customerIssuer, "SYN-CUSTOMER-CLIENT-01")
	if err != nil {
		return identity.VerifiedCustomerClientToken{}, err
	}
	return identity.NewVerifiedCustomerClientToken(subject, false), nil
}

// customerRegister 是客户渠道册的替身：主体绑在（customerTenant, SYN-ACCOUNT-01）上，持 capabilities 列出的各项授予。
type customerRegister struct {
	capabilities []identity.CustomerCapability
	err          error
}

func (register customerRegister) FindCustomerClient(_ context.Context, subject identity.CustomerClientSubject) (identity.CustomerClientStanding, bool, error) {
	if register.err != nil {
		return identity.CustomerClientStanding{}, false, register.err
	}
	reference, err := identity.NewCredentialReference("SYN-CREDENTIAL-REF/customer-01")
	if err != nil {
		return identity.CustomerClientStanding{}, false, err
	}
	binding, err := identity.NewCustomerClientBinding(subject, customerTenant, "SYN-ACCOUNT-01", reference, false, "SYN-BASIS-BINDING")
	if err != nil {
		return identity.CustomerClientStanding{}, false, err
	}
	interval, err := identity.NewEffectiveInterval(customerNow.Add(-time.Hour), time.Time{})
	if err != nil {
		return identity.CustomerClientStanding{}, false, err
	}
	var grants []identity.RecordedCustomerClientGrant
	for index, capability := range register.capabilities {
		grant, err := identity.NewCustomerClientGrant(customerTenant, fmt.Sprintf("SYN-CUSTOMER-GRANT-%d", index), subject, capability, interval, "SYN-BASIS-GRANT")
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

type customerAdmission struct {
	admitted bool
	err      error
}

func (admission customerAdmission) Admits(context.Context, string, identity.AdmissionRequirement, time.Time) (bool, error) {
	return admission.admitted, admission.err
}

func customerAuthenticator(t *testing.T, verifier customerVerifier, register customerRegister, admission customerAdmission) *noaccess.CustomerPreAdviceAuthenticator {
	t.Helper()
	minter, err := identity.NewCustomerClientMinter(verifier, register, admission, func() time.Time { return customerNow })
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err := noaccess.NewCustomerPreAdviceAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return authenticator
}

type preAdviceAuthentication func(*noaccess.CustomerPreAdviceAuthenticator, func() (string, error)) (nodeopshttp.CustomerIdentity, error)

var preAdviceAuthentications = map[identity.CustomerCapability]preAdviceAuthentication{
	identity.CustomerCapabilityDeliveryPreAdviceSubmission: func(authenticator *noaccess.CustomerPreAdviceAuthenticator, id func() (string, error)) (nodeopshttp.CustomerIdentity, error) {
		return authenticator.AuthenticateDeliveryPreAdviceSubmission(context.Background(), nodeopshttp.PresentedCustomerCredential{Token: "t"}, id)
	},
	identity.CustomerCapabilityDeliveryPreAdviceWithdrawal: func(authenticator *noaccess.CustomerPreAdviceAuthenticator, id func() (string, error)) (nodeopshttp.CustomerIdentity, error) {
		return authenticator.AuthenticateDeliveryPreAdviceWithdrawal(context.Background(), nodeopshttp.PresentedCustomerCredential{Token: "t"}, id)
	},
}

// Covers: AT-NO-069（认证半边）——持这一口授予、落在准入范围内的客户端，交回的客户身份整组取自册上的绑定：租户、客户账户，客户端是
// 发行方 # 主体，来源请求键是接入请求标识去两端空白。
func TestCustomerPreAdviceAuthenticatorTranslatesTheEnvelope(t *testing.T) {
	for capability, authenticate := range preAdviceAuthentications {
		t.Run(capability.String(), func(t *testing.T) {
			authenticator := customerAuthenticator(t, customerVerifier{}, customerRegister{capabilities: []identity.CustomerCapability{capability}}, customerAdmission{admitted: true})
			customer, err := authenticate(authenticator, func() (string, error) { return " SYN-ACCESS-REQUEST-PA-0001 ", nil })
			if err != nil {
				t.Fatal(err)
			}
			if customer.Tenant.String() != customerTenant || customer.CustomerAccount.String() != "SYN-ACCOUNT-01" ||
				customer.Channel != customerIssuer+"#SYN-CUSTOMER-CLIENT-01" || customer.RequestKey != "SYN-ACCESS-REQUEST-PA-0001" {
				t.Fatalf("客户身份 = %+v", customer)
			}
		})
	}
}

// Covers: 票 ps-external-mark-relations/09 完成判据各信封格——共享能力的格各译成 nodeopshttp 的一格、不互相顶替；缺接入请求标识译成
// 坏载荷；读载荷自己的错原样过。
func TestCustomerPreAdviceAuthenticatorTranslatesEveryGrade(t *testing.T) {
	malformed := fmt.Errorf("%w: payload is not JSON", nodeopshttp.ErrMalformedRequest)
	for capability, authenticate := range preAdviceAuthentications {
		granted := customerRegister{capabilities: []identity.CustomerCapability{capability}}
		for name, testCase := range map[string]struct {
			verifier  customerVerifier
			register  customerRegister
			admission customerAdmission
			id        func() (string, error)
			want      error
		}{
			"issuer unset":         {customerVerifier{err: identity.ErrAccessChannelNotConfigured}, granted, customerAdmission{admitted: true}, nil, nodeopshttp.ErrAccessChannelNotConfigured},
			"token rejected":       {customerVerifier{err: identity.ErrCredentialRejected}, granted, customerAdmission{admitted: true}, nil, nodeopshttp.ErrCustomerClientCredentialRejected},
			"key set unreachable":  {customerVerifier{err: identity.ErrCredentialVerifierUnavailable}, granted, customerAdmission{admitted: true}, nil, nodeopshttp.ErrIdentityDependencyUnavailable},
			"not granted":          {customerVerifier{}, customerRegister{}, customerAdmission{admitted: true}, nil, nodeopshttp.ErrCustomerClientNotGranted},
			"register unreadable":  {customerVerifier{}, customerRegister{err: errors.New("connection refused")}, customerAdmission{admitted: true}, nil, nodeopshttp.ErrIdentityDependencyUnavailable},
			"outside admission":    {customerVerifier{}, granted, customerAdmission{}, nil, nodeopshttp.ErrOutsideAdmissionScope},
			"admission unreadable": {customerVerifier{}, granted, customerAdmission{err: errors.New("unreachable")}, nil, nodeopshttp.ErrIdentityDependencyUnavailable},
			"no access request id": {customerVerifier{}, granted, customerAdmission{admitted: true}, func() (string, error) { return " ", nil }, nodeopshttp.ErrMalformedRequest},
			"payload unreadable":   {customerVerifier{}, granted, customerAdmission{admitted: true}, func() (string, error) { return "", malformed }, nodeopshttp.ErrMalformedRequest},
		} {
			t.Run(capability.String()+"/"+name, func(t *testing.T) {
				authenticator := customerAuthenticator(t, testCase.verifier, testCase.register, testCase.admission)
				id := testCase.id
				if id == nil {
					id = func() (string, error) { return "SYN-ACCESS-REQUEST-PA-0001", nil }
				}
				_, err := authenticate(authenticator, id)
				if !errors.Is(err, testCase.want) {
					t.Fatalf("err = %v, want %v", err, testCase.want)
				}
				for _, other := range []error{
					nodeopshttp.ErrAccessChannelNotConfigured, nodeopshttp.ErrCustomerClientCredentialRejected, nodeopshttp.ErrCustomerClientNotGranted,
					nodeopshttp.ErrOutsideAdmissionScope, nodeopshttp.ErrIdentityDependencyUnavailable, nodeopshttp.ErrMalformedRequest,
				} {
					if other != testCase.want && errors.Is(err, other) {
						t.Fatalf("err = %v 也被 %v 认出", err, other)
					}
				}
			})
		}
	}
	if _, err := noaccess.NewCustomerPreAdviceAuthenticator(nil); err == nil {
		t.Fatal("缺铸造器也建起来了")
	}
}
