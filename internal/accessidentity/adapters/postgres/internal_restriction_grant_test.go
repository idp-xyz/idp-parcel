package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: UC-CC-011（授予往返，票 launch-wiring/01）——ADR-0334 决定七——「内部合规限制建立」与「内部合规限制决定」两种运营决定的授予登得进操作者册
// （access_identity 0030 放开决定种类那条 CHECK），读回各是它自己，两种互不相通，也不与关务建案串；之前各份放开的种类仍登得进。
func TestTheInternalRestrictionEstablishmentAndDecisionAreGrantedApartFromEachOther(t *testing.T) {
	register := newOperatorRegister(t)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	grantTo := func(label string, kinds ...accessidentity.DecisionKind) accessidentity.OperatorSubject {
		subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-LW01-"+label)
		expectOutcome(t, "bind "+label, register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
		for index, kind := range kinds {
			grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-LW01-"+label+string(rune('A'+index)), subject, kind, interval, "SYN-GRANT-BASIS")
			if err != nil {
				t.Fatal(err)
			}
			expectOutcome(t, kind.String(), register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
		}
		return subject
	}
	maker := grantTo("A", accessidentity.DecisionInternalRestrictionEstablishment, accessidentity.DecisionAlternateJourneyStart)
	decider := grantTo("B", accessidentity.DecisionInternalRestrictionDecision)

	at := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	first := register.standing(t, maker)
	if !first.HoldsDecisionAt(accessidentity.DecisionInternalRestrictionEstablishment, at) ||
		first.HoldsDecisionAt(accessidentity.DecisionInternalRestrictionDecision, at) ||
		first.HoldsDecisionAt(accessidentity.DecisionCustomsCaseEstablishment, at) {
		t.Fatalf("建立授予没有原样读回，或与决定、关务建案串了：%+v", first.Grants())
	}
	second := register.standing(t, decider)
	if !second.HoldsDecisionAt(accessidentity.DecisionInternalRestrictionDecision, at) ||
		second.HoldsDecisionAt(accessidentity.DecisionInternalRestrictionEstablishment, at) {
		t.Fatalf("决定授予没有原样读回，或串到了建立一侧：%+v", second.Grants())
	}
	for _, raw := range []string{"INTERNAL_RESTRICTION_ESTABLISHMENT", "INTERNAL_RESTRICTION_DECISION"} {
		if kind, err := accessidentity.ParseDecisionKind(raw); err != nil || kind.String() != raw {
			t.Fatalf("ParseDecisionKind(%q) = %q, %v", raw, kind, err)
		}
	}
}
