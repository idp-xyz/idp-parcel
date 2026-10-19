package domain

import (
	"errors"
	"math/big"
	"slices"
	"strings"
)

// ErrInvalidDemandSnapshot：需求量快照不成立——来源不认得、同一维度出现两次、数量不是正十进制或单位缺席。
var ErrInvalidDemandSnapshot = errors.New("transport fulfillment: invalid demand snapshot")

// DemandSource 说需求量快照取自哪里。今天只内置收寄前的客户申报一种（ADR-0201 决定八）：节点实测后的再准备另立票，
// 那时多一个来源词，原有快照的来源不被改写——同一段同一包裹按申报与按实测各判一次，是两份可以并排看的依据。
type DemandSource uint8

const (
	DemandSourceInvalid DemandSource = iota
	DemandFromDeclaration
)

// String 写出来源词。用计划与 ADR 原文的 `declaration`，与计价侧申报测量引用的写法同一个词：同一份客户申报在两个上下文
// 里叫同一个名字，对账的人不必翻译。
func (source DemandSource) String() string {
	switch source {
	case DemandFromDeclaration:
		return "declaration"
	default:
		return ""
	}
}

// ParseDemandSource 把 String 写出的词认回来，供读回走构造门。
func ParseDemandSource(word string) (DemandSource, error) {
	if word == DemandFromDeclaration.String() {
		return DemandFromDeclaration, nil
	}
	return DemandSourceInvalid, ErrInvalidDemandSnapshot
}

// DemandQuantity 是需求量快照在一个容量维度上的数量与单位。数量是正十进制串，按原样保存：申报的「1.50」不折成浮点，
// 也不预先换成哪个池的单位——同一份快照要与单位各异的几个池比，换算只在比的那一刻按池的单位做。
type DemandQuantity struct {
	Dimension CapacityDimension
	Amount    string
	Unit      QuantityUnitReference
}

func (quantity DemandQuantity) valid() bool {
	_, ok := parsePositiveDecimal(quantity.Amount)
	return quantity.Dimension.valid() && ok && quantity.Unit.valid()
}

// DemandSnapshot 是一个载运对象此刻的各维需求量（UC-TF-003「需求量快照」）：来源、资料版本锚与逐维数量。某一维没有
// 数量就是那一维`待补充`——快照不替它填默认值，也不用别的维度折算出来（ADR-0201 决定八「跨维不折算」）。
type DemandSnapshot struct {
	source     DemandSource
	anchor     string
	quantities []DemandQuantity
}

// RehydrateDemandSnapshot 是读回一份快照的门：来源可认、各维数量成立、同一维度只出现一次。锚可缺席——客户没报或
// 提供方此刻说不出按哪一版时，快照如实不带锚。
func RehydrateDemandSnapshot(source DemandSource, anchor string, quantities []DemandQuantity) (DemandSnapshot, error) {
	if source != DemandFromDeclaration || strings.IndexByte(anchor, 0) >= 0 {
		return DemandSnapshot{}, ErrInvalidDemandSnapshot
	}
	seen := map[CapacityDimension]bool{}
	normalized := make([]DemandQuantity, 0, len(quantities))
	for _, quantity := range quantities {
		if !quantity.valid() || seen[quantity.Dimension] {
			return DemandSnapshot{}, ErrInvalidDemandSnapshot
		}
		seen[quantity.Dimension] = true
		normalized = append(normalized, quantity)
	}
	slices.SortFunc(normalized, func(left, right DemandQuantity) int {
		return CompareCapacityDimensions(left.Dimension, right.Dimension)
	})
	return DemandSnapshot{source: source, anchor: strings.TrimSpace(anchor), quantities: normalized}, nil
}

// DeclaredDemandSpec 是客户申报里与需求量有关的原文：毛重一格、外廓三边一格，各自可整格缺席；锚是提供方答出的资料
// 版本锚，缺席即空串。数量与单位都按提供方的拼写原样交来，认不认得那个单位在比的那一刻才判。
type DeclaredDemandSpec struct {
	Anchor        string
	WeightValue   string
	WeightUnit    string
	Length        string
	Width         string
	Height        string
	DimensionUnit string
}

