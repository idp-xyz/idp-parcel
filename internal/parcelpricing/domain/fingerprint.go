package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

// canonicalVersionReference 只带身份三元（ADR-0108 Decision 二）：声明时附带的指纹不进摘要，
// 同一份引用带不带指纹，内容摘要与语义摘要逐字相同。
type canonicalVersionReference struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Version string `json:"version"`
}

func canonicalReference(reference VersionReference) canonicalVersionReference {
	return canonicalVersionReference{
		Kind:    string(reference.kind),
		ID:      reference.id,
		Version: reference.version,
	}
}

type canonicalMoney struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

func canonicalMoneyValue(money Money) canonicalMoney {
	return canonicalMoney{Amount: money.amount.String(), Currency: money.currency.String()}
}

type canonicalRateEntryDocument struct {
	ID      string         `json:"id"`
	Zone    string         `json:"zone"`
	Minimum string         `json:"minimum"`
	Maximum string         `json:"maximum"`
	Unit    string         `json:"unit"`
	Amount  canonicalMoney `json:"amount"`
}

func canonicalRateEntryValue(entry RateEntry) canonicalRateEntryDocument {
	return canonicalRateEntryDocument{
		ID:      entry.id.String(),
		Zone:    entry.zone,
		Minimum: entry.minimum.value.String(),
		Maximum: rateMaximumText(entry),
		Unit:    entry.minimum.unit.String(),
		Amount:  canonicalMoneyValue(entry.amount),
	}
}

type canonicalFirstContinueDocument struct {
	ID          string         `json:"id"`
	Zone        string         `json:"zone"`
	FirstWeight string         `json:"first_weight"`
	FirstAmount canonicalMoney `json:"first_amount"`
	Step        string         `json:"step"`
	StepAmount  canonicalMoney `json:"step_amount"`
	Unit        string         `json:"unit"`
}

func canonicalFirstContinueValue(rate FirstContinueRate) canonicalFirstContinueDocument {
	return canonicalFirstContinueDocument{
		ID:          rate.id.String(),
		Zone:        rate.zone,
		FirstWeight: rate.firstWeight.value.String(),
		FirstAmount: canonicalMoneyValue(rate.firstAmount),
		Step:        rate.step.value.String(),
		StepAmount:  canonicalMoneyValue(rate.stepAmount),
		Unit:        rate.firstWeight.unit.String(),
	}
}

type canonicalUnitPriceDocument struct {
	ID            string         `json:"id"`
	Zone          string         `json:"zone"`
	AmountPerUnit canonicalMoney `json:"amount_per_unit"`
}

func canonicalUnitPriceValue(rate UnitPriceRate) canonicalUnitPriceDocument {
	return canonicalUnitPriceDocument{
		ID:            rate.id.String(),
		Zone:          rate.zone,
		AmountPerUnit: canonicalMoneyValue(rate.amountPerUnit),
	}
}

type canonicalRateSelectionDocument struct {
	Family string         `json:"family"`
	ID     string         `json:"id"`
	Zone   string         `json:"zone"`
	Amount canonicalMoney `json:"amount"`
}

func canonicalRateSelectionValue(selection RateSelection) canonicalRateSelectionDocument {
	return canonicalRateSelectionDocument{
		Family: string(selection.family),
		ID:     selection.id.String(),
		Zone:   selection.zone,
		Amount: canonicalMoneyValue(selection.amount),
	}
}

type canonicalRateTableDocument struct {
	Reference     canonicalVersionReference        `json:"reference"`
	Family        string                           `json:"family"`
	Currency      string                           `json:"currency"`
	Unit          string                           `json:"unit"`
	Period        string                           `json:"period"`
	Entries       []canonicalRateEntryDocument     `json:"entries"`
	FirstContinue []canonicalFirstContinueDocument `json:"first_continue"`
	UnitPrice     []canonicalUnitPriceDocument     `json:"unit_price"`
}

func canonicalRateTableValue(table RateTableVersion) canonicalRateTableDocument {
	entries := make([]canonicalRateEntryDocument, 0, len(table.entries))
	for _, entry := range table.entries {
		entries = append(entries, canonicalRateEntryValue(entry))
	}
	firstContinue := make([]canonicalFirstContinueDocument, 0, len(table.firstContinue))
	for _, rate := range table.firstContinue {
		firstContinue = append(firstContinue, canonicalFirstContinueValue(rate))
	}
	unitPrice := make([]canonicalUnitPriceDocument, 0, len(table.unitPrice))
	for _, rate := range table.unitPrice {
		unitPrice = append(unitPrice, canonicalUnitPriceValue(rate))
	}
	return canonicalRateTableDocument{
		FirstContinue: firstContinue,
		UnitPrice:     unitPrice,
		Reference:     canonicalReference(table.reference),
		Family:        string(table.family),
		Currency:      table.currency.String(),
		Unit:          table.unit.String(),
		Period:        table.period.canonicalString(),
		Entries:       entries,
	}
}

