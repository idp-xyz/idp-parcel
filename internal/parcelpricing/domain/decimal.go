package domain

import (
	"math/big"
	"strings"
)

// Decimal is an immutable, non-exponential base-10 number. The coefficient is
// kept as text so copying a Decimal can never alias a mutable big.Int.
type Decimal struct {
	coefficient string
	scale       uint32
}

const (
	// The limits are technical protection limits, not a currency scale.
	DecimalMaxDigits     = 256
	DecimalMaxScale      = 128
	DecimalMaxTextLength = DecimalMaxDigits + DecimalMaxScale + 2
)

func NewDecimal(raw string) (Decimal, error) {
	return ParseDecimal(raw)
}

func ParseDecimal(raw string) (Decimal, error) {
	value := strings.TrimSpace(raw)
	if value == "" || strings.ContainsAny(value, "eE") {
		return Decimal{}, ErrInvalidDecimal
	}
	if len(value) > DecimalMaxTextLength {
		return Decimal{}, ErrDecimalPrecisionExceeded
	}

	negative := false
	if value[0] == '+' || value[0] == '-' {
		negative = value[0] == '-'
		value = value[1:]
	}
	if value == "" || strings.Count(value, ".") > 1 {
		return Decimal{}, ErrInvalidDecimal
	}

	integerPart, fractionalPart := value, ""
	if dot := strings.IndexByte(value, '.'); dot >= 0 {
		integerPart, fractionalPart = value[:dot], value[dot+1:]
		if integerPart == "" {
			integerPart = "0"
		}
		if fractionalPart == "" {
			return Decimal{}, ErrInvalidDecimal
		}
	}
	if !allDigits(integerPart) || (fractionalPart != "" && !allDigits(fractionalPart)) {
		return Decimal{}, ErrInvalidDecimal
	}
	if len(fractionalPart) > DecimalMaxScale {
		return Decimal{}, ErrDecimalPrecisionExceeded
	}

	digits := strings.TrimLeft(integerPart+fractionalPart, "0")
	if digits == "" {
		return Decimal{coefficient: "0"}, nil
	}
	scale := uint32(len(fractionalPart))
	for scale > 0 && strings.HasSuffix(digits, "0") {
		digits = strings.TrimSuffix(digits, "0")
		scale--
	}
	if len(digits) > DecimalMaxDigits {
		return Decimal{}, ErrDecimalPrecisionExceeded
	}
	if negative {
		digits = "-" + digits
	}
	return Decimal{coefficient: digits, scale: scale}, nil
}

// ParseCanonical accepts only the canonical decimal representation emitted by
// String. This is useful at serialization boundaries where alternate spellings
// must not produce different content digests.
func ParseCanonical(raw string) (Decimal, error) {
	value, err := ParseDecimal(raw)
	if err != nil || value.String() != raw {
		return Decimal{}, ErrInvalidDecimal
	}
	return value, nil
}

func NewDecimalFromInt64(value int64) Decimal {
	if value == 0 {
		return Decimal{coefficient: "0"}
	}
	return Decimal{coefficient: big.NewInt(value).String()}
}

func DecimalFromInt64(value int64) Decimal {
	return NewDecimalFromInt64(value)
}

func (value Decimal) valid() bool {
	if value.scale > DecimalMaxScale || value.coefficient == "" {
		return false
	}
	coefficient := value.coefficient
	if coefficient[0] == '-' {
		coefficient = coefficient[1:]
	}
	if coefficient == "" || !allDigits(coefficient) || len(coefficient) > DecimalMaxDigits {
		return false
	}
	if coefficient != "0" && strings.HasPrefix(coefficient, "0") {
		return false
	}
	if coefficient == "0" && value.scale != 0 {
		return false
	}
	return true
}

func (value Decimal) String() string {
	if !value.valid() {
		return ""
	}
	negative := strings.HasPrefix(value.coefficient, "-")
	digits := value.coefficient
	if negative {
		digits = digits[1:]
	}
	if value.scale == 0 {
		if negative {
			return "-" + digits
		}
		return digits
	}
	if len(digits) <= int(value.scale) {
		digits = strings.Repeat("0", int(value.scale)-len(digits)+1) + digits
	}
	point := len(digits) - int(value.scale)
	result := digits[:point] + "." + digits[point:]
	if negative {
		return "-" + result
	}
	return result
}

func (value Decimal) CanonicalString() string {
	return value.String()
}

func (value Decimal) IsZero() bool {
	return value.valid() && value.coefficient == "0"
}

