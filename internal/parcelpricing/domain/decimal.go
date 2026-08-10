package domain

import (
	"math/big"
	"strings"
)

// Decimal 是不可变、非指数表示的十进制数。系数以文本保存，因此复制一个 Decimal 绝不会
// 与某个可变的 big.Int 共享底层。
type Decimal struct {
	coefficient string
	scale       uint32
}

const (
	// 这些上限是技术保护限制，不是币种的小数位数。
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

// ParseCanonical 只接受 String 输出的那种规范十进制表示。它用在序列化边界上——
// 那里不允许同一个数的不同写法产生不同的内容摘要。
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

// DivRoundToIncrement 先除以 divisor，再把商落到 increment 的整数倍上。两步合成一次
// 运算，是因为精确商不一定是有限小数：先做除法会给中间结果强加一个没人声明过的精度。
// 在放大后的整数上运算，能让结果对调用方声明的取整方式保持精确。RoundingNone 因同一个
// 理由被拒绝——根本不存在一个有限的商可以留着不进位。
func (value Decimal) DivRoundToIncrement(divisor, increment Decimal, mode RoundingMode) (Decimal, error) {
	if !value.valid() || !divisor.valid() || !increment.valid() || value.IsNegative() || divisor.Sign() <= 0 || increment.Sign() <= 0 {
		return Decimal{}, ErrInvalidRoundingPolicy
	}
	if !mode.valid() || mode == RoundingNone {
		return Decimal{}, ErrInvalidRoundingPolicy
	}
	// 结果就是 value / divisor 里装得下多少个进位单位。
	numerator := new(big.Int).Mul(value.bigCoefficient(), pow10(divisor.scale+increment.scale))
	denominator := new(big.Int).Mul(divisor.bigCoefficient(), increment.bigCoefficient())
	denominator.Mul(denominator, pow10(value.scale))
	multiples, remainder := new(big.Int), new(big.Int)
	multiples.QuoRem(numerator, denominator, remainder)
	if remainder.Sign() != 0 && mode == RoundingCeiling {
		multiples.Add(multiples, big.NewInt(1))
	}
	return decimalFromBig(multiples.Mul(multiples, increment.bigCoefficient()), increment.scale)
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
