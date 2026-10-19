package accessidentity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// 本文件证首方客户族在客户追踪视图查阅口的铸造（票 demo-chain-gaps/96；ADR-0139 决定二、四、八；UC-VE-008）：一口一词的授予、
// 身份只取自册上的绑定、查阅口不判准入范围。合成主体只记 `S`。

func mintTrackingViewQuery(minter *accessidentity.CustomerClientMinter) (accessidentity.CustomerTrackingViewQueryEnvelope, error) {
	return minter.MintTrackingViewQuery(context.Background(), accessidentity.NewIntegrationClientCredential("presented.customer.token", nil))
}

// Covers: AT-VE-150（铸造半边）、ADR-0139 决定二——持查阅授予的客户端铸出的身份就是册上那一行绑定的（租户，客户账户）。
func TestTrackingViewQueryMintsTheBindingOfTheRegister(t *testing.T) {
	minter := newPreAdviceMinter(t, customerVerifierFake{verified: verifiedCustomerToken(t, false)},
		customerRegistryFake{standing: customerStandingHolding(t, accessidentity.CustomerCapabilityTrackingViewQuery), found: true},
		accessidentity.UnconfiguredAdmissionScope{})
	envelope, err := mintTrackingViewQuery(minter)
	if err != nil {
		t.Fatalf("持查阅授予的客户端：%v", err)
	}
	if !envelope.Minted() || envelope.TenantID() != customerTenant || envelope.CustomerAccountID() != customerAccount {
		t.Fatalf("查阅身份 = (%q, %q, minted %v), want 册上绑定的 (%q, %q)",
			envelope.TenantID(), envelope.CustomerAccountID(), envelope.Minted(), customerTenant, customerAccount)
	}
	if (accessidentity.CustomerTrackingViewQueryEnvelope{}).Minted() {
		t.Fatal("零值答已铸——包外写得出的零值不可用作身份")
	}
}

// Covers: AT-VE-172——授予按口登：只持委托提交、送站预报等别的能力，查阅口答未授予；只持查阅授予，委托提交口也答未授予。
func TestTrackingViewQueryNeedsItsOwnGrant(t *testing.T) {
	others := customerRegistryFake{standing: customerStandingHolding(t,
		accessidentity.CustomerCapabilityShipmentRequestSubmission,
		accessidentity.CustomerCapabilityDeliveryPreAdviceSubmission,
		accessidentity.CustomerCapabilityDeliveryPreAdviceWithdrawal,
		accessidentity.CustomerCapabilityDeliveryHandoverConfirmation,
	), found: true}
	minter := newPreAdviceMinter(t, customerVerifierFake{verified: verifiedCustomerToken(t, false)}, others, accessidentity.UnconfiguredAdmissionScope{})
	_, err := mintTrackingViewQuery(minter)
	assertCustomerGrade(t, err, "not granted")

	queryOnly := customerRegistryFake{standing: customerStandingHolding(t, accessidentity.CustomerCapabilityTrackingViewQuery), found: true}
	minter = newPreAdviceMinter(t, customerVerifierFake{verified: verifiedCustomerToken(t, false)}, queryOnly, accessidentity.UnconfiguredAdmissionScope{})
	_, err = mintCustomerSubmission(minter, &requestReader{values: map[string]string{accessidentity.AccessRequestIdentifier: "SYN-ACCESS-REQUEST-0001"}})
	assertCustomerGrade(t, err, "not granted")
}

// Covers: AT-VE-173、ADR-0139 决定八「查阅口不判准入范围」——准入范围答不在范围或读不动，查阅照样铸出，准入那一步一次都没被问。
func TestTrackingViewQueryDoesNotAskTheAdmissionScope(t *testing.T) {
	for name, probe := range map[string]admissionProbe{
		"outside":     {admitted: false},
		"unavailable": {err: errors.New("governance register is down")},
	} {
		t.Run(name, func(t *testing.T) {
			var asked []accessidentity.AdmissionRequirement
			probe.asked = &asked
			minter := newPreAdviceMinter(t, customerVerifierFake{verified: verifiedCustomerToken(t, false)},
				customerRegistryFake{standing: customerStandingHolding(t, accessidentity.CustomerCapabilityTrackingViewQuery), found: true}, probe)
			if _, err := mintTrackingViewQuery(minter); err != nil {
				t.Fatalf("查阅口：%v", err)
			}
			if len(asked) != 0 {
				t.Fatalf("查阅口问了准入范围：%v", asked)
			}
		})
	}
}

