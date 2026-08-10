package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidCommercialBasisSnapshot = errors.New("parcel shipment: invalid commercial basis snapshot")
	ErrInvalidDeclaredAsOf            = errors.New("parcel shipment: invalid declared as-of")
	ErrInvalidReachabilityJudgment    = errors.New("parcel shipment: invalid reachability judgment")
	ErrInvalidFinancialControlResult  = errors.New("parcel shipment: invalid financial control result")
	ErrInvalidApplicableCheckGroups   = errors.New("parcel shipment: invalid applicable check groups")
	ErrInvalidPendingRoutingAllowance = errors.New("parcel shipment: invalid pending routing allowance")
)

// PendingRoutingBasis 是服务产品允许「无可行候选也可接受」所依据的商业事实。依据本身属
// party-commercial，这里只记引用。
type PendingRoutingBasis struct{ requiredValue }

func NewPendingRoutingBasis(value string) (PendingRoutingBasis, error) {
	required, err := newRequiredValue("pending routing basis", value)
	return PendingRoutingBasis{required}, err
}

// PendingRoutingAllowance 是所采用服务产品对待路由的明确许可。零值是「未许可」。
//
// 许可必须携带依据：用例只在「服务产品明确允许待路由并保留该商业依据」时才准许在没有可行
// 候选的情况下接受，没有依据的许可与一次默认放行分不开。依据随商业依据快照进入接受决定与
// 预计承诺，因此「保留」由快照本身完成，不需要另建一条保存路径。
type PendingRoutingAllowance struct {
	basis PendingRoutingBasis
}

func NewPendingRoutingAllowance(basis PendingRoutingBasis) (PendingRoutingAllowance, error) {
	if !basis.valid() {
		return PendingRoutingAllowance{}, ErrInvalidPendingRoutingAllowance
	}
	return PendingRoutingAllowance{basis: basis}, nil
}

func (allowance PendingRoutingAllowance) Allowed() bool {
	return allowance.basis.valid()
}

func (allowance PendingRoutingAllowance) Basis() PendingRoutingBasis {
	return allowance.basis
}

// ApplicableCheckGroups 是所采用接单规则包为当前提交版本声明的适用校验组集合。
//
// 它区分「没有声明」与「声明了集合」，零值是前者。没有声明不等于没有组适用，而是无从知道
// 该判哪些组——用例把适用性判给 PC-RULE，本上下文不得代它拟一个默认集合。
//
// 构造期拒绝空集，因此凡是声明过的集合都非空：一个不要求任何校验的规则包等于无条件接受，
// 那不是规则包该能表达的东西。
type ApplicableCheckGroups struct {
	groups []AcceptanceCheckGroup
}

func NewApplicableCheckGroups(groups ...AcceptanceCheckGroup) (ApplicableCheckGroups, error) {
	if len(groups) == 0 {
		return ApplicableCheckGroups{}, ErrInvalidApplicableCheckGroups
	}
	seen := make(map[AcceptanceCheckGroup]struct{}, len(groups))
	for _, group := range groups {
		if !group.valid() {
			return ApplicableCheckGroups{}, ErrInvalidApplicableCheckGroups
		}
		if _, exists := seen[group]; exists {
			return ApplicableCheckGroups{}, ErrInvalidApplicableCheckGroups
		}
		seen[group] = struct{}{}
	}
	return ApplicableCheckGroups{groups: append([]AcceptanceCheckGroup(nil), groups...)}, nil
}

func (applicable ApplicableCheckGroups) Declared() bool {
	return len(applicable.groups) > 0
}

// ManualReviewPolicy 是所采用接单规则包对「这份委托要不要人工复核」的声明。
//
// 它刻意没有`已完成`：复核做没做完是运营发生的事，属本上下文接受判断任务的状态，规则包
// 不拥有它。把三值的 ManualReviewState 整个挂到商业依据上，等于让 party-commercial 声明
// 一件它不拥有的事实。编排把本策略与任务侧的完成情况合成 ManualReviewState。
//
// 零值是「未声明」。缺声明不等于不要求复核——用例把复核触发条件判给规则包，本上下文在这里
// 兜任何一边都是替它作答。
type ManualReviewPolicy uint8

const (
	ManualReviewNotDeclaredByRules ManualReviewPolicy = iota
	ManualReviewNotRequiredByRules
	ManualReviewRequiredByRules
)

func (policy ManualReviewPolicy) Declared() bool {
	return policy == ManualReviewNotRequiredByRules || policy == ManualReviewRequiredByRules
}

