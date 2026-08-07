package domain

import (
	"fmt"
	"sort"
	"strings"
)

// VolumetricFactor is the card's own divisor, the length unit it reads, and the
// precision its quotient is declared to. The card states it — L4 reads
// "volumetric pounds = length × width × height in inches / 250" — so this
// package never carries a divisor of its own. The weight unit comes from the
// rounding increment, because a divisor that turns cubic inches into pounds
// says nothing about any other pair of units.
type VolumetricFactor struct {
	divisor    Decimal
	lengthUnit LengthUnit
	rounding   WeightRoundingPolicy
}

func NewVolumetricFactor(divisor Decimal, lengthUnit LengthUnit, rounding WeightRoundingPolicy) (VolumetricFactor, error) {
	factor := VolumetricFactor{divisor: divisor, lengthUnit: lengthUnit, rounding: rounding}
	if !factor.valid() {
		return VolumetricFactor{}, ErrInvalidVolumetricFactor
	}
	return factor, nil
}

func (factor VolumetricFactor) Divisor() Decimal               { return factor.divisor }
func (factor VolumetricFactor) LengthUnit() LengthUnit         { return factor.lengthUnit }
func (factor VolumetricFactor) WeightUnit() WeightUnit         { return factor.rounding.unit() }
func (factor VolumetricFactor) Rounding() WeightRoundingPolicy { return factor.rounding }

// Apply turns a volume into the volumetric weight the card declares. A volume
// measured in another unit is refused rather than converted: the conversion
// rule would itself have to be a versioned declaration.
func (factor VolumetricFactor) Apply(volume Volume) (Weight, error) {
	if !factor.valid() || !volume.valid() {
		return Weight{}, ErrInvalidVolumetricFactor
	}
	if volume.unit != factor.lengthUnit {
		return Weight{}, ErrLengthUnitMismatch
	}
	segment, ok := factor.rounding.sole()
	if !ok {
		return Weight{}, ErrInvalidVolumetricFactor
	}
	value, err := volume.value.DivRoundToIncrement(factor.divisor, segment.increment.value, segment.mode)
	if err != nil {
		return Weight{}, err
	}
	return NewWeight(value, segment.increment.unit)
}

func (factor VolumetricFactor) valid() bool {
	// A divisor declares the precision of one quotient, so it takes a single
	// rounding segment: banding by weight is impossible here because the weight
	// is what the division produces.
	segment, ok := factor.rounding.sole()
	if !ok {
		return false
	}
	// RoundingNone is excluded because an exact quotient need not terminate in
	// base 10; the precision has to be declared rather than left to the code.
	return factor.divisor.valid() && factor.divisor.Sign() > 0 && factor.lengthUnit.valid() &&
		factor.rounding.valid() && segment.mode != RoundingNone
}

type PricingWeightPolicy struct {
	reference  VersionReference
	method     PricingWeightMethod
	rounding   WeightRoundingPolicy
	volumetric *VolumetricFactor
}

func NewPricingWeightPolicy(
	reference VersionReference,
	method PricingWeightMethod,
	rounding WeightRoundingPolicy,
	volumetric *VolumetricFactor,
) (PricingWeightPolicy, error) {
	policy := PricingWeightPolicy{reference: reference, method: method, rounding: rounding}
	if volumetric != nil {
		factor := *volumetric
		policy.volumetric = &factor
	}
	if !policy.valid() {
		return PricingWeightPolicy{}, ErrInvalidRoundingPolicy
	}
	return policy, nil
}

func (policy PricingWeightPolicy) VolumetricFactor() (VolumetricFactor, bool) {
	if policy.volumetric == nil {
		return VolumetricFactor{}, false
	}
	return *policy.volumetric, true
}

func (policy PricingWeightPolicy) Reference() VersionReference    { return policy.reference }
func (policy PricingWeightPolicy) Method() PricingWeightMethod    { return policy.method }
func (policy PricingWeightPolicy) Rounding() WeightRoundingPolicy { return policy.rounding }

func (policy PricingWeightPolicy) valid() bool {
	if policy.reference.kind != ArtifactWeightPolicy || !policy.reference.valid() || !policy.method.valid() || !policy.rounding.valid() {
		return false
	}
	// A method that never reads a divisor must not declare one, and MAX cannot
	// reach a volumetric weight without it.
	switch policy.method {
	case PricingWeightActualOnly:
		return policy.volumetric == nil
	case PricingWeightMax:
		return policy.volumetric != nil && policy.volumetric.valid() &&
			policy.volumetric.rounding.unit() == policy.rounding.unit()
	default:
		return false
	}
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
	reference        VersionReference
	scope            PricingScopeID
	direction        PricingDirection
	purpose          PricingPurpose
	baseChargeCode   ChargeCode
	aggregation      AggregationMode
	period           EffectivePeriod
	rateTable        RateTableVersion
	weight           PricingWeightPolicy
	rules            []FixedChargeRule
	structures       PricingPlanStructures
	manifest         VersionManifest
	canonicalization string
	contentDigest    string
}

