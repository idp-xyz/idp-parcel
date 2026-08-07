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

type canonicalRateTableDocument struct {
	Reference canonicalVersionReference    `json:"reference"`
	Kind      string                       `json:"kind"`
	Currency  string                       `json:"currency"`
	Unit      string                       `json:"unit"`
	Period    string                       `json:"period"`
	Entries   []canonicalRateEntryDocument `json:"entries"`
}

func canonicalRateTableValue(table RateTableVersion) canonicalRateTableDocument {
	entries := make([]canonicalRateEntryDocument, 0, len(table.entries))
	for _, entry := range table.entries {
		entries = append(entries, canonicalRateEntryValue(entry))
	}
	return canonicalRateTableDocument{
		Reference: canonicalReference(table.reference),
		Kind:      string(table.kind),
		Currency:  table.currency.String(),
		Unit:      table.unit.String(),
		Period:    table.period.canonicalString(),
		Entries:   entries,
	}
}

type canonicalWeightPolicyDocument struct {
	Reference canonicalVersionReference `json:"reference"`
	Method    string                    `json:"method"`
	Mode      string                    `json:"rounding_mode"`
	Increment string                    `json:"increment"`
	Unit      string                    `json:"unit"`
}

func canonicalWeightPolicyValue(policy BillableWeightPolicy) canonicalWeightPolicyDocument {
	return canonicalWeightPolicyDocument{
		Reference: canonicalReference(policy.reference),
		Method:    string(policy.method),
		Mode:      string(policy.rounding.mode),
		Increment: policy.rounding.increment.value.String(),
		Unit:      policy.rounding.increment.unit.String(),
	}
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

type canonicalPricingPlan struct {
	Reference   canonicalVersionReference     `json:"reference"`
	Scope       string                        `json:"scope"`
	Direction   string                        `json:"direction"`
	Purpose     string                        `json:"purpose"`
	BaseCode    string                        `json:"base_charge_code"`
	Aggregation string                        `json:"aggregation"`
	Period      string                        `json:"period"`
	RateTable   canonicalRateTableDocument    `json:"rate_table"`
	Weight      canonicalWeightPolicyDocument `json:"weight_policy"`
	Rules       []canonicalChargeRuleDocument `json:"rules"`
	Manifest    []canonicalVersionReference   `json:"manifest"`
}

func calculatePricingPlanContentDigest(plan PricingPlanVersion) string {
	rules := make([]canonicalChargeRuleDocument, 0, len(plan.rules))
	for _, rule := range plan.rules {
		rules = append(rules, canonicalChargeRuleValue(rule))
	}
	manifest := make([]canonicalVersionReference, 0, len(plan.manifest.references))
	for _, reference := range plan.manifest.references {
		manifest = append(manifest, canonicalReference(reference))
	}
	document := canonicalPricingPlan{
		Reference:   canonicalReference(plan.reference),
		Scope:       plan.scope.String(),
		Direction:   plan.direction.String(),
		Purpose:     plan.purpose.String(),
		BaseCode:    plan.baseChargeCode.String(),
		Aggregation: string(plan.aggregation),
		Period:      plan.period.canonicalString(),
		RateTable:   canonicalRateTableValue(plan.rateTable),
		Weight:      canonicalWeightPolicyValue(plan.weight),
		Rules:       rules,
		Manifest:    manifest,
	}
	return hashCanonical(document)
}

type canonicalBillableWeightDocument struct {
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

func canonicalBillableWeightValue(result BillableWeightResult) canonicalBillableWeightDocument {
	document := canonicalBillableWeightDocument{
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
	Volumetric  string                       `json:"volumetric,omitempty"`
	Dimensions  *canonicalDimensionsDocument `json:"dimensions,omitempty"`
	BusinessAt  string                       `json:"business_at"`
	Facts       []canonicalVersionReference  `json:"facts"`
}

type canonicalEvaluation struct {
	NumericProfile string                           `json:"numeric_profile"`
	Status         string                           `json:"status"`
	Input          canonicalEvaluationInput         `json:"input"`
	Direction      string                           `json:"direction"`
	Purpose        string                           `json:"purpose"`
	PlanReference  canonicalVersionReference        `json:"plan_reference"`
	PlanPeriod     string                           `json:"plan_period"`
	TablePeriod    string                           `json:"table_period"`
	PlanContent    string                           `json:"plan_content"`
	Manifest       []canonicalVersionReference      `json:"manifest"`
	BillableWeight *canonicalBillableWeightDocument `json:"billable_weight,omitempty"`
	MatchedRate    *canonicalRateEntryDocument      `json:"matched_rate,omitempty"`
	ChargeLines    []canonicalChargeLineDocument    `json:"charge_lines"`
	Total          *canonicalMoney                  `json:"total,omitempty"`
	Issues         []canonicalEvaluationIssue       `json:"issues"`
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
	if volumetric, ok := evaluation.input.VolumetricWeight(); ok {
		input.Volumetric = volumetric.value.String()
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
		NumericProfile: "decimal-bigint-v1",
		Status:         string(evaluation.status),
		Input:          input,
		Direction:      evaluation.direction.String(),
		Purpose:        evaluation.purpose.String(),
		PlanReference:  canonicalReference(evaluation.planReference),
		PlanPeriod:     evaluation.planPeriod.canonicalString(),
		TablePeriod:    evaluation.tablePeriod.canonicalString(),
		PlanContent:    evaluation.planContentDigest,
		Manifest:       manifest,
		Issues:         make([]canonicalEvaluationIssue, 0, len(evaluation.issues)),
	}
	for _, issue := range evaluation.issues {
		document.Issues = append(document.Issues, canonicalEvaluationIssueValue(issue))
	}
	if evaluation.billableWeight != nil {
		billable := canonicalBillableWeightValue(*evaluation.billableWeight)
		document.BillableWeight = &billable
	}
	if evaluation.matchedRate != nil {
		matched := canonicalRateEntryValue(*evaluation.matchedRate)
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
