package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: ADR-0384 决定二、AT-WF-067——待认领到货认领与处置各自过得了决定种类 CHECK，且互相独立：
// 持认领授予的不因此持有处置授予。
func TestUnclaimedArrivalDecisionKindsPassCheckIndependently(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-UNCLAIMED-CLAIM")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	claimGrant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-UNCLAIMED-CLAIM", subject,
		accessidentity.DecisionUnclaimedArrivalClaim, interval, "SYN-GRANT-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, "claim grant", register.grant(t, claimGrant), accessidentity.OperatorRegistrationRecorded)
	at := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	standing := register.standing(t, subject)
	if !standing.HoldsDecisionAt(accessidentity.DecisionUnclaimedArrivalClaim, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionUnclaimedArrivalDisposition, at) {
		t.Fatalf("只授认领不应同时持有处置：grants = %+v", standing.Grants())
	}

	disposeSubject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-UNCLAIMED-DISPOSE")
	expectOutcome(t, "bind dispose", register.bind(t, bindingOf(t, disposeSubject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	disposeGrant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-UNCLAIMED-DISPOSE", disposeSubject,
		accessidentity.DecisionUnclaimedArrivalDisposition, interval, "SYN-GRANT-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, "dispose grant", register.grant(t, disposeGrant), accessidentity.OperatorRegistrationRecorded)
	disposeStanding := register.standing(t, disposeSubject)
	if !disposeStanding.HoldsDecisionAt(accessidentity.DecisionUnclaimedArrivalDisposition, at) ||
		disposeStanding.HoldsDecisionAt(accessidentity.DecisionUnclaimedArrivalClaim, at) {
		t.Fatalf("只授处置不应同时持有认领：grants = %+v", disposeStanding.Grants())
	}
}
