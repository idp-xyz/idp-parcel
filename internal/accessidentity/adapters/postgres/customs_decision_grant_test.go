package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: AT-CC-445（接入身份半边）——建案、提交授权确认、申报提交执行三种运营决定的授予登得进操作者册（access_identity 0018
// 放开决定种类那条 CHECK），读回各是各的一种：持提交授权确认的不因此持有申报提交执行，此前各份放开的种类照旧登得进。
func TestTheCustomsDecisionKindsAreGrantedAndReadBackSeparately(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-PSB18")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for index, kind := range []accessidentity.DecisionKind{
		accessidentity.DecisionCustomsCaseEstablishment,
		accessidentity.DecisionCustomsSubmissionAuthorization,
		accessidentity.DecisionDeliveryHandoverConfirmation,
	} {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-PSB18-"+string(rune('A'+index)), subject,
			kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, kind.String(), register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	}

	standing := register.standing(t, subject)
	at := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	if !standing.HoldsDecisionAt(accessidentity.DecisionCustomsCaseEstablishment, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionCustomsSubmissionAuthorization, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionDeliveryHandoverConfirmation, at) {
		t.Fatalf("决定种类没有原样读回：%+v", standing.Grants())
	}
	if standing.HoldsDecisionAt(accessidentity.DecisionCustomsDeclarationSubmission, at) {
		t.Fatal("持提交授权确认的读成了也持申报提交执行：批准与实际提交必须分开授予")
	}

	submitter := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-PSB18-SUBMIT")
	expectOutcome(t, "bind submitter", register.bind(t, bindingOf(t, submitter, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-PSB18-SUBMIT", submitter,
		accessidentity.DecisionCustomsDeclarationSubmission, interval, "SYN-GRANT-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, "submission", register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	if submitted := register.standing(t, submitter); !submitted.HoldsDecisionAt(accessidentity.DecisionCustomsDeclarationSubmission, at) ||
		submitted.HoldsDecisionAt(accessidentity.DecisionCustomsSubmissionAuthorization, at) {
		t.Fatalf("申报提交执行的授予没有原样读回：%+v", submitted.Grants())
	}
}
