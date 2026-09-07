package domain

import (
	"errors"
	"sort"
)

var (
	// ErrInvalidPreAcceptanceFinancialControlPolicy 拒绝立不住的接受前财务控制策略正文。
	ErrInvalidPreAcceptanceFinancialControlPolicy = errors.New("party commercial: invalid pre-acceptance financial control policy")
	// ErrInvalidPreAcceptanceControlItem 拒绝立不住的控制项行。
	ErrInvalidPreAcceptanceControlItem = errors.New("party commercial: invalid pre-acceptance control item")
	// ErrDuplicatePreAcceptanceControlItem 拒绝同一键下的第二行：同一范围上同一种控制两行、或两行
	// 抢同一个判断顺序。与「立不住」分格，恢复动作不同——前者去掉多出的那一行，后者去补内容。
	ErrDuplicatePreAcceptanceControlItem = errors.New("party commercial: duplicate pre-acceptance control item")
)

// PreAcceptanceControlKind 是策略正文里一项控制的种类，封闭集（ADR-0115 Decision 一）。
//
// 集合里刻意没有「明确无控制」那一格。CONTEXT 点名的三种里，「无控制」由客户合同版本按范围声明
// 并保存不适用依据（合同层两处：版本级声明与按费用范围的绑定）；策略正文若有这一格，一个范围就能
// 经合同绑定里的策略引用指名它而不给依据——那正是 CONTEXT 明禁的「用缺失结果或默认通过代替」。
// 它是行的键而不是父行上的枚举列：「不得硬编码为永久互斥的三选一枚举」在这里的落法是一版策略可以
// 有多行，集合以迁移放宽而扩。
type PreAcceptanceControlKind uint8

const (
	PreAcceptanceControlKindInvalid PreAcceptanceControlKind = iota
	PrepaidFreezeControl
	CreditCheckControl
)

func (kind PreAcceptanceControlKind) valid() bool {
	return kind >= PrepaidFreezeControl && kind <= CreditCheckControl
}

func (kind PreAcceptanceControlKind) String() string {
	switch kind {
	case PrepaidFreezeControl:
		return "PREPAID_FREEZE"
	case CreditCheckControl:
		return "CREDIT_CHECK"
	default:
		return ""
	}
}

// ControlFailureDisposition 是一项控制不通过时委托的去向，封闭两值，逐字对应 UC-PS-001
// 「任一必需控制不通过时按策略拒绝或进入授权处置；不得默认放行」。
//
// 它只答去向，不拥有拒绝决定：接受或拒绝决定归 parcel-shipment，控制结果归 settlement-accounting，
// 本上下文只说合同约定了哪一条路。零值不是任何一条路——「没说失败了怎么办」的控制项立不住。
type ControlFailureDisposition uint8

const (
	ControlFailureDispositionInvalid ControlFailureDisposition = iota
	RejectOnControlFailure
	AuthorizedDispositionOnControlFailure
)

func (disposition ControlFailureDisposition) valid() bool {
	return disposition >= RejectOnControlFailure && disposition <= AuthorizedDispositionOnControlFailure
}

func (disposition ControlFailureDisposition) String() string {
	switch disposition {
	case RejectOnControlFailure:
		return "REJECT"
	case AuthorizedDispositionOnControlFailure:
		return "AUTHORIZED_DISPOSITION"
	default:
		return ""
	}
}

// ControlResponsibilityReference 指名一项控制失败或需要补偿时的责任方（CONTEXT「失败或补偿
// 责任」）。是开放引用而不是 PartyID：谁承担属实例半边，合同上写的可能是一方参与方、一条条款
// 或一个角色，本上下文只携带引用。
type ControlResponsibilityReference struct{ requiredValue }

func NewControlResponsibilityReference(value string) (ControlResponsibilityReference, error) {
	required, err := newRequiredValue("control responsibility reference", value)
	return ControlResponsibilityReference{required}, err
}

// JointPassCondition 是一版策略的共同通过条件，封闭集，首发只有「全部控制通过」一值
// （ADR-0115 Decision 三）。
//
// 只有一值仍然成为独立类型、仍然必填，是为了让这个条件是租户**说出来的**而不是产品替它默认的：
// 将来放宽（比如按顺序任一通过）时既有正文一字不动，消费方的穷举分派在新值上响亮失败而不是
// 静默放行。「任一结果通过即可接受」按 pn-02-w03 不得推导，所以今天不发明它。
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

// PreAcceptanceControlItem 是策略正文的一行：在哪个费用范围上做哪一种控制、排第几、不通过
// 时委托去哪、谁负责（ADR-0115 Decision 二）。
//
// 范围用 ChargeScopeReference 而不是另造一种引用：客户合同按费用范围把范围绑到策略上，结算政策
// 也按费用范围切方式，消费方要拿委托的费用范围一路对下来，三处必须说同一种话。
type PreAcceptanceControlItem struct {
	kind           PreAcceptanceControlKind
	scope          ChargeScopeReference
	order          int
	disposition    ControlFailureDisposition
	responsibility ControlResponsibilityReference
}

// NewPreAcceptanceControlItem 只校形状不校值：种类与处置在集内、范围与责任引用非空、顺序为正。
// 顺序从 1 起数：零与负数不是「排第几」的答案，而「没排」在这里不是一个合法状态——CONTEXT 要求
// 组合控制明确判断顺序，单项控制的顺序就是 1。
func NewPreAcceptanceControlItem(
	kind PreAcceptanceControlKind,
	scope ChargeScopeReference,
	order int,
	disposition ControlFailureDisposition,
	responsibility ControlResponsibilityReference,
) (PreAcceptanceControlItem, error) {
	if !kind.valid() || !scope.valid() || order <= 0 || !disposition.valid() || !responsibility.valid() {
		return PreAcceptanceControlItem{}, ErrInvalidPreAcceptanceControlItem
	}
	return PreAcceptanceControlItem{
		kind:           kind,
		scope:          scope,
		order:          order,
		disposition:    disposition,
		responsibility: responsibility,
	}, nil
}