func (policy ManualReviewPolicy) String() string {
	switch policy {
	case ManualReviewNotRequiredByRules:
		return "NOT_REQUIRED"
	case ManualReviewRequiredByRules:
		return "REQUIRED"
	default:
		return ""
	}
}

// 这里的类型是 parcel-shipment 自己对其他上下文所拥有事实的引用。party-commercial 与
// network-routing 各自保有自己的模型；本上下文只记录所采用的引用与快照——这正是两边
// 各自演进而互不改写对方对象的原因。

type CommercialResolutionID struct{ requiredValue }

func NewCommercialResolutionID(value string) (CommercialResolutionID, error) {
	required, err := newRequiredValue("commercial resolution ID", value)
	return CommercialResolutionID{required}, err
}

type RulePackageReference struct{ requiredValue }

func NewRulePackageReference(value string) (RulePackageReference, error) {
	required, err := newRequiredValue("rule package reference", value)
	return RulePackageReference{required}, err
}

type CommercialViewRevision struct{ requiredValue }

func NewCommercialViewRevision(value string) (CommercialViewRevision, error) {
	required, err := newRequiredValue("commercial view revision", value)
	return CommercialViewRevision{required}, err
}

type AsOfPolicyVersion struct{ requiredValue }

func NewAsOfPolicyVersion(value string) (AsOfPolicyVersion, error) {
	required, err := newRequiredValue("as-of policy version", value)
	return AsOfPolicyVersion{required}, err
}

type ReachabilityJudgmentID struct{ requiredValue }

func NewReachabilityJudgmentID(value string) (ReachabilityJudgmentID, error) {
	required, err := newRequiredValue("reachability judgment ID", value)
	return ReachabilityJudgmentID{required}, err
}

// JudgmentKind 指名一类由所采用规则包声明 `asOf` 策略的下游判断。取值与消费它的编排
// 同时出现。
type JudgmentKind uint8

const (
	JudgmentKindInvalid JudgmentKind = iota
	ReachabilityJudgmentKind
	FinancialControlJudgmentKind
)

func (kind JudgmentKind) valid() bool {
	return kind >= ReachabilityJudgmentKind && kind <= FinancialControlJudgmentKind
}

func (kind JudgmentKind) String() string {
	switch kind {
	case ReachabilityJudgmentKind:
		return "REACHABILITY"
	case FinancialControlJudgmentKind:
		return "FINANCIAL_CONTROL"
	default:
		return ""
	}
}

// DeclaredAsOf 是所采用规则包为某一类判断声明的时点。parcel-shipment 据此形成值，
// 绝不自己发明一个，所以没有声明的判断根本无法继续。
type DeclaredAsOf struct {
	kind          JudgmentKind
	at            time.Time
	policyVersion AsOfPolicyVersion
}

func NewDeclaredAsOf(kind JudgmentKind, at time.Time, policyVersion AsOfPolicyVersion) (DeclaredAsOf, error) {
	if !kind.valid() || at.IsZero() || !policyVersion.valid() {
		return DeclaredAsOf{}, ErrInvalidDeclaredAsOf
	}
	return DeclaredAsOf{kind: kind, at: at.UTC(), policyVersion: policyVersion}, nil
}

func (declared DeclaredAsOf) Kind() JudgmentKind {
	return declared.kind
}

func (declared DeclaredAsOf) At() time.Time {
	return declared.at
}

func (declared DeclaredAsOf) PolicyVersion() AsOfPolicyVersion {
	return declared.policyVersion
}

// JudgmentAsOf 是实际送给权威提供方、并由其校验回显的时点。它与声明同形，因为形成值
// 时不得添加任何策略没有授权的东西。
type JudgmentAsOf = DeclaredAsOf

// CommercialBasisSnapshot 是 parcel-shipment 对一次唯一商业解析所保留的部分：解析
// 标识、采用的接单规则包、解析当时的权威视图修订，以及该规则包声明的各项时点。它不
// 持有任何商业版本内容，那些内容属 party-commercial。
type CommercialBasisSnapshot struct {
	resolutionID   CommercialResolutionID
	rulePackage    RulePackageReference
	viewRevision   CommercialViewRevision
	declaredAsOf   []DeclaredAsOf
	applicable     ApplicableCheckGroups
	manualReview   ManualReviewPolicy
	pendingRouting PendingRoutingAllowance
}

