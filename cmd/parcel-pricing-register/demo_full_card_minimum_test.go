package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// 演示租户的第三张完整售价卡 SYN-PLAN-CN-SG-FULL-03（范围 SYN-SCOPE-04）照种子原样重建后，对各类包裹各真算一份评价：
// 偏远一条规则一个费用代码按档位查价表（T1 收 12、T2 收 28、T0 不收），单票合计最低收费 120.99。期望金额是手算的
// （首重 0.5 kg 的 Z1 55、续重每 0.5 kg 18，Z2 48 与 15，燃油 = 基础运费 × 序列当期 11%，逐行取整到分），不是从
// 实现读回来的；种子被改动、引擎语义变了，这里先红。证据只记 S。
//
// Covers: AT-PP-040
//
// 下限 120.99 取 FULL-01 小件市区一票（2 kg）的合计——它是基础运费加燃油 109 + 11.99，所以那一票恰好等于下限；
// 基础价最低的一档合计也有 61.05，造不出低于 30 的合计，故不用 30。
func TestTheDemoFullCardThreeMinimumChargeAndTierTablePriceTheSevenKindsOfParcelByHand(t *testing.T) {
	seeds := filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing")
	read := func(name string) []byte {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(seeds, name))
		if err != nil {
			t.Fatalf("读种子 %s：%v", name, err)
		}
		return raw
	}
	card, err := domain.RehydratePriceCardRegistration(read("price-card-cn-sg-full-03.json"))
	if err != nil {
		t.Fatalf("重建完整价卡三：%v", err)
	}
	if card.Plan().CanonicalizationVersion() != "PPC-6" {
		t.Fatalf("完整价卡三带总额下限与档位查表，应写 PPC-6，实为 %s", card.Plan().CanonicalizationVersion())
	}
	if card.Plan().Scope().String() != "SYN-SCOPE-04" {
		t.Fatalf("计价范围 %s，应是 SYN-SCOPE-04", card.Plan().Scope())
	}
	remoteRules := 0
	for _, rule := range card.Plan().Structures().SurchargeRules() {
		if rule.Calculation().TableKey() == domain.TableKeyRemoteTier {
			remoteRules++
			if rule.Code().String() != "REMOTE_AREA_SURCHARGE" {
				t.Fatalf("按档位查表的规则费用代码 %s，应是 REMOTE_AREA_SURCHARGE", rule.Code())
			}
		}
	}
	if remoteRules != 1 {
		t.Fatalf("按档位查表的规则 %d 条，应恰好一条（一条规则一个费用代码）", remoteRules)
	}
	zone, err := domain.RehydrateReferenceCatalogueRegistration(read("reference-catalogue-zone-cn-sg-full.json"))
	if err != nil {
		t.Fatalf("重建完整价卡的分区目录：%v", err)
	}
	tier, err := domain.RehydrateReferenceCatalogueRegistration(read("reference-catalogue-remote-tier-cn-sg.json"))
	if err != nil {
		t.Fatalf("重建偏远档位目录：%v", err)
	}
	fuel, err := domain.RehydrateReferenceSeriesRegistration(read("reference-series-fuel-full.json"))
	if err != nil {
		t.Fatalf("重建燃油序列：%v", err)
	}

	basisAt := time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)
	tickets := []struct {
		name        string
		kilograms   string
		sides       [3]string
		destination string
		total       string
		lines       map[string]string
		explained   string
	}{
		// ① 1 kg 小件市区：基础 55 + 18 = 73，燃油 8.03，合计 81.03 低于下限 120.99，补差 39.96。
		{"①轻小件市区触发最低收费", "1", [3]string{"20", "15", "10"}, "018956", "120.99",
			map[string]string{"BASE_FREIGHT": "73", "FUEL_SURCHARGE": "8.03", "MINIMUM_CHARGE": "39.96"},
			"below the declared minimum 120.99 CNY"},
		// ② T1：偏远费 12，合计 132.99 高于下限。
		{"②小件偏远一档", "2", [3]string{"30", "20", "10"}, "098123", "132.99",
			map[string]string{"BASE_FREIGHT": "109", "FUEL_SURCHARGE": "11.99", "REMOTE_AREA_SURCHARGE": "12"},
			"looked up by remote tier T1"},
		// ③ T2：同一个费用代码、同一条规则，金额 28。
		{"③小件偏远二档", "2", [3]string{"30", "20", "10"}, "627123", "148.99",
			map[string]string{"BASE_FREIGHT": "109", "FUEL_SURCHARGE": "11.99", "REMOTE_AREA_SURCHARGE": "28"},
			"looked up by remote tier T2"},
		// ④ 体积重 120×80×60÷5000 = 115.2，进到 115.5 kg：首重 55 + 230 个续重 × 18 = 4195。
		{"④大件只超尺寸", "3", [3]string{"120", "80", "60"}, "018956", "4691.45",
			map[string]string{"BASE_FREIGHT": "4195", "FUEL_SURCHARGE": "461.45", "OVERSIZE_SURCHARGE": "35"},
			"within the declared bounds"},
		// ⑤ 体积重 4.8 低于实重 25：首重 55 + 49 个续重 × 18 = 937。
		{"⑤重件只超重", "25", [3]string{"40", "30", "20"}, "018956", "1065.07",
			map[string]string{"BASE_FREIGHT": "937", "FUEL_SURCHARGE": "103.07", "OVERWEIGHT_SURCHARGE": "25"},
			"within the declared bounds"},
		// ⑥ 2 kg 小件市区：109 + 11.99 = 120.99，恰好等于下限，不补差，解释如实记下「等于下限」。
		{"⑥小件市区恰好等于下限", "2", [3]string{"30", "20", "10"}, "018956", "120.99",
			map[string]string{"BASE_FREIGHT": "109", "FUEL_SURCHARGE": "11.99"},
			"equal to the declared minimum 120.99 CNY"},
		// ⑦ 南向 Z2 市区（238，T0）：48 + 3 × 15 = 93，燃油 10.23，合计 103.23，补差 17.76；T0 不收偏远费。
		{"⑦Z2市区触发最低收费", "2", [3]string{"30", "20", "10"}, "238456", "120.99",
			map[string]string{"BASE_FREIGHT": "93", "FUEL_SURCHARGE": "10.23", "MINIMUM_CHARGE": "17.76"},
			"below the declared minimum 120.99 CNY"},
	}
	for _, ticket := range tickets {
		t.Run(ticket.name, func(t *testing.T) {
			route, err := domain.NewPostalRoute("200120", ticket.destination)
			if err != nil {
				t.Fatalf("邮编路线：%v", err)
			}
			sides, err := domain.NewDimensions(pptest.Value(t, domain.ParseDecimal, ticket.sides[0]),
				pptest.Value(t, domain.ParseDecimal, ticket.sides[1]), pptest.Value(t, domain.ParseDecimal, ticket.sides[2]),
				domain.LengthUnitCentimeter)
			if err != nil {
				t.Fatalf("尺寸：%v", err)
			}
			weight, err := domain.NewWeightFromString(ticket.kilograms, domain.WeightUnitKilogram)
			if err != nil {
				t.Fatalf("重量：%v", err)
			}
			subject, err := domain.NewEstimateSubject("SYN-ESTIMATE-FULL3-" + ticket.name)
			if err != nil {
				t.Fatalf("评价主体：%v", err)
			}
			input, err := domain.NewPostalPricingInputSnapshot(card.Tenant(), card.Plan().Scope(), subject, route, weight, &sides, basisAt)
			if err != nil {
				t.Fatalf("计价输入：%v", err)
			}
			zoneReading, applicable := zone.ResolveAt(basisAt, route)
			if !applicable {
				t.Fatalf("分区目录在计价时点不生效")
			}
			tierReading, applicable := tier.ResolveAt(basisAt, route)
			if !applicable {
				t.Fatalf("偏远档位目录在计价时点不生效")
			}
			input, err = input.WithReferenceCatalogues(zoneReading, tierReading)
			if err != nil {
				t.Fatalf("补目录读数：%v", err)
			}
			fuelReading, applicable := fuel.ResolveAt(basisAt)
			if !applicable {
				t.Fatalf("燃油序列在计价时点没有期次")
			}
			input, err = input.WithReferenceSeries(fuelReading.Value())
			if err != nil {
				t.Fatalf("补序列读数：%v", err)
			}

			evaluation := pptest.Evaluate(t, "SYN-EVAL-FULL3-"+ticket.name, card.Plan(), input)
			if evaluation.Status() != domain.EvaluationCompleted {
				t.Fatalf("状态 %s，问题项 %v", evaluation.Status(), evaluation.Issues())
			}
			total, _ := evaluation.Total()
			if total.Amount().String() != ticket.total {
				t.Fatalf("合计 %s，手算 %s", total.Amount(), ticket.total)
			}
			got := map[string]string{}
			sum := pptest.Value(t, domain.ParseDecimal, "0")
			for _, line := range evaluation.ChargeLines() {
				got[line.Code().String()] = line.Amount().Amount().String()
				sum, err = sum.Add(line.Amount().Amount())
				if err != nil {
					t.Fatalf("行求和：%v", err)
				}
			}
			if len(got) != len(ticket.lines) {
				t.Fatalf("费用行 %v，手算 %v", got, ticket.lines)
			}
			for code, amount := range ticket.lines {
				if got[code] != amount {
					t.Fatalf("费用行 %s = %s，手算 %s（全部 %v）", code, got[code], amount, got)
				}
			}
			// 页面上看到的各行加起来就是合计：补差行不是整笔，是差额。
			if sum.String() != ticket.total {
				t.Fatalf("各行相加 %s，应等于合计 %s", sum, ticket.total)
			}
			found := false
			for _, line := range evaluation.Explanation() {
				if strings.Contains(line, ticket.explained) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("解释里没有 %q：%v", ticket.explained, evaluation.Explanation())
			}
		})
	}
}
