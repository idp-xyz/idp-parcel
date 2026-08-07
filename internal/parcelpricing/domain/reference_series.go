package domain

import "fmt"

// ReferenceSeriesValue is one series reading, resolved at the pricing base time
// and frozen into the evaluation's input.
//
// ADR-0013 gives pricing the registration and versioning of these series but
// not their values: the fuel rate is published by the carrier and the exchange
// rate comes from the finance side. The value therefore arrives with the
// snapshot rather than being fetched during the evaluation — fetching it would
// make a replay read whatever the series holds today.
type ReferenceSeriesValue struct {
	kind      ReferenceSeriesKind
	reference VersionReference
	value     Decimal
}

func NewReferenceSeriesValue(kind ReferenceSeriesKind, reference VersionReference, value Decimal) (ReferenceSeriesValue, error) {
	resolved := ReferenceSeriesValue{kind: kind, reference: reference, value: value}
	if !resolved.valid() {
		return ReferenceSeriesValue{}, ErrInvalidReferenceSeries
	}
	return resolved, nil
}

func (resolved ReferenceSeriesValue) Kind() ReferenceSeriesKind   { return resolved.kind }
func (resolved ReferenceSeriesValue) Reference() VersionReference { return resolved.reference }
func (resolved ReferenceSeriesValue) Value() Decimal              { return resolved.value }

func (resolved ReferenceSeriesValue) valid() bool {
	return resolved.kind.valid() &&
		resolved.reference.kind == ArtifactReferenceSeries && resolved.reference.valid() &&
		resolved.value.valid() && !resolved.value.IsNegative()
}

// resolveSeries matches every series a plan bound against the readings the
// snapshot carries.
//
// A binding with no reading is missing evidence: the rate exists, this
// evaluation simply was not handed it, so the evaluation waits. A reading of a
// different version is a disagreement rather than a gap — pricing against a
// version the plan never declared would quietly bill at the wrong rate — so it
// is a conflict.
func (input PricingInputSnapshot) resolveSeries(bindings []ReferenceSeriesBinding) (map[ReferenceSeriesKind]ReferenceSeriesValue, error) {
	resolved := make(map[ReferenceSeriesKind]ReferenceSeriesValue, len(bindings))
	for _, binding := range bindings {
		reading, found := input.seriesReading(binding.kind)
		if !found {
			return nil, fmt.Errorf("%w: no reading for %s", ErrMissingReferenceSeriesValue, binding.kind)
		}
		if compareVersionReferences(reading.reference, binding.reference) != 0 {
			return nil, fmt.Errorf("%w: %s reading is %s but the plan bound %s",
				ErrReferenceSeriesVersionConflict, binding.kind, reading.reference.ID(), binding.reference.ID())
		}
		resolved[binding.kind] = reading
	}
	return resolved, nil
}

func (input PricingInputSnapshot) seriesReading(kind ReferenceSeriesKind) (ReferenceSeriesValue, bool) {
	for _, reading := range input.seriesValues {
		if reading.kind == kind {
			return reading, true
		}
	}
	return ReferenceSeriesValue{}, false
}

// describeRate names both halves of a series-sourced rate. CONTEXT: 燃油费率是
// 承运商当周公布费率与价卡折扣系数的乘积，两者都必须写入版本清单，只保留乘积结果
// 视为解释不完整 — with only the product, a reader cannot tell a carrier rate
// change from a renegotiated discount.
func (calculation SurchargeCalculation) describeRate(series map[ReferenceSeriesKind]ReferenceSeriesValue) string {
	if calculation.seriesKind == nil || calculation.seriesFactor == nil {
		return ""
	}
	reading, resolved := series[*calculation.seriesKind]
	if !resolved {
		return ""
	}
	return fmt.Sprintf(" at published %s %s × card factor %s",
		*calculation.seriesKind, reading.value.String(), calculation.seriesFactor.String())
}

// effectiveRate is the published reading multiplied by the card's own factor.
// CONTEXT requires both to reach the version manifest and the explanation:
// keeping only the product would leave a reader unable to tell a rate change
// from a discount change.
func (calculation SurchargeCalculation) effectiveRate(reading ReferenceSeriesValue) (Decimal, error) {
	if calculation.seriesFactor == nil {
		return Decimal{}, ErrInvalidSurchargeRule
	}
	return reading.value.Mul(*calculation.seriesFactor)
}
