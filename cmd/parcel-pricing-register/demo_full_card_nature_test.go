package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// 演示租户的第二张完整售价卡 SYN-PLAN-CN-SG-FULL-02（范围 SYN-SCOPE-03）照种子原样重建后真算七件包裹（ADR-0347）：它与 FULL-01 的
// 基础运费、燃油、偏远、超尺寸逐项相同，只在超重费（按计价重量超过 20 千克，而不是实重）与住宅派送费（收件地址性质为住宅加收 6）
// 上不同。期望金额是手算的（首重 0.5 kg 55、续重每 0.5 kg 18，Z2 为 48 与 15；燃油 = 基础运费 × 序列当期 11%，逐行取整到分），
// 不是从实现读回来的；种子被改动、引擎语义变了，这里先红。证据只记 S。
func TestTheSecondDemoFullCardPricesByChargeableWeightAndAddressNatureByHand(t *testing.T) {
	seeds := filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing")
	read := func(name string) []byte {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(seeds, name))
		if err != nil {
			t.Fatalf("读种子 %s：%v", name, err)
		}
		return raw
	}
	card, err := domain.RehydratePriceCardRegistration(read("price-card-cn-sg-full-02.json"))
	if err != nil {
		t.Fatalf("重建第二张完整价卡：%v", err)
	}
	if card.Plan().Scope().String() != "SYN-SCOPE-03" || card.Plan().Reference().ID() != "SYN-PLAN-CN-SG-FULL-02" {
		t.Fatalf("种子不是 FULL-02 / SYN-SCOPE-03：%s %s", card.Plan().Reference().ID(), card.Plan().Scope())
	}
	zone, err := domain.RehydrateReferenceCatalogueRegistration(read("reference-catalogue-zone-cn-sg-full.json"))
	if err != nil {
		t.Fatalf("重建分区目录：%v", err)
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
		nature      string // 空即不声明
		kilograms   string
		sides       [3]string
		destination string
		status      domain.EvaluationStatus
		total       string
		lines       map[string]string
	}{
		{"住宅小件市区", "RESIDENTIAL", "2", [3]string{"30", "20", "10"}, "018956", domain.EvaluationCompleted, "126.99",
			map[string]string{"BASE_FREIGHT": "109", "FUEL_SURCHARGE": "11.99", "RESIDENTIAL_DELIVERY_SURCHARGE": "6"}},
		{"商业小件市区", "COMMERCIAL", "2", [3]string{"30", "20", "10"}, "018956", domain.EvaluationCompleted, "120.99",
			map[string]string{"BASE_FREIGHT": "109", "FUEL_SURCHARGE": "11.99"}},
		// 没声明地址性质：这张卡有规则按它判定，评价待判断、没有合计——不当商业也不当住宅。
		{"没声明地址性质", "", "2", [3]string{"30", "20", "10"}, "018956", domain.EvaluationPending, "", nil},
		// 住宅与偏远档位是两个独立维度，同一票两笔都收。
		{"住宅加偏远一档", "RESIDENTIAL", "2", [3]string{"30", "20", "10"}, "098123", domain.EvaluationCompleted, "138.99",
			map[string]string{"BASE_FREIGHT": "109", "FUEL_SURCHARGE": "11.99", "REMOTE_AREA_SURCHARGE": "12", "RESIDENTIAL_DELIVERY_SURCHARGE": "6"}},
		// 实重 5 千克，体积重 60×50×40÷5000 = 24：计价重量 24 千克超过 20，收超重费；Z2 首重 48 + 47 个续重 × 15 = 753。
		{"轻泡件按计价重量超重", "COMMERCIAL", "5", [3]string{"60", "50", "40"}, "238956", domain.EvaluationCompleted, "860.83",
			map[string]string{"BASE_FREIGHT": "753", "FUEL_SURCHARGE": "82.83", "OVERWEIGHT_SURCHARGE": "25"}},
		// 体积重 50.25×40×50÷5000 = 20.1，计价重量按 0.5 进位得 20.5，阈值之上的最小可表示值：超重；
		// Z1 首重 55 + 40 个续重 × 18 = 775；二档偏远 28；住宅 6。
		{"住宅偏远二档阈值加最小位", "RESIDENTIAL", "8", [3]string{"50.25", "40", "50"}, "627123", domain.EvaluationCompleted, "919.25",
			map[string]string{"BASE_FREIGHT": "775", "FUEL_SURCHARGE": "85.25", "OVERWEIGHT_SURCHARGE": "25", "REMOTE_AREA_EXTENDED_SURCHARGE": "28", "RESIDENTIAL_DELIVERY_SURCHARGE": "6"}},
		// 体积重 50×40×50÷5000 恰为 20.0：等于阈值，不触发。Z1 首重 55 + 39 个续重 × 18 = 757。
		{"体积重恰等于阈值", "COMMERCIAL", "8", [3]string{"50", "40", "50"}, "018956", domain.EvaluationCompleted, "840.27",
			map[string]string{"BASE_FREIGHT": "757", "FUEL_SURCHARGE": "83.27"}},
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
			subject, err := domain.NewEstimateSubject("SYN-ESTIMATE-FULL2-" + ticket.name)
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
			if ticket.nature != "" {
				nature, err := domain.NewAddressNature(ticket.nature)
				if err != nil {
					t.Fatalf("地址性质：%v", err)
				}
				input, err = input.WithAddressNature(nature)
				if err != nil {
					t.Fatalf("补地址性质：%v", err)
				}
			}

			evaluation := pptest.Evaluate(t, "SYN-EVAL-FULL2-"+ticket.name, card.Plan(), input)
			if evaluation.Status() != ticket.status {
				t.Fatalf("状态 %s，手算 %s，问题项 %v", evaluation.Status(), ticket.status, evaluation.Issues())
			}
			if ticket.status == domain.EvaluationPending {
				if codes := evaluation.Issues(); len(codes) != 1 || codes[0].Code() != "ADDRESS_NATURE_REQUIRED" {
					t.Fatalf("待判断的问题项 %v，想要 ADDRESS_NATURE_REQUIRED", codes)
				}
				// 与别的晚期待判断（序列读数、偏远档位）一样，已成形的基础运费行留在评价上，但没有合计。
				if _, formed := evaluation.Total(); formed {
					t.Fatal("待判断的评价带着合计")
				}
				return
			}
			total, _ := evaluation.Total()
			if total.Amount().String() != ticket.total {
				t.Fatalf("合计 %s，手算 %s", total.Amount(), ticket.total)
			}
			got := map[string]string{}
			for _, line := range evaluation.ChargeLines() {
				got[line.Code().String()] = line.Amount().Amount().String()
			}
			if len(got) != len(ticket.lines) {
				t.Fatalf("费用行 %v，手算 %v", got, ticket.lines)
			}
			for code, amount := range ticket.lines {
				if got[code] != amount {
					t.Fatalf("费用行 %s = %s，手算 %s（全部 %v）", code, got[code], amount, got)
				}
			}
		})
	}
}

