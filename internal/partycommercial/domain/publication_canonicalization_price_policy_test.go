package domain_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证价格规则册接进 PCC-1 的那一格（票 admin-write-faces/14）：同一正文两次算同串、发布期邻接答复与口径各是正文
// 的一部分、文档键名镜像受控批文、与发布时同一套门在预览上就答、快照折得回正文。

func pricePolicyBodyFor(t *testing.T, direction, planDirection domain.PriceDirection, conversion domain.PlanBindingConversion) domain.PricePolicyBody {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	return domain.PricePolicyBody{
		Direction:     direction,
		PricingPlan:   commercialValue(t, domain.NewPricingPlanReference, "plan-1@v1"),
		PlanDirection: planDirection,
		Conversion:    conversion,
		Scope:         commercialValue(t, domain.NewCommercialScopeReference, "pricing-scope-1"),
		Effective:     interval,
	}
}

func taxCaliber(t *testing.T, disposition domain.TaxDisposition, classification string) domain.TaxCaliber {
	t.Helper()
	var reference domain.TaxClassificationReference
	if classification != "" {
		reference = commercialValue(t, domain.NewTaxClassificationReference, classification)
	}
	tax, err := domain.NewTaxCaliber(disposition, reference)
	if err != nil {
		t.Fatalf("new tax caliber: %v", err)
	}
	return tax
}

func volumetricCaliber(t *testing.T, direction domain.PriceDirection, factor string) domain.VolumetricCaliber {
	t.Helper()
	var reference domain.VolumetricFactorReference
	if factor != "" {
		reference = commercialValue(t, domain.NewVolumetricFactorReference, factor)
	}
	volumetric, err := domain.NewVolumetricCaliber(direction, reference)
	if err != nil {
		t.Fatalf("new volumetric caliber: %v", err)
	}
	return volumetric
}

// sellPolicyWithCaliber 是一份 SELL 政策带完整口径（未税 + 分类、销售系数、汇率三格）；汇率取 price_policy_caliber_test.go
// 的 fxCaliber。
func sellPolicyWithCaliber(t *testing.T, withFx bool) domain.PricePolicyBody {
	t.Helper()
	body := pricePolicyBodyFor(t, domain.SellDirection, domain.SellDirection, domain.PlanBindingConversionNone)
	caliber := &domain.PricePolicyCaliberBody{
		Tax:        taxCaliber(t, domain.TaxExclusive, "vat-standard"),
		Volumetric: volumetricCaliber(t, domain.SellDirection, "sell-divisor-5000-cm"),
	}
	if withFx {
		fx := fxCaliber(t)
		caliber.Fx = &fx
	}
	body.Caliber = caliber
	return body
}

func canonicalPricePolicy(t *testing.T, body domain.PricePolicyBody) domain.CanonicalPublicationContent {
	t.Helper()
	canonical, err := domain.CanonicalizePublicationContent(domain.PublicationContent{
		Kind:        domain.PriceRuleObject,
		PricePolicy: &body,
	})
	if err != nil {
		t.Fatalf("canonicalize price policy: %v", err)
	}
	return canonical
}

