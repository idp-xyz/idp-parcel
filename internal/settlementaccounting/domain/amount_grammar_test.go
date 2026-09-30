package domain

import "testing"

func TestAmountGrammarSubtractsDeductibleThenScalesThenCaps(t *testing.T) {
	grammar := amountGrammar(t, 100_000, 5_000, 1_000)

	composition, err := grammar.Compose(10_000)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if composition.AfterDeductibleMinor() != 9_000 || composition.ScaledMinor() != 4_500 ||
		composition.AmountMinor() != 4_500 || composition.Capped() {
		t.Fatalf("展开 = 免赔后 %d 比例后 %d 金额 %d 封顶 %v，想要 9000 / 4500 / 4500 / 未封顶",
			composition.AfterDeductibleMinor(), composition.ScaledMinor(), composition.AmountMinor(), composition.Capped())
	}
	if !composition.FormsAmount() {
		t.Fatal("4500 应形成金额")
	}
}

func TestAmountGrammarCapsAfterTheRatio(t *testing.T) {
	grammar := amountGrammar(t, 5_000, 8_000, 1_000)

	composition, err := grammar.Compose(10_000)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if composition.ScaledMinor() != 7_200 || composition.AmountMinor() != 5_000 || !composition.Capped() {
		t.Fatalf("比例后 %d 金额 %d 封顶 %v，想要 7200 / 5000 / 已封顶",
			composition.ScaledMinor(), composition.AmountMinor(), composition.Capped())
	}
}

func TestAmountGrammarFloorsToTheMinorUnit(t *testing.T) {
	grammar := amountGrammar(t, 10_000, 3_333, 0)

	composition, err := grammar.Compose(100)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if composition.AmountMinor() != 33 || composition.Capped() {
		t.Fatalf("金额 %d 封顶 %v，想要 33 且未封顶", composition.AmountMinor(), composition.Capped())
	}
}

func TestAmountGrammarThatRoundsToZeroDoesNotFormAnAmount(t *testing.T) {
	grammar := amountGrammar(t, 10_000, 10_000, 100)

	composition, err := grammar.Compose(100)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if composition.AmountMinor() != 0 || composition.FormsAmount() {
		t.Fatalf("金额 %d forms %v，想要 0 且不形成", composition.AmountMinor(), composition.FormsAmount())
	}
}

func TestAmountGrammarRejectsValuesOutsideTheClosedRange(t *testing.T) {
	for _, values := range [][3]int64{
		{-1, 10_000, 0},
		{1, 10_001, 0},
		{1, -1, 0},
		{1, 10_000, -1},
	} {
		if _, err := NewAmountGrammar(values[0], values[1], values[2]); err == nil {
			t.Fatalf("取值 %v 被收下", values)
		}
	}
	if _, err := amountGrammar(t, 1, 10_000, 0).Compose(0); err == nil {
		t.Fatal("主张为 0 仍算出了金额")
	}
}

func TestAZeroAmountGrammarIsNotAnUnconfiguredGrammar(t *testing.T) {
	registered, err := NewAmountGrammar(0, 0, 0)
	if err != nil {
		t.Fatalf("全零登记：%v", err)
	}
	composition, err := registered.Compose(10_000)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if composition.FormsAmount() {
		t.Fatal("限额 0、比例 0 仍形成了金额")
	}
	var unset AmountGrammar
	if _, err := unset.Compose(10_000); err == nil {
		t.Fatal("零值文法被当成一份登记")
	}
}

func amountGrammar(t *testing.T, limit, ratio, deductible int64) AmountGrammar {
	t.Helper()
	grammar, err := NewAmountGrammar(limit, ratio, deductible)
	if err != nil {
		t.Fatalf("grammar: %v", err)
	}
	return grammar
}
