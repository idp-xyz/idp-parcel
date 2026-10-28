package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: `AT-TF-370`（授予往返）——ADR-0337 决定一——「监管运输处置承接」与「替代或退运旅程启动」两种运营决定的授予登得进操作者册（access_identity 0029
// 放开决定种类那条 CHECK），读回各是它自己，两种互不相通，也不与运输委托提交串；之前各份放开的种类（含 0028 的收寄后服务处置）仍登得进。
func TestTheRegulatoryAcceptanceAndJourneyStartDecisionsAreGrantedApartFromEachOther(t *testing.T) {
	register := newOperatorRegister(t)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	grantTo := func(label string, kinds ...accessidentity.DecisionKind) accessidentity.OperatorSubject {
		subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-CODE43-"+label)
		expectOutcome(t, "bind "+label, register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
		for index, kind := range kinds {
			grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-CODE43-"+label+string(rune('A'+index)), subject, kind, interval, "SYN-GRANT-BASIS")
			if err != nil {
				t.Fatal(err)
			}
			expectOutcome(t, kind.String(), register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
		}
		return subject
	}
	acceptor := grantTo("A", accessidentity.DecisionRegulatoryDispositionAcceptance, accessidentity.DecisionPostIntakeServiceDisposition)
	starter := grantTo("B", accessidentity.DecisionAlternateJourneyStart)

	at := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	first := register.standing(t, acceptor)
	if !first.HoldsDecisionAt(accessidentity.DecisionRegulatoryDispositionAcceptance, at) ||
		first.HoldsDecisionAt(accessidentity.DecisionAlternateJourneyStart, at) ||
		first.HoldsDecisionAt(accessidentity.DecisionTransportCommissionSubmission, at) {
		t.Fatalf("承接授予没有原样读回，或与旅程启动、委托提交串了：%+v", first.Grants())
	}
	second := register.standing(t, starter)
	if !second.HoldsDecisionAt(accessidentity.DecisionAlternateJourneyStart, at) ||
		second.HoldsDecisionAt(accessidentity.DecisionRegulatoryDispositionAcceptance, at) {
		t.Fatalf("旅程启动授予没有原样读回，或串到了承接一侧：%+v", second.Grants())
	}
	for _, raw := range []string{"REGULATORY_DISPOSITION_ACCEPTANCE", "ALTERNATE_JOURNEY_START"} {
		if kind, err := accessidentity.ParseDecisionKind(raw); err != nil || kind.String() != raw {
			t.Fatalf("ParseDecisionKind(%q) = %q, %v", raw, kind, err)
		}
	}
}
