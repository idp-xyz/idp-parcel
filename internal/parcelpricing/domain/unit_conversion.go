package domain

import (
	"fmt"
	"math/big"
	"strings"

	"go.idp.xyz/idp-parcel/internal/platform/refdata"
)

// 本文件是评价内的单位换算（ADR-0346）：输入实重与尺寸的单位和方案声明的查表单位 / 体积系数长度单位不同时，
// 评价按产品内置的单位目录（refdata）把输入换算过去再走原有规则，而不是答单位不一致。
//
// 三条纪律：
//   - 单位本来就一致的评价不产生换算：EvaluatePricing 先走原函数，不构造换算器，没有换算步骤、没有解释行，
//     既有评价的字节一个不动。（读回门 validUnitConversions 与访问口 UnitConversions 对每个评价都会被调到，
//     换算记录为空时是空操作。）
//   - 换算只发生在 EvaluatePricing 里（calculatePricingWeight 带着 converter 才换）；直接调用
//     CalculatePricingWeight 的人仍然得到单位不一致，换算与否是评价这一层的事。
//   - 换算所用的系数记在评价上（UnitConversionStep），重放只认记下的系数、不再查目录——目录随发版会变，
//     而重放要重现的是当初那次评价。

// ConvertedQuantity 指出换算施加在评价的哪一个量上。
type ConvertedQuantity string

const (
	// ConvertedActualWeight 是输入实重；票级 / 主单级主体上是成员清单的合计实重。
	ConvertedActualWeight ConvertedQuantity = "ACTUAL_WEIGHT"
	// ConvertedTotalVolumetricWeight 是票级 / 主单级主体由调用方声明的合计体积重。
	ConvertedTotalVolumetricWeight ConvertedQuantity = "TOTAL_VOLUMETRIC_WEIGHT"
	// ConvertedDimensions 是包裹尺寸三边（最长、次长、最短）。
	ConvertedDimensions ConvertedQuantity = "DIMENSIONS"
)

func (quantity ConvertedQuantity) valid() bool {
	switch quantity {
	case ConvertedActualWeight, ConvertedTotalVolumetricWeight, ConvertedDimensions:
		return true
	default:
		return false
	}
}

func (quantity ConvertedQuantity) category() refdata.UnitCategory {
	if quantity == ConvertedDimensions {
		return refdata.UnitCategoryLength
	}
	return refdata.UnitCategoryWeight
}

func (quantity ConvertedQuantity) valueCount() int {
	if quantity == ConvertedDimensions {
		return 3
	}
	return 1
}

// dimensionsDisplayPlaces 是尺寸换算结果不是有限小数时，解释里展示换算后三边所截的小数位。
// 只用于展示：体积重由精确有理数算出，不读这几个截断值（见 volumetricWeightFromDimensions）。
const dimensionsDisplayPlaces = 16

// UnitConversionStep 是评价里一次单位换算的记录：原值与原单位、换算后值与目标单位、所用系数。
// 它随评价入册（只进快照，不进规范化文档——换算后的值已经落在摘要覆盖的计价重量里，见 ADR-0346 决定五），
// 重放凭它重算。
type UnitConversionStep struct {
	quantity ConvertedQuantity
	from     string
	to       string
	// factor 是「1 个 from 折多少个 to」的精确有理数文本：有限小数写十进制（0.45359237），否则写既约分数
	// （100000000/45359237）。取 refdata 目录的精确值，不经浮点。
	factor    string
	original  []Decimal
	converted []Decimal
	// exact 为假时 converted 是按 places 位小数向零截断的值（换算结果不是有限小数）；为真时 places 恒为零。
	exact  bool
	places uint32
}

