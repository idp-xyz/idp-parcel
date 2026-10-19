package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: 票 demo-chain-gaps/23「运营决定种类 TRANSPORT_COMMISSION_SUBMISSION」的册面——`运输委托提交`这一种决定的授予登得进
// 操作者册、读回仍是这一种（库面决定种类封闭集随领域同笔放开），不与别的决定种类混读；先放开的待识别重核仍登得进去，证约束
// 重建时没有漏抄已有的种类。
func TestTheTransportCommissionSubmissionDecisionIsGrantedAndReadBack(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-DCG23")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for grantID, kind := range map[string]accessidentity.DecisionKind{
		"SYN-DECISION-DCG23":         accessidentity.DecisionTransportCommissionSubmission,
		"SYN-DECISION-DCG23-RECHECK": accessidentity.DecisionPendingIdentityRecheck,
	} {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", grantID, subject, kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, string(kind)+" decision grant", register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	}

	standing := register.standing(t, subject)
	at := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	if !standing.HoldsDecisionAt(accessidentity.DecisionTransportCommissionSubmission, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionPendingIdentityRecheck, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionSegmentClosure, at) {
		t.Fatalf("决定种类没有原样读回：%+v", standing.Grants())
	}
}
