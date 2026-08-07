package domain

import (
	"fmt"
	"sort"
	"strings"
)

// SurchargeCalculation carries the method and the parameters that method needs.
// Keeping the parameters here rather than on the rule is what lets one rule
// shape serve every method in the closed set.
type SurchargeCalculation struct {
	method     ChargeMethod
	amount     *Money
	table      *RateTableVersion
	percentage *Decimal
	basis      string
	operands   []SurchargeCalculation
}

func NewFixedAmountSurcharge(amount Money) (SurchargeCalculation, error) {
	return validSurcharge(SurchargeCalculation{method: ChargeMethodFixedAmount, amount: &amount})
}

func NewTableLookupSurcharge(table RateTableVersion) (SurchargeCalculation, error) {
	return validSurcharge(SurchargeCalculation{method: ChargeMethodTableLookup, table: &table})
}

// NewPercentOfBasisSurcharge takes the ID of the ChargeDependency that defines
// the basis. Naming the dependency rather than restating a code list keeps one
// definition of the basis; the plan rejects a rule whose named basis it does
// not declare.
func NewPercentOfBasisSurcharge(percentage Decimal, basisDependencyID string) (SurchargeCalculation, error) {
	return validSurcharge(SurchargeCalculation{
		method:     ChargeMethodPercentOfBasis,
		percentage: &percentage,
		basis:      basisDependencyID,
	})
}

// NewGreaterOfSurcharge takes the greater of two other methods. Neither operand
// may itself be a greater-of: the card asks for a choice between two amounts,
// not an arbitrarily nested expression.
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

