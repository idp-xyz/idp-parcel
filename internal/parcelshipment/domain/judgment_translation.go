package domain

// 本文件把其他上下文拥有的判断译成 parcel-shipment 自己的校验结果。翻译放在领域层而不
// 是编排里，是因为「哪个取值算失败」正是本上下文的接受语言：`network-routing` 只说可达
// 与否，是否据此不接受由这里回答。

// CommercialApplicability 是 parcel-shipment 对一次商业解析的两分：权威把话说完了（适用，
// 或确定不适用），还是根本没得出答案。
//
// 它刻意不是 party-commercial 结果代数的副本。那边的非唯一取值有五种——`无适用依据`、
// `适用冲突`、`解析未决`、`输入未受理`、`已失效`——那是它的语言；本上下文只需要接受条件矩阵
// 自己的那两列，确定性不通过与无法判定。究竟是哪一种由原因引用带过来，不在这里重新声明
// 一套口径。
//
// 压缩只发生在校验结果这一层。编排另按原因取各自的未决原因与续办引用，否则一次越权探测
// （`AT-PC-028` 的`输入未受理`）与一次权威读不到会共用一个引用，续办时催的也是同一个人。
type CommercialApplicability uint8

const (
	CommercialApplicabilityInvalid CommercialApplicability = iota
	CommerciallyApplicable
	CommerciallyNotApplicable
	CommercialApplicabilityUndetermined
)

func (applicability CommercialApplicability) String() string {
	switch applicability {
	case CommerciallyApplicable:
		return "APPLICABLE"
	case CommerciallyNotApplicable:
		return "NOT_APPLICABLE"
	case CommercialApplicabilityUndetermined:
		return "UNDETERMINED"
	default:
		return ""
	}
}

// CommercialBasisChecksFor 把一次商业解析的适用性译成它所回答的那三组校验结果。
//
// 三组共用同一个结论，因为一次唯一解析同时回答客户关系、法人合同与产品服务——那正是`唯一`
// 的含义。解析不成立时本上下文也说不出是三者中的哪一组不成立：说得出，就意味着把
// party-commercial 的判断在这里重做了一遍。原因引用足够按原因维度统计，那是用例的要求。
//
// 非通过必须携带原因，与 NewAcceptanceCheck 的不变量一致：没有原因的拒绝说不出拒的是什么。
func CommercialBasisChecksFor(
	applicability CommercialApplicability,
	reason CheckReason,
) ([]AcceptanceCheck, error) {
	var outcome CheckOutcome
	switch applicability {
	case CommerciallyApplicable:
		outcome = CheckPassed
	case CommerciallyNotApplicable:
		outcome = CheckFailed
	case CommercialApplicabilityUndetermined:
		outcome = CheckUndetermined
	default:
		return nil, ErrInvalidCommercialBasisSnapshot
	}

	groups := []AcceptanceCheckGroup{
		CustomerRelationshipCheck,
		LegalEntityAndContractCheck,
		ProductAndServiceCheck,
	}
	checks := make([]AcceptanceCheck, 0, len(groups))
	for _, group := range groups {
		// 解析没得出答案要由本方重问 party-commercial，客户补什么都改不了这一轮的结论。
		check, err := newCheck(group, DeclaredParcelID{}, outcome, reason, ResumeByInternalRetry)
		if err != nil {
			return nil, err
		}
		checks = append(checks, check)
	}
	return checks, nil
}

