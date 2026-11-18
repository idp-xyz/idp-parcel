package domain_test

import (
	"errors"
	"slices"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func perParcelAdoption(t *testing.T) domain.PreAcceptanceEstimateAdoption {
	t.Helper()
	adoption, err := domain.NewPreAcceptanceEstimateAdoption(
		mustRef(t, domain.NewSettlementAccountID, "ACCT-1"),
		domain.PerParcelSellTrialSum,
		mustRef(t, domain.NewEstimatePricingScope, "SCOPE-SELL-1"),
	)
	if err != nil {
		t.Fatalf("adoption: %v", err)
	}
	return adoption
}

func totaledTrial(t *testing.T, parcel string, minor int64, currency string) domain.PreAcceptanceTrial {
	t.Helper()
	trial, err := domain.NewPreAcceptanceTrial(parcel, "EST-"+parcel, []string{"PRICE_CARD:SYN-PLAN@v1", "REFERENCE_SERIES:FUEL@v1"},
		minor, mustRef(t, domain.NewCurrencyCode, currency))
	if err != nil {
		t.Fatalf("trial: %v", err)
	}
	return trial
}

// Covers: ADR-0179 决定二、六——冻结金额等于逐件试算合计；估价连同形态、计价范围、价卡、逐件评价标识与版本清单一起交回，同一组输入
// 复算出同一合计。
func TestAPerParcelSellTrialSumAddsEveryParcelAndKeepsItsProvenance(t *testing.T) {
	cny := mustRef(t, domain.NewCurrencyCode, "CNY")
	trials := []domain.PreAcceptanceTrial{totaledTrial(t, "P-1", 1250, "CNY"), totaledTrial(t, "P-2", 800, "CNY")}

	estimate, gap, err := domain.FormPerParcelSellTrialSum(perParcelAdoption(t), perParcelAdoption(t).PricingScope(), "PRICE_CARD:SYN-PLAN@v1", cny, trials)
	if err != nil || gap != domain.PreAcceptanceEstimateFormed {
		t.Fatalf("gap = %s err = %v, 想要已形成", gap, err)
	}
	if estimate.TotalMinor() != 2050 || estimate.Currency() != cny {
		t.Fatalf("合计 = %d %s, 想要 2050 CNY", estimate.TotalMinor(), estimate.Currency())
	}
	if estimate.Form() != domain.PerParcelSellTrialSum || estimate.PricingScope().String() != "SCOPE-SELL-1" ||
		estimate.PriceCard() != "PRICE_CARD:SYN-PLAN@v1" {
		t.Fatalf("来历 = %s %s %s, 想要形态、计价范围与价卡都在", estimate.Form(), estimate.PricingScope(), estimate.PriceCard())
	}
	kept := estimate.Trials()
	if len(kept) != 2 || kept[0].Evaluation() != "EST-P-1" || !slices.Equal(kept[1].Versions(), []string{"PRICE_CARD:SYN-PLAN@v1", "REFERENCE_SERIES:FUEL@v1"}) {
		t.Fatalf("留痕 = %+v, 想要逐件评价标识与版本清单原样", kept)
	}

	again, _, _ := domain.FormPerParcelSellTrialSum(perParcelAdoption(t), perParcelAdoption(t).PricingScope(), "PRICE_CARD:SYN-PLAN@v1", cny, kept)
	if again.TotalMinor() != estimate.TotalMinor() {
		t.Fatalf("按留痕复算得 %d, 想要 %d", again.TotalMinor(), estimate.TotalMinor())
	}
}

// Covers: ADR-0380 决定七——估价头上的计价范围是这次实际问的那一个，不从采用行读。新形状合同的采用行不带范围，合同只声明
// 一个来源时各件也不另记来源：头上那一格取调用方交来的范围。采用行带着另一个范围也不作数。各件自己记了来源时取第一件的。
// 两处都没有范围是调用方漏交，报错，不留一行没有范围的估价。
func TestTheEstimateScopeIsTheOneAskedNotTheAdoptionRow(t *testing.T) {
	cny := mustRef(t, domain.NewCurrencyCode, "CNY")
	contractScope := mustRef(t, domain.NewEstimatePricingScope, "SCOPE-CONTRACT-1")
	bare, err := domain.NewPreAcceptanceEstimateAdoption(
		mustRef(t, domain.NewSettlementAccountID, "ACCT-1"), domain.PerParcelSellTrialSum, domain.EstimatePricingScope{})
	if err != nil {
		t.Fatalf("不带范围的采用行：%v", err)
	}
	plain := []domain.PreAcceptanceTrial{totaledTrial(t, "P-1", 1250, "CNY")}

	estimate, gap, err := domain.FormPerParcelSellTrialSum(bare, contractScope, "PRICE_CARD:SYN-PLAN@v1", cny, plain)
	if err != nil || gap != domain.PreAcceptanceEstimateFormed || estimate.PricingScope() != contractScope {
		t.Fatalf("采用行不带范围：scope = %q gap = %s err = %v，想要合同的那一个来源", estimate.PricingScope(), gap, err)
	}

	estimate, _, err = domain.FormPerParcelSellTrialSum(perParcelAdoption(t), contractScope, "PRICE_CARD:SYN-PLAN@v1", cny, plain)
	if err != nil || estimate.PricingScope() != contractScope {
		t.Fatalf("采用行带着另一个范围：scope = %q err = %v，想要这次问的那一个", estimate.PricingScope(), err)
	}

	landed := mustRef(t, domain.NewEstimatePricingScope, "SCOPE-CONTRACT-2")
	sourced, err := plain[0].WithSellPriceSource(landed, "PRICE_CARD:SYN-PLAN-2@v1")
	if err != nil {
		t.Fatalf("记来源：%v", err)
	}
	estimate, _, err = domain.FormPerParcelSellTrialSum(bare, contractScope, "PRICE_CARD:SYN-PLAN@v1", cny, []domain.PreAcceptanceTrial{sourced})
	if err != nil || estimate.PricingScope() != landed {
		t.Fatalf("各件自己记了来源：scope = %q err = %v，想要第一件落到的那一个", estimate.PricingScope(), err)
	}

	if _, _, err := domain.FormPerParcelSellTrialSum(bare, domain.EstimatePricingScope{}, "PRICE_CARD:SYN-PLAN@v1", cny, plain); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("err = %v，问的范围与各件的来源都没有时应报错", err)
	}
}

