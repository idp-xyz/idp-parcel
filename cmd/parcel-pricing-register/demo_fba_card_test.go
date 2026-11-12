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

// 演示租户的 FBA 头程售价卡 SYN-PLAN-CN-SG-FBA-01（范围 SYN-SCOPE-01，票 declared-goods-lines/17）照种子原样重建后，对演示段
// 55 用的那只箱子（12.5 kg、60×40×40 cm）按两个分区各真算一份。期望是手算的，不是从实现读回来的：体积重 60×40×40÷6000 = 16，
// 大于实重 12.5，整公斤进位仍是 16 kg；Z1 每公斤 24 → 384，Z2 每公斤 21 → 336。种子被改动、引擎语义变了，这里先红；演示段
// 55 断言的「FBA 卡算 384」也就跟着有了出处。证据只记 S。
//
// Covers: ADR-0382 决定一、四在演示种子上的那一面——卡只指名 FBA 产品、写 PPC-7、内容摘要钉住。
func TestTheDemoFBACardNamesItsProductAndPricesTheDemoBoxPerKilogramByHand(t *testing.T) {
	seeds := filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing")
	read := func(name string) []byte {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(seeds, name))
		if err != nil {
			t.Fatalf("读种子 %s：%v", name, err)
		}
		return raw
	}
	card, err := domain.RehydratePriceCardRegistration(read("price-card-cn-sg-fba.json"))
	if err != nil {
		t.Fatalf("重建 FBA 卡：%v", err)
	}
	plan := card.Plan()
	if plan.Reference().ID() != "SYN-PLAN-CN-SG-FBA-01" || plan.Reference().Version() != "v1" {
		t.Fatalf("方案 %s@%s，应是 SYN-PLAN-CN-SG-FBA-01@v1", plan.Reference().ID(), plan.Reference().Version())
	}
	if plan.CanonicalizationVersion() != "PPC-7" {
		t.Fatalf("带适用服务产品的卡应写 PPC-7，实为 %s", plan.CanonicalizationVersion())
	}
	// 摘要取证于 4178889d0 的 seedgen 产物；种子重生成后变了就是卡的内容变了，演示段对评价摘要的断言同样会变。
	const wantDigest = "c1e528ac137ecb3b994e053aab91bd23514cafec1936e7c4ebeafbbd73a3152f"
	if plan.ContentDigest() != wantDigest {
		t.Fatalf("内容摘要 %s，钉住的是 %s", plan.ContentDigest(), wantDigest)
	}
	if plan.Scope().String() != "SYN-SCOPE-01" || plan.Direction() != domain.PricingDirectionSell ||
		plan.Purpose() != domain.PricingPurposeCustomerCharge {
		t.Fatalf("范围 %s、方向 %s、目的 %s，应是 SYN-SCOPE-01、SELL、客户费用", plan.Scope(), plan.Direction(), plan.Purpose())
	}
	products := plan.ServiceProducts()
	if len(products) != 1 || products[0].String() != "SYN-PROD-CN-SG-FBA" {
		t.Fatalf("适用服务产品 %v，应只有 SYN-PROD-CN-SG-FBA", products)
	}

	zone, err := domain.RehydrateReferenceCatalogueRegistration(read("reference-catalogue-zone-cn-sg.json"))
	if err != nil {
		t.Fatalf("重建分区目录：%v", err)
	}
	basisAt := time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC)
	for _, box := range []struct {
		name        string
		destination string
		total       string
		explained   string
	}{
		{"Z1 新加坡 018", "018000", "384", "16 KG at 24 per KG"},
		{"Z2 新加坡 238", "238000", "336", "16 KG at 21 per KG"},
	} {
		t.Run(box.name, func(t *testing.T) {
			route, err := domain.NewPostalRoute("200000", box.destination)
			if err != nil {
				t.Fatalf("邮编路线：%v", err)
			}
			sides, err := domain.NewDimensions(pptest.Value(t, domain.ParseDecimal, "60"),
				pptest.Value(t, domain.ParseDecimal, "40"), pptest.Value(t, domain.ParseDecimal, "40"),
				domain.LengthUnitCentimeter)
			if err != nil {
				t.Fatalf("尺寸：%v", err)
			}
			weight, err := domain.NewWeightFromString("12.5", domain.WeightUnitKilogram)
			if err != nil {
				t.Fatalf("重量：%v", err)
			}
			subject, err := domain.NewEstimateSubject("SYN-ESTIMATE-FBA-" + box.destination)
			if err != nil {
				t.Fatalf("评价主体：%v", err)
			}
			input, err := domain.NewPostalPricingInputSnapshot(card.Tenant(), plan.Scope(), subject, route, weight, &sides, basisAt)
			if err != nil {
				t.Fatalf("计价输入：%v", err)
			}
			reading, applicable := zone.ResolveAt(basisAt, route)
			if !applicable {
				t.Fatalf("分区目录在计价时点不生效")
			}
			input, err = input.WithReferenceCatalogues(reading)
			if err != nil {
				t.Fatalf("补目录读数：%v", err)
			}

			evaluation := pptest.Evaluate(t, "SYN-EVAL-FBA-"+box.destination, plan, input)
			if evaluation.Status() != domain.EvaluationCompleted {
				t.Fatalf("状态 %s，问题项 %v", evaluation.Status(), evaluation.Issues())
			}
			total, _ := evaluation.Total()
			if total.Amount().String() != box.total || total.Currency().String() != "CNY" {
				t.Fatalf("合计 %s %s，手算 %s CNY", total.Amount(), total.Currency(), box.total)
			}
			// 只有基础运费一行：这张卡没有固定附加费，也不绑燃油序列。
			lines := evaluation.ChargeLines()
			if len(lines) != 1 || lines[0].Code().String() != "BASE_FREIGHT" {
				t.Fatalf("费用行 %v，应只有 BASE_FREIGHT 一行", lines)
			}
			found := false
			for _, line := range evaluation.Explanation() {
				if strings.Contains(line, box.explained) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("解释里没有 %q：%v", box.explained, evaluation.Explanation())
			}
		})
	}
}