// ReachabilityCheckFor 把一次包裹级三值判断译成校验结果。
//
// `资料不足`译成`无法判定`而不是`未通过`：用例明禁把它映射为不可达。一次缺资料可以补齐
// 后重判，而确定性失败会拒掉整份当前提交版本。
//
// `不可达`在服务产品明确允许待路由时译成`通过`。这一条挂在`不可达`而不是别的取值上，是因为
// network-routing 的 ConcludeReachability 只在候选全部评估过、无一合格且无全局缺口时才给出
// 它——那正是用例说的「没有可行候选」。空候选空间在那边被拒绝为`未形成判断`，到不了这里。
//
// 许可赦免不了`资料不足`：那是还不知道有没有可行候选，拿许可盖住未知等于在没有判断的情况下
// 接受。许可所依据的商业事实随快照进入接受决定，用例要求的「保留该商业依据」由此满足。
//
// `不适用`译成`通过`。这不是默认放行：构造期已经强制它携带商业不适用依据，因此与一次没能
// 形成的判断分得开。另外两种译法都不成立——译成`无法判定`会对着一个本就不该问的问题无休止
// 内部重试，译成`不可达`则是在这里替 network-routing 作判断。
func ReachabilityCheckFor(
	judgment ReachabilityJudgment,
	pendingRouting PendingRoutingAllowance,
) (AcceptanceCheck, error) {
	if !judgment.valid() {
		return AcceptanceCheck{}, ErrInvalidReachabilityJudgment
	}

	var outcome CheckOutcome
	var reasonValue string
	// 资料不足由客户来补，不是重问 network-routing 就能变的：权威已经把话说完了，说的正是
	// 声明资料不够判。拿它当内部重试会对着一份没变过的声明资料重试到底，还永远不通知客户。
	resumePath := ResumeByInternalRetry
	// 逐取值分派而不是拿 default 收尾：提供方日后新增一个取值，落到兜底那一格不会有任何
	// 测试变红，而那一格正好是本上下文的接受语言。
	switch judgment.Value() {
	case ReachabilityReachable, ReachabilityNotApplicable:
		outcome = CheckPassed
	case ReachabilityUnreachable:
		if pendingRouting.Allowed() {
			outcome = CheckPassed
			break
		}
		outcome, reasonValue = CheckFailed, "REACHABILITY_UNREACHABLE"
	case ReachabilityInsufficientEvidence:
		outcome, reasonValue = CheckUndetermined, "REACHABILITY_INSUFFICIENT_EVIDENCE"
		resumePath = ResumeByCustomerSupplement
	default:
		return AcceptanceCheck{}, ErrInvalidReachabilityJudgment
	}

	return checkWithReason(NetworkReachabilityCheck, judgment.DeclaredParcelID(), outcome, reasonValue, resumePath)
}

// FinancialControlCheckFor 把一次接受前财务控制结果译成校验结果。
//
// 从未形成的控制在这里被拒绝，而不是译成`无法判定`。防「控制没形成却接受」的是 Decide 的
// 适用组覆盖检查——本组被声明适用却一项校验都没到场就不接受；在这里再造一项`无法判定`是
// 同一条规则的第二处实现，而且合同本就不要求财务控制时那一项永远满足不了，反倒把不适用
// 读成了缺一项。与 ReachabilityCheckFor 拒绝无效判断同理：译不出的输入交调用方处置。
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
		return AcceptanceCheck{}, ErrInvalidFinancialControlResult
	}

	// 不指名成员：控制作用在整份委托上，指名了会让聚合把它当作某个成员已被判断，从而
	// 以遗漏方式放过其余成员。本组没有`无法判定`那一支，续办路径给什么都不会被用上。
	return checkWithReason(
		PreAcceptanceFinancialControlCheck,
		DeclaredParcelID{},
		outcome,
		reasonValue,
		ResumeByInternalRetry,
	)
}

// checkWithReason 只在非通过时形成原因。通过的校验不带原因，与构造器的不变量一致：只有非
// 通过才需要解释。
func checkWithReason(
	group AcceptanceCheckGroup,
	parcelID DeclaredParcelID,
	outcome CheckOutcome,
	reasonValue string,
	resumePath ResumePath,
) (AcceptanceCheck, error) {
	reason := CheckReason{}
	if reasonValue != "" {
		formed, err := NewCheckReason(reasonValue)
		if err != nil {
			return AcceptanceCheck{}, err
		}
		reason = formed
	}
	return newCheck(group, parcelID, outcome, reason, resumePath)
}

// newCheck 按结论挑构造器：`无法判定`必须指名续办方，其余两种不带。翻译函数都走这里，免得
// 每个分支各写一次这个分派。
func newCheck(
	group AcceptanceCheckGroup,
	parcelID DeclaredParcelID,
	outcome CheckOutcome,
	reason CheckReason,
	resumePath ResumePath,
) (AcceptanceCheck, error) {
	if outcome == CheckUndetermined {
		return NewUndeterminedAcceptanceCheck(group, parcelID, reason, resumePath)
	}
	return NewAcceptanceCheck(group, parcelID, outcome, reason)
}
