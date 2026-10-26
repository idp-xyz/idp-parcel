package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: ADR-0284 决定一——「授权改路」这一种运营决定的授予登得进操作者册（access_identity 0023 放开决定种类那条 CHECK），读回
// 是它自己、不与别的种类混读；此前各份放开的种类照旧登得进。
func TestTheAuthorizedRerouteDecisionIsGrantedAndReadBack(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-CODE29")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for index, kind := range []accessidentity.DecisionKind{
		accessidentity.DecisionAuthorizedReroute,
		accessidentity.DecisionTransportChargeOccurrenceInvalidation,
	} {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-CODE29-"+string(rune('A'+index)), subject,
			kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, kind.String(), register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	}

	standing := register.standing(t, subject)
	at := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	if !standing.HoldsDecisionAt(accessidentity.DecisionAuthorizedReroute, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionTransportChargeOccurrenceInvalidation, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionTransportCommissionSubmission, at) {
		t.Fatalf("决定种类没有原样读回：%+v", standing.Grants())
	}
	if kind, err := accessidentity.ParseDecisionKind("AUTHORIZED_REROUTE"); err != nil || kind != accessidentity.DecisionAuthorizedReroute {
		t.Fatalf("ParseDecisionKind = %q, %v", kind, err)
	}
}