// 第二张完整价卡与第一张并存、互不顶替：范围不同（评价按范围选卡），第一张的种子仍是原样的那一张。
func TestTheTwoDemoFullCardsLiveInTheirOwnScopes(t *testing.T) {
	seeds := filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing")
	read := func(name string) domain.PriceCardRegistration {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(seeds, name))
		if err != nil {
			t.Fatalf("读种子 %s：%v", name, err)
		}
		card, err := domain.RehydratePriceCardRegistration(raw)
		if err != nil {
			t.Fatalf("重建 %s：%v", name, err)
		}
		return card
	}
	first, second := read("price-card-cn-sg-full.json"), read("price-card-cn-sg-full-02.json")
	if first.Plan().Scope() == second.Plan().Scope() || first.Plan().Reference().ID() == second.Plan().Reference().ID() {
		t.Fatalf("两张完整价卡的范围或标识重叠：%s/%s 与 %s/%s",
			first.Plan().Scope(), first.Plan().Reference().ID(), second.Plan().Scope(), second.Plan().Reference().ID())
	}
	if first.Plan().Structures().ReadsFeature(domain.FeatureAddressNature) || first.Plan().Structures().ReadsFeature(domain.FeatureChargeableWeight) {
		t.Fatal("第一张完整价卡读了地址性质或计价重量：它必须是原样的那一张")
	}
	if !second.Plan().Structures().ReadsFeature(domain.FeatureAddressNature) || !second.Plan().Structures().ReadsFeature(domain.FeatureChargeableWeight) {
		t.Fatal("第二张完整价卡没读地址性质或计价重量")
	}
}