// Covers: ADR-0179 决定二的停法——任一份没有合计、任一份合计的币种不是控制作用域的币种、合计不是正数、一份试算都没有，都答未形成并
// 指名停在哪一格；不拿零、部分合计或换算顶替。单份为负与合计加不进 int64 各占一格，不并进「合计不是正数」：格是为了留下停在哪里，
// 三种停法并成一格，处理尝试上就分不出是价卡给了负数、合计太大还是本就合出零。
func TestAPerParcelSellTrialSumStopsInsteadOfFillingAGap(t *testing.T) {
	untotaled, err := domain.NewUntotaledPreAcceptanceTrial("P-2", "EST-P-2", nil)
	if err != nil {
		t.Fatalf("untotaled: %v", err)
	}
	for name, tc := range map[string]struct {
		trials []domain.PreAcceptanceTrial
		want   domain.PreAcceptanceEstimateGap
	}{
		"一份试算都没有":    {want: domain.PreAcceptanceEstimateHasNoTrial},
		"一份没有合计":     {trials: []domain.PreAcceptanceTrial{totaledTrial(t, "P-1", 1250, "CNY"), untotaled}, want: domain.PreAcceptanceEstimateTrialWithoutTotal},
		"一份币种不对":     {trials: []domain.PreAcceptanceTrial{totaledTrial(t, "P-1", 1250, "CNY"), totaledTrial(t, "P-2", 100, "USD")}, want: domain.PreAcceptanceEstimateCurrencyMismatch},
		"合计为零":       {trials: []domain.PreAcceptanceTrial{totaledTrial(t, "P-1", 0, "CNY")}, want: domain.PreAcceptanceEstimateNotPositive},
		"一份为负而合计仍为正": {trials: []domain.PreAcceptanceTrial{totaledTrial(t, "P-1", 500, "CNY"), totaledTrial(t, "P-2", -1, "CNY")}, want: domain.PreAcceptanceEstimateTrialNegative},
		"加不进 int64":  {trials: []domain.PreAcceptanceTrial{totaledTrial(t, "P-1", 1<<62, "CNY"), totaledTrial(t, "P-2", 1<<62, "CNY")}, want: domain.PreAcceptanceEstimateSumOverflow},
	} {
		t.Run(name, func(t *testing.T) {
			estimate, gap, err := domain.FormPerParcelSellTrialSum(perParcelAdoption(t), perParcelAdoption(t).PricingScope(), "PRICE_CARD:SYN-PLAN@v1",
				mustRef(t, domain.NewCurrencyCode, "CNY"), tc.trials)
			if err != nil || gap != tc.want || estimate.TotalMinor() != 0 {
				t.Fatalf("gap = %s total = %d err = %v, 想要停在 %s、不出金额", gap, estimate.TotalMinor(), err, tc.want)
			}
		})
	}
	named := map[string]domain.PreAcceptanceEstimateGap{}
	for _, gap := range []domain.PreAcceptanceEstimateGap{
		domain.PreAcceptanceEstimateHasNoTrial, domain.PreAcceptanceEstimateTrialWithoutTotal,
		domain.PreAcceptanceEstimateCurrencyMismatch, domain.PreAcceptanceEstimateNotPositive,
		domain.PreAcceptanceEstimateTrialNegative, domain.PreAcceptanceEstimateSumOverflow,
	} {
		if other, clash := named[gap.String()]; clash || gap.String() == "" {
			t.Fatalf("停格 %d 的名字 %q 空着或与 %d 撞名——停在哪一格分不出来", gap, gap.String(), other)
		}
		named[gap.String()] = gap
	}

	if _, _, err := domain.FormPerParcelSellTrialSum(domain.PreAcceptanceEstimateAdoption{}, perParcelAdoption(t).PricingScope(), "PRICE_CARD:SYN-PLAN@v1",
		mustRef(t, domain.NewCurrencyCode, "CNY"), nil); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("没有采用行 err = %v, 想要拒——形态只能来自采用行", err)
	}
	if _, err := domain.NewPreAcceptanceTrial("", "EST-1", nil, 1, mustRef(t, domain.NewCurrencyCode, "CNY")); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("没有件 err = %v, 想要拒", err)
	}
}
