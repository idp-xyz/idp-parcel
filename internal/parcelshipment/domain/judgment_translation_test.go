package domain_test

import (
	"errors"
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

func pendingRoutingAllowed(t *testing.T) domain.PendingRoutingAllowance {
	t.Helper()
	allowance, err := domain.NewPendingRoutingAllowance(
		mustValue(t, domain.NewPendingRoutingBasis, "PC-PENDING-ROUTING-1"),
	)
	if err != nil {
		t.Fatalf("new pending routing allowance: %v", err)
	}
	return allowance
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
	check, err := domain.ReachabilityCheckFor(
		reachabilityJudgment(t, domain.ReachabilityInsufficientEvidence),
		domain.PendingRoutingAllowance{},
	)
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
	passed, err := domain.ReachabilityCheckFor(
		reachabilityJudgment(t, domain.ReachabilityReachable),
		domain.PendingRoutingAllowance{},
	)
	if err != nil {
		t.Fatalf("reachability check: %v", err)
	}
	if passed.Outcome() != domain.CheckPassed {
		t.Fatalf("outcome = %q, want PASSED", passed.Outcome())
	}

	failed, err := domain.ReachabilityCheckFor(
		reachabilityJudgment(t, domain.ReachabilityUnreachable),
		domain.PendingRoutingAllowance{},
	)
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
	check, err := domain.ReachabilityCheckFor(judgment, domain.PendingRoutingAllowance{})
	if err != nil {
		t.Fatalf("reachability check: %v", err)
	}

	if check.DeclaredParcelID() != judgment.DeclaredParcelID() {
		t.Fatalf("parcel = %q, want %q", check.DeclaredParcelID(), judgment.DeclaredParcelID())
	}
}

// Covers: UC-PS-001「只有服务产品明确允许待路由并保留该商业依据时，才可以在没有可行候选
// 的情况下接受」与 AT-PS-007 — `不可达`就是无可行候选（network-routing 的 ConcludeReachability
// 在候选全部评估过、无一合格且无全局缺口时才给出它），所以待路由许可正是挂在这一值上。
func TestPendingRoutingAllowancePassesAnUnreachableParcel(t *testing.T) {
	check, err := domain.ReachabilityCheckFor(
		reachabilityJudgment(t, domain.ReachabilityUnreachable),
		pendingRoutingAllowed(t),
	)
	if err != nil {
		t.Fatalf("reachability check: %v", err)
	}

	if check.Outcome() != domain.CheckPassed {
		t.Fatalf("outcome = %q; a product that allows pending routing was still blocked", check.Outcome())
	}
}

// Covers: UC-PS-001 接受条件矩阵`标准网络可达性`「资料不足不得映射为不可达」— 待路由许可
// 只赦免`不可达`，不赦免`资料不足`。前者是已经查明没有可行候选，后者是还不知道；拿许可
// 盖住未知，等于在没有判断的情况下接受。
func TestPendingRoutingAllowanceDoesNotRescueInsufficientEvidence(t *testing.T) {
	check, err := domain.ReachabilityCheckFor(
		reachabilityJudgment(t, domain.ReachabilityInsufficientEvidence),
		pendingRoutingAllowed(t),
	)
	if err != nil {
		t.Fatalf("reachability check: %v", err)
	}

	if check.Outcome() != domain.CheckUndetermined {
		t.Fatalf("outcome = %q; an allowance was used to accept an unjudged parcel", check.Outcome())
	}
}

// Covers: UC-PS-001「并保留该商业依据」— 没有依据的待路由许可与一次默认放行分不开，因此
// 构造期就不成立。
func TestAPendingRoutingAllowanceWithoutABasisCannotBeBuilt(t *testing.T) {
	if _, err := domain.NewPendingRoutingAllowance(domain.PendingRoutingBasis{}); !errors.Is(
		err, domain.ErrInvalidPendingRoutingAllowance,
	) {
		t.Fatalf("error = %v, want ErrInvalidPendingRoutingAllowance", err)
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

// Covers: UC-PS-001 接受条件矩阵`接受前财务控制`「不得默认放行」— 从未形成的控制译不出校验
// 结果。挡住「控制没形成却接受」的是 Decide 的适用组覆盖检查；在翻译这一层再造一项
// `无法判定`会是同一条规则的第二处实现，且合同本就不要求财务控制时它永远满足不了。
func TestAnUnformedControlCannotBeTranslated(t *testing.T) {
	if _, err := domain.FinancialControlCheckFor(domain.FinancialControlResult{}); !errors.Is(
		err, domain.ErrInvalidFinancialControlResult,
	) {
		t.Fatalf("error = %v, want ErrInvalidFinancialControlResult", err)
	}
}
