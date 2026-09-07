package domain

import (
	"errors"
	"strings"
	"testing"
)

// 本文件钉住 ADR-0123 落地后的形状：Decimal 的规范写法是 valid() 的一部分，同一个数在字段层只有一种
// 写法；算术一律经 decimalFromBig 落成规范写法；重建边界 decimalFrom 按字段原样构造、由 valid() 判写法，
// 非规范写法随整图重验一起被拒。
//
// 它的前身钉的是缺口——valid() 不拒尾随零、两种写法进摘要是两个串、decimalFrom 原样收回、percentShare
// 真会产出——那四格在票 wiring-baseline-remainder/07 里逐条取证过，缺口补上那天前提失效，按它们自己
// 写明的「若此处变红说明 valid() 已收紧，本测试的前提要重写」改成现在这样。

// TestValidRejectsEverySecondSpellingOfTheSameNumber 钉住第一格：valid() 是规范性检查。
//
// 无前导零、零无标度两条原本就在；ADR-0123 补上「标度大于零时系数不以零结尾」与「零不带负号」。标度为零
// 时系数末位的零是数字本身的一部分（100 就是 100），不在此列。
func TestValidRejectsEverySecondSpellingOfTheSameNumber(t *testing.T) {
	for _, spelling := range []Decimal{
		{coefficient: "1"},
		{coefficient: "100"},
		{coefficient: "15", scale: 1},
		{coefficient: "-15", scale: 1},
		{coefficient: "0"},
		{coefficient: "1", scale: DecimalMaxScale},
	} {
		if !spelling.valid() {
			t.Fatalf("规范写法 coefficient=%q scale=%d 本应通过 valid()", spelling.coefficient, spelling.scale)
		}
	}
	for _, spelling := range []Decimal{
		{coefficient: "100", scale: 2},
		{coefficient: "150", scale: 2},
		{coefficient: "-150", scale: 2},
		{coefficient: "10", scale: 1},
		{coefficient: "-0"},
		{coefficient: "0", scale: 1},
		{coefficient: "01"},
	} {
		if spelling.valid() {
			t.Fatalf("非规范写法 coefficient=%q scale=%d 竟通过了 valid()", spelling.coefficient, spelling.scale)
		}
		if spelling.String() != "" {
			t.Fatalf("立不住的写法 String() 应为空，实际 %q", spelling.String())
		}
	}
}

// TestSameNumberArrivesAtOneSpellingFromEveryPath 钉住第二格，也是要害：不同路径算出同一个数，字段写法
// 逐字相同，于是 String() 相同——语义摘要按 String() 取值，同一个数只可能是一个串。
func TestSameNumberArrivesAtOneSpellingFromEveryPath(t *testing.T) {
	paths := map[string]func() (Decimal, error){
		"文本入口宽收 1.50": func() (Decimal, error) { return ParseDecimal("1.50") },
		"文本入口宽收 01.5": func() (Decimal, error) { return ParseDecimal("01.5") },
		"percentShare(150)": func() (Decimal, error) {
			product, err := ParseDecimal("150")
			if err != nil {
				return Decimal{}, err
			}
			return percentShare(product)
		},
		"15 × 0.1": func() (Decimal, error) {
			left, err := ParseDecimal("15")
			if err != nil {
				return Decimal{}, err
			}
			right, err := ParseDecimal("0.1")
			if err != nil {
				return Decimal{}, err
			}
			return left.Mul(right)
		},
		"0.75 + 0.750": func() (Decimal, error) {
			left, err := ParseDecimal("0.75")
			if err != nil {
				return Decimal{}, err
			}
			right, err := ParseDecimal("0.750")
			if err != nil {
				return Decimal{}, err
			}
			return left.Add(right)
		},
		"3.00 − 1.5": func() (Decimal, error) {
			left, err := ParseDecimal("3.00")
			if err != nil {
				return Decimal{}, err
			}
			right, err := ParseDecimal("1.5")
			if err != nil {
				return Decimal{}, err
			}
			return left.Sub(right)
		},
	}
	for name, path := range paths {
		value, err := path()
		if err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		if value.coefficient != "15" || value.scale != 1 {
			t.Fatalf("%s 落成 coefficient=%q scale=%d，想要唯一的规范写法 15/1", name, value.coefficient, value.scale)
		}
		if value.String() != "1.5" {
			t.Fatalf("%s 的 String() = %q，想要 1.5", name, value.String())
		}
	}
}

