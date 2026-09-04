package domain

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrPricingPlanSnapshotInvalid 表示快照解不出一张立得住的价卡——坏写入在重建处
// 暴露，而不是变成一个看起来合法的可执行工件。
var ErrPricingPlanSnapshotInvalid = errors.New("parcel pricing: invalid pricing plan snapshot")

// 本文件是价卡（定价方案版本）的持久化快照口，与评价快照同一条纪律：序列化形状留在
// 领域而不是适配器——方案是几十个值对象组成的闭合规则工件，字段全部未导出，在适配器
// 里复刻整张图等于为同一形状立第二个口径。重建后必过 plan.valid()：它逐层重验价表
// 区间、币种/单位一致、结构件有序性，并按当前规范化版本重算内容摘要自校，这正是
// CONTEXT「版本引用相同、规范化版本也相同而内容摘要不同，视为版本内容冲突」在读回
// 门上的落点。
//
// 快照携带产生它的规范化版本。规范化版本不是当前构建支持的那一个时，重算摘要在结构
// 上不可能（ADR-0014），重建拒绝而不是硬算——那是「结构上算不出来」，不是版本内容
// 冲突。

type lengthThresholdSnapshot struct {
	Value decimalSnapshot `json:"value"`
	Unit  string          `json:"unit"`
}

type volumeThresholdSnapshot struct {
	Value decimalSnapshot `json:"value"`
	Unit  string          `json:"unit"`
}

type featureConditionSnapshot struct {
	Source          string                   `json:"source"`
	Operator        string                   `json:"operator"`
	LengthThreshold *lengthThresholdSnapshot `json:"lengthThreshold,omitempty"`
	VolumeThreshold *volumeThresholdSnapshot `json:"volumeThreshold,omitempty"`
	WeightThreshold *weightSnapshot          `json:"weightThreshold,omitempty"`
	Category        string                   `json:"category,omitempty"`
}

type triggerConditionSnapshot struct {
	Kind      string                     `json:"kind"`
	Predicate *featureConditionSnapshot  `json:"predicate,omitempty"`
	Operands  []triggerConditionSnapshot `json:"operands,omitempty"`
}

type rateEntrySnapshot struct {
	ID      string          `json:"id"`
	Zone    string          `json:"zone"`
	Minimum weightSnapshot  `json:"minimum"`
	Maximum *weightSnapshot `json:"maximum,omitempty"`
	Amount  moneySnapshot   `json:"amount"`
}

type firstContinueRateSnapshot struct {
	ID          string         `json:"id"`
	Zone        string         `json:"zone"`
	FirstWeight weightSnapshot `json:"firstWeight"`
	FirstAmount moneySnapshot  `json:"firstAmount"`
	Step        weightSnapshot `json:"step"`
	StepAmount  moneySnapshot  `json:"stepAmount"`
}

type unitPriceRateSnapshot struct {
	ID            string        `json:"id"`
	Zone          string        `json:"zone"`
	AmountPerUnit moneySnapshot `json:"amountPerUnit"`
}

type rateTableSnapshot struct {
	Reference     versionReferenceSnapshot    `json:"reference"`
	Family        string                      `json:"family"`
	Currency      string                      `json:"currency"`
	Unit          string                      `json:"unit"`
	Period        periodSnapshot              `json:"period"`
	Entries       []rateEntrySnapshot         `json:"entries,omitempty"`
	FirstContinue []firstContinueRateSnapshot `json:"firstContinue,omitempty"`
	UnitPrice     []unitPriceRateSnapshot     `json:"unitPrice,omitempty"`
}

type roundingSegmentSnapshot struct {
	Mode      string          `json:"mode"`
	Increment weightSnapshot  `json:"increment"`
	Maximum   *weightSnapshot `json:"maximum,omitempty"`
}

type volumetricFactorSnapshot struct {
	Divisor    decimalSnapshot           `json:"divisor"`
	LengthUnit string                    `json:"lengthUnit"`
	Rounding   []roundingSegmentSnapshot `json:"rounding"`
}

