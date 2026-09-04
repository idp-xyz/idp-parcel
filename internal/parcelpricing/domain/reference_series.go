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
	// currency 只在金额序列的取值上有（ADR-0110 Decision 一）：费率是裸小数，金额带币种。
	currency *Currency
	// absent 记「在用版本在计价基准时点没有期次」（ADR-0110 Decision 三「窗外无期次」）：版本引用在、取值缺席。
	// 只对金额序列成立——费率序列没有「窗外」这一格，缺期次就是缺证据。
	absent bool
}

func NewReferenceSeriesValue(kind ReferenceSeriesKind, reference VersionReference, value Decimal) (ReferenceSeriesValue, error) {
	resolved := ReferenceSeriesValue{kind: kind, reference: reference, value: value}
	if !resolved.valid() {
		return ReferenceSeriesValue{}, ErrInvalidReferenceSeries
	}
	return resolved, nil
}

// NewPublishedAmountSeriesValue 是金额序列的一期取值：带币种的金额，币种是否与方案一致在评价里判。
func NewPublishedAmountSeriesValue(reference VersionReference, amount Money) (ReferenceSeriesValue, error) {
	currency := amount.currency
	resolved := ReferenceSeriesValue{kind: ReferenceSeriesPublishedAmount, reference: reference, value: amount.amount, currency: &currency}
	if !resolved.valid() {
		return ReferenceSeriesValue{}, ErrInvalidReferenceSeries
	}
	return resolved, nil
}

// NewAbsentSeriesReading 记下「查过这一版金额序列、基准时点不在任何期次内」。它冻结进评价输入，让窗外行为
// 在纯函数里按卡的声明分流，且重放能落在同一个结论上；根本没有在用版本时不造这条读数——那是缺口不是窗外。
func NewAbsentSeriesReading(kind ReferenceSeriesKind, reference VersionReference) (ReferenceSeriesValue, error) {
	resolved := ReferenceSeriesValue{kind: kind, reference: reference, absent: true}
	if !resolved.valid() {
		return ReferenceSeriesValue{}, ErrInvalidReferenceSeries
	}
	return resolved, nil
}

// Amount 只在金额序列的已解出取值上给出。
func (resolved ReferenceSeriesValue) Amount() (Money, bool) {
	if resolved.currency == nil || resolved.absent {
		return Money{}, false
	}
	return Money{amount: resolved.value, currency: *resolved.currency}, true
}

