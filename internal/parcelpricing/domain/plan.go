package domain

import (
	"fmt"
	"sort"
	"strings"
)

type WeightRoundingPolicy struct {
	mode      RoundingMode
	increment Weight
}

func NewWeightRoundingPolicy(mode RoundingMode, increment Weight) (WeightRoundingPolicy, error) {
	if !mode.valid() || !increment.valid() || increment.value.Sign() <= 0 {
		return WeightRoundingPolicy{}, ErrInvalidRoundingPolicy
	}
	if mode == RoundingNone && !increment.value.Equal(NewDecimalFromInt64(1)) {
		return WeightRoundingPolicy{}, fmt.Errorf("%w: NONE requires increment 1", ErrInvalidRoundingPolicy)
	}
	return WeightRoundingPolicy{mode: mode, increment: increment}, nil
}

func (policy WeightRoundingPolicy) Mode() RoundingMode { return policy.mode }
func (policy WeightRoundingPolicy) Increment() Weight  { return policy.increment }

func (policy WeightRoundingPolicy) valid() bool {
	_, err := NewWeightRoundingPolicy(policy.mode, policy.increment)
	return err == nil
}

type BillableWeightPolicy struct {
	reference VersionReference
	method    BillableWeightMethod
	rounding  WeightRoundingPolicy
}

func NewBillableWeightPolicy(
	reference VersionReference,
	method BillableWeightMethod,
	rounding WeightRoundingPolicy,
) (BillableWeightPolicy, error) {
	if reference.kind != ArtifactWeightPolicy || !reference.valid() || !method.valid() || !rounding.valid() {
		return BillableWeightPolicy{}, ErrInvalidRoundingPolicy
	}
	return BillableWeightPolicy{reference: reference, method: method, rounding: rounding}, nil
}

func (policy BillableWeightPolicy) Reference() VersionReference    { return policy.reference }
func (policy BillableWeightPolicy) Method() BillableWeightMethod   { return policy.method }
func (policy BillableWeightPolicy) Rounding() WeightRoundingPolicy { return policy.rounding }

func (policy BillableWeightPolicy) valid() bool {
	return policy.reference.kind == ArtifactWeightPolicy && policy.reference.valid() && policy.method.valid() && policy.rounding.valid()
}

type FixedChargeRule struct {
	id          string
	chargeCode  ChargeCode
	description string
	effect      ChargeEffect
	amount      Money
	order       int
}

func NewFixedChargeRule(id string, code ChargeCode, description string, effect ChargeEffect, amount Money, order int) (FixedChargeRule, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id || !code.valid() || strings.TrimSpace(description) == "" || strings.TrimSpace(description) != description || !effect.valid() || !amount.valid() || order < 1 {
		return FixedChargeRule{}, ErrInvalidChargeRule
	}
	return FixedChargeRule{id: id, chargeCode: code, description: description, effect: effect, amount: amount, order: order}, nil
}

func (rule FixedChargeRule) ID() string           { return rule.id }
func (rule FixedChargeRule) Code() ChargeCode     { return rule.chargeCode }
func (rule FixedChargeRule) Description() string  { return rule.description }
func (rule FixedChargeRule) Effect() ChargeEffect { return rule.effect }
func (rule FixedChargeRule) Amount() Money        { return rule.amount }
func (rule FixedChargeRule) Order() int           { return rule.order }

func (rule FixedChargeRule) valid() bool {
	return strings.TrimSpace(rule.id) != "" && strings.TrimSpace(rule.id) == rule.id && rule.chargeCode.valid() && strings.TrimSpace(rule.description) != "" && strings.TrimSpace(rule.description) == rule.description && rule.effect.valid() && rule.amount.valid() && rule.order >= 1
}

type PricingPlanVersion struct {
	reference      VersionReference
	scope          PricingScopeID
	direction      PricingDirection
	purpose        PricingPurpose
	baseChargeCode ChargeCode
	aggregation    AggregationMode
	period         EffectivePeriod
	rateTable      RateTableVersion
	weight         BillableWeightPolicy
	rules          []FixedChargeRule
	manifest       VersionManifest
	contentDigest  string
}

