package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: 票 pn-09-build/08——入库预报货主口的能力词过得了客户能力 CHECK。主体只持这四个里的提交那一个，不持委托提交。
// 其余三个词各登在另一个主体上，用来对上 SQL 里的词表；它们不进这个主体的现状。
func TestInboundPreAdviceCustomerGrantPassesTheCapabilityCheck(t *testing.T) {
	register := newCustomerRegister(t)
	subject := customerSubjectOf(t, "https://auth.syn.example/customers", "SYN-CUSTOMER-CLIENT-IPA-01")
	expectCustomerOutcome(t, "绑定", register.bind(t, customerBindingOf(t, subject, defaultCustomerBinding("SYN-TENANT-01"))),
		accessidentity.CustomerClientRegistrationRecorded)

	startsAt := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	interval, err := accessidentity.NewEffectiveInterval(startsAt, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := accessidentity.NewCustomerClientGrant("SYN-TENANT-01", "SYN-CUSTOMER-GRANT-INBOUND-PRE-ADVICE-SUBMISSION", subject,
		accessidentity.CustomerCapabilityInboundPreAdviceSubmission, interval, "SYN-CUSTOMER-GRANT-BASIS-IPA")
	if err != nil {
		t.Fatal(err)
	}
	expectCustomerOutcome(t, "提交", register.grant(t, grant), accessidentity.CustomerClientRegistrationRecorded)

	standing := register.standing(t, subject)
	if !standing.HoldsAt(accessidentity.CustomerCapabilityInboundPreAdviceSubmission, startsAt) {
		t.Fatal("提交授予没按字面量读回")
	}
	if standing.HoldsAt(accessidentity.CustomerCapabilityShipmentRequestSubmission, startsAt) ||
		standing.HoldsAt(accessidentity.CustomerCapabilityInboundPreAdviceWithdrawal, startsAt) ||
		standing.HoldsAt(accessidentity.CustomerCapabilityWarehouseRead, startsAt) {
		t.Fatal("这个主体不该同时持有委托提交或其余仓配词")
	}
	if grants := standing.Grants(); len(grants) != 1 || grants[0].Grant().Capability() != accessidentity.CustomerCapabilityInboundPreAdviceSubmission {
		t.Fatalf("现状 = %v", grants)
	}

	others := []accessidentity.CustomerCapability{
		accessidentity.CustomerCapabilityInboundPreAdviceArrivalNotice,
		accessidentity.CustomerCapabilityInboundPreAdviceWithdrawal,
		accessidentity.CustomerCapabilityWarehouseRead,
	}
	for index, capability := range others {
		other := customerSubjectOf(t, "https://auth.syn.example/customers", "SYN-CUSTOMER-CLIENT-IPA-0"+string(rune('2'+index)))
		expectCustomerOutcome(t, capability.String()+" 绑定", register.bind(t, customerBindingOf(t, other, defaultCustomerBinding("SYN-TENANT-01"))),
			accessidentity.CustomerClientRegistrationRecorded)
		otherGrant, err := accessidentity.NewCustomerClientGrant("SYN-TENANT-01", "SYN-CUSTOMER-GRANT-"+capability.String(), other,
			capability, interval, "SYN-CUSTOMER-GRANT-BASIS-IPA")
		if err != nil {
			t.Fatal(err)
		}
		expectCustomerOutcome(t, capability.String(), register.grant(t, otherGrant), accessidentity.CustomerClientRegistrationRecorded)
	}
}
