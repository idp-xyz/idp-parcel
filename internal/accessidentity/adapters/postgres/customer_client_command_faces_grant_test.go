package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: AT-PS-123（票 first-party-api/06）——首方客户命令五口的五个能力词授予都过得了库上那条能力 CHECK（access_identity 0024），按字面量
// 读回、各自只答自己那一项；不在词表里的串被库挡下（词表在 Go 一处、CHECK 一处，两处同笔改）。
func TestCustomerCommandFaceGrantsPassTheCapabilityCheckAndRoundTrip(t *testing.T) {
	register := newCustomerRegister(t)
	subject := customerSubjectOf(t, "https://auth.syn.example/customers", "SYN-CUSTOMER-CLIENT-CF-01")
	expectCustomerOutcome(t, "绑定", register.bind(t, customerBindingOf(t, subject, defaultCustomerBinding("SYN-TENANT-01"))),
		accessidentity.CustomerClientRegistrationRecorded)

	startsAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	interval, err := accessidentity.NewEffectiveInterval(startsAt, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	capabilities := []accessidentity.CustomerCapability{
		accessidentity.CustomerCapabilityShipmentRequestWithdrawal,
		accessidentity.CustomerCapabilityParcelCancellation,
		accessidentity.CustomerCapabilitySourceDataAmendment,
		accessidentity.CustomerCapabilityShipmentRequestSupplement,
		accessidentity.CustomerCapabilityClaimSubmission,
	}
	for _, capability := range capabilities {
		grant, err := accessidentity.NewCustomerClientGrant("SYN-TENANT-01", "SYN-CUSTOMER-GRANT-"+capability.String(), subject,
			capability, interval, "SYN-CUSTOMER-GRANT-BASIS-CF")
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