func (value Decimal) IsNegative() bool {
	return value.valid() && strings.HasPrefix(value.coefficient, "-")
}

func (value Decimal) Sign() int {
	if !value.valid() || value.IsZero() {
		return 0
	}
	if value.IsNegative() {
		return -1
	}
	return 1
}

func (value Decimal) Equal(other Decimal) bool {
	return value.valid() && other.valid() && value.Cmp(other) == 0
}

func (value Decimal) Compare(other Decimal) int {
	return value.Cmp(other)
}

func (value Decimal) Cmp(other Decimal) int {
	if !value.valid() || !other.valid() {
		return 0
	}
	commonScale := value.scale
	if other.scale > commonScale {
		commonScale = other.scale
	}
	left := value.scaledCoefficient(commonScale)
	right := other.scaledCoefficient(commonScale)
	return left.Cmp(right)
}

func (value Decimal) Add(other Decimal) (Decimal, error) {
	return value.combine(other, false)
}

func (value Decimal) Sub(other Decimal) (Decimal, error) {
	return value.combine(other, true)
}

func (value Decimal) Mul(other Decimal) (Decimal, error) {
	if !value.valid() || !other.valid() {
		return Decimal{}, ErrInvalidDecimal
	}
	left := value.bigCoefficient()
	right := other.bigCoefficient()
	left.Mul(left, right)
	return decimalFromBig(left, value.scale+other.scale)
}

func (value Decimal) Abs() Decimal {
	if !value.IsNegative() {
		return value
	}
	return Decimal{coefficient: strings.TrimPrefix(value.coefficient, "-"), scale: value.scale}
}

func (value Decimal) RoundToIncrement(increment Decimal, mode RoundingMode) (Decimal, error) {
	if !value.valid() || !increment.valid() || value.IsNegative() || increment.Sign() <= 0 {
		return Decimal{}, ErrInvalidRoundingPolicy
	}
	if !mode.valid() {
		return Decimal{}, ErrInvalidRoundingPolicy
	}
	if mode == RoundingNone {
		return value, nil
	}

	commonScale := value.scale
	if increment.scale > commonScale {
		commonScale = increment.scale
	}
	dividend := value.scaledCoefficient(commonScale)
	divisor := increment.scaledCoefficient(commonScale)
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(dividend, divisor, remainder)
	if remainder.Sign() != 0 {
		switch mode {
		case RoundingCeiling:
			quotient.Add(quotient, big.NewInt(1))
		}
	}
	result := new(big.Int).Mul(quotient, divisor)
	return decimalFromBig(result, commonScale)
}

func (value Decimal) combine(other Decimal, subtract bool) (Decimal, error) {
	if !value.valid() || !other.valid() {
		return Decimal{}, ErrInvalidDecimal
	}
	commonScale := value.scale
	if other.scale > commonScale {
		commonScale = other.scale
	}
	left := value.scaledCoefficient(commonScale)
	right := other.scaledCoefficient(commonScale)
	if subtract {
		left.Sub(left, right)
	} else {
		left.Add(left, right)
	}
	return decimalFromBig(left, commonScale)
}

func (value Decimal) bigCoefficient() *big.Int {
	coefficient := value.coefficient
	result := new(big.Int)
	result.SetString(coefficient, 10)
	return result
}

func (value Decimal) scaledCoefficient(scale uint32) *big.Int {
	result := value.bigCoefficient()
	if scale > value.scale {
		result.Mul(result, pow10(scale-value.scale))
	}
	return result
}

func decimalFromBig(coefficient *big.Int, scale uint32) (Decimal, error) {
	if coefficient == nil {
		return Decimal{}, ErrDecimalPrecisionExceeded
	}
	if coefficient.Sign() == 0 {
		return Decimal{coefficient: "0"}, nil
	}
	text := coefficient.String()
	negative := strings.HasPrefix(text, "-")
	digits := text
	if negative {
		digits = digits[1:]
	}
	for scale > 0 && strings.HasSuffix(digits, "0") {
		digits = strings.TrimSuffix(digits, "0")
		scale--
	}
	if scale > DecimalMaxScale {
		return Decimal{}, ErrDecimalPrecisionExceeded
	}
	if len(digits) > DecimalMaxDigits {
		return Decimal{}, ErrDecimalPrecisionExceeded
	}
	if negative {
		digits = "-" + digits
	}
	return Decimal{coefficient: digits, scale: scale}, nil
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func pow10(scale uint32) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), new(big.Int).SetUint64(uint64(scale)), nil)
}
