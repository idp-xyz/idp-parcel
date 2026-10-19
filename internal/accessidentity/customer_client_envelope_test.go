package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

var customerMintAt = time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)

type customerVerifierFake struct {
	verified accessidentity.VerifiedCustomerClientToken
	err      error
}

func (fake customerVerifierFake) VerifyCustomerClientCredential(
	context.Context,
	accessidentity.IntegrationClientCredential,
) (accessidentity.VerifiedCustomerClientToken, error) {
	return fake.verified, fake.err
}

// customerRegistryFake 记下被问过几次：令牌不过时册一次都不该被读（ADR-0139 决定二「令牌那一步不读任何登记册」）。
type customerRegistryFake struct {
	standing accessidentity.CustomerClientStanding
	found    bool
	err      error
	asked    *int
}

func (fake customerRegistryFake) FindCustomerClient(
	context.Context,
	accessidentity.CustomerClientSubject,
) (accessidentity.CustomerClientStanding, bool, error) {
	if fake.asked != nil {
		*fake.asked++
	}
	return fake.standing, fake.found, fake.err
}

func verifiedCustomerToken(t *testing.T, certificateBound bool) accessidentity.VerifiedCustomerClientToken {
	t.Helper()
	return accessidentity.NewVerifiedCustomerClientToken(customerSubjectOf(t, "SYN-CUSTOMER-CLIENT-01"), certificateBound)
}

// customerStandingWith 登一个绑在（customerTenant, customerAccount）上的客户端；granted 为真时授提交能力，从
// customerMintAt 前一天起、到后一天止。
func customerStandingWith(t *testing.T, certificateBound, granted bool) accessidentity.CustomerClientStanding {
	t.Helper()
	subject := customerSubjectOf(t, "SYN-CUSTOMER-CLIENT-01")
	var grants []accessidentity.RecordedCustomerClientGrant
	if granted {
		interval := clientIntervalOf(t, customerMintAt.Add(-24*time.Hour), customerMintAt.Add(24*time.Hour))
		grants = append(grants, recordedCustomerGrantOf(t, customerGrantOf(t, "SYN-CUSTOMER-GRANT-01", subject, interval), nil))
	}
	standing, err := accessidentity.NewCustomerClientStanding(customerBindingOf(t, subject, customerTenant, certificateBound), grants)
	if err != nil {
		t.Fatal(err)
	}
	return standing
}

func newCustomerMinter(
	t *testing.T,
	verifier accessidentity.CustomerClientCredentialVerifier,
	registry accessidentity.CustomerClientRegistry,
) *accessidentity.CustomerClientMinter {
	t.Helper()
	minter, err := accessidentity.NewCustomerClientMinter(verifier, registry, accessidentity.UnconfiguredAdmissionScope{},
		func() time.Time { return customerMintAt })
	if err != nil {
		t.Fatalf("NewCustomerClientMinter: %v", err)
	}
	return minter
}

// requestReader 交出带给定接入请求标识的渠道请求，并记下自己被调过没有。
type requestReader struct {
	values map[string]string
	err    error
	read   bool
}

func (reader *requestReader) channelRequest() (accessidentity.ChannelRequest, error) {
	reader.read = true
	if reader.err != nil {
		return accessidentity.ChannelRequest{}, reader.err
	}
	return accessidentity.NewChannelRequest(reader.values), nil
}

func mintCustomerSubmission(minter *accessidentity.CustomerClientMinter, reader *requestReader) (accessidentity.SubmissionEnvelope, error) {
	return minter.MintSubmission(context.Background(),
		accessidentity.NewIntegrationClientCredential("presented.customer.token", nil), reader.channelRequest)
}

// Covers: 票 first-party-api/00 完成判据「正例铸出首方信封」与 ADR-0139 决定二——租户与客户账户只取自册上的绑定，来源由
// 客户端主体推出，来源请求键是载荷里的接入请求标识（去两端空白）；渠道请求里自报的租户与账户一个字也进不了信封。
func TestGrantedCustomerClientMintsASubmissionEnvelopeFromItsBinding(t *testing.T) {
	minter := newCustomerMinter(t, customerVerifierFake{verified: verifiedCustomerToken(t, false)},
		customerRegistryFake{standing: customerStandingWith(t, false, true), found: true})
	reader := &requestReader{values: map[string]string{
		accessidentity.AccessRequestIdentifier: "  SYN-ACCESS-REQUEST-0001\t",
		"tenantId":                             "TENANT-PROD-9",
		"customerAccountId":                    "ACCOUNT-PROD-9",
	}}

	submission, err := mintCustomerSubmission(minter, reader)
	if err != nil {
		t.Fatalf("granted customer client: %v", err)
	}
	envelope := submission.Envelope()
	if envelope.TenantID() != customerTenant || envelope.CustomerAccountID() != customerAccount {
		t.Fatalf("信封身份 = (%q, %q), want 册上绑定的 (%q, %q)", envelope.TenantID(), envelope.CustomerAccountID(), customerTenant, customerAccount)
	}
	if want := "SYN-CUSTOMER-CLIENT-01@" + customerIssuer; envelope.Source() != want {
		t.Fatalf("来源 = %q, want %q", envelope.Source(), want)
	}
	if envelope.RequestKey() != "SYN-ACCESS-REQUEST-0001" {
		t.Fatalf("来源请求键 = %q, want 去两端空白的接入请求标识", envelope.RequestKey())
	}
}

