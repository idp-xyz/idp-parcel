package domain

import (
	"errors"
	"sort"
)

var (
	// ErrInvalidControlItemResult 拒绝立不住的控制项结果。与 ErrInvalidFinancialControlResult 分格：
	// 一项坏在自己身上（种类集外、顺序为零、受限无原因）与整份结果坏在组合上（零项、顺序重复、
	// 条件集外）要改的地方不同。
	ErrInvalidControlItemResult = errors.New("parcel shipment: invalid control item result")
	// ErrInvalidRehydratedFinancialControlResult 是重建门因库里那一行自身而拒绝时给出的理由，分格
	// 依据同 ErrInvalidRehydratedShipmentRequest：它说的是「这份已经记下的东西不可能是本上下文判出
	// 来的」，处置是去查那一行或写它的适配器，不是让调用方改请求重来。
	ErrInvalidRehydratedFinancialControlResult = errors.New("parcel shipment: invalid rehydrated financial control result")
)

type FinancialControlResultID struct{ requiredValue }

func NewFinancialControlResultID(value string) (FinancialControlResultID, error) {
	required, err := newRequiredValue("financial control result ID", value)
	return FinancialControlResultID{required}, err
}

// ControlBasisReference 是一次非通过的接受前财务控制所依据的事实：某一项业务限制的原因，或者
// 合同明确无控制的商业不适用依据。用例把后者的保存责任判给本上下文，但依据本身来自
// party-commercial 或 settlement-accounting，这里只记引用。
type ControlBasisReference struct{ requiredValue }

func NewControlBasisReference(value string) (ControlBasisReference, error) {
	required, err := newRequiredValue("control basis reference", value)
	return ControlBasisReference{required}, err
}

// ControlItemKind 镜像策略正文里一项控制的种类：本上下文自有封闭集，与 settlement-accounting /
// party-commercial 的同名词汇不 import（ADR-0025 两侧各有封闭集、适配器全函数翻译）。
//
// 它与 SettlementMethodEcho「留引用不建枚举」相反地建了枚举，因为本上下文要按种类判「成立的项里
// 有没有预付冻结」——那是 HELD 与 CREDIT_EXPOSED 两种拼法的分界，引用字符串判不了。集合里刻意
// 没有「明确无控制」：那不是一项控制，由合同带依据声明，走 NewInapplicableFinancialControlResult。
type ControlItemKind uint8

const (
	ControlItemKindInvalid ControlItemKind = iota
	// PrepaidFreezeControlItem 在运营结算余额上占用明确金额——成立即有一笔冻结在提供方账本上。
	PrepaidFreezeControlItem
	// CreditCheckControlItem 在信用暴露账本上占用额度——成立即有一笔暴露在提供方账本上。
	CreditCheckControlItem
)

func (kind ControlItemKind) valid() bool {
	return kind >= PrepaidFreezeControlItem && kind <= CreditCheckControlItem
}

func (kind ControlItemKind) String() string {
	switch kind {
	case PrepaidFreezeControlItem:
		return "PREPAID_FREEZE"
	case CreditCheckControlItem:
		return "CREDIT_CHECK"
	default:
		return ""
	}
}

// ControlItemConclusion 是一项控制执行后自己的结论：成立，或形成`业务限制`。它不是接受判断——多项
// 合起来算不算通过由共同通过条件判（SA CONTEXT 把这一步判给本上下文，但判的是整份结果，不是一项）。
type ControlItemConclusion uint8

const (
	ControlItemConclusionInvalid ControlItemConclusion = iota
	ControlItemSatisfied
	ControlItemRestricted
)

func (conclusion ControlItemConclusion) valid() bool {
	return conclusion == ControlItemSatisfied || conclusion == ControlItemRestricted
}

func (conclusion ControlItemConclusion) String() string {
	switch conclusion {
	case ControlItemSatisfied:
		return "SATISFIED"
	case ControlItemRestricted:
		return "RESTRICTED"
	default:
		return ""
	}
}

// ControlItemResult 是本上下文对 settlement-accounting 一项已执行控制的采用引用：种类 × 判断顺序 ×
// 结论 × 受限原因。它是执行记录不是判决（CONTEXT 词条「控制项结果」）。
//
// 受限必带该项自己的原因（没有原因的`业务限制`说不出限制什么），成立不带——成立的依据在提供方的
// 占用记录本身，这里再带一份就是第二处定义，与 checkWithReason「通过的校验不带原因」同一条纪律。
type ControlItemResult struct {
	kind       ControlItemKind
	order      uint32
	conclusion ControlItemConclusion
	basis      ControlBasisReference
}

