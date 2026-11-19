package domain_test

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 本文件证登记里的一格十进制数值（包材外廓的边长与重量）：只认 `12` 或 `12.50` 这种写法，整数部分至多 9 位、小数部分至多 6 位；数值按数值比，
// 不按写法比；回显用去掉多余前导零与尾零的写法。

func measureOf(t *testing.T, text string) domain.Measure {
	t.Helper()
	measure, err := domain.NewMeasure(text)
	if err != nil {
		t.Fatalf("NewMeasure(%q)：%v", text, err)
	}
	return measure
}

// Covers: AT-WF-004（包材的部分）「回显用去掉多余前导零与尾零的写法」——合规的写法都收，规范写法去掉整数部分多余的前导零与小数部分的尾零，
// 整数末尾的零不动（100 还是 100），零一律写成 0。
func TestAMeasureIsWrittenWithoutExtraLeadingOrTrailingZeros(t *testing.T) {
	for written, canonical := range map[string]string{
		"12":               "12",
		"12.50":            "12.5",
		"12.00":            "12",
		"012":              "12",
		"0012.5000":        "12.5",
		"0.25":             "0.25",
		"00.250":           "0.25",
		"0.5":              "0.5",
		"0.000001":         "0.000001",
		"0.100000":         "0.1",
		"100":              "100",
		"100.0":            "100",
		"1000.500":         "1000.5",
		"15.5":             "15.5",
		"000000012":        "12",
		"123456789":        "123456789",
		"123456789.123456": "123456789.123456",
		"999999999.000000": "999999999",
		"0":                "0",
		"0.0":              "0",
		"000":              "0",
		"0.000000":         "0",
		"000000000.000000": "0",
	} {
		got, err := domain.NewMeasure(written)
		if err != nil {
			t.Errorf("NewMeasure(%q)：%v，want 收下", written, err)
			continue
		}
		if got.String() != canonical {
			t.Errorf("NewMeasure(%q).String() = %q，want %q", written, got.String(), canonical)
		}
	}
}

// Covers: AT-WF-004（包材的部分）「数值按数值比，不按写法比」——同一个数的各种写法构造出同一个值（直接用 == 比），不同的数是不同的值，
// 整数末尾的零不是尾零（100 与 1 不同）。
func TestMeasuresOfTheSameNumberAreEqualWhateverTheWriting(t *testing.T) {
	same := [][]string{
		{"12.5", "12.50", "012.5", "0012.500000", "12.500"},
		{"12", "12.0", "012", "12.000000", "000000012"},
		{"0.25", "00.25", "0.250", "0.250000"},
		{"0", "0.0", "000", "0.000000"},
		{"100", "100.0", "0100", "100.000000"},
	}
	for _, writings := range same {
		first := measureOf(t, writings[0])
		for _, other := range writings[1:] {
			if got := measureOf(t, other); got != first {
				t.Errorf("%q 与 %q 是同一个数，却构造出不同的值：%+v、%+v", writings[0], other, first, got)
			}
		}
	}
	for _, pair := range [][2]string{
		{"100", "1"}, {"12.5", "125"}, {"12.5", "1.25"}, {"0.1", "0.01"}, {"0.000001", "0"}, {"12.5", "12.500001"},
	} {
		if measureOf(t, pair[0]) == measureOf(t, pair[1]) {
			t.Errorf("%q 与 %q 是不同的数，却构造出同一个值", pair[0], pair[1])
		}
	}
}

