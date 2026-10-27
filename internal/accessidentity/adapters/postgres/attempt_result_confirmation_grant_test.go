package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: `AT-TF-361`（授予往返）——ADR-0297 决定二——「揽收尝试结果认定」与「派送尝试结果认定」两种运营决定的授予登得进操作者册（access_identity 0027 放开决定种类那条
// CHECK），读回各是它自己，两种互不相通，也不与承运商首次有效收寄判断串；既有各份放开的种类仍登得进。
func TestTheAttemptResultConfirmationDecisionsAreGrantedAndReadBackApartFromEachOther(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-CODE90")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for index, kind := range []accessidentity.DecisionKind{
		accessidentity.DecisionPickupAttemptResultConfirmation,
		accessidentity.DecisionControlReleaseRefusalHandling,
	} {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-CODE90-"+string(rune('A'+index)), subject,
			kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, kind.String(), register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	}
	// 派送一种单独授予给另一位操作者，证两种授予不串。
	other := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-CODE90-B")
	expectOutcome(t, "bind other", register.bind(t, bindingOf(t, other, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	deliveryGrant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-CODE90-C", other,
		accessidentity.DecisionDeliveryAttemptResultConfirmation, interval, "SYN-GRANT-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, "delivery", register.grant(t, deliveryGrant), accessidentity.OperatorRegistrationRecorded)

	at := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	first := register.standing(t, subject)
	if !first.HoldsDecisionAt(accessidentity.DecisionPickupAttemptResultConfirmation, at) ||
		first.HoldsDecisionAt(accessidentity.DecisionDeliveryAttemptResultConfirmation, at) ||
		first.HoldsDecisionAt(accessidentity.DecisionCarrierFirstEffectivePickupJudgment, at) {
		t.Fatalf("揽收尝试结果认定没有原样读回，或与派送一侧、收寄判断串了：%+v", first.Grants())
	}
	second := register.standing(t, other)
	if !second.HoldsDecisionAt(accessidentity.DecisionDeliveryAttemptResultConfirmation, at) ||
		second.HoldsDecisionAt(accessidentity.DecisionPickupAttemptResultConfirmation, at) {
		t.Fatalf("派送尝试结果认定没有原样读回，或串到了揽收一侧：%+v", second.Grants())
	}
	for _, raw := range []string{"PICKUP_ATTEMPT_RESULT_CONFIRMATION", "DELIVERY_ATTEMPT_RESULT_CONFIRMATION"} {
		if kind, err := accessidentity.ParseDecisionKind(raw); err != nil || kind.String() != raw {
			t.Fatalf("ParseDecisionKind(%q) = %q, %v", raw, kind, err)
		}
	}
}
