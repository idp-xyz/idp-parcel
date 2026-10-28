package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: UC-CC-009 门禁核对（授予往返，票 launch-wiring/03）——「放行门禁核对」的授予登得进操作者册（access_identity 0032 放开决定种类那条 CHECK），
// 读回是它自己，与关务建案、关案互不相通；之前各份放开的种类（含 0031 的关务案件关闭）仍登得进。
func TestTheReleaseGateVerificationDecisionIsGrantedApartFromCaseClosure(t *testing.T) {
	register := newOperatorRegister(t)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-LW03-A")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	for index, kind := range []accessidentity.DecisionKind{accessidentity.DecisionReleaseGateVerification, accessidentity.DecisionCustomsCaseClosure} {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-LW03-"+string(rune('A'+index)), subject, kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, kind.String(), register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	}
	other := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-LW03-B")
	expectOutcome(t, "bind other", register.bind(t, bindingOf(t, other, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-LW03-X", other, accessidentity.DecisionCustomsCaseClosure, interval, "SYN-GRANT-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, "closure only", register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)

	at := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	if !register.standing(t, subject).HoldsDecisionAt(accessidentity.DecisionReleaseGateVerification, at) {
		t.Fatal("门禁核对授予没有原样读回")
	}
	if register.standing(t, other).HoldsDecisionAt(accessidentity.DecisionReleaseGateVerification, at) {
		t.Fatal("只持关案授予的人读成了持有门禁核对授予")
	}
	if kind, err := accessidentity.ParseDecisionKind("RELEASE_GATE_VERIFICATION"); err != nil || kind != accessidentity.DecisionReleaseGateVerification {
		t.Fatalf("ParseDecisionKind = %q, %v", kind, err)
	}
}