func NewPricingPlanVersion(
	reference VersionReference,
	scope PricingScopeID,
	direction PricingDirection,
	purpose PricingPurpose,
	baseChargeCode ChargeCode,
	period EffectivePeriod,
	rateTable RateTableVersion,
	weight BillableWeightPolicy,
	rules []FixedChargeRule,
	dependencies ...VersionReference,
) (PricingPlanVersion, error) {
	if reference.kind != ArtifactPricingPlan || !reference.valid() || !scope.valid() || !direction.valid() || !purpose.valid() || !baseChargeCode.valid() || !period.valid() || !rateTable.valid() || !weight.valid() {
		return PricingPlanVersion{}, ErrInvalidPricingPlan
	}
	if purpose.pairedDirection() != direction {
		return PricingPlanVersion{}, ErrDirectionPurposeMismatch
	}
	if !period.Within(rateTable.period) {
		return PricingPlanVersion{}, ErrPricingPeriodConflict
	}
	if weight.rounding.increment.unit != rateTable.unit {
		return PricingPlanVersion{}, ErrWeightUnitMismatch
	}
	copyOfRules := append([]FixedChargeRule(nil), rules...)
	seenRules := make(map[string]struct{}, len(copyOfRules))
	seenCodes := map[string]struct{}{baseChargeCode.String(): {}}
	seenOrders := make(map[int]struct{}, len(copyOfRules))
	for _, rule := range copyOfRules {
		if !rule.valid() || rule.amount.currency != rateTable.currency {
			return PricingPlanVersion{}, ErrInvalidPricingPlan
		}
		if _, exists := seenRules[rule.id]; exists {
			return PricingPlanVersion{}, fmt.Errorf("%w: %s", ErrDuplicateChargeRule, rule.id)
		}
		seenRules[rule.id] = struct{}{}
		if _, exists := seenCodes[rule.chargeCode.String()]; exists {
			return PricingPlanVersion{}, fmt.Errorf("%w: %s", ErrDuplicateChargeCode, rule.chargeCode.String())
		}
		seenCodes[rule.chargeCode.String()] = struct{}{}
		if _, exists := seenOrders[rule.order]; exists {
			return PricingPlanVersion{}, fmt.Errorf("%w: %d", ErrDuplicateChargeRuleOrder, rule.order)
		}
		seenOrders[rule.order] = struct{}{}
	}
	sort.SliceStable(copyOfRules, func(left, right int) bool {
		if copyOfRules[left].order != copyOfRules[right].order {
			return copyOfRules[left].order < copyOfRules[right].order
		}
		return copyOfRules[left].id < copyOfRules[right].id
	})

	manifestReferences := make([]VersionReference, 0, 4+len(dependencies))
	manifestReferences = append(manifestReferences, reference, rateTable.reference, weight.reference, NumericProfileV1Reference())
	manifestReferences = append(manifestReferences, dependencies...)
	manifest, err := NewVersionManifest(manifestReferences)
	if err != nil {
		return PricingPlanVersion{}, ErrInvalidPricingPlan
	}
	plan := PricingPlanVersion{
		reference:      reference,
		scope:          scope,
		direction:      direction,
		purpose:        purpose,
		baseChargeCode: baseChargeCode,
		aggregation:    AggregationPerPackage,
		period:         period,
		rateTable:      rateTable,
		weight:         weight,
		rules:          copyOfRules,
		manifest:       manifest,
	}
	plan.contentDigest = calculatePricingPlanContentDigest(plan)
	return plan, nil
}

func (plan PricingPlanVersion) Reference() VersionReference        { return plan.reference }
func (plan PricingPlanVersion) Scope() PricingScopeID              { return plan.scope }
func (plan PricingPlanVersion) Direction() PricingDirection        { return plan.direction }
func (plan PricingPlanVersion) Purpose() PricingPurpose            { return plan.purpose }
func (plan PricingPlanVersion) BaseChargeCode() ChargeCode         { return plan.baseChargeCode }
func (plan PricingPlanVersion) Aggregation() AggregationMode       { return plan.aggregation }
func (plan PricingPlanVersion) EffectivePeriod() EffectivePeriod   { return plan.period }
func (plan PricingPlanVersion) RateTable() RateTableVersion        { return plan.rateTable }
func (plan PricingPlanVersion) WeightPolicy() BillableWeightPolicy { return plan.weight }
func (plan PricingPlanVersion) Rules() []FixedChargeRule {
	return append([]FixedChargeRule(nil), plan.rules...)
}
func (plan PricingPlanVersion) Manifest() VersionManifest { return plan.manifest }
func (plan PricingPlanVersion) ContentDigest() string     { return plan.contentDigest }

func (plan PricingPlanVersion) valid() bool {
	if plan.reference.kind != ArtifactPricingPlan || !plan.reference.valid() || !plan.scope.valid() || !plan.direction.valid() || !plan.purpose.valid() || plan.purpose.pairedDirection() != plan.direction || !plan.baseChargeCode.valid() || plan.aggregation != AggregationPerPackage || !plan.period.valid() || !plan.rateTable.valid() || !plan.weight.valid() || !plan.manifest.valid() || plan.contentDigest == "" {
		return false
	}
	if !plan.period.Within(plan.rateTable.period) || plan.weight.rounding.increment.unit != plan.rateTable.unit {
		return false
	}
	seenRuleIDs := make(map[string]struct{}, len(plan.rules))
	seenCodes := map[string]struct{}{plan.baseChargeCode.String(): {}}
	seenRuleOrders := make(map[int]struct{}, len(plan.rules))
	for index, rule := range plan.rules {
		if !rule.valid() || rule.amount.currency != plan.rateTable.currency {
			return false
		}
		if _, exists := seenRuleIDs[rule.id]; exists {
			return false
		}
		seenRuleIDs[rule.id] = struct{}{}
		if _, exists := seenCodes[rule.chargeCode.String()]; exists {
			return false
		}
		seenCodes[rule.chargeCode.String()] = struct{}{}
		if _, exists := seenRuleOrders[rule.order]; exists {
			return false
		}
		seenRuleOrders[rule.order] = struct{}{}
		if index > 0 && plan.rules[index-1].order >= rule.order {
			return false
		}
	}
	for _, required := range []VersionReference{
		plan.reference,
		plan.rateTable.reference,
		plan.weight.reference,
		NumericProfileV1Reference(),
	} {
		found := false
		for _, reference := range plan.manifest.references {
			if reference == required {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if plan.contentDigest != calculatePricingPlanContentDigest(plan) {
		return false
	}
	return true
}
