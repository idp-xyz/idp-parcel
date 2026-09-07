package domain

import (
	"errors"
	"sort"
	"time"
)

var (
	ErrInvalidControlPolicy = errors.New("settlement accounting: invalid pre-acceptance control policy")
	ErrInvalidControlAsOf   = errors.New("settlement accounting: invalid control asOf")
)

// TenantID 是运营集团租户。按 ADR-0003 它是最高数据隔离边界，因此余额、冻结与控制结果
// 的读取都必须在签名上带着它，而不是从 context 里补。
type TenantID struct{ requiredValue }

func NewTenantID(value string) (TenantID, error) {
	required, err := newRequiredValue("tenant ID", value)
	return TenantID{required}, err
}

// ControlBasisReference 指名「本范围接受前无财务控制」所依据的商业事实。CONTEXT 要求
// 这条不适用依据由 `party-commercial` 提供，本上下文只引用不自造。
type ControlBasisReference struct{ requiredValue }

func NewControlBasisReference(value string) (ControlBasisReference, error) {
	required, err := newRequiredValue("control basis reference", value)
	return ControlBasisReference{required}, err
}

// CommercialResolutionReference 回指一次已固定的商业解析。它是本上下文向商业侧提问
// 「这个范围要不要接受前财务控制」时唯一带得动的键。
//
// 为什么不是合同版本：结算作用域（法人/账户/币种）与商业侧的声明键（客户合同版本）不同维，
// 由本上下文自行从作用域反查合同等于在这里发明第二处账户映射定义，而 CONTEXT 明禁本上下文
// 自行推导商业结论。为什么不是整份闭包：调用方带着闭包回来，键就可以被替换（ADR-0027 /
// ADR-0062 「消费方只回指标识」）。
//
// 它是调用方回显来的，不是本上下文铸造的——施加路径的作用域本就派生自同一次解析，两者
// 因此必然同源。
type CommercialResolutionReference struct{ requiredValue }

func NewCommercialResolutionReference(value string) (CommercialResolutionReference, error) {
	required, err := newRequiredValue("commercial resolution reference", value)
	return CommercialResolutionReference{required}, err
}

type AsOfSemantic struct{ requiredValue }

func NewAsOfSemantic(value string) (AsOfSemantic, error) {
	required, err := newRequiredValue("asOf semantic", value)
	return AsOfSemantic{required}, err
}

type AsOfStrategyVersion struct{ requiredValue }

func NewAsOfStrategyVersion(value string) (AsOfStrategyVersion, error) {
	required, err := newRequiredValue("asOf strategy version", value)
	return AsOfStrategyVersion{required}, err
}

// ControlAsOf 是选出接受前财务控制策略所依据的业务时点。CONTEXT 要求每项控制结果保存
// 实际采用的 `asOf` 语义和值，所以语义、取值与策略版本三项一同携带——只留一个时刻证明
// 不了它来自哪条策略。它与冻结发生时间是两回事：`asOf` 决定按哪一版策略判断，冻结时间
// 说明资金何时被占用。
type ControlAsOf struct {
	semantic        AsOfSemantic
	at              time.Time
	strategyVersion AsOfStrategyVersion
}

func NewControlAsOf(semantic AsOfSemantic, at time.Time, strategyVersion AsOfStrategyVersion) (ControlAsOf, error) {
	if !semantic.valid() || at.IsZero() || !strategyVersion.valid() {
		return ControlAsOf{}, ErrInvalidControlAsOf
	}
	return ControlAsOf{semantic: semantic, at: at.UTC(), strategyVersion: strategyVersion}, nil
}

func (asOf ControlAsOf) Semantic() AsOfSemantic {
	return asOf.semantic
}

func (asOf ControlAsOf) At() time.Time {
	return asOf.at
}

func (asOf ControlAsOf) StrategyVersion() AsOfStrategyVersion {
	return asOf.strategyVersion
}

func (asOf ControlAsOf) Valid() bool {
	return asOf.semantic.valid() && !asOf.at.IsZero() && asOf.strategyVersion.valid()
}

// ControlRequirement 回答合同对本范围要不要执行接受前财务控制。两个取值都不是控制结果：
// `不要求`说的是这项控制不该做，而`业务限制`说的是做了但余额不够。
type ControlRequirement uint8

const (
	ControlRequirementInvalid ControlRequirement = iota
	ControlRequired
	ControlNotRequired
)

func (requirement ControlRequirement) valid() bool {
	return requirement >= ControlRequired && requirement <= ControlNotRequired
}

func (requirement ControlRequirement) String() string {
	switch requirement {
	case ControlRequired:
		return "REQUIRED"
	case ControlNotRequired:
		return "NOT_REQUIRED"
	default:
		return ""
	}
}

