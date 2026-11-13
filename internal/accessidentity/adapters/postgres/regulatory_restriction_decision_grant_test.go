package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: 迁移 access_identity 0043 的册面——`监管限制建立`与`监管限制解除`两种运营决定的授予登得进操作者册、读回仍是各自
// 那一种；先放开的内部限制建立与对账单争议处置仍登得进去，证约束重建时没有漏抄已有的种类。
func TestTheRegulatoryRestrictionDecisionsAreGrantedAndReadBack(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-REGULATORY-RESTRICTION")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for grantID, kind := range map[string]accessidentity.DecisionKind{
		"SYN-DECISION-RR-ESTABLISH": accessidentity.DecisionRegulatoryRestrictionEstablishment,
		"SYN-DECISION-RR-RELEASE":   accessidentity.DecisionRegulatoryRestrictionRelease,
		"SYN-DECISION-RR-INTERNAL":  accessidentity.DecisionInternalRestrictionEstablishment,
		"SYN-DECISION-RR-DISPUTE":   accessidentity.DecisionStatementDisputeResolution,
	} {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", grantID, subject, kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, string(kind)+" decision grant", register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	}

	standing := register.standing(t, subject)
	at := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	if !standing.HoldsDecisionAt(accessidentity.DecisionRegulatoryRestrictionEstablishment, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionInternalRestrictionEstablishment, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionRegulatoryRestrictionRelease, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionStatementDisputeResolution, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionInternalRestrictionDecision, at) {
		t.Fatalf("决定种类没有原样读回：%+v", standing.Grants())
	}
}
