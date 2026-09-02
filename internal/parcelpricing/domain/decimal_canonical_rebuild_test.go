package domain

import "testing"

// 本文件回答一个取证问题：`ParseCanonical` 的注释说它用在序列化边界上，「那里不允许同一个数
// 的不同写法产生不同的内容摘要」，而重建边界 `decimalFrom` 直接按字段构造、不解析也不校验。
// 那道后置门（`evaluation.valid()` 的语义摘要自校）拦不拦得住非规范写法，决定这是一处真缺口
// 还是一句该改的注释。

// TestNonCanonicalPairSurvivesValid 钉住第一格：`valid()` 不是规范性检查。
//
// 它拒绝前导零、拒绝「系数为零而标度非零」，唯独不拒绝**标度大于零时的尾随零**——而那恰恰是
// `ParseDecimal` 会规范掉的那一种。于是同一个数存在两种都能通过 `valid()` 的内部表示。
func TestNonCanonicalPairSurvivesValid(t *testing.T) {
	canonical, err := ParseDecimal("1")
	if err != nil {
		t.Fatalf("解析规范写法失败：%v", err)
	}
	nonCanonical := Decimal{coefficient: "100", scale: 2}

	if !nonCanonical.valid() {
		t.Fatalf("非规范写法本应通过 valid()，实际被拒——若此处变红说明 valid() 已收紧，本测试的前提要重写")
	}
	if canonical.Cmp(nonCanonical) != 0 {
		t.Fatalf("两者应当是同一个数：%q 与 %q", canonical.String(), nonCanonical.String())
	}
}

// TestSameNumberTwoSpellingsHashDifferently 钉住第二格，也是要害：**同一个数的两种写法进摘要
// 得到不同的串**。
//
// 语义摘要按 `String()` 取值（见 fingerprint.go 里评价输入各维的取法），而两种写法的 `String()`
// 本就不同。所以后置门那道自校**守不住这一格**：它比对的是「摘要与本图是否自洽」，不是「本图
// 的数是不是规范写法」。一份非规范但自洽的快照能整套通过。
func TestSameNumberTwoSpellingsHashDifferently(t *testing.T) {
	canonical, err := ParseDecimal("1")
	if err != nil {
		t.Fatalf("解析规范写法失败：%v", err)
	}
	nonCanonical := Decimal{coefficient: "100", scale: 2}

	if canonical.String() == nonCanonical.String() {
		t.Fatalf("两种写法的 String() 竟相同，本测试的前提不成立：%q", canonical.String())
	}
	if canonical.Cmp(nonCanonical) != 0 {
		t.Fatalf("前提不成立：两者不是同一个数")
	}
}

// TestDecimalFromRebuildsWithoutNormalising 钉住第三格：重建边界原样接收那两个字段。
//
// `decimalFrom` 不解析也不规范化，因此上面那种写法只要进了快照就会原样回到内存里。它自己
// 不是缺陷——缺陷取决于有没有东西在别处产出非规范写法。
func TestDecimalFromRebuildsWithoutNormalising(t *testing.T) {
	rebuilt := decimalFrom(decimalSnapshot{Coefficient: "100", Scale: 2})

	if rebuilt.coefficient != "100" || rebuilt.scale != 2 {
		t.Fatalf("重建应当原样接收，实际得到 coefficient=%q scale=%d", rebuilt.coefficient, rebuilt.scale)
	}
	if rebuilt.String() != "1.00" {
		t.Fatalf("重建后的写法应为 1.00，实际 %q", rebuilt.String())
	}
}

// TestScaleShiftProducesNonCanonicalPair 钉住第四格，也是把前三格从「理论上可构造」变成
// 「生产路径上真的会产出」的那一格。
//
// `charge_dependency_execution.go` 里那处按字段直接构造（系数照抄、标度加二）不走任何规范化。
// 系数末位为零时，它产出的正是尾随零那种非规范写法。
func TestScaleShiftProducesNonCanonicalPair(t *testing.T) {
	product, err := ParseDecimal("100")
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}

	shifted := Decimal{coefficient: product.coefficient, scale: product.scale + 2}

	if !shifted.valid() {
		t.Fatalf("移位结果本应通过 valid()")
	}
	canonical, err := ParseDecimal(shifted.String())
	if err != nil {
		t.Fatalf("按其字面重解失败：%v", err)
	}
	if canonical.coefficient == shifted.coefficient && canonical.scale == shifted.scale {
		t.Fatalf("移位结果竟已是规范写法，本测试的前提要重写：coefficient=%q scale=%d",
			shifted.coefficient, shifted.scale)
	}
}
