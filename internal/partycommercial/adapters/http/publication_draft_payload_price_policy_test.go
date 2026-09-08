package commercialhttp_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件证价格规则册在运营操作者面载荷上的那一格（票 admin-write-faces/14）：同一份载荷过预览与过录入逐字节同摘要、
// 正文连同口径（含汇率）与发布期邻接答复原样到达领域；逐格问题按 pricePolicy.* 路径收齐，口径两条件格的在场规则由
// 服务端答在条件格上（显隐是呈现不是裁门）；口径节没有方向键；跨格的绑定矩阵在预览上答`未受理`带成因。

const pricePolicyPayload = `{
  "kind": "PRICE_RULE",
  "objectId": "price-1",
  "version": "v1",
  "scope": "SYN-SCOPE-PC02C",
  "effectiveStartsAt": "2026-08-01T00:00:00Z",
  "pricePolicy": {
    "direction": "SELL",
    "pricingPlan": "PLAN-CN-SG-SELL@v1",
    "planDirection": "BUY",
    "conversion": "FROZEN_BUY_EVALUATION",
    "scope": "pricing-scope-1",
    "effectiveStartsAt": "2026-08-01T00:00:00Z",
    "effectiveEndsAt": "2026-12-31T16:00:00Z",
    "caliber": {
      "taxDisposition": "TAX_EXCLUSIVE",
      "taxClassification": "vat-standard",
      "volumetricFactor": "sell-divisor-5000-cm",
      "fx": {"quoteType": "boc-cash-selling", "asOfSemantics": "AT_ORDER_DATE", "asOfPolicyVersion": "asof-policy/v3"}
    }
  }
}`

func problemFields(t *testing.T, err error) map[string]bool {
	t.Helper()
	var problems *commercialhttp.PublicationPayloadProblems
	if !errors.As(err, &problems) || !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("err = %v (%T), want *PublicationPayloadProblems", err, err)
	}
	fields := map[string]bool{}
	for _, problem := range problems.Problems {
		fields[problem.Field] = true
	}
	return fields
}

// Covers: ADR-0126 Decision 四 — 价格规则载荷过预览与过录入从同一个 Publication 出发，摘要只在领域一处算、逐字节相同；
// 正文七格、口径三格与汇率三格、发布期邻接答复（planDirection / conversion，ADR-0057）原样到达领域，不被推断也不被改写。
func TestPricePolicyPreviewAndSubmissionShareTheDigest(t *testing.T) {
	payload := decodePublication(t, pricePolicyPayload)
	tenant := pcNew(t, domain.NewTenantID, pcTenant)

	previewCommand, err := payload.PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("preview command: %v", err)
	}
	preview, err := application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), previewCommand)
	if err != nil || preview.Outcome() != application.CommercialPublicationPreviewed {
		t.Fatalf("preview = %q, %v (cause %v)", preview.Outcome(), err, preview.RefusalCause())
	}
	previewed, _ := preview.Canonical()
	if !strings.HasPrefix(previewed.Digest().String(), "PCC-1:") {
		t.Fatalf("digest %s does not carry PCC-1", previewed.Digest())
	}

	submitCommand, err := payload.SubmitCommand(tenant, pcNew(t, domain.NewOperatorSubjectReference, "op-submitter"))
	if err != nil {
		t.Fatalf("submit command: %v", err)
	}
	submitted, err := application.NewSubmitPublicationDraftHandler(newDraftStore(), pcClock{at: pcNow}).Handle(context.Background(), submitCommand)
	if err != nil || submitted.Outcome() != application.PublicationDraftSubmitted {
		t.Fatalf("submit = %q, %v", submitted.Outcome(), err)
	}
	draft, _ := submitted.Draft()
	if draft.Canonical().Digest() != previewed.Digest() {
		t.Fatalf("preview digest %s ≠ submitted digest %s", previewed.Digest(), draft.Canonical().Digest())
	}
	if draft.Kind() != domain.PriceRuleObject {
		t.Fatalf("draft kind = %s, want PRICE_RULE", draft.Kind())
	}

	body := submitCommand.Content.PricePolicy
	if body == nil || body.Direction != domain.SellDirection || body.PlanDirection != domain.BuyDirection ||
		body.Conversion != domain.PlanBindingFrozenBuyEvaluation || body.PricingPlan.String() != "PLAN-CN-SG-SELL@v1" ||
		body.Scope.String() != "pricing-scope-1" {
		t.Fatalf("content = %#v", submitCommand.Content)
	}
	if endsAt, bounded := body.Effective.EndsAt(); !bounded || endsAt.IsZero() {
		t.Fatal("effectiveEndsAt did not travel into the interval")
	}
	if body.Caliber == nil {
		t.Fatal("caliber did not travel")
	}
	classification, applies := body.Caliber.Tax.Classification()
	if body.Caliber.Tax.Disposition() != domain.TaxExclusive || !applies || classification.String() != "vat-standard" {
		t.Fatalf("tax caliber = %#v", body.Caliber.Tax)
	}
	factor, declared := body.Caliber.Volumetric.Factor()
	if body.Caliber.Volumetric.Direction() != domain.SellDirection || !declared || factor.String() != "sell-divisor-5000-cm" {
		t.Fatalf("volumetric caliber = %#v", body.Caliber.Volumetric)
	}
	if body.Caliber.Fx == nil || body.Caliber.Fx.QuoteType().String() != "boc-cash-selling" ||
		body.Caliber.Fx.AsOfSemantics().String() != "AT_ORDER_DATE" || body.Caliber.Fx.AsOfPolicyVersion().String() != "asof-policy/v3" {
		t.Fatalf("fx caliber = %#v", body.Caliber.Fx)
	}
}