// CommercialBasisSnapshotSpec 是形成一次快照所需的全部输入。用结构体而不是位置参数，
// 是因为后四项都是所采用规则包与服务产品的声明：每多一条声明就多一个参数，位置参数会
// 让调用点变成一串认不出的同型值。与 AcceptanceDecisionSpec 同一形状。
type CommercialBasisSnapshotSpec struct {
	ResolutionID   CommercialResolutionID
	RulePackage    RulePackageReference
	ViewRevision   CommercialViewRevision
	DeclaredAsOf   []DeclaredAsOf
	Applicable     ApplicableCheckGroups
	ManualReview   ManualReviewPolicy
	PendingRouting PendingRoutingAllowance
}

func NewCommercialBasisSnapshot(spec CommercialBasisSnapshotSpec) (CommercialBasisSnapshot, error) {
	if !spec.ResolutionID.valid() || !spec.RulePackage.valid() || !spec.ViewRevision.valid() {
		return CommercialBasisSnapshot{}, ErrInvalidCommercialBasisSnapshot
	}
	seen := make(map[JudgmentKind]struct{}, len(spec.DeclaredAsOf))
	for _, declared := range spec.DeclaredAsOf {
		if !declared.kind.valid() || declared.at.IsZero() || !declared.policyVersion.valid() {
			return CommercialBasisSnapshot{}, ErrInvalidDeclaredAsOf
		}
		if _, exists := seen[declared.kind]; exists {
			return CommercialBasisSnapshot{}, ErrInvalidDeclaredAsOf
		}
		seen[declared.kind] = struct{}{}
	}
	return CommercialBasisSnapshot{
		resolutionID:   spec.ResolutionID,
		rulePackage:    spec.RulePackage,
		viewRevision:   spec.ViewRevision,
		declaredAsOf:   append([]DeclaredAsOf(nil), spec.DeclaredAsOf...),
		applicable:     spec.Applicable,
		manualReview:   spec.ManualReview,
		pendingRouting: spec.PendingRouting,
	}, nil
}

// PendingRoutingAllowance 返回所采用服务产品对待路由的许可。零值即未许可——`不可达`因此
// 拒绝整份版本，方向落在拒绝一侧。
func (snapshot CommercialBasisSnapshot) PendingRoutingAllowance() PendingRoutingAllowance {
	return snapshot.pendingRouting
}

// ManualReviewPolicy 返回规则包对人工复核的声明。未声明时返回零值——编排据此保持未决，
// 不替规则包在「要求」与「不要求」之间挑一个。
func (snapshot CommercialBasisSnapshot) ManualReviewPolicy() ManualReviewPolicy {
	return snapshot.manualReview
}

func (snapshot CommercialBasisSnapshot) ResolutionID() CommercialResolutionID {
	return snapshot.resolutionID
}

func (snapshot CommercialBasisSnapshot) RulePackage() RulePackageReference {
	return snapshot.rulePackage
}

func (snapshot CommercialBasisSnapshot) ViewRevision() CommercialViewRevision {
	return snapshot.viewRevision
}

// AsOfFor 返回规则包为某一类判断声明的时点。没有声明时报告缺席而不是给默认值——在这里
// 顶上任何一个时刻，正是用例禁止的「用一个全局时间代替」。
func (snapshot CommercialBasisSnapshot) AsOfFor(kind JudgmentKind) (JudgmentAsOf, bool) {
	for _, declared := range snapshot.declaredAsOf {
		if declared.kind == kind {
			return declared, true
		}
	}
	return DeclaredAsOf{}, false
}

func (snapshot CommercialBasisSnapshot) valid() bool {
	return snapshot.resolutionID.valid() && snapshot.rulePackage.valid() && snapshot.viewRevision.valid()
}

// ReachabilityValue 以采用引用的形式镜像 network-routing 的三值判断。parcel-shipment
// 从不产生它，只记录拥有它的上下文判断了什么；三个取值没有一个是接受决定。
type ReachabilityValue uint8

const (
	ReachabilityValueInvalid ReachabilityValue = iota
	ReachabilityReachable
	ReachabilityUnreachable
	ReachabilityInsufficientEvidence
)

func (value ReachabilityValue) valid() bool {
	return value >= ReachabilityReachable && value <= ReachabilityInsufficientEvidence
}

func (value ReachabilityValue) String() string {
	switch value {
	case ReachabilityReachable:
		return "REACHABLE"
	case ReachabilityUnreachable:
		return "UNREACHABLE"
	case ReachabilityInsufficientEvidence:
		return "INSUFFICIENT_EVIDENCE"
	default:
		return ""
	}
}

type ReachabilityJudgment struct {
	judgmentID ReachabilityJudgmentID
	parcelID   DeclaredParcelID
	value      ReachabilityValue
	asOf       JudgmentAsOf
}