// NewControlItemResult 只校形状：种类与结论在集内、顺序从 1 起（零在 SQL 与领域里都不是一个位置，
// 与 PC 正文 NewPreAcceptanceControlItem 同一判据）、依据与结论互补在场。
func NewControlItemResult(
	kind ControlItemKind,
	order uint32,
	conclusion ControlItemConclusion,
	basis ControlBasisReference,
) (ControlItemResult, error) {
	if !kind.valid() || order == 0 || !conclusion.valid() {
		return ControlItemResult{}, ErrInvalidControlItemResult
	}
	if (conclusion == ControlItemRestricted) != basis.valid() {
		return ControlItemResult{}, ErrInvalidControlItemResult
	}
	return ControlItemResult{kind: kind, order: order, conclusion: conclusion, basis: basis}, nil
}

func (item ControlItemResult) Kind() ControlItemKind {
	return item.kind
}

func (item ControlItemResult) Order() uint32 {
	return item.order
}

func (item ControlItemResult) Conclusion() ControlItemConclusion {
	return item.conclusion
}

// Basis 只在`业务限制`时给出：它说的是这一项为什么没成立。
func (item ControlItemResult) Basis() ControlBasisReference {
	return item.basis
}

// Satisfied 报告这一项成立——也就是提供方账本上为它形成了一笔占用。
func (item ControlItemResult) Satisfied() bool {
	return item.conclusion == ControlItemSatisfied
}

func (item ControlItemResult) valid() bool {
	return item.kind.valid() && item.order > 0 && item.conclusion.valid() &&
		(item.conclusion == ControlItemRestricted) == item.basis.valid()
}

// JointPassCondition 镜像策略声明的共同通过条件：本上下文自有封闭集，首发一值。它随结果保存，
// 回答「按哪个条件判出的结论」；集外取值在构造期报错不吸收——一个本上下文还不会算的组合子若
// 静默按「全部通过」折，就是替租户决定了怎么合并控制结果（ADR-0115 决定三点名的红线）。
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

// FinancialControlOutcome 是接受侧结论：本上下文按共同通过条件从逐项结果推出的那一格。取值没有
// 一个是接受决定，也刻意没有「视同通过」——用例对本步的要求是不得默认放行，而一个表示「没控制成
// 但先过」的取值正是默认放行的载体。控制没能形成时，编排保持判断任务未决，不在这里凑一个结果。
//
// `已冻结`与`信用暴露已记录`是「成立」的两种拼法（ADR-0047 第四格、ADR-0125 决定二），只区分成立
// 的项里有没有预付冻结；它们是逐项结果的投影，不是第二个来源——「执行了什么、有没有暴露」去看
// Items，结论只答按条件成没成立。
type FinancialControlOutcome uint8

const (
	FinancialControlOutcomeInvalid FinancialControlOutcome = iota
	FinancialControlHeld
	FinancialControlRestricted
	FinancialControlNotApplicable
	FinancialControlCreditExposed
)

func (outcome FinancialControlOutcome) valid() bool {
	return outcome >= FinancialControlHeld && outcome <= FinancialControlCreditExposed
}

func (outcome FinancialControlOutcome) String() string {
	switch outcome {
	case FinancialControlHeld:
		return "HELD"
	case FinancialControlRestricted:
		return "RESTRICTED"
	case FinancialControlNotApplicable:
		return "NOT_APPLICABLE"
	case FinancialControlCreditExposed:
		return "CREDIT_EXPOSED"
	default:
		return ""
	}
}

// FinancialControlResult 是接受前财务控制采用结果（CONTEXT 词条）：settlement-accounting 交回的逐项
// 控制项结果、策略声明的共同通过条件，以及本上下文按该条件推出的接受侧结论。
//
// 结论没有公开的直接入口：已执行的结果只能经 NewExecutedFinancialControlResult 由逐项推出，明确无
// 控制经 NewInapplicableFinancialControlResult，库里的行经 RehydrateFinancialControlResult 校验后收下。
// 留一个「结论由调用方交入」的构造器，就留着结论与逐项不一致的入口（ADR-0125 决定一）。
type FinancialControlResult struct {
	resultID  FinancialControlResultID
	outcome   FinancialControlOutcome
	basis     ControlBasisReference
	asOf      JudgmentAsOf
	items     []ControlItemResult
	jointPass JointPassCondition
}

