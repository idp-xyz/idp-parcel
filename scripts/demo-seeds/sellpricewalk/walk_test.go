package sellpricewalk

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	ppidentity "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/identity"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// 灌完演示库之后跑（票 commercial-four-module-requirements/21）。平时的全量测试不带这只库，直接跳过。
func TestDemoSellPriceWalk(t *testing.T) {
	if os.Getenv("IDP_PARCEL_DEMO_CHECK") != "1" {
		t.Skip("要 IDP_PARCEL_DEMO_CHECK=1，并且 DSN 指向已灌入销售价格来源种子的库")
	}
	dsn := os.Getenv("IDP_PARCEL_POSTGRES_DSN")
	if dsn == "" {
		t.Fatal("IDP_PARCEL_POSTGRES_DSN 未设置")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("连接：%v", err)
	}
	t.Cleanup(pool.Close)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	handler, err := newWalkHandler(db)
	if err != nil {
		t.Fatalf("装配：%v", err)
	}

	var report string
	err = db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		var walkErr error
		report, walkErr = walk(txCtx, handler)
		return walkErr
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(report)
}

func newWalkHandler(db *bentopg.DB) (*application.FormEvaluationFromRequestHandler, error) {
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		return nil, err
	}
	seriesReviews, err := pppostgres.NewReferenceSeriesReviews(db)
	if err != nil {
		return nil, err
	}
	seriesVersions, err := pppostgres.NewReferenceSeriesVersions(db)
	if err != nil {
		return nil, err
	}
	catalogueReviews, err := pppostgres.NewReferenceCatalogueReviews(db)
	if err != nil {
		return nil, err
	}
	catalogueVersions, err := pppostgres.NewReferenceCatalogueVersions(db)
	if err != nil {
		return nil, err
	}
	identities, err := ppidentity.NewEvaluationIdentities()
	if err != nil {
		return nil, err
	}
	store := &memoryEvaluations{}
	at := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	evaluate := application.NewEvaluatePricingHandler(application.EvaluatePricingDeps{
		Store:            store,
		Downstream:       noopHandoff{},
		Clock:            fixedClock{at: at},
		InForce:          seriesReviews,
		SeriesVersions:   seriesVersions,
		CatalogueInForce: catalogueReviews,
		Catalogues:       catalogueVersions,
	})
	return application.NewFormEvaluationFromRequestHandler(application.FormEvaluationFromRequestDeps{
		Evaluations:     store,
		PriceCards:      cards,
		Identity:        identities,
		Evaluate:        evaluate,
		Inputs:          postalInputs{at: at},
		ServiceProducts: noAcceptedProduct{},
	})
}

