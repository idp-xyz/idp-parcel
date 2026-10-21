package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: 票 demo-chain-gaps/57（ADR-0232 决定一、四）——「解除履约段采用」与「运输收费发生项失效」两种运营决定的授予登得进操作者册
// （access_identity 0022 放开决定种类那条 CHECK），读回各是各的、不与采用和登记两种混读；此前各份放开的种类照旧登得进。
func TestTheAdoptionWithdrawalAndOccurrenceInvalidationDecisionsAreGrantedAndReadBack(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-DCG57")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for index, kind := range []accessidentity.DecisionKind{
		accessidentity.DecisionFulfillmentSegmentCommissionAdoptionWithdrawal,
		accessidentity.DecisionTransportChargeOccurrenceInvalidation,
		accessidentity.DecisionDeliveryDispatchReask,
	} {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-DCG57-"+string(rune('A'+index)), subject,
			kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, kind.String(), register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	}

	standing := register.standing(t, subject)
	at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	if !standing.HoldsDecisionAt(accessidentity.DecisionFulfillmentSegmentCommissionAdoptionWithdrawal, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionTransportChargeOccurrenceInvalidation, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionDeliveryDispatchReask, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionFulfillmentSegmentCommissionAdoption, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionTransportChargeOccurrenceRegistration, at) {
		t.Fatalf("决定种类没有原样读回：%+v", standing.Grants())
	}
}
