package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证三条路把申报的服务产品交给选卡、记进快照（ADR-0382 决定三）：接受前估价取这一版申报的产品；正式 SELL 评价取覆盖包裹的
// 已接受委托申报的产品，BUY 不问；试算取请求带的。真库上的选卡结果见 adapters/postgres 的 service_product_selection_paths_test。

func TestPreAcceptanceSelectsByTheDeclaredProductAndRecordsItOnEachEstimate(t *testing.T) {
	declaration := routedDeclaration(t, measuredParcel(t, "SHR-SYN-01/1"))
	declaration.ServiceProduct = mustValue(t, domain.NewServiceProductReference, "SYN-PROD-CN-SG-FBA")
	fixture := newPreAcceptanceFixture(t, inForceCard(catalogueBoundPlan(t)), declaration)

	result, err := fixture.handler.Handle(context.Background(), preAcceptanceCommand(t))
	if err != nil || result.Outcome != application.PreAcceptanceEstimatesFormed || len(result.Estimates) != 1 {
		t.Fatalf("result = %+v err = %v", result, err)
	}
	if len(fixture.cards.products) != 1 || fixture.cards.products[0].String() != "SYN-PROD-CN-SG-FBA" {
		t.Fatalf("选卡用的产品 = %v，want 这一版申报的 FBA", fixture.cards.products)
	}
	if product, declared := result.Estimates[0].Evaluation.Input().ServiceProduct(); !declared || product.String() != "SYN-PROD-CN-SG-FBA" {
		t.Fatalf("估价快照记的产品 = %q %v", product.String(), declared)
	}

	undeclared := newPreAcceptanceFixture(t, inForceCard(catalogueBoundPlan(t)), routedDeclaration(t, measuredParcel(t, "SHR-SYN-01/1")))
	result, err = undeclared.handler.Handle(context.Background(), preAcceptanceCommand(t))
	if err != nil || len(undeclared.cards.products) != 1 || undeclared.cards.products[0].Declared() {
		t.Fatalf("没申报：选卡用的产品 = %v err = %v，want 零值", undeclared.cards.products, err)
	}
	if _, declared := result.Estimates[0].Evaluation.Input().ServiceProduct(); declared {
		t.Fatal("没申报产品的估价快照记了产品")
	}
}

func TestAFormalSellEvaluationSelectsByTheAcceptedRequestsProductAndBuyNeverAsks(t *testing.T) {
	sell := newSellFixture(t)
	sell.products.product = mustValue(t, domain.NewServiceProductReference, "SYN-PROD-CN-SG-FBA")
	result, err := sell.handler.Handle(context.Background(), sellCommand(t))
	if err != nil || result.Outcome != application.RequestEvaluationFormed {
		t.Fatalf("result = %+v err = %v", result, err)
	}
	if len(sell.products.parcels) != 1 || sell.products.parcels[0] != "SYN-PARCEL-1" {
		t.Fatalf("产品读口问的包裹 = %v，want 请求点名的那一件", sell.products.parcels)
	}
	if len(sell.cards.products) != 1 || sell.cards.products[0].String() != "SYN-PROD-CN-SG-FBA" {
		t.Fatalf("选卡用的产品 = %v", sell.cards.products)
	}
	if product, declared := result.Evaluation.Input().ServiceProduct(); !declared || product.String() != "SYN-PROD-CN-SG-FBA" {
		t.Fatalf("正式评价快照记的产品 = %q %v", product.String(), declared)
	}

	buy := newFormFixture(t, resolvedInputs(t))
	buy.products.product = mustValue(t, domain.NewServiceProductReference, "SYN-PROD-CN-SG-FBA")
	if result, err := buy.handler.Handle(context.Background(), requestCommand(t)); err != nil || result.Outcome != application.RequestEvaluationFormed {
		t.Fatalf("BUY：result = %+v err = %v", result, err)
	}
	if len(buy.products.parcels) != 0 {
		t.Fatalf("BUY 请求问了产品读口 %d 次，want 一次都不问", len(buy.products.parcels))
	}
}

func TestAnUnreadableAcceptedProductLeavesTheFormalEvaluationUndecidedBeforeSelection(t *testing.T) {
	sell := newSellFixture(t)
	sell.products.err = errors.New("shipment read unreachable")
	result, err := sell.handler.Handle(context.Background(), sellCommand(t))
	if err != nil || result.Outcome != application.RequestEvaluationUndecided || result.Reason != application.RequestServiceProductUnavailable {
		t.Fatalf("result = %+v err = %v，want 未决、点名产品读口", result, err)
	}
	if len(sell.cards.directions) != 0 || sell.store.saved != 0 || sell.identity.minted != 0 {
		t.Fatalf("产品读不回却选了卡 %d 次、入册 %d、铸造 %d", len(sell.cards.directions), sell.store.saved, sell.identity.minted)
	}
	if application.RequestServiceProductUnavailable.String() != "SERVICE_PRODUCT_UNAVAILABLE" {
		t.Fatalf("对外码 = %q", application.RequestServiceProductUnavailable.String())
	}
}

