package domain

type LengthUnit string

const (
	LengthUnitInch       LengthUnit = "IN"
	LengthUnitCentimeter LengthUnit = "CM"
)

func NewLengthUnit(value string) (LengthUnit, error) {
	unit := LengthUnit(value)
	if !unit.valid() {
		return "", ErrInvalidLengthUnit
	}
	return unit, nil
}

func (unit LengthUnit) String() string { return string(unit) }

func (unit LengthUnit) valid() bool {
	switch unit {
	case LengthUnitInch, LengthUnitCentimeter:
		return true
	default:
		return false
	}
}

type Length struct {
	value Decimal
	unit  LengthUnit
}

func NewLength(value Decimal, unit LengthUnit) (Length, error) {
	if !value.valid() || !unit.valid() {
		return Length{}, ErrInvalidLength
	}
	return Length{value: value, unit: unit}, nil
}

func (length Length) Value() Decimal   { return length.value }
func (length Length) Unit() LengthUnit { return length.unit }

func (length Length) valid() bool {
	return length.value.valid() && length.unit.valid()
}

// Volume 是一个立方量。Unit 报出底层的长度单位，所以单位为 LengthUnitInch 的 Volume
// 表示的是多少立方英寸。
type Volume struct {
	value Decimal
	unit  LengthUnit
}

func newVolume(value Decimal, unit LengthUnit) (Volume, error) {
	if !value.valid() || !unit.valid() {
		return Volume{}, ErrInvalidDimensions
	}
	return Volume{value: value, unit: unit}, nil
}

// NewVolume 构造一个被声明出来、而不是派生出来的体积——价卡的超限阈值本身就是以立方量
// 写明的，不是从三边算出来的。
func NewVolume(value Decimal, unit LengthUnit) (Volume, error) {
	return newVolume(value, unit)
}

func (volume Volume) Value() Decimal   { return volume.value }
func (volume Volume) Unit() LengthUnit { return volume.unit }

func (volume Volume) valid() bool {
	return volume.value.valid() && volume.unit.valid()
}

// Dimensions 保存包裹的三边，按实际大小排序而不是按声明顺序。价卡的尺寸阈值一律写作
// 「最长边」与「次长边」，所以这个排序是值的一部分，不是每条规则各自重新推导的东西。
type Dimensions struct {
	longest  Decimal
	second   Decimal
	shortest Decimal
	unit     LengthUnit
}

func NewDimensions(length, width, height Decimal, unit LengthUnit) (Dimensions, error) {
	if !unit.valid() {
		return Dimensions{}, ErrInvalidDimensions
	}
	sides := [3]Decimal{length, width, height}
	for _, side := range sides {
		if !side.valid() || side.Sign() <= 0 {
			return Dimensions{}, ErrInvalidDimensions
		}
	}
	for outer := 0; outer < len(sides); outer++ {
		for inner := outer + 1; inner < len(sides); inner++ {
			if sides[inner].Cmp(sides[outer]) > 0 {
				sides[outer], sides[inner] = sides[inner], sides[outer]
			}
		}
	}
	return Dimensions{longest: sides[0], second: sides[1], shortest: sides[2], unit: unit}, nil
}

func (dimensions Dimensions) LongestSide() Length {
	return Length{value: dimensions.longest, unit: dimensions.unit}
}

func (dimensions Dimensions) SecondLongestSide() Length {
	return Length{value: dimensions.second, unit: dimensions.unit}
}

func (dimensions Dimensions) ShortestSide() Length {
	return Length{value: dimensions.shortest, unit: dimensions.unit}
}

// LengthPlusGirth 是卡自己的公式，即长加围：最长边加其余两边各两倍。
func (dimensions Dimensions) LengthPlusGirth() (Length, error) {
	if !dimensions.valid() {
		return Length{}, ErrInvalidDimensions
	}
	two := NewDecimalFromInt64(2)
	total := dimensions.longest
	for _, side := range []Decimal{dimensions.second, dimensions.shortest} {
		doubled, err := side.Mul(two)
		if err != nil {
			return Length{}, err
		}
		total, err = total.Add(doubled)
		if err != nil {
			return Length{}, err
		}
	}
	return NewLength(total, dimensions.unit)
}

// Volume 是三边之积，并携带三边度量所用的单位。
func (dimensions Dimensions) Volume() (Volume, error) {
	if !dimensions.valid() {
		return Volume{}, ErrInvalidDimensions
	}
	product := dimensions.longest
	for _, side := range []Decimal{dimensions.second, dimensions.shortest} {
		multiplied, err := product.Mul(side)
		if err != nil {
			return Volume{}, err
		}
		product = multiplied
	}
	return newVolume(product, dimensions.unit)
}

func (dimensions Dimensions) Unit() LengthUnit { return dimensions.unit }

func (dimensions Dimensions) valid() bool {
	return dimensions.unit.valid() &&
		dimensions.longest.valid() && dimensions.second.valid() && dimensions.shortest.valid() &&
		dimensions.shortest.Sign() > 0 &&
		dimensions.longest.Cmp(dimensions.second) >= 0 && dimensions.second.Cmp(dimensions.shortest) >= 0
}
