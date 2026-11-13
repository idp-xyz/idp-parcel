package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// Covers: 迁移 access_identity 0044 的册面——异常案件接单、关闭、归并三种运营决定的授予都登得进操作者册、读回仍是各自
// 那一种；先放开的监管限制解除仍登得进去，证约束重建时没有漏抄已有的种类。
func TestTheExceptionCaseDecisionsAreGrantedAndReadBack(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, "https://id.syn.example/dex", "SYN-OPERATOR-EXCEPTION-CASE")
	expectOutcome(t, "bind", register.bind(t, bindingOf(t, subject, "SYN-TENANT-01", "SYN-BIND-BASIS")), accessidentity.OperatorRegistrationRecorded)
	interval, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]accessidentity.DecisionKind{
		"SYN-DECISION-EC-TAKE-UP": accessidentity.DecisionExceptionCaseTakeUp,
		"SYN-DECISION-EC-CLOSURE": accessidentity.DecisionExceptionCaseClosure,
		"SYN-DECISION-EC-MERGE":   accessidentity.DecisionExceptionCaseMerge,
		"SYN-DECISION-EC-RELEASE": accessidentity.DecisionRegulatoryRestrictionRelease,
	}
	for grantID, kind := range kinds {
		grant, err := accessidentity.NewOperationDecisionGrant("SYN-TENANT-01", grantID, subject, kind, interval, "SYN-GRANT-BASIS")
		if err != nil {
			t.Fatal(err)
		}
		expectOutcome(t, string(kind)+" decision grant", register.grant(t, grant), accessidentity.OperatorRegistrationRecorded)
	}
	standing := register.standing(t, subject)
	at := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	for _, kind := range kinds {
		if !standing.HoldsDecisionAt(kind, at) {
			t.Fatalf("%s 没有原样读回：%+v", kind, standing.Grants())
		}
	}
	if standing.HoldsDecisionAt(accessidentity.DecisionRegulatoryRestrictionEstablishment, at) {
		t.Fatal("没授过的决定种类读成了持有")
	}
}