// Covers: ADR-0139 决定四——查阅口的各格与其余各口同形：令牌那一侧原样交回且不读册；不在册、区间外同答未授予；册读不动答依赖
// 故障；要求证书绑定而令牌未绑答令牌不过。
func TestTrackingViewQueryAnswersTheCustomerGrades(t *testing.T) {
	expired := func() customerRegistryFake {
		subject := customerSubjectOf(t, "SYN-CUSTOMER-CLIENT-01")
		interval := clientIntervalOf(t, customerMintAt.Add(-48*time.Hour), customerMintAt.Add(-24*time.Hour))
		grant, err := accessidentity.NewCustomerClientGrant(customerTenant, "SYN-CUSTOMER-GRANT-QUERY", subject,
			accessidentity.CustomerCapabilityTrackingViewQuery, interval, "SYN-CUSTOMER-GRANT-BASIS-01")
		if err != nil {
			t.Fatal(err)
		}
		standing, err := accessidentity.NewCustomerClientStanding(customerBindingOf(t, subject, customerTenant, false),
			[]accessidentity.RecordedCustomerClientGrant{recordedCustomerGrantOf(t, grant, nil)})
		if err != nil {
			t.Fatal(err)
		}
		return customerRegistryFake{standing: standing, found: true}
	}()
	certificateBound := func() customerRegistryFake {
		subject := customerSubjectOf(t, "SYN-CUSTOMER-CLIENT-01")
		grant, err := accessidentity.NewCustomerClientGrant(customerTenant, "SYN-CUSTOMER-GRANT-QUERY", subject,
			accessidentity.CustomerCapabilityTrackingViewQuery, clientIntervalOf(t, customerGrantStartsAt, time.Time{}), "SYN-CUSTOMER-GRANT-BASIS-01")
		if err != nil {
			t.Fatal(err)
		}
		standing, err := accessidentity.NewCustomerClientStanding(customerBindingOf(t, subject, customerTenant, true),
			[]accessidentity.RecordedCustomerClientGrant{recordedCustomerGrantOf(t, grant, nil)})
		if err != nil {
			t.Fatal(err)
		}
		return customerRegistryFake{standing: standing, found: true}
	}()
	granted := customerRegistryFake{standing: customerStandingHolding(t, accessidentity.CustomerCapabilityTrackingViewQuery), found: true}
	unbound := customerVerifierFake{verified: verifiedCustomerToken(t, false)}

	for name, testCase := range map[string]struct {
		verifier accessidentity.CustomerClientCredentialVerifier
		registry customerRegistryFake
		want     string
		// tokenSide 为真的几格在令牌那一步就答出，册一次都不该被读。
		tokenSide bool
	}{
		"issuer unset":                {accessidentity.UnconfiguredCustomerClientCredentialVerifier{}, granted, "not configured", true},
		"token rejected":              {customerVerifierFake{err: accessidentity.ErrCredentialRejected}, granted, "credential rejected", true},
		"key set unreachable":         {customerVerifierFake{err: accessidentity.ErrCredentialVerifierUnavailable}, granted, "verifier unavailable", true},
		"not on the register":         {unbound, customerRegistryFake{}, "not granted", false},
		"grant interval ended":        {unbound, expired, "not granted", false},
		"register unreadable":         {unbound, customerRegistryFake{err: errors.New("connection refused")}, "registry unavailable", false},
		"certificate binding missing": {unbound, certificateBound, "credential rejected", false},
	} {
		t.Run(name, func(t *testing.T) {
			asked := 0
			registry := testCase.registry
			registry.asked = &asked
			minter := newPreAdviceMinter(t, testCase.verifier, registry, accessidentity.UnconfiguredAdmissionScope{})
			_, err := mintTrackingViewQuery(minter)
			assertCustomerGrade(t, err, testCase.want)
			if testCase.tokenSide && asked != 0 {
				t.Fatalf("令牌那一步没过却查了 %d 次册", asked)
			}
		})
	}
}

// Covers: 一口一词（票 first-party-api/00「代决」一节「客户面能力词表一口一词」）——查阅口的词进封闭词表；它不在铸造侧判准入，
// 所以登不进准入范围对照。
func TestTrackingViewQueryWordJoinsTheVocabularyButNotTheAdmissionMapping(t *testing.T) {
	if got, err := accessidentity.ParseCustomerCapability("CUSTOMER_TRACKING_VIEW_QUERY"); err != nil || got != accessidentity.CustomerCapabilityTrackingViewQuery {
		t.Fatalf("CUSTOMER_TRACKING_VIEW_QUERY：%q, %v", got, err)
	}
	requirement := accessidentity.AdmissionRequirement{
		Capability: accessidentity.CustomerCommandAdmissionCapability,
		FactKind:   accessidentity.CustomerCapabilityTrackingViewQuery.String(),
	}
	interval := clientIntervalOf(t, customerGrantStartsAt, time.Time{})
	if _, err := accessidentity.NewAdmissionScopeMapping(customerTenant, "SYN-MAP-1", requirement, "SYN-PILOT-SCOPE/customer-commands@v1", interval, "SYN-BASIS"); !errors.Is(err, accessidentity.ErrAdmissionRequirementUnknown) {
		t.Fatalf("查阅口的词登进了准入范围对照：err = %v", err)
	}
}