func NewReachabilityJudgment(
	judgmentID ReachabilityJudgmentID,
	parcelID DeclaredParcelID,
	value ReachabilityValue,
	asOf JudgmentAsOf,
) (ReachabilityJudgment, error) {
	if !judgmentID.valid() || !parcelID.valid() || !value.valid() ||
		asOf.at.IsZero() || !asOf.policyVersion.valid() {
		return ReachabilityJudgment{}, ErrInvalidReachabilityJudgment
	}
	return ReachabilityJudgment{judgmentID: judgmentID, parcelID: parcelID, value: value, asOf: asOf}, nil
}

func (judgment ReachabilityJudgment) JudgmentID() ReachabilityJudgmentID {
	return judgment.judgmentID
}

func (judgment ReachabilityJudgment) DeclaredParcelID() DeclaredParcelID {
	return judgment.parcelID
}

func (judgment ReachabilityJudgment) Value() ReachabilityValue {
	return judgment.value
}

func (judgment ReachabilityJudgment) AsOf() JudgmentAsOf {
	return judgment.asOf
}

func (judgment ReachabilityJudgment) valid() bool {
	return judgment.judgmentID.valid() && judgment.parcelID.valid() && judgment.value.valid()
}

type FinancialControlResultID struct{ requiredValue }

func NewFinancialControlResultID(value string) (FinancialControlResultID, error) {
	required, err := newRequiredValue("financial control result ID", value)
	return FinancialControlResultID{required}, err
}

// ControlBasisReference 是一次非通过的接受前财务控制所依据的事实：业务限制的原因，或者
// 合同明确无控制的商业不适用依据。用例把后者的保存责任判给本上下文，但依据本身来自
// party-commercial，这里只记引用。
type ControlBasisReference struct{ requiredValue }

func NewControlBasisReference(value string) (ControlBasisReference, error) {
	required, err := newRequiredValue("control basis reference", value)
	return ControlBasisReference{required}, err
}

// FinancialControlOutcome 以采用引用的形式镜像 settlement-accounting 的接受前控制结果。
// 三个取值没有一个是接受决定，也刻意没有第四个「视同通过」——用例对本步的要求是不得默认
// 放行，而一个表示「没控制成但先过」的取值正是默认放行的载体。控制没能形成时，编排保持
// 判断任务未决，不在这里凑一个结果。
type FinancialControlOutcome uint8

const (
	FinancialControlOutcomeInvalid FinancialControlOutcome = iota
	FinancialControlHeld
	FinancialControlRestricted
	FinancialControlNotApplicable
)

func (outcome FinancialControlOutcome) valid() bool {
	return outcome >= FinancialControlHeld && outcome <= FinancialControlNotApplicable
}

func (outcome FinancialControlOutcome) String() string {
	switch outcome {
	case FinancialControlHeld:
		return "HELD"
	case FinancialControlRestricted:
		return "RESTRICTED"
	case FinancialControlNotApplicable:
		return "NOT_APPLICABLE"
	default:
		return ""
	}
}

// FinancialControlResult 是本上下文对一次接受前财务控制所保留的引用。
//
// 除`已冻结`外的结果都必须携带依据，与 RouteCandidate 要求非合格候选必须带淘汰原因同理：
// 没有依据的`业务限制`说不出限制什么，没有依据的`明确无控制`则与「默认信用通过」无从分辨，
// 而用例明禁后者。
type FinancialControlResult struct {
	resultID FinancialControlResultID
	outcome  FinancialControlOutcome
	basis    ControlBasisReference
	asOf     JudgmentAsOf
}

func NewFinancialControlResult(
	resultID FinancialControlResultID,
	outcome FinancialControlOutcome,
	basis ControlBasisReference,
	asOf JudgmentAsOf,
) (FinancialControlResult, error) {
	if !resultID.valid() || !outcome.valid() ||
		asOf.at.IsZero() || !asOf.policyVersion.valid() {
		return FinancialControlResult{}, ErrInvalidFinancialControlResult
	}
	if outcome != FinancialControlHeld && !basis.valid() {
		return FinancialControlResult{}, ErrInvalidFinancialControlResult
	}
	return FinancialControlResult{resultID: resultID, outcome: outcome, basis: basis, asOf: asOf}, nil
}

func (result FinancialControlResult) ResultID() FinancialControlResultID {
	return result.resultID
}

func (result FinancialControlResult) Outcome() FinancialControlOutcome {
	return result.outcome
}

func (result FinancialControlResult) Basis() ControlBasisReference {
	return result.basis
}

func (result FinancialControlResult) AsOf() JudgmentAsOf {
	return result.asOf
}