// Covers: ADR-0126 Decision 一（加册不换号）— 价格规则接进 PCC-1：同一正文两次算逐字节同串；发布期邻接答复（planDirection、
// conversion，ADR-0057 Decision 四）与口径节各是正文的一部分——换一格就是另一个串；口径在不在、汇率在不在也各是另一个串。
func TestPricePolicyDigestIsStableAndCoversBindingAndCaliber(t *testing.T) {
	first := canonicalPricePolicy(t, sellPolicyWithCaliber(t, true))
	again := canonicalPricePolicy(t, sellPolicyWithCaliber(t, true))
	if first.Digest() != again.Digest() {
		t.Fatalf("same price policy body produced two digests: %s / %s", first.Digest(), again.Digest())
	}
	if first.Canonicalization() != "PCC-1" || !strings.HasPrefix(first.Digest().String(), "PCC-1:") {
		t.Fatalf("price policy must be canonicalized under PCC-1, got %s", first.Digest())
	}
	if !domain.IsRegisterCanonicalized(domain.PriceRuleObject) {
		t.Fatal("IsRegisterCanonicalized must answer true for PRICE_RULE once this register is wired")
	}

	withoutFx := canonicalPricePolicy(t, sellPolicyWithCaliber(t, false))
	if withoutFx.Digest() == first.Digest() {
		t.Fatal("dropping the fx caliber must change the digest")
	}
	bare := canonicalPricePolicy(t, pricePolicyBodyFor(t, domain.SellDirection, domain.SellDirection, domain.PlanBindingConversionNone))
	if bare.Digest() == withoutFx.Digest() {
		t.Fatal("dropping the whole caliber must change the digest")
	}
	crossBound := canonicalPricePolicy(t, pricePolicyBodyFor(t, domain.SellDirection, domain.BuyDirection, domain.PlanBindingFrozenBuyEvaluation))
	if crossBound.Digest() == bare.Digest() {
		t.Fatal("binding a BUY plan with a declared conversion must not share a digest with a same-direction binding")
	}

	// 同一时刻两种时区写法不产生第二个串（UTC 归一）。
	shanghai := time.FixedZone("Asia/Shanghai", 8*3600)
	inUTC := pricePolicyBodyFor(t, domain.BuyDirection, domain.BuyDirection, domain.PlanBindingConversionNone)
	inShanghai := inUTC
	inUTC.Effective = commercialInterval(t, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC))
	inShanghai.Effective = commercialInterval(t, time.Date(2026, 1, 3, 8, 0, 0, 0, shanghai), time.Date(2026, 7, 1, 0, 0, 0, 0, shanghai))
	if canonicalPricePolicy(t, inUTC).Digest() != canonicalPricePolicy(t, inShanghai).Digest() {
		t.Fatal("same instants in two zones produced two digests")
	}
}

func commercialInterval(t *testing.T, startsAt, endsAt time.Time) domain.EffectiveInterval {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		t.Fatalf("new effective interval: %v", err)
	}
	return interval
}

// Covers: 票 admin-write-faces/14「canonical 文档键名镜像受控批文」— pricePolicy 节的键与 cmd/parcel-commercial 批文
// pricePolicyBodyDocument / pricePolicyCaliberDocument / fxCaliberDocument 同名；口径节没有方向键；三处可缺的键缺席即不在场
// （不适用因而没有分类、采购方向因而没有系数、不涉外币因而没有 fx）；没口径的正文整键缺席。
func TestPricePolicyDocumentMirrorsTheBatchKeys(t *testing.T) {
	canonical := canonicalPricePolicy(t, sellPolicyWithCaliber(t, true))

	var document struct {
		Canonicalization string `json:"canonicalization"`
		Kind             string `json:"kind"`
		PricePolicy      struct {
			Direction         string                     `json:"direction"`
			PricingPlan       string                     `json:"pricingPlan"`
			PlanDirection     string                     `json:"planDirection"`
			Conversion        string                     `json:"conversion"`
			Scope             string                     `json:"scope"`
			EffectiveStartsAt string                     `json:"effectiveStartsAt"`
			Caliber           map[string]json.RawMessage `json:"caliber"`
		} `json:"pricePolicy"`
	}
	if err := json.Unmarshal(canonical.Document(), &document); err != nil {
		t.Fatalf("decode document %s: %v", canonical.Document(), err)
	}
	if document.Canonicalization != "PCC-1" || document.Kind != "PRICE_RULE" {
		t.Fatalf("document header = %q / %q", document.Canonicalization, document.Kind)
	}
	body := document.PricePolicy
	if body.Direction != "SELL" || body.PricingPlan != "plan-1@v1" || body.PlanDirection != "SELL" || body.Conversion != "NONE" ||
		body.Scope != "pricing-scope-1" || body.EffectiveStartsAt != "2026-01-03T00:00:00Z" {
		t.Fatalf("pricePolicy = %#v", body)
	}
	if string(body.Caliber["taxDisposition"]) != `"TAX_EXCLUSIVE"` || string(body.Caliber["taxClassification"]) != `"vat-standard"` ||
		string(body.Caliber["volumetricFactor"]) != `"sell-divisor-5000-cm"` {
		t.Fatalf("caliber = %s", body.Caliber)
	}
	if _, present := body.Caliber["direction"]; present {
		t.Fatalf("the caliber node must not carry a direction key: %s", body.Caliber)
	}
	var fx map[string]string
	if err := json.Unmarshal(body.Caliber["fx"], &fx); err != nil {
		t.Fatalf("decode fx %s: %v", body.Caliber["fx"], err)
	}
	if fx["quoteType"] != "boc-cash-selling" || fx["asOfSemantics"] != "AT_ORDER_DATE" || fx["asOfPolicyVersion"] != "asof-policy/v3" {
		t.Fatalf("fx = %#v", fx)
	}
	if strings.Contains(string(canonical.Document()), "effectiveEndsAt") {
		t.Fatalf("an open-ended interval must omit effectiveEndsAt: %s", canonical.Document())
	}

	buyNotApplicable := pricePolicyBodyFor(t, domain.BuyDirection, domain.BuyDirection, domain.PlanBindingConversionNone)
	buyNotApplicable.Caliber = &domain.PricePolicyCaliberBody{
		Tax:        taxCaliber(t, domain.TaxNotApplicable, ""),
		Volumetric: volumetricCaliber(t, domain.BuyDirection, ""),
	}
	sparse := string(canonicalPricePolicy(t, buyNotApplicable).Document())
	for _, absent := range []string{"taxClassification", "volumetricFactor", `"fx"`} {
		if strings.Contains(sparse, absent) {
			t.Fatalf("BUY / TAX_NOT_APPLICABLE without fx must omit %s: %s", absent, sparse)
		}
	}
	bare := string(canonicalPricePolicy(t, pricePolicyBodyFor(t, domain.BuyDirection, domain.BuyDirection, domain.PlanBindingConversionNone)).Document())
	if strings.Contains(bare, "caliber") {
		t.Fatalf("a body without a caliber must omit the whole key: %s", bare)
	}
}

