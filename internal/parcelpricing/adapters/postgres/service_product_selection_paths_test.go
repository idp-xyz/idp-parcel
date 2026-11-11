package postgres_test

import (
	"context"
	"testing"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	ppidentity "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/identity"
	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证申报的服务产品进售价卡选卡的三条路（ADR-0378 决定二、三；票 declared-goods-lines/15）：同一计价
// 范围登一张不声明的卡与一张指名 FBA 的卡，接受前估价、正式 SELL 评价与试算都按申报的产品走指名优先；正式评价与试算的快照记着
// 产品；只有指名卡而委托没申报时估价停在未配置。价卡登记册、在用解析与评价库是真的，小包托运那两口与输入读口用替身。夹具全为
// 合成值，证据只记 S。

type declarationWithProduct struct{ declaration ports.SubmissionDeclaration }

func (double declarationWithProduct) LoadSubmissionDeclaration(context.Context, domain.TenantID, string, string) (ports.SubmissionDeclaration, bool, error) {
	return double.declaration, true, nil
}

type acceptedProduct struct {
	product domain.ServiceProductReference
}

func (double acceptedProduct) LoadAcceptedServiceProduct(context.Context, domain.TenantID, string) (domain.ServiceProductReference, error) {
	return double.product, nil
}

// sellParcelInput 是按包裹的 SELL 输入：分区 Z1、1 千克，与 catalogPlan 那张卡对得上。
type sellParcelInput struct{ weight domain.Weight }

func (double sellParcelInput) ResolvePricingInput(_ context.Context, query ports.PricingInputQuery) (ports.PricingInputResolution, error) {
	parcel, err := domain.NewPackageID(query.Sources.Parcel)
	if err != nil {
		return ports.PricingInputResolution{}, err
	}
	subject, err := domain.NewAcceptedPackageSubject(parcel)
	if err != nil {
		return ports.PricingInputResolution{}, err
	}
	input, err := domain.NewPricingInputSnapshot(query.Tenant, query.Scope, subject, "Z1", double.weight, nil, query.BasisAt)
	if err != nil {
		return ports.PricingInputResolution{}, err
	}
	return ports.PricingInputResolution{Outcome: ports.PricingInputResolved, Input: input}, nil
}

type selectionPaths struct {
	db         *bentopg.DB
	catalog    *adapter.PriceCards
	transactor bentoapp.Transactor
	clock      evaluationHandoffClock
}

func newSelectionPaths(t *testing.T, cards ...domain.PricingPlanVersion) selectionPaths {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalog, err := adapter.NewPriceCards(db)
	if err != nil {
		t.Fatalf("构造价卡仓储：%v", err)
	}
	paths := selectionPaths{db: db, catalog: catalog, transactor: db.Transactor(), clock: evaluationHandoffClock{at: catalogAsOf}}
	for _, card := range cards {
		if outcome := registerCard(t, catalog, paths.transactor, t.Context(), cardRegistration(t, "tenant-a", card)); outcome != ports.PriceCardRegistered {
			t.Fatalf("登记 %s = %d", card.Reference().ID(), outcome)
		}
	}
	return paths
}

// preAcceptance 按申报的产品形成受理前估价，交回结果。卡没绑分区目录，受理前没有分区可给，逐件答缺分区——选中的卡照样随结果交回，
// 这里证的是选了哪张。
func (paths selectionPaths) preAcceptance(t *testing.T, product string) application.FormPreAcceptanceEstimatesResult {
	t.Helper()
	planGiven, err := application.NewFormPlanGivenEstimateEvaluationsHandler(application.FormPlanGivenEstimateEvaluationsDeps{Clock: paths.clock})
	if err != nil {
		t.Fatal(err)
	}
	weight := catalogWeight(t, "1")
	handler, err := application.NewFormPreAcceptanceEstimatesHandler(application.FormPreAcceptanceEstimatesDeps{
		PriceCards: paths.catalog,
		Declarations: declarationWithProduct{declaration: ports.SubmissionDeclaration{
			Parcels:        []ports.DeclaredParcel{{Parcel: "SYN-PARCEL-1", Weight: &weight}},
			ServiceProduct: productOf(t, product),
		}},
		Estimates: planGiven,
		Evidence:  domain.EvidenceSynthetic,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := handler.Handle(t.Context(), application.FormPreAcceptanceEstimatesCommand{
		Tenant: evaluationValue(t, domain.NewTenantID, "tenant-a"), Scope: evaluationValue(t, domain.NewPricingScopeID, "scope-1"),
		ShipmentRequest: "SYN-SHR-1", SubmissionVersion: "SYN-SV-1", At: catalogAsOf,
		Settlement: evaluationValue(t, domain.NewCurrency, "USD"),
	})
	if err != nil {
		t.Fatalf("受理前估价（%q）：%v", product, err)
	}
	return result
}

// formal 按包裹形成一份正式 SELL·客户费用评价，产品取「覆盖包裹的已接受委托申报的」那一口。
func (paths selectionPaths) formal(t *testing.T, request, product string) application.FormEvaluationFromRequestResult {
	t.Helper()
	evaluations, err := adapter.NewEvaluations(paths.db)
	if err != nil {
		t.Fatal(err)
	}
	store, err := outbox.NewStore(paths.db)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := adapter.NewOutboxEvaluationHandoff(paths.db, store, paths.clock)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := ppidentity.NewEvaluationIdentities()
	if err != nil {
		t.Fatal(err)
	}
	handler, err := application.NewFormEvaluationFromRequestHandler(application.FormEvaluationFromRequestDeps{
		Evaluations:     evaluations,
		PriceCards:      paths.catalog,
		Identity:        identities,
		Evaluate:        application.NewEvaluatePricingHandler(application.EvaluatePricingDeps{Store: evaluations, Downstream: handoff, Clock: paths.clock}),
		Inputs:          sellParcelInput{weight: catalogWeight(t, "1")},
		ServiceProducts: acceptedProduct{product: productOf(t, product)},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := application.FormEvaluationFromRequestCommand{
		Tenant:    evaluationValue(t, domain.NewTenantID, "tenant-a"),
		Request:   evaluationValue(t, domain.NewEvaluationRequestReference, request),
		Scope:     evaluationValue(t, domain.NewPricingScopeID, "scope-1"),
		Direction: domain.PricingDirectionSell,
		Purpose:   domain.PricingPurposeCustomerCharge,
		BasisAt:   catalogAsOf,
		Sources:   ports.EligibleSourceReferences{Parcel: "SYN-PARCEL-" + request},
		Evidence:  domain.EvidenceSynthetic,
	}
	var result application.FormEvaluationFromRequestResult
	inCatalogTx(t, paths.transactor, t.Context(), func(ctx context.Context) error {
		result, err = handler.Handle(ctx, command)
		return err
	})
	return result
}

// estimate 是试算：带或不带产品，逐卡交回。
func (paths selectionPaths) estimate(t *testing.T, product string) application.FormEstimateEvaluationsResult {
	t.Helper()
	handler, err := application.NewFormEstimateEvaluationsHandler(application.FormEstimateEvaluationsDeps{
		PriceCards: paths.catalog, InForce: paths.catalog, Clock: paths.clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := handler.Handle(t.Context(), application.FormEstimateEvaluationsCommand{
		Tenant: evaluationValue(t, domain.NewTenantID, "tenant-a"), Scope: evaluationValue(t, domain.NewPricingScopeID, "scope-1"),
		Direction: domain.PricingDirectionSell, BasisAt: catalogAsOf, Weight: catalogWeight(t, "1"), Zone: "Z1",
		ServiceProduct: productOf(t, product),
	})
	if err != nil {
		t.Fatalf("试算（%q）：%v", product, err)
	}
	return result
}

func snapshotProduct(evaluation domain.PricingEvaluation) string {
	product, _ := evaluation.Input().ServiceProduct()
	return product.String()
}

func TestTheDeclaredServiceProductPicksTheCardOnAllThreePaths(t *testing.T) {
	paths := newSelectionPaths(t, sellCard(t, "plan-rest"), sellCard(t, "plan-fba", productFBA))

	for index, test := range []struct {
		product string
		card    string
	}{
		{productFBA, "plan-fba"},
		{productExpress, "plan-rest"},
		{"", "plan-rest"},
	} {
		label := test.product
		if label == "" {
			label = "没申报"
		}

		if result := paths.preAcceptance(t, test.product); result.Plan.ID() != test.card {
			t.Fatalf("%s：接受前估价选了 %q（%s），want %s", label, result.Plan.ID(), result.Outcome, test.card)
		}

		formal := paths.formal(t, []string{"R1", "R2", "R3"}[index], test.product)
		if formal.Outcome != application.RequestEvaluationFormed || formal.Evaluation.PlanReference().ID() != test.card {
			t.Fatalf("%s：正式 SELL 评价 %s 用了 %q，want %s", label, formal.Outcome, formal.Evaluation.PlanReference().ID(), test.card)
		}
		if got := snapshotProduct(formal.Evaluation); got != test.product {
			t.Fatalf("%s：正式评价快照记的产品 = %q，want %q", label, got, test.product)
		}

		estimate := paths.estimate(t, test.product)
		if estimate.Outcome != application.EstimateEvaluationsFormed || len(estimate.Candidates) != 1 ||
			estimate.Candidates[0].Plan.ID() != test.card {
			t.Fatalf("%s：试算 %s %+v，want 恰一张 %s", label, estimate.Outcome, estimate.Candidates, test.card)
		}
		if got := snapshotProduct(estimate.Candidates[0].Evaluation); got != test.product {
			t.Fatalf("%s：试算快照记的产品 = %q，want %q", label, got, test.product)
		}
	}
}

func TestOnlyANamedCardAndAnUndeclaredRequestStopsTheEstimateAtNotConfigured(t *testing.T) {
	paths := newSelectionPaths(t, sellCard(t, "plan-fba", productFBA))

	if result := paths.preAcceptance(t, ""); result.Outcome != application.PreAcceptancePriceCardNotConfigured || result.Plan.ID() != "" {
		t.Fatalf("没申报产品、只有指名卡：%s %q，want 未配置、不挑卡", result.Outcome, result.Plan.ID())
	}
	if result := paths.formal(t, "R1", ""); result.Outcome != application.RequestPriceCardNotConfigured {
		t.Fatalf("正式评价：%s，want 未配置", result.Outcome)
	}
	if result := paths.estimate(t, ""); result.Outcome != application.EstimatePriceCardNotConfigured {
		t.Fatalf("试算：%s，want 未配置", result.Outcome)
	}
	if result := paths.preAcceptance(t, productFBA); result.Plan.ID() != "plan-fba" {
		t.Fatalf("申报 FBA：%s %q", result.Outcome, result.Plan.ID())
	}
}
