package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: 票 ps-external-mark-relations/09 做什么「能力词」——送站预报提交、送站预报撤回两格授予过得了库上那条能力 CHECK
// （access_identity 0014），按字面量读回，各自只答自己那一项；委托提交那一格照旧过得去。
func TestDeliveryPreAdviceGrantsPassTheCapabilityCheckAndRoundTrip(t *testing.T) {
	register := newCustomerRegister(t)
	subject := customerSubjectOf(t, "https://auth.syn.example/customers", "SYN-CUSTOMER-CLIENT-PA-01")
	expectCustomerOutcome(t, "绑定", register.bind(t, customerBindingOf(t, subject, defaultCustomerBinding("SYN-TENANT-01"))),
		accessidentity.CustomerClientRegistrationRecorded)

	startsAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	interval, err := accessidentity.NewEffectiveInterval(startsAt, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	capabilities := []accessidentity.CustomerCapability{
		accessidentity.CustomerCapabilityShipmentRequestSubmission,
		accessidentity.CustomerCapabilityDeliveryPreAdviceSubmission,
		accessidentity.CustomerCapabilityDeliveryPreAdviceWithdrawal,
	}
	for _, capability := range capabilities {
		grant, err := accessidentity.NewCustomerClientGrant("SYN-TENANT-01", "SYN-CUSTOMER-GRANT-"+capability.String(), subject,
			capability, interval, "SYN-CUSTOMER-GRANT-BASIS-PA")
		if err != nil {
			t.Fatalf("%s：%v", capability, err)
		}
		expectCustomerOutcome(t, capability.String(), register.grant(t, grant), accessidentity.CustomerClientRegistrationRecorded)
	}

	standing := register.standing(t, subject)
	read := map[accessidentity.CustomerCapability]bool{}
	for _, recorded := range standing.Grants() {
		read[recorded.Grant().Capability()] = true
	}
	for _, capability := range capabilities {
		if !read[capability] || !standing.HoldsAt(capability, startsAt) {
			t.Fatalf("%s 没按字面量读回或不生效：%v", capability, read)
		}
	}
}
