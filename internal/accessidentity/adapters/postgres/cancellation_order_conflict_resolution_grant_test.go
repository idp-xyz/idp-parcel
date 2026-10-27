package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: AT-PS-166；ADR-0295 决定二——「取消先后冲突解开」这一种运营决定的授予登得进操作者册（access_identity 0026 放开决定种类那条 CHECK），读回是它自己，
// 不与授权处置、释放拒收处理混读：持任一种的人不因此持有另两种；此前各份放开的种类都登得进（0026 取此前各份的并集）。
func TestTheCancellationOrderConflictResolutionDecisionIsGrantedAndReadBackApartFromTheOthers(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-CODE76")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for index, kind := range []accessidentity.DecisionKind{
		accessidentity.DecisionCancellationOrderConflictResolution,
		accessidentity.DecisionControlReleaseRefusalHandling,
		accessidentity.DecisionAuthorizedReroute,
	} {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-CODE76-"+string(rune('A'+index)), subject,
			kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, kind.String(), register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	}

	standing := register.standing(t, subject)
	at := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	if !standing.HoldsDecisionAt(accessidentity.DecisionCancellationOrderConflictResolution, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionControlReleaseRefusalHandling, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionAuthorizedReroute, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionAuthorizedDisposition, at) {
		t.Fatalf("决定种类没有原样读回，或与另几种串了：%+v", standing.Grants())
	}
	if kind, err := accessidentity.ParseDecisionKind("CANCELLATION_ORDER_CONFLICT_RESOLUTION"); err != nil ||
		kind != accessidentity.DecisionCancellationOrderConflictResolution {
		t.Fatalf("ParseDecisionKind = %q, %v", kind, err)
	}

	onlyOthers := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-CODE76-OTHER")
	expectOutcome(t, "bind other", register.bind(t, bindingOf(t, onlyOthers, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-CODE76-OTHER", onlyOthers,
		accessidentity.DecisionAuthorizedDisposition, interval, "SYN-GRANT-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, "grant other", register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	if register.standing(t, onlyOthers).HoldsDecisionAt(accessidentity.DecisionCancellationOrderConflictResolution, at) {
		t.Fatal("只持授权处置的操作者被读成持有取消先后冲突解开")
	}
}
