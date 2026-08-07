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

// Volume is a cubic measure. Unit reports the underlying length unit, so a
// Volume in LengthUnitInch is a count of cubic inches.
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

func (volume Volume) Value() Decimal   { return volume.value }
func (volume Volume) Unit() LengthUnit { return volume.unit }

func (volume Volume) valid() bool {
	return volume.value.valid() && volume.unit.valid()
}

// Dimensions holds a package's three sides ordered by magnitude rather than by
// the order they were declared in. The rate card's dimension thresholds all
// read "longest side" and "second longest side", so the ordering is part of the
// value, not something each rule re-derives.
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

// LengthPlusGirth is the card's own formula: longest side plus twice each of
// the two remaining sides.
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

// Volume is the product of the three sides, carrying the unit the sides were
// measured in.
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