// TestDecimalFromKeepsTheFieldsAndLetsValidJudgeTheSpelling 钉住第三格：重建边界仍按字段原样构造、不规范化，
// 写法由 valid() 判。非规范写法重建出来的是一个立不住的 Decimal，整图重验时随整份快照一起被拒——不是被
// 悄悄改成规范写法（那会把新出现的非规范产出者藏到第一次重放才露头）。
func TestDecimalFromKeepsTheFieldsAndLetsValidJudgeTheSpelling(t *testing.T) {
	canonical := decimalFrom(decimalSnapshot{Coefficient: "1"})
	if !canonical.valid() || canonical.String() != "1" {
		t.Fatalf("规范写法重建后应立得住且为 1，实际 valid=%v String=%q", canonical.valid(), canonical.String())
	}

	rebuilt := decimalFrom(decimalSnapshot{Coefficient: "100", Scale: 2})
	if rebuilt.coefficient != "100" || rebuilt.scale != 2 {
		t.Fatalf("重建应当原样接收字段，实际 coefficient=%q scale=%d", rebuilt.coefficient, rebuilt.scale)
	}
	if rebuilt.valid() {
		t.Fatal("非规范写法重建后竟立得住——valid() 没有在判写法")
	}
	if _, err := NewMoney(rebuilt, Currency{code: "USD"}); !errors.Is(err, ErrInvalidMoney) {
		t.Fatalf("非规范写法进 NewMoney 应被拒，实际 err = %v", err)
	}
}

// TestPercentShareProducesTheCanonicalSpelling 钉住第四格：生产路径上曾经唯一按字段移位的产出者改走
// decimalFromBig，系数末位为零时不再产出尾随零那种写法；除以一百仍只是小数点移位，数一个都没变。
func TestPercentShareProducesTheCanonicalSpelling(t *testing.T) {
	for _, tc := range []struct {
		product, want string
	}{
		{"100", "1"},
		{"150", "1.5"},
		{"12345", "123.45"},
		{"0.5", "0.005"},
		{"-200", "-2"},
		{"0", "0"},
	} {
		product, err := ParseDecimal(tc.product)
		if err != nil {
			t.Fatalf("解析 %s：%v", tc.product, err)
		}
		share, err := percentShare(product)
		if err != nil {
			t.Fatalf("percentShare(%s)：%v", tc.product, err)
		}
		if share.String() != tc.want {
			t.Fatalf("percentShare(%s) = %q，想要 %s", tc.product, share.String(), tc.want)
		}
		expected, err := ParseDecimal(tc.want)
		if err != nil {
			t.Fatalf("解析 %s：%v", tc.want, err)
		}
		if share.coefficient != expected.coefficient || share.scale != expected.scale {
			t.Fatalf("percentShare(%s) 的字段 = %q/%d，与文本入口给出的规范写法 %q/%d 不同",
				tc.product, share.coefficient, share.scale, expected.coefficient, expected.scale)
		}
	}

	// 标度上限守在同一处：移位后越界仍是精度越界，不是别的错。
	deep, err := ParseDecimal("0." + strings.Repeat("0", DecimalMaxScale-2) + "1")
	if err != nil {
		t.Fatalf("解析标度 %d 的数：%v", DecimalMaxScale-1, err)
	}
	if _, err := percentShare(deep); !errors.Is(err, ErrDecimalPrecisionExceeded) {
		t.Fatalf("移位越过标度上限应报 ErrDecimalPrecisionExceeded，实际 %v", err)
	}
}