// DeclaredDemand 把一件包裹的客户申报译成需求量快照（ADR-0201 决定八，产品策略）。只认三个维度：
//
//   - 重量取申报毛重，数量与单位照抄；
//   - 体积取外廓三边的乘积，单位写作长度单位后接「3」（CM → CM3）——三边共用一个单位是申报本身的约束；
//   - 件数恒为 1 件：一件声明包裹就是一件，这一维的来源是对象本身。它只配件数维的池，不顶替重量或体积，那正是 UC-TF-003
//     「不用对象数量……替代实际容量需求」要防的。
//
// 袋位、托盘位、舱位在申报里没有来源，快照不带它们，比到那一维的池时就是`待补充`。
func DeclaredDemand(spec DeclaredDemandSpec) (DemandSnapshot, error) {
	var quantities []DemandQuantity
	if spec.WeightValue != "" || spec.WeightUnit != "" {
		unit, err := NewQuantityUnitReference(spec.WeightUnit)
		if err != nil {
			return DemandSnapshot{}, ErrInvalidDemandSnapshot
		}
		quantities = append(quantities, DemandQuantity{Dimension: CapacityWeight, Amount: spec.WeightValue, Unit: unit})
	}
	if spec.Length != "" || spec.Width != "" || spec.Height != "" || spec.DimensionUnit != "" {
		volume, err := boxVolume(spec.Length, spec.Width, spec.Height)
		if err != nil {
			return DemandSnapshot{}, err
		}
		unit, err := NewQuantityUnitReference(spec.DimensionUnit + volumeUnitSuffix)
		if err != nil || strings.TrimSpace(spec.DimensionUnit) == "" {
			return DemandSnapshot{}, ErrInvalidDemandSnapshot
		}
		quantities = append(quantities, DemandQuantity{Dimension: CapacityVolume, Amount: volume, Unit: unit})
	}
	quantities = append(quantities, DemandQuantity{
		Dimension: CapacityPieces,
		Amount:    "1",
		Unit:      QuantityUnitReference{requiredValue{value: piecesUnit}},
	})
	return RehydrateDemandSnapshot(DemandFromDeclaration, spec.Anchor, quantities)
}

func (snapshot DemandSnapshot) Source() DemandSource {
	return snapshot.source
}

// Anchor 交回资料版本锚；第二个返回值为假表示快照不带锚，不是「锚读不到」。
func (snapshot DemandSnapshot) Anchor() (string, bool) {
	return snapshot.anchor, snapshot.anchor != ""
}

// Quantities 按维度在封闭词里的次序交回逐维数量的拷贝。
func (snapshot DemandSnapshot) Quantities() []DemandQuantity {
	return append([]DemandQuantity(nil), snapshot.quantities...)
}

func (snapshot DemandSnapshot) valid() bool {
	return snapshot.source == DemandFromDeclaration
}

func (snapshot DemandSnapshot) quantityFor(dimension CapacityDimension) (DemandQuantity, bool) {
	for _, quantity := range snapshot.quantities {
		if quantity.Dimension == dimension {
			return quantity, true
		}
	}
	return DemandQuantity{}, false
}

// volumeUnitSuffix 与 piecesUnit 是快照自己写下的两个单位拼写：体积由三边乘出，单位只能由长度单位派生；件数由对象本身
// 得出，没有提供方拼写可抄。
const (
	volumeUnitSuffix = "3"
	piecesUnit       = "PCS"
)

// measureKind 是单位的量纲。同一维度内按量纲换算，跨维度不折算（ADR-0201 决定八）：重量池只认质量单位，体积池只认
// 体积单位，件数池只认计数单位。
type measureKind uint8

const (
	measureNone measureKind = iota
	measureMass
	measureVolume
	measureCount
)

func dimensionMeasure(dimension CapacityDimension) measureKind {
	switch dimension {
	case CapacityWeight:
		return measureMass
	case CapacityVolume:
		return measureVolume
	case CapacityPieces:
		return measureCount
	default:
		return measureNone
	}
}