// SettlementMethod 是预付/账期的封闭二值（SA 自有词汇，与 PC 的解析结果对应但不 import）。
// 刻意没有第三格「客户默认」：未解析出方式的范围没有可执行的控制分支，那是`待判断`不是
// 某种通行做法（ADR-0047，呼应 ADR-0044 的方式是解析输出）。
type SettlementMethod uint8

const (
	SettlementMethodInvalid SettlementMethod = iota
	PrepaidSettlement
	TermsSettlement
)

func (method SettlementMethod) valid() bool {
	return method == PrepaidSettlement || method == TermsSettlement
}

func (method SettlementMethod) String() string {
	switch method {
	case PrepaidSettlement:
		return "PREPAID"
	case TermsSettlement:
		return "TERMS"
	default:
		return ""
	}
}

// AdoptedPolicyReference 指名本次控制实际采用的结算政策。CONTEXT 硬句：每项冻结与信用
// 暴露必须保存实际采用的结算政策、预付/账期方式及其适用范围——没有它，事后无从回答
// 「这笔控制凭什么走的这条路」。
type AdoptedPolicyReference struct{ requiredValue }

func NewAdoptedPolicyReference(value string) (AdoptedPolicyReference, error) {
	required, err := newRequiredValue("adopted policy reference", value)
	return AdoptedPolicyReference{required}, err
}

// ControlPolicyReference 指名本次控制实际采用的接受前财务控制策略版本——控制项从它的正文
// 里来（ADR-0115 / ADR-0122）。它与 AdoptedPolicyReference 是两份不同的采用依据：结算政策
// 说这个范围按预付还是账期结算，控制策略说接受前要执行哪些控制；两者都要保存，哪一份都
// 替不了另一份。
type ControlPolicyReference struct{ requiredValue }

func NewControlPolicyReference(value string) (ControlPolicyReference, error) {
	required, err := newRequiredValue("control policy reference", value)
	return ControlPolicyReference{required}, err
}

// ControlKind 是策略正文里一项控制的种类，本上下文自有词汇（与 party-commercial 的正文词汇
// 同名但不 import，纪律同 SettlementMethod）。封闭集里刻意没有「明确无控制」：那由合同声明
// 并带不适用依据（ADR-0115 决定一），进了这里就是给「默认通过」开一条路。
type ControlKind uint8

const (
	ControlKindInvalid ControlKind = iota
	// PrepaidFreezeControl 在运营结算余额上占用明确金额——走冻结账本。
	PrepaidFreezeControl
	// CreditCheckControl 在信用暴露账本上占用额度——走暴露账本。
	CreditCheckControl
)

func (kind ControlKind) valid() bool {
	return kind >= PrepaidFreezeControl && kind <= CreditCheckControl
}

func (kind ControlKind) String() string {
	switch kind {
	case PrepaidFreezeControl:
		return "PREPAID_FREEZE"
	case CreditCheckControl:
		return "CREDIT_CHECK"
	default:
		return ""
	}
}

// JointPassCondition 是一版策略的共同通过条件，封闭集，首发一值。它随答复带回给执行方与
// 调用方读，本上下文**不据它汇总**：CONTEXT 写死「由 `parcel-shipment` 按策略的共同通过条件
// 形成接受判断」，本上下文只让每项结果各自成立。集外取值报错不吸收（ADR-0025）。
type JointPassCondition uint8

const (
	JointPassConditionInvalid JointPassCondition = iota
	AllControlsPass
)

func (condition JointPassCondition) valid() bool {
	return condition == AllControlsPass
}

func (condition JointPassCondition) String() string {
	switch condition {
	case AllControlsPass:
		return "ALL_CONTROLS_PASS"
	default:
		return ""
	}
}

// ControlItem 是策略正文里要执行的一项控制：种类 × 判断顺序。失败处置与责任不在其中——
// 它们答的是委托去向，归 parcel-shipment 经自己的商业缝读，本上下文执行控制不需要它们
// （ADR-0122 决定四）。
type ControlItem struct {
	kind  ControlKind
	order uint32
}

// NewControlItem 造一项控制。顺序必须是正整数：零值顺序在 SQL 与领域里都不是一个位置。
func NewControlItem(kind ControlKind, order uint32) (ControlItem, error) {
	if !kind.valid() || order == 0 {
		return ControlItem{}, ErrInvalidControlPolicy
	}
	return ControlItem{kind: kind, order: order}, nil
}

func (item ControlItem) Kind() ControlKind {
	return item.kind
}

func (item ControlItem) Order() uint32 {
	return item.order
}

