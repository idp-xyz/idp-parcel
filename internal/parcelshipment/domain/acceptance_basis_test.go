package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

var controlPolicyFormedAsOf = time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)

// declaredAsOfFor 是规则包的声明：只有语义与政策版本，没有值。
func declaredAsOfFor(t *testing.T, kind domain.JudgmentKind) domain.DeclaredAsOf {
	t.Helper()
	declared, err := domain.NewDeclaredAsOf(
		kind,
		mustValue(t, domain.NewAsOfSemanticsReference, "ASOF-SEMANTICS-"+kind.String()),
		mustValue(t, domain.NewAsOfPolicyVersion, "asof-policy-v1"),
	)
	if err != nil {
		t.Fatalf("new declared asOf: %v", err)
	}
	return declared
}

// echoedAsOfFor 冒充提供方第二阶段随校验结果交回的那份政策。它与声明另立一型，因此夹具
// 也得走这条路——把声明直接塞进 NewJudgmentAsOf 已经编译不过。
func echoedAsOfFor(t *testing.T, kind domain.JudgmentKind) domain.EchoedAsOfPolicy {
	t.Helper()
	echoed, err := domain.NewEchoedAsOfPolicy(
		kind,
		mustValue(t, domain.NewAsOfSemanticsReference, "ASOF-SEMANTICS-"+kind.String()),
		mustValue(t, domain.NewAsOfPolicyVersion, "asof-policy-v1"),
	)
	if err != nil {
		t.Fatalf("new echoed asOf policy: %v", err)
	}
	return echoed
}

// financialControlAsOf 是已由提供方校验回显的时点，即第二阶段的产物。
func financialControlAsOf(t *testing.T) domain.JudgmentAsOf {
	t.Helper()
	asOf, err := domain.NewJudgmentAsOf(
		controlPolicyFormedAsOf,
		echoedAsOfFor(t, domain.FinancialControlJudgmentKind),
	)
	if err != nil {
		t.Fatalf("new judgment asOf: %v", err)
	}
	return asOf
}
