package domain

import (
	"errors"
	"strings"
)

const (
	// MeasureMaxIntegerDigits 与 MeasureMaxFractionDigits 是登记一格数值的位数上限：整数部分至多 9 位、小数部分至多 6 位，按写出来的位数数，
	// 前导零与尾零也算。库里的 numeric(15, 6) 恰好装得下这两个上限，所以写法过了这道门，落库就不会溢出。
	MeasureMaxIntegerDigits  = 9
	MeasureMaxFractionDigits = 6
)

var (
	// ErrMalformedMeasure 是数值的写法不成立：只认 `12` 或 `12.50` 这种十进制数——只有数字与至多一个小数点，小数点两边都要有数字；
	// 不带正负号、指数、千分位与空白。
	ErrMalformedMeasure = errors.New("warehouse fulfillment: malformed measure")
	// ErrMeasureTooLong 是写法对、但位数超了上限（见 MeasureMaxIntegerDigits 与 MeasureMaxFractionDigits）。
	ErrMeasureTooLong = errors.New("warehouse fulfillment: measure has too many digits")
)

// Measure 是登记里的一格十进制数值（包材外廓的三条边与重量）：精确十进制，不经浮点，只存数值不存写法——`12.5`、`12.50`、`012.5` 是同一个
// 数，构造出来就是同一个值，所以内容相同的判定直接用 ==（见 PackagingMaterialContent）。
//
// 内部是「系数文本 + 小数位数」的规范形：系数不带前导零，小数位数大于零时系数不以零结尾，零就是 {"0", 0}。整数末尾的零属于系数
// （`100` 的系数是 "100"），不会被当尾零去掉。零值（没经构造器）的系数是空串，与构造出来的零 {"0", 0} 分得开。
//
// Measure 只管「是一个写法合规、位数不超的非负数」。「为正」是登记时才拦的一道（只拦将要落册的新修订，读回与重放不经它，同国家码与
// 业务时区的做法），由登记用例判；库上另以 CHECK 兜住。
type Measure struct {
	coefficient string
	scale       uint8
}

// NewMeasure 按上面的写法解一格数值；写法不对答 ErrMalformedMeasure，位数超了答 ErrMeasureTooLong。错误里不带取值。
func NewMeasure(text string) (Measure, error) {
	whole, fraction, dotted := strings.Cut(text, ".")
	if !asciiDigits(whole) || (dotted && !asciiDigits(fraction)) {
		// 第二个小数点会留在 fraction 里，不是数字，同样落到这里。
		return Measure{}, ErrMalformedMeasure
	}
	if len(whole) > MeasureMaxIntegerDigits || len(fraction) > MeasureMaxFractionDigits {
		return Measure{}, ErrMeasureTooLong
	}

	digits := strings.TrimLeft(whole+fraction, "0")
	if digits == "" {
		return Measure{coefficient: "0"}, nil
	}
	scale := len(fraction)
	for scale > 0 && strings.HasSuffix(digits, "0") {
		digits = strings.TrimSuffix(digits, "0")
		scale--
	}
	return Measure{coefficient: digits, scale: uint8(scale)}, nil
}

// asciiDigits 只认 ASCII 的 0 至 9，且至少一位：别的文字里的数字、空串与空白都不是。
func asciiDigits(text string) bool {
	if text == "" {
		return false
	}
	for index := 0; index < len(text); index++ {
		if text[index] < '0' || text[index] > '9' {
			return false
		}
	}
	return true
}

func (measure Measure) valid() bool {
	return measure.coefficient != ""
}

// IsZero 回答这个数是不是零（含 0、0.0、000 这类写法）。没经构造器的零值答否：它不是一个数。
func (measure Measure) IsZero() bool {
	return measure.coefficient == "0"
}

// String 交回去掉多余前导零与尾零的写法：12.50 回显 12.5，12.00 回显 12，012 回显 12。没经构造器的零值交回空串。
func (measure Measure) String() string {
	if !measure.valid() {
		return ""
	}
	if measure.scale == 0 {
		return measure.coefficient
	}
	digits := measure.coefficient
	if len(digits) <= int(measure.scale) {
		digits = strings.Repeat("0", int(measure.scale)-len(digits)+1) + digits
	}
	point := len(digits) - int(measure.scale)
	return digits[:point] + "." + digits[point:]
}
