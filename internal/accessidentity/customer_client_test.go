package accessidentity_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

const (
	customerIssuer  = "https://auth.syn.example/customers"
	customerTenant  = "SYN-TENANT-01"
	customerAccount = "SYN-ACCOUNT-01"
)

var customerGrantStartsAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func customerSubjectOf(t *testing.T, sub string) accessidentity.CustomerClientSubject {
	t.Helper()
	subject, err := accessidentity.NewCustomerClientSubject(customerIssuer, sub)
	if err != nil {
		t.Fatalf("NewCustomerClientSubject: %v", err)
	}
	return subject
}

func customerBindingOf(t *testing.T, subject accessidentity.CustomerClientSubject, tenant string, certificateBound bool) accessidentity.CustomerClientBinding {
	t.Helper()
	binding, err := accessidentity.NewCustomerClientBinding(subject, tenant, customerAccount,
		credentialReferenceOf(t, "SYN-CREDENTIAL-REF/customer-01"), certificateBound, "SYN-CUSTOMER-CLIENT-BASIS-01")
	if err != nil {
		t.Fatalf("NewCustomerClientBinding: %v", err)
	}
	return binding
}

func customerGrantOf(
	t *testing.T,
	grantID string,
	subject accessidentity.CustomerClientSubject,
	interval accessidentity.EffectiveInterval,
) accessidentity.CustomerClientGrant {
	t.Helper()
	grant, err := accessidentity.NewCustomerClientGrant(customerTenant, grantID, subject,
		accessidentity.CustomerCapabilityShipmentRequestSubmission, interval, "SYN-CUSTOMER-GRANT-BASIS-01")
	if err != nil {
		t.Fatalf("NewCustomerClientGrant: %v", err)
	}
	return grant
}

func recordedCustomerGrantOf(
	t *testing.T,
	grant accessidentity.CustomerClientGrant,
	revocation *accessidentity.GrantRevocation,
) accessidentity.RecordedCustomerClientGrant {
	t.Helper()
	recorded, err := accessidentity.NewRecordedCustomerClientGrant(grant, revocation)
	if err != nil {
		t.Fatalf("NewRecordedCustomerClientGrant: %v", err)
	}
	return recorded
}

// Covers: ADR-0139 决定二「客户渠道册」——一行缺任何一件都建不成：主体两件、租户、客户账户、凭据引用与登记依据。
func TestCustomerClientBindingRefusesEveryMissingPiece(t *testing.T) {
	if _, err := accessidentity.NewCustomerClientSubject(customerIssuer, "  "); !errors.Is(err, accessidentity.ErrInvalidCustomerClientSubject) {
		t.Fatalf("sub 空白：err = %v", err)
	}
	if _, err := accessidentity.NewCustomerClientSubject("", "SYN-CUSTOMER-CLIENT-01"); !errors.Is(err, accessidentity.ErrInvalidCustomerClientSubject) {
		t.Fatalf("发行方缺席：err = %v", err)
	}
	subject := customerSubjectOf(t, "SYN-CUSTOMER-CLIENT-01")
	reference := credentialReferenceOf(t, "SYN-CREDENTIAL-REF/customer-01")
	for name, build := range map[string]func() error{
		"零值主体": func() error {
			_, err := accessidentity.NewCustomerClientBinding(accessidentity.CustomerClientSubject{}, customerTenant, customerAccount, reference, false, "b")
			return err
		},
		"缺租户": func() error {
			_, err := accessidentity.NewCustomerClientBinding(subject, " ", customerAccount, reference, false, "b")
			return err
		},
		"缺客户账户": func() error {
			_, err := accessidentity.NewCustomerClientBinding(subject, customerTenant, "", reference, false, "b")
			return err
		},
		"缺凭据引用": func() error {
			_, err := accessidentity.NewCustomerClientBinding(subject, customerTenant, customerAccount, accessidentity.CredentialReference{}, false, "b")
			return err
		},
		"缺依据": func() error {
			_, err := accessidentity.NewCustomerClientBinding(subject, customerTenant, customerAccount, reference, false, "  ")
			return err
		},
	} {
		if err := build(); !errors.Is(err, accessidentity.ErrIncompleteCustomerClientRegistration) {
			t.Fatalf("%s：err = %v, want ErrIncompleteCustomerClientRegistration", name, err)
		}
	}
}

