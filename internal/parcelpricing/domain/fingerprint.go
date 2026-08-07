package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

type canonicalVersionReference struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

func canonicalReference(reference VersionReference) canonicalVersionReference {
	return canonicalVersionReference{
		Kind:    string(reference.kind),
		ID:      reference.id,
		Version: reference.version,
		Digest:  reference.digest,
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
}

func canonicalChargeRuleValue(rule FixedChargeRule) canonicalChargeRuleDocument {
	return canonicalChargeRuleDocument{
		ID:          rule.id,
		Code:        rule.chargeCode.String(),
		Description: rule.description,
		Effect:      string(rule.effect),
		Amount:      canonicalMoneyValue(rule.amount),
		Order:       rule.order,
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
	// Read the threshold through the condition rather than off one field: a
	// weight or volume threshold lives in a different field, and hashing the
	// length field regardless would give every non-length condition the same
	// empty threshold in the digest.
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
}

func canonicalSurchargeCalculationValue(calculation SurchargeCalculation) canonicalSurchargeCalculationDocument {
	document := canonicalSurchargeCalculationDocument{
		Method:   calculation.method.String(),
		Basis:    calculation.basis,
		Operands: make([]canonicalSurchargeCalculationDocument, 0, len(calculation.operands)),
	}
	if calculation.amount != nil {
		amount := canonicalMoneyValue(*calculation.amount)
		document.Amount = &amount
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

type canonicalReferenceSeriesDocument struct {
	Kind      string                    `json:"kind"`
	Reference canonicalVersionReference `json:"reference"`
}

func canonicalReferenceSeriesValue(binding ReferenceSeriesBinding) canonicalReferenceSeriesDocument {
	return canonicalReferenceSeriesDocument{
		Kind:      binding.kind.String(),
		Reference: canonicalReference(binding.reference),
	}
}

// canonicalizationVersion identifies the shape of the canonical documents that
// content and semantic digests are computed from. Digests are only comparable
// within the same canonicalization version; widening the shape must bump this
// value rather than rewrite the existing one. See ADR-0014.
// PPC-2 widened the shape in three places at once, as the rule model design
// decided: the rate table gained the first-continue and unit-price families,
// the rounding policy became a segment list, and a lookup now records a rate
// selection instead of a bracket. Batching them costs one version instead of
// three, and every version has to be supported for as long as evaluations
// recorded under it can be replayed.
// PPC-3 turned a rule's condition from a single predicate into a trigger, so
// the condition document is now a kind with an optional predicate and operands
// rather than a bare predicate. A rule that reads one predicate serialises as a
// PREDICATE trigger, which is a different shape from the bare predicate PPC-2
// wrote; there is no widening that leaves the old bytes intact.
const canonicalizationVersion = "PPC-3"

// CurrentCanonicalizationVersion reports the shape this build canonicalizes
// under. An artifact recorded under any other value cannot have its digest
// recomputed here.
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
	document := canonicalPricingPlan{
		Canonicalization: canonicalizationVersion,
		Reference:        canonicalReference(plan.reference),
		Scope:            plan.scope.String(),
		Direction:        plan.direction.String(),
		Purpose:          plan.purpose.String(),
		BaseCode:         plan.baseChargeCode.String(),
		Aggregation:      string(plan.aggregation),
		Period:           plan.period.canonicalString(),
		RateTable:        canonicalRateTableValue(plan.rateTable),
		Weight:           canonicalWeightPolicyValue(plan.weight),
		Rules:            rules,
		SurchargeRules:   surcharges,
		Dependencies:     dependencies,
		ReferenceSeries:  series,
		Manifest:         manifest,
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
	ChargeLines      []canonicalChargeLineDocument   `json:"charge_lines"`
	Total            *canonicalMoney                 `json:"total,omitempty"`
	Issues           []canonicalEvaluationIssue      `json:"issues"`
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
		{left.Digest, right.Digest},
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