// unitFactors 是准备评估内置认得的单位与它们相对本量纲基准单位（千克、立方厘米、件）的倍数，属产品策略那一格：
// 换算系数是公制与英制的定义值，不是租户取值。拼写按大写原样比，不折大小写——与计价侧单位表同一纪律，`kg` 认不出就
// 答`容量待确认`并点名单位，比替登记方猜一个单位好查。要多认一个单位是产品改动，随规则版本走。
var unitFactors = map[string]struct {
	kind   measureKind
	factor string
}{
	"G":   {measureMass, "1/1000"},
	"KG":  {measureMass, "1"},
	"T":   {measureMass, "1000"},
	"LB":  {measureMass, "0.45359237"},
	"OZ":  {measureMass, "0.028349523125"},
	"MM3": {measureVolume, "1/1000"},
	"CM3": {measureVolume, "1"},
	"L":   {measureVolume, "1000"},
	"M3":  {measureVolume, "1000000"},
	"IN3": {measureVolume, "16.387064"},
	"FT3": {measureVolume, "28316.846592"},
	"PCS": {measureCount, "1"},
}

// convertQuantity 把一个维度上的数量从 from 单位换到 to 单位。两个单位都得认得、且都属这一维的量纲，否则答不出——
// 那一格是`单位不可换算`，不是`不够`。
func convertQuantity(dimension CapacityDimension, amount *big.Rat, from, to QuantityUnitReference) (*big.Rat, bool) {
	kind := dimensionMeasure(dimension)
	source, sourceKnown := unitFactors[from.String()]
	target, targetKnown := unitFactors[to.String()]
	if kind == measureNone || !sourceKnown || !targetKnown || source.kind != kind || target.kind != kind {
		return nil, false
	}
	sourceFactor, _ := new(big.Rat).SetString(source.factor)
	targetFactor, _ := new(big.Rat).SetString(target.factor)
	converted := new(big.Rat).Mul(amount, sourceFactor)
	return converted.Quo(converted, targetFactor), true
}

// parsePositiveDecimal 只认「数字串」或「数字串.数字串」且不为零。big.Rat 自己还认分数与指数写法，那会让同一个量有几种
// 拼写进快照，摘要随拼写变。
func parsePositiveDecimal(raw string) (*big.Rat, bool) {
	whole, fraction, dotted := strings.Cut(raw, ".")
	if !allDigits(whole) || (dotted && !allDigits(fraction)) {
		return nil, false
	}
	value, ok := new(big.Rat).SetString(raw)
	if !ok || value.Sign() <= 0 {
		return nil, false
	}
	return value, true
}

func allDigits(raw string) bool {
	if raw == "" {
		return false
	}
	for _, character := range raw {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

// boxVolume 算三边乘积，写成精确的十进制串：三个有限小数之积仍是有限小数，小数位不多于三者小数位之和。
func boxVolume(length, width, height string) (string, error) {
	product := big.NewRat(1, 1)
	scale := 0
	for _, edge := range []string{length, width, height} {
		value, ok := parsePositiveDecimal(edge)
		if !ok {
			return "", ErrInvalidDemandSnapshot
		}
		if _, fraction, dotted := strings.Cut(edge, "."); dotted {
			scale += len(fraction)
		}
		product.Mul(product, value)
	}
	return trimDecimal(product.FloatString(scale)), nil
}

// trimDecimal 去掉小数部分末尾的零与悬空的小数点，让同一个量只有一种写法。
func trimDecimal(rendered string) string {
	if !strings.Contains(rendered, ".") {
		return rendered
	}
	rendered = strings.TrimRight(rendered, "0")
	return strings.TrimSuffix(rendered, ".")
}

// requestedDecimalPlaces 是换算后需求量写进依据快照时保留的小数位。比较用的是精确有理数，写下来的只是给人看的那一格：
// 英制换公制的商可能除不尽，总得在某一位停下。
const requestedDecimalPlaces = 6
