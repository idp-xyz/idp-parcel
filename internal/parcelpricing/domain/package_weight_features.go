package domain

import (
	"errors"
	"fmt"
)

// 本文件管判定条件读「体积重」与「自身计价重量」这两个派生特征时，方案装配与评价各自要做的事（ADR-0347）。
//
// 两个量都由包裹自己的实重、尺寸与卡的重量策略决定，不取决于任何一条规则命不命中，所以评价器可以在判定任何条件
// 之前一次算出来填进 PackageFeatures。自身计价重量是「派生、进位」的结果，**不含条件最低计价重量的抬高**——
// 抬高由判定条件触发，条件读到抬高的结果就成了自己读自己；两者是同一条派生链的前后两段，特征取前一段。

// reads 报出触发条件的任一叶子是否读了给定的特征来源。
func (trigger TriggerCondition) reads(source FeatureSource) bool {
	if trigger.kind == TriggerPredicate {
		return trigger.predicate.source == source
	}
	for _, operand := range trigger.operands {
		if operand.reads(source) {
			return true
		}
	}
	return false
}

// collectPredicates 把触发条件的全部叶子按声明先后收进 into。
func (trigger TriggerCondition) collectPredicates(into []FeatureCondition) []FeatureCondition {
	if trigger.kind == TriggerPredicate {
		return append(into, trigger.predicate)
	}
	for _, operand := range trigger.operands {
		into = operand.collectPredicates(into)
	}
	return into
}

// triggers 收齐方案里全部的触发条件：拒收条款、附加费规则的触发条件与条件最低计价重量的触发条件。
func (structures PricingPlanStructures) triggers() []TriggerCondition {
	var triggers []TriggerCondition
	for _, rule := range structures.exclusions {
		triggers = append(triggers, rule.condition)
	}
	for _, rule := range structures.surchargeRules {
		triggers = append(triggers, rule.condition)
		if rule.minimumWeight != nil {
			triggers = append(triggers, rule.minimumWeight.condition)
		}
	}
	return triggers
}

// ReadsFeature 报出方案的任一判定条件是否读了给定的特征来源。编排据此在发起评价之前就知道这张卡要发起方声明哪一格
// （例如读地址性质的卡要地址性质），缺了点名缺项，而不是等评价落待判断。
func (structures PricingPlanStructures) ReadsFeature(source FeatureSource) bool {
	for _, trigger := range structures.triggers() {
		if trigger.reads(source) {
			return true
		}
	}
	return false
}

// packageWeightPredicates 收齐方案里读体积重或自身计价重量的判定条件。
func (structures PricingPlanStructures) packageWeightPredicates() []FeatureCondition {
	var predicates []FeatureCondition
	for _, trigger := range structures.triggers() {
		for _, predicate := range trigger.collectPredicates(nil) {
			if predicate.source == FeatureVolumetricWeight || predicate.source == FeatureChargeableWeight {
				predicates = append(predicates, predicate)
			}
		}
	}
	return predicates
}

// readsPackageWeights 报出方案是否有判定条件读体积重或自身计价重量。评价只在这时多算一遍包裹自身的重量：
// 没有这样的条件，评价与引入这两个特征之前走同一条路径、逐字节相同。
func (structures PricingPlanStructures) readsPackageWeights() bool {
	return len(structures.packageWeightPredicates()) > 0
}

// volumetricThresholdScale 报出读体积重的判定条件里阈值最细的小数位。体积重在票级 / 主单级主体上是调用方声明的
// 合计体积重，单位与价表不同时会被换算并可能截断；阈值是拿来跟它比较的有限小数，必须并入截断位数的边界（ADR-0346
// 越权风险点 2 要求日后新增的比较量都并入），否则「截断不改变任何比较结果」的论证对这些阈值不成立。
func (structures PricingPlanStructures) volumetricThresholdScale() uint32 {
	var scale uint32
	for _, predicate := range structures.packageWeightPredicates() {
		if predicate.source == FeatureVolumetricWeight && predicate.weightThreshold.value.scale > scale {
			scale = predicate.weightThreshold.value.scale
		}
	}
	return scale
}

