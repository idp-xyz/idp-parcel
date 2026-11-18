package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// 演示租户的专属售价卡 SYN-PLAN-CN-SG-EXCL-01（范围 SYN-SCOPE-EXCL-01，票 commercial-four-module-requirements/21）照种子原样
// 重建后，对一只 1 kg、10×10×10 cm 的箱子真算。期望是手算的，不是从实现读回来的：体积重 1000÷5000 = 0.2，小于实重 1，
// 计费重按 0.5 kg 进位仍是 1 kg，即首重 0.5 加一档续重。
//
//   - 专属卡 Z1：50 + 16 + 偏远附加 5 = 71。
//   - 公共卡 Z1：55 + 18 + 5 = 78。同一只箱子差 7 元，演示上看得出用的是哪一张卡。
//   - 专属卡 Z2：分区目录解得出 Z2，运费表没有这一格——正是合同声明的后一个范围要接住的那一种落空。
//
// 种子被改动、引擎语义变了，这里先红。证据只记 S。
func TestTheExclusiveDemoCardPricesOnlyTheNegotiatedZoneBelowThePublicCard(t *testing.T) {
	seeds := filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing")
	read := func(name string) []byte {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(seeds, name))
		if err != nil {
			t.Fatalf("读种子 %s：%v", name, err)
		}
		return raw
	}
	card, err := domain.RehydratePriceCardRegistration(read("price-card-cn-sg-exclusive.json"))
	if err != nil {
		t.Fatalf("重建专属卡：%v", err)
	}
	exclusive := card.Plan()
	// 摘要出自 seedgen（exclusive_card.go）；种子重生成后变了就是卡的内容变了。
	const wantDigest = "054c3d14e0f0b5b8593a625d8001524b2cd98193f120476aa1cbdd9e0cdd9354"
	if exclusive.ContentDigest() != wantDigest {
		t.Fatalf("内容摘要 %s，钉住的是 %s", exclusive.ContentDigest(), wantDigest)
	}
	if exclusive.Scope().String() != "SYN-SCOPE-EXCL-01" || exclusive.Direction() != domain.PricingDirectionSell ||
		exclusive.Purpose() != domain.PricingPurposeCustomerCharge {
		t.Fatalf("范围 %s、方向 %s、目的 %s，应是 SYN-SCOPE-EXCL-01、SELL、客户费用", exclusive.Scope(), exclusive.Direction(), exclusive.Purpose())
	}
	rates := exclusive.RateTable().FirstContinueRates()
	if len(rates) != 1 || rates[0].Zone() != "Z1" {
		t.Fatalf("运费表 %d 行，应只有谈下来的 Z1", len(rates))
	}
	catalogues := exclusive.Structures().ReferenceCatalogues()
	if len(catalogues) != 1 || catalogues[0].CatalogueID() != "SYN-CAT-ZONE-CN-SG" {
		t.Fatalf("目录 %v，应绑解得出 Z1 与 Z2 的 SYN-CAT-ZONE-CN-SG", catalogues)
	}
	series := exclusive.Structures().ReferenceSeries()
	if len(series) != 1 || series[0].SeriesID() != "SYN-SERIES-FUEL-01" {
		t.Fatalf("序列 %v，应与公共卡绑同一条燃油序列", series)
	}

	publicCard, err := domain.RehydratePriceCardRegistration(read("price-card-cn-sg.json"))
	if err != nil {
		t.Fatalf("重建公共卡：%v", err)
	}
	zone, err := domain.RehydrateReferenceCatalogueRegistration(read("reference-catalogue-zone-cn-sg.json"))
	if err != nil {
		t.Fatalf("重建分区目录：%v", err)
	}
	fuel, err := domain.RehydrateReferenceSeriesRegistration(read("reference-series-fuel.json"))
	if err != nil {
		t.Fatalf("重建燃油序列：%v", err)
	}
	basisAt := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	fuelReading, covered := fuel.ResolveAt(basisAt)
	if !covered {
		t.Fatal("燃油序列在计价时点没有期次")
	}

	evaluate := func(t *testing.T, id string, plan domain.PricingPlanVersion, destination string) domain.PricingEvaluation {
		t.Helper()
		route, err := domain.NewPostalRoute("200000", destination)
		if err != nil {
			t.Fatalf("邮编路线：%v", err)
		}
		sides, err := domain.NewDimensions(pptest.Value(t, domain.ParseDecimal, "10"),
			pptest.Value(t, domain.ParseDecimal, "10"), pptest.Value(t, domain.ParseDecimal, "10"),
			domain.LengthUnitCentimeter)
		if err != nil {
			t.Fatalf("尺寸：%v", err)
		}
		weight, err := domain.NewWeightFromString("1", domain.WeightUnitKilogram)
		if err != nil {
			t.Fatalf("重量：%v", err)
		}
		subject, err := domain.NewEstimateSubject("SYN-ESTIMATE-" + id)
		if err != nil {
			t.Fatalf("评价主体：%v", err)
		}
		input, err := domain.NewPostalPricingInputSnapshot(card.Tenant(), plan.Scope(), subject, route, weight, &sides, basisAt)
		if err != nil {
			t.Fatalf("计价输入：%v", err)
		}
		reading, applicable := zone.ResolveAt(basisAt, route)
		if !applicable {
			t.Fatal("分区目录在计价时点不生效")
		}
		if input, err = input.WithReferenceCatalogues(reading); err != nil {
			t.Fatalf("补目录读数：%v", err)
		}
		if input, err = input.WithReferenceSeries(fuelReading.Value()); err != nil {
			t.Fatalf("补序列取值：%v", err)
		}
		return pptest.Evaluate(t, "SYN-EVAL-"+id, plan, input)
	}

	for _, box := range []struct {
		name  string
		plan  domain.PricingPlanVersion
		total string
	}{
		{"专属卡 Z1", exclusive, "71"},
		{"公共卡 Z1", publicCard.Plan(), "78"},
	} {
		t.Run(box.name, func(t *testing.T) {
			evaluation := evaluate(t, box.plan.Reference().ID()+"-018", box.plan, "018000")
			if evaluation.Status() != domain.EvaluationCompleted {
				t.Fatalf("状态 %s，问题项 %v", evaluation.Status(), evaluation.Issues())
			}
			total, _ := evaluation.Total()
			if total.Amount().String() != box.total || total.Currency().String() != "CNY" {
				t.Fatalf("合计 %s %s，手算 %s CNY", total.Amount(), total.Currency(), box.total)
			}
		})
	}

	t.Run("专属卡 Z2 是运费表缺格", func(t *testing.T) {
		evaluation := evaluate(t, "EXCL-238", exclusive, "238000")
		if evaluation.Status() != domain.EvaluationPending || !evaluation.BaseRateTableMiss() {
			t.Fatalf("状态 %s、运费表缺格 %v、问题项 %v：没谈到的 Z2 应停在基础运费表没有这一格上",
				evaluation.Status(), evaluation.BaseRateTableMiss(), evaluation.Issues())
		}
	})
}
