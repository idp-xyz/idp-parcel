package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func chargeLine(t *testing.T, code string, effect domain.ChargeLineEffect, amount, currency string) domain.PreAcceptanceTrialChargeLine {
	t.Helper()
	line, err := domain.NewPreAcceptanceTrialChargeLine(mustRef(t, domain.NewPricingChargeCode, code), effect, amount,
		mustRef(t, domain.NewCurrencyCode, currency))
	if err != nil {
		t.Fatalf("charge line: %v", err)
	}
	return line
}

// Covers: ADR-0379 决定一——逐件补记计价重量与费用行（代码、增减、金额与币种），随估价原样带到留痕；费用行的金额是评价算出的十进制
// 原文、币种是卡本币，不换写成最小币单位；没补记的那一份两格都答缺席，不拿零或空列表顶。
func TestATrialCarriesItsPricingWeightAndChargeLinesIntoTheEstimate(t *testing.T) {
	weight, err := domain.NewPreAcceptanceTrialPricingWeight("12.5", "KG")
	if err != nil {
		t.Fatalf("weight: %v", err)
	}
	lines := []domain.PreAcceptanceTrialChargeLine{
		chargeLine(t, "BASE_FREIGHT", domain.ChargeLineAdd, "13.005", "USD"),
		chargeLine(t, "PROMO", domain.ChargeLineDeduct, "0.5", "USD"),
	}
	detailed, err := totaledTrial(t, "P-1", 1250, "CNY").WithPricingWeight(weight).WithChargeLines(lines)
	if err != nil {
		t.Fatalf("with charge lines: %v", err)
	}
	bare := totaledTrial(t, "P-2", 800, "CNY")

	estimate, gap, err := domain.FormPerParcelSellTrialSum(perParcelAdoption(t), "PRICE_CARD:SYN-PLAN@v1",
		mustRef(t, domain.NewCurrencyCode, "CNY"), []domain.PreAcceptanceTrial{detailed, bare})
	if err != nil || gap != domain.PreAcceptanceEstimateFormed {
		t.Fatalf("gap = %s err = %v", gap, err)
	}
	kept := estimate.Trials()
	gotWeight, weighed := kept[0].PricingWeight()
	gotLines, listed := kept[0].ChargeLines()
	if !weighed || gotWeight.Value() != "12.5" || gotWeight.Unit() != "KG" || !listed || len(gotLines) != 2 {
		t.Fatalf("补记的一份 = %+v %t / %+v %t", gotWeight, weighed, gotLines, listed)
	}
	if gotLines[1].Code().String() != "PROMO" || gotLines[1].Effect() != domain.ChargeLineDeduct || gotLines[1].Amount() != "0.5" ||
		gotLines[0].Amount() != "13.005" || gotLines[0].Currency().String() != "USD" ||
		gotLines[0].Effect().String() != "ADD" || gotLines[1].Effect().String() != "DEDUCT" {
		t.Fatalf("费用行 = %+v", gotLines)
	}
	if _, weighed := kept[1].PricingWeight(); weighed {
		t.Fatal("没补记的那一份长出了计价重量")
	}
	if _, listed := kept[1].ChargeLines(); listed {
		t.Fatal("没补记的那一份长出了费用行")
	}
	// 交出去的是副本：改调用方手里的切片不改留痕。
	gotLines[0] = gotLines[1]
	if again, _ := estimate.Trials()[0].ChargeLines(); again[0].Code().String() != "BASE_FREIGHT" {
		t.Fatal("费用行被调用方改动")
	}
}

