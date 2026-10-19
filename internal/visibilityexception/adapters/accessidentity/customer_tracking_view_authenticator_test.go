package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	identity "go.idp.xyz/idp-parcel/internal/accessidentity"
	veaccess "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/accessidentity"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
)

// 本文件证客户追踪视图查阅口的首方认证适配（票 demo-chain-gaps/96；ADR-0139 决定四）：经真的首方铸造器铸查阅身份，把（租户，客户
// 账户）译成本上下文的类型；共享接入身份能力的各格逐格译成 visibilityhttp 的格。核验方与客户渠道册是替身，合成主体只记 `S`。

const (
	customerIssuer  = "https://id.syn.example/customers"
	customerTenant  = "SYN-TENANT-01"
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

// customerRegister 登一个绑在（customerTenant, customerAccount）上的客户端，名下每一项能力各授一笔、不设终点。
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
	binding, err := identity.NewCustomerClientBinding(subject, customerTenant, customerAccount, reference, false, "SYN-BASIS-BINDING")
	if err != nil {
		return identity.CustomerClientStanding{}, false, err
	}
	interval, err := identity.NewEffectiveInterval(customerNow.Add(-24*time.Hour), time.Time{})
	if err != nil {
		return identity.CustomerClientStanding{}, false, err
	}
	var grants []identity.RecordedCustomerClientGrant
	for _, capability := range register.capabilities {
		grant, err := identity.NewCustomerClientGrant(customerTenant, "SYN-CUSTOMER-GRANT-"+capability.String(), subject, capability, interval, "SYN-BASIS-GRANT")
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

// refusingAdmission 对任何问询都答不在准入范围：查阅口若判了准入，就会在这里被拦下。
type refusingAdmission struct{}

func (refusingAdmission) Admits(context.Context, string, identity.AdmissionRequirement, time.Time) (bool, error) {
	return false, nil
}

func trackingViewAuthenticator(t *testing.T, verifier identity.CustomerClientCredentialVerifier, register customerRegister) *veaccess.CustomerTrackingViewAuthenticator {
	t.Helper()
	minter, err := identity.NewCustomerClientMinter(verifier, register, refusingAdmission{}, func() time.Time { return customerNow })
	if err != nil {
		t.Fatal(err)
	}
	built, err := veaccess.NewCustomerTrackingViewAuthenticator(minter)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

// Covers: AT-VE-150（认证半边）、AT-VE-173——持查阅授予的客户端认证到册上绑定的（租户，客户账户）；准入范围答不在范围，查阅
// 照样过。
func TestTrackingViewQueryAuthenticatesToTheBoundCustomerAccount(t *testing.T) {
	built := trackingViewAuthenticator(t, customerVerifier{}, customerRegister{capabilities: []identity.CustomerCapability{identity.CustomerCapabilityTrackingViewQuery}})
	viewer, err := built.AuthenticateTrackingViewQuery(context.Background(), visibilityhttp.PresentedCustomerCredential{Token: "presented.customer.token"})
	if err != nil {
		t.Fatalf("持查阅授予的客户端：%v", err)
	}
	if viewer.Tenant.String() != customerTenant || viewer.Customer.String() != customerAccount {
		t.Fatalf("认证结果 = (%q, %q), want (%q, %q)", viewer.Tenant.String(), viewer.Customer.String(), customerTenant, customerAccount)
	}
}

// Covers: ADR-0139 决定四——接入身份能力的格逐格译成 visibilityhttp 的格，每一格只许被自己那个哨兵认出。只持委托提交授予的客户端
// 在查阅口答未授予（AT-VE-172）。
func TestTrackingViewQueryGradesTranslateIntoVisibilityGrades(t *testing.T) {
	query := customerRegister{capabilities: []identity.CustomerCapability{identity.CustomerCapabilityTrackingViewQuery}}
	cases := map[string]struct {
		verifier identity.CustomerClientCredentialVerifier
		register customerRegister
		token    string
		want     error
	}{
		"issuer parameters unset": {identity.UnconfiguredCustomerClientCredentialVerifier{}, query, "t", visibilityhttp.ErrAccessChannelNotConfigured},
		"token missing":           {customerVerifier{}, query, "", visibilityhttp.ErrCustomerClientCredentialRejected},
		"only a submission grant": {customerVerifier{}, customerRegister{capabilities: []identity.CustomerCapability{identity.CustomerCapabilityShipmentRequestSubmission}}, "t", visibilityhttp.ErrCustomerClientNotGranted},
		"issuer key set down":     {customerVerifier{err: identity.ErrCredentialVerifierUnavailable}, query, "t", visibilityhttp.ErrIdentityDependencyUnavailable},
		"customer register down":  {customerVerifier{}, customerRegister{err: errors.New("down")}, "t", visibilityhttp.ErrIdentityDependencyUnavailable},
	}
	grades := []error{
		visibilityhttp.ErrAccessChannelNotConfigured,
		visibilityhttp.ErrCustomerClientCredentialRejected,
		visibilityhttp.ErrCustomerClientNotGranted,
		visibilityhttp.ErrIdentityDependencyUnavailable,
		visibilityhttp.ErrOperatorCredentialRejected,
		visibilityhttp.ErrOperatorNotGranted,
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := trackingViewAuthenticator(t, testCase.verifier, testCase.register).
				AuthenticateTrackingViewQuery(context.Background(), visibilityhttp.PresentedCustomerCredential{Token: testCase.token})
			for _, grade := range grades {
				if errors.Is(err, grade) != (grade == testCase.want) {
					t.Fatalf("err = %v: errors.Is(%v) = %v", err, grade, errors.Is(err, grade))
				}
			}
		})
	}
}

// Covers: 依赖缺席即拒——不给铸造器的认证方构造不出来。
func TestTrackingViewAuthenticatorNeedsAMinter(t *testing.T) {
	if _, err := veaccess.NewCustomerTrackingViewAuthenticator(nil); err == nil {
		t.Fatal("没有铸造器的认证方构造出来了")
	}
}
