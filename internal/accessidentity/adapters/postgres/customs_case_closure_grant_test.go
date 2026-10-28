package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: UC-CC-010 关闭入口（授予往返，票 launch-wiring/02）——「关务案件关闭」的授予登得进操作者册（access_identity 0031 放开决定种类那条 CHECK），
// 读回是它自己，与关务建案互不相通；之前各份放开的种类（含 0030 的内部合规限制两种）仍登得进。
func TestTheCustomsCaseClosureDecisionIsGrantedApartFromCaseEstablishment(t *testing.T) {
	register := newOperatorRegister(t)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-LW02-A")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	for index, kind := range []accessidentity.DecisionKind{accessidentity.DecisionCustomsCaseClosure, accessidentity.DecisionInternalRestrictionDecision} {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-LW02-"+string(rune('A'+index)), subject, kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, kind.String(), register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	}
	at := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	standing := register.standing(t, subject)
	if !standing.HoldsDecisionAt(accessidentity.DecisionCustomsCaseClosure, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionCustomsCaseEstablishment, at) {
		t.Fatalf("关案授予没有原样读回，或与建案串了：%+v", standing.Grants())
	}
	if kind, err := accessidentity.ParseDecisionKind("CUSTOMS_CASE_CLOSURE"); err != nil || kind != accessidentity.DecisionCustomsCaseClosure {
		t.Fatalf("ParseDecisionKind = %q, %v", kind, err)
	}
}