type weightPolicySnapshot struct {
	Reference  versionReferenceSnapshot  `json:"reference"`
	Method     string                    `json:"method"`
	Rounding   []roundingSegmentSnapshot `json:"rounding"`
	Volumetric *volumetricFactorSnapshot `json:"volumetricFactor,omitempty"`
}

type fixedChargeRuleSnapshot struct {
	ID          string        `json:"id"`
	Code        string        `json:"code"`
	Description string        `json:"description"`
	Effect      string        `json:"effect"`
	Amount      moneySnapshot `json:"amount"`
	Order       int           `json:"order"`
}

type surchargeCalculationSnapshot struct {
	Method       string                         `json:"method"`
	Amount       *moneySnapshot                 `json:"amount,omitempty"`
	Table        *rateTableSnapshot             `json:"table,omitempty"`
	Percentage   *decimalSnapshot               `json:"percentage,omitempty"`
	SeriesKind   string                         `json:"seriesKind,omitempty"`
	SeriesFactor *decimalSnapshot               `json:"seriesFactor,omitempty"`
	Basis        string                         `json:"basis,omitempty"`
	Operands     []surchargeCalculationSnapshot `json:"operands,omitempty"`
}

type conditionalMinimumSnapshot struct {
	ID        string                   `json:"id"`
	Condition triggerConditionSnapshot `json:"condition"`
	Minimum   weightSnapshot           `json:"minimum"`
}

type surchargeRuleSnapshot struct {
	ID               string                       `json:"id"`
	Code             string                       `json:"code"`
	Description      string                       `json:"description"`
	Effect           string                       `json:"effect"`
	Condition        triggerConditionSnapshot     `json:"condition"`
	Calculation      surchargeCalculationSnapshot `json:"calculation"`
	Exclusivity      string                       `json:"exclusivity"`
	ExclusivityGroup string                       `json:"exclusivityGroup,omitempty"`
	Priority         int                          `json:"priority,omitempty"`
	MinimumWeight    *conditionalMinimumSnapshot  `json:"conditionalMinimumWeight,omitempty"`
}

type chargeDependencySnapshot struct {
	ID          string   `json:"id"`
	Code        string   `json:"code"`
	Composition string   `json:"composition"`
	Includes    []string `json:"includes,omitempty"`
	Excludes    []string `json:"excludes,omitempty"`
}

type referenceSeriesBindingSnapshot struct {
	Kind     string `json:"kind"`
	SeriesID string `json:"seriesId"`
}

type exclusionRuleSnapshot struct {
	ID        string                   `json:"id"`
	Clause    string                   `json:"clause"`
	Condition triggerConditionSnapshot `json:"condition"`
}

// amountRoundingSnapshot 是金额取整策略在快照里的形状（ADR-0107）：模式、进位单位（带币种）、应用点。
type amountRoundingSnapshot struct {
	Mode      string        `json:"mode"`
	Increment moneySnapshot `json:"increment"`
	Points    []string      `json:"points"`
}

func amountRoundingDocumentOf(policy AmountRoundingPolicy) amountRoundingSnapshot {
	points := make([]string, 0, len(policy.points))
	for _, point := range policy.points {
		points = append(points, point.String())
	}
	return amountRoundingSnapshot{Mode: string(policy.mode), Increment: moneyOf(policy.increment), Points: points}
}

func amountRoundingFrom(snapshot amountRoundingSnapshot) AmountRoundingPolicy {
	points := make([]AmountRoundingPoint, 0, len(snapshot.Points))
	for _, point := range snapshot.Points {
		points = append(points, AmountRoundingPoint(point))
	}
	return AmountRoundingPolicy{mode: RoundingMode(snapshot.Mode), increment: moneyFrom(snapshot.Increment), points: points}
}

