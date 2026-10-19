package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: 票 ps-external-mark-relations/12 地盘 `migrations/access_identity/`——「送站交接确认」这一种运营决定的授予登得进操作者册，
// 读回仍是这一种（access_identity 0016 放开决定种类那条 CHECK），不与身份冲突处置混读；此前各份放开的种类照旧登得进。
func TestTheDeliveryHandoverConfirmationDecisionIsGrantedAndReadBack(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-PSXM12")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for index, kind := range []accessidentity.DecisionKind{
		accessidentity.DecisionDeliveryHandoverConfirmation,
		accessidentity.DecisionBookingRequestCancellation,
	} {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-PSXM12-"+string(rune('A'+index)), subject,
			kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, kind.String(), register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	}

	standing := register.standing(t, subject)
	at := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	if !standing.HoldsDecisionAt(accessidentity.DecisionDeliveryHandoverConfirmation, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionBookingRequestCancellation, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionIdentityConflictDisposition, at) {
		t.Fatalf("决定种类没有原样读回：%+v", standing.Grants())
	}
}

// Covers: 同上，客户面那一格——「送站交接确认」这一项客户面能力的授予过得了库上那条能力 CHECK（access_identity 0016），按字面量
// 读回、只答自己那一项；0014 放开的预报两格与委托提交照旧过得去。
func TestTheDeliveryHandoverConfirmationCustomerGrantPassesTheCapabilityCheck(t *testing.T) {
	register := newCustomerRegister(t)
	subject := customerSubjectOf(t, "https://auth.syn.example/customers", "SYN-CUSTOMER-CLIENT-HN-01")
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
		accessidentity.CustomerCapabilityDeliveryHandoverConfirmation,
	}
	for _, capability := range capabilities {
		grant, err := accessidentity.NewCustomerClientGrant("SYN-TENANT-01", "SYN-CUSTOMER-GRANT-"+capability.String(), subject,
			capability, interval, "SYN-CUSTOMER-GRANT-BASIS-HN")
		if err != nil {
			t.Fatalf("%s：%v", capability, err)
		}
		expectCustomerOutcome(t, capability.String(), register.grant(t, grant), accessidentity.CustomerClientRegistrationRecorded)
	}
	standing := register.standing(t, subject)
	for _, capability := range capabilities {
		if !standing.HoldsAt(capability, startsAt) {
			t.Fatalf("%s 没按字面量读回或不生效", capability)
		}
	}
}