// basisDependencyIDs reports every dependency ID this calculation names,
// including through a greater-of operand.
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
	populated := 0
	for _, present := range []bool{
		calculation.amount != nil,
		calculation.table != nil,
		calculation.percentage != nil,
		len(calculation.operands) > 0,
	} {
		if present {
			populated++
		}
	}
	if populated != 1 {
		return false
	}
	switch calculation.method {
	case ChargeMethodFixedAmount:
		return calculation.amount != nil && calculation.amount.valid()
	case ChargeMethodTableLookup:
		return calculation.table != nil && calculation.table.valid()
	case ChargeMethodPercentOfBasis:
		return calculation.percentage != nil && calculation.percentage.valid() &&
			!calculation.percentage.IsNegative() && trimmed(calculation.basis)
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

// ConditionalMinimumWeight raises the plan-level pricing weight when its
// condition holds. The card declares it inside a surcharge clause, but its
// effect is the one weight the base rate lookup and every later basis read, not
// a private basis for the declaring rule.
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

// SurchargeRule is a versioned rule that produces a charge line beyond the base
// freight when its condition holds.
// ExclusivityStance is what a card says about whether this surcharge competes
// with others or stands beside them. It is three-valued on purpose: carriers
// disagree here — UPS puts its large-package charge in the same exclusive set
// as additional handling while FedEx collects both — so an unset group must not
// be readable as "stands alone". Silence is its own state and a plan refuses it.
type ExclusivityStance string

const (
	ExclusivityUndeclared ExclusivityStance = ""
	ExclusivityStandalone ExclusivityStance = "STANDALONE"
	ExclusivityGrouped    ExclusivityStance = "GROUPED"
)

func (stance ExclusivityStance) String() string { return string(stance) }

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

// InExclusivityGroup returns the rule as a member of a group at the given
// priority. A priority is only meaningful against the other members of a group,
// so the two are declared together or not at all.
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

// Standalone records that the card collects this surcharge alongside the
// others. It is a declaration in its own right, not the absence of one.
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

// declaredCurrencies reports the currency of every amount the rule states: a
// fixed amount, a banded table, and each operand of a greater-of. Every one has
// to be reported, because nothing downstream can catch a foreign amount —
// charge totals accumulate as bare decimals and are stamped with the plan's
// currency at the end, so an unchecked operand is billed as the plan's own on
// an evaluation that still reads as completed. This gate is the only guard.
// A percentage states no currency of its own; it takes the one its basis is in.
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

// declaredWeightUnits reports every weight unit the rule states: the unit a
// banded table is read in, and the unit a conditional minimum raises to. Both
// are used against the pricing weight, which the plan states in the unit its
// base table bands in, so a unit the plan does not price in could never be
// read. The conditional minimum is the reason this belongs at formation rather
// than at evaluation: it would fail only on the parcels that trip its clause,
// so one card would price some packages and conflict on others.
func (rule SurchargeRule) declaredWeightUnits() []WeightUnit {
	units := rule.calculation.lookupWeightUnits()
	if rule.minimumWeight != nil {
		units = append(units, rule.minimumWeight.minimum.unit)
	}
	return units
}

// lookupWeightUnits reports the unit of every rate table this calculation reads
// a band from, including through a greater-of operand.
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

// ChargeBasisComposition says how a dependency's basis is assembled. The card
// needs both shapes: the fuel basis is every other charge minus a named
// exclusion, while a percentage surcharge names the charges it applies to.
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

// ChargeDependency declares that one charge takes the subtotal of others as its
// basis. Both the composition and the exclusion set are explicit: declaration
// order never implies a dependency, because order is presentation and the basis
// is a rule. A charge is never part of its own basis.
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

// ReferenceSeriesKind is the closed set of external numeric series a plan may
// resolve. ADR-0013 gives pricing the registration and versioning of these
// series but not their values.
type ReferenceSeriesKind string

const (
	ReferenceSeriesFuelRate     ReferenceSeriesKind = "FUEL_RATE"
	ReferenceSeriesExchangeRate ReferenceSeriesKind = "EXCHANGE_RATE"
)

func (kind ReferenceSeriesKind) String() string { return string(kind) }

func (kind ReferenceSeriesKind) valid() bool {
	switch kind {
	case ReferenceSeriesFuelRate, ReferenceSeriesExchangeRate:
		return true
	default:
		return false
	}
}

// ReferenceSeriesBinding ties a plan to one registered series version. The
// binding names the series, never a value: the value is resolved per evaluation
// at the pricing base time and frozen into that evaluation's manifest.
type ReferenceSeriesBinding struct {
	kind      ReferenceSeriesKind
	reference VersionReference
}

func NewReferenceSeriesBinding(kind ReferenceSeriesKind, reference VersionReference) (ReferenceSeriesBinding, error) {
	binding := ReferenceSeriesBinding{kind: kind, reference: reference}
	if !binding.valid() {
		return ReferenceSeriesBinding{}, ErrInvalidReferenceSeries
	}
	return binding, nil
}

func (binding ReferenceSeriesBinding) Kind() ReferenceSeriesKind   { return binding.kind }
func (binding ReferenceSeriesBinding) Reference() VersionReference { return binding.reference }

func (binding ReferenceSeriesBinding) valid() bool {
	return binding.kind.valid() && binding.reference.kind == ArtifactReferenceSeries && binding.reference.valid()
}

// PricingPlanStructures carries the rule structures a released plan declares
// beyond its base table and unconditional fixed rules. The zero value declares
// none.
//
// It is one value rather than several constructor parameters so that the
// canonical plan document has a slot for every structure CONTEXT lists under
// 版本内容摘要 before any of them is executable. Widening the canonical shape
// after real evaluations exist would move the digest of plans that never used
// the new structure and report every replay as a version content conflict.
type PricingPlanStructures struct {
	surchargeRules  []SurchargeRule
	dependencies    []ChargeDependency
	referenceSeries []ReferenceSeriesBinding
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
		// Whether this surcharge competes with the others is the carrier's rule
		// and differs between carriers, so a card that never said cannot be
		// published under either reading.
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
		return compareVersionReferences(copyOfSeries[left].reference, copyOfSeries[right].reference) < 0
	})
	declaredBases := make(map[string]struct{}, len(copyOfDependencies))
	for _, dependency := range copyOfDependencies {
		declaredBases[dependency.id] = struct{}{}
	}
	for _, rule := range copyOfRules {
		for _, basis := range rule.calculation.basisDependencyIDs() {
			if _, declared := declaredBases[basis]; !declared {
				return PricingPlanStructures{}, fmt.Errorf("%w: %s names undeclared basis %s", ErrInvalidChargeDependency, rule.id, basis)
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

// Declared reports whether the plan declares any structure the evaluator must
// execute before it can produce a complete amount.
func (structures PricingPlanStructures) Declared() bool {
	return len(structures.surchargeRules) > 0 || len(structures.dependencies) > 0 || len(structures.referenceSeries) > 0
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
	for _, rule := range structures.surchargeRules {
		for _, basis := range rule.calculation.basisDependencyIDs() {
			if _, declared := declaredBases[basis]; !declared {
				return false
			}
		}
	}
	seenKinds := make(map[ReferenceSeriesKind]struct{}, len(structures.referenceSeries))
	for index, binding := range structures.referenceSeries {
		if !binding.valid() {
			return false
		}
		if index > 0 && compareVersionReferences(structures.referenceSeries[index-1].reference, binding.reference) >= 0 {
			return false
		}
		if _, exists := seenKinds[binding.kind]; exists {
			return false
		}
		seenKinds[binding.kind] = struct{}{}
	}
	return true
}

func trimmed(value string) bool {
	return strings.TrimSpace(value) != "" && strings.TrimSpace(value) == value
}