type canonicalRoundingSegmentDocument struct {
	Mode      string `json:"rounding_mode"`
	Increment string `json:"increment"`
	Unit      string `json:"unit"`
	Maximum   string `json:"maximum,omitempty"`
}

func canonicalRoundingValue(policy WeightRoundingPolicy) []canonicalRoundingSegmentDocument {
	segments := make([]canonicalRoundingSegmentDocument, 0, len(policy.segments))
	for _, segment := range policy.segments {
		document := canonicalRoundingSegmentDocument{
			Mode:      string(segment.mode),
			Increment: segment.increment.value.String(),
			Unit:      segment.increment.unit.String(),
		}
		if segment.hasMaximum {
			document.Maximum = segment.maximum.value.String()
		}
		segments = append(segments, document)
	}
	return segments
}

type canonicalVolumetricFactorDocument struct {
	Divisor    string                             `json:"divisor"`
	LengthUnit string                             `json:"length_unit"`
	Rounding   []canonicalRoundingSegmentDocument `json:"rounding"`
}

func canonicalVolumetricFactorValue(factor VolumetricFactor) canonicalVolumetricFactorDocument {
	return canonicalVolumetricFactorDocument{
		Divisor:    factor.divisor.String(),
		LengthUnit: factor.lengthUnit.String(),
		Rounding:   canonicalRoundingValue(factor.rounding),
	}
}

type canonicalWeightPolicyDocument struct {
	Reference  canonicalVersionReference          `json:"reference"`
	Method     string                             `json:"method"`
	Rounding   []canonicalRoundingSegmentDocument `json:"rounding"`
	Volumetric *canonicalVolumetricFactorDocument `json:"volumetric_factor"`
}

func canonicalWeightPolicyValue(policy PricingWeightPolicy) canonicalWeightPolicyDocument {
	document := canonicalWeightPolicyDocument{
		Reference: canonicalReference(policy.reference),
		Method:    string(policy.method),
		Rounding:  canonicalRoundingValue(policy.rounding),
	}
	if policy.volumetric != nil {
		factor := canonicalVolumetricFactorValue(*policy.volumetric)
		document.Volumetric = &factor
	}
	return document
}

type canonicalChargeRuleDocument struct {
	ID          string         `json:"id"`
	Code        string         `json:"charge_code"`
	Description string         `json:"description"`
	Effect      string         `json:"effect"`
	Amount      canonicalMoney `json:"amount"`
	Order       int            `json:"order"`
	// 按件计收（ADR-0111）时才有；每主体一次的规则省略，逐包裹的卡字节不变。
	Unit string `json:"unit,omitempty"`
}

func canonicalChargeRuleValue(rule FixedChargeRule) canonicalChargeRuleDocument {
	return canonicalChargeRuleDocument{
		ID:          rule.id,
		Code:        rule.chargeCode.String(),
		Description: rule.description,
		Effect:      string(rule.effect),
		Amount:      canonicalMoneyValue(rule.amount),
		Order:       rule.order,
		Unit:        rule.unit.String(),
	}
}

type canonicalFeatureConditionDocument struct {
	Source    string `json:"source"`
	Operator  string `json:"operator"`
	Threshold string `json:"threshold"`
	Unit      string `json:"unit"`
}

type canonicalTriggerDocument struct {
	Kind      string                             `json:"kind"`
	Predicate *canonicalFeatureConditionDocument `json:"predicate"`
	Operands  []canonicalTriggerDocument         `json:"operands"`
}

func canonicalTriggerValue(trigger TriggerCondition) canonicalTriggerDocument {
	document := canonicalTriggerDocument{
		Kind:     trigger.kind.String(),
		Operands: make([]canonicalTriggerDocument, 0, len(trigger.operands)),
	}
	if trigger.kind == TriggerPredicate {
		predicate := canonicalFeatureConditionValue(trigger.predicate)
		document.Predicate = &predicate
	}
	for _, operand := range trigger.operands {
		document.Operands = append(document.Operands, canonicalTriggerValue(operand))
	}
	return document
}

func canonicalFeatureConditionValue(condition FeatureCondition) canonicalFeatureConditionDocument {
	// 通过判定条件本身读阈值，而不是直接取某一个字段：重量或体积阈值存放在另一个字段，
	// 一律去哈希长度字段，会让所有非长度类条件在摘要里都得到同一个空阈值。
	return canonicalFeatureConditionDocument{
		Source:    condition.source.String(),
		Operator:  condition.operator.String(),
		Threshold: condition.ThresholdValue().String(),
		Unit:      condition.ThresholdUnit(),
	}
}

