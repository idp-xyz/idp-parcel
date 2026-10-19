package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: 票 demo-chain-gaps/24 的册面（迁移 access_identity 0013）——`订舱提交`这一种运营决定的授予登得进操作者册、读回仍是
// 这一种，不与委托提交混读；先放开的委托提交与身份冲突处置仍登得进去，证约束重建时没有漏抄已有的种类。
func TestTheBookingRequestSubmissionDecisionIsGrantedAndReadBack(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-DCG24")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for grantID, kind := range map[string]accessidentity.DecisionKind{
		"SYN-DECISION-DCG24":            accessidentity.DecisionBookingRequestSubmission,
		"SYN-DECISION-DCG24-COMMISSION": accessidentity.DecisionTransportCommissionSubmission,
		"SYN-DECISION-DCG24-CONFLICT":   accessidentity.DecisionIdentityConflictDisposition,
	} {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", grantID, subject, kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, string(kind)+" decision grant", register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	}

	standing := register.standing(t, subject)
	at := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	if !standing.HoldsDecisionAt(accessidentity.DecisionBookingRequestSubmission, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionTransportCommissionSubmission, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionIdentityConflictDisposition, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionSegmentClosure, at) {
		t.Fatalf("决定种类没有原样读回：%+v", standing.Grants())
	}
}

// Covers: 票 demo-chain-gaps/24 的册面（迁移 access_identity 0013、ADR-0202 决定二）——`承运订舱答复`这一类外部事实的授予
// 登得进集成客户端册、读回仍是这一类，不因此持有承运轨迹；既有的外部资金事实仍登得进去，证约束重建时没有漏抄。
func TestTheCarrierBookingAnswerFactIsGrantedAndReadBack(t *testing.T) {
	register := newClientRegister(t)
	subject := clientSubjectOf(t, syntheticIssuer, "SYN-CLIENT-CARRIER-DCG24")
	spec := defaultClientBinding("SYN-TENANT-01")
	spec.source = "SYN-PARTY-CARRIER-TRUNK-01"
	expectClientOutcome(t, "bind", register.bind(t, clientBindingOf(t, subject, spec)), accessidentity.IntegrationClientRegistrationRecorded)
	startsAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for grantID, fact := range map[string]accessidentity.ExternalFactType{
		"SYN-CLIENT-GRANT-DCG24-ANSWER": accessidentity.FactCarrierBookingAnswer,
		"SYN-CLIENT-GRANT-DCG24-FUNDS":  accessidentity.FactExternalFunds,
	} {
		expectClientOutcome(t, string(fact)+" grant",
			register.grant(t, clientGrantOf(t, "SYN-TENANT-01", grantID, subject, fact, startsAt, time.Time{})),
			accessidentity.IntegrationClientRegistrationRecorded)
	}

	standing := register.standing(t, subject)
	at := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	if standing.Binding().SourceIdentity() != "SYN-PARTY-CARRIER-TRUNK-01" ||
		!standing.HoldsAt(accessidentity.FactCarrierBookingAnswer, at) ||
		!standing.HoldsAt(accessidentity.FactExternalFunds, at) ||
		standing.HoldsAt(accessidentity.FactCarrierTracking, at) {
		t.Fatalf("事实类型或来源身份没有原样读回：%+v", standing.Grants())
	}
}