type pricingPlanSnapshot struct {
	Canonicalization string                           `json:"canonicalization"`
	Reference        versionReferenceSnapshot         `json:"reference"`
	Scope            string                           `json:"scope"`
	Direction        string                           `json:"direction"`
	Purpose          string                           `json:"purpose"`
	BaseChargeCode   string                           `json:"baseChargeCode"`
	Aggregation      string                           `json:"aggregation"`
	Period           periodSnapshot                   `json:"period"`
	RateTable        rateTableSnapshot                `json:"rateTable"`
	WeightPolicy     weightPolicySnapshot             `json:"weightPolicy"`
	Rules            []fixedChargeRuleSnapshot        `json:"rules,omitempty"`
	SurchargeRules   []surchargeRuleSnapshot          `json:"surchargeRules,omitempty"`
	Dependencies     []chargeDependencySnapshot       `json:"chargeDependencies,omitempty"`
	ReferenceSeries  []referenceSeriesBindingSnapshot `json:"referenceSeries,omitempty"`
	Exclusions       []exclusionRuleSnapshot          `json:"exclusions,omitempty"`
	AmountRounding   *amountRoundingSnapshot          `json:"amountRounding,omitempty"`
	Manifest         []versionReferenceSnapshot       `json:"manifest"`
	ContentDigest    string                           `json:"contentDigest"`
}

// MarshalPricingPlanSnapshot 把一张价卡折成持久化快照。只接受立得住的价卡——写入前
// 的门与读回的门是同一道。
func MarshalPricingPlanSnapshot(plan PricingPlanVersion) ([]byte, error) {
	if !plan.valid() {
		return nil, ErrPricingPlanSnapshotInvalid
	}
	return json.Marshal(pricingPlanDocumentOf(plan))
}

// RehydratePricingPlanSnapshot 从快照重建价卡并整图重验。快照的规范化版本不是当前
// 构建支持的那一个时拒绝重建——按别的形状重算摘要在结构上不可能（ADR-0014）。
func RehydratePricingPlanSnapshot(raw []byte) (PricingPlanVersion, error) {
	var document pricingPlanSnapshot
	if err := json.Unmarshal(raw, &document); err != nil {
		return PricingPlanVersion{}, fmt.Errorf("%w: %v", ErrPricingPlanSnapshotInvalid, err)
	}
	if document.Canonicalization != canonicalizationVersion {
		return PricingPlanVersion{}, fmt.Errorf("%w: snapshot records %q, this build canonicalizes %q",
			ErrCanonicalizationVersionUnsupported, document.Canonicalization, canonicalizationVersion)
	}
	plan := pricingPlanFrom(document)
	if !plan.valid() {
		return PricingPlanVersion{}, ErrPricingPlanSnapshotInvalid
	}
	return plan, nil
}

func pricingPlanDocumentOf(plan PricingPlanVersion) pricingPlanSnapshot {
	document := pricingPlanSnapshot{
		Canonicalization: plan.canonicalization,
		Reference:        versionReferenceOf(plan.reference),
		Scope:            plan.scope.String(),
		Direction:        string(plan.direction),
		Purpose:          string(plan.purpose),
		BaseChargeCode:   plan.baseChargeCode.String(),
		Aggregation:      string(plan.aggregation),
		Period:           periodSnapshot{StartsAt: plan.period.startsAt, EndsAt: plan.period.endsAt},
		RateTable:        rateTableDocumentOf(plan.rateTable),
		WeightPolicy:     weightPolicyDocumentOf(plan.weight),
		ContentDigest:    plan.contentDigest,
		Manifest:         make([]versionReferenceSnapshot, 0, len(plan.manifest.references)),
	}
	for _, rule := range plan.rules {
		document.Rules = append(document.Rules, fixedChargeRuleSnapshot{
			ID: rule.id, Code: rule.chargeCode.String(), Description: rule.description,
			Effect: string(rule.effect), Amount: moneyOf(rule.amount), Order: rule.order,
		})
	}
	for _, rule := range plan.structures.surchargeRules {
		document.SurchargeRules = append(document.SurchargeRules, surchargeRuleDocumentOf(rule))
	}
	for _, dependency := range plan.structures.dependencies {
		document.Dependencies = append(document.Dependencies, chargeDependencyDocumentOf(dependency))
	}
	for _, binding := range plan.structures.referenceSeries {
		document.ReferenceSeries = append(document.ReferenceSeries, referenceSeriesBindingSnapshot{
			Kind: binding.kind.String(), SeriesID: binding.seriesID,
		})
	}
	for _, rule := range plan.structures.exclusions {
		document.Exclusions = append(document.Exclusions, exclusionRuleSnapshot{
			ID: rule.id, Clause: rule.clause, Condition: triggerDocumentOf(rule.condition),
		})
	}
	if plan.structures.amountRounding != nil {
		rounding := amountRoundingDocumentOf(*plan.structures.amountRounding)
		document.AmountRounding = &rounding
	}
	for _, reference := range plan.manifest.references {
		document.Manifest = append(document.Manifest, versionReferenceOf(reference))
	}
	return document
}