type canonicalSurchargeCalculationDocument struct {
	Method     string                                  `json:"method"`
	Amount     *canonicalMoney                         `json:"amount"`
	Table      *canonicalRateTableDocument             `json:"table"`
	Percentage string                                  `json:"percentage"`
	Basis      string                                  `json:"basis"`
	Operands   []canonicalSurchargeCalculationDocument `json:"operands"`
	// 来自序列的费率，两半分别进入摘要。只哈希两者的乘积，会让一次费率变化与一次
	// 折扣变化互相抵消而无人察觉。
	SeriesKind   string `json:"series_kind,omitempty"`
	SeriesFactor string `json:"series_factor,omitempty"`
	// 取当期序列定额（ADR-0110）：序列标识与窗外行为两格都进摘要——只差窗外声明的两张卡收法不同。
	SeriesID    string `json:"series_id,omitempty"`
	OutOfWindow string `json:"out_of_window,omitempty"`
}

func canonicalSurchargeCalculationValue(calculation SurchargeCalculation) canonicalSurchargeCalculationDocument {
	document := canonicalSurchargeCalculationDocument{
		Method:      calculation.method.String(),
		Basis:       calculation.basis,
		Operands:    make([]canonicalSurchargeCalculationDocument, 0, len(calculation.operands)),
		SeriesID:    calculation.seriesID,
		OutOfWindow: calculation.outOfWindow.String(),
	}
	if calculation.amount != nil {
		amount := canonicalMoneyValue(*calculation.amount)
		document.Amount = &amount
	}
	if calculation.seriesKind != nil {
		document.SeriesKind = calculation.seriesKind.String()
	}
	if calculation.seriesFactor != nil {
		document.SeriesFactor = calculation.seriesFactor.String()
	}
	if calculation.table != nil {
		table := canonicalRateTableValue(*calculation.table)
		document.Table = &table
	}
	if calculation.percentage != nil {
		document.Percentage = calculation.percentage.String()
	}
	for _, operand := range calculation.operands {
		document.Operands = append(document.Operands, canonicalSurchargeCalculationValue(operand))
	}
	return document
}

type canonicalConditionalMinimumWeightDocument struct {
	ID        string                   `json:"id"`
	Condition canonicalTriggerDocument `json:"condition"`
	Minimum   string                   `json:"minimum"`
	Unit      string                   `json:"unit"`
}

func canonicalConditionalMinimumWeightValue(minimum ConditionalMinimumWeight) canonicalConditionalMinimumWeightDocument {
	return canonicalConditionalMinimumWeightDocument{
		ID:        minimum.id,
		Condition: canonicalTriggerValue(minimum.condition),
		Minimum:   minimum.minimum.value.String(),
		Unit:      minimum.minimum.unit.String(),
	}
}

type canonicalSurchargeRuleDocument struct {
	ID               string                                     `json:"id"`
	Code             string                                     `json:"charge_code"`
	Description      string                                     `json:"description"`
	Effect           string                                     `json:"effect"`
	Condition        canonicalTriggerDocument                   `json:"condition"`
	Calculation      canonicalSurchargeCalculationDocument      `json:"calculation"`
	Exclusivity      string                                     `json:"exclusivity"`
	ExclusivityGroup string                                     `json:"exclusivity_group"`
	Priority         int                                        `json:"priority"`
	MinimumWeight    *canonicalConditionalMinimumWeightDocument `json:"conditional_minimum_weight"`
	Unit             string                                     `json:"unit,omitempty"`
}

func canonicalSurchargeRuleValue(rule SurchargeRule) canonicalSurchargeRuleDocument {
	document := canonicalSurchargeRuleDocument{
		ID:               rule.id,
		Code:             rule.chargeCode.String(),
		Description:      rule.description,
		Effect:           string(rule.effect),
		Condition:        canonicalTriggerValue(rule.condition),
		Calculation:      canonicalSurchargeCalculationValue(rule.calculation),
		Exclusivity:      rule.exclusivity.String(),
		ExclusivityGroup: rule.exclusivityGroup,
		Priority:         rule.priority,
		Unit:             rule.unit.String(),
	}
	if rule.minimumWeight != nil {
		minimum := canonicalConditionalMinimumWeightValue(*rule.minimumWeight)
		document.MinimumWeight = &minimum
	}
	return document
}

type canonicalChargeDependencyDocument struct {
	ID          string   `json:"id"`
	Code        string   `json:"charge_code"`
	Composition string   `json:"composition"`
	Includes    []string `json:"includes"`
	Excludes    []string `json:"excludes"`
}

