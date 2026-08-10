package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

var controlPolicyFormedAsOf = time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)

func financialControlAsOf(t *testing.T) domain.JudgmentAsOf {
	t.Helper()
	asOf, err := domain.NewDeclaredAsOf(
		domain.FinancialControlJudgmentKind,
		controlPolicyFormedAsOf,
		mustValue(t, domain.NewAsOfPolicyVersion, "asof-policy-v1"),
	)
	if err != nil {
		t.Fatalf("new declared asOf: %v", err)
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