// ExecutedFinancialControlSpec 是一次已执行控制在本上下文的全部输入：结论不在其中，由构造期推出。
type ExecutedFinancialControlSpec struct {
	// ResultID 取控制请求身份（提供方账本的内部编号不出上下文，`业务限制`根本没有编号），释放
	// 按它认领——ADR-0027「消费方凭据不持有提供方不曾签发的东西」。
	ResultID  FinancialControlResultID
	Items     []ControlItemResult
	JointPass JointPassCondition
	AsOf      JudgmentAsOf
}

// NewExecutedFinancialControlResult 从逐项结果按共同通过条件推出接受侧结论。
//
// 形状与 PC 正文的行级约束同形（ADR-0115 决定二）：至少一项、判断顺序唯一、同一种控制至多一项；
// 逐项按顺序排定，与交入顺序无关。条件集外在这里就拒绝，而不是等到形成接受判断时——那时逐项
// 已经进了任务记录。
func NewExecutedFinancialControlResult(spec ExecutedFinancialControlSpec) (FinancialControlResult, error) {
	if !spec.ResultID.valid() || !spec.AsOf.valid() || len(spec.Items) == 0 || !spec.JointPass.valid() {
		return FinancialControlResult{}, ErrInvalidFinancialControlResult
	}
	items, err := orderedControlItems(spec.Items)
	if err != nil {
		return FinancialControlResult{}, err
	}
	outcome, basis, err := jointConclusion(items, spec.JointPass)
	if err != nil {
		return FinancialControlResult{}, err
	}
	return FinancialControlResult{
		resultID:  spec.ResultID,
		outcome:   outcome,
		basis:     basis,
		asOf:      spec.AsOf,
		items:     items,
		jointPass: spec.JointPass,
	}, nil
}

// NewInapplicableFinancialControlResult 形成`明确无控制`：合同为本范围声明接受前无财务控制。
//
// 必带商业不适用依据——没有依据的`明确无控制`与「默认信用通过」无从分辨，而用例明禁后者。不带
// 结果标识：那一支下 settlement-accounting 不形成冻结，也就没有签发标识可引用，强行要一个只能由
// 适配器发明（与可达性`不适用`同一条理由）。不带逐项也不带条件：没有执行过任何一项。
func NewInapplicableFinancialControlResult(
	basis ControlBasisReference,
	asOf JudgmentAsOf,
) (FinancialControlResult, error) {
	if !basis.valid() || !asOf.valid() {
		return FinancialControlResult{}, ErrInvalidFinancialControlResult
	}
	return FinancialControlResult{outcome: FinancialControlNotApplicable, basis: basis, asOf: asOf}, nil
}

// RehydrateFinancialControlResultSpec 是采用结果在库里的样子：结论与依据是当时记下的产物，这里当
// 数据收下。
type RehydrateFinancialControlResultSpec struct {
	ResultID  FinancialControlResultID
	Outcome   FinancialControlOutcome
	Basis     ControlBasisReference
	AsOf      JudgmentAsOf
	Items     []ControlItemResult
	JointPass JointPassCondition
}

// RehydrateFinancialControlResult 是登记册重建采用结果的那扇门（ADR-0028 重建只校验不重算）。
//
// 它不用今天的推导覆盖当时记下的结论，只核两者在所记条件下一致：结论或依据对不上逐项、`明确无
// 控制`却记了逐项或条件，都是坏数据——那一行不可能是本上下文判出来的，按 ErrInvalidRehydrated
// FinancialControlResult 拒绝，让读的人去查那一行或写它的适配器。一致时交回的与首次构造等值。
func RehydrateFinancialControlResult(spec RehydrateFinancialControlResultSpec) (FinancialControlResult, error) {
	if spec.Outcome == FinancialControlNotApplicable {
		if len(spec.Items) != 0 || spec.JointPass != JointPassConditionInvalid || spec.ResultID.valid() {
			return FinancialControlResult{}, ErrInvalidRehydratedFinancialControlResult
		}
		result, err := NewInapplicableFinancialControlResult(spec.Basis, spec.AsOf)
		if err != nil {
			return FinancialControlResult{}, ErrInvalidRehydratedFinancialControlResult
		}
		return result, nil
	}
	rebuilt, err := NewExecutedFinancialControlResult(ExecutedFinancialControlSpec{
		ResultID:  spec.ResultID,
		Items:     spec.Items,
		JointPass: spec.JointPass,
		AsOf:      spec.AsOf,
	})
	if err != nil {
		return FinancialControlResult{}, ErrInvalidRehydratedFinancialControlResult
	}
	if rebuilt.outcome != spec.Outcome || rebuilt.basis != spec.Basis {
		return FinancialControlResult{}, ErrInvalidRehydratedFinancialControlResult
	}
	return rebuilt, nil
}