func pricingPlanFrom(document pricingPlanSnapshot) PricingPlanVersion {
	plan := PricingPlanVersion{
		reference:        versionReferenceFrom(document.Reference),
		scope:            PricingScopeID{identifier{value: document.Scope}},
		direction:        PricingDirection(document.Direction),
		purpose:          PricingPurpose(document.Purpose),
		baseChargeCode:   ChargeCode{value: document.BaseChargeCode},
		aggregation:      AggregationMode(document.Aggregation),
		period:           EffectivePeriod{startsAt: document.Period.StartsAt, endsAt: document.Period.EndsAt},
		rateTable:        rateTableFrom(document.RateTable),
		weight:           weightPolicyFrom(document.WeightPolicy),
		canonicalization: document.Canonicalization,
		contentDigest:    document.ContentDigest,
	}
	for _, rule := range document.Rules {
		plan.rules = append(plan.rules, FixedChargeRule{
			id: rule.ID, chargeCode: ChargeCode{value: rule.Code}, description: rule.Description,
			effect: ChargeEffect(rule.Effect), amount: moneyFrom(rule.Amount), order: rule.Order,
		})
	}
	for _, rule := range document.SurchargeRules {
		plan.structures.surchargeRules = append(plan.structures.surchargeRules, surchargeRuleFrom(rule))
	}
	for _, dependency := range document.Dependencies {
		plan.structures.dependencies = append(plan.structures.dependencies, chargeDependencyFrom(dependency))
	}
	for _, binding := range document.ReferenceSeries {
		plan.structures.referenceSeries = append(plan.structures.referenceSeries, ReferenceSeriesBinding{
			kind: ReferenceSeriesKind(binding.Kind), seriesID: binding.SeriesID,
		})
	}
	for _, rule := range document.Exclusions {
		plan.structures.exclusions = append(plan.structures.exclusions, ExclusionRule{
			id: rule.ID, clause: rule.Clause, condition: triggerFrom(rule.Condition),
		})
	}
	if document.AmountRounding != nil {
		rounding := amountRoundingFrom(*document.AmountRounding)
		plan.structures.amountRounding = &rounding
	}
	references := make([]VersionReference, 0, len(document.Manifest))
	for _, reference := range document.Manifest {
		references = append(references, versionReferenceFrom(reference))
	}
	plan.manifest = VersionManifest{references: references}
	return plan
}

func rateTableDocumentOf(table RateTableVersion) rateTableSnapshot {
	document := rateTableSnapshot{
		Reference: versionReferenceOf(table.reference),
		Family:    string(table.family),
		Currency:  table.currency.String(),
		Unit:      string(table.unit),
		Period:    periodSnapshot{StartsAt: table.period.startsAt, EndsAt: table.period.endsAt},
	}
	for _, entry := range table.entries {
		row := rateEntrySnapshot{
			ID: entry.id.String(), Zone: entry.zone,
			Minimum: weightOf(entry.minimum), Amount: moneyOf(entry.amount),
		}
		if entry.hasMaximum {
			maximum := weightOf(entry.maximum)
			row.Maximum = &maximum
		}
		document.Entries = append(document.Entries, row)
	}
	for _, rate := range table.firstContinue {
		document.FirstContinue = append(document.FirstContinue, firstContinueRateSnapshot{
			ID: rate.id.String(), Zone: rate.zone,
			FirstWeight: weightOf(rate.firstWeight), FirstAmount: moneyOf(rate.firstAmount),
			Step: weightOf(rate.step), StepAmount: moneyOf(rate.stepAmount),
		})
	}
	for _, rate := range table.unitPrice {
		document.UnitPrice = append(document.UnitPrice, unitPriceRateSnapshot{
			ID: rate.id.String(), Zone: rate.zone, AmountPerUnit: moneyOf(rate.amountPerUnit),
		})
	}
	return document
}