// PreAcceptanceControlPolicy 是商业侧对「这个范围要不要接受前财务控制、要执行哪些控制项」
// 的回答。
//
// `不要求`必须携带依据。CONTEXT 明写不得用一次虚假零金额冻结或默认信用通过冒充无控制，
// 而没有依据的`不要求`正是「默认信用通过」的做法——它让一次未执行的控制看起来像通过了。
// 零金额那条路已经被 `NewFreezeRequest` 在构造期堵死，这里堵的是另一条。
//
// `要求`必须携带控制项集合、共同通过条件与采用的控制策略版本（ADR-0122）：控制项决定
// 走哪几条控制路、按什么顺序；此外仍带结算方式与采用的结算政策引用，因为 CONTEXT 要求每项
// 冻结与信用暴露保存「实际采用的结算政策、预付/账期方式」——方式从此只是结果上要保存的
// 一格，不再是选路的开关（ADR-0047 那条派生正是 pn-02-w03 禁的）。两种形状经各自的构造函数
// 进来，混搭（要求带不适用依据、不要求带控制项）没有入口。
type PreAcceptanceControlPolicy struct {
	requirement   ControlRequirement
	basis         ControlBasisReference
	items         []ControlItem
	jointPass     JointPassCondition
	controlPolicy ControlPolicyReference
	method        SettlementMethod
	adoptedPolicy AdoptedPolicyReference
}

// NewNoControlPolicy 造「本范围接受前无财务控制」的回答，商业不适用依据必备。
func NewNoControlPolicy(basis ControlBasisReference) (PreAcceptanceControlPolicy, error) {
	if !basis.valid() {
		return PreAcceptanceControlPolicy{}, ErrInvalidControlPolicy
	}
	return PreAcceptanceControlPolicy{requirement: ControlNotRequired, basis: basis}, nil
}

// NewRequiredControlPolicy 造「本范围要求接受前财务控制」的回答。至少一项控制；同一种控制
// 不得两行、两行不得抢同一个判断顺序（与 party-commercial 正文的行级约束同形，ADR-0115
// 决定二）——违反任一条都答不出「该按哪条执行」。交回的控制项按判断顺序排好，执行方照序走。
func NewRequiredControlPolicy(
	items []ControlItem,
	jointPass JointPassCondition,
	controlPolicy ControlPolicyReference,
	method SettlementMethod,
	adoptedPolicy AdoptedPolicyReference,
) (PreAcceptanceControlPolicy, error) {
	if len(items) == 0 || !jointPass.valid() || !controlPolicy.valid() ||
		!method.valid() || !adoptedPolicy.valid() {
		return PreAcceptanceControlPolicy{}, ErrInvalidControlPolicy
	}
	kinds := make(map[ControlKind]struct{}, len(items))
	orders := make(map[uint32]struct{}, len(items))
	ordered := make([]ControlItem, 0, len(items))
	for _, item := range items {
		if !item.kind.valid() || item.order == 0 {
			return PreAcceptanceControlPolicy{}, ErrInvalidControlPolicy
		}
		if _, seen := kinds[item.kind]; seen {
			return PreAcceptanceControlPolicy{}, ErrInvalidControlPolicy
		}
		if _, seen := orders[item.order]; seen {
			return PreAcceptanceControlPolicy{}, ErrInvalidControlPolicy
		}
		kinds[item.kind] = struct{}{}
		orders[item.order] = struct{}{}
		ordered = append(ordered, item)
	}
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].order < ordered[right].order })
	return PreAcceptanceControlPolicy{
		requirement:   ControlRequired,
		items:         ordered,
		jointPass:     jointPass,
		controlPolicy: controlPolicy,
		method:        method,
		adoptedPolicy: adoptedPolicy,
	}, nil
}

func (policy PreAcceptanceControlPolicy) Requirement() ControlRequirement {
	return policy.requirement
}

func (policy PreAcceptanceControlPolicy) Basis() ControlBasisReference {
	return policy.basis
}

// Items 按判断顺序交回要执行的控制项。交回副本：调用方改它改不到策略。
func (policy PreAcceptanceControlPolicy) Items() []ControlItem {
	return append([]ControlItem(nil), policy.items...)
}

func (policy PreAcceptanceControlPolicy) JointPassCondition() JointPassCondition {
	return policy.jointPass
}

func (policy PreAcceptanceControlPolicy) ControlPolicy() ControlPolicyReference {
	return policy.controlPolicy
}

func (policy PreAcceptanceControlPolicy) Method() SettlementMethod {
	return policy.method
}

func (policy PreAcceptanceControlPolicy) AdoptedPolicy() AdoptedPolicyReference {
	return policy.adoptedPolicy
}

// ControlRequired 报告是否应当继续执行控制。零值答否且不带依据，因此拿它去构造一个
// 「无控制」结果会缺依据——这是拦住「端口没答话被当成不要求控制」的办法。
func (policy PreAcceptanceControlPolicy) ControlRequired() bool {
	return policy.requirement == ControlRequired
}