func (item PreAcceptanceControlItem) Kind() PreAcceptanceControlKind {
	return item.kind
}

func (item PreAcceptanceControlItem) Scope() ChargeScopeReference {
	return item.scope
}

// EvaluationOrder 是本项在整份策略里的判断顺序，从 1 起、版本内唯一。
func (item PreAcceptanceControlItem) EvaluationOrder() int {
	return item.order
}

func (item PreAcceptanceControlItem) FailureDisposition() ControlFailureDisposition {
	return item.disposition
}

func (item PreAcceptanceControlItem) Responsibility() ControlResponsibilityReference {
	return item.responsibility
}

func (item PreAcceptanceControlItem) valid() bool {
	return item.kind.valid() && item.scope.valid() && item.order > 0 &&
		item.disposition.valid() && item.responsibility.valid()
}

// PreAcceptanceFinancialControlPolicy 是一个接受前财务控制策略版本的正文：要执行的控制项集合与
// 它们的共同通过条件（ADR-0115）。
//
// **本类型刻意不带**信用政策版本、结算账户、价格依据、金额与阈值：那些各有所有者（本上下文的信用
// 政策册、PAR-SET-01/02、settlement-accounting 形成的结果），写进来就是同一件事第二处定义。也不带
// 合同引用——合同 → 策略这层关系由客户合同正文的绑定拥有。
type PreAcceptanceFinancialControlPolicy struct {
	version   CommercialVersion
	jointPass JointPassCondition
	items     []PreAcceptanceControlItem
}

// NewPreAcceptanceFinancialControlPolicy 在版本已生效且类别正确时形成一份策略正文。
//
// 类别必须是 PreAcceptanceFinancialControlPolicyObject：挂错类别的版本仍是合法的商业版本、入册与被
// 选中都不报错，错要到下游取不到控制项时才显形，而那时它长得像「这个合同没配控制」（判据同
// NewCustomerServiceRuleVersion）。至少一项控制：零项不是「显式无控制」——那一句由合同声明，正文里
// 允许零项会造出「登记了但什么都没说」与「没登记」两种在消费侧同形的空。同一范围上同一种控制至多
// 一行，判断顺序版本内唯一——两行抢一个顺序或同键两行，都答不出该按哪条。
func NewPreAcceptanceFinancialControlPolicy(
	version CommercialVersion,
	jointPass JointPassCondition,
	items []PreAcceptanceControlItem,
) (PreAcceptanceFinancialControlPolicy, error) {
	if version.kind != PreAcceptanceFinancialControlPolicyObject ||
		version.status != CommercialVersionEffective ||
		!jointPass.valid() ||
		len(items) == 0 {
		return PreAcceptanceFinancialControlPolicy{}, ErrInvalidPreAcceptanceFinancialControlPolicy
	}

	type itemKey struct {
		kind  PreAcceptanceControlKind
		scope ChargeScopeReference
	}
	seenKeys := make(map[itemKey]struct{}, len(items))
	seenOrders := make(map[int]struct{}, len(items))
	filed := make([]PreAcceptanceControlItem, 0, len(items))
	for _, item := range items {
		if !item.valid() {
			return PreAcceptanceFinancialControlPolicy{}, ErrInvalidPreAcceptanceControlItem
		}
		key := itemKey{kind: item.kind, scope: item.scope}
		if _, duplicate := seenKeys[key]; duplicate {
			return PreAcceptanceFinancialControlPolicy{}, ErrDuplicatePreAcceptanceControlItem
		}
		if _, duplicate := seenOrders[item.order]; duplicate {
			return PreAcceptanceFinancialControlPolicy{}, ErrDuplicatePreAcceptanceControlItem
		}
		seenKeys[key] = struct{}{}
		seenOrders[item.order] = struct{}{}
		filed = append(filed, item)
	}
	sort.Slice(filed, func(left, right int) bool {
		return filed[left].order < filed[right].order
	})

	return PreAcceptanceFinancialControlPolicy{
		version:   version,
		jointPass: jointPass,
		items:     filed,
	}, nil
}

func (policy PreAcceptanceFinancialControlPolicy) Version() CommercialVersion {
	return policy.version
}

func (policy PreAcceptanceFinancialControlPolicy) JointPassCondition() JointPassCondition {
	return policy.jointPass
}

// Items 按判断顺序交回全部控制项（副本）。
func (policy PreAcceptanceFinancialControlPolicy) Items() []PreAcceptanceControlItem {
	return append([]PreAcceptanceControlItem(nil), policy.items...)
}

// ItemsFor 按判断顺序交回某个费用范围上的控制项（副本）。空切片是「这一版对该范围没有规定控制」
// ——它不是「该范围无控制」：后者是合同层的一句话，要带依据，本正文说不了它。
func (policy PreAcceptanceFinancialControlPolicy) ItemsFor(scope ChargeScopeReference) []PreAcceptanceControlItem {
	items := make([]PreAcceptanceControlItem, 0, len(policy.items))
	for _, item := range policy.items {
		if item.scope == scope {
			items = append(items, item)
		}
	}
	return items
}
