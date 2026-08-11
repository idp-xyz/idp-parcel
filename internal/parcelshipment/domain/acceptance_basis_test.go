package domain_test

import (
	"errors"
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

// Covers: UC-PS-001 步骤 7 与接受条件「不得默认放行」— 非`已冻结`的结果必须携带依据。
// 没有依据的`明确无控制`与一次默认信用通过无从分辨，而后者是用例明禁的。
func TestANonHeldControlResultWithoutABasisCannotBeBuilt(t *testing.T) {
	for _, outcome := range []domain.FinancialControlOutcome{
		domain.FinancialControlRestricted,
		domain.FinancialControlNotApplicable,
	} {
		t.Run(outcome.String(), func(t *testing.T) {
			if _, err := domain.NewFinancialControlResult(
				mustValue(t, domain.NewFinancialControlResultID, "SAC-1"),
				outcome,
				domain.ControlBasisReference{},
				financialControlAsOf(t),
			); !errors.Is(err, domain.ErrInvalidFinancialControlResult) {
				t.Fatalf("error = %v, want ErrInvalidFinancialControlResult", err)
			}
		})
	}
}

// Covers: CONTEXT「只采用预付冻结、信用校验、明确无控制或合同明确组合所形成的权威结果」
// —— 一次实际执行过的冻结自己就是依据，要求它另附一份会把执行过的控制挡在门外。
func TestAHeldControlResultStandsOnTheFreezeItself(t *testing.T) {
	result, err := domain.NewFinancialControlResult(
		mustValue(t, domain.NewFinancialControlResultID, "SAC-1"),
		domain.FinancialControlHeld,
		domain.ControlBasisReference{},
		financialControlAsOf(t),
	)
	if err != nil {
		t.Fatalf("new financial control result: %v", err)
	}
	if result.Outcome() != domain.FinancialControlHeld {
		t.Fatalf("outcome = %q, want HELD", result.Outcome())
	}
}

// Covers: ADR-0027「消费方凭据不持有提供方不曾签发的东西」— settlement-accounting 在
// `明确无控制`那一支下不形成冻结，因而没有结果标识可交回；要求一个，只能由适配器发明。
// 其余取值反过来必须带标识。
func TestOnlyAnExecutedControlCarriesAnAuthorityIdentifier(t *testing.T) {
	basis := mustValue(t, domain.NewControlBasisReference, "CONTRACT_DECLARES_NO_PRE_ACCEPTANCE_CONTROL")
	notApplicable, err := domain.NewFinancialControlResult(
		domain.FinancialControlResultID{},
		domain.FinancialControlNotApplicable,
		basis,
		financialControlAsOf(t),
	)
	if err != nil {
		t.Fatalf("new financial control result: %v——无控制被要求提供一个权威没签发的标识", err)
	}
	if notApplicable.ResultID().String() != "" {
		t.Fatalf("result ID = %q; 无控制凭空得到了一个标识", notApplicable.ResultID())
	}

	if _, err := domain.NewFinancialControlResult(
		domain.FinancialControlResultID{},
		domain.FinancialControlHeld,
		domain.ControlBasisReference{},
		financialControlAsOf(t),
	); !errors.Is(err, domain.ErrInvalidFinancialControlResult) {
		t.Fatalf("error = %v, want ErrInvalidFinancialControlResult——执行过的控制没有标识却被接受", err)
	}
}

// Covers: UC-PS-001 步骤 4B「权威提供方校验并回显」— 控制结果必须带回它判断时所依据的
// 时点；没有时点的结果说不出自己按哪一版策略判过。
func TestAControlResultWithoutTheJudgedAsOfCannotBeBuilt(t *testing.T) {
	if _, err := domain.NewFinancialControlResult(
		mustValue(t, domain.NewFinancialControlResultID, "SAC-1"),
		domain.FinancialControlHeld,
		domain.ControlBasisReference{},
		domain.JudgmentAsOf{},
	); !errors.Is(err, domain.ErrInvalidFinancialControlResult) {
		t.Fatalf("error = %v, want ErrInvalidFinancialControlResult", err)
	}
}