// Covers: ADR-0139 决定二「授予按客户面能力显式登记」——能力词表是封闭的：不认的词解析即拒，包外转型递进来的也在构造
// 授予时拦下；能力不在词表里的授予登不进册。
func TestCustomerCapabilityIsAClosedVocabulary(t *testing.T) {
	if capability, err := accessidentity.ParseCustomerCapability("SHIPMENT_REQUEST_SUBMISSION"); err != nil ||
		capability != accessidentity.CustomerCapabilityShipmentRequestSubmission {
		t.Fatalf("提交能力：%q, %v", capability, err)
	}
	for _, unknown := range []string{"shipment_request_submission", "SHIPMENT_REQUEST_WITHDRAWAL", "", "shipments:submit"} {
		if _, err := accessidentity.ParseCustomerCapability(unknown); !errors.Is(err, accessidentity.ErrCustomerCapabilityUnknown) {
			t.Fatalf("%q：err = %v, want ErrCustomerCapabilityUnknown", unknown, err)
		}
	}
	interval := clientIntervalOf(t, customerGrantStartsAt, time.Time{})
	_, err := accessidentity.NewCustomerClientGrant(customerTenant, "SYN-CUSTOMER-GRANT-01", customerSubjectOf(t, "SYN-CUSTOMER-CLIENT-01"),
		accessidentity.CustomerCapability("CLAIM_SUBMISSION"), interval, "b")
	if !errors.Is(err, accessidentity.ErrCustomerCapabilityUnknown) {
		t.Fatalf("转型递进来的能力：err = %v", err)
	}
}

// Covers: ADR-0139 决定二「生效区间与撤销挂在授予上」——授予只在区间内、撤销时刻前生效；现状里混进别的主体或别的租户
// 名下的授予即拒。
func TestCustomerClientStandingHoldsOnlyEffectiveGrants(t *testing.T) {
	subject := customerSubjectOf(t, "SYN-CUSTOMER-CLIENT-01")
	march := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	ending := customerGrantOf(t, "SYN-CUSTOMER-GRANT-01", subject, clientIntervalOf(t, customerGrantStartsAt, march))
	standing, err := accessidentity.NewCustomerClientStanding(customerBindingOf(t, subject, customerTenant, false),
		[]accessidentity.RecordedCustomerClientGrant{recordedCustomerGrantOf(t, ending, nil)})
	if err != nil {
		t.Fatalf("NewCustomerClientStanding: %v", err)
	}
	capability := accessidentity.CustomerCapabilityShipmentRequestSubmission
	if standing.HoldsAt(capability, customerGrantStartsAt.Add(-time.Second)) || !standing.HoldsAt(capability, customerGrantStartsAt) ||
		!standing.HoldsAt(capability, march.Add(-time.Second)) || standing.HoldsAt(capability, march) {
		t.Fatal("授予只该在 [起点, 终点) 内生效")
	}

	open := customerGrantOf(t, "SYN-CUSTOMER-GRANT-02", subject, clientIntervalOf(t, customerGrantStartsAt, time.Time{}))
	revocation, err := accessidentity.NewGrantRevocation(customerTenant, "SYN-CUSTOMER-GRANT-02", march, "SYN-REVOKE-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := accessidentity.NewCustomerClientStanding(customerBindingOf(t, subject, customerTenant, false),
		[]accessidentity.RecordedCustomerClientGrant{recordedCustomerGrantOf(t, open, &revocation)})
	if err != nil {
		t.Fatal(err)
	}
	if !revoked.HoldsAt(capability, march.Add(-time.Second)) || revoked.HoldsAt(capability, march) {
		t.Fatal("撤销时刻起授予就不该生效")
	}
	if _, err := accessidentity.NewRecordedCustomerClientGrant(ending, &revocation); !errors.Is(err, accessidentity.ErrRevocationDoesNotMatchGrant) {
		t.Fatalf("撤别的授予：err = %v", err)
	}

	stranger := customerGrantOf(t, "SYN-CUSTOMER-GRANT-03", customerSubjectOf(t, "SYN-CUSTOMER-CLIENT-02"),
		clientIntervalOf(t, customerGrantStartsAt, time.Time{}))
	if _, err := accessidentity.NewCustomerClientStanding(customerBindingOf(t, subject, customerTenant, false),
		[]accessidentity.RecordedCustomerClientGrant{recordedCustomerGrantOf(t, stranger, nil)}); !errors.Is(err, accessidentity.ErrCustomerClientGrantOutsideBinding) {
		t.Fatalf("混进别的主体的授予：err = %v", err)
	}
	if _, err := accessidentity.NewCustomerClientStanding(customerBindingOf(t, subject, "SYN-TENANT-02", false),
		[]accessidentity.RecordedCustomerClientGrant{recordedCustomerGrantOf(t, open, nil)}); !errors.Is(err, accessidentity.ErrCustomerClientGrantOutsideBinding) {
		t.Fatalf("混进别的租户的授予：err = %v", err)
	}
}
