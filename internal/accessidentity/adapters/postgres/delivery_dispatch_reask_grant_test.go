package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: 票 demo-chain-gaps/70（ADR-0230 决定四）——「派送重问」这一种运营决定的授予登得进操作者册（access_identity 0019 放开决定种类
// 那条 CHECK），读回仍是这一种、不与手工建派送任务混读；此前各份放开的种类照旧登得进。
func TestTheDeliveryDispatchReaskDecisionIsGrantedAndReadBack(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-DCG70")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for index, kind := range []accessidentity.DecisionKind{
		accessidentity.DecisionDeliveryDispatchReask,
		accessidentity.DecisionDeliveryHandoverConfirmation,
		accessidentity.DecisionCustomsDeclarationSubmission,
	} {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-DCG70-"+string(rune('A'+index)), subject,
			kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, kind.String(), register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	}

	standing := register.standing(t, subject)
	at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	if !standing.HoldsDecisionAt(accessidentity.DecisionDeliveryDispatchReask, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionDeliveryHandoverConfirmation, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionCustomsDeclarationSubmission, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionDispatchTaskRegistration, at) {
		t.Fatalf("决定种类没有原样读回：%+v", standing.Grants())
	}
}