func (step UnitConversionStep) Quantity() ConvertedQuantity { return step.quantity }
func (step UnitConversionStep) FromUnit() string            { return step.from }
func (step UnitConversionStep) ToUnit() string              { return step.to }
func (step UnitConversionStep) Factor() string              { return step.factor }
func (step UnitConversionStep) Original() []Decimal         { return append([]Decimal(nil), step.original...) }
func (step UnitConversionStep) Converted() []Decimal {
	return append([]Decimal(nil), step.converted...)
}

// Exact 说明换算后的值是否是精确值；为假时 Places 报出截断的小数位。
func (step UnitConversionStep) Exact() bool    { return step.exact }
func (step UnitConversionStep) Places() uint32 { return step.places }

func (step UnitConversionStep) explain() string {
	exactness := " (exact)"
	if !step.exact {
		tail := "; weight rounding is applied to this value and gives the same result as rounding the exact quotient"
		if step.quantity == ConvertedDimensions {
			tail = "; the volumetric weight was derived from the exact product, the converted sides are shown for reading only"
		}
		exactness = fmt.Sprintf(" (not a finite decimal: truncated toward zero at %d decimal places%s)", step.places, tail)
	}
	return fmt.Sprintf("unit conversion %s: %s %s -> %s %s at factor %s%s",
		step.quantity, joinDecimals(step.original), step.from, joinDecimals(step.converted), step.to, step.factor, exactness)
}

func joinDecimals(values []Decimal) string {
	texts := make([]string, 0, len(values))
	for _, value := range values {
		texts = append(texts, value.String())
	}
	return strings.Join(texts, " x ")
}

// valid 是读回门：记录自洽才算立得住——两端单位在目录里且与被换算的量同类别、系数是规范写法的正有理数、
// 换算后的值确实等于原值乘系数（精确或按记下的位数向零截断）。系数本身不对照当前目录：目录随发版会变，
// 记下的系数才是当初那次评价用的。
func (step UnitConversionStep) valid() bool {
	if !step.quantity.valid() || step.from == step.to {
		return false
	}
	from, found := refdata.Unit(step.from)
	if !found || from.Category != step.quantity.category() {
		return false
	}
	to, found := refdata.Unit(step.to)
	if !found || to.Category != step.quantity.category() {
		return false
	}
	factor, ok := parseCanonicalFactor(step.factor)
	if !ok {
		return false
	}
	if len(step.original) != step.quantity.valueCount() || len(step.converted) != step.quantity.valueCount() {
		return false
	}
	if step.exact != (step.places == 0) {
		return false
	}
	for index, original := range step.original {
		if !original.valid() || original.IsNegative() || !step.converted[index].valid() {
			return false
		}
		if step.quantity == ConvertedDimensions && original.Sign() <= 0 {
			return false
		}
		want, ok := convertValue(original, factor, step.exact, step.places)
		if !ok || !want.Equal(step.converted[index]) || want.String() != step.converted[index].String() {
			return false
		}
	}
	return true
}

// unitConverter 是一次评价里换算系数的来源与记录。frozen 为假时系数取自目录（首次评价）；为真时只认
// recorded 里记下的系数（重放、同标识重复请求），从不查目录。
type unitConverter struct {
	frozen   bool
	recorded []UnitConversionStep
	steps    []UnitConversionStep
	// conditionScale 是方案里读体积重的判定条件的阈值里最细的小数位：这些阈值要拿去比较可能被截断的换算后合计
	// 体积重，须并入截断位数的边界（ADR-0347 决定五）。方案不读体积重时为零，位数公式与此前逐字相同。
	conditionScale uint32
}

func newUnitConverter(frozen bool, recorded []UnitConversionStep) *unitConverter {
	return &unitConverter{frozen: frozen, recorded: recorded}
}

