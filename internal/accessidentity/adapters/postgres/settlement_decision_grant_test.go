package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: AT-SA-221、AT-SA-222（接入身份半边）——客户费用确认与客户对账单发布两种运营决定的授予登得进操作者册（access_identity
// 0034 放开决定种类那条 CHECK），读回各是各的一种：持确认的不因此能发布。
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
}
