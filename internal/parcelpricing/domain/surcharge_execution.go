package domain

import (
	"errors"
	"fmt"
	"sort"
)

// surchargeOutcome 记录一条已声明规则的处理结果，包括那些未命中的规则。CONTEXT 要求
// 未命中也要留痕：只列命中结果的解释无法对着卡逐条核对，因为读的人分不清一条规则是
// 未命中，还是根本没被判定过。
type surchargeOutcome struct {
	rule     SurchargeRule
	matched  bool
	selected bool
	deferred bool
	amount   Money
	note     string
	// source 是金额的来源说明（取当期序列定额时为「取自序列 X@V」），随解释输出。
	source string
}

// unexecutable 指出本构建无法计价的第一项已声明结构。
//
// 这道门存在的目的，是不让一个声明了无人执行的费用的方案只按基础价表计价。它随每一项
// 能力落地而收窄，现在**已经恒不成立**：每一项已声明结构都有执行器。保留而不删除，是
// 因为 default 分支正是用来抓「新增了一种计算方法却没有配执行器」的——它挡住的失败是
// 静默少收，而没有别的检查会注意到这件事。
func (structures PricingPlanStructures) unexecutable() (string, bool) {
	for _, rule := range structures.surchargeRules {
		switch rule.calculation.method {
		case ChargeMethodFixedAmount, ChargeMethodTableLookup, ChargeMethodPercentOfBasis, ChargeMethodGreaterOf, ChargeMethodSeriesAmount:
		default:
			return fmt.Sprintf("%s calculation on %s", rule.calculation.method, rule.id), true
		}
	}
	return "", false
}

// minimumRaise 是一条已触发的条件最低计价重量，连同它所属的规则一起保留，
// 使解释能指名是哪一条条款，而不是只给出一个数字。
type minimumRaise struct {
	id      string
	minimum Weight
}

// resolveMinimums 报出每一条条款成立的条件最低计价重量。该条款写在某条附加费规则内部，
// 抬高的却是方案级计价重量，所以它在计价重量定下来之前解析，而不是跟着声明它的那条
// 附加费一起处理。
func (structures PricingPlanStructures) resolveMinimums(features PackageFeatures) ([]minimumRaise, error) {
	raises := make([]minimumRaise, 0, len(structures.surchargeRules))
	for _, rule := range structures.surchargeRules {
		if rule.minimumWeight == nil {
			continue
		}
		held, err := rule.minimumWeight.condition.Matches(features)
		if err != nil {
			return nil, err
		}
		if held {
			raises = append(raises, minimumRaise{id: rule.minimumWeight.id, minimum: rule.minimumWeight.minimum})
		}
	}
	return raises, nil
}

// highestMinimum 选出要施加的下限。CONTEXT：同一评价可存在多条，同时触发时取其中最高
// 者——卡上有两条，40 LB 和 90 LB，同时触发两条的包裹按 90 计。
func highestMinimum(raises []minimumRaise) (minimumRaise, bool) {
	var highest minimumRaise
	found := false
	for _, raise := range raises {
		if !found || raise.minimum.value.Cmp(highest.minimum.value) > 0 {
			highest, found = raise, true
		}
	}
	return highest, found
}

// surchargeContext 是一条规则在包裹自身特征之外可以读到的东西：包裹落在哪个分区，
// 以及基础运费使用的计价重量。分档附加费读的是与基础价表同一个重量，两者因此不可能
// 对「这件包裹有多重」产生分歧。
type surchargeContext struct {
	features      PackageFeatures
	zone          string
	pricingWeight Weight
	series        map[ReferenceSeriesKind]ReferenceSeriesValue
	// amounts 是金额序列的读数，按序列标识索引（ADR-0110）；窗外无期次的读数也在这里，值缺席。
	amounts map[string]ReferenceSeriesValue
}

// errSeriesAmountOutOfWindow 在 resolve 内部标记「金额序列窗外且卡声明不计收」：这条规则不形成费用行，也不是
// 任何一种失败。它不出 resolveSurcharges——那里把它译成未计收的结果并留痕。
var errSeriesAmountOutOfWindow = errors.New("parcel pricing: series amount out of window, not charged")