// factorFor 取 from 折 to 的精确系数。目录里没有、跨类别、或冻结模式下没记过，都答不出。
func (converter *unitConverter) factorFor(from, to string) (*big.Rat, string, bool) {
	if converter.frozen {
		for _, step := range converter.recorded {
			if step.from == from && step.to == to {
				factor, ok := parseCanonicalFactor(step.factor)
				return factor, step.factor, ok
			}
		}
		return nil, "", false
	}
	factor, err := refdata.Convert(big.NewRat(1, 1), from, to)
	if err != nil || factor.Sign() <= 0 {
		return nil, "", false
	}
	return factor, factorText(factor), true
}

// recordedFor 在冻结模式下取记下的那一条：同一个量、同一对单位。
func (converter *unitConverter) recordedFor(quantity ConvertedQuantity, from, to string) (UnitConversionStep, bool) {
	for _, step := range converter.recorded {
		if step.quantity == quantity && step.from == from && step.to == to {
			return step, true
		}
	}
	return UnitConversionStep{}, false
}

func (converter *unitConverter) record(step UnitConversionStep) {
	converter.steps = append(converter.steps, step)
}

func (converter *unitConverter) recordedSteps() []UnitConversionStep {
	return append([]UnitConversionStep(nil), converter.steps...)
}

// convertWeight 把重量换算到 to。换算结果不是有限小数时：不允许截断的取整策略（含 NONE 段）一律拒；
// 允许时按 guardPlaces 保留足够位数——见 guardPlaces 里「截断不改变任何取整与比较结果」的论证。
// 冻结模式（重放、同标识重复请求）连截断位数也只认记下的：摘要里的实重、进位前重量是截断后的十进制串，
// 日后调整 guardPlaces 或 conversionBoundaryScale，不能让既有评价重放出另一串。记下的位数不得小于当前公式推出的
// 下限——小于就是记录与当前对不上，不静默改用较小位数，按答不出处理；大于下限无妨（论证只要求不小于下限）。
// 答不出一律是原来的单位不一致，不另起一个错：错误文字进评价的问题项和语义摘要，冻结模式的重放必须得出同一句。
func (converter *unitConverter) convertWeight(quantity ConvertedQuantity, weight Weight, to WeightUnit, boundaryScale uint32, allowInexact bool) (Weight, error) {
	factor, text, ok := converter.factorFor(weight.unit.String(), to.String())
	if !ok {
		return Weight{}, ErrWeightUnitMismatch
	}
	product := new(big.Rat).Mul(decimalRat(weight.value), factor)
	step := UnitConversionStep{
		quantity: quantity,
		from:     weight.unit.String(),
		to:       to.String(),
		factor:   text,
		original: []Decimal{weight.value},
	}
	value, exact := ratToDecimalExact(product)
	if exact {
		step.exact = true
	} else {
		if !allowInexact {
			return Weight{}, ErrWeightUnitMismatch
		}
		places := guardPlaces(product, boundaryScale)
		if converter.frozen {
			recorded, found := converter.recordedFor(quantity, weight.unit.String(), to.String())
			if !found || recorded.exact || recorded.places < places {
				return Weight{}, ErrWeightUnitMismatch
			}
			places = recorded.places
		}
		truncated, ok := ratTruncate(product, places)
		if !ok {
			return Weight{}, ErrWeightUnitMismatch
		}
		value, step.places = truncated, places
	}
	converted, err := NewWeight(value, to)
	if err != nil {
		return Weight{}, ErrWeightUnitMismatch
	}
	step.converted = []Decimal{converted.value}
	converter.record(step)
	return converted, nil
}

