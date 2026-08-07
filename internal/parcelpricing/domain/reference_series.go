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
	kind       ReferenceSeriesKind
	reference  VersionReference
	value      Decimal
	quoteBasis *VersionReference
}

func NewReferenceSeriesValue(kind ReferenceSeriesKind, reference VersionReference, value Decimal) (ReferenceSeriesValue, error) {
	resolved := ReferenceSeriesValue{kind: kind, reference: reference, value: value}
	if !resolved.valid() {
		return ReferenceSeriesValue{}, ErrInvalidReferenceSeries
	}
	return resolved, nil
}

// NewQuotedReferenceSeriesValue carries the commercial policy version that
// declares how the reading was quoted. CONTEXT: 汇率口径——牌价类型、取值时点规则
// 和加点规则——由商业价格政策版本化声明；不接受未声明口径的裸汇率. A number with no
// stated basis cannot be argued about later, because nobody can say which rate
// it was supposed to be.
func NewQuotedReferenceSeriesValue(kind ReferenceSeriesKind, reference VersionReference, value Decimal, quoteBasis VersionReference) (ReferenceSeriesValue, error) {
	resolved := ReferenceSeriesValue{kind: kind, reference: reference, value: value, quoteBasis: &quoteBasis}
	if !resolved.valid() {
		return ReferenceSeriesValue{}, ErrInvalidReferenceSeries
	}
	return resolved, nil
}

func (resolved ReferenceSeriesValue) Kind() ReferenceSeriesKind   { return resolved.kind }
func (resolved ReferenceSeriesValue) Reference() VersionReference { return resolved.reference }
func (resolved ReferenceSeriesValue) Value() Decimal              { return resolved.value }

func (resolved ReferenceSeriesValue) QuoteBasis() (VersionReference, bool) {
	if resolved.quoteBasis == nil {
		return VersionReference{}, false
	}
	return *resolved.quoteBasis, true
}

func (resolved ReferenceSeriesValue) valid() bool {
	if !resolved.kind.valid() ||
		resolved.reference.kind != ArtifactReferenceSeries || !resolved.reference.valid() ||
		!resolved.value.valid() || resolved.value.IsNegative() {
		return false
	}
	if resolved.quoteBasis != nil {
		if resolved.quoteBasis.kind != ArtifactCommercialPolicy || !resolved.quoteBasis.valid() {
			return false
		}
	}
	// A fuel rate needs no quote basis: its discount factor is stated on the
	// card. An exchange rate does, and the card has no say in it.
	if resolved.kind == ReferenceSeriesExchangeRate && resolved.quoteBasis == nil {
		return false
	}
	return true
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

// ConversionStep records turning the card's own currency into the currency the
// contract settles in. CONTEXT keeps both sides: 换算必须保留原币金额与所引用的
// 汇率序列版本，只保留结算币种金额视为解释不完整 — a dispute is argued in the
// original currency, so an evaluation holding only the converted figure cannot
// answer it.
type ConversionStep struct {
	original  Money
	rate      Decimal
	series    VersionReference
	converted Money
}

func (step ConversionStep) Original() Money                   { return step.original }
func (step ConversionStep) Rate() Decimal                     { return step.rate }
func (step ConversionStep) SeriesReference() VersionReference { return step.series }
func (step ConversionStep) Converted() Money                  { return step.converted }

func (step ConversionStep) valid() bool {
	return step.original.valid() && step.converted.valid() &&
		step.rate.valid() && step.rate.Sign() > 0 &&
		step.series.kind == ArtifactReferenceSeries && step.series.valid() &&
		step.original.currency != step.converted.currency
}

// convert turns an amount into the settlement currency at the resolved rate.
func convertAmount(original Money, reading ReferenceSeriesValue, settlement Currency) (ConversionStep, error) {
	if !original.valid() || !reading.valid() || !settlement.valid() {
		return ConversionStep{}, ErrInvalidReferenceSeries
	}
	product, err := original.amount.Mul(reading.value)
	if err != nil {
		return ConversionStep{}, err
	}
	converted, err := NewMoney(product, settlement)
	if err != nil {
		return ConversionStep{}, err
	}
	step := ConversionStep{original: original, rate: reading.value, series: reading.reference, converted: converted}
	if !step.valid() {
		return ConversionStep{}, ErrInvalidReferenceSeries
	}
	return step, nil
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
