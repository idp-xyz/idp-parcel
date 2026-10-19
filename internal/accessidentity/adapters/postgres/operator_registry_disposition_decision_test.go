package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: 票 no-reception-identity/06「授予」的册面——`身份冲突处置`这一种决定的授予登得进操作者册、读回仍是这一种（库面决定种类
// 封闭集随领域同笔放开），不与待识别重核混读。
func TestTheIdentityConflictDispositionDecisionIsGrantedAndReadBack(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-NRI06")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-NRI06", subject,
		accessidentity.DecisionIdentityConflictDisposition, interval, "SYN-GRANT-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, "disposition decision grant", register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)

	standing := register.standing(t, subject)
	at := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	if !standing.HoldsDecisionAt(accessidentity.DecisionIdentityConflictDisposition, at) || standing.HoldsDecisionAt(accessidentity.DecisionPendingIdentityRecheck, at) {
		t.Fatalf("决定种类没有原样读回：%+v", standing.Grants())
	}
}