func walk(ctx context.Context, handler *application.FormEvaluationFromRequestHandler) (string, error) {
	public := []string{"SYN-SCOPE-01"}
	exclusiveFirst := []string{"SYN-SCOPE-EXCL-01", "SYN-SCOPE-01"}

	estimate21, err := form(ctx, handler, "SYN-REQ-21-ESTIMATE", "SYN-PKG-21", public)
	if err != nil {
		return "", err
	}
	charge21, err := form(ctx, handler, "SYN-REQ-21-CHARGE", "SYN-PKG-21", public)
	if err != nil {
		return "", err
	}
	if err := sameMoney("客户 21", estimate21, charge21); err != nil {
		return "", err
	}
	if estimate21.evaluation.PlanReference().ID() != "SYN-PLAN-CN-SG-01" {
		return "", fmt.Errorf("客户 21 用了 %s，应是公共卡", estimate21.evaluation.PlanReference().ID())
	}

	estimate22, err := form(ctx, handler, "SYN-REQ-22-ESTIMATE", "SYN-PKG-22", exclusiveFirst)
	if err != nil {
		return "", err
	}
	charge22, err := form(ctx, handler, "SYN-REQ-22-CHARGE", "SYN-PKG-22", exclusiveFirst)
	if err != nil {
		return "", err
	}
	if err := sameMoney("客户 22", estimate22, charge22); err != nil {
		return "", err
	}
	if estimate22.evaluation.PlanReference().ID() != "SYN-PLAN-CN-SG-EXCL-01" {
		return "", fmt.Errorf("专属线路用了 %s，应是专属卡", estimate22.evaluation.PlanReference().ID())
	}
	if got := trail(estimate22.result.ScopeTrail); !strings.HasPrefix(got, "SYN-SCOPE-EXCL-01:") || strings.Contains(got, "SYN-SCOPE-01:") {
		return "", fmt.Errorf("专属线路的来源轨迹是 %s，不该去问公共范围", got)
	}

	missed, err := form(ctx, handler, "SYN-REQ-22-UNNEGOTIATED", "SYN-PKG-22-Z2", exclusiveFirst)
	if err != nil {
		return "", err
	}
	if missed.evaluation.PlanReference().ID() != "SYN-PLAN-CN-SG-01" {
		return "", fmt.Errorf("没谈到的线路用了 %s，应落到公共卡", missed.evaluation.PlanReference().ID())
	}
	if got := trail(missed.result.ScopeTrail); !strings.HasPrefix(got, "SYN-SCOPE-EXCL-01:BASE_RATE_NOT_IN_TABLE,SYN-SCOPE-01:") {
		return "", fmt.Errorf("没谈到的线路的来源轨迹是 %s", got)
	}
	explanation := strings.Join(missed.evaluation.Explanation(), "\n")
	for _, line := range []string{
		"sell price source SYN-SCOPE-EXCL-01: BASE_RATE_NOT_IN_TABLE",
		"sell price source SYN-SCOPE-01:",
	} {
		if !strings.Contains(explanation, line) {
			return "", fmt.Errorf("解释里没有 %q\n%s", line, explanation)
		}
	}
	return fmt.Sprintf("客户 21 同额 %s；客户 22 专属线路同额 %s；没谈到的线路落到 %s",
		estimate21.money, estimate22.money, missed.money), nil
}

type formed struct {
	result     application.FormEvaluationFromRequestResult
	evaluation domain.PricingEvaluation
	money      string
}

func form(ctx context.Context, handler *application.FormEvaluationFromRequestHandler, request, parcel string, scopes []string) (formed, error) {
	none := formed{}
	command, err := commandOf(request, parcel, scopes)
	if err != nil {
		return none, err
	}
	result, err := handler.Handle(ctx, command)
	if err != nil {
		return none, fmt.Errorf("%s：%w", request, err)
	}
	if result.Outcome != application.RequestEvaluationFormed || !result.HasEvaluation {
		return none, fmt.Errorf("%s 结果 %s，原因 %s，缺 %v", request, result.Outcome, result.Reason, result.Missing)
	}
	if result.Evaluation.Status() != domain.EvaluationCompleted {
		return none, fmt.Errorf("%s 评价 %s，问题 %v，解释 %v", request, result.Evaluation.Status(), result.Evaluation.Issues(), result.Evaluation.Explanation())
	}
	total, ok := result.Evaluation.Total()
	if !ok {
		return none, fmt.Errorf("%s 没有合计", request)
	}
	return formed{result: result, evaluation: result.Evaluation, money: total.Amount().String() + " " + total.Currency().String()}, nil
}

func sameMoney(label string, estimate, charge formed) error {
	if estimate.money != charge.money {
		return fmt.Errorf("%s 受理前估价 %s，预估客户费用 %s", label, estimate.money, charge.money)
	}
	return nil
}

func commandOf(request, parcel string, scopes []string) (application.FormEvaluationFromRequestCommand, error) {
	none := application.FormEvaluationFromRequestCommand{}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		return none, err
	}
	reference, err := domain.NewEvaluationRequestReference(request)
	if err != nil {
		return none, err
	}
	scope, err := domain.NewPricingScopeID(scopes[0])
	if err != nil {
		return none, err
	}
	listed := make([]domain.PricingScopeID, 0, len(scopes))
	for _, each := range scopes {
		parsed, err := domain.NewPricingScopeID(each)
		if err != nil {
			return none, err
		}
		listed = append(listed, parsed)
	}
	currency, err := domain.NewCurrency("CNY")
	if err != nil {
		return none, err
	}
	return application.FormEvaluationFromRequestCommand{
		Tenant:     tenant,
		Request:    reference,
		Scope:      scope,
		Scopes:     listed,
		Direction:  domain.PricingDirectionSell,
		Purpose:    domain.PricingPurposeCustomerCharge,
		BasisAt:    time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		Sources:    ports.EligibleSourceReferences{Parcel: parcel},
		Settlement: currency,
		Evidence:   domain.EvidenceSynthetic,
	}, nil
}

