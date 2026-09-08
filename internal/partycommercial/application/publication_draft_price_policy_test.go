package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Covers: 票 admin-write-faces/14「口径随正文同笔登记」— 价格规则载体从录入到发布走一条路：正文连同口径（含汇率）与发布期
// 邻接答复（planDirection / conversion，ADR-0057）一起进载体摘要，发布时由 declarationsOfContent 交回同一份声明，既有发布用例
// 正文先写、口径紧随同一次处理登记；版本按载体摘要入册（对账门恒成立）。不给「先发正文、回头补口径」的两步。
func TestPublishingAPriceRuleDraftRegistersBodyAndCaliberInTheSameStroke(t *testing.T) {
	drafts := newDraftRegistryDouble()
	body := pricePolicyBody(t, domain.SellDirection, domain.BuyDirection, domain.PlanBindingFrozenBuyEvaluation)
	body.Caliber = sellCaliber(t, true)
	content := domain.PublicationContent{Kind: domain.PriceRuleObject, PricePolicy: &domain.PricePolicyBody{
		Direction:     body.Direction,
		PricingPlan:   body.PricingPlan,
		PlanDirection: body.PlanDirection,
		Conversion:    body.Conversion,
		Scope:         body.Scope,
		Effective:     body.Effective,
		Caliber:       &domain.PricePolicyCaliberBody{Tax: body.Caliber.Tax, Volumetric: body.Caliber.Volumetric, Fx: body.Caliber.Fx},
	}}
	shell := draftShell(t, domain.PriceRuleObject, "price-1", "v1")

	submitted, err := application.NewSubmitPublicationDraftHandler(drafts, fixedClock{at: draftSubmittedAt}).Handle(context.Background(),
		application.SubmitPublicationDraftCommand{Shell: shell, Content: content, Submitter: pcValue(t, domain.NewOperatorSubjectReference, "op-submitter")})
	if err != nil || submitted.Outcome() != application.PublicationDraftSubmitted {
		t.Fatalf("录入 = %q, %v", submitted.Outcome(), err)
	}
	approved, err := application.NewApprovePublicationDraftHandler(drafts, approvalDutyRuleDouble{rule: dutyRule(t, true, ""), found: true}, fixedClock{at: draftApprovedAt}).Handle(
		context.Background(), application.ApprovePublicationDraftCommand{
			Tenant: shell.TenantID, Kind: shell.Kind, ObjectID: shell.ObjectID, Version: shell.Version, Approver: operatorSubject(t, "op-approver"),
		})
	if err != nil || approved.Outcome() != application.PublicationDraftApproved {
		t.Fatalf("批准 = %q, %v", approved.Outcome(), err)
	}

	registry := &publicationRegistryDouble{}
	publisher := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: draftPublishedAt}, &operatorRegistrationHandoffDouble{})
	result, err := application.NewPublishPublicationDraftHandler(drafts, publisher, fixedClock{at: draftPublishedAt}).Handle(context.Background(),
		application.PublishPublicationDraftCommand{Tenant: shell.TenantID, Kind: shell.Kind, ObjectID: shell.ObjectID, Version: shell.Version})
	if err != nil {
		t.Fatalf("发布：%v", err)
	}
	publication, ok := result.Publication()
	if result.Outcome() != application.PublicationDraftPublished || !ok || publication.Outcome() != application.CommercialVersionPublishedEffective {
		t.Fatalf("outcome = %q, publication = %q, %v", result.Outcome(), publication.Outcome(), ok)
	}

	if len(registry.savedVersions) != 1 || len(registry.savedPrice) != 1 || len(registry.savedCaliber) != 1 {
		t.Fatalf("saved %d versions / %d price bodies / %d calibers, want 1 / 1 / 1",
			len(registry.savedVersions), len(registry.savedPrice), len(registry.savedCaliber))
	}
	if registry.savedPrice[0].Direction() != domain.SellDirection ||
		registry.savedPlanDirections[0] != domain.BuyDirection ||
		registry.savedConversions[0] != domain.PlanBindingFrozenBuyEvaluation {
		t.Fatalf("方向 / 方案方向 / 转换没有原样从载体到达持久化面：%v %v %v",
			registry.savedPrice[0].Direction(), registry.savedPlanDirections[0], registry.savedConversions[0])
	}
	caliber := registry.savedCaliber[0]
	if fx, declared := caliber.Fx(); !declared || fx.QuoteType().String() != "boc-cash-selling" || caliber.Tax().Disposition() != domain.TaxExclusive {
		t.Fatalf("口径没有原样从载体到达：%#v", caliber)
	}
	if registry.declarationLog[len(registry.declarationLog)-2] != "price-policy-body" ||
		registry.declarationLog[len(registry.declarationLog)-1] != "price-policy-caliber" {
		t.Fatalf("写入顺序 = %v，口径必须紧跟正文", registry.declarationLog)
	}
	stored, _, _ := drafts.LoadDraft(context.Background(), shell.TenantID, shell.Kind, shell.ObjectID, shell.Version)
	if registry.savedVersions[0].ContentDigest() != stored.Canonical().Digest() {
		t.Fatalf("入册摘要 %s ≠ 载体摘要 %s", registry.savedVersions[0].ContentDigest(), stored.Canonical().Digest())
	}
	reports := publication.Declarations()
	if len(reports) != 2 || reports[0].Channel != application.PricePolicyBodyChannel || reports[1].Channel != application.PricePolicyCaliberChannel {
		t.Fatalf("报告 = %#v, want PRICE_POLICY_BODY 与 PRICE_POLICY_CALIBER 各一条", reports)
	}
}