func canonicalChargeDependencyValue(dependency ChargeDependency) canonicalChargeDependencyDocument {
	document := canonicalChargeDependencyDocument{
		ID:          dependency.id,
		Code:        dependency.dependent.String(),
		Composition: dependency.composition.String(),
		Includes:    make([]string, 0, len(dependency.includes)),
		Excludes:    make([]string, 0, len(dependency.excludes)),
	}
	for _, code := range dependency.includes {
		document.Includes = append(document.Includes, code.String())
	}
	for _, code := range dependency.excludes {
		document.Excludes = append(document.Excludes, code.String())
	}
	return document
}

type canonicalExclusionRuleDocument struct {
	ID        string                   `json:"id"`
	Clause    string                   `json:"clause"`
	Condition canonicalTriggerDocument `json:"condition"`
}

func canonicalExclusionRuleValue(rule ExclusionRule) canonicalExclusionRuleDocument {
	return canonicalExclusionRuleDocument{
		ID:        rule.id,
		Clause:    rule.clause,
		Condition: canonicalTriggerValue(rule.condition),
	}
}

type canonicalReferenceSeriesDocument struct {
	Kind     string `json:"kind"`
	SeriesID string `json:"series_id"`
}

func canonicalReferenceSeriesValue(binding ReferenceSeriesBinding) canonicalReferenceSeriesDocument {
	return canonicalReferenceSeriesDocument{
		Kind:     binding.kind.String(),
		SeriesID: binding.seriesID,
	}
}

// canonicalizationVersion 标识内容摘要与评价语义摘要所依据的规范化文档形状。摘要只在
// 同一规范化版本内可比；拓宽形状必须递增这个值，而不是就地改写既有形状。见 ADR-0014。
// PPC-2 按规则模型设计的决定，一次拓宽了三处：价表新增首重加续重与计费重乘单价两个
// 价表族，取整策略变成分段列表，查表结果改为记录一次费率选择而不是一个档位。合并成
// 一次只花一个版本而不是三个，而每个版本都必须在「按它记录的评价还能重放」期间持续
// 支持。
// PPC-3 把规则的条件从单个判定条件改成触发条件，于是条件文档现在是「一个种类 + 可选
// 判定条件 + 操作数」，不再是一个裸判定条件。只读一个判定条件的规则会序列化成
// PREDICATE 触发条件，形状与 PPC-2 写出的裸判定条件不同；没有任何一种拓宽能让旧字节
// 保持原样。
// PPC-4 把序列绑定从「序列版本引用」改成「序列标识」（ADR-0099）：绑定文档由
// kind + reference 变为 kind + series_id，方案清单不再含序列版本引用。这不是拓宽而是
// 换对象——旧字节里那个版本引用在新形状下没有落点，PPC-3 快照因此在重建门被拒。
// PPC-5 是一次合并换号（ADR-0108 Decision 四援引 PPC-2 先例，同期实施的几份 ADR 只花一个
// 版本）：版本引用去掉 digest 只剩三元（ADR-0108）——它出现在方案引用、价表引用、重量策略
// 引用、清单与评价的事实引用、序列取值引用、换算步骤里，每一处的字节都变；随后同批落地的
// 拓宽（ADR-0107 金额取整策略、ADR-0109 计价参考目录绑定、ADR-0110 金额序列与取当期定额、
// ADR-0111 逐委托/逐主单聚合）都记在这一号下。换号发生在第一份改规范化文档的提交里而不是
// 最后一笔：不换号的中间态会让同一个版本号下存在两套字节，PPC-4 快照重算出来的摘要既不等于
// 原值又不会被版本门挡住。
const canonicalizationVersion = "PPC-5"

// CurrentCanonicalizationVersion 报出本构建按哪套形状做规范化。按其他取值记录的工件，
// 在这里无法重新算出其摘要。
func CurrentCanonicalizationVersion() string { return canonicalizationVersion }

