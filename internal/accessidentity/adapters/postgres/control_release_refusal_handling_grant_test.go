package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: AT-PS-164；ADR-0293 决定三——「释放拒收处理」这一种运营决定的授予登得进操作者册（access_identity 0025 放开决定种类那条 CHECK），读回是它自己，
// 不与授权处置混读：持授权处置的人不因此持有它，反过来也一样；此前各份放开的种类都登得进。
func TestTheControlReleaseRefusalHandlingDecisionIsGrantedAndReadBackApartFromTheDisposition(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-CODE95")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for index, kind := range []accessidentity.DecisionKind{
		accessidentity.DecisionControlReleaseRefusalHandling,
		accessidentity.DecisionAuthorizedReroute,
	} {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-CODE95-"+string(rune('A'+index)), subject,
			kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, kind.String(), register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	}

	standing := register.standing(t, subject)
	at := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	if !standing.HoldsDecisionAt(accessidentity.DecisionControlReleaseRefusalHandling, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionAuthorizedReroute, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionAuthorizedDisposition, at) {
		t.Fatalf("决定种类没有原样读回，或释放拒收处理与授权处置串了：%+v", standing.Grants())
	}
	if kind, err := accessidentity.ParseDecisionKind("CONTROL_RELEASE_REFUSAL_HANDLING"); err != nil ||
		kind != accessidentity.DecisionControlReleaseRefusalHandling {
		t.Fatalf("ParseDecisionKind = %q, %v", kind, err)
	}
}