// checkPackageWeightReads 在方案装配时判读体积重与自身计价重量的条件立不立得住。三件事都是「对每一件包裹必然
// 同一种结果」的声明错误，放到评价期只会在每次评价上重复失败：
//
//   - 体积重要卡声明了体积系数：未声明系数的卡（只按实重计价）根本算不出体积重；
//   - 两个量都落在价表的重量单位上，阈值须用同一个单位（特征判定不换算阈值，ADR-0346 决定六）；
//   - 条件最低计价重量的触发条件不读自身计价重量，那一道在 NewConditionalMinimumWeight 上，不在这里。
func (structures PricingPlanStructures) checkPackageWeightReads(weight PricingWeightPolicy, tableUnit WeightUnit) error {
	for _, predicate := range structures.packageWeightPredicates() {
		if predicate.source == FeatureVolumetricWeight && weight.volumetric == nil {
			return fmt.Errorf("%w: %s is read by a condition but the weight policy declares no volumetric factor", ErrInvalidPricingPlan, predicate.source)
		}
		if predicate.weightThreshold.unit != tableUnit {
			return fmt.Errorf("%w: %s is measured in the rate table unit %s, its threshold is declared in %s",
				ErrWeightUnitMismatch, predicate.source, tableUnit, predicate.weightThreshold.unit)
		}
	}
	return nil
}

// ErrConditionalMinimumReadsChargeableWeight 表示条件最低计价重量的触发条件读了自身计价重量。自身计价重量不含抬高，
// 所以这里并没有环；拒它是保守的取舍，理由与可逆性见 ADR-0347 决定三。
var ErrConditionalMinimumReadsChargeableWeight = fmt.Errorf("%w: a conditional minimum weight cannot be triggered by the chargeable weight feature", ErrInvalidSurchargeRule)

// packageOwnWeights 按卡的重量策略算包裹自身的体积重与自身计价重量（不含任何条件最低计价重量的抬高）。
// 与正式计价重量同一件实现：先按不换算的老路算，老路答了单位不一致才带着换算器再算（ADR-0346 决定二）；换算记录
// 一并交回，供本次失败时留在评价上——成功时正式计价重量那一遍会记下同样的换算，这里不重复记。
func packageOwnWeights(request EvaluationRequest) (PricingWeightResult, []UnitConversionStep, error) {
	result, err := CalculatePricingWeight(request.input, request.plan.weight, request.plan.rateTable.unit)
	if errors.Is(err, ErrWeightUnitMismatch) || errors.Is(err, ErrLengthUnitMismatch) {
		converter := newUnitConverter(request.conversionsFrozen, request.recordedConversions)
		converter.conditionScale = request.plan.structures.volumetricThresholdScale()
		result, err = calculatePricingWeight(request.input, request.plan.weight, request.plan.rateTable.unit, converter)
		return result, converter.recordedSteps(), err
	}
	return result, nil, err
}

// explainOwnWeights 写下条件读到的那两个值，使未命中的规则也说得出它拿什么比：这两个值在费用行与正式计价重量
// 里都不单独出现（后者可能已被条件最低计价重量抬高）。
func (result PricingWeightResult) explainOwnWeights() string {
	volumetric := "not declared"
	if result.volumetric != nil {
		volumetric = result.volumetric.value.String() + " " + string(result.volumetric.unit)
	}
	return fmt.Sprintf("package weights read by conditions, before any conditional minimum: volumetric=%s; chargeable=%s %s",
		volumetric, result.rounded.value.String(), result.rounded.unit)
}

// withoutLines 从解释里各撤掉 remove 中每一行的第一处，其余原样。
func withoutLines(explanation []string, remove []string) []string {
	if len(remove) == 0 {
		return explanation
	}
	pending := append([]string(nil), remove...)
	kept := make([]string, 0, len(explanation))
	for _, line := range explanation {
		dropped := false
		for index, candidate := range pending {
			if candidate == line {
				pending = append(pending[:index], pending[index+1:]...)
				dropped = true
				break
			}
		}
		if !dropped {
			kept = append(kept, line)
		}
	}
	return kept
}
