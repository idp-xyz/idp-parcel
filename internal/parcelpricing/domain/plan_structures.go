package domain

import (
	"fmt"
	"sort"
	"strings"
)

// SurchargeCalculation 携带计算方法及该方法所需的参数。把参数放在这里而不是放在规则
// 上，才使得同一种规则形状能服务闭合集合里的每一种计算方法。
type SurchargeCalculation struct {
	seriesKind   *ReferenceSeriesKind
	seriesFactor *Decimal
	method       ChargeMethod
	amount       *Money
	table        *RateTableVersion
	percentage   *Decimal
	basis        string
	operands     []SurchargeCalculation
	// seriesID 与 outOfWindow 只在「取当期序列定额」上有：金额序列按标识引用（一张卡可绑多条），窗外行为
	// 由卡声明（ADR-0110 Decision 三）。
	seriesID    string
	outOfWindow OutOfWindowBehaviour
}

// OutOfWindowBehaviour 是绑了金额序列的附加费规则在卡上声明的窗外行为（ADR-0110 Decision 三），封闭两格：
// 不计收——PSS 的常态，窗外就是不收，零是登记出来的答案；待判断——金额尚未公布、不能当零。未声明的卡不合法：
// 机制不猜哪一格。
type OutOfWindowBehaviour string

const (
	OutOfWindowNotCharged OutOfWindowBehaviour = "NOT_CHARGED"
	OutOfWindowPending    OutOfWindowBehaviour = "PENDING"
)

func (behaviour OutOfWindowBehaviour) String() string { return string(behaviour) }

func (behaviour OutOfWindowBehaviour) valid() bool {
	switch behaviour {
	case OutOfWindowNotCharged, OutOfWindowPending:
		return true
	default:
		return false
	}
}

// NewSeriesAmountSurcharge 声明「取当期序列定额」：金额来自标识为 seriesID 的金额序列在评价基准时点的那一期；
// 窗外（该期次不存在）按 behaviour 分流。方案必须同时绑定这条序列，那一道在 NewPricingPlanStructures 上判。
func NewSeriesAmountSurcharge(seriesID string, behaviour OutOfWindowBehaviour) (SurchargeCalculation, error) {
	return validSurcharge(SurchargeCalculation{
		method:      ChargeMethodSeriesAmount,
		seriesID:    seriesID,
		outOfWindow: behaviour,
	})
}

// SeriesAmount 只在「取当期序列定额」上给出：序列标识与窗外行为。
func (calculation SurchargeCalculation) SeriesAmount() (string, OutOfWindowBehaviour, bool) {
	if calculation.method != ChargeMethodSeriesAmount {
		return "", "", false
	}
	return calculation.seriesID, calculation.outOfWindow, true
}

// amountSeriesIDs 报出本计算引用的每一条金额序列的标识，包括藏在取较大值操作数里的。
func (calculation SurchargeCalculation) amountSeriesIDs() []string {
	switch calculation.method {
	case ChargeMethodSeriesAmount:
		return []string{calculation.seriesID}
	case ChargeMethodGreaterOf:
		var ids []string
		for _, operand := range calculation.operands {
			ids = append(ids, operand.amountSeriesIDs()...)
		}
		return ids
	default:
		return nil
	}
}

func NewFixedAmountSurcharge(amount Money) (SurchargeCalculation, error) {
	return validSurcharge(SurchargeCalculation{method: ChargeMethodFixedAmount, amount: &amount})
}

func NewTableLookupSurcharge(table RateTableVersion) (SurchargeCalculation, error) {
	return validSurcharge(SurchargeCalculation{method: ChargeMethodTableLookup, table: &table})
}

// NewPercentOfBasisSurcharge 接收定义基数的那条 ChargeDependency 的 ID。指名费用依赖
// 而不是再抄一份费用代码清单，使基数只有一处定义；方案会拒绝那些指名了它并未声明的
// 基数的规则。
func NewPercentOfBasisSurcharge(percentage Decimal, basisDependencyID string) (SurchargeCalculation, error) {
	return validSurcharge(SurchargeCalculation{
		method:     ChargeMethodPercentOfBasis,
		percentage: &percentage,
		basis:      basisDependencyID,
	})
}

