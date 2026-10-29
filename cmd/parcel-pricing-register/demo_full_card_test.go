package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// 演示租户的完整售价卡 SYN-PLAN-CN-SG-FULL-01（范围 SYN-SCOPE-02）照种子原样重建后，对六类包裹各真算一份评价：
// 小件市区、小件落偏远一档与二档、大件只超尺寸、重件只超重、大件加重件。期望金额是手算的（首重 0.5 kg 55、续重每 0.5 kg 18，
// 燃油 = 基础运费 × 序列当期 11%，逐行取整到分），不是从实现读回来的；种子被改动、引擎语义变了，这里先红。
// 目录与序列都按种子文件重建、经复核登记那一份的读法取值，与正式评价补齐读数的形状一致。证据只记 S。
func TestTheDemoFullCardPricesTheSixKindsOfParcelByHand(t *testing.T) {
	seeds := filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing")
	read := func(name string) []byte {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(seeds, name))
		if err != nil {
			t.Fatalf("读种子 %s：%v", name, err)
		}
		return raw
	}
	card, err := domain.RehydratePriceCardRegistration(read("price-card-cn-sg-full.json"))
	if err != nil {
		t.Fatalf("重建完整价卡：%v", err)
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
	}{
		{"小件市区", "2", [3]string{"30", "20", "10"}, "018956", "120.99",
			map[string]string{"BASE_FREIGHT": "109", "FUEL_SURCHARGE": "11.99"}},
		{"小件偏远一档", "2", [3]string{"30", "20", "10"}, "098123", "132.99",
			map[string]string{"BASE_FREIGHT": "109", "FUEL_SURCHARGE": "11.99", "REMOTE_AREA_SURCHARGE": "12"}},
		{"小件偏远二档", "2", [3]string{"30", "20", "10"}, "627123", "148.99",
			map[string]string{"BASE_FREIGHT": "109", "FUEL_SURCHARGE": "11.99", "REMOTE_AREA_EXTENDED_SURCHARGE": "28"}},
		// 体积重 120×80×60÷5000 = 115.2，进到 115.5 kg：首重 55 + 230 个续重 × 18 = 4195。
		{"大件只超尺寸", "3", [3]string{"120", "80", "60"}, "018956", "4691.45",
			map[string]string{"BASE_FREIGHT": "4195", "FUEL_SURCHARGE": "461.45", "OVERSIZE_SURCHARGE": "35"}},
		// 体积重 4.8 低于实重 25：首重 55 + 49 个续重 × 18 = 937。
		{"重件只超重", "25", [3]string{"40", "30", "20"}, "018956", "1065.07",
			map[string]string{"BASE_FREIGHT": "937", "FUEL_SURCHARGE": "103.07", "OVERWEIGHT_SURCHARGE": "25"}},
		// 超尺寸与超重同组同级，取金额较高者：35 胜 25，超重留痕不计收。
		{"大件加重件", "25", [3]string{"120", "80", "60"}, "018956", "4691.45",
			map[string]string{"BASE_FREIGHT": "4195", "FUEL_SURCHARGE": "461.45", "OVERSIZE_SURCHARGE": "35"}},
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
			subject, err := domain.NewEstimateSubject("SYN-ESTIMATE-FULL-" + ticket.name)
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

			evaluation := pptest.Evaluate(t, "SYN-EVAL-FULL-"+ticket.name, card.Plan(), input)
			if evaluation.Status() != domain.EvaluationCompleted {
				t.Fatalf("状态 %s，问题项 %v", evaluation.Status(), evaluation.Issues())
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

// 完整价卡随带的三份复核文档都过 CLI 的文档翻译，复核责任方不是登记责任方（演示租户的参考复核职责规则要求不同主体），
// 否则要到 seed.sh 的复核门上才红。
func TestTheDemoFullCardReviewSeedsParseWithADistinctReviewer(t *testing.T) {
	seeds := filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing")
	for _, name := range []string{"reference-catalogue-zone-cn-sg-full-review.json", "reference-catalogue-remote-tier-cn-sg-review.json"} {
		raw, err := os.ReadFile(filepath.Join(seeds, name))
		if err != nil {
			t.Fatalf("读种子 %s：%v", name, err)
		}
		command, err := parseCatalogueReviewDocument(raw)
		if err != nil || command.Reviewer == "SYN-PRICING-OPS-01" || command.Decision != domain.SeriesReviewApproved {
			t.Fatalf("种子 %s 过不了文档翻译、复核人就是登记人或结论不是通过：%+v err=%v", name, command, err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(seeds, "reference-series-fuel-full-review.json"))
	if err != nil {
		t.Fatalf("读燃油序列复核种子：%v", err)
	}
	command, err := parseReviewDocument(raw)
	if err != nil || command.Reviewer == "SYN-PRICING-OPS-01" || command.SeriesID != "SYN-SERIES-FUEL-FULL-01" ||
		command.Decision != domain.SeriesReviewApproved {
		t.Fatalf("燃油序列复核种子过不了文档翻译、复核人就是登记人或指错了序列：%+v err=%v", command, err)
	}
}
