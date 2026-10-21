package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: AT-SA-201、AT-CC-447（接入身份半边）——外部资金事实采用与监管凭证登记两种运营决定的授予登得进操作者册（access_identity
// 0020 放开决定种类那条 CHECK），读回各是各的一种：持资金事实采用的不因此持凭证登记，反过来也一样；此前各份放开的种类照旧登得进。
func TestTheManualExternalFactDecisionKindsAreGrantedAndReadBackSeparately(t *testing.T) {
	register := newOperatorRegister(t)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)

	finance := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-DCG140-FINANCE")
	expectOutcome(t, "bind finance", register.bind(t, bindingOf(t, finance, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	for index, kind := range []accessidentity.DecisionKind{
		accessidentity.DecisionExternalFundsFactAdoption,
		accessidentity.DecisionCustomsDeclarationSubmission,
	} {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-DCG140-F"+string(rune('A'+index)), finance,
			kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, kind.String(), register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	}
	standing := register.standing(t, finance)
	if !standing.HoldsDecisionAt(accessidentity.DecisionExternalFundsFactAdoption, at) ||
		!standing.HoldsDecisionAt(accessidentity.DecisionCustomsDeclarationSubmission, at) {
		t.Fatalf("决定种类没有原样读回：%+v", standing.Grants())
	}
	if standing.HoldsDecisionAt(accessidentity.DecisionRegulatoryCredentialRegistration, at) {
		t.Fatal("持资金事实采用的读成了也持监管凭证登记：两类各自授予")
	}

	broker := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-DCG140-CUSTOMS")
	expectOutcome(t, "bind customs", register.bind(t, bindingOf(t, broker, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", "SYN-DECISION-DCG140-C", broker,
		accessidentity.DecisionRegulatoryCredentialRegistration, interval, "SYN-GRANT-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	expectOutcome(t, "credential", register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	if registered := register.standing(t, broker); !registered.HoldsDecisionAt(accessidentity.DecisionRegulatoryCredentialRegistration, at) ||
		registered.HoldsDecisionAt(accessidentity.DecisionExternalFundsFactAdoption, at) {
		t.Fatalf("监管凭证登记的授予没有原样读回：%+v", registered.Grants())
	}
}
