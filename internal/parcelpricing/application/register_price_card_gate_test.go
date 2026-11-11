package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证登记较新规范化版本的部署开关（ADR-0352）：门在登记用例里、仓储之前，只看方案记的版本是不是基础版。

func gatedRegisterHandler(catalog *priceCardCatalogDouble, handoff *pricingParameterDouble, enabled bool) *application.RegisterPriceCardHandler {
	return application.NewRegisterPriceCardHandler(application.RegisterPriceCardDeps{
		Catalog: catalog, Registered: handoff, NewerCanonicalizationEnabled: enabled,
	})
}

// Covers: AT-PP-048，ADR-0352「基础版之外要开关」——开关关 / 开 × 基础版 / 较新版四格：基础版的卡两种开关下都照常入册并交
// 「参数已登记」；较新版的卡开关关着答 NEWER_CANONICALIZATION_NOT_ENABLED，仓储与交接口一次都不被叫到；开着照常入册。
func TestRegisteringANewerCanonicalizationNeedsTheDeploymentSwitch(t *testing.T) {
	cases := []struct {
		name       string
		plan       func(*testing.T) domain.PricingPlanVersion
		enabled    bool
		want       application.RegisterPriceCardOutcome
		wantCalled bool
	}{
		{"基础版·开关关", minimalPlan, false, application.PriceCardRecorded, true},
		{"基础版·开关开", minimalPlan, true, application.PriceCardRecorded, true},
		{"较新版·开关关", totalBoundedPlan, false, application.PriceCardRegistrationNewerCanonicalizationNotEnabled, false},
		{"较新版·开关开", totalBoundedPlan, true, application.PriceCardRecorded, true},
		// 指名适用服务产品的卡写 PPC-7（ADR-0378 决定四），照 ADR-0352 同一道门。
		{"PPC-7·开关关", serviceProductPlan, false, application.PriceCardRegistrationNewerCanonicalizationNotEnabled, false},
		{"PPC-7·开关开", serviceProductPlan, true, application.PriceCardRecorded, true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			catalog := &priceCardCatalogDouble{outcome: ports.PriceCardRegistered}
			handoff := &pricingParameterDouble{}
			outcome, err := gatedRegisterHandler(catalog, handoff, test.enabled).Handle(context.Background(),
				application.RegisterPriceCardCommand{Registration: registrationFixtureOf(t, test.plan(t))})
			if err != nil {
				t.Fatalf("登记：%v", err)
			}
			if outcome != test.want {
				t.Fatalf("outcome = %s，want %s", outcome, test.want)
			}
			if called := len(catalog.registered) == 1; called != test.wantCalled {
				t.Fatalf("仓储收到 %d 份登记，want 到达仓储 = %v", len(catalog.registered), test.wantCalled)
			}
			if handed := len(handoff.intents) == 1; handed != test.wantCalled {
				t.Fatalf("交接 %d 封，want 交接 = %v", len(handoff.intents), test.wantCalled)
			}
		})
	}
	if name := application.PriceCardRegistrationNewerCanonicalizationNotEnabled.String(); name != "NEWER_CANONICALIZATION_NOT_ENABLED" {
		t.Fatalf("对外码 = %q", name)
	}
}

// Covers: AT-PP-048，ADR-0352 的明示取舍「门不看库」——同一版较新的卡已在册，开关关着时重放也拒：仓储就算会答已在册也不被问到，
// 不补发交接。开关打开后同一份重放照常答已在册。
func TestAReplayOfANewerCanonicalizationIsRefusedWhileTheSwitchIsOff(t *testing.T) {
	registration := registrationFixtureOf(t, totalBoundedPlan(t))
	catalog := &priceCardCatalogDouble{outcome: ports.PriceCardAlreadyRegistered}
	handoff := &pricingParameterDouble{}
	outcome, err := gatedRegisterHandler(catalog, handoff, false).Handle(context.Background(),
		application.RegisterPriceCardCommand{Registration: registration})
	if err != nil || outcome != application.PriceCardRegistrationNewerCanonicalizationNotEnabled {
		t.Fatalf("开关关着的重放：outcome = %s err = %v", outcome, err)
	}
	if len(catalog.registered) != 0 || len(handoff.intents) != 0 {
		t.Fatalf("门之后不该有动作：仓储 %d 次，交接 %d 封", len(catalog.registered), len(handoff.intents))
	}
	outcome, err = gatedRegisterHandler(catalog, handoff, true).Handle(context.Background(),
		application.RegisterPriceCardCommand{Registration: registration})
	if err != nil || outcome != application.PriceCardAlreadyOnRegister {
		t.Fatalf("开关开着的重放：outcome = %s err = %v", outcome, err)
	}
}

// Covers: AT-PP-049，ADR-0352 对批准并发布的约束——登记被门拒时发布答 PUBLICATION_NOT_LANDED、登记那一格原样交回，草稿留在
// `已批准`不推进；开关打开后同一份批准再发布即落定，不用重批。
func TestAPublicationRefusedByTheSwitchCanBeRetriedOnceItIsOn(t *testing.T) {
	approved := approvedPriceCardDraftOf(t, totalBoundedPlan(t))
	drafts := newDraftProgressFake(approved)
	catalog := &priceCardCatalogDouble{outcome: ports.PriceCardRegistered}
	command := application.PublishPriceCardDraftCommand{Tenant: draftTenant(t), Plan: approved.Plan()}

	publisher := func(enabled bool) *application.PublishPriceCardDraftHandler {
		handler, err := application.NewPublishPriceCardDraftHandler(drafts,
			gatedRegisterHandler(catalog, &pricingParameterDouble{}, enabled), fixedClock{at: progressClockAt.Add(time.Minute)})
		if err != nil {
			t.Fatalf("构造发布编排：%v", err)
		}
		return handler
	}

	refused, err := publisher(false).Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("开关关着发布：%v", err)
	}
	if refused.Outcome != application.PriceCardPublicationNotLanded || !refused.HasRegistration ||
		refused.Registration != application.PriceCardRegistrationNewerCanonicalizationNotEnabled || refused.HasDraft {
		t.Fatalf("开关关着发布 = %+v", refused)
	}
	if len(drafts.advanced) != 0 || len(catalog.registered) != 0 {
		t.Fatalf("被门拒的发布不该推进草稿或写登记册：推进 %d 次，登记 %d 次", len(drafts.advanced), len(catalog.registered))
	}
	if still, _, _ := drafts.LoadPriceCardDraft(context.Background(), draftTenant(t), approved.Plan()); still.Status() != domain.PriceCardDraftStatusApproved {
		t.Fatalf("草稿状态 = %v，want 仍是已批准", still.Status())
	}

	retried, err := publisher(true).Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("开关开后重试：%v", err)
	}
	if retried.Outcome != application.PriceCardDraftPublished || retried.Registration != application.PriceCardRecorded {
		t.Fatalf("开关开后重试 = %+v", retried)
	}
}

// serviceProductPlan 是 minimalPlan 指名一个适用服务产品：带了这一格，方案写 PPC-7。
func serviceProductPlan(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	plan, err := minimalPlan(t).WithServiceProducts(mustValue(t, domain.NewServiceProductReference, "SYN-PROD-FBA"))
	if err != nil {
		t.Fatalf("声明适用产品：%v", err)
	}
	if plan.CanonicalizationVersion() != "PPC-7" {
		t.Fatalf("指名产品的方案记 %s，要 PPC-7", plan.CanonicalizationVersion())
	}
	return plan
}