// Covers: 票 admin-write-faces/14「显隐是呈现不是裁门」— 折成文档前过的是与发布时同一套门：逐格非零（ErrInvalidPricePolicy）、
// 未声明转换的跨向绑定（ErrPriceDirectionBindingConflict，AT-PC-033）、同向却声明转换（ErrInvalidPricePolicy）、口径零格与
// 指向零值的 fx（ErrInvalidPricePolicyCaliber）、口径方向与政策方向不一致（ErrPricePolicyCaliberDirectionMismatch）；
// 正文缺席与冒别册的名照旧分开答。
func TestPricePolicyCanonicalizationRefusesWhatPublicationWouldRefuse(t *testing.T) {
	_, err := domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.PriceRuleObject})
	if !errors.Is(err, domain.ErrPublicationContentAbsent) {
		t.Fatalf("no body: err = %v, want ErrPublicationContentAbsent", err)
	}
	body := pricePolicyBodyFor(t, domain.SellDirection, domain.SellDirection, domain.PlanBindingConversionNone)
	_, err = domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.CreditPolicyObject, PricePolicy: &body})
	if !errors.Is(err, domain.ErrPublicationContentKindMismatch) {
		t.Fatalf("price body under credit kind: err = %v, want ErrPublicationContentKindMismatch", err)
	}

	mismatched := pricePolicyBodyFor(t, domain.BuyDirection, domain.BuyDirection, domain.PlanBindingConversionNone)
	mismatched.Caliber = &domain.PricePolicyCaliberBody{
		Tax:        taxCaliber(t, domain.TaxNotApplicable, ""),
		Volumetric: volumetricCaliber(t, domain.SellDirection, "sell-divisor-5000-cm"),
	}
	zeroTax := sellPolicyWithCaliber(t, false)
	zeroTax.Caliber = &domain.PricePolicyCaliberBody{Volumetric: zeroTax.Caliber.Volumetric}
	zeroFx := sellPolicyWithCaliber(t, false)
	zeroFx.Caliber = &domain.PricePolicyCaliberBody{Tax: zeroFx.Caliber.Tax, Volumetric: zeroFx.Caliber.Volumetric, Fx: &domain.FxCaliber{}}

	refusals := map[string]struct {
		body domain.PricePolicyBody
		want error
	}{
		"零值正文":       {body: domain.PricePolicyBody{}, want: domain.ErrInvalidPricePolicy},
		"未声明转换的跨向绑定": {body: pricePolicyBodyFor(t, domain.SellDirection, domain.BuyDirection, domain.PlanBindingConversionNone), want: domain.ErrPriceDirectionBindingConflict},
		"采购政策绑销售方案":  {body: pricePolicyBodyFor(t, domain.BuyDirection, domain.SellDirection, domain.PlanBindingFrozenBuyEvaluation), want: domain.ErrPriceDirectionBindingConflict},
		"同向却声明转换":    {body: pricePolicyBodyFor(t, domain.SellDirection, domain.SellDirection, domain.PlanBindingFrozenBuyEvaluation), want: domain.ErrInvalidPricePolicy},
		"口径方向与政策不一致": {body: mismatched, want: domain.ErrPricePolicyCaliberDirectionMismatch},
		"口径税务格为零":    {body: zeroTax, want: domain.ErrInvalidPricePolicyCaliber},
		"汇率指向零值":     {body: zeroFx, want: domain.ErrInvalidPricePolicyCaliber},
	}
	for name, refusal := range refusals {
		t.Run(name, func(t *testing.T) {
			body := refusal.body
			_, err := domain.CanonicalizePublicationContent(domain.PublicationContent{Kind: domain.PriceRuleObject, PricePolicy: &body})
			if !errors.Is(err, refusal.want) {
				t.Fatalf("err = %v, want %v", err, refusal.want)
			}
		})
	}
}