// volumetricWeightFromDimensions 在尺寸单位与体积系数声明的长度单位不同时算体积重：三边乘系数的立方得到
// 精确体积（有理数），除以除数、落到声明的进位单位上一步完成——与 Decimal.DivRoundToIncrement 同一个理由：
// 精确商不一定是有限小数，先把三边各自换成十进制会给中间结果强加一个没人声明过的精度（CM 折 IN 就是
// 无限小数）。取整模式与进位单位原样取自体积系数，取整的判法与 DivRoundToIncrement 共用 roundsUp。
func (converter *unitConverter) volumetricWeightFromDimensions(sides Dimensions, factor VolumetricFactor) (Weight, error) {
	ratio, text, ok := converter.factorFor(sides.unit.String(), factor.lengthUnit.String())
	if !ok {
		return Weight{}, ErrLengthUnitMismatch
	}
	segment, sole := factor.rounding.sole()
	if !sole {
		return Weight{}, ErrInvalidVolumetricFactor
	}
	original := []Decimal{sides.longest, sides.second, sides.shortest}
	volume := big.NewRat(1, 1)
	converted := make([]Decimal, 0, len(original))
	allExact := true
	for _, side := range original {
		scaled := new(big.Rat).Mul(decimalRat(side), ratio)
		volume.Mul(volume, scaled)
		shown, exact := ratToDecimalExact(scaled)
		if !exact {
			allExact = false
		}
		converted = append(converted, shown)
	}
	step := UnitConversionStep{
		quantity: ConvertedDimensions,
		from:     sides.unit.String(),
		to:       factor.lengthUnit.String(),
		factor:   text,
		original: original,
		exact:    allExact,
	}
	if !allExact {
		step.places = dimensionsDisplayPlaces
		// 展示位数在冻结模式下也取记下的：它不进计算与摘要，但重放出来的记录该与原记录逐字相同。
		if recorded, found := converter.recordedFor(ConvertedDimensions, step.from, step.to); converter.frozen && found && !recorded.exact {
			step.places = recorded.places
		}
		converted = converted[:0]
		for _, side := range original {
			truncated, ok := ratTruncate(new(big.Rat).Mul(decimalRat(side), ratio), step.places)
			if !ok {
				return Weight{}, ErrLengthUnitMismatch
			}
			converted = append(converted, truncated)
		}
	}
	step.converted = converted
	value, err := roundRatToIncrement(volume, factor.divisor, segment.increment.value, segment.mode)
	if err != nil {
		return Weight{}, err
	}
	derived, err := NewWeight(value, segment.increment.unit)
	if err != nil {
		return Weight{}, err
	}
	converter.record(step)
	return derived, nil
}

// roundRatToIncrement 把精确有理数 quantity 除以 divisor 再落到 increment 的整数倍上。取整判法与
// Decimal.DivRoundToIncrement 同一个内核（roundsUp），只是被除数从十进制放宽成有理数——换算系数把体积
// 变成了有理数。
func roundRatToIncrement(quantity *big.Rat, divisor, increment Decimal, mode RoundingMode) (Decimal, error) {
	if quantity.Sign() < 0 || !divisor.valid() || !increment.valid() || divisor.Sign() <= 0 || increment.Sign() <= 0 {
		return Decimal{}, ErrInvalidRoundingPolicy
	}
	if !mode.valid() || mode == RoundingNone {
		return Decimal{}, ErrInvalidRoundingPolicy
	}
	multiples := new(big.Rat).Quo(quantity, decimalRat(divisor))
	multiples.Quo(multiples, decimalRat(increment))
	whole, remainder := new(big.Int), new(big.Int)
	whole.QuoRem(multiples.Num(), multiples.Denom(), remainder)
	if roundsUp(mode, remainder, multiples.Denom()) {
		whole.Add(whole, big.NewInt(1))
	}
	return decimalFromBig(whole.Mul(whole, increment.bigCoefficient()), increment.scale)
}

