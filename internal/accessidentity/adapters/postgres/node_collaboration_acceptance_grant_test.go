package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: `AT-NO-133`（授予往返）——ADR-0338 决定一——「节点监管协作承接」这一种运营决定的授予登得进操作者册（access_identity 0031 放开决定
// 种类那条 CHECK），读回就是它自己，不与监管运输处置承接、送站交接确认串；之前各份放开的种类（含 0029 的两种）仍登得进。
func TestTheNodeCollaborationAcceptanceDecisionIsGrantedApartFromTheOthers(t *testing.T) {
	register := newOperatorRegister(t)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-NO04")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	for index, kind := range []accessidentity.DecisionKind{
		accessidentity.DecisionNodeCollaborationAcceptance,
		accessidentity.DecisionRegulatoryDispositionAcceptance,
	} {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-NO04-"+string(rune('A'+index)), subject, kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, kind.String(), register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	}

	at := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	standing := register.standing(t, subject)
	if !standing.HoldsDecisionAt(accessidentity.DecisionNodeCollaborationAcceptance, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionDeliveryHandoverConfirmation, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionAlternateJourneyStart, at) {
		t.Fatalf("节点监管协作承接授予没有原样读回，或与别的决定串了：%+v", standing.Grants())
	}
	if kind, err := accessidentity.ParseDecisionKind("NODE_COLLABORATION_ACCEPTANCE"); err != nil || kind != accessidentity.DecisionNodeCollaborationAcceptance {
		t.Fatalf("ParseDecisionKind = %q, %v", kind, err)
	}
}
