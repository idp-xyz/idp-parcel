package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func reachabilityAsOf(t *testing.T) domain.JudgmentAsOf {
	t.Helper()
	asOf, err := domain.NewDeclaredAsOf(
		domain.ReachabilityJudgmentKind,
		controlPolicyFormedAsOf,
		mustValue(t, domain.NewAsOfPolicyVersion, "asof-policy-v1"),
	)
	if err != nil {
		t.Fatalf("new declared asOf: %v", err)
	}
	return asOf
}

func reachabilityJudgment(t *testing.T, value domain.ReachabilityValue) domain.ReachabilityJudgment {
	t.Helper()
	judgment, err := domain.NewReachabilityJudgment(
		mustValue(t, domain.NewReachabilityJudgmentID, "NRJ-1"),
		mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		value,
		reachabilityAsOf(t),
	)
	if err != nil {
		t.Fatalf("new reachability judgment: %v", err)
	}
	return judgment
}

// Covers: UC-PS-001 接受条件矩阵`标准网络可达性`「资料不足不得映射为不可达」— 三值判断
// 的第三值形成`无法判定`。译成`未通过`会让一次缺资料变成确定性拒绝，而两者的续办路径和
// 拒绝统计都不是一回事。
func TestInsufficientEvidenceIsUndeterminedNotUnreachable(t *testing.T) {
	check, err := domain.ReachabilityCheckFor(reachabilityJudgment(t, domain.ReachabilityInsufficientEvidence))
	if err != nil {
		t.Fatalf("reachability check: %v", err)
	}

	if check.Outcome() != domain.CheckUndetermined {
		t.Fatalf("outcome = %q, want UNDETERMINED", check.Outcome())
	}
	if check.Group() != domain.NetworkReachabilityCheck {
		t.Fatalf("group = %q, want NETWORK_REACHABILITY", check.Group())
	}
	if check.Reason().String() == "" {
		t.Fatal("an undetermined check carries no structured reason")
	}
}

// Covers: UC-PS-001 接受条件矩阵`标准网络可达性`「明确不可达时整份当前提交版本不能直接
// 接受」— 可达通过、不可达失败，且失败必须携带结构化原因供拒绝按原因维度统计。
func TestReachableAndUnreachableTranslateToPassAndFailure(t *testing.T) {
	passed, err := domain.ReachabilityCheckFor(reachabilityJudgment(t, domain.ReachabilityReachable))
	if err != nil {
		t.Fatalf("reachability check: %v", err)
	}
	if passed.Outcome() != domain.CheckPassed {
		t.Fatalf("outcome = %q, want PASSED", passed.Outcome())
	}

	failed, err := domain.ReachabilityCheckFor(reachabilityJudgment(t, domain.ReachabilityUnreachable))
	if err != nil {
		t.Fatalf("reachability check: %v", err)
	}
	if failed.Outcome() != domain.CheckFailed {
		t.Fatalf("outcome = %q, want FAILED", failed.Outcome())
	}
	if failed.Reason().String() == passed.Reason().String() {
		t.Fatal("unreachable and reachable report the same reason")
	}
}

// Covers: UC-PS-001 接受基线「接受基线覆盖当前提交版本的完整声明成员」— 校验结果必须
// 指名它判断的那个成员，否则聚合分辨不出哪个成员被判断过。
func TestAReachabilityCheckNamesTheParcelItJudged(t *testing.T) {
	judgment := reachabilityJudgment(t, domain.ReachabilityReachable)
	check, err := domain.ReachabilityCheckFor(judgment)
	if err != nil {
		t.Fatalf("reachability check: %v", err)
	}

	if check.DeclaredParcelID() != judgment.DeclaredParcelID() {
		t.Fatalf("parcel = %q, want %q", check.DeclaredParcelID(), judgment.DeclaredParcelID())
	}
}

func financialControlResult(t *testing.T, outcome domain.FinancialControlOutcome) domain.FinancialControlResult {
	t.Helper()
	basis := domain.ControlBasisReference{}
	if outcome != domain.FinancialControlHeld {
		basis = mustValue(t, domain.NewControlBasisReference, "PC-CONTROL-BASIS-1")
	}
	result, err := domain.NewFinancialControlResult(
		mustValue(t, domain.NewFinancialControlResultID, "SAC-1"),
		outcome,
		basis,
		financialControlAsOf(t),
	)
	if err != nil {
		t.Fatalf("new financial control result: %v", err)
	}
	return result
}

// Covers: UC-PS-001 接受条件矩阵`接受前财务控制`「任一必需控制不通过时按策略拒绝」—
// `已冻结`通过、`业务限制`失败。`明确无控制`也通过，但它凭的是合同声明的商业不适用依据：
// 构造期已经强制它携带依据，所以这一条通过与「默认信用通过」分得开。
func TestFinancialControlOutcomesTranslateToChecks(t *testing.T) {
	expected := map[domain.FinancialControlOutcome]domain.CheckOutcome{
		domain.FinancialControlHeld:          domain.CheckPassed,
		domain.FinancialControlNotApplicable: domain.CheckPassed,
		domain.FinancialControlRestricted:    domain.CheckFailed,
	}

	for outcome, want := range expected {
		t.Run(outcome.String(), func(t *testing.T) {
			check, err := domain.FinancialControlCheckFor(financialControlResult(t, outcome))
			if err != nil {
				t.Fatalf("financial control check: %v", err)
			}

			if check.Outcome() != want {
				t.Fatalf("outcome = %q, want %q", check.Outcome(), want)
			}
			if check.Group() != domain.PreAcceptanceFinancialControlCheck {
				t.Fatalf("group = %q, want PRE_ACCEPTANCE_FINANCIAL_CONTROL", check.Group())
			}
			// 财务控制作用在整份委托上，不指名成员：指名了会让聚合把它当成某个成员
			// 已被判断，从而以遗漏方式放过其余成员。
			if check.DeclaredParcelID().String() != "" {
				t.Fatalf("parcel = %q; the control applies to the whole request", check.DeclaredParcelID())
			}
		})
	}
}

// Covers: UC-PS-001 接受条件矩阵`接受前财务控制`「不得默认放行」— 控制根本没有形成时译成
// `无法判定`。省掉这一项校验，聚合会看到「没有失败也没有待判断」而径直接受，那是一次以
// 遗漏方式实现的默认放行。
func TestAnUnformedControlBecomesUndeterminedRatherThanVanishing(t *testing.T) {
	check, err := domain.FinancialControlCheckFor(domain.FinancialControlResult{})
	if err != nil {
		t.Fatalf("financial control check: %v", err)
	}

	if check.Outcome() != domain.CheckUndetermined {
		t.Fatalf("outcome = %q, want UNDETERMINED", check.Outcome())
	}
	if check.Group() != domain.PreAcceptanceFinancialControlCheck {
		t.Fatalf("group = %q, want PRE_ACCEPTANCE_FINANCIAL_CONTROL", check.Group())
	}
	if check.Reason().String() == "" {
		t.Fatal("an undetermined check carries no structured reason")
	}
}