// Covers: ADR-0126 Decision 四「逐格问题」— 价格政策正文与口径的每一格问题按 pricePolicy.<键> 路径收齐再答，不撞第一格
// 就停；封闭集三格集合外（含空转换——不代填 NONE）各是自己那一格的问题；汇率节在场则三格缺一即点名那一格；立得住的格
// 不被报。
func TestPricePolicyPayloadCollectsEveryFieldProblem(t *testing.T) {
	raw := `{
	  "kind": "PRICE_RULE",
	  "objectId": "price-1",
	  "version": "v1",
	  "scope": "SYN-SCOPE-PC02C",
	  "effectiveStartsAt": "2026-08-01T00:00:00Z",
	  "pricePolicy": {
	    "direction": "SELL",
	    "pricingPlan": "",
	    "planDirection": "BOTH",
	    "conversion": "",
	    "scope": "pricing-scope-1",
	    "effectiveStartsAt": "2026-08-01T00:00:00Z",
	    "effectiveEndsAt": "2026-07-01T00:00:00Z",
	    "caliber": {
	      "taxDisposition": "TAX_MAYBE",
	      "volumetricFactor": "sell-divisor-5000-cm",
	      "fx": {"quoteType": "boc-cash-selling", "asOfSemantics": " ", "asOfPolicyVersion": ""}
	    }
	  }
	}`
	fields := problemFields(t, secondOf(decodePublication(t, raw).Publication(pcNew(t, domain.NewTenantID, pcTenant))))
	for _, want := range []string{"pricePolicy.pricingPlan", "pricePolicy.planDirection", "pricePolicy.conversion",
		"pricePolicy.effectiveEndsAt", "pricePolicy.caliber.taxDisposition",
		"pricePolicy.caliber.fx.asOfSemantics", "pricePolicy.caliber.fx.asOfPolicyVersion"} {
		if !fields[want] {
			t.Errorf("problem for %q missing; got %v", want, fields)
		}
	}
	for _, valid := range []string{"pricePolicy.direction", "pricePolicy.scope", "pricePolicy.effectiveStartsAt",
		"pricePolicy.caliber.volumetricFactor", "pricePolicy.caliber.fx.quoteType", "pricePolicy.caliber.fx", "scope"} {
		if fields[valid] {
			t.Errorf("a valid field %q was reported: %v", valid, fields)
		}
	}
}