func rateTableFrom(document rateTableSnapshot) RateTableVersion {
	table := RateTableVersion{
		reference: versionReferenceFrom(document.Reference),
		family:    RateTableFamily(document.Family),
		currency:  Currency{code: document.Currency},
		unit:      WeightUnit(document.Unit),
		period:    EffectivePeriod{startsAt: document.Period.StartsAt, endsAt: document.Period.EndsAt},
	}
	for _, row := range document.Entries {
		entry := RateEntry{
			id: RateEntryID{identifier{value: row.ID}}, zone: row.Zone,
			minimum: weightFrom(row.Minimum), amount: moneyFrom(row.Amount),
		}
		if row.Maximum != nil {
			entry.maximum = weightFrom(*row.Maximum)
			entry.hasMaximum = true
		}
		table.entries = append(table.entries, entry)
	}
	for _, row := range document.FirstContinue {
		table.firstContinue = append(table.firstContinue, FirstContinueRate{
			id: RateEntryID{identifier{value: row.ID}}, zone: row.Zone,
			firstWeight: weightFrom(row.FirstWeight), firstAmount: moneyFrom(row.FirstAmount),
			step: weightFrom(row.Step), stepAmount: moneyFrom(row.StepAmount),
		})
	}
	for _, row := range document.UnitPrice {
		table.unitPrice = append(table.unitPrice, UnitPriceRate{
			id: RateEntryID{identifier{value: row.ID}}, zone: row.Zone,
			amountPerUnit: moneyFrom(row.AmountPerUnit),
		})
	}
	return table
}

func roundingDocumentOf(policy WeightRoundingPolicy) []roundingSegmentSnapshot {
	segments := make([]roundingSegmentSnapshot, 0, len(policy.segments))
	for _, segment := range policy.segments {
		row := roundingSegmentSnapshot{Mode: string(segment.mode), Increment: weightOf(segment.increment)}
		if segment.hasMaximum {
			maximum := weightOf(segment.maximum)
			row.Maximum = &maximum
		}
		segments = append(segments, row)
	}
	return segments
}

func roundingFrom(rows []roundingSegmentSnapshot) WeightRoundingPolicy {
	policy := WeightRoundingPolicy{}
	for _, row := range rows {
		segment := WeightRoundingSegment{mode: RoundingMode(row.Mode), increment: weightFrom(row.Increment)}
		if row.Maximum != nil {
			segment.maximum = weightFrom(*row.Maximum)
			segment.hasMaximum = true
		}
		policy.segments = append(policy.segments, segment)
	}
	return policy
}

func weightPolicyDocumentOf(policy PricingWeightPolicy) weightPolicySnapshot {
	document := weightPolicySnapshot{
		Reference: versionReferenceOf(policy.reference),
		Method:    string(policy.method),
		Rounding:  roundingDocumentOf(policy.rounding),
	}
	if policy.volumetric != nil {
		factor := volumetricFactorSnapshot{
			Divisor:    decimalOf(policy.volumetric.divisor),
			LengthUnit: string(policy.volumetric.lengthUnit),
			Rounding:   roundingDocumentOf(policy.volumetric.rounding),
		}
		document.Volumetric = &factor
	}
	return document
}

