package domain

import "testing"

// 两个渠道可以在边界两侧以完全相同的方式进位，收的钱却不同，因为边界决定一个重量落到哪
// 一侧。只对模式与进位单位取哈希，会让一个已发布版本挪动该边界而不报内容冲突。
func TestRoundingSegmentBoundsEnterTheDigest(t *testing.T) {
	fine, _ := NewWeightFromString("0.001", WeightUnitKilogram)
	coarse, _ := NewWeightFromString("1", WeightUnitKilogram)
	lowBound, _ := NewWeightFromString("2", WeightUnitKilogram)
	highBound, _ := NewWeightFromString("5", WeightUnitKilogram)

	lowSegment, err := NewWeightRoundingSegment(RoundingCeiling, fine, lowBound)
	if err != nil {
		t.Fatalf("low segment: %v", err)
	}
	highSegment, err := NewWeightRoundingSegment(RoundingCeiling, fine, highBound)
	if err != nil {
		t.Fatalf("high segment: %v", err)
	}
	open, err := NewOpenEndedWeightRoundingSegment(RoundingCeiling, coarse)
	if err != nil {
		t.Fatalf("open segment: %v", err)
	}

	lower, err := NewSegmentedWeightRoundingPolicy([]WeightRoundingSegment{lowSegment, open})
	if err != nil {
		t.Fatalf("lower policy: %v", err)
	}
	higher, err := NewSegmentedWeightRoundingPolicy([]WeightRoundingSegment{highSegment, open})
	if err != nil {
		t.Fatalf("higher policy: %v", err)
	}
	if hashCanonical(canonicalRoundingValue(lower)) == hashCanonical(canonicalRoundingValue(higher)) {
		t.Fatal("segment boundary was omitted from the canonical rounding document")
	}

	single, err := NewWeightRoundingPolicy(RoundingCeiling, coarse)
	if err != nil {
		t.Fatalf("single policy: %v", err)
	}
	if hashCanonical(canonicalRoundingValue(single)) == hashCanonical(canonicalRoundingValue(lower)) {
		t.Fatal("segment count was omitted from the canonical rounding document")
	}
}

// 首重加续重的金额是卡声明的两个金额之和，所以步数必须是精确整数：出现小数步等于凭空发明
// 一个卡从未声明过的精度。这里把算术钉在几个别扭的边界上，而不是信赖除法辅助函数的契约。
func TestFirstContinueStepCountIsExactAtEveryBoundary(t *testing.T) {
	currency, _ := NewCurrency("USD")
	id, _ := NewRateEntryID("fc-boundary")
	first, _ := NewWeightFromString("0.5", WeightUnitKilogram)
	step, _ := NewWeightFromString("0.5", WeightUnitKilogram)
	firstAmount, _ := NewMoneyFromString("30", currency)
	stepAmount, _ := NewMoneyFromString("8", currency)
	rate, err := NewFirstContinueRate(id, "Z1", first, firstAmount, step, stepAmount)
	if err != nil {
		t.Fatalf("rate: %v", err)
	}

	for _, testCase := range []struct{ weight, want string }{
		{"0.0001", "30"}, // 远低于首重
		{"0.5", "30"},    // 恰好首重，零步
		{"0.5001", "38"}, // 超出一丝就起一整步
		{"1", "38"},      // 恰好一步，不是两步
		{"1.0001", "46"}, // 超出一步一丝，起第二步
		{"1.5", "46"},    // 恰好两步，不是三步
	} {
		weight, _ := NewWeightFromString(testCase.weight, WeightUnitKilogram)
		amount, _, err := rate.price(weight)
		if err != nil {
			t.Fatalf("price %s: %v", testCase.weight, err)
		}
		if got := amount.amount.String(); got != testCase.want {
			t.Fatalf("%s kg priced at %s, want %s", testCase.weight, got, testCase.want)
		}
	}
}

// 比它所计量的超出量还细的步长，仍必须进位到一整步。定标整数除法才使这一步精确；浮点商会
// 落在 0.9999… 上，结果一分不收。
func TestFirstContinueHandlesAStepFinerThanTheExcess(t *testing.T) {
	currency, _ := NewCurrency("USD")
	id, _ := NewRateEntryID("fc-fine-step")
	first, _ := NewWeightFromString("1", WeightUnitKilogram)
	step, _ := NewWeightFromString("0.001", WeightUnitKilogram)
	firstAmount, _ := NewMoneyFromString("10", currency)
	stepAmount, _ := NewMoneyFromString("0.02", currency)
	rate, err := NewFirstContinueRate(id, "Z1", first, firstAmount, step, stepAmount)
	if err != nil {
		t.Fatalf("rate: %v", err)
	}
	weight, _ := NewWeightFromString("1.0025", WeightUnitKilogram)
	amount, _, err := rate.price(weight)
	if err != nil {
		t.Fatalf("price: %v", err)
	}
	// 0.0025 的超出量按 0.001 的步长是 2.5 步，按 3 步计收。
	if got := amount.amount.String(); got != "10.06" {
		t.Fatalf("priced at %s, want 10.06 (10 + 3 × 0.02)", got)
	}
}