// Covers: AT-WF-004（包材的部分）数值写法的各种坏载荷——负号、正号、指数、千分位、只有小数点、小数点一边没有数字、第二个小数点、空串、空白（首尾与
// 内部）、非 ASCII 的数字、十六进制与别的文字，一律答 ErrMalformedMeasure，错误里不带取值。
func TestAMeasureOutsideTheWritingIsMalformedAndTheErrorDoesNotEchoIt(t *testing.T) {
	for name, written := range map[string]string{
		"负号":         "-12",
		"负零":         "-0",
		"正号":         "+12",
		"小写指数":       "1e3",
		"大写指数":       "1E3",
		"千分位":        "1,000",
		"空格千分位":      "1 000",
		"只有小数点":      ".",
		"小数点前没有数字":   ".5",
		"小数点后没有数字":   "5.",
		"两个小数点":      "1.2.3",
		"空串":         "",
		"只有空白":       "   ",
		"首部空白":       " 12",
		"尾部空白":       "12 ",
		"内部空白":       "1 2",
		"换行":         "12\n",
		"全角数字":       "１２",
		"阿拉伯-印度数字":   "٣",
		"十六进制":       "0x10",
		"下划线分隔":      "1_000",
		"NaN":        "NaN",
		"Infinity":   "Infinity",
		"字母":         "abc",
		"数字夹字母":      "12a",
		"小数部分带符号":    "1.-5",
		"小数点后带空白":    "1. 5",
		"小数点用逗号":     "1,5",
		"带单位":        "12cm",
		"JSON null":  "null",
		"JSON 数字的写法": "12e0",
	} {
		_, err := domain.NewMeasure(written)
		if !errors.Is(err, domain.ErrMalformedMeasure) {
			t.Errorf("%s（%q）：err = %v，want ErrMalformedMeasure", name, written, err)
			continue
		}
		if written != "" && strings.Contains(err.Error(), written) {
			t.Errorf("%s：错误正文回显了取值 %q：%q", name, written, err.Error())
		}
	}
}

// Covers: AT-WF-004（包材的部分）「整数部分至多 9 位，小数部分至多 6 位」——位数按写出来的位数数，前导零与尾零也算；刚好够的收，多一位的答
// ErrMeasureTooLong。写法与位数都不对时先答写法。
func TestAMeasureWithTooManyDigitsIsTooLong(t *testing.T) {
	for written, accepted := range map[string]bool{
		"123456789":         true,
		"123456789.123456":  true,
		"000000012":         true,
		"0.123456":          true,
		"12.500000":         true,
		"1234567890":        false,
		"0123456789":        false,
		"0000000012":        false,
		"1.1234567":         false,
		"12.5000000":        false,
		"0.0000000":         false,
		"1234567890.123456": false,
		"123456789.1234567": false,
	} {
		_, err := domain.NewMeasure(written)
		switch {
		case accepted && err != nil:
			t.Errorf("NewMeasure(%q)：%v，want 收下", written, err)
		case !accepted && !errors.Is(err, domain.ErrMeasureTooLong):
			t.Errorf("NewMeasure(%q)：err = %v，want ErrMeasureTooLong", written, err)
		case !accepted && strings.Contains(err.Error(), written):
			t.Errorf("错误正文回显了取值 %q：%q", written, err.Error())
		}
	}
	if _, err := domain.NewMeasure("-1234567890"); !errors.Is(err, domain.ErrMalformedMeasure) {
		t.Errorf("写法与位数都不对：err = %v，want 先答写法 ErrMalformedMeasure", err)
	}
	if domain.MeasureMaxIntegerDigits != 9 || domain.MeasureMaxFractionDigits != 6 {
		t.Errorf("位数上限 = %d、%d，want 9、6（库里的 numeric(15, 6) 与它对得上）", domain.MeasureMaxIntegerDigits, domain.MeasureMaxFractionDigits)
	}
}

// Covers: AT-WF-004（包材的部分）数值为零——零的各种写法都是零；正数不是零；没经构造器的零值不是一个数：它不是零，也没有写法。
func TestAMeasureKnowsWhetherItIsZeroAndAnUnconstructedOneIsNotANumber(t *testing.T) {
	for _, written := range []string{"0", "0.0", "000", "0.000000", "000000000.000000"} {
		if !measureOf(t, written).IsZero() {
			t.Errorf("%q 该是零", written)
		}
	}
	for _, written := range []string{"0.000001", "1", "100", "0.5", "999999999.999999"} {
		if measureOf(t, written).IsZero() {
			t.Errorf("%q 不该是零", written)
		}
	}
	var unconstructed domain.Measure
	if unconstructed.IsZero() || unconstructed.String() != "" {
		t.Errorf("零值 = (IsZero %v, %q)，want 不是零、没有写法", unconstructed.IsZero(), unconstructed.String())
	}
	if unconstructed == measureOf(t, "0") {
		t.Errorf("没经构造器的零值与构造出来的零被当成了同一个值")
	}
}