func weightPolicyFrom(document weightPolicySnapshot) PricingWeightPolicy {
	policy := PricingWeightPolicy{
		reference: versionReferenceFrom(document.Reference),
		method:    PricingWeightMethod(document.Method),
		rounding:  roundingFrom(document.Rounding),
	}
	if document.Volumetric != nil {
		factor := VolumetricFactor{
			divisor:    decimalFrom(document.Volumetric.Divisor),
			lengthUnit: LengthUnit(document.Volumetric.LengthUnit),
			rounding:   roundingFrom(document.Volumetric.Rounding),
		}
		policy.volumetric = &factor
	}
	return policy
}

func featureConditionDocumentOf(condition FeatureCondition) featureConditionSnapshot {
	document := featureConditionSnapshot{
		Source:   condition.source.String(),
		Operator: condition.operator.String(),
	}
	// 按特征的量纲只写它实际持有的那个阈值字段；把三个字段都写出去，会让重建后的
	// 条件多出两个从未声明过的零值阈值。
	switch condition.source.measure() {
	case measureLength:
		document.LengthThreshold = &lengthThresholdSnapshot{
			Value: decimalOf(condition.lengthThreshold.value),
			Unit:  string(condition.lengthThreshold.unit),
		}
	case measureVolume:
		document.VolumeThreshold = &volumeThresholdSnapshot{
			Value: decimalOf(condition.volumeThreshold.value),
			Unit:  string(condition.volumeThreshold.unit),
		}
	case measureWeight:
		threshold := weightOf(condition.weightThreshold)
		document.WeightThreshold = &threshold
	case measureCategory:
		document.Category = condition.categoryExpected.String()
	}
	return document
}

func featureConditionFrom(document featureConditionSnapshot) FeatureCondition {
	condition := FeatureCondition{
		source:           FeatureSource(document.Source),
		operator:         ComparisonOperator(document.Operator),
		categoryExpected: CategoryValue(document.Category),
	}
	if document.LengthThreshold != nil {
		condition.lengthThreshold = Length{
			value: decimalFrom(document.LengthThreshold.Value),
			unit:  LengthUnit(document.LengthThreshold.Unit),
		}
	}
	if document.VolumeThreshold != nil {
		condition.volumeThreshold = Volume{
			value: decimalFrom(document.VolumeThreshold.Value),
			unit:  LengthUnit(document.VolumeThreshold.Unit),
		}
	}
	if document.WeightThreshold != nil {
		condition.weightThreshold = weightFrom(*document.WeightThreshold)
	}
	return condition
}

func triggerDocumentOf(trigger TriggerCondition) triggerConditionSnapshot {
	document := triggerConditionSnapshot{Kind: trigger.kind.String()}
	if trigger.kind == TriggerPredicate {
		predicate := featureConditionDocumentOf(trigger.predicate)
		document.Predicate = &predicate
	}
	for _, operand := range trigger.operands {
		document.Operands = append(document.Operands, triggerDocumentOf(operand))
	}
	return document
}

func triggerFrom(document triggerConditionSnapshot) TriggerCondition {
	trigger := TriggerCondition{kind: TriggerKind(document.Kind)}
	if document.Predicate != nil {
		trigger.predicate = featureConditionFrom(*document.Predicate)
	}
	for _, operand := range document.Operands {
		trigger.operands = append(trigger.operands, triggerFrom(operand))
	}
	return trigger
}

func surchargeCalculationDocumentOf(calculation SurchargeCalculation) surchargeCalculationSnapshot {
	document := surchargeCalculationSnapshot{
		Method: calculation.method.String(),
		Basis:  calculation.basis,
	}
	if calculation.amount != nil {
		amount := moneyOf(*calculation.amount)
		document.Amount = &amount
	}
	if calculation.table != nil {
		table := rateTableDocumentOf(*calculation.table)
		document.Table = &table
	}
	if calculation.percentage != nil {
		percentage := decimalOf(*calculation.percentage)
		document.Percentage = &percentage
	}
	if calculation.seriesKind != nil {
		document.SeriesKind = calculation.seriesKind.String()
	}
	if calculation.seriesFactor != nil {
		factor := decimalOf(*calculation.seriesFactor)
		document.SeriesFactor = &factor
	}
	for _, operand := range calculation.operands {
		document.Operands = append(document.Operands, surchargeCalculationDocumentOf(operand))
	}
	return document
}

