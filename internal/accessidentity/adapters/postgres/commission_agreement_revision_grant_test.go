package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: 票 demo-chain-gaps/88（ADR-0231 决定三）——「委托协议换版」这一种运营决定的授予登得进操作者册（access_identity 0021 放开决定种类
// 那条 CHECK），读回仍是这一种、不与提交委托或取消委托混读；此前各份放开的种类照旧登得进。
func TestTheCommissionAgreementRevisionDecisionIsGrantedAndReadBack(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-DCG88")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for index, kind := range []accessidentity.DecisionKind{
		accessidentity.DecisionTransportCommissionAgreementRevision,
		accessidentity.DecisionRegulatoryCredentialRegistration,
		accessidentity.DecisionDeliveryDispatchReask,
	} {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-DCG88-"+string(rune('A'+index)), subject,
			kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, kind.String(), register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	}

	standing := register.standing(t, subject)
	at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	if !standing.HoldsDecisionAt(accessidentity.DecisionTransportCommissionAgreementRevision, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionRegulatoryCredentialRegistration, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionDeliveryDispatchReask, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionTransportCommissionSubmission, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionTransportCommissionCancellation, at) {
		t.Fatalf("决定种类没有原样读回：%+v", standing.Grants())
	}
}