// NewSeriesRateSurcharge 按基数百分比计收，但费率不是卡上的一个数字，而是承运商公布
// 的读数，再乘以卡上确实写明的折扣系数。`L5` 配 `F1` 正是这种情形：当周费率乘 80%。
// 两个数分开保留，读的人才能分清是费率变了还是折扣变了。
func NewSeriesRateSurcharge(kind ReferenceSeriesKind, factor Decimal, basisDependencyID string) (SurchargeCalculation, error) {
	if !kind.valid() {
		return SurchargeCalculation{}, ErrInvalidSurchargeRule
	}
	// 计算方法仍是`按基数百分比`：不同的是费率从哪来，不是金额怎么产生。CONTEXT 把
	// 计算方法闭合为四种，而费率的来源与算术是两个不同的轴。
	return validSurcharge(SurchargeCalculation{
		method:       ChargeMethodPercentOfBasis,
		seriesKind:   &kind,
		seriesFactor: &factor,
		basis:        basisDependencyID,
	})
}

// NewGreaterOfSurcharge 在另外两种计算方法之间取较大值。两个操作数都不能自身又是取
// 较大值：卡上要的是在两个金额之间二选一，不是任意嵌套的表达式。
func NewGreaterOfSurcharge(first, second SurchargeCalculation) (SurchargeCalculation, error) {
	return validSurcharge(SurchargeCalculation{
		method:   ChargeMethodGreaterOf,
		operands: []SurchargeCalculation{first, second},
	})
}

func validSurcharge(calculation SurchargeCalculation) (SurchargeCalculation, error) {
	if !calculation.valid() {
		return SurchargeCalculation{}, ErrInvalidSurchargeRule
	}
	return calculation, nil
}

func (calculation SurchargeCalculation) Method() ChargeMethod { return calculation.method }

func (calculation SurchargeCalculation) FixedAmount() (Money, bool) {
	if calculation.method != ChargeMethodFixedAmount || calculation.amount == nil {
		return Money{}, false
	}
	return *calculation.amount, true
}

func (calculation SurchargeCalculation) LookupTable() (RateTableVersion, bool) {
	if calculation.method != ChargeMethodTableLookup || calculation.table == nil {
		return RateTableVersion{}, false
	}
	return *calculation.table, true
}

func (calculation SurchargeCalculation) PercentOfBasis() (Decimal, string, bool) {
	if calculation.method != ChargeMethodPercentOfBasis || calculation.percentage == nil {
		return Decimal{}, "", false
	}
	return *calculation.percentage, calculation.basis, true
}

func (calculation SurchargeCalculation) Operands() []SurchargeCalculation {
	return append([]SurchargeCalculation(nil), calculation.operands...)
}

// basisDependencyIDs 报出本计算指名的每一个费用依赖 ID，包括藏在取较大值操作数里的。
func (calculation SurchargeCalculation) basisDependencyIDs() []string {
	switch calculation.method {
	case ChargeMethodPercentOfBasis:
		return []string{calculation.basis}
	case ChargeMethodGreaterOf:
		var names []string
		for _, operand := range calculation.operands {
			names = append(names, operand.basisDependencyIDs()...)
		}
		return names
	default:
		return nil
	}
}