// resolveSurcharges 拿包裹特征逐条判定所有已声明规则，再施加卡上的相互作用规则。
// 独立计收的规则全部收取；同一互斥组内的规则相互竞争，组内至多计收一条。
func (structures PricingPlanStructures) resolveSurcharges(reading surchargeContext) ([]surchargeOutcome, error) {
	outcomes := make([]surchargeOutcome, 0, len(structures.surchargeRules))
	for _, rule := range structures.surchargeRules {
		matched, err := rule.condition.Matches(reading.features)
		if err != nil {
			return nil, err
		}
		outcome := surchargeOutcome{rule: rule, matched: matched}
		if matched {
			// 读取基数的费用此刻还定不了值：基数要对仍在收集中的费用行求和。
			// 它留到第二遍、按依赖顺序计价。
			if rule.calculation.needsBasis() {
				outcome.deferred = true
				outcome.selected = true
			} else {
				amount, err := rule.calculation.resolve(reading, nil)
				switch {
				case errors.Is(err, errSeriesAmountOutOfWindow):
					// 窗外不计收（ADR-0110 Decision 三）：条件成立了，卡说这期不收——留痕但不成行、不进互斥竞争。
					outcome.selected = false
					outcome.note = fmt.Sprintf("series amount out of window; the card declares %s", OutOfWindowNotCharged)
				case err != nil:
					return nil, err
				default:
					outcome.amount = amount
					outcome.selected = true
					outcome.source = rule.calculation.describeAmountSource(reading.amounts)
				}
			}
		}
		outcomes = append(outcomes, outcome)
	}
	if err := selectWithinExclusivityGroups(outcomes); err != nil {
		return nil, err
	}
	return outcomes, nil
}

