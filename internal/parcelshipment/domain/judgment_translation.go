package domain

// 本文件把其他上下文拥有的判断译成 parcel-shipment 自己的校验结果。翻译放在领域层而不
// 是编排里，是因为「哪个取值算失败」正是本上下文的接受语言：`network-routing` 只说可达
// 与否，是否据此不接受由这里回答。

// ReachabilityCheckFor 把一次包裹级三值判断译成校验结果。
//
// `资料不足`译成`无法判定`而不是`未通过`：用例明禁把它映射为不可达。一次缺资料可以补齐
// 后重判，而确定性失败会拒掉整份当前提交版本。
//
// 缺口：用例给`不可达`留了「允许待路由产品除外」这一条，本函数尚未实现它——服务产品的
// 待路由许可及其商业依据还没有被 CommercialBasisSnapshot 表达。在那之前`不可达`一律译成
// `未通过`，方向落在拒绝一侧而不是接受一侧，因此不构成默认放行。
func ReachabilityCheckFor(judgment ReachabilityJudgment) (AcceptanceCheck, error) {
	if !judgment.valid() {
		return AcceptanceCheck{}, ErrInvalidReachabilityJudgment
	}

	var outcome CheckOutcome
	var reasonValue string
	switch judgment.Value() {
	case ReachabilityReachable:
		outcome = CheckPassed
	case ReachabilityUnreachable:
		outcome, reasonValue = CheckFailed, "REACHABILITY_UNREACHABLE"
	default:
		outcome, reasonValue = CheckUndetermined, "REACHABILITY_INSUFFICIENT_EVIDENCE"
	}

	return checkWithReason(NetworkReachabilityCheck, judgment.DeclaredParcelID(), outcome, reasonValue)
}

// FinancialControlCheckFor 把一次接受前财务控制结果译成校验结果。
//
// 它接收零值而不是要求调用方先判断有没有结果：没有形成的控制译成`无法判定`，而不是让这
// 一项校验消失。省掉它，聚合会看到「没有失败也没有待判断」而径直接受——那正是用例明禁
// 的默认放行，只不过是以遗漏的方式实现的。
//
// `明确无控制`译成`通过`。这不是默认放行：构造期已经强制该结果携带合同声明的商业不适用
// 依据，因此它与一次没能执行的控制分得开。
func FinancialControlCheckFor(result FinancialControlResult) (AcceptanceCheck, error) {
	var outcome CheckOutcome
	var reasonValue string
	switch result.Outcome() {
	case FinancialControlHeld, FinancialControlNotApplicable:
		outcome = CheckPassed
	case FinancialControlRestricted:
		outcome, reasonValue = CheckFailed, "FINANCIAL_CONTROL_RESTRICTED"
	default:
		outcome, reasonValue = CheckUndetermined, "FINANCIAL_CONTROL_NOT_FORMED"
	}

	// 不指名成员：控制作用在整份委托上，指名了会让聚合把它当作某个成员已被判断，从而
	// 以遗漏方式放过其余成员。
	return checkWithReason(PreAcceptanceFinancialControlCheck, DeclaredParcelID{}, outcome, reasonValue)
}

// checkWithReason 只在非通过时形成原因。通过的校验不带原因，与 NewAcceptanceCheck 的
// 不变量一致：只有非通过才需要解释。
func checkWithReason(
	group AcceptanceCheckGroup,
	parcelID DeclaredParcelID,
	outcome CheckOutcome,
	reasonValue string,
) (AcceptanceCheck, error) {
	reason := CheckReason{}
	if reasonValue != "" {
		formed, err := NewCheckReason(reasonValue)
		if err != nil {
			return AcceptanceCheck{}, err
		}
		reason = formed
	}
	return NewAcceptanceCheck(group, parcelID, outcome, reason)
}
