package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: AT-SA-221、AT-SA-222、AT-SA-225、AT-SA-226（接入身份半边）——客户费用确认、客户对账单发布、供应商账单接收与审核应付四种运营
// 决定的授予登得进操作者册（access_identity 0034、0035 放开决定种类那条 CHECK），读回各是各的一种：持确认的不因此能发布。
func TestTheSettlementDecisionKindsAreGrantedAndReadBackSeparately(t *testing.T) {
	register := newOperatorRegister(t)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	clerk := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-PSB22-CLERK")
	expectOutcome(t, "bind clerk", register.bind(t, bindingOf(t, clerk, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-PSB22-CONFIRM", clerk,
		accessidentity.DecisionChargeConfirmation, interval, "SYN-GRANT-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, "confirm", register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)

	standing := register.standing(t, clerk)
	if !standing.HoldsDecisionAt(accessidentity.DecisionChargeConfirmation, at) ||
		standing.HoldsDecisionAt(accessidentity.DecisionStatementPublication, at) {
		t.Fatalf("确认的授予没有原样读回，或读成了也能发布：%+v", standing.Grants())
	}

	publisher := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-PSB22-PUBLISHER")
	expectOutcome(t, "bind publisher", register.bind(t, bindingOf(t, publisher, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	publication, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-PSB22-PUBLISH", publisher,
		accessidentity.DecisionStatementPublication, interval, "SYN-GRANT-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, "publish", register.grant(t, publication), accessidentity.OperatorRegistrationRecorded)
	if registered := register.standing(t, publisher); !registered.HoldsDecisionAt(accessidentity.DecisionStatementPublication, at) ||
		registered.HoldsDecisionAt(accessidentity.DecisionChargeConfirmation, at) {
		t.Fatalf("发布的授予没有原样读回：%+v", registered.Grants())
	}

	for index, kind := range []accessidentity.DecisionKind{accessidentity.DecisionSupplierBillReception, accessidentity.DecisionSupplierBillAudit} {
		holder := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-PSB22-BILL-"+string(rune('A'+index)))
		expectOutcome(t, "bind bill", register.bind(t, bindingOf(t, holder, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-PSB22-BILL-"+string(rune('A'+index)), holder,
			kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, kind.String(), register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
		if !register.standing(t, holder).HoldsDecisionAt(kind, at) {
			t.Fatalf("%s 没有原样读回", kind)
		}
	}

	// Covers: AT-SA-228、AT-SA-229（接入身份半边）——登记争议与处理争议两种授予登得进（access_identity 0037），各是各的一种：
	// 能代客户登记的不因此能处理（ADR-0354 决定一、二）。
	for index, kind := range []accessidentity.DecisionKind{
		accessidentity.DecisionStatementDisputeRegistration, accessidentity.DecisionStatementDisputeResolution,
	} {
		holder := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-DISPUTE-"+string(rune('A'+index)))
		expectOutcome(t, "bind dispute", register.bind(t, bindingOf(t, holder, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-DISPUTE-"+string(rune('A'+index)), holder,
			kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, kind.String(), register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
		standing := register.standing(t, holder)
		other := accessidentity.DecisionStatementDisputeResolution
		if kind == other {
			other = accessidentity.DecisionStatementDisputeRegistration
		}
		if !standing.HoldsDecisionAt(kind, at) || standing.HoldsDecisionAt(other, at) {
			t.Fatalf("%s 没有原样读回，或读成了也持另一种：%+v", kind, standing.Grants())
		}
	}
}