// customerGrades 是首方族铸造的各格（ADR-0139 决定四）：每一格只许被自己那个哨兵认出。
var customerGrades = map[string]error{
	"not configured":        accessidentity.ErrAccessChannelNotConfigured,
	"credential rejected":   accessidentity.ErrCredentialRejected,
	"not granted":           accessidentity.ErrCustomerClientNotGranted,
	"verifier unavailable":  accessidentity.ErrCredentialVerifierUnavailable,
	"registry unavailable":  accessidentity.ErrCustomerClientRegistryUnavailable,
	"outside admission":     accessidentity.ErrOutsideAdmissionScope,
	"admission unavailable": accessidentity.ErrAdmissionScopeUnavailable,
	"request key":           accessidentity.ErrRequestKeyNotDerived,
}

func assertCustomerGrade(t *testing.T, err error, want string) {
	t.Helper()
	for name, sentinel := range customerGrades {
		if got := errors.Is(err, sentinel); got != (name == want) {
			t.Fatalf("err = %v: errors.Is(%s) = %v, want only %q", err, name, got, want)
		}
	}
}

// Covers: 票 first-party-api/00 完成判据「令牌无效 / 证书绑定不符 401；不在册或无生效授予 403；依赖读不动 503」与
// ADR-0139 决定四——令牌那一侧的三格原样交回；不在册、无授予、区间外、已撤销同答未授予；册读不动答依赖故障；要求证书
// 绑定而令牌未绑答令牌不过，绑了即放行。
func TestCustomerClientAnswerGradesDoNotStandInForEachOther(t *testing.T) {
	unbound := customerVerifierFake{verified: verifiedCustomerToken(t, false)}
	bound := customerVerifierFake{verified: verifiedCustomerToken(t, true)}
	expired := func() customerRegistryFake {
		subject := customerSubjectOf(t, "SYN-CUSTOMER-CLIENT-01")
		interval := clientIntervalOf(t, customerMintAt.Add(-48*time.Hour), customerMintAt.Add(-24*time.Hour))
		standing, err := accessidentity.NewCustomerClientStanding(customerBindingOf(t, subject, customerTenant, false),
			[]accessidentity.RecordedCustomerClientGrant{recordedCustomerGrantOf(t, customerGrantOf(t, "SYN-CUSTOMER-GRANT-01", subject, interval), nil)})
		if err != nil {
			t.Fatal(err)
		}
		return customerRegistryFake{standing: standing, found: true}
	}()
	revoked := func() customerRegistryFake {
		subject := customerSubjectOf(t, "SYN-CUSTOMER-CLIENT-01")
		interval := clientIntervalOf(t, customerMintAt.Add(-24*time.Hour), time.Time{})
		revocation, err := accessidentity.NewGrantRevocation(customerTenant, "SYN-CUSTOMER-GRANT-01", customerMintAt.Add(-time.Hour), "SYN-REVOKE-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		standing, err := accessidentity.NewCustomerClientStanding(customerBindingOf(t, subject, customerTenant, false),
			[]accessidentity.RecordedCustomerClientGrant{recordedCustomerGrantOf(t, customerGrantOf(t, "SYN-CUSTOMER-GRANT-01", subject, interval), &revocation)})
		if err != nil {
			t.Fatal(err)
		}
		return customerRegistryFake{standing: standing, found: true}
	}()
	granted := customerRegistryFake{standing: customerStandingWith(t, false, true), found: true}

	for name, testCase := range map[string]struct {
		verifier accessidentity.CustomerClientCredentialVerifier
		registry customerRegistryFake
		want     string
	}{
		"issuer unset":                  {accessidentity.UnconfiguredCustomerClientCredentialVerifier{}, granted, "not configured"},
		"token rejected":                {customerVerifierFake{err: accessidentity.ErrCredentialRejected}, granted, "credential rejected"},
		"key set unreachable":           {customerVerifierFake{err: accessidentity.ErrCredentialVerifierUnavailable}, granted, "verifier unavailable"},
		"not on the register":           {unbound, customerRegistryFake{}, "not granted"},
		"no grant":                      {unbound, customerRegistryFake{standing: customerStandingWith(t, false, false), found: true}, "not granted"},
		"grant interval ended":          {unbound, expired, "not granted"},
		"grant revoked":                 {unbound, revoked, "not granted"},
		"register unreadable":           {unbound, customerRegistryFake{err: errors.New("connection refused")}, "registry unavailable"},
		"certificate-bound token wants": {unbound, customerRegistryFake{standing: customerStandingWith(t, true, true), found: true}, "credential rejected"},
	} {
		t.Run(name, func(t *testing.T) {
			minter := newCustomerMinter(t, testCase.verifier, testCase.registry)
			reader := &requestReader{values: map[string]string{accessidentity.AccessRequestIdentifier: "SYN-ACCESS-REQUEST-0001"}}
			_, err := mintCustomerSubmission(minter, reader)
			assertCustomerGrade(t, err, testCase.want)
			if reader.read {
				t.Fatal("认证没过却读了请求——未认证的调用方会看到载荷校验的结果")
			}
		})
	}

	minter := newCustomerMinter(t, bound, customerRegistryFake{standing: customerStandingWith(t, true, true), found: true})
	if _, err := mintCustomerSubmission(minter, &requestReader{values: map[string]string{accessidentity.AccessRequestIdentifier: "SYN-ACCESS-REQUEST-0001"}}); err != nil {
		t.Fatalf("要求证书绑定、令牌也绑了：%v", err)
	}
}