type canonicalPricingPlan struct {
	Canonicalization string                              `json:"canonicalization"`
	Reference        canonicalVersionReference           `json:"reference"`
	Scope            string                              `json:"scope"`
	Direction        string                              `json:"direction"`
	Purpose          string                              `json:"purpose"`
	BaseCode         string                              `json:"base_charge_code"`
	Aggregation      string                              `json:"aggregation"`
	Period           string                              `json:"period"`
	RateTable        canonicalRateTableDocument          `json:"rate_table"`
	Weight           canonicalWeightPolicyDocument       `json:"weight_policy"`
	Rules            []canonicalChargeRuleDocument       `json:"rules"`
	SurchargeRules   []canonicalSurchargeRuleDocument    `json:"surcharge_rules"`
	Dependencies     []canonicalChargeDependencyDocument `json:"charge_dependencies"`
	ReferenceSeries  []canonicalReferenceSeriesDocument  `json:"reference_series"`
	Manifest         []canonicalVersionReference         `json:"manifest"`
	// 方案不拒收任何东西时省略该字段，使得所有在排除规则出现之前发布的方案规范化成
	// 相同的字节、摘要继续可比，而不必额外花掉一个规范化版本。
	Exclusions []canonicalExclusionRuleDocument `json:"exclusions,omitempty"`
	// 金额取整策略是价卡内容（ADR-0107 Decision 三），进内容摘要；未声明时省略——两张只差
	// 「声明了 / 没声明」的卡摘要必须不同，而没声明的卡彼此之间不该因这一格多出差异。
	AmountRounding *canonicalAmountRoundingDocument `json:"amount_rounding,omitempty"`
	// 目录绑定（ADR-0109 Decision 三、四）：绑了目录的卡与保留调用方给分区的卡在这一格分开；没绑的
	// 卡省略，理由同上一格。
	ReferenceCatalogues []canonicalReferenceCatalogueDocument `json:"reference_catalogues,omitempty"`
}

type canonicalReferenceCatalogueDocument struct {
	Kind        string `json:"kind"`
	CatalogueID string `json:"catalogue_id"`
}

func canonicalReferenceCatalogueValue(link ReferenceCatalogueLink) canonicalReferenceCatalogueDocument {
	return canonicalReferenceCatalogueDocument{Kind: link.kind.String(), CatalogueID: link.catalogueID}
}

type canonicalAmountRoundingDocument struct {
	Mode      string         `json:"mode"`
	Increment canonicalMoney `json:"increment"`
	Points    []string       `json:"points"`
}

func canonicalAmountRoundingValue(policy AmountRoundingPolicy) canonicalAmountRoundingDocument {
	points := make([]string, 0, len(policy.points))
	for _, point := range policy.points {
		points = append(points, point.String())
	}
	return canonicalAmountRoundingDocument{
		Mode:      string(policy.mode),
		Increment: canonicalMoneyValue(policy.increment),
		Points:    points,
	}
}

// canonicalAmountRoundingStep 是评价里一次取整的规范化形状：点、对象、模式、进位单位、前后值全进
// 语义摘要——只哈希取整后的合计，会让一次改模式与一次改进位单位互相抵消而无人察觉。
type canonicalAmountRoundingStep struct {
	Point     string         `json:"point"`
	Subject   string         `json:"subject,omitempty"`
	Mode      string         `json:"mode"`
	Increment canonicalMoney `json:"increment"`
	Before    canonicalMoney `json:"before"`
	After     canonicalMoney `json:"after"`
}

func canonicalAmountRoundingSteps(steps []AmountRoundingStep) []canonicalAmountRoundingStep {
	if len(steps) == 0 {
		return nil
	}
	documents := make([]canonicalAmountRoundingStep, 0, len(steps))
	for _, step := range steps {
		documents = append(documents, canonicalAmountRoundingStep{
			Point:     step.point.String(),
			Subject:   step.subject,
			Mode:      string(step.mode),
			Increment: canonicalMoneyValue(step.increment),
			Before:    canonicalMoneyValue(step.before),
			After:     canonicalMoneyValue(step.after),
		})
	}
	return documents
}

func calculatePricingPlanContentDigest(plan PricingPlanVersion) string {
	rules := make([]canonicalChargeRuleDocument, 0, len(plan.rules))
	for _, rule := range plan.rules {
		rules = append(rules, canonicalChargeRuleValue(rule))
	}
	surcharges := make([]canonicalSurchargeRuleDocument, 0, len(plan.structures.surchargeRules))
	for _, rule := range plan.structures.surchargeRules {
		surcharges = append(surcharges, canonicalSurchargeRuleValue(rule))
	}
	dependencies := make([]canonicalChargeDependencyDocument, 0, len(plan.structures.dependencies))
	for _, dependency := range plan.structures.dependencies {
		dependencies = append(dependencies, canonicalChargeDependencyValue(dependency))
	}
	series := make([]canonicalReferenceSeriesDocument, 0, len(plan.structures.referenceSeries))
	for _, binding := range plan.structures.referenceSeries {
		series = append(series, canonicalReferenceSeriesValue(binding))
	}
	manifest := make([]canonicalVersionReference, 0, len(plan.manifest.references))
	for _, reference := range plan.manifest.references {
		manifest = append(manifest, canonicalReference(reference))
	}
	var exclusions []canonicalExclusionRuleDocument
	for _, rule := range plan.structures.exclusions {
		exclusions = append(exclusions, canonicalExclusionRuleValue(rule))
	}
	var amountRounding *canonicalAmountRoundingDocument
	if plan.structures.amountRounding != nil {
		rounding := canonicalAmountRoundingValue(*plan.structures.amountRounding)
		amountRounding = &rounding
	}
	var catalogues []canonicalReferenceCatalogueDocument
	for _, link := range plan.structures.referenceCatalogues {
		catalogues = append(catalogues, canonicalReferenceCatalogueValue(link))
	}
	document := canonicalPricingPlan{
		Canonicalization:    canonicalizationVersion,
		Exclusions:          exclusions,
		AmountRounding:      amountRounding,
		ReferenceCatalogues: catalogues,
		Reference:           canonicalReference(plan.reference),
		Scope:               plan.scope.String(),
		Direction:           plan.direction.String(),
		Purpose:             plan.purpose.String(),
		BaseCode:            plan.baseChargeCode.String(),
		Aggregation:         string(plan.aggregation),
		Period:              plan.period.canonicalString(),
		RateTable:           canonicalRateTableValue(plan.rateTable),
		Weight:              canonicalWeightPolicyValue(plan.weight),
		Rules:               rules,
		SurchargeRules:      surcharges,
		Dependencies:        dependencies,
		ReferenceSeries:     series,
		Manifest:            manifest,
	}
	return hashCanonical(document)
}