// Absent 说明这条读数是「查过、无期次」。
func (resolved ReferenceSeriesValue) Absent() bool { return resolved.absent }

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
		resolved.reference.kind != ArtifactReferenceSeries || !resolved.reference.valid() {
		return false
	}
	if resolved.absent {
		// 缺席读数只对金额序列成立，且不带取值、币种与口径。
		return resolved.kind.carriesAmount() && resolved.value == (Decimal{}) && resolved.currency == nil && resolved.quoteBasis == nil
	}
	if !resolved.value.valid() || resolved.value.IsNegative() {
		return false
	}
	// 金额序列的取值带币种，费率序列的不带（ADR-0110 Decision 一）。
	if resolved.kind.carriesAmount() != (resolved.currency != nil) {
		return false
	}
	if resolved.currency != nil && !resolved.currency.valid() {
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
// 拿到的取值来自另一条序列则是分歧而不是缺口——按方案从未声明过的序列计价，会静默地
// 用错误的费率收费——所以那是`冲突`。版本不在这里比：方案绑的是序列标识（ADR-0099），
// 同一条序列的任何一版都可以是这次评价用的那一版，用了哪一版由评价清单冻结。
func (input PricingInputSnapshot) resolveSeries(bindings []ReferenceSeriesBinding) (map[ReferenceSeriesKind]ReferenceSeriesValue, map[string]ReferenceSeriesValue, error) {
	rates := make(map[ReferenceSeriesKind]ReferenceSeriesValue, len(bindings))
	amounts := make(map[string]ReferenceSeriesValue, len(bindings))
	for _, binding := range bindings {
		reading, found := input.boundReading(binding)
		if !found {
			return nil, nil, &missingSeriesReadingError{
				binding: binding,
				message: fmt.Sprintf("%s: no reading for %s %s", ErrMissingReferenceSeriesValue.Error(), binding.kind, binding.seriesID),
			}
		}
		if reading.reference.ID() != binding.seriesID {
			return nil, nil, fmt.Errorf("%w: %s reading comes from series %s but the plan bound series %s",
				ErrReferenceSeriesVersionConflict, binding.kind, reading.reference.ID(), binding.seriesID)
		}
		if binding.kind.carriesAmount() {
			amounts[binding.seriesID] = reading
			continue
		}
		rates[binding.kind] = reading
	}
	return rates, amounts, nil
}

// missingSeriesReadingError 是「缺一期序列取值」带着它缺的那条绑定（ADR-0105 Context 点名：resolveSeries 在
// 失败那一刻手上有整条绑定，此前只把它格式化进了文字）。它 Is ErrMissingReferenceSeriesValue——既有的分流
// 判据不变；问题项的主体从这里取，不从文字反解析。文字与此前逐字相同，因为它进规范化文档。
type missingSeriesReadingError struct {
	binding ReferenceSeriesBinding
	message string
}

func (err *missingSeriesReadingError) Error() string { return err.message }

func (err *missingSeriesReadingError) Is(target error) bool {
	return target == ErrMissingReferenceSeriesValue
}

// boundSeriesReferences 列出快照里落在方案绑定上的序列版本引用——这些是本次评价实际
// 采用的版本，要进评价自己的清单。落在绑定之外的取值不进清单：方案没声明过的序列不
// 参与计价，冻结它会让清单说了一件评价没做的事。窗外无期次的缺席读数照样进清单：查过就是采用过。
func (input PricingInputSnapshot) boundSeriesReferences(bindings []ReferenceSeriesBinding) []VersionReference {
	references := make([]VersionReference, 0, len(bindings))
	for _, binding := range bindings {
		reading, found := input.boundReading(binding)
		if found && reading.reference.ID() == binding.seriesID {
			references = append(references, reading.reference)
		}
	}
	return references
}

// boundReading 交回落在某条绑定上的读数：费率序列按种类找（每种至多一条），金额序列按标识找（同种可多条）。
// 费率种类下读数来自别的序列时照样交回，让调用方把「绑错了」报成冲突而不是缺口。
func (input PricingInputSnapshot) boundReading(binding ReferenceSeriesBinding) (ReferenceSeriesValue, bool) {
	if binding.kind.carriesAmount() {
		return input.amountReading(binding.seriesID)
	}
	return input.seriesReading(binding.kind)
}

func (input PricingInputSnapshot) seriesReading(kind ReferenceSeriesKind) (ReferenceSeriesValue, bool) {
	for _, reading := range input.seriesValues {
		if reading.kind == kind {
			return reading, true
		}
	}
	return ReferenceSeriesValue{}, false
}

func (input PricingInputSnapshot) amountReading(seriesID string) (ReferenceSeriesValue, bool) {
	for _, reading := range input.seriesValues {
		if reading.kind.carriesAmount() && reading.reference.ID() == seriesID {
			return reading, true
		}
	}
	return ReferenceSeriesValue{}, false
}

// ReadingKey 是一条读数在快照里的去重键，也是它与方案绑定对上的键：费率序列按种类（一张卡每种一条），金额
// 序列按（种类，标识）——同一条金额序列两期取值会把选哪一个交给遍历顺序决定，而两条不同的金额序列各是各的。
// 编排层补齐读数时用同一个键，规则只在这里定一次。
func (resolved ReferenceSeriesValue) ReadingKey() string {
	return seriesReadingKey(resolved.kind, resolved.reference.ID())
}

// ReadingKey 是这条绑定要对上的读数键，与 ReferenceSeriesValue.ReadingKey 同一条规则。
func (binding ReferenceSeriesBinding) ReadingKey() string {
	return seriesReadingKey(binding.kind, binding.seriesID)
}

func seriesReadingKey(kind ReferenceSeriesKind, seriesID string) string {
	if kind.carriesAmount() {
		return kind.String() + "|" + seriesID
	}
	return kind.String()
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

// describeAmountSource 写出取当期序列定额的金额取自哪条序列哪一版（ADR-0110 Consequences）。其它计算交回空串。
func (calculation SurchargeCalculation) describeAmountSource(amounts map[string]ReferenceSeriesValue) string {
	if calculation.method != ChargeMethodSeriesAmount {
		return ""
	}
	reading, found := amounts[calculation.seriesID]
	if !found || reading.absent {
		return ""
	}
	return fmt.Sprintf(" taken from published amount series %s@%s", reading.reference.ID(), reading.reference.Version())
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
