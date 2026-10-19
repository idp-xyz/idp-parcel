package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: 票 demo-chain-gaps/25 的册面（迁移 access_identity 0015）——`履约段采用运输委托`、`取消运输委托`、`取消订舱`三种运营
// 决定的授予登得进操作者册、读回各是各的，持有其一不读成持有另一种；0013 放开的订舱提交与 0012 放开的容量释放仍登得进去，
// 证约束重建时没有漏抄已有的种类。
func TestTheAdoptionAndCancellationDecisionsAreGrantedAndReadBack(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-DCG25")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for grantID, kind := range map[string]accessidentity.DecisionKind{
		"SYN-DECISION-DCG25-ADOPTION":            accessidentity.DecisionFulfillmentSegmentCommissionAdoption,
		"SYN-DECISION-DCG25-COMMISSION-CANCEL":   accessidentity.DecisionTransportCommissionCancellation,
		"SYN-DECISION-DCG25-BOOKING-SUBMISSION":  accessidentity.DecisionBookingRequestSubmission,
		"SYN-DECISION-DCG25-CAPACITY-RELEASE":    accessidentity.DecisionCapacityRelease,
		"SYN-DECISION-DCG25-IDENTITY-CONFLICT-1": accessidentity.DecisionIdentityConflictDisposition,
	} {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", grantID, subject, kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, string(kind)+" decision grant", register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	}

	standing := register.standing(t, subject)
	at := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	if !standing.HoldsDecisionAt(accessidentity.DecisionFulfillmentSegmentCommissionAdoption, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionTransportCommissionCancellation, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionBookingRequestSubmission, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionCapacityRelease, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionIdentityConflictDisposition, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionBookingRequestCancellation, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionTransportCommissionSubmission, at) {
		t.Fatalf("决定种类没有原样读回：%+v", standing.Grants())
	}

	// 取消订舱单独再授一笔：它与取消委托是两种，前一笔不替它开门。
	grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-DCG25-BOOKING-CANCEL", subject,
		accessidentity.DecisionBookingRequestCancellation, interval, "SYN-GRANT-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, "booking cancellation grant", register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	if !register.standing(t, subject).HoldsDecisionAt(accessidentity.DecisionBookingRequestCancellation, at) {
		t.Fatal("取消订舱的授予没有读回")
	}
}