// needsBasis 报出给这项计算定值是否需要仍在收集中的费用行。取较大值从任一操作数继承
// 这个需求：拿一个定额下限去和一个尚未解出的百分比比较，结果永远是取那个下限。
func (calculation SurchargeCalculation) needsBasis() bool {
	switch calculation.method {
	case ChargeMethodPercentOfBasis:
		return true
	case ChargeMethodGreaterOf:
		for _, operand := range calculation.operands {
			if operand.needsBasis() {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// resolve 产出一条已命中规则计收的金额。分档价表里的区间空档是卡的空档，不是规则未
// 命中——触发条件确实成立了——所以查表错误原样向外传递，评价保持等待。第一遍传入的
// basis 为 nil，那一遍只给不读取基数的计算定值。
func (calculation SurchargeCalculation) resolve(reading surchargeContext, basis *dependencyBasis) (Money, error) {
	switch calculation.method {
	case ChargeMethodFixedAmount:
		amount, ok := calculation.FixedAmount()
		if !ok {
			return Money{}, ErrInvalidSurchargeRule
		}
		return amount, nil
	case ChargeMethodTableLookup:
		table, ok := calculation.LookupTable()
		if !ok {
			return Money{}, ErrInvalidSurchargeRule
		}
		selection, err := table.Lookup(reading.zone, reading.pricingWeight)
		if err != nil {
			return Money{}, err
		}
		return selection.amount, nil
	case ChargeMethodPercentOfBasis:
		if basis == nil {
			return Money{}, fmt.Errorf("%w: %s valued before its basis", ErrPlanStructuresNotExecutable, calculation.method)
		}
		return basis.share(calculation, reading.series)
	case ChargeMethodSeriesAmount:
		published, found := reading.amounts[calculation.seriesID]
		if !found {
			// 绑定门保证卡绑了这条序列，resolveSeries 保证有读数；到这里没有只可能是结构坏了。
			return Money{}, fmt.Errorf("%w: amount series %s has no reading", ErrPlanStructuresNotExecutable, calculation.seriesID)
		}
		if published.absent {
			switch calculation.outOfWindow {
			case OutOfWindowNotCharged:
				return Money{}, errSeriesAmountOutOfWindow
			default:
				// 带上绑定：问题项的「涉及序列」主体（ADR-0105）从这里取种类与标识。
				return Money{}, &missingSeriesReadingError{
					binding: ReferenceSeriesBinding{kind: ReferenceSeriesPublishedAmount, seriesID: calculation.seriesID},
					message: fmt.Sprintf("%s: amount series %s in-force version %s has no period at the pricing basis time and the card declares %s",
						ErrMissingReferenceSeriesValue.Error(), calculation.seriesID, published.reference.Version(), calculation.outOfWindow),
				}
			}
		}
		amount, ok := published.Amount()
		if !ok {
			return Money{}, ErrInvalidSurchargeRule
		}
		return amount, nil
	case ChargeMethodGreaterOf:
		var best Money
		compared := false
		for _, operand := range calculation.operands {
			amount, err := operand.resolve(reading, basis)
			// 窗外不计收的操作数不参与比较：卡说这期那一项不收，另一项照常成立；两项都不收才整条不收。
			if errors.Is(err, errSeriesAmountOutOfWindow) {
				continue
			}
			if err != nil {
				return Money{}, err
			}
			if !compared || amount.amount.Cmp(best.amount) > 0 {
				best, compared = amount, true
			}
		}
		if !compared {
			return Money{}, errSeriesAmountOutOfWindow
		}
		if !best.valid() {
			return Money{}, ErrInvalidSurchargeRule
		}
		return best, nil
	default:
		return Money{}, fmt.Errorf("%w: %s", ErrPlanStructuresNotExecutable, calculation.method)
	}
}

// selectWithinExclusivityGroups 每组只留一条。
//
// 先按声明优先级、后按金额定先后。**优先级 1 最高**：优先级声明为从 1 起的正序数，
// 所以序号最靠前的那一级胜出。两者的先后次序要紧——某一组里优先级最高的那条金额反而
// 更小时，仍必须计收较小的那个金额，而单纯「取最大」会算错。
func selectWithinExclusivityGroups(outcomes []surchargeOutcome) error {
	best := make(map[string]int, len(outcomes))
	for index := range outcomes {
		outcome := &outcomes[index]
		if !outcome.matched || outcome.rule.exclusivity != ExclusivityGrouped {
			continue
		}
		group := outcome.rule.exclusivityGroup
		incumbent, seen := best[group]
		if !seen {
			best[group] = index
			continue
		}
		preferred, err := preferSurcharge(outcomes[incumbent], *outcome)
		if err != nil {
			return err
		}
		if preferred {
			outcomes[index].selected = false
			outcomes[index].note = fmt.Sprintf("suppressed by %s in exclusivity group %s", outcomes[incumbent].rule.id, group)
			continue
		}
		outcomes[incumbent].selected = false
		outcomes[incumbent].note = fmt.Sprintf("suppressed by %s in exclusivity group %s", outcome.rule.id, group)
		best[group] = index
	}
	return nil
}

// preferSurcharge 报出在位的那条是否保住该组。同级且金额相同的两条规则无从分辨，
// 而 CONTEXT 禁止任选，因此那是一个要交给人裁决的`冲突`。
func preferSurcharge(incumbent, challenger surchargeOutcome) (bool, error) {
	if incumbent.rule.priority != challenger.rule.priority {
		return incumbent.rule.priority < challenger.rule.priority, nil
	}
	comparison := incumbent.amount.amount.Cmp(challenger.amount.amount)
	if comparison == 0 {
		return false, fmt.Errorf("%w: %s and %s share rank %d and amount in group %s",
			ErrRateTableConflict, incumbent.rule.id, challenger.rule.id, incumbent.rule.priority, incumbent.rule.exclusivityGroup)
	}
	return comparison > 0, nil
}

// explain 为每条规则输出一行，命中与否都输出，使解释能对着卡逐条读下来。取当期序列定额的规则写明金额取自
// 哪条序列哪一版（ADR-0110 Consequences「取自序列 X 第 N 期」那一格）。
func (outcome surchargeOutcome) explain() string {
	switch {
	case !outcome.matched:
		return fmt.Sprintf("surcharge %s did not apply: %s not met",
			outcome.rule.id, outcome.rule.condition.describe())
	case !outcome.selected:
		return fmt.Sprintf("surcharge %s applied but was not collected: %s", outcome.rule.id, outcome.note)
	default:
		return fmt.Sprintf("surcharge %s applied for %s%s", outcome.rule.id, outcome.amount.amount.String(), outcome.source)
	}
}

// sortedSurchargeOutcomes 让费用行与解释保持确定的顺序，不受上面 map 遍历顺序影响。
func sortedSurchargeOutcomes(outcomes []surchargeOutcome) []surchargeOutcome {
	sorted := append([]surchargeOutcome(nil), outcomes...)
	sort.SliceStable(sorted, func(left, right int) bool {
		return sorted[left].rule.id < sorted[right].rule.id
	})
	return sorted
}