// Covers: 补记几格的构造门——费用行代码、方向、金额与币种必填，一份评价至少一行；计价重量值与单位必填。金额的文法归计价，本上下文
// 不判它是不是合法十进制。
func TestTheTrialDetailRefusesBlankOrNegativeCells(t *testing.T) {
	code := mustRef(t, domain.NewPricingChargeCode, "BASE")
	cny := mustRef(t, domain.NewCurrencyCode, "CNY")
	for name, build := range map[string]func() error{
		"代码缺": func() error {
			_, err := domain.NewPreAcceptanceTrialChargeLine(domain.PricingChargeCode{}, domain.ChargeLineAdd, "1", cny)
			return err
		},
		"方向缺": func() error {
			_, err := domain.NewPreAcceptanceTrialChargeLine(code, domain.ChargeLineEffectInvalid, "1", cny)
			return err
		},
		"金额缺": func() error {
			_, err := domain.NewPreAcceptanceTrialChargeLine(code, domain.ChargeLineAdd, " ", cny)
			return err
		},
		"币种缺": func() error {
			_, err := domain.NewPreAcceptanceTrialChargeLine(code, domain.ChargeLineAdd, "1", domain.CurrencyCode{})
			return err
		},
		"零行": func() error {
			_, err := totaledTrial(t, "P-1", 1, "CNY").WithChargeLines(nil)
			return err
		},
		"重量值缺": func() error {
			_, err := domain.NewPreAcceptanceTrialPricingWeight(" ", "KG")
			return err
		},
		"重量单位缺": func() error {
			_, err := domain.NewPreAcceptanceTrialPricingWeight("1", "")
			return err
		},
	} {
		if err := build(); !errors.Is(err, domain.ErrBlankValue) {
			t.Errorf("%s：err = %v, 想要 ErrBlankValue", name, err)
		}
	}
}

// Covers: ADR-0379 决定一——选卡用的服务产品随估价留痕；没申报即不记，空白不收。
func TestAnEstimateRecordsTheServiceProductItsCardWasSelectedFor(t *testing.T) {
	estimate, _, err := domain.FormPerParcelSellTrialSum(perParcelAdoption(t), "PRICE_CARD:SYN-PLAN@v1",
		mustRef(t, domain.NewCurrencyCode, "CNY"), []domain.PreAcceptanceTrial{totaledTrial(t, "P-1", 100, "CNY")})
	if err != nil {
		t.Fatalf("estimate: %v", err)
	}
	if _, declared := estimate.ServiceProduct(); declared {
		t.Fatal("没申报的估价长出了服务产品")
	}
	product, err := estimate.WithServiceProduct("SYN-PRODUCT-FBA")
	if err != nil {
		t.Fatalf("with service product: %v", err)
	}
	if got, declared := product.ServiceProduct(); !declared || got != "SYN-PRODUCT-FBA" || product.TotalMinor() != 100 {
		t.Fatalf("服务产品 = %q %t", got, declared)
	}
	if _, err := estimate.WithServiceProduct("  "); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("空白产品 err = %v", err)
	}
}

// Covers: ADR-0379 决定一「价卡照旧存一个字面引用；读口把它拆成方案标识、版本与内容指纹」——字面的写法与拆法在同一处，互为逆；
// 拆不开的字面答不成立，不猜。
func TestAVersionReferenceLiteralSplitsBackIntoItsParts(t *testing.T) {
	for _, parts := range []domain.VersionReferenceParts{
		{Kind: "pricing-plan", ID: "SYN-PLAN-SELL", Version: "v1", Fingerprint: "ab12"},
		{Kind: "pricing-plan", ID: "SYN-PLAN-SELL", Version: "v1"},
	} {
		literal := domain.VersionReferenceLiteral(parts)
		split, ok := domain.SplitVersionReferenceLiteral(literal)
		if !ok || split != parts {
			t.Fatalf("%q 拆回 %+v %t, 想要 %+v", literal, split, ok, parts)
		}
	}
	if literal := domain.VersionReferenceLiteral(domain.VersionReferenceParts{Kind: "pricing-plan", ID: "P", Version: "v1", Fingerprint: "f"}); literal != "pricing-plan:P@v1#f" {
		t.Fatalf("字面 = %q", literal)
	}
	for _, broken := range []string{"", "pricing-plan", "pricing-plan:P", ":P@v1", "pricing-plan:@v1", "pricing-plan:P@", "pricing-plan:P@v1#"} {
		if split, ok := domain.SplitVersionReferenceLiteral(broken); ok {
			t.Errorf("%q 拆成了 %+v", broken, split)
		}
	}
}
