package domain

import "testing"

// Two channels can round identically on both sides of a boundary and still
// charge differently, because the boundary decides which side a weight lands
// on. Hashing only the mode and increment would let a released version move
// that boundary without reporting a content conflict.
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

// The first-continue amount is a sum of two amounts the card declares, so the
// step count must be an exact integer: a fractional step would silently invent
// a precision the card never stated. This pins the arithmetic at the awkward
// boundaries rather than trusting the division helper's contract.
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
		{"0.0001", "30"}, // far below the first weight
		{"0.5", "30"},    // exactly the first weight, zero steps
		{"0.5001", "38"}, // a sliver over starts a whole step
		{"1", "38"},      // exactly one step, not two
		{"1.0001", "46"}, // a sliver over one step starts the second
		{"1.5", "46"},    // exactly two steps, not three
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

// A step finer than the excess it measures must still round up to one whole
// step. Scaled-integer division is what makes this exact; a float quotient
// would land on 0.9999… and charge nothing.
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
	// 0.0025 excess over a 0.001 step is 2.5 steps, charged as 3.
	if got := amount.amount.String(); got != "10.06" {
		t.Fatalf("priced at %s, want 10.06 (10 + 3 × 0.02)", got)
	}
}