// Covers: ADR-0126 Decision 三（正文快照）— 价格规则的规范化文档折得回正文：方向、方案引用、发布期邻接答复、范围、区间与
// 口径（税务分类、销售系数、汇率三格）逐格回到领域值对象，折回去再算一遍与列里的摘要相等；没口径折回 nil、没 fx 折回 nil。
// 快照里坏一格（SELL 绑 BUY 却把转换改成 NONE；采购方向却带系数）折不回——快照是数据，正文立不立得住仍由构造门说。
func TestPricePolicyDocumentRehydratesToTheSameBody(t *testing.T) {
	body := sellPolicyWithCaliber(t, true)
	canonical := canonicalPricePolicy(t, body)

	content, err := domain.RehydratePublicationContent(canonical.Canonicalization(), canonical.Document())
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if content.Kind != domain.PriceRuleObject || content.PricePolicy == nil || content.CreditPolicy != nil {
		t.Fatalf("rehydrated content = %#v", content)
	}
	rehydrated := content.PricePolicy
	if rehydrated.Direction != domain.SellDirection || rehydrated.PlanDirection != domain.SellDirection ||
		rehydrated.Conversion != domain.PlanBindingConversionNone || rehydrated.PricingPlan != body.PricingPlan ||
		rehydrated.Scope != body.Scope || rehydrated.Effective != body.Effective {
		t.Fatalf("rehydrated body = %#v, want %#v", rehydrated, body)
	}
	if rehydrated.Caliber == nil {
		t.Fatal("caliber did not rehydrate")
	}
	if rehydrated.Caliber.Tax != body.Caliber.Tax || rehydrated.Caliber.Volumetric != body.Caliber.Volumetric {
		t.Fatalf("caliber = %#v, want %#v", rehydrated.Caliber, body.Caliber)
	}
	if rehydrated.Caliber.Fx == nil || *rehydrated.Caliber.Fx != *body.Caliber.Fx {
		t.Fatalf("fx = %#v, want %#v", rehydrated.Caliber.Fx, body.Caliber.Fx)
	}
	recomputed, err := domain.CanonicalizePublicationContent(content)
	if err != nil {
		t.Fatalf("recanonicalize: %v", err)
	}
	if recomputed.Digest() != canonical.Digest() {
		t.Fatalf("rehydrated body digests to %s, column says %s", recomputed.Digest(), canonical.Digest())
	}

	withoutFx := canonicalPricePolicy(t, sellPolicyWithCaliber(t, false))
	content, err = domain.RehydratePublicationContent(withoutFx.Canonicalization(), withoutFx.Document())
	if err != nil || content.PricePolicy == nil || content.PricePolicy.Caliber == nil || content.PricePolicy.Caliber.Fx != nil {
		t.Fatalf("an absent fx must rehydrate to nil: %#v, %v", content.PricePolicy, err)
	}
	bare := canonicalPricePolicy(t, pricePolicyBodyFor(t, domain.SellDirection, domain.BuyDirection, domain.PlanBindingFrozenBuyEvaluation))
	content, err = domain.RehydratePublicationContent(bare.Canonicalization(), bare.Document())
	if err != nil || content.PricePolicy == nil || content.PricePolicy.Caliber != nil {
		t.Fatalf("an absent caliber must rehydrate to nil: %#v, %v", content.PricePolicy, err)
	}
	if content.PricePolicy.PlanDirection != domain.BuyDirection || content.PricePolicy.Conversion != domain.PlanBindingFrozenBuyEvaluation {
		t.Fatalf("publish-time adjacent replies did not rehydrate: %#v", content.PricePolicy)
	}

	corruptedBinding := strings.Replace(string(bare.Document()), `"conversion":"FROZEN_BUY_EVALUATION"`, `"conversion":"NONE"`, 1)
	if _, err := domain.RehydratePublicationContent(bare.Canonicalization(), []byte(corruptedBinding)); !errors.Is(err, domain.ErrPriceDirectionBindingConflict) {
		t.Fatalf("corrupted binding: err = %v, want ErrPriceDirectionBindingConflict", err)
	}
	corruptedCaliber := strings.Replace(string(canonical.Document()), `"direction":"SELL"`, `"direction":"BUY"`, 1)
	corruptedCaliber = strings.Replace(corruptedCaliber, `"planDirection":"SELL"`, `"planDirection":"BUY"`, 1)
	if _, err := domain.RehydratePublicationContent(canonical.Canonicalization(), []byte(corruptedCaliber)); !errors.Is(err, domain.ErrInvalidVolumetricCaliber) {
		t.Fatalf("a BUY policy carrying a volumetric factor: err = %v, want ErrInvalidVolumetricCaliber", err)
	}
}