type canonicalPricingWeightDocument struct {
	Method        string `json:"method"`
	Actual        string `json:"actual"`
	Volumetric    string `json:"volumetric,omitempty"`
	Raw           string `json:"raw"`
	Rounded       string `json:"rounded"`
	Unit          string `json:"unit"`
	RoundingMode  string `json:"rounding_mode"`
	Increment     string `json:"increment"`
	IncrementUnit string `json:"increment_unit"`
}

func canonicalPricingWeightValue(result PricingWeightResult) canonicalPricingWeightDocument {
	document := canonicalPricingWeightDocument{
		Method:        string(result.method),
		Actual:        result.actual.value.String(),
		Raw:           result.raw.value.String(),
		Rounded:       result.rounded.value.String(),
		Unit:          result.rounded.unit.String(),
		RoundingMode:  string(result.roundingMode),
		Increment:     result.increment.value.String(),
		IncrementUnit: result.increment.unit.String(),
	}
	if result.volumetric != nil {
		document.Volumetric = result.volumetric.value.String()
	}
	return document
}

type canonicalChargeLineDocument struct {
	ID          string         `json:"id"`
	Kind        string         `json:"kind"`
	Code        string         `json:"charge_code"`
	Scope       string         `json:"scope"`
	Basis       string         `json:"basis"`
	Method      string         `json:"method"`
	Description string         `json:"description"`
	Effect      string         `json:"effect"`
	Amount      canonicalMoney `json:"amount"`
	Order       int            `json:"order"`
	Source      string         `json:"source"`
}

func canonicalChargeLineValue(line ChargeLine) canonicalChargeLineDocument {
	return canonicalChargeLineDocument{
		ID:          line.id,
		Kind:        string(line.kind),
		Code:        line.chargeCode.String(),
		Scope:       string(line.scope),
		Basis:       string(line.basis),
		Method:      string(line.method),
		Description: line.description,
		Effect:      string(line.effect),
		Amount:      canonicalMoneyValue(line.amount),
		Order:       line.order,
		Source:      line.sourceRef,
	}
}

type canonicalDimensionsDocument struct {
	Longest  string `json:"longest"`
	Second   string `json:"second"`
	Shortest string `json:"shortest"`
	Unit     string `json:"unit"`
}

func canonicalDimensionsValue(dimensions Dimensions) canonicalDimensionsDocument {
	return canonicalDimensionsDocument{
		Longest:  dimensions.longest.String(),
		Second:   dimensions.second.String(),
		Shortest: dimensions.shortest.String(),
		Unit:     dimensions.unit.String(),
	}
}

type canonicalEvaluationInput struct {
	Tenant      string                       `json:"tenant"`
	Scope       string                       `json:"scope"`
	SubjectKind string                       `json:"subject_kind"`
	Subject     string                       `json:"subject"`
	Zone        string                       `json:"zone"`
	Actual      string                       `json:"actual"`
	Unit        string                       `json:"unit"`
	Dimensions  *canonicalDimensionsDocument `json:"dimensions,omitempty"`
	BusinessAt  string                       `json:"business_at"`
	Facts       []canonicalVersionReference  `json:"facts"`
	// 快照不带任何序列取值时省略该字段，使得所有在计价参考序列出现之前记录的评价
	// 规范化成相同的字节、摘要继续可比。
	Series []canonicalSeriesValueDocument `json:"series,omitempty"`
	// 邮编路线与目录读数（ADR-0109）同理省略：只给分区的评价字节不变。读数带「解出了 / 没解出」那一格，
	// 查过同一版而没查到的待判断，重放要落在同一个摘要上。
	Postal     *canonicalPostalRouteDocument       `json:"postal,omitempty"`
	Catalogues []canonicalCatalogueReadingDocument `json:"catalogues,omitempty"`
	// 成员清单（ADR-0111 Decision 二）进语义摘要：同一主单成员变了就是另一次评价。单包裹主体省略。
	Members *canonicalMemberManifestDocument `json:"members,omitempty"`
}

