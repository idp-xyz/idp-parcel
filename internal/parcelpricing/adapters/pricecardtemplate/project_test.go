package pricecardtemplate

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 读回再导入：演示租户的完整合成价卡（FULL-03）与较简的成本卡，规范化版本与内容摘要都不动。
// 电子表格与结构化工作簿两条载体都走同一道译码。
func TestProjectingAValidatedCardImportsBackToTheSameDigest(t *testing.T) {
	for _, card := range []filled{full03Card(), costCard()} {
		reading := read(t, card.bytes(t))
		plan := validated(t, reading)
		projected, gap := projectPriceCard(plan, reading.Content.DirectionAuthorization)
		if gap.Where != "" {
			t.Fatalf("%s 表达不了：%s：%s", plan.Reference().ID(), gap.Where, gap.Conclusion)
		}
		fromWorkbook := validated(t, Reader{}.ReadPriceCardWorkbook(projected.json))
		fromSheet := validated(t, read(t, projected.xlsx))
		for _, again := range []domain.PricingPlanVersion{fromWorkbook, fromSheet} {
			if again.ContentDigest() != plan.ContentDigest() || again.CanonicalizationVersion() != plan.CanonicalizationVersion() {
				t.Fatalf("%s 读回再导入：摘要 %s（%s），原卡 %s（%s）", plan.Reference().ID(),
					again.ContentDigest(), again.CanonicalizationVersion(), plan.ContentDigest(), plan.CanonicalizationVersion())
			}
		}
		if plan.Reference().ID() != "SYN-PLAN-CN-SG-FULL-03" {
			continue
		}
		got := registrationPlanBlock(t, fromWorkbook, reading.Content.DirectionAuthorization)
		want := registrationPlanBlock(t, plan, reading.Content.DirectionAuthorization)
		if !reflect.DeepEqual(got, want) {
			gotRaw, _ := json.MarshalIndent(got, "", " ")
			wantRaw, _ := json.MarshalIndent(want, "", " ")
			t.Fatalf("FULL-03 的 plan 块读回再导入后变了\n读回：%s\n原卡：%s", gotRaw, wantRaw)
		}
	}
}

// 带指纹的版本引用是导入模板不覆盖的结构：交残缺工作簿会丢掉指纹，所以答业务结论并点名那一处。
func TestProjectRefusesAFingerprintedVersionReference(t *testing.T) {
	reading := read(t, costCard().bytes(t))
	plan := validated(t, reading)
	tenant := value(t, domain.NewTenantID, "SYN-TENANT-01")
	source := try(domain.NewSourceFileIdentity("SYN-CARD.xlsx", strings.Repeat("ab", 32)))
	raw := try(domain.MarshalPriceCardRegistration(try(domain.NewPriceCardRegistration(
		tenant, plan, source, reading.Content.DirectionAuthorization, "SYN-PRICING-GOVERNOR-01"))))
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	reference := document["plan"].(map[string]any)["rateTable"].(map[string]any)["reference"].(map[string]any)
	reference["fingerprint"] = "declared-when-registered"
	stamped, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	registration := try(domain.RehydratePriceCardRegistration(stamped))
	_, gap := projectPriceCard(registration.Plan(), registration.DirectionAuthorization())
	if gap.Where == "" || gap.Conclusion == "" {
		t.Fatal("带指纹的价表引用不该交回工作簿")
	}
	if !strings.Contains(gap.Where, registration.Plan().RateTable().Reference().ID()) {
		t.Fatalf("应点名那张价表，答的是 %s：%s", gap.Where, gap.Conclusion)
	}
}

// 对客价格表与完整投影共用同一份表和金额行，只在 disclosedCardField 那一处拿掉价格方向授权。
func TestDisclosedRateSheetKeepsAmountsAndDropsDirectionAuthorization(t *testing.T) {
	reading := read(t, sellCard().bytes(t))
	plan := validated(t, reading)
	full, fullGap := projectPriceCard(plan, reading.Content.DirectionAuthorization)
	if fullGap.Where != "" {
		t.Fatal(fullGap.Conclusion)
	}
	projection, err := Reader{}.ProjectDisclosedRateSheet(plan, reading.Content.DirectionAuthorization)
	if err != nil || projection.Gap.Where != "" {
		t.Fatalf("对客价格表：%v %s", err, projection.Gap.Conclusion)
	}
	fullText := string(full.xlsx)
	text := string(projection.Document.Body)
	authID := reading.Content.DirectionAuthorization.ID()
	if !strings.Contains(fullText, authID) || !strings.Contains(fullText, "directionAuthorizationId") {
		t.Fatal("完整投影应当还带着价格方向授权，否则看不出对客表删掉了什么")
	}
	if strings.Contains(text, authID) || strings.Contains(text, "directionAuthorizationId") || strings.Contains(text, "directionAuthorizationVersion") {
		t.Fatal("对客价格表不该出现价格方向授权")
	}
	for _, amount := range []string{"55", "18", "48", "15", "5"} {
		if !strings.Contains(text, amount) || !strings.Contains(fullText, amount) {
			t.Fatalf("金额 %s 应当在两份投影里都在", amount)
		}
	}
}
