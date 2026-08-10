package domain

import "fmt"

// ReferenceSeriesValue 是一期序列取值，按计价基准时点解析并冻结进评价的输入。
//
// ADR-0013 把这些序列的登记与版本化交给计价，但数值不归计价生产：燃油费率由承运商公布，
// 汇率来自财务侧。取值因此随快照进来，而不是在评价过程中现取——现取会让重放读到序列
// 今天的值。
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

// NewQuotedReferenceSeriesValue 携带声明该取值口径的商业价格政策版本。CONTEXT：汇率
// 口径——牌价类型、取值时点规则和加点规则——由商业价格政策版本化声明；不接受未声明口径
// 的裸汇率。一个没有口径的数字事后无从争辩，因为没人说得出它本该是哪个汇率。
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
	// 燃油费率不需要口径依据：它的折扣系数写在卡上。汇率需要，而卡对它没有发言权。
	if resolved.kind == ReferenceSeriesExchangeRate && resolved.quoteBasis == nil {
		return false
	}
	return true
}

// resolveSeries 把方案绑定的每一个序列与快照携带的取值逐一对上。
//
// 绑定了却没有取值是证据不足：费率是存在的，只是这次评价没拿到，所以评价保持`待判断`。
// 拿到的是另一个版本的取值则是分歧而不是缺口——按方案从未声明过的版本计价，会静默地
// 用错误的费率收费——所以那是`冲突`。
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

// describeRate 分别写出来自序列的费率的两半。CONTEXT：燃油费率是承运商当周公布费率与
// 价卡折扣系数的乘积，两者都必须写入版本清单，只保留乘积结果视为解释不完整——只有乘积
// 时，读的人分不清是承运商调了费率还是重新谈了折扣。
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

// ConversionStep 记录把卡本币换算为合同结算币种的过程。CONTEXT 要求两侧都留下：换算
// 必须保留原币金额与所引用的汇率序列版本，只保留结算币种金额视为解释不完整——争议是按
// 原币争的，所以只留换算后数字的评价答不上来。
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

// convertAmount 按已解析的汇率把一笔金额换算成结算币种。
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

// effectiveRate 是公布取值乘以卡自己的折扣系数。CONTEXT 要求两者都进入版本清单和解释：
// 只留乘积会让读的人分不清是费率变了还是折扣变了。
func (calculation SurchargeCalculation) effectiveRate(reading ReferenceSeriesValue) (Decimal, error) {
	if calculation.seriesFactor == nil {
		return Decimal{}, ErrInvalidSurchargeRule
	}
	return reading.value.Mul(*calculation.seriesFactor)
}