// guardPlaces 决定换算结果不是有限小数时向零截断保留的小数位。
//
// 论证（ADR-0346 决定四）：设精确值 W 化成既约分数后分母为 Q，boundaryScale 为 g——此后拿 W 比较或落点的
// 所有有限小数（取整进位单位的整数倍与其中点、分段上界、条件最低重量、体积重）的小数位都不超过 g。
// 任一这样的有限小数 B 与 W 不等时，|W−B| ≥ 1/(Q·10^g)（通分后分子是非零整数）。保留 S 位后截断值
// Wt 满足 0 ≤ W−Wt < 10^−S，只要 S > log10(Q)+g，就没有 B 落在 [Wt, W] 之间，于是 W 与 Wt 对每个 B
// 的大小关系相同——比较、选段、CEILING / HALF_UP 的取整结果与不截断时逐一相同。
// 取 S = Q 的十进制位数 + g + 1，严格大于 log10(Q)+g。
func guardPlaces(value *big.Rat, boundaryScale uint32) uint32 {
	return uint32(len(value.Denom().String())) + boundaryScale + 1
}

// decimalRat 把十进制数还原成精确有理数。
func decimalRat(value Decimal) *big.Rat {
	return new(big.Rat).SetFrac(value.bigCoefficient(), pow10(value.scale))
}

// ratToDecimalExact 在有理数是有限小数时交回精确的十进制数，否则第二个返回值为假。
func ratToDecimalExact(value *big.Rat) (Decimal, bool) {
	remaining := new(big.Int).Set(value.Denom())
	two, five := big.NewInt(2), big.NewInt(5)
	var twos, fives uint32
	quotient, remainder := new(big.Int), new(big.Int)
	for {
		quotient.QuoRem(remaining, two, remainder)
		if remainder.Sign() != 0 {
			break
		}
		remaining.Set(quotient)
		twos++
	}
	for {
		quotient.QuoRem(remaining, five, remainder)
		if remainder.Sign() != 0 {
			break
		}
		remaining.Set(quotient)
		fives++
	}
	if remaining.Cmp(big.NewInt(1)) != 0 {
		return Decimal{}, false
	}
	scale := twos
	if fives > scale {
		scale = fives
	}
	coefficient := new(big.Int).Mul(value.Num(), pow10(scale))
	coefficient.Quo(coefficient, value.Denom())
	result, err := decimalFromBig(coefficient, scale)
	return result, err == nil
}

// ratTruncate 把非负有理数向零截断到 places 位小数。
func ratTruncate(value *big.Rat, places uint32) (Decimal, bool) {
	coefficient := new(big.Int).Mul(value.Num(), pow10(places))
	coefficient.Quo(coefficient, value.Denom())
	result, err := decimalFromBig(coefficient, places)
	return result, err == nil
}

// convertValue 按记录的方式（精确或截断到 places 位）把原值乘系数，读回门拿它重算。
func convertValue(original Decimal, factor *big.Rat, exact bool, places uint32) (Decimal, bool) {
	product := new(big.Rat).Mul(decimalRat(original), factor)
	if exact {
		return ratToDecimalExact(product)
	}
	return ratTruncate(product, places)
}

// factorText 是系数的规范文本：有限小数写十进制，否则写既约分数。
func factorText(factor *big.Rat) string {
	if exact, ok := ratToDecimalExact(factor); ok {
		return exact.String()
	}
	return factor.String()
}

// parseCanonicalFactor 解系数文本。只收规范写法（再渲染必须逐字相同）：同一个系数在记录里只有一种写法，
// 与 Decimal 的规范写法同一条纪律（ADR-0123 决定一）。
func parseCanonicalFactor(text string) (*big.Rat, bool) {
	factor, ok := new(big.Rat).SetString(text)
	if !ok || factor.Sign() <= 0 || factorText(factor) != text {
		return nil, false
	}
	return factor, true
}

