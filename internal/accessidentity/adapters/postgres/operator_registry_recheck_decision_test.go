package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: 票 no-reception-source/03「人工重核命令口归运营决定」的册面——`待识别重核`这一种决定的授予登得进操作者册、读回仍是这一
// 种（库面决定种类封闭集随领域同笔放开），不与别的决定种类混读。
func TestThePendingIdentityRecheckDecisionIsGrantedAndReadBack(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-NRS03")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-NRS03", subject,
		accessidentity.DecisionPendingIdentityRecheck, interval, "SYN-GRANT-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, "recheck decision grant", register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)

	standing := register.standing(t, subject)
	at := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	if !standing.HoldsDecisionAt(accessidentity.DecisionPendingIdentityRecheck, at) || standing.HoldsDecisionAt(accessidentity.DecisionSegmentClosure, at) {
		t.Fatalf("决定种类没有原样读回：%+v", standing.Grants())
	}
}