func TestATrialPassesItsProductToTheCardSetAndTheSnapshot(t *testing.T) {
	cards := &applicablePriceCardsDouble{plans: []domain.PricingPlanVersion{minimalPlan(t)}}
	handler, err := application.NewFormEstimateEvaluationsHandler(application.FormEstimateEvaluationsDeps{
		PriceCards: cards, InForce: &priceCardResolverDouble{}, Clock: fixedClock{at: requestBasisAt},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := application.FormEstimateEvaluationsCommand{
		Tenant: mustValue(t, domain.NewTenantID, "tenant-1"), Scope: mustValue(t, domain.NewPricingScopeID, "scope-1"),
		Direction: domain.PricingDirectionSell, BasisAt: requestBasisAt, Weight: mustWeight(t, "1"), Zone: "Z1",
		ServiceProduct: mustValue(t, domain.NewServiceProductReference, "SYN-PROD-CN-SG-FBA"),
	}
	result, err := handler.Handle(context.Background(), command)
	if err != nil || result.Outcome != application.EstimateEvaluationsFormed || len(result.Candidates) != 1 {
		t.Fatalf("result = %+v err = %v", result, err)
	}
	if len(cards.products) != 1 || cards.products[0].String() != "SYN-PROD-CN-SG-FBA" {
		t.Fatalf("卡集按的产品 = %v", cards.products)
	}
	evaluation := result.Candidates[0].Evaluation
	if product, declared := evaluation.Input().ServiceProduct(); !declared || product.String() != "SYN-PROD-CN-SG-FBA" {
		t.Fatalf("试算快照记的产品 = %q %v", product.String(), declared)
	}

	// 产品不进试算标识：同一份声明、同一张卡，带不带产品是同一份评价标识（受理前估价的留痕要对得上复算）。
	command.ServiceProduct = domain.ServiceProductReference{}
	plain, err := handler.Handle(context.Background(), command)
	if err != nil || len(plain.Candidates) != 1 || plain.Candidates[0].Evaluation.ID() != evaluation.ID() {
		t.Fatalf("不带产品：%+v err = %v", plain, err)
	}
	if len(cards.products) != 2 || cards.products[1].Declared() {
		t.Fatalf("不带产品时卡集按的产品 = %v，want 零值", cards.products)
	}
}

var _ ports.AcceptedServiceProductView = (*acceptedProductDouble)(nil)

// 只有 SELL·客户费用的卡能声明适用产品（ADR-0382 决定一）：BUY 或 INTERNAL 的试算带了产品，受理门拒，卡集一次都不装——否则
// 快照与解释会说那张卡是按产品选的。
func TestATrialOnANonSellDirectionRefusesAServiceProduct(t *testing.T) {
	for _, direction := range []domain.PricingDirection{domain.PricingDirectionBuy, domain.PricingDirectionInternal} {
		cards := &applicablePriceCardsDouble{plans: []domain.PricingPlanVersion{minimalPlan(t)}}
		handler, err := application.NewFormEstimateEvaluationsHandler(application.FormEstimateEvaluationsDeps{
			PriceCards: cards, InForce: &priceCardResolverDouble{}, Clock: fixedClock{at: requestBasisAt},
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := handler.Handle(context.Background(), application.FormEstimateEvaluationsCommand{
			Tenant: mustValue(t, domain.NewTenantID, "tenant-1"), Scope: mustValue(t, domain.NewPricingScopeID, "scope-1"),
			Direction: direction, BasisAt: requestBasisAt, Weight: mustWeight(t, "1"), Zone: "Z1",
			ServiceProduct: mustValue(t, domain.NewServiceProductReference, "SYN-PROD-CN-SG-FBA"),
		})
		if err != nil || result.Outcome != application.EstimateNotAccepted || cards.calls != 0 {
			t.Fatalf("%s 带产品：%s err = %v 装载 %d 次，want 未受理、一次都不装", direction, result.Outcome, err, cards.calls)
		}
	}
}

// 未配置与适用冲突两格交回选卡用的产品：消费侧要说清是哪个产品没有卡、哪个产品上撞了。
func TestTheFormalStopCellsCarryTheProductTheySelectedBy(t *testing.T) {
	for name, resolution := range map[string]ports.PriceCardInForceResolution{
		"未配置":  {Outcome: ports.PriceCardNotConfigured},
		"适用冲突": {Outcome: ports.PriceCardApplicabilityConflict, Candidates: []domain.VersionReference{sellPlan(t).Reference()}},
	} {
		sell := newSellFixture(t)
		sell.priceCards.resolution = resolution
		sell.products.product = mustValue(t, domain.NewServiceProductReference, "SYN-PROD-CN-SG-FBA")
		result, err := sell.handler.Handle(context.Background(), sellCommand(t))
		if err != nil || result.ServiceProduct.String() != "SYN-PROD-CN-SG-FBA" {
			t.Fatalf("%s：%s 带的产品 = %q err = %v", name, result.Outcome, result.ServiceProduct.String(), err)
		}
	}
}