func trail(steps []application.SellPriceSourceStep) string {
	parts := make([]string, 0, len(steps))
	for _, step := range steps {
		parts = append(parts, step.Scope.String()+":"+step.Kind)
	}
	return strings.Join(parts, ",")
}

type postalInputs struct {
	at time.Time
}

func (inputs postalInputs) ResolvePricingInput(_ context.Context, query ports.PricingInputQuery) (ports.PricingInputResolution, error) {
	none := ports.PricingInputResolution{}
	subject, err := domain.NewAcceptedPackageSubject(mustPackage(query.Sources.Parcel))
	if err != nil {
		return none, err
	}
	weight, err := domain.NewWeight(mustDecimal("1"), domain.WeightUnitKilogram)
	if err != nil {
		return none, err
	}
	sides, err := domain.NewDimensions(mustDecimal("10"), mustDecimal("10"), mustDecimal("10"), domain.LengthUnitCentimeter)
	if err != nil {
		return none, err
	}
	route, err := domain.NewPostalRoute("200000", destinationOf(query.Sources.Parcel))
	if err != nil {
		return none, err
	}
	snapshot, err := domain.NewPostalPricingInputSnapshot(query.Tenant, query.Scope, subject, route, weight, &sides, inputs.at)
	if err != nil {
		return none, err
	}
	if currency, ok := query.SettlementCurrency(); ok {
		snapshot, err = snapshot.WithSettlementCurrency(currency)
		if err != nil {
			return none, err
		}
	}
	return ports.PricingInputResolution{Outcome: ports.PricingInputResolved, Input: snapshot}, nil
}

func destinationOf(parcel string) string {
	if strings.HasSuffix(parcel, "-Z2") {
		return "238000"
	}
	return "018000"
}

func mustPackage(id string) domain.PackageID {
	parsed, err := domain.NewPackageID(id)
	if err != nil {
		panic(err)
	}
	return parsed
}

func mustDecimal(value string) domain.Decimal {
	parsed, err := domain.ParseDecimal(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

type noAcceptedProduct struct{}

func (noAcceptedProduct) LoadAcceptedServiceProduct(context.Context, domain.TenantID, string) (domain.ServiceProductReference, error) {
	return domain.ServiceProductReference{}, nil
}

type noopHandoff struct{}

func (noopHandoff) HandOffEvaluation(context.Context, ports.EvaluationHandoffIntent) error {
	return nil
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

type memoryEvaluations struct {
	mu        sync.Mutex
	byID      map[string]domain.PricingEvaluation
	byRequest map[string]domain.PricingEvaluation
}

func (store *memoryEvaluations) FindByID(_ context.Context, id domain.EvaluationID) (domain.PricingEvaluation, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	found, ok := store.byID[id.String()]
	return found, ok, nil
}

func (store *memoryEvaluations) Save(_ context.Context, evaluation domain.PricingEvaluation) (ports.EvaluationSaveOutcome, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.byID == nil {
		store.byID = map[string]domain.PricingEvaluation{}
		store.byRequest = map[string]domain.PricingEvaluation{}
	}
	store.byID[evaluation.ID().String()] = evaluation
	if reference, ok := evaluation.RequestReference(); ok {
		store.byRequest[reference.String()] = evaluation
	}
	return ports.EvaluationSaved, nil
}

func (store *memoryEvaluations) FindByRequestReference(_ context.Context, _ domain.TenantID, reference domain.EvaluationRequestReference) (domain.PricingEvaluation, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	found, ok := store.byRequest[reference.String()]
	return found, ok, nil
}