type canonicalMemberManifestDocument struct {
	Members         []string `json:"members"`
	TotalActual     string   `json:"total_actual"`
	TotalVolumetric string   `json:"total_volumetric,omitempty"`
	Unit            string   `json:"unit"`
}

func canonicalMemberManifestValue(manifest MemberManifest) canonicalMemberManifestDocument {
	document := canonicalMemberManifestDocument{
		Members:     make([]string, 0, len(manifest.members)),
		TotalActual: manifest.totalActual.value.String(),
		Unit:        manifest.totalActual.unit.String(),
	}
	for _, member := range manifest.members {
		document.Members = append(document.Members, member.String())
	}
	if manifest.totalVolumetric != nil {
		document.TotalVolumetric = manifest.totalVolumetric.value.String()
	}
	return document
}

type canonicalPostalRouteDocument struct {
	Origin      string `json:"origin,omitempty"`
	Destination string `json:"destination"`
}

type canonicalCatalogueReadingDocument struct {
	Kind      string                    `json:"kind"`
	Reference canonicalVersionReference `json:"reference"`
	Value     string                    `json:"value,omitempty"`
	Resolved  bool                      `json:"resolved"`
}

func canonicalCatalogueReadings(readings []ResolvedCatalogueValue) []canonicalCatalogueReadingDocument {
	if len(readings) == 0 {
		return nil
	}
	documents := make([]canonicalCatalogueReadingDocument, 0, len(readings))
	for _, reading := range readings {
		documents = append(documents, canonicalCatalogueReadingDocument{
			Kind:      reading.kind.String(),
			Reference: canonicalReference(reading.reference),
			Value:     string(reading.value),
			Resolved:  reading.resolved,
		})
	}
	return documents
}

type canonicalSeriesValueDocument struct {
	Kind       string                     `json:"kind"`
	Reference  canonicalVersionReference  `json:"reference"`
	Value      string                     `json:"value"`
	QuoteBasis *canonicalVersionReference `json:"quote_basis,omitempty"`
	// 金额序列的币种与「窗外无期次」两格（ADR-0110）；费率序列的读数字节不变。
	Currency string `json:"currency,omitempty"`
	Absent   bool   `json:"absent,omitempty"`
}

// canonicalConversionDocument 把一次换算的两侧都留在摘要里。只哈希换算后的数字，
// 会让原币金额与汇率一同变化、互相抵消而无人察觉。
type canonicalConversionDocument struct {
	Original  canonicalMoney            `json:"original"`
	Rate      string                    `json:"rate"`
	Series    canonicalVersionReference `json:"series"`
	Converted canonicalMoney            `json:"converted"`
}

func canonicalConversionValue(step ConversionStep) canonicalConversionDocument {
	return canonicalConversionDocument{
		Original:  canonicalMoneyValue(step.original),
		Rate:      step.rate.String(),
		Series:    canonicalReference(step.series),
		Converted: canonicalMoneyValue(step.converted),
	}
}

func canonicalSeriesValues(values []ReferenceSeriesValue) []canonicalSeriesValueDocument {
	if len(values) == 0 {
		return nil
	}
	documents := make([]canonicalSeriesValueDocument, 0, len(values))
	for _, value := range values {
		document := canonicalSeriesValueDocument{
			Kind:      value.kind.String(),
			Reference: canonicalReference(value.reference),
			Value:     value.value.String(),
			Absent:    value.absent,
		}
		if value.currency != nil {
			document.Currency = value.currency.String()
		}
		if value.quoteBasis != nil {
			basis := canonicalReference(*value.quoteBasis)
			document.QuoteBasis = &basis
		}
		documents = append(documents, document)
	}
	return documents
}

type canonicalEvaluation struct {
	Canonicalization string                          `json:"canonicalization"`
	NumericProfile   string                          `json:"numeric_profile"`
	Status           string                          `json:"status"`
	Input            canonicalEvaluationInput        `json:"input"`
	Direction        string                          `json:"direction"`
	Purpose          string                          `json:"purpose"`
	PlanReference    canonicalVersionReference       `json:"plan_reference"`
	PlanPeriod       string                          `json:"plan_period"`
	TablePeriod      string                          `json:"table_period"`
	PlanContent      string                          `json:"plan_content"`
	Manifest         []canonicalVersionReference     `json:"manifest"`
	PricingWeight    *canonicalPricingWeightDocument `json:"pricing_weight,omitempty"`
	MatchedRate      *canonicalRateSelectionDocument `json:"matched_rate,omitempty"`
	Conversion       *canonicalConversionDocument    `json:"conversion,omitempty"`
	ChargeLines      []canonicalChargeLineDocument   `json:"charge_lines"`
	// 没有任何一次取整时省略：未声明策略的卡与 ADR-0107 之前的评价在这一格上字节相同。
	AmountRounding []canonicalAmountRoundingStep `json:"amount_rounding,omitempty"`
	Total          *canonicalMoney               `json:"total,omitempty"`
	Issues         []canonicalEvaluationIssue    `json:"issues"`
}

type canonicalEvaluationIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func canonicalEvaluationIssueValue(issue EvaluationIssue) canonicalEvaluationIssue {
	return canonicalEvaluationIssue{Code: issue.code, Message: issue.message}
}

func hashPricingEvaluation(evaluation PricingEvaluation) string {
	facts := make([]canonicalVersionReference, 0, len(evaluation.input.factReferences))
	for _, fact := range evaluation.input.factReferences {
		facts = append(facts, canonicalReference(fact.reference))
	}
	sort.SliceStable(facts, func(left, right int) bool {
		return compareCanonicalReferences(facts[left], facts[right]) < 0
	})
	input := canonicalEvaluationInput{
		Tenant:      evaluation.input.tenantID.String(),
		Scope:       evaluation.input.scope.String(),
		SubjectKind: evaluation.input.subject.kind.String(),
		Subject:     evaluation.input.subject.id,
		Zone:        evaluation.input.zone,
		Actual:      evaluation.input.actualWeight.value.String(),
		Unit:        evaluation.input.actualWeight.unit.String(),
		BusinessAt:  evaluation.input.businessAt.UTC().Format(time.RFC3339Nano),
		Facts:       facts,
		Series:      canonicalSeriesValues(evaluation.input.seriesValues),
		Catalogues:  canonicalCatalogueReadings(evaluation.input.catalogueReadings),
	}
	if route, declared := evaluation.input.PostalRoute(); declared {
		input.Postal = &canonicalPostalRouteDocument{Origin: route.origin, Destination: route.destination}
	}
	if manifest, declared := evaluation.input.Members(); declared {
		members := canonicalMemberManifestValue(manifest)
		input.Members = &members
	}
	if sides, ok := evaluation.input.Dimensions(); ok {
		declared := canonicalDimensionsValue(sides)
		input.Dimensions = &declared
	}
	manifest := make([]canonicalVersionReference, 0, len(evaluation.manifest.references))
	for _, reference := range evaluation.manifest.references {
		manifest = append(manifest, canonicalReference(reference))
	}
	sort.SliceStable(manifest, func(left, right int) bool {
		return compareCanonicalReferences(manifest[left], manifest[right]) < 0
	})
	document := canonicalEvaluation{
		Canonicalization: canonicalizationVersion,
		NumericProfile:   "decimal-bigint-v1",
		Status:           string(evaluation.status),
		Input:            input,
		Direction:        evaluation.direction.String(),
		Purpose:          evaluation.purpose.String(),
		PlanReference:    canonicalReference(evaluation.planReference),
		PlanPeriod:       evaluation.planPeriod.canonicalString(),
		TablePeriod:      evaluation.tablePeriod.canonicalString(),
		PlanContent:      evaluation.planContentDigest,
		Manifest:         manifest,
		AmountRounding:   canonicalAmountRoundingSteps(evaluation.amountRounding),
		Issues:           make([]canonicalEvaluationIssue, 0, len(evaluation.issues)),
	}
	for _, issue := range evaluation.issues {
		document.Issues = append(document.Issues, canonicalEvaluationIssueValue(issue))
	}
	if evaluation.pricingWeight != nil {
		weightDocument := canonicalPricingWeightValue(*evaluation.pricingWeight)
		document.PricingWeight = &weightDocument
	}
	if evaluation.matchedRate != nil {
		matched := canonicalRateSelectionValue(*evaluation.matchedRate)
		document.MatchedRate = &matched
	}
	if evaluation.conversion != nil {
		conversion := canonicalConversionValue(*evaluation.conversion)
		document.Conversion = &conversion
	}
	for _, line := range evaluation.chargeLines {
		document.ChargeLines = append(document.ChargeLines, canonicalChargeLineValue(line))
	}
	if evaluation.total != nil {
		total := canonicalMoneyValue(*evaluation.total)
		document.Total = &total
	}
	return hashCanonical(document)
}

func hashCanonical(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func compareCanonicalReferences(left, right canonicalVersionReference) int {
	for _, pair := range [][2]string{
		{left.Kind, right.Kind},
		{left.ID, right.ID},
		{left.Version, right.Version},
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