// Covers: ADR-0139 决定二「先由 ClientVerifier 校验……这一步不读任何登记册」——令牌不过时册一次都没被读：在册与不在册在
// 令牌不过的调用方面前答同一格，册的成员不可被枚举。
func TestCustomerClientTokenIsVerifiedBeforeTheRegisterIsRead(t *testing.T) {
	asked := 0
	minter := newCustomerMinter(t, customerVerifierFake{err: accessidentity.ErrCredentialRejected},
		customerRegistryFake{standing: customerStandingWith(t, false, true), found: true, asked: &asked})
	_, err := mintCustomerSubmission(minter, &requestReader{})
	assertCustomerGrade(t, err, "credential rejected")
	if asked != 0 {
		t.Fatalf("令牌不过却查了 %d 次册", asked)
	}
}

// Covers: ADR-0139 决定五「接入请求标识……由首方族的 RequestKeyDerivation 推导为来源请求键」——载荷没给或给了空白，答推导
// 不出（恢复动作在客户系统补请求），不折进令牌或授予那几格；读请求自己的错原样交回。
func TestCustomerSubmissionWithoutAnAccessRequestIdentifierIsNotSealed(t *testing.T) {
	minter := newCustomerMinter(t, customerVerifierFake{verified: verifiedCustomerToken(t, false)},
		customerRegistryFake{standing: customerStandingWith(t, false, true), found: true})
	for name, values := range map[string]map[string]string{
		"absent": {},
		"blank":  {accessidentity.AccessRequestIdentifier: " \t "},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := mintCustomerSubmission(minter, &requestReader{values: values})
			assertCustomerGrade(t, err, "request key")
		})
	}

	malformed := errors.New("payload is not JSON")
	if _, err := mintCustomerSubmission(minter, &requestReader{err: malformed}); !errors.Is(err, malformed) {
		t.Fatalf("读请求的错 = %v, want 原样交回", err)
	}
	if _, err := minter.MintSubmission(context.Background(), accessidentity.NewIntegrationClientCredential("t", nil), nil); !errors.Is(err, accessidentity.ErrNilDependency) {
		t.Fatalf("不给读请求的口：err = %v", err)
	}
}

func TestCustomerClientMinterRefusesMissingDependencies(t *testing.T) {
	verifier := customerVerifierFake{}
	registry := customerRegistryFake{}
	now := func() time.Time { return customerMintAt }
	for name, build := range map[string]func() (*accessidentity.CustomerClientMinter, error){
		"verifier": func() (*accessidentity.CustomerClientMinter, error) {
			return accessidentity.NewCustomerClientMinter(nil, registry, accessidentity.UnconfiguredAdmissionScope{}, now)
		},
		"registry": func() (*accessidentity.CustomerClientMinter, error) {
			return accessidentity.NewCustomerClientMinter(verifier, nil, accessidentity.UnconfiguredAdmissionScope{}, now)
		},
		"admission": func() (*accessidentity.CustomerClientMinter, error) {
			return accessidentity.NewCustomerClientMinter(verifier, registry, nil, now)
		},
		"clock": func() (*accessidentity.CustomerClientMinter, error) {
			return accessidentity.NewCustomerClientMinter(verifier, registry, accessidentity.UnconfiguredAdmissionScope{}, nil)
		},
	} {
		if _, err := build(); !errors.Is(err, accessidentity.ErrNilDependency) {
			t.Fatalf("缺 %s：err = %v", name, err)
		}
	}
}