func NewPricingPlanVersion(
	reference VersionReference,
	scope PricingScopeID,
	direction PricingDirection,
	purpose PricingPurpose,
	baseChargeCode ChargeCode,
	period EffectivePeriod,
	rateTable RateTableVersion,
	weight PricingWeightPolicy,
	rules []FixedChargeRule,
	structures PricingPlanStructures,
	dependencies ...VersionReference,
) (PricingPlanVersion, error) {
	if reference.kind != ArtifactPricingPlan || !reference.valid() || !scope.valid() || !direction.valid() || !purpose.valid() || !baseChargeCode.valid() || !period.valid() || !rateTable.valid() || !weight.valid() || !structures.valid() {
		return PricingPlanVersion{}, ErrInvalidPricingPlan
	}
	if purpose.pairedDirection() != direction {
		return PricingPlanVersion{}, ErrDirectionPurposeMismatch
	}
	if !period.Within(rateTable.period) {
		return PricingPlanVersion{}, ErrPricingPeriodConflict
	}
	if weight.rounding.unit() != rateTable.unit {
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
	for _, surcharge := range structures.surchargeRules {
		if surcharge.amountCurrency() != nil && *surcharge.amountCurrency() != rateTable.currency {
			return PricingPlanVersion{}, ErrCurrencyMismatch
		}
		for _, unit := range surcharge.declaredWeightUnits() {
			if unit != rateTable.unit {
				return PricingPlanVersion{}, ErrWeightUnitMismatch
			}
		}
		if _, exists := seenCodes[surcharge.chargeCode.String()]; exists {
			return PricingPlanVersion{}, fmt.Errorf("%w: %s", ErrDuplicateChargeCode, surcharge.chargeCode.String())
		}
		seenCodes[surcharge.chargeCode.String()] = struct{}{}
	}

	manifestReferences := make([]VersionReference, 0, 4+len(structures.referenceSeries)+len(dependencies))
	manifestReferences = append(manifestReferences, reference, rateTable.reference, weight.reference, NumericProfileV1Reference())
	for _, binding := range structures.referenceSeries {
		manifestReferences = append(manifestReferences, binding.reference)
	}
	manifestReferences = append(manifestReferences, dependencies...)
	manifest, err := NewVersionManifest(manifestReferences)
	if err != nil {
		return PricingPlanVersion{}, ErrInvalidPricingPlan
	}
	plan := PricingPlanVersion{
		reference:        reference,
		scope:            scope,
		direction:        direction,
		purpose:          purpose,
		baseChargeCode:   baseChargeCode,
		aggregation:      AggregationPerPackage,
		period:           period,
		rateTable:        rateTable,
		weight:           weight,
		rules:            copyOfRules,
		structures:       structures,
		manifest:         manifest,
		canonicalization: canonicalizationVersion,
	}
	plan.contentDigest = calculatePricingPlanContentDigest(plan)
	return plan, nil
}

func (plan PricingPlanVersion) Reference() VersionReference       { return plan.reference }
func (plan PricingPlanVersion) Scope() PricingScopeID             { return plan.scope }
func (plan PricingPlanVersion) Direction() PricingDirection       { return plan.direction }
func (plan PricingPlanVersion) Purpose() PricingPurpose           { return plan.purpose }
func (plan PricingPlanVersion) BaseChargeCode() ChargeCode        { return plan.baseChargeCode }
func (plan PricingPlanVersion) Aggregation() AggregationMode      { return plan.aggregation }
func (plan PricingPlanVersion) EffectivePeriod() EffectivePeriod  { return plan.period }
func (plan PricingPlanVersion) RateTable() RateTableVersion       { return plan.rateTable }
func (plan PricingPlanVersion) WeightPolicy() PricingWeightPolicy { return plan.weight }
func (plan PricingPlanVersion) Rules() []FixedChargeRule {
	return append([]FixedChargeRule(nil), plan.rules...)
}
func (plan PricingPlanVersion) Structures() PricingPlanStructures { return plan.structures }
func (plan PricingPlanVersion) Manifest() VersionManifest         { return plan.manifest }
func (plan PricingPlanVersion) ContentDigest() string             { return plan.contentDigest }

// CanonicalizationVersion reports the shape the content digest was produced
// under. Digests are only comparable within the same value. See ADR-0014.
func (plan PricingPlanVersion) CanonicalizationVersion() string { return plan.canonicalization }

func (plan PricingPlanVersion) valid() bool {
	if plan.reference.kind != ArtifactPricingPlan || !plan.reference.valid() || !plan.scope.valid() || !plan.direction.valid() || !plan.purpose.valid() || plan.purpose.pairedDirection() != plan.direction || !plan.baseChargeCode.valid() || plan.aggregation != AggregationPerPackage || !plan.period.valid() || !plan.rateTable.valid() || !plan.weight.valid() || !plan.structures.valid() || !plan.manifest.valid() || plan.canonicalization == "" || plan.contentDigest == "" {
		return false
	}
	if !plan.period.Within(plan.rateTable.period) || plan.weight.rounding.unit() != plan.rateTable.unit {
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
	for _, surcharge := range plan.structures.surchargeRules {
		if surcharge.amountCurrency() != nil && *surcharge.amountCurrency() != plan.rateTable.currency {
			return false
		}
		for _, unit := range surcharge.declaredWeightUnits() {
			if unit != plan.rateTable.unit {
				return false
			}
		}
		if _, exists := seenCodes[surcharge.chargeCode.String()]; exists {
			return false
		}
		seenCodes[surcharge.chargeCode.String()] = struct{}{}
	}
	requiredReferences := []VersionReference{
		plan.reference,
		plan.rateTable.reference,
		plan.weight.reference,
		NumericProfileV1Reference(),
	}
	for _, binding := range plan.structures.referenceSeries {
		requiredReferences = append(requiredReferences, binding.reference)
	}
	for _, required := range requiredReferences {
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