// conversionBoundaryScale 报出计价重量一路上要拿来比较或落点的有限小数里最细的小数位（guardPlaces 的 g）：
// 各取整段的进位单位（加一位，HALF_UP 的中点）与分段上界、条件最低重量、体积系数的进位单位、
// 成员清单声明的合计体积重。
func conversionBoundaryScale(input PricingInputSnapshot, policy PricingWeightPolicy, floors []Weight) uint32 {
	var scale uint32
	widen := func(value Decimal, extra uint32) {
		if value.scale+extra > scale {
			scale = value.scale + extra
		}
	}
	for _, segment := range policy.rounding.segments {
		widen(segment.increment.value, 1)
		if segment.hasMaximum {
			widen(segment.maximum.value, 0)
		}
	}
	for _, floor := range floors {
		widen(floor.value, 0)
	}
	if factor, declared := policy.VolumetricFactor(); declared {
		for _, segment := range factor.rounding.segments {
			widen(segment.increment.value, 0)
		}
	}
	if input.members != nil && input.members.totalVolumetric != nil {
		widen(input.members.totalVolumetric.value, 0)
	}
	return scale
}

// allowsTruncation 报出取整策略能不能容下一个截断过的重量：含 NONE 段的策略不进位，截断的值会直接进查表与
// 乘单价，那是静默改金额，一律不收。
func (policy WeightRoundingPolicy) allowsTruncation() bool {
	for _, segment := range policy.segments {
		if segment.mode == RoundingNone {
			return false
		}
	}
	return true
}

// UnitConversions 交回本次评价做过的每一次单位换算，按发生的先后。空即没有换算——单位本来就一致，
// 或这张评价没走到需要换算的那一步。
func (evaluation PricingEvaluation) UnitConversions() []UnitConversionStep {
	return append([]UnitConversionStep(nil), evaluation.unitConversions...)
}

// WithRecordedUnitConversions 让请求只认记下的换算系数、不查目录：同标识重复请求拿原评价的记录来比对
// 时用（原评价记的是当初那次的系数，目录此刻可能已随发版换了）。传空记录同样是「冻结」——此前答单位不一致
// 的评价照旧答单位不一致，而不是此刻换算后改判。
func (request EvaluationRequest) WithRecordedUnitConversions(steps []UnitConversionStep) EvaluationRequest {
	copyOfRequest := request
	copyOfRequest.conversionsFrozen = true
	copyOfRequest.recordedConversions = append([]UnitConversionStep(nil), steps...)
	return copyOfRequest
}

// validUnitConversions 是评价读回时对换算记录的整体验：每条自洽，每个量至多换一次，同一对单位只有一个系数，
// 记录的原值与输入对得上，换算后的实重与计价重量里的实重对得上。
func (evaluation PricingEvaluation) validUnitConversions() bool {
	seenQuantities := make(map[ConvertedQuantity]struct{}, len(evaluation.unitConversions))
	factors := make(map[[2]string]string, len(evaluation.unitConversions))
	for _, step := range evaluation.unitConversions {
		if !step.valid() {
			return false
		}
		if _, repeated := seenQuantities[step.quantity]; repeated {
			return false
		}
		seenQuantities[step.quantity] = struct{}{}
		pair := [2]string{step.from, step.to}
		if prior, exists := factors[pair]; exists && prior != step.factor {
			return false
		}
		factors[pair] = step.factor
		switch step.quantity {
		case ConvertedActualWeight:
			actual := evaluation.input.actualWeight
			if step.from != actual.unit.String() || !step.original[0].Equal(actual.value) {
				return false
			}
			if evaluation.pricingWeight != nil {
				converted := evaluation.pricingWeight.actual
				if step.to != converted.unit.String() || !step.converted[0].Equal(converted.value) {
					return false
				}
			}
		case ConvertedDimensions:
			sides, declared := evaluation.input.Dimensions()
			if !declared || step.from != sides.unit.String() ||
				!step.original[0].Equal(sides.longest) || !step.original[1].Equal(sides.second) || !step.original[2].Equal(sides.shortest) {
				return false
			}
		case ConvertedTotalVolumetricWeight:
			members, declared := evaluation.input.Members()
			if !declared {
				return false
			}
			total, declared := members.TotalVolumetricWeight()
			if !declared || step.from != total.unit.String() || !step.original[0].Equal(total.value) {
				return false
			}
		}
	}
	return true
}