// orderedControlItems 逐项过构造门、按判断顺序排定，并守顺序唯一与种类唯一。
func orderedControlItems(items []ControlItemResult) ([]ControlItemResult, error) {
	seenOrders := make(map[uint32]struct{}, len(items))
	seenKinds := make(map[ControlItemKind]struct{}, len(items))
	ordered := make([]ControlItemResult, 0, len(items))
	for _, item := range items {
		if !item.valid() {
			return nil, ErrInvalidFinancialControlResult
		}
		if _, duplicate := seenOrders[item.order]; duplicate {
			return nil, ErrInvalidFinancialControlResult
		}
		if _, duplicate := seenKinds[item.kind]; duplicate {
			return nil, ErrInvalidFinancialControlResult
		}
		seenOrders[item.order] = struct{}{}
		seenKinds[item.kind] = struct{}{}
		ordered = append(ordered, item)
	}
	sort.Slice(ordered, func(left, right int) bool {
		return ordered[left].order < ordered[right].order
	})
	return ordered, nil
}

// jointConclusion 是 SA CONTEXT「由 parcel-shipment 按策略的共同通过条件形成接受判断」落在代码里的
// 那一处，也是唯一一处。逐值分派、default 报错不吸收（ADR-0025）。
//
// 「全部通过」之下：任一项受限即 `RESTRICTED`，依据取判断顺序最靠前的受限项自己的原因——提供方今天
// 停在第一处限制，所以至多一项受限，但这里不把那条假设写进来；全部成立时，成立的项里有预付冻结即
// `HELD`、否则 `CREDIT_EXPOSED`（ADR-0122 决定四「两者并存时以资金已被占用那一句为准」）。
func jointConclusion(
	items []ControlItemResult,
	condition JointPassCondition,
) (FinancialControlOutcome, ControlBasisReference, error) {
	switch condition {
	case AllControlsPass:
		for _, item := range items {
			if item.conclusion == ControlItemRestricted {
				return FinancialControlRestricted, item.basis, nil
			}
		}
		for _, item := range items {
			if item.kind == PrepaidFreezeControlItem {
				return FinancialControlHeld, ControlBasisReference{}, nil
			}
		}
		return FinancialControlCreditExposed, ControlBasisReference{}, nil
	default:
		return FinancialControlOutcomeInvalid, ControlBasisReference{}, ErrInvalidFinancialControlResult
	}
}

func (result FinancialControlResult) ResultID() FinancialControlResultID {
	return result.resultID
}

// Outcome 是接受侧结论。零值表示尚未形成控制（ports.RecordedJudgments 的约定）。
func (result FinancialControlResult) Outcome() FinancialControlOutcome {
	return result.outcome
}

// Basis 在 `RESTRICTED` 时是受限项自己的原因，在 `NOT_APPLICABLE` 时是合同的商业不适用依据，成立时为空。
func (result FinancialControlResult) Basis() ControlBasisReference {
	return result.basis
}

func (result FinancialControlResult) AsOf() JudgmentAsOf {
	return result.asOf
}

// Items 按判断顺序交回逐项结果（副本）。`明确无控制`与尚未形成都是空。
func (result FinancialControlResult) Items() []ControlItemResult {
	return append([]ControlItemResult(nil), result.items...)
}

// JointPassCondition 是结论按哪个条件推出的；`明确无控制`与尚未形成都是零值。
func (result FinancialControlResult) JointPassCondition() JointPassCondition {
	return result.jointPass
}

// OccupationFormed 报告提供方账本上有没有为这次控制形成占用：任一项成立即有。
//
// 它是释放该看的那一格，而不是 Outcome（ADR-0125 决定四）：第一项冻结成立、第二项受限的结论是
// `RESTRICTED`，那笔冻结却确已占下；`CREDIT_EXPOSED` 占的是额度，同样要释放。释放按请求身份在两本账
// 各认领一次、限制不入账本（ADR-0122 决定三），所以「有成立项就发」既不漏也不会对着不存在的占用重试。
func (result FinancialControlResult) OccupationFormed() bool {
	for _, item := range result.items {
		if item.Satisfied() {
			return true
		}
	}
	return false
}