func (calculation SurchargeCalculation) valid() bool {
	if !calculation.method.valid() {
		return false
	}
	// 百分比费率要么写成卡上的一个数字，要么写成公布序列乘卡上折扣系数——
	// 两者不能同时出现，也不能都不出现。
	seriesRate := calculation.seriesKind != nil && calculation.seriesFactor != nil
	if (calculation.seriesKind != nil) != (calculation.seriesFactor != nil) {
		return false
	}
	seriesAmount := calculation.seriesID != "" || calculation.outOfWindow != ""
	populated := 0
	for _, present := range []bool{
		calculation.amount != nil,
		calculation.table != nil,
		calculation.percentage != nil || seriesRate,
		len(calculation.operands) > 0,
		seriesAmount,
	} {
		if present {
			populated++
		}
	}
	if populated != 1 {
		return false
	}
	if calculation.percentage != nil && seriesRate {
		return false
	}
	switch calculation.method {
	case ChargeMethodSeriesAmount:
		return trimmed(calculation.seriesID) && calculation.outOfWindow.valid()
	case ChargeMethodFixedAmount:
		return calculation.amount != nil && calculation.amount.valid()
	case ChargeMethodTableLookup:
		return calculation.table != nil && calculation.table.valid()
	case ChargeMethodPercentOfBasis:
		if !trimmed(calculation.basis) {
			return false
		}
		if seriesRate {
			return calculation.seriesKind.valid() &&
				calculation.seriesFactor.valid() && !calculation.seriesFactor.IsNegative()
		}
		return calculation.percentage != nil && calculation.percentage.valid() && !calculation.percentage.IsNegative()
	case ChargeMethodGreaterOf:
		if len(calculation.operands) != 2 {
			return false
		}
		for _, operand := range calculation.operands {
			if operand.method == ChargeMethodGreaterOf || !operand.valid() {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// ConditionalMinimumWeight 在其触发条件成立时抬高方案级计价重量。卡把它写在某条附加费
// 条款里，但它作用的是基础价查表与后续每一次基数读取所共用的那一个计价重量，不是声明
// 它的那条规则的私有基数。
type ConditionalMinimumWeight struct {
	id        string
	condition TriggerCondition
	minimum   Weight
}

func NewConditionalMinimumWeight(id string, condition TriggerCondition, minimum Weight) (ConditionalMinimumWeight, error) {
	value := ConditionalMinimumWeight{id: id, condition: condition, minimum: minimum}
	if !value.valid() {
		return ConditionalMinimumWeight{}, ErrInvalidSurchargeRule
	}
	return value, nil
}

func (value ConditionalMinimumWeight) ID() string                  { return value.id }
func (value ConditionalMinimumWeight) Condition() TriggerCondition { return value.condition }
func (value ConditionalMinimumWeight) Minimum() Weight             { return value.minimum }

func (value ConditionalMinimumWeight) valid() bool {
	return trimmed(value.id) && value.condition.valid() && value.minimum.valid() && value.minimum.value.Sign() > 0
}

// ExclusivityStance 表达卡对「这条附加费是与其他附加费竞争，还是与它们并列计收」的
// 说法。它有意取三个值：承运商在这一点上并不一致——UPS 把大件费与额外操作费放进同一个
// 互斥组，FedEx 两项都收——所以未设置互斥组绝不能被读成「独立计收」。沉默本身是一种
// 状态，方案拒绝它。
type ExclusivityStance string

const (
	ExclusivityUndeclared ExclusivityStance = ""
	ExclusivityStandalone ExclusivityStance = "STANDALONE"
	ExclusivityGrouped    ExclusivityStance = "GROUPED"
)

func (stance ExclusivityStance) String() string { return string(stance) }

// SurchargeRule 是一条版本化规则：触发条件成立时，在基础运费之外产生一条评价费用行。
type SurchargeRule struct {
	id               string
	chargeCode       ChargeCode
	description      string
	effect           ChargeEffect
	condition        TriggerCondition
	calculation      SurchargeCalculation
	exclusivity      ExclusivityStance
	exclusivityGroup string
	priority         int
	minimumWeight    *ConditionalMinimumWeight
}

func NewSurchargeRule(
	id string,
	code ChargeCode,
	description string,
	effect ChargeEffect,
	condition TriggerCondition,
	calculation SurchargeCalculation,
) (SurchargeRule, error) {
	rule := SurchargeRule{
		id:          id,
		chargeCode:  code,
		description: description,
		effect:      effect,
		condition:   condition,
		calculation: calculation,
	}
	if !rule.valid() {
		return SurchargeRule{}, ErrInvalidSurchargeRule
	}
	return rule, nil
}

// InExclusivityGroup 把规则作为某个互斥组的成员、以给定优先级返回。优先级只有相对于
// 组内其他成员才有意义，所以两者要么一起声明，要么都不声明。
func (rule SurchargeRule) InExclusivityGroup(group string, priority int) (SurchargeRule, error) {
	if !trimmed(group) || priority < 1 {
		return SurchargeRule{}, ErrInvalidSurchargeRule
	}
	rule.exclusivity = ExclusivityGrouped
	rule.exclusivityGroup = group
	rule.priority = priority
	if !rule.valid() {
		return SurchargeRule{}, ErrInvalidSurchargeRule
	}
	return rule, nil
}

// Standalone 记录卡把这条附加费与其他附加费并列计收。它本身就是一次声明，
// 而不是「没有声明」。
func (rule SurchargeRule) Standalone() (SurchargeRule, error) {
	rule.exclusivity = ExclusivityStandalone
	rule.exclusivityGroup = ""
	rule.priority = 0
	if !rule.valid() {
		return SurchargeRule{}, ErrInvalidSurchargeRule
	}
	return rule, nil
}

func (rule SurchargeRule) WithConditionalMinimumWeight(minimum ConditionalMinimumWeight) (SurchargeRule, error) {
	if !minimum.valid() {
		return SurchargeRule{}, ErrInvalidSurchargeRule
	}
	rule.minimumWeight = &minimum
	if !rule.valid() {
		return SurchargeRule{}, ErrInvalidSurchargeRule
	}
	return rule, nil
}

func (rule SurchargeRule) ID() string                        { return rule.id }
func (rule SurchargeRule) Code() ChargeCode                  { return rule.chargeCode }
func (rule SurchargeRule) Description() string               { return rule.description }
func (rule SurchargeRule) Effect() ChargeEffect              { return rule.effect }
func (rule SurchargeRule) Condition() TriggerCondition       { return rule.condition }
func (rule SurchargeRule) Calculation() SurchargeCalculation { return rule.calculation }
func (rule SurchargeRule) Priority() int                     { return rule.priority }

func (rule SurchargeRule) Exclusivity() ExclusivityStance { return rule.exclusivity }

func (rule SurchargeRule) ExclusivityGroup() (string, bool) {
	if rule.exclusivity != ExclusivityGrouped {
		return "", false
	}
	return rule.exclusivityGroup, true
}

func (rule SurchargeRule) ConditionalMinimumWeight() (ConditionalMinimumWeight, bool) {
	if rule.minimumWeight == nil {
		return ConditionalMinimumWeight{}, false
	}
	return *rule.minimumWeight, true
}

// declaredCurrencies 报出规则声明的每一笔金额的币种：定额、分档价表，以及取较大值的
// 每一个操作数。一个都不能漏，因为下游没有任何环节能拦住外币金额——费用合计是以裸小数
// 累加的，最后才盖上方案的币种，于是一个未经校验的操作数会被当作方案本币计入，而那次
// 评价读起来仍是`已完成`。这道门是唯一的防线。
// 百分比本身不声明币种，它取所依据基数的币种。
func (rule SurchargeRule) declaredCurrencies() []Currency {
	return rule.calculation.declaredCurrencies()
}

func (calculation SurchargeCalculation) declaredCurrencies() []Currency {
	switch {
	case calculation.amount != nil:
		return []Currency{calculation.amount.currency}
	case calculation.table != nil:
		return []Currency{calculation.table.currency}
	default:
		var currencies []Currency
		for _, operand := range calculation.operands {
			currencies = append(currencies, operand.declaredCurrencies()...)
		}
		return currencies
	}
}

// declaredWeightUnits 报出规则声明的每一个重量单位：分档价表按哪个单位查，以及条件
// 最低计价重量抬高到哪个单位。两者都作用于计价重量，而计价重量由方案按其基础价表的
// 分档单位表达，所以方案不据以计价的单位根本读不出来。条件最低计价重量正是这道校验
// 必须放在工件构造期而不是评价期的原因：放到评价期，它只会在触发该条款的包裹上失败，
// 于是同一张卡对一部分包裹算得出价、对另一部分报冲突。
func (rule SurchargeRule) declaredWeightUnits() []WeightUnit {
	units := rule.calculation.lookupWeightUnits()
	if rule.minimumWeight != nil {
		units = append(units, rule.minimumWeight.minimum.unit)
	}
	return units
}

// lookupWeightUnits 报出本计算从中查档的每一张价表的单位，包括藏在取较大值操作数里的。
func (calculation SurchargeCalculation) lookupWeightUnits() []WeightUnit {
	if calculation.table != nil {
		return []WeightUnit{calculation.table.unit}
	}
	var units []WeightUnit
	for _, operand := range calculation.operands {
		units = append(units, operand.lookupWeightUnits()...)
	}
	return units
}

func (rule SurchargeRule) valid() bool {
	if !trimmed(rule.id) || !trimmed(rule.description) || !rule.chargeCode.valid() ||
		!rule.effect.valid() || !rule.condition.valid() || !rule.calculation.valid() {
		return false
	}
	switch rule.exclusivity {
	case ExclusivityUndeclared, ExclusivityStandalone:
		if rule.exclusivityGroup != "" || rule.priority != 0 {
			return false
		}
	case ExclusivityGrouped:
		if !trimmed(rule.exclusivityGroup) || rule.priority < 1 {
			return false
		}
	default:
		return false
	}
	return rule.minimumWeight == nil || rule.minimumWeight.valid()
}

// ChargeBasisComposition 说明一条费用依赖的基数如何构成。卡两种形状都要：燃油的基数是
// 本票全部其他费用减去指名的排除项，而百分比附加费则逐项列举它适用于哪些费用。
type ChargeBasisComposition string

const (
	ChargeBasisAllCharges    ChargeBasisComposition = "ALL_CHARGES"
	ChargeBasisListedCharges ChargeBasisComposition = "LISTED_CHARGES"
)

func (composition ChargeBasisComposition) String() string { return string(composition) }

func (composition ChargeBasisComposition) valid() bool {
	switch composition {
	case ChargeBasisAllCharges, ChargeBasisListedCharges:
		return true
	default:
		return false
	}
}

// ChargeDependency 声明一条费用以其他费用的小计为基数。基数构成与排除集都显式给出：
// 声明顺序永远不隐含依赖，因为顺序是呈现，基数是规则。被依赖的费用行本身永不进入
// 自己的基数。
type ChargeDependency struct {
	id          string
	dependent   ChargeCode
	composition ChargeBasisComposition
	includes    []ChargeCode
	excludes    []ChargeCode
}

func NewAllChargesDependency(id string, dependent ChargeCode, excludes []ChargeCode) (ChargeDependency, error) {
	return newChargeDependency(id, dependent, ChargeBasisAllCharges, nil, excludes)
}

func NewListedChargeDependency(id string, dependent ChargeCode, includes, excludes []ChargeCode) (ChargeDependency, error) {
	return newChargeDependency(id, dependent, ChargeBasisListedCharges, includes, excludes)
}

func newChargeDependency(
	id string,
	dependent ChargeCode,
	composition ChargeBasisComposition,
	includes, excludes []ChargeCode,
) (ChargeDependency, error) {
	dependency := ChargeDependency{
		id:          id,
		dependent:   dependent,
		composition: composition,
		includes:    sortedChargeCodes(includes),
		excludes:    sortedChargeCodes(excludes),
	}
	if !dependency.valid() {
		return ChargeDependency{}, ErrInvalidChargeDependency
	}
	return dependency, nil
}

func (dependency ChargeDependency) ID() string       { return dependency.id }
func (dependency ChargeDependency) Code() ChargeCode { return dependency.dependent }
func (dependency ChargeDependency) Composition() ChargeBasisComposition {
	return dependency.composition
}
func (dependency ChargeDependency) Includes() []ChargeCode {
	return append([]ChargeCode(nil), dependency.includes...)
}
func (dependency ChargeDependency) Excludes() []ChargeCode {
	return append([]ChargeCode(nil), dependency.excludes...)
}

func (dependency ChargeDependency) valid() bool {
	if !trimmed(dependency.id) || !dependency.dependent.valid() || !dependency.composition.valid() {
		return false
	}
	switch dependency.composition {
	case ChargeBasisAllCharges:
		if len(dependency.includes) != 0 {
			return false
		}
	case ChargeBasisListedCharges:
		if len(dependency.includes) == 0 {
			return false
		}
	}
	seen := make(map[string]struct{}, len(dependency.includes)+len(dependency.excludes))
	for _, group := range [][]ChargeCode{dependency.includes, dependency.excludes} {
		for index, code := range group {
			if !code.valid() || code == dependency.dependent {
				return false
			}
			if index > 0 && group[index-1].String() >= code.String() {
				return false
			}
			if _, exists := seen[code.String()]; exists {
				return false
			}
			seen[code.String()] = struct{}{}
		}
	}
	return true
}

func sortedChargeCodes(codes []ChargeCode) []ChargeCode {
	if len(codes) == 0 {
		return nil
	}
	sorted := append([]ChargeCode(nil), codes...)
	sort.SliceStable(sorted, func(left, right int) bool {
		return sorted[left].String() < sorted[right].String()
	})
	return sorted
}

// ReferenceSeriesKind 是方案可以解析的外部数值序列的封闭集合。ADR-0013 把这些序列的
// 登记与版本化交给计价，但数值本身不归计价生产。
type ReferenceSeriesKind string

const (
	ReferenceSeriesFuelRate     ReferenceSeriesKind = "FUEL_RATE"
	ReferenceSeriesExchangeRate ReferenceSeriesKind = "EXCHANGE_RATE"
	// ReferenceSeriesPublishedAmount 是第三种序列（ADR-0110 Decision 一）：承运商按期公布的**金额**（高峰 /
	// 需求附加费一类），一期取值带币种。它与两种费率序列的差别不止取值类型：费率序列由计算按种类引用、一张
	// 卡每种至多绑一条；金额序列由计算按序列标识引用，一张卡可绑多条（每分区一条，Decision 四）。
	ReferenceSeriesPublishedAmount ReferenceSeriesKind = "PUBLISHED_AMOUNT"
)

func (kind ReferenceSeriesKind) String() string { return string(kind) }

func (kind ReferenceSeriesKind) valid() bool {
	switch kind {
	case ReferenceSeriesFuelRate, ReferenceSeriesExchangeRate, ReferenceSeriesPublishedAmount:
		return true
	default:
		return false
	}
}

// carriesAmount 报出该种序列的取值是不是带币种的金额；费率序列的取值是裸小数。
func (kind ReferenceSeriesKind) carriesAmount() bool {
	return kind == ReferenceSeriesPublishedAmount
}

// ReferenceSeriesBinding 把方案绑定到一条计价参考序列——按种类与序列标识，不按序列版本
// （ADR-0099 决定一）。绑定指名的是序列，绝不是取值也不是版本：用哪一版由评价形成时刻的
// 在用序列版本决定，取值按计价基准时点在该版本内解析，两者一并冻结进该次评价的版本清单。
// 绑住版本的代价是序列每出一版、引用它的每张卡都要重登，而按日公布的汇率会让这条路在
// 结构上走不通。
type ReferenceSeriesBinding struct {
	kind     ReferenceSeriesKind
	seriesID string
}

func NewReferenceSeriesBinding(kind ReferenceSeriesKind, seriesID string) (ReferenceSeriesBinding, error) {
	binding := ReferenceSeriesBinding{kind: kind, seriesID: seriesID}
	if !binding.valid() {
		return ReferenceSeriesBinding{}, ErrInvalidReferenceSeries
	}
	return binding, nil
}

func (binding ReferenceSeriesBinding) Kind() ReferenceSeriesKind { return binding.kind }

// SeriesID 是租户内的序列标识，与登记册里的序列 ID 同一取值。
func (binding ReferenceSeriesBinding) SeriesID() string { return binding.seriesID }

func (binding ReferenceSeriesBinding) valid() bool {
	return binding.kind.valid() && trimmed(binding.seriesID)
}

func compareSeriesBindings(left, right ReferenceSeriesBinding) int {
	for _, pair := range [][2]string{
		{string(left.kind), string(right.kind)},
		{left.seriesID, right.seriesID},
	} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}

// PricingPlanStructures 携带一个已发布方案在基础价表和无条件固定规则之外声明的规则
// 结构。零值表示一项都没声明。
//
// 它做成一个值而不是几个构造参数，是为了让规范化方案文档在任何一项结构可执行之前，
// 就为 CONTEXT 在`版本内容摘要`下列出的每一种结构都留好位置。等真实评价已经存在之后
// 再拓宽规范化形状，会让那些从未用过新结构的方案的内容摘要发生位移，把每一次重放都
// 报成版本内容冲突。
type PricingPlanStructures struct {
	surchargeRules  []SurchargeRule
	dependencies    []ChargeDependency
	referenceSeries []ReferenceSeriesBinding
	exclusions      []ExclusionRule
	// amountRounding 是卡声明的金额取整策略（ADR-0107），可缺。挂在这里而不是构造参数位，理由同拒收
	// 条款：它是可缺的另一个轴，多数 SYN 卡不声明；「与重量取整同形」说的是声明的形状（模式 + 进位
	// 单位），不是构造参数的位置。
	amountRounding *AmountRoundingPolicy
	// referenceCatalogues 是卡声明的目录绑定（ADR-0109），可缺：没绑目录的卡保留调用方给分区的路径，
	// 两条路径由这一格是否为空在内容摘要里分开。
	referenceCatalogues []ReferenceCatalogueLink
}

func NewPricingPlanStructures(
	surchargeRules []SurchargeRule,
	dependencies []ChargeDependency,
	referenceSeries []ReferenceSeriesBinding,
) (PricingPlanStructures, error) {
	copyOfRules := append([]SurchargeRule(nil), surchargeRules...)
	seenRuleIDs := make(map[string]struct{}, len(copyOfRules))
	for _, rule := range copyOfRules {
		if !rule.valid() {
			return PricingPlanStructures{}, ErrInvalidSurchargeRule
		}
		// 这条附加费是否与其他附加费竞争，是承运商的规则，且各家不同，
		// 所以一张从未说过的卡，按哪一种读法都不能发布。
		if rule.exclusivity == ExclusivityUndeclared {
			return PricingPlanStructures{}, fmt.Errorf("%w: %s", ErrUndeclaredExclusivity, rule.id)
		}
		if _, exists := seenRuleIDs[rule.id]; exists {
			return PricingPlanStructures{}, fmt.Errorf("%w: %s", ErrDuplicateSurchargeRule, rule.id)
		}
		seenRuleIDs[rule.id] = struct{}{}
	}
	sort.SliceStable(copyOfRules, func(left, right int) bool {
		return copyOfRules[left].id < copyOfRules[right].id
	})
	copyOfDependencies := append([]ChargeDependency(nil), dependencies...)
	seenDependencyIDs := make(map[string]struct{}, len(copyOfDependencies))
	for _, dependency := range copyOfDependencies {
		if !dependency.valid() {
			return PricingPlanStructures{}, ErrInvalidChargeDependency
		}
		if _, exists := seenDependencyIDs[dependency.id]; exists {
			return PricingPlanStructures{}, fmt.Errorf("%w: %s", ErrInvalidChargeDependency, dependency.id)
		}
		seenDependencyIDs[dependency.id] = struct{}{}
	}
	sort.SliceStable(copyOfDependencies, func(left, right int) bool {
		return copyOfDependencies[left].id < copyOfDependencies[right].id
	})
	copyOfSeries := append([]ReferenceSeriesBinding(nil), referenceSeries...)
	for _, binding := range copyOfSeries {
		if !binding.valid() {
			return PricingPlanStructures{}, ErrInvalidReferenceSeries
		}
	}
	sort.SliceStable(copyOfSeries, func(left, right int) bool {
		return compareSeriesBindings(copyOfSeries[left], copyOfSeries[right]) < 0
	})
	declaredBases := make(map[string]struct{}, len(copyOfDependencies))
	for _, dependency := range copyOfDependencies {
		declaredBases[dependency.id] = struct{}{}
	}
	boundAmountSeries := make(map[string]struct{}, len(copyOfSeries))
	for _, binding := range copyOfSeries {
		if binding.kind.carriesAmount() {
			boundAmountSeries[binding.seriesID] = struct{}{}
		}
	}
	for _, rule := range copyOfRules {
		for _, basis := range rule.calculation.basisDependencyIDs() {
			if _, declared := declaredBases[basis]; !declared {
				return PricingPlanStructures{}, fmt.Errorf("%w: %s names undeclared basis %s", ErrInvalidChargeDependency, rule.id, basis)
			}
		}
		// 取当期序列定额的规则必须指名一条已绑定的金额序列：绑定是评价解析读数的唯一入口，没绑就永远解不到。
		for _, seriesID := range rule.calculation.amountSeriesIDs() {
			if _, bound := boundAmountSeries[seriesID]; !bound {
				return PricingPlanStructures{}, fmt.Errorf("%w: %s names unbound amount series %s", ErrInvalidReferenceSeries, rule.id, seriesID)
			}
		}
	}
	structures := PricingPlanStructures{
		surchargeRules:  copyOfRules,
		dependencies:    copyOfDependencies,
		referenceSeries: copyOfSeries,
	}
	if !structures.valid() {
		return PricingPlanStructures{}, ErrInvalidPlanStructures
	}
	return structures, nil
}

func (structures PricingPlanStructures) SurchargeRules() []SurchargeRule {
	return append([]SurchargeRule(nil), structures.surchargeRules...)
}

func (structures PricingPlanStructures) ChargeDependencies() []ChargeDependency {
	return append([]ChargeDependency(nil), structures.dependencies...)
}

func (structures PricingPlanStructures) ReferenceSeries() []ReferenceSeriesBinding {
	return append([]ReferenceSeriesBinding(nil), structures.referenceSeries...)
}

// WithExclusionRules 返回携带卡上拒收条款的结构集合。它做成 builder 而不是构造参数，
// 因为一项都不声明才是常态，而已有的三个参数描述的是卡收什么费；拒收是另一个轴。
func (structures PricingPlanStructures) WithExclusionRules(rules ...ExclusionRule) (PricingPlanStructures, error) {
	copyOfRules := append([]ExclusionRule(nil), rules...)
	seen := make(map[string]struct{}, len(copyOfRules))
	for _, rule := range copyOfRules {
		if !rule.valid() {
			return PricingPlanStructures{}, ErrInvalidExclusionRule
		}
		if _, exists := seen[rule.id]; exists {
			return PricingPlanStructures{}, fmt.Errorf("%w: %s", ErrInvalidExclusionRule, rule.id)
		}
		seen[rule.id] = struct{}{}
	}
	sort.SliceStable(copyOfRules, func(left, right int) bool {
		return copyOfRules[left].id < copyOfRules[right].id
	})
	structures.exclusions = copyOfRules
	if !structures.valid() {
		return PricingPlanStructures{}, ErrInvalidPlanStructures
	}
	return structures, nil
}

func (structures PricingPlanStructures) ExclusionRules() []ExclusionRule {
	return append([]ExclusionRule(nil), structures.exclusions...)
}

// WithAmountRounding 返回携带金额取整策略的结构集合（ADR-0107 Decision 一、二）。进位单位的币种要
// 与卡币种一致，那一道在 NewPricingPlanVersion 上判——结构集合自己不知道卡的币种。
func (structures PricingPlanStructures) WithAmountRounding(policy AmountRoundingPolicy) (PricingPlanStructures, error) {
	if !policy.valid() {
		return PricingPlanStructures{}, ErrInvalidAmountRoundingPolicy
	}
	declared := policy
	structures.amountRounding = &declared
	if !structures.valid() {
		return PricingPlanStructures{}, ErrInvalidPlanStructures
	}
	return structures, nil
}

// AmountRounding 交回卡声明的金额取整策略；未声明时第二个返回值为假——那不是缺陷，是
// ADR-0107 Decision 四说的「未声明即不取整并记问题项」那一格的输入。
func (structures PricingPlanStructures) AmountRounding() (AmountRoundingPolicy, bool) {
	if structures.amountRounding == nil {
		return AmountRoundingPolicy{}, false
	}
	return *structures.amountRounding, true
}

// Declared 报出方案是否声明了任何结构，使得求值器必须先执行它才能产出完整金额。
func (structures PricingPlanStructures) Declared() bool {
	return len(structures.surchargeRules) > 0 || len(structures.dependencies) > 0 ||
		len(structures.referenceSeries) > 0 || len(structures.exclusions) > 0 ||
		len(structures.referenceCatalogues) > 0
}

func (structures PricingPlanStructures) valid() bool {
	seenCodes := make(map[string]struct{}, len(structures.surchargeRules))
	for index, rule := range structures.surchargeRules {
		if !rule.valid() || rule.exclusivity == ExclusivityUndeclared {
			return false
		}
		if index > 0 && structures.surchargeRules[index-1].id >= rule.id {
			return false
		}
		if _, exists := seenCodes[rule.chargeCode.String()]; exists {
			return false
		}
		seenCodes[rule.chargeCode.String()] = struct{}{}
	}
	seenDependents := make(map[string]struct{}, len(structures.dependencies))
	for index, dependency := range structures.dependencies {
		if !dependency.valid() {
			return false
		}
		if index > 0 && structures.dependencies[index-1].id >= dependency.id {
			return false
		}
		if _, exists := seenDependents[dependency.dependent.String()]; exists {
			return false
		}
		seenDependents[dependency.dependent.String()] = struct{}{}
	}
	declaredBases := make(map[string]struct{}, len(structures.dependencies))
	for _, dependency := range structures.dependencies {
		declaredBases[dependency.id] = struct{}{}
	}
	boundAmountSeries := make(map[string]struct{}, len(structures.referenceSeries))
	for _, binding := range structures.referenceSeries {
		if binding.kind.carriesAmount() {
			boundAmountSeries[binding.seriesID] = struct{}{}
		}
	}
	for _, rule := range structures.surchargeRules {
		for _, basis := range rule.calculation.basisDependencyIDs() {
			if _, declared := declaredBases[basis]; !declared {
				return false
			}
		}
		for _, seriesID := range rule.calculation.amountSeriesIDs() {
			if _, bound := boundAmountSeries[seriesID]; !bound {
				return false
			}
		}
	}
	seenKinds := make(map[ReferenceSeriesKind]struct{}, len(structures.referenceSeries))
	for index, binding := range structures.referenceSeries {
		if !binding.valid() {
			return false
		}
		if index > 0 && compareSeriesBindings(structures.referenceSeries[index-1], binding) >= 0 {
			return false
		}
		// 费率序列由计算按种类引用，每种至多一条；金额序列由计算按序列标识引用，同种可绑多条（每分区一条，
		// ADR-0110 Decision 四），排序已保证（种类，标识）不重。
		if binding.kind.carriesAmount() {
			continue
		}
		if _, exists := seenKinds[binding.kind]; exists {
			return false
		}
		seenKinds[binding.kind] = struct{}{}
	}
	for index, rule := range structures.exclusions {
		if !rule.valid() {
			return false
		}
		if index > 0 && structures.exclusions[index-1].id >= rule.id {
			return false
		}
	}
	if structures.amountRounding != nil && !structures.amountRounding.valid() {
		return false
	}
	return validCatalogueLinks(structures.referenceCatalogues)
}

func trimmed(value string) bool {
	return strings.TrimSpace(value) != "" && strings.TrimSpace(value) == value
}
