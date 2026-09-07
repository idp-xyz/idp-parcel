package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidCommercialBasisSnapshot = errors.New("parcel shipment: invalid commercial basis snapshot")
	ErrInvalidDeclaredAsOf            = errors.New("parcel shipment: invalid declared as-of")
	ErrInvalidJudgmentAsOf            = errors.New("parcel shipment: invalid judgment as-of")
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

// Groups 交回声明的适用校验组。重建与快照往返按公开访问器保全，不能读未导出切片。
func (applicable ApplicableCheckGroups) Groups() []AcceptanceCheckGroup {
	return append([]AcceptanceCheckGroup(nil), applicable.groups...)
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

// AsOfSemanticsReference 标明一项判断锚到哪个业务时点。它保持为不透明引用：语义属
// party-commercial 登记的版本化政策，本上下文只携带引用，既不解释它，也不提供一组取值
// 供人挑选。
type AsOfSemanticsReference struct{ requiredValue }

func NewAsOfSemanticsReference(value string) (AsOfSemanticsReference, error) {
	required, err := newRequiredValue("as-of semantics reference", value)
	return AsOfSemanticsReference{required}, err
}

// DeclaredAsOf 是所采用规则包为某一类判断声明的时点锚：锚到哪种业务时点语义，依据该政策的
// 哪个版本。没有声明的判断根本无法继续。
//
// 它刻意不带值。值由本上下文逐项形成、再由 party-commercial 校验回显（`UC-PC-002` 步骤 6），
// 声明这一头本来就只有语义没有时刻。给它一个时刻字段，适配器就只能在第一阶段结果里填一个
// 无人授权的值——那正是「不得用一个全局时间代替」要挡的事。
type DeclaredAsOf struct {
	kind          JudgmentKind
	semantics     AsOfSemanticsReference
	policyVersion AsOfPolicyVersion
}

func NewDeclaredAsOf(
	kind JudgmentKind,
	semantics AsOfSemanticsReference,
	policyVersion AsOfPolicyVersion,
) (DeclaredAsOf, error) {
	if !kind.valid() || !semantics.valid() || !policyVersion.valid() {
		return DeclaredAsOf{}, ErrInvalidDeclaredAsOf
	}
	return DeclaredAsOf{kind: kind, semantics: semantics, policyVersion: policyVersion}, nil
}

func (declared DeclaredAsOf) Kind() JudgmentKind {
	return declared.kind
}

func (declared DeclaredAsOf) Semantics() AsOfSemanticsReference {
	return declared.semantics
}

func (declared DeclaredAsOf) PolicyVersion() AsOfPolicyVersion {
	return declared.policyVersion
}

func (declared DeclaredAsOf) valid() bool {
	return declared.kind.valid() && declared.semantics.valid() && declared.policyVersion.valid()
}

// EchoedAsOfPolicy 是 party-commercial 在第二阶段随校验结果交回的那份时点政策。
//
// 它与 DeclaredAsOf 内容同形却另立一型，因为两者的来路不同：声明来自第一阶段的规则包，回显
// 来自权威对本方所形成值的确认。只有后者能证明这个时刻被授权过。同型时 NewJudgmentAsOf 会
// 连声明一起收下，「规则包说锚在这」就能冒充「权威确认过这个时刻」——那正是本类型要挡的。
//
// 它只该由适配器从提供方第二阶段的答复构造，不得由第一阶段的声明转手而来。
type EchoedAsOfPolicy struct {
	kind          JudgmentKind
	semantics     AsOfSemanticsReference
	policyVersion AsOfPolicyVersion
}

func NewEchoedAsOfPolicy(
	kind JudgmentKind,
	semantics AsOfSemanticsReference,
	policyVersion AsOfPolicyVersion,
) (EchoedAsOfPolicy, error) {
	if !kind.valid() || !semantics.valid() || !policyVersion.valid() {
		return EchoedAsOfPolicy{}, ErrInvalidJudgmentAsOf
	}
	return EchoedAsOfPolicy{kind: kind, semantics: semantics, policyVersion: policyVersion}, nil
}

// Matches 报告回显交回的政策与第一阶段的声明是否一致。不一致不是本类型的错误，而是编排要
// 处理的业务事实——规则包在两次调用之间换过了，本轮不能继续。
func (echoed EchoedAsOfPolicy) Matches(declared DeclaredAsOf) bool {
	return echoed.kind == declared.kind &&
		echoed.semantics == declared.semantics &&
		echoed.policyVersion == declared.policyVersion
}

// JudgmentAsOf 是已经形成、并由 party-commercial 校验回显过的时点：本上下文选定的值，连同
// 提供方交回的那份政策。
//
// 它与 DeclaredAsOf 分成两个类型，而不是同一个类型多一个可选的值字段。送进 ReachabilityRequest
// 与 FinancialControlRequest 的必须是回显过的那一份；两者同型时签名分不出来，一次「规则包说
// 锚在这」会被当成「权威确认过这个时刻」用出去，而后者才是权威判断得以复核的依据。
type JudgmentAsOf struct {
	kind          JudgmentKind
	at            time.Time
	semantics     AsOfSemanticsReference
	policyVersion AsOfPolicyVersion
}

// NewJudgmentAsOf 只收回显的政策，收不下第一阶段的声明——这一条由类型而不是注释保证。
func NewJudgmentAsOf(at time.Time, echoed EchoedAsOfPolicy) (JudgmentAsOf, error) {
	if at.IsZero() || !echoed.kind.valid() ||
		!echoed.semantics.valid() || !echoed.policyVersion.valid() {
		return JudgmentAsOf{}, ErrInvalidJudgmentAsOf
	}
	return JudgmentAsOf{
		kind:          echoed.kind,
		at:            at.UTC(),
		semantics:     echoed.semantics,
		policyVersion: echoed.policyVersion,
	}, nil
}

func (asOf JudgmentAsOf) Kind() JudgmentKind {
	return asOf.kind
}

func (asOf JudgmentAsOf) At() time.Time {
	return asOf.at
}

func (asOf JudgmentAsOf) Semantics() AsOfSemanticsReference {
	return asOf.semantics
}

func (asOf JudgmentAsOf) PolicyVersion() AsOfPolicyVersion {
	return asOf.policyVersion
}

func (asOf JudgmentAsOf) valid() bool {
	return !asOf.at.IsZero() && asOf.kind.valid() &&
		asOf.semantics.valid() && asOf.policyVersion.valid()
}

// SettlementPolicyEcho 指名解析采用的那份结算政策版本。
type SettlementPolicyEcho struct{ requiredValue }

func NewSettlementPolicyEcho(value string) (SettlementPolicyEcho, error) {
	required, err := newRequiredValue("settlement policy echo", value)
	return SettlementPolicyEcho{required}, err
}

// SettlementMethodEcho 回显解析出的预付/账期方式。本上下文不按它分支（那是 SA 的事），
// 留引用不建枚举。
type SettlementMethodEcho struct{ requiredValue }

func NewSettlementMethodEcho(value string) (SettlementMethodEcho, error) {
	required, err := newRequiredValue("settlement method echo", value)
	return SettlementMethodEcho{required}, err
}

// SettlementLegalEntityEcho / SettlementCounterpartyEcho / SettlementCurrencyEcho 是采用
// 政策适用范围里推导结算作用域所需的三维。各自成类型：三个都是字符串引用，合用一个类型
// 会让法人与相对方在调用点悄悄换位。
type SettlementLegalEntityEcho struct{ requiredValue }

func NewSettlementLegalEntityEcho(value string) (SettlementLegalEntityEcho, error) {
	required, err := newRequiredValue("settlement legal entity echo", value)
	return SettlementLegalEntityEcho{required}, err
}

type SettlementCounterpartyEcho struct{ requiredValue }

func NewSettlementCounterpartyEcho(value string) (SettlementCounterpartyEcho, error) {
	required, err := newRequiredValue("settlement counterparty echo", value)
	return SettlementCounterpartyEcho{required}, err
}

type SettlementCurrencyEcho struct{ requiredValue }

func NewSettlementCurrencyEcho(value string) (SettlementCurrencyEcho, error) {
	required, err := newRequiredValue("settlement currency echo", value)
	return SettlementCurrencyEcho{required}, err
}

// AdoptedSettlementTermsSpec 是回显一份采用结算政策所需的全部输入（≥5 入参用 Spec，
// 分界见 CommercialBasisSnapshotSpec）。
type AdoptedSettlementTermsSpec struct {
	Policy       SettlementPolicyEcho
	Method       SettlementMethodEcho
	LegalEntity  SettlementLegalEntityEcho
	Counterparty SettlementCounterpartyEcho
	Currency     SettlementCurrencyEcho
}

// AdoptedSettlementTerms 是解析采用的结算政策在本上下文的回显：方式与推导结算作用域
// 所需的三维（ADR-0044 让 PC 交得出，ADR-0047 的作用域缝在消费方用它换取结算账户）。
// 它是引用回显不是政策内容——政策版本生命周期仍属 party-commercial。
type AdoptedSettlementTerms struct {
	policy       SettlementPolicyEcho
	method       SettlementMethodEcho
	legalEntity  SettlementLegalEntityEcho
	counterparty SettlementCounterpartyEcho
	currency     SettlementCurrencyEcho
}

// NewAdoptedSettlementTerms 五件全要：半截的回显推不出作用域，还会让「没带政策」与
// 「带了但缺维」在读取处混成一格。
func NewAdoptedSettlementTerms(spec AdoptedSettlementTermsSpec) (AdoptedSettlementTerms, error) {
	if !spec.Policy.valid() || !spec.Method.valid() ||
		!spec.LegalEntity.valid() || !spec.Counterparty.valid() || !spec.Currency.valid() {
		return AdoptedSettlementTerms{}, ErrInvalidCommercialBasisSnapshot
	}
	return AdoptedSettlementTerms{
		policy:       spec.Policy,
		method:       spec.Method,
		legalEntity:  spec.LegalEntity,
		counterparty: spec.Counterparty,
		currency:     spec.Currency,
	}, nil
}

func (terms AdoptedSettlementTerms) Policy() SettlementPolicyEcho {
	return terms.policy
}

func (terms AdoptedSettlementTerms) Method() SettlementMethodEcho {
	return terms.method
}

func (terms AdoptedSettlementTerms) LegalEntity() SettlementLegalEntityEcho {
	return terms.legalEntity
}

func (terms AdoptedSettlementTerms) Counterparty() SettlementCounterpartyEcho {
	return terms.counterparty
}

func (terms AdoptedSettlementTerms) Currency() SettlementCurrencyEcho {
	return terms.currency
}

func (terms AdoptedSettlementTerms) present() bool {
	return terms.policy.valid()
}

// CommercialBasisSnapshot 是 parcel-shipment 对一次唯一商业解析所保留的部分：解析
// 标识、采用的接单规则包、解析当时的权威视图修订，以及该规则包声明的各项时点。它不
// 持有任何商业版本内容，那些内容属 party-commercial。
type CommercialBasisSnapshot struct {
	resolutionID    CommercialResolutionID
	rulePackage     RulePackageReference
	viewRevision    CommercialViewRevision
	declaredAsOf    []DeclaredAsOf
	applicable      ApplicableCheckGroups
	manualReview    ManualReviewPolicy
	pendingRouting  PendingRoutingAllowance
	settlementTerms AdoptedSettlementTerms
}

// CommercialBasisSnapshotSpec 是形成一次快照所需的全部输入。
//
// 本包的分界是**入参数量**，不是值对象与实体之分，而且它只单向成立：≥5 个输入一律用 Spec
// 结构体（`SubmitShipmentRequestSpec` 5、`ReachabilityJudgmentSpec` 5、
// `SafeHandoffAssessmentSpec` 10、`ProductionOwnershipDecisionSpec` 14），零反例——凡多参
// 位置构造器入参最多 4 个，不靠「一共几个」那种会过期且变红不了的计数（MCP-2 `C2`，实测于
// `337e02b` 时原句写「七个」却至少漏了同文件的 `NewDeclaredAsOf` / `NewEchoedAsOfPolicy` /
// `NewJudgmentAsOf` 与 `NewOwnershipValidityInterval`）。举例：`NewSourceIdentity`、
// `NewSubmissionCandidate`、`NewAcceptanceCheck`、`NewControlItemResult`、
// `NewSourceSubmissionFingerprint` 皆为 4，`NewSubmissionBatchCandidate` 与前三个 AsOf 类为
// 3，`NewAdmissionScope` / `NewJudgmentAsOf` / `NewOwnershipValidityInterval` 为 2。
// `ReachabilityJudgmentSpec` 正是越线后改过来的：`不适用`要携带依据，入参从 4 涨到 5。
//
// 反向不成立，别照着推：≤4 时两种写法都行。`AcceptanceDecisionSpec`、
// `ManualReviewCompletionSpec`、`ProcessingAttemptSpec` 都只有 4 个字段却用 Spec，因为三者
// 都还要长——`BD-PS-002`、`BD-PS-001` 的参数落下来就会加字段。本构造器正是从 4 涨到 7 的，
// 没有人在 7 这个数上作过选择；用 Spec 的好处正是这种增长不必回头改每个调用点。
//
// 「值对象用位置参数」不构成反对理由：`partycommercial` 的 `CommercialVersion` 自称值类型，
// 用的同样是 Spec。跨包也不通用，`parcelpricing` 的 `NewPricingPlanVersion` 有十一个位置参数；
// 这条线只在本包内成立，别据它去改那边。
//
// 翻转条件：本包出现一个 ≥5 入参、长期保持位置参数且调用点仍读得清楚的构造器。届时该重议的
// 是这条线本身，不是本文件。
type CommercialBasisSnapshotSpec struct {
	ResolutionID   CommercialResolutionID
	RulePackage    RulePackageReference
	ViewRevision   CommercialViewRevision
	DeclaredAsOf   []DeclaredAsOf
	Applicable     ApplicableCheckGroups
	ManualReview   ManualReviewPolicy
	PendingRouting PendingRoutingAllowance
	// SettlementTerms 缺席即解析未采用结算政策（闭包不要求结算依据的场景）；非零值只能
	// 来自 NewAdoptedSettlementTerms，五维齐备由它保证。
	SettlementTerms AdoptedSettlementTerms
}

func NewCommercialBasisSnapshot(spec CommercialBasisSnapshotSpec) (CommercialBasisSnapshot, error) {
	if !spec.ResolutionID.valid() || !spec.RulePackage.valid() || !spec.ViewRevision.valid() {
		return CommercialBasisSnapshot{}, ErrInvalidCommercialBasisSnapshot
	}
	seen := make(map[JudgmentKind]struct{}, len(spec.DeclaredAsOf))
	for _, declared := range spec.DeclaredAsOf {
		if !declared.valid() {
			return CommercialBasisSnapshot{}, ErrInvalidDeclaredAsOf
		}
		if _, exists := seen[declared.kind]; exists {
			return CommercialBasisSnapshot{}, ErrInvalidDeclaredAsOf
		}
		seen[declared.kind] = struct{}{}
	}
	return CommercialBasisSnapshot{
		resolutionID:    spec.ResolutionID,
		rulePackage:     spec.RulePackage,
		viewRevision:    spec.ViewRevision,
		declaredAsOf:    append([]DeclaredAsOf(nil), spec.DeclaredAsOf...),
		applicable:      spec.Applicable,
		manualReview:    spec.ManualReview,
		pendingRouting:  spec.PendingRouting,
		settlementTerms: spec.SettlementTerms,
	}, nil
}

// SettlementTerms 交回解析采用的结算政策回显。缺席是真话：这次解析不含结算依据，作用域
// 推导据此停下而不是拿别的引用凑。
func (snapshot CommercialBasisSnapshot) SettlementTerms() (AdoptedSettlementTerms, bool) {
	return snapshot.settlementTerms, snapshot.settlementTerms.present()
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

func (snapshot CommercialBasisSnapshot) Applicable() ApplicableCheckGroups {
	return snapshot.applicable
}

func (snapshot CommercialBasisSnapshot) DeclaredAsOf() []DeclaredAsOf {
	return append([]DeclaredAsOf(nil), snapshot.declaredAsOf...)
}

// DeclaredAsOfFor 返回规则包为某一类判断声明的时点锚。没有声明时报告缺席而不是给默认值——
// 在这里顶上任何一个语义或时刻，正是用例禁止的「用一个全局时间代替」。
//
// 它交回的是声明而不是可直接送出的时点：值还没形成，形成之后还要由 party-commercial 校验
// 回显，那是第二阶段的事。
func (snapshot CommercialBasisSnapshot) DeclaredAsOfFor(kind JudgmentKind) (DeclaredAsOf, bool) {
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

// sameAs 是商业依据快照的完整值等价。Decide 把同一份 Basis 赋给决定与预计承诺，因此重建
// 不能只比解析标识与视图修订——规则包、时点声明、适用组、复核策略、待路由许可与结算回显
// 分叉后仍是两份不同的采用事实。切片按构造顺序逐项比：本路径原样赋同一值，顺序也是身份。
func (snapshot CommercialBasisSnapshot) sameAs(other CommercialBasisSnapshot) bool {
	if snapshot.resolutionID != other.resolutionID ||
		snapshot.rulePackage != other.rulePackage ||
		snapshot.viewRevision != other.viewRevision ||
		snapshot.manualReview != other.manualReview ||
		snapshot.pendingRouting != other.pendingRouting ||
		snapshot.settlementTerms != other.settlementTerms {
		return false
	}
	if len(snapshot.declaredAsOf) != len(other.declaredAsOf) {
		return false
	}
	for index := range snapshot.declaredAsOf {
		if snapshot.declaredAsOf[index] != other.declaredAsOf[index] {
			return false
		}
	}
	if len(snapshot.applicable.groups) != len(other.applicable.groups) {
		return false
	}
	for index := range snapshot.applicable.groups {
		if snapshot.applicable.groups[index] != other.applicable.groups[index] {
			return false
		}
	}
	return true
}

// ReachabilityBasisReference 指名「本服务不要求运营企业形成可达性判断」所依据的商业事实。
// 依据属 party-commercial，由 network-routing 判定后随`不适用`交回，这里只记引用。
type ReachabilityBasisReference struct{ requiredValue }

func NewReachabilityBasisReference(value string) (ReachabilityBasisReference, error) {
	required, err := newRequiredValue("reachability basis reference", value)
	return ReachabilityBasisReference{required}, err
}

// ReachabilityValue 以采用引用的形式镜像 network-routing 对一次可达性请求的答复：三值判断
// 本身，加上「这个问题不该问」。parcel-shipment 从不产生它，只记录拥有它的上下文答了什么；
// 四个取值没有一个是接受决定。
//
// `不适用`与三值并列而不另设类型，与 FinancialControlOutcome 同一形状：提供方在这一支下
// 同样不形成判断，而消费方要的是「这一组校验该怎么落」，那是同一个答复维度。它刻意不与
// `可达`合并——合并之后一次本就不必问的判断会被记成一次问过且通得过的判断，而 UC-NR-002
// 明禁以`不适用`代替三值判断中的任何一个。
type ReachabilityValue uint8

const (
	ReachabilityValueInvalid ReachabilityValue = iota
	ReachabilityReachable
	ReachabilityUnreachable
	ReachabilityInsufficientEvidence
	ReachabilityNotApplicable
)

func (value ReachabilityValue) valid() bool {
	return value >= ReachabilityReachable && value <= ReachabilityNotApplicable
}

func (value ReachabilityValue) String() string {
	switch value {
	case ReachabilityReachable:
		return "REACHABLE"
	case ReachabilityUnreachable:
		return "UNREACHABLE"
	case ReachabilityInsufficientEvidence:
		return "INSUFFICIENT_EVIDENCE"
	case ReachabilityNotApplicable:
		return "NOT_APPLICABLE"
	default:
		return ""
	}
}

// ReachabilityJudgment 是本上下文对一次可达性答复所保留的引用。
//
// `不适用`必须携带依据，且不要求判断标识：那一支下 network-routing 不形成判断，也就没有
// 签发标识可引用，强行要一个只能由适配器发明；而没有依据的`不适用`与一次悄悄放行分不开。
// 其余三值反过来必须带标识——它们都是权威已经形成的判断。
type ReachabilityJudgment struct {
	judgmentID ReachabilityJudgmentID
	parcelID   DeclaredParcelID
	value      ReachabilityValue
	basis      ReachabilityBasisReference
	asOf       JudgmentAsOf
}

// ReachabilityJudgmentSpec 是形成一次可达性判断引用所需的全部输入。入参从 4 涨到 5，按本包
// ≥5 用 Spec 的分界改成结构体。
type ReachabilityJudgmentSpec struct {
	JudgmentID ReachabilityJudgmentID
	ParcelID   DeclaredParcelID
	Value      ReachabilityValue
	Basis      ReachabilityBasisReference
	AsOf       JudgmentAsOf
}

func NewReachabilityJudgment(spec ReachabilityJudgmentSpec) (ReachabilityJudgment, error) {
	if !spec.ParcelID.valid() || !spec.Value.valid() || !spec.AsOf.valid() {
		return ReachabilityJudgment{}, ErrInvalidReachabilityJudgment
	}
	if spec.Value == ReachabilityNotApplicable && !spec.Basis.valid() {
		return ReachabilityJudgment{}, ErrInvalidReachabilityJudgment
	}
	if spec.Value != ReachabilityNotApplicable && !spec.JudgmentID.valid() {
		return ReachabilityJudgment{}, ErrInvalidReachabilityJudgment
	}
	return ReachabilityJudgment{
		judgmentID: spec.JudgmentID,
		parcelID:   spec.ParcelID,
		value:      spec.Value,
		basis:      spec.Basis,
		asOf:       spec.AsOf,
	}, nil
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

// Basis 只在`不适用`时给出：它说的是这个问题为什么不该问，其余三值都是问过之后的答案。
func (judgment ReachabilityJudgment) Basis() ReachabilityBasisReference {
	return judgment.basis
}

func (judgment ReachabilityJudgment) valid() bool {
	if !judgment.parcelID.valid() || !judgment.value.valid() {
		return false
	}
	if judgment.value == ReachabilityNotApplicable {
		return judgment.basis.valid()
	}
	return judgment.judgmentID.valid()
}

// 接受前财务控制采用结果（FinancialControlResult 及其逐项）住在 financial_control_result.go：
// 它自 ADR-0125 起长成逐项结果 + 共同通过条件 + 推导出的结论，体量与本文件其余引用类型不同。