// Covers: 反查与 String() 同一份名单——规范化文档与运营载荷里的方向、转换、税务口径都是那一个词；集合外、空串与小写答
// false，空转换不折成 NONE、空税务口径不折成不适用。
func TestPricePolicyClosedSetsRoundTripByName(t *testing.T) {
	for _, direction := range []domain.PriceDirection{domain.BuyDirection, domain.SellDirection, domain.InternalDirection} {
		if named, known := domain.PriceDirectionNamed(direction.String()); !known || named != direction {
			t.Fatalf("%s: PriceDirectionNamed = %v, %v", direction, named, known)
		}
	}
	for _, conversion := range []domain.PlanBindingConversion{domain.PlanBindingConversionNone, domain.PlanBindingFrozenBuyEvaluation} {
		if named, known := domain.PlanBindingConversionNamed(conversion.String()); !known || named != conversion {
			t.Fatalf("%s: PlanBindingConversionNamed = %v, %v", conversion, named, known)
		}
	}
	for _, disposition := range []domain.TaxDisposition{domain.TaxInclusive, domain.TaxExclusive, domain.TaxNotApplicable} {
		if named, known := domain.TaxDispositionNamed(disposition.String()); !known || named != disposition {
			t.Fatalf("%s: TaxDispositionNamed = %v, %v", disposition, named, known)
		}
	}
	for _, name := range []string{"", "sell", "BOTH", "none", "TAX_MAYBE"} {
		if _, known := domain.PriceDirectionNamed(name); known {
			t.Fatalf("%q must not name a direction", name)
		}
		if _, known := domain.PlanBindingConversionNamed(name); known {
			t.Fatalf("%q must not name a conversion", name)
		}
		if _, known := domain.TaxDispositionNamed(name); known {
			t.Fatalf("%q must not name a tax disposition", name)
		}
	}
}