// Covers: 票 admin-write-faces/14「显隐是呈现，不是裁门」— 口径两条件格的在场规则由服务端按 CHECK 同形的构造门答在条件格上：
// 含税却没给分类（显了没填）、不适用却给了分类（隐了却传了）都落在 taxClassification；销售方向没给系数、采购方向给了系数都落在
// volumetricFactor。方向自己集合外时系数那一格不再判，问题只在 direction 上。
func TestPricePolicyCaliberCouplingIsAnsweredOnTheConditionalField(t *testing.T) {
	cases := map[string]struct {
		caliber string
		want    string
		absent  string
	}{
		"含税却没给分类":  {caliber: `{"taxDisposition": "TAX_INCLUSIVE"}`, want: "pricePolicy.caliber.taxClassification", absent: "pricePolicy.caliber.taxDisposition"},
		"不适用却给了分类": {caliber: `{"taxDisposition": "TAX_NOT_APPLICABLE", "taxClassification": "vat-standard", "volumetricFactor": "f"}`, want: "pricePolicy.caliber.taxClassification", absent: "pricePolicy.caliber.volumetricFactor"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			raw := strings.Replace(pricePolicyPayload, `"caliber": {
      "taxDisposition": "TAX_EXCLUSIVE",
      "taxClassification": "vat-standard",
      "volumetricFactor": "sell-divisor-5000-cm",
      "fx": {"quoteType": "boc-cash-selling", "asOfSemantics": "AT_ORDER_DATE", "asOfPolicyVersion": "asof-policy/v3"}
    }`, `"caliber": `+tc.caliber, 1)
			if !strings.Contains(raw, tc.caliber) {
				t.Fatalf("fixture did not take the caliber replacement")
			}
			// 销售方向下第一例没给系数，也该在 volumetricFactor 上点名；第二例给了系数则不在。
			fields := problemFields(t, secondOf(decodePublication(t, raw).Publication(pcNew(t, domain.NewTenantID, pcTenant))))
			if !fields[tc.want] {
				t.Fatalf("problem for %q missing; got %v", tc.want, fields)
			}
			if fields[tc.absent] {
				t.Fatalf("%q must not be reported; got %v", tc.absent, fields)
			}
		})
	}

	sellWithoutFactor := strings.Replace(pricePolicyPayload, `"volumetricFactor": "sell-divisor-5000-cm",`, ``, 1)
	fields := problemFields(t, secondOf(decodePublication(t, sellWithoutFactor).Publication(pcNew(t, domain.NewTenantID, pcTenant))))
	if !fields["pricePolicy.caliber.volumetricFactor"] || fields["pricePolicy.caliber.taxClassification"] {
		t.Fatalf("SELL without a factor: got %v", fields)
	}

	buyWithFactor := strings.Replace(pricePolicyPayload, `"direction": "SELL"`, `"direction": "BUY"`, 1)
	buyWithFactor = strings.Replace(buyWithFactor, `"conversion": "FROZEN_BUY_EVALUATION"`, `"conversion": "NONE"`, 1)
	fields = problemFields(t, secondOf(decodePublication(t, buyWithFactor).Publication(pcNew(t, domain.NewTenantID, pcTenant))))
	if !fields["pricePolicy.caliber.volumetricFactor"] || len(fields) != 1 {
		t.Fatalf("BUY with a factor must be reported on volumetricFactor only: got %v", fields)
	}

	unknownDirection := strings.Replace(pricePolicyPayload, `"direction": "SELL"`, `"direction": "sell"`, 1)
	fields = problemFields(t, secondOf(decodePublication(t, unknownDirection).Publication(pcNew(t, domain.NewTenantID, pcTenant))))
	if !fields["pricePolicy.direction"] || fields["pricePolicy.caliber.volumetricFactor"] {
		t.Fatalf("an unknown direction must not cascade into the factor: got %v", fields)
	}
}

// Covers: 口径节没有方向键——载荷里出现 caliber.direction 按未知键拒（判据同受控批文的 pricePolicyCaliberDocument）；正文挂在
// 别的类别下由领域答`正文与类别不符`；SELL 绑 BUY 却把转换写成 NONE 是跨格的绑定矩阵，每格都立得住、解码放行，预览上由领域
// 答`未受理`带 ErrPriceDirectionBindingConflict（AT-PC-033）——表单不裁这道门，服务端答什么显什么。
func TestPricePolicyPayloadHasNoCaliberDirectionAndBindingIsJudgedByTheDomain(t *testing.T) {
	withDirection := strings.Replace(pricePolicyPayload, `"taxDisposition": "TAX_EXCLUSIVE",`, `"direction": "SELL", "taxDisposition": "TAX_EXCLUSIVE",`, 1)
	if _, err := commercialhttp.DecodeCommercialPublicationPayload(strings.NewReader(withDirection)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
		t.Fatalf("caliber with direction: err = %v, want ErrMalformedRequest", err)
	}

	tenant := pcNew(t, domain.NewTenantID, pcTenant)
	underCreditKind := strings.Replace(pricePolicyPayload, `"kind": "PRICE_RULE"`, `"kind": "CREDIT_POLICY"`, 1)
	command, err := decodePublication(t, underCreditKind).PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("preview command: %v", err)
	}
	preview, err := application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), command)
	if err != nil || preview.Outcome() != application.CommercialPublicationPreviewNotAccepted || !errors.Is(preview.RefusalCause(), domain.ErrPublicationContentKindMismatch) {
		t.Fatalf("preview = %q, cause = %v, %v; want NOT_ACCEPTED / ErrPublicationContentKindMismatch", preview.Outcome(), preview.RefusalCause(), err)
	}

	undeclaredConversion := strings.Replace(pricePolicyPayload, `"conversion": "FROZEN_BUY_EVALUATION"`, `"conversion": "NONE"`, 1)
	command, err = decodePublication(t, undeclaredConversion).PreviewCommand(tenant)
	if err != nil {
		t.Fatalf("every field stands on its own; the binding is not a field problem: %v", err)
	}
	preview, err = application.NewPreviewCommercialPublicationHandler().Handle(context.Background(), command)
	if err != nil || preview.Outcome() != application.CommercialPublicationPreviewNotAccepted || !errors.Is(preview.RefusalCause(), domain.ErrPriceDirectionBindingConflict) {
		t.Fatalf("preview = %q, cause = %v, %v; want NOT_ACCEPTED / ErrPriceDirectionBindingConflict", preview.Outcome(), preview.RefusalCause(), err)
	}
}

// secondOf 取 Publication 三返回值里的 error，让断言只看问题集。
func secondOf(_ domain.PublicationDraftShell, _ domain.PublicationContent, err error) error {
	return err
}
