package domain

import "testing"

func TestMinimumSpendShortfallIsTheGapBelowTheFloor(t *testing.T) {
	terms, err := NewMinimumSpendTerms(80_000)
	if err != nil {
		t.Fatalf("terms: %v", err)
	}
	assessment, err := terms.AssessMinimumSpend(50_000)
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if assessment.AmountMinor() != 30_000 || !assessment.FormsAmount() {
		t.Fatalf("补差 = %d forms %v，想要 30000", assessment.AmountMinor(), assessment.FormsAmount())
	}
	met, err := terms.AssessMinimumSpend(80_000)
	if err != nil {
		t.Fatalf("assess met: %v", err)
	}
	if met.AmountMinor() != 0 || met.FormsAmount() {
		t.Fatalf("达到最低消费仍形成补差 %d", met.AmountMinor())
	}
}

func TestVolumeFloorChargesTheMissingQuantityAtTheRegisteredRate(t *testing.T) {
	terms, err := NewVolumeFloorTerms(1_000, 50)
	if err != nil {
		t.Fatalf("terms: %v", err)
	}
	assessment, err := terms.AssessVolumeFloor(400)
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if assessment.AmountMinor() != 30_000 || !assessment.FormsAmount() {
		t.Fatalf("金额 = %d，想要 600×50 = 30000", assessment.AmountMinor())
	}
	met, err := terms.AssessVolumeFloor(1_000)
	if err != nil {
		t.Fatalf("assess met: %v", err)
	}
	if met.FormsAmount() {
		t.Fatal("达到保底量仍形成了金额")
	}
}

func TestTieredRebateAppliesEachBandOnlyToItsSlice(t *testing.T) {
	low, err := NewRebateTier(10_000, 0)
	if err != nil {
		t.Fatalf("low: %v", err)
	}
	mid, err := NewRebateTier(50_000, 500)
	if err != nil {
		t.Fatalf("mid: %v", err)
	}
	high, err := NewOpenRebateTier(1_000)
	if err != nil {
		t.Fatalf("high: %v", err)
	}
	terms, err := NewTieredRebateTerms([]RebateTier{low, mid, high})
	if err != nil {
		t.Fatalf("terms: %v", err)
	}
	assessment, err := terms.AssessTieredRebate(60_000)
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if assessment.AmountMinor() != 3_000 {
		t.Fatalf("返利 = %d，想要 2000+1000 = 3000", assessment.AmountMinor())
	}
}

func TestSpendAboveTheLastBoundedTierIsNotRebated(t *testing.T) {
	only, err := NewRebateTier(10_000, 1_000)
	if err != nil {
		t.Fatalf("tier: %v", err)
	}
	terms, err := NewTieredRebateTerms([]RebateTier{only})
	if err != nil {
		t.Fatalf("terms: %v", err)
	}
	assessment, err := terms.AssessTieredRebate(25_000)
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if assessment.AmountMinor() != 1_000 {
		t.Fatalf("返利 = %d，想要只覆盖到 10000 的 1000", assessment.AmountMinor())
	}
}

func TestPeriodicFeeNotApplicableDoesNotInventAnAmount(t *testing.T) {
	assessment, err := NewPeriodicFeeNotApplicable().AssessNotApplicable()
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if !assessment.NotApplicable() || assessment.FormsAmount() {
		t.Fatal("不适用被当成了一笔费用")
	}
	var unset PeriodicFeeTerms
	if _, err := unset.AssessMinimumSpend(1); err == nil {
		t.Fatal("零值形态被当成一份登记")
	}
}

func TestPeriodicFeeTermsRejectABrokenSchedule(t *testing.T) {
	if _, err := NewMinimumSpendTerms(-1); err == nil {
		t.Fatal("负的最低消费被收下")
	}
	if _, err := NewVolumeFloorTerms(-1, 1); err == nil {
		t.Fatal("负的保底量被收下")
	}
	open, err := NewOpenRebateTier(100)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	bounded, err := NewRebateTier(10, 100)
	if err != nil {
		t.Fatalf("bounded: %v", err)
	}
	if _, err := NewTieredRebateTerms([]RebateTier{open, bounded}); err == nil {
		t.Fatal("开放档不在最后仍被收下")
	}
	terms, err := NewMinimumSpendTerms(1)
	if err != nil {
		t.Fatalf("terms: %v", err)
	}
	if _, err := terms.AssessMinimumSpend(-1); err == nil {
		t.Fatal("负的已计金额仍算出了补差")
	}
}