func surchargeCalculationFrom(document surchargeCalculationSnapshot) SurchargeCalculation {
	calculation := SurchargeCalculation{
		method: ChargeMethod(document.Method),
		basis:  document.Basis,
	}
	if document.Amount != nil {
		amount := moneyFrom(*document.Amount)
		calculation.amount = &amount
	}
	if document.Table != nil {
		table := rateTableFrom(*document.Table)
		calculation.table = &table
	}
	if document.Percentage != nil {
		percentage := decimalFrom(*document.Percentage)
		calculation.percentage = &percentage
	}
	if document.SeriesKind != "" {
		kind := ReferenceSeriesKind(document.SeriesKind)
		calculation.seriesKind = &kind
	}
	if document.SeriesFactor != nil {
		factor := decimalFrom(*document.SeriesFactor)
		calculation.seriesFactor = &factor
	}
	for _, operand := range document.Operands {
		calculation.operands = append(calculation.operands, surchargeCalculationFrom(operand))
	}
	return calculation
}

func surchargeRuleDocumentOf(rule SurchargeRule) surchargeRuleSnapshot {
	document := surchargeRuleSnapshot{
		ID:               rule.id,
		Code:             rule.chargeCode.String(),
		Description:      rule.description,
		Effect:           string(rule.effect),
		Condition:        triggerDocumentOf(rule.condition),
		Calculation:      surchargeCalculationDocumentOf(rule.calculation),
		Exclusivity:      rule.exclusivity.String(),
		ExclusivityGroup: rule.exclusivityGroup,
		Priority:         rule.priority,
	}
	if rule.minimumWeight != nil {
		minimum := conditionalMinimumSnapshot{
			ID:        rule.minimumWeight.id,
			Condition: triggerDocumentOf(rule.minimumWeight.condition),
			Minimum:   weightOf(rule.minimumWeight.minimum),
		}
		document.MinimumWeight = &minimum
	}
	return document
}

func surchargeRuleFrom(document surchargeRuleSnapshot) SurchargeRule {
	rule := SurchargeRule{
		id:               document.ID,
		chargeCode:       ChargeCode{value: document.Code},
		description:      document.Description,
		effect:           ChargeEffect(document.Effect),
		condition:        triggerFrom(document.Condition),
		calculation:      surchargeCalculationFrom(document.Calculation),
		exclusivity:      ExclusivityStance(document.Exclusivity),
		exclusivityGroup: document.ExclusivityGroup,
		priority:         document.Priority,
	}
	if document.MinimumWeight != nil {
		minimum := ConditionalMinimumWeight{
			id:        document.MinimumWeight.ID,
			condition: triggerFrom(document.MinimumWeight.Condition),
			minimum:   weightFrom(document.MinimumWeight.Minimum),
		}
		rule.minimumWeight = &minimum
	}
	return rule
}

func chargeDependencyDocumentOf(dependency ChargeDependency) chargeDependencySnapshot {
	document := chargeDependencySnapshot{
		ID:          dependency.id,
		Code:        dependency.dependent.String(),
		Composition: dependency.composition.String(),
	}
	for _, code := range dependency.includes {
		document.Includes = append(document.Includes, code.String())
	}
	for _, code := range dependency.excludes {
		document.Excludes = append(document.Excludes, code.String())
	}
	return document
}

func chargeDependencyFrom(document chargeDependencySnapshot) ChargeDependency {
	dependency := ChargeDependency{
		id:          document.ID,
		dependent:   ChargeCode{value: document.Code},
		composition: ChargeBasisComposition(document.Composition),
	}
	for _, code := range document.Includes {
		dependency.includes = append(dependency.includes, ChargeCode{value: code})
	}
	for _, code := range document.Excludes {
		dependency.excludes = append(dependency.excludes, ChargeCode{value: code})
	}
	return dependency
}
