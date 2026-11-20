package pricecardtemplate

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证 PPT-4：票级卡声明计价重量的聚合方式与逐件求和的进位点（ADR-0364 决定三）。
// 两格必填、不预选；主单级与逐包裹留空；更早的模板没有这两行，票级卡按合计后派生读。
// 工作簿投影把声明了的两格写回去，再导入得到同一张卡。

func shipmentCostCard(method, point string) filled {
	card := costCard()
	card.card["templateVersion"] = templateVersionV4
	card.card["aggregation"] = "PER_SHIPMENT"
	if method != "" {
		card.card[cardFieldShipmentWeightAggregation] = method
	}
	if point != "" {
		card.card[cardFieldShipmentWeightRoundingPoint] = point
	}
	return card
}

func TestPPT4ShipmentCardDeclaresHowWeightIsAggregated(t *testing.T) {
	totals := validated(t, read(t, shipmentCostCard("TOTALS_THEN_DERIVE", "").bytes(t)))
	method, ok := totals.ShipmentWeightAggregation()
	if !ok || method != domain.ShipmentWeightTotalsThenDerive || totals.CanonicalizationVersion() != "PPC-8" {
		t.Fatalf("合计后派生：%s %v，版本 %s", method, ok, totals.CanonicalizationVersion())
	}
	if _, hasPoint := totals.ShipmentWeightRoundingPoint(); hasPoint {
		t.Fatal("合计后派生不该带进位点")
	}

	each := validated(t, read(t, shipmentCostCard("PER_PIECE_THEN_SUM", "ROUND_EACH_THEN_SUM").bytes(t)))
	method, ok = each.ShipmentWeightAggregation()
	point, hasPoint := each.ShipmentWeightRoundingPoint()
	if !ok || method != domain.ShipmentWeightPerPieceThenSum || !hasPoint || point != domain.ShipmentWeightRoundEachThenSum {
		t.Fatalf("逐件进位：%s %v，进位点 %s %v", method, ok, point, hasPoint)
	}

	fromForm := readWorkbookJSON(t, shipmentCostCard("PER_PIECE_THEN_SUM", "SUM_THEN_ROUND_ONCE").workbookJSON(t))
	plan := validated(t, fromForm)
	point, hasPoint = plan.ShipmentWeightRoundingPoint()
	if !hasPoint || point != domain.ShipmentWeightSumThenRoundOnce || plan.ContentDigest() == totals.ContentDigest() {
		t.Fatalf("表单读回的进位点 %s %v", point, hasPoint)
	}
}

func TestPPT4RejectsAShipmentCardThatDoesNotChoose(t *testing.T) {
	cases := []struct {
		name   string
		method string
		point  string
		want   string
	}{
		{"没填聚合方式", "", "", cardFieldShipmentWeightAggregation},
		{"逐件求和没填进位点", "PER_PIECE_THEN_SUM", "", cardFieldShipmentWeightRoundingPoint},
		{"合计后派生却填了进位点", "TOTALS_THEN_DERIVE", "SUM_THEN_ROUND_ONCE", cardFieldShipmentWeightRoundingPoint},
	}
	for _, tc := range cases {
		reading := read(t, shipmentCostCard(tc.method, tc.point).bytes(t))
		if reading.Content != nil {
			t.Fatalf("%s：不该读出方案：%s", tc.name, describe(reading.Problems))
		}
		found := false
		for _, problem := range reading.Problems {
			if problem.Column == tc.want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s：问题没落在 %s：%s", tc.name, tc.want, describe(reading.Problems))
		}
	}
}

func TestPPT4DoesNotOpenTheOptionOnAMasterDocument(t *testing.T) {
	card := costCard()
	card.card["aggregation"] = "PER_MASTER_DOCUMENT"
	card.card[cardFieldShipmentWeightAggregation] = "TOTALS_THEN_DERIVE"
	reading := read(t, card.bytes(t))
	if reading.Content != nil {
		t.Fatal("主单级填了聚合方式却读出了方案")
	}
	if !strings.Contains(describe(reading.Problems), "主单级") {
		t.Fatalf("问题：%s", describe(reading.Problems))
	}
}

func TestAnOlderShipmentCardStillReadsAsTotalsThenDerive(t *testing.T) {
	card := shipmentCostCard("", "")
	card.card["templateVersion"] = templateVersionV3
	delete(card.card, cardFieldShipmentWeightAggregation)
	delete(card.card, cardFieldShipmentWeightRoundingPoint)
	plan := validated(t, read(t, card.bytes(t)))
	_, declared := plan.ShipmentWeightAggregation()
	if declared || plan.CanonicalizationVersion() != "PPC-5" {
		t.Fatalf("PPT-3 票级卡应未声明、写 PPC-5：%s", plan.CanonicalizationVersion())
	}
}

func TestProjectingAShipmentCardReadsTheTwoFieldsBack(t *testing.T) {
	reading := read(t, shipmentCostCard("PER_PIECE_THEN_SUM", "ROUND_EACH_THEN_SUM").bytes(t))
	plan := validated(t, reading)
	projected, gap := projectPriceCard(plan, reading.Content.DirectionAuthorization)
	if gap.Where != "" {
		t.Fatalf("投影表达不了：%s：%s", gap.Where, gap.Conclusion)
	}
	if projected.version != templateVersionV4 {
		t.Fatalf("投影版本 %s，应是 PPT-4", projected.version)
	}
	again := validated(t, Reader{}.ReadPriceCardWorkbook(projected.json))
	if again.ContentDigest() != plan.ContentDigest() || again.CanonicalizationVersion() != "PPC-8" {
		t.Fatalf("读回 %s（%s），原卡 %s（%s）", again.ContentDigest(), again.CanonicalizationVersion(), plan.ContentDigest(), plan.CanonicalizationVersion())
	}
	method, _ := again.ShipmentWeightAggregation()
	point, _ := again.ShipmentWeightRoundingPoint()
	if method != domain.ShipmentWeightPerPieceThenSum || point != domain.ShipmentWeightRoundEachThenSum {
		t.Fatalf("读回的两格 %s / %s", method, point)
	}
	fromSheet := validated(t, read(t, projected.xlsx))
	if fromSheet.ContentDigest() != plan.ContentDigest() {
		t.Fatal("电子表格读回的摘要变了")
	}
}
