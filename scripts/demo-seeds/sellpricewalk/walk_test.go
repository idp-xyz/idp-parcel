// Package sellpricewalk 在灌完演示种子的库上核对销售价格来源这条链（票 commercial-four-module-requirements/21）。
//
// 它核三件事，都读灌进库的那一份，不读种子文件：
//
//  1. 两位新客户的合同版本各自声明的来源与顺序，经结算读合同约定的那只点读口读回；旧合同读回「没有约定」。
//  2. 受理前估价与正式 SELL 评价是计价里的两条编排，各走各的有序范围。同一件包裹、同一份来源列表，两条编排要
//     选到同一张卡、算出同一个金额——预估客户费用的金额整组出自正式评价，所以这就是「估价与预估客户费用同额」
//     在计价这一侧的那一半。
//  3. 专属卡没写的线路落到合同声明的后一个范围，评价的解释写得出问过哪几格。
//
// 没有在跑的服务、没有委托：包裹的重量、尺寸与邮编由本文件给，两条编排拿到的是同一份。委托从提交走到预估客户费用
// 形成的那一遍在演示动线里（demo-run/60-sell-price-sources.sh）。评价不入库，演示库不因核对多出行。
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
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

const (
	tenantName      = "SYN-TENANT-01"
	commercialScope = "SYN-SCOPE-01"
	publicScope     = "SYN-SCOPE-01"
	exclusiveScope  = "SYN-SCOPE-EXCL-01"
	publicPlan      = "SYN-PLAN-CN-SG-01"
	exclusivePlan   = "SYN-PLAN-CN-SG-EXCL-01"
	// 目的邮编按演示分区目录 SYN-CAT-ZONE-CN-SG：018 是 Z1，238 是 Z2。
	negotiatedPostal   = "018000"
	unnegotiatedPostal = "238000"
)

// basisAt 是计价基准时点，也当评价形成时刻用：落在种子价卡、目录与燃油序列都在用的 2026 年里。
var basisAt = time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

// TestDemoSellPriceSources 只在指名的演示库上跑。平时的全量测试不带这只库，跳过。
func TestDemoSellPriceSources(t *testing.T) {
	if os.Getenv("IDP_PARCEL_DEMO_CHECK") != "1" {
		t.Skip("要 IDP_PARCEL_DEMO_CHECK=1，并且 IDP_PARCEL_POSTGRES_DSN 指向已灌入演示种子的库（见 check-sell-price-sources.sh）")
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
	walk, err := newWalk(db)
	if err != nil {
		t.Fatalf("装配：%v", err)
	}

	t.Run("合同声明的来源", func(t *testing.T) {
		for _, contract := range []struct {
			id, version string
			sources     []string
		}{
			{"SYN-CONTRACT-21", "v1", []string{publicScope}},
			{"SYN-CONTRACT-22", "v1", []string{exclusiveScope, publicScope}},
			// 旧规范化版本的两份合同没有这一层：读回的是「没有约定」，不是空列表。
			{"SYN-CONTRACT-01", "v2", nil},
			{"SYN-CONTRACT-02", "v1", nil},
		} {
			sources, declared, err := walk.contractSources(ctx, contract.id, contract.version)
			if err != nil {
				t.Fatalf("%s@%s：%v", contract.id, contract.version, err)
			}
			if declared != (contract.sources != nil) || strings.Join(sources, ">") != strings.Join(contract.sources, ">") {
				t.Errorf("%s@%s 读回 声明=%v 来源=%v，应是 声明=%v 来源=%v",
					contract.id, contract.version, declared, sources, contract.sources != nil, contract.sources)
			}
		}
	})

	t.Run("采用册", func(t *testing.T) {
		for _, row := range []struct {
			table, account string
			scope          *string
		}{
			// 旧合同的账户照旧由采用册说计价范围；新形状合同的两户不登这一格。
			{"pre_acceptance_estimate_adoption", "SYN-SETTLEMENT-ACCOUNT-01", text(publicScope)},
			{"sell_billing_moment_adoption", "SYN-SETTLEMENT-ACCOUNT-01", text(publicScope)},
			{"pre_acceptance_estimate_adoption", "SYN-SETTLEMENT-ACCOUNT-21", nil},
			{"sell_billing_moment_adoption", "SYN-SETTLEMENT-ACCOUNT-21", nil},
			{"pre_acceptance_estimate_adoption", "SYN-SETTLEMENT-ACCOUNT-22", nil},
			{"sell_billing_moment_adoption", "SYN-SETTLEMENT-ACCOUNT-22", nil},
		} {
			var scope *string
			// 表名取自上面写死的两项，不来自输入。
			err := pool.QueryRow(ctx,
				`SELECT pricing_scope_id FROM settlement_accounting.`+row.table+` WHERE tenant_id = $1 AND account_id = $2`,
				tenantName, row.account).Scan(&scope)
			if err != nil {
				t.Fatalf("%s %s：%v", row.table, row.account, err)
			}
			if describe(scope) != describe(row.scope) {
				t.Errorf("%s %s 的计价范围是 %s，应是 %s", row.table, row.account, describe(scope), describe(row.scope))
			}
		}
	})

	for _, parcel := range []struct {
		name     string
		contract string
		postal   string
		plan     string
		scope    string
		total    string
		trail    string
	}{
		// 金额是手算的，箱子与演示动线提交的那一件相同：1.50 kg、20×15×10 cm，计费重 1.5 kg，即首重加两档续重。
		// 公共卡 Z1：55 + 2×18 + 5 = 96；专属卡 Z1：50 + 2×16 + 5 = 87；公共卡 Z2：48 + 2×15 + 5 = 83。
		{"只声明公共范围的客户", "SYN-CONTRACT-21", negotiatedPostal, publicPlan, publicScope, "96", ""},
		{"专属在前的客户，谈下来的线路", "SYN-CONTRACT-22", negotiatedPostal, exclusivePlan, exclusiveScope, "87",
			exclusiveScope + ":SELECTED"},
		{"专属在前的客户，没谈到的线路", "SYN-CONTRACT-22", unnegotiatedPostal, publicPlan, publicScope, "83",
			exclusiveScope + ":BASE_RATE_NOT_IN_TABLE," + publicScope + ":SELECTED"},
	} {
		t.Run(parcel.name, func(t *testing.T) {
			sources, declared, err := walk.contractSources(ctx, parcel.contract, "v1")
			if err != nil || !declared {
				t.Fatalf("读 %s 的来源：声明=%v err=%v", parcel.contract, declared, err)
			}
			id := parcel.contract + "-" + parcel.postal

			estimate, err := walk.estimate(ctx, id, sources, parcel.postal)
			if err != nil {
				t.Fatalf("受理前估价：%v", err)
			}
			formal, err := walk.formal(ctx, id, sources, parcel.postal)
			if err != nil {
				t.Fatalf("正式评价：%v", err)
			}
			for _, side := range []struct {
				name string
				got  priced
			}{{"受理前估价", estimate}, {"正式评价", formal}} {
				if side.got.plan != parcel.plan || side.got.scope != parcel.scope || side.got.total != parcel.total+" CNY" {
					t.Errorf("%s 用 %s（范围 %s）算出 %s，应是 %s（范围 %s）%s CNY",
						side.name, side.got.plan, side.got.scope, side.got.total, parcel.plan, parcel.scope, parcel.total)
				}
				if side.got.trail != parcel.trail {
					t.Errorf("%s 问过的来源是 %q，应是 %q", side.name, side.got.trail, parcel.trail)
				}
			}
			if estimate.total != formal.total || estimate.plan != formal.plan {
				t.Errorf("两条编排不同答：受理前估价 %s / %s，正式评价 %s / %s", estimate.plan, estimate.total, formal.plan, formal.total)
			}
			// 解释里逐格写出问过的来源：只带一个范围时没有这种行。
			for _, step := range strings.Split(parcel.trail, ",") {
				if step == "" {
					continue
				}
				scope, kind, _ := strings.Cut(step, ":")
				line := "sell price source " + scope + ": " + kind
				if !contains(formal.explanation, line) {
					t.Errorf("正式评价的解释里没有 %q：%v", line, formal.explanation)
				}
			}
			t.Logf("%s：%s，%s", parcel.name, formal.plan, formal.total)
		})
	}
}

// walk 把两条编排装在灌好的库上：选卡、目录与序列读真库；包裹事实与评价册是本文件的替身。
type walk struct {
	contracts    *pcpostgres.CommercialPublications
	arrangements *pcpostgres.ContractSellPriceArrangements
	formalHandle *application.FormEvaluationFromRequestHandler
	estimates    *application.FormPreAcceptanceEstimatesHandler
	parcels      *declaredParcels
}

func newWalk(db *bentopg.DB) (*walk, error) {
	contracts, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		return nil, err
	}
	arrangements, err := pcpostgres.NewContractSellPriceArrangements(db)
	if err != nil {
		return nil, err
	}
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
	clock := fixedClock{at: basisAt}
	parcels := &declaredParcels{byRequest: map[string]declaredParcel{}}
	store := &memoryEvaluations{}

	formal, err := application.NewFormEvaluationFromRequestHandler(application.FormEvaluationFromRequestDeps{
		Evaluations: store,
		PriceCards:  cards,
		Identity:    identities,
		Evaluate: application.NewEvaluatePricingHandler(application.EvaluatePricingDeps{
			Store:            store,
			Downstream:       noopHandoff{},
			Clock:            clock,
			InForce:          seriesReviews,
			SeriesVersions:   seriesVersions,
			CatalogueInForce: catalogueReviews,
			Catalogues:       catalogueVersions,
		}),
		Inputs:          parcels,
		ServiceProducts: noAcceptedProduct{},
	})
	if err != nil {
		return nil, err
	}
	planGiven, err := application.NewFormPlanGivenEstimateEvaluationsHandler(application.FormPlanGivenEstimateEvaluationsDeps{
		Clock:            clock,
		SeriesInForce:    seriesReviews,
		SeriesVersions:   seriesVersions,
		CatalogueInForce: catalogueReviews,
		Catalogues:       catalogueVersions,
	})
	if err != nil {
		return nil, err
	}
	estimates, err := application.NewFormPreAcceptanceEstimatesHandler(application.FormPreAcceptanceEstimatesDeps{
		PriceCards:   cards,
		Declarations: parcels,
		Estimates:    planGiven,
		Evidence:     domain.EvidenceSynthetic,
	})
	if err != nil {
		return nil, err
	}
	return &walk{contracts: contracts, arrangements: arrangements, formalHandle: formal, estimates: estimates, parcels: parcels}, nil
}

// contractSources 走结算读合同约定的那一口：先在商业范围的整册里找到这一版合同，再点读它的销售价格约定。
// declared 为假即这一版合同没有约定。
func (w *walk) contractSources(ctx context.Context, contract, version string) (sources []string, declared bool, err error) {
	tenant, err := pcdomain.NewTenantID(tenantName)
	if err != nil {
		return nil, false, err
	}
	scope, err := pcdomain.NewCommercialScopeReference(commercialScope)
	if err != nil {
		return nil, false, err
	}
	id, err := pcdomain.NewCommercialObjectID(contract)
	if err != nil {
		return nil, false, err
	}
	label, err := pcdomain.NewCommercialVersionLabel(version)
	if err != nil {
		return nil, false, err
	}
	registry, err := w.contracts.LoadForScope(ctx, tenant, scope)
	if err != nil {
		return nil, false, err
	}
	found, exists := registry.Lookup(tenant, pcdomain.CustomerContractObject, id, label)
	if !exists {
		return nil, false, fmt.Errorf("合同 %s@%s 不在册", contract, version)
	}
	arrangement, declared, err := w.arrangements.LoadContractSellPriceArrangement(ctx, tenant, found)
	if err != nil || !declared {
		return nil, declared, err
	}
	listed, priced := arrangement.Sources()
	if !priced {
		return nil, true, fmt.Errorf("合同 %s@%s 声明的是不计销售价，不是来源列表", contract, version)
	}
	for _, source := range listed {
		sources = append(sources, source.String())
	}
	return sources, true, nil
}

// priced 是一条编排对一件包裹的答案里要比的那几格。
type priced struct {
	plan        string
	scope       string
	total       string
	trail       string
	explanation []string
}

func pricedOf(evaluation domain.PricingEvaluation, trail []application.SellPriceSourceStep) (priced, error) {
	if evaluation.Status() != domain.EvaluationCompleted {
		return priced{}, fmt.Errorf("评价 %s，问题 %v，解释 %v", evaluation.Status(), evaluation.Issues(), evaluation.Explanation())
	}
	total, ok := evaluation.Total()
	if !ok {
		return priced{}, fmt.Errorf("评价没有合计")
	}
	steps := make([]string, 0, len(trail))
	for _, step := range trail {
		steps = append(steps, step.Scope.String()+":"+step.Kind)
	}
	return priced{
		plan:        evaluation.PlanReference().ID(),
		scope:       evaluation.Input().Scope().String(),
		total:       total.Amount().String() + " " + total.Currency().String(),
		trail:       strings.Join(steps, ","),
		explanation: evaluation.Explanation(),
	}, nil
}

func scopesOf(sources []string) ([]domain.PricingScopeID, error) {
	scopes := make([]domain.PricingScopeID, 0, len(sources))
	for _, source := range sources {
		scope, err := domain.NewPricingScopeID(source)
		if err != nil {
			return nil, err
		}
		scopes = append(scopes, scope)
	}
	if len(scopes) == 0 {
		return nil, fmt.Errorf("来源列表是空的")
	}
	return scopes, nil
}

// estimate 走受理前估价那条编排（FormPreAcceptanceEstimatesHandler）。
func (w *walk) estimate(ctx context.Context, id string, sources []string, postal string) (priced, error) {
	scopes, err := scopesOf(sources)
	if err != nil {
		return priced{}, err
	}
	tenant, err := domain.NewTenantID(tenantName)
	if err != nil {
		return priced{}, err
	}
	cny, err := domain.NewCurrency("CNY")
	if err != nil {
		return priced{}, err
	}
	request := "SYN-WALK-REQUEST-" + id
	if err := w.parcels.declare(request, "SYN-WALK-PARCEL-"+id, postal); err != nil {
		return priced{}, err
	}
	result, err := w.estimates.Handle(ctx, application.FormPreAcceptanceEstimatesCommand{
		Tenant:            tenant,
		Scope:             scopes[0],
		Scopes:            scopes,
		ShipmentRequest:   request,
		SubmissionVersion: "v1",
		At:                basisAt,
		Settlement:        cny,
	})
	if err != nil {
		return priced{}, err
	}
	if result.Outcome != application.PreAcceptanceEstimatesFormed || len(result.Estimates) != 1 {
		return priced{}, fmt.Errorf("结果 %s，原因 %d，%d 件", result.Outcome, result.Reason, len(result.Estimates))
	}
	parcel := result.Estimates[0]
	if parcel.Answer != application.EstimateCandidateEvaluated {
		return priced{}, fmt.Errorf("这一件答 %v，缺 %v", parcel.Answer, parcel.Missing)
	}
	return pricedOf(parcel.Evaluation, parcel.SourceTrail)
}

// formal 走正式 SELL 评价那条编排（FormEvaluationFromRequestHandler），预估客户费用的金额出自它。
func (w *walk) formal(ctx context.Context, id string, sources []string, postal string) (priced, error) {
	scopes, err := scopesOf(sources)
	if err != nil {
		return priced{}, err
	}
	tenant, err := domain.NewTenantID(tenantName)
	if err != nil {
		return priced{}, err
	}
	cny, err := domain.NewCurrency("CNY")
	if err != nil {
		return priced{}, err
	}
	reference, err := domain.NewEvaluationRequestReference("SYN-WALK-SELL-REQUEST-" + id)
	if err != nil {
		return priced{}, err
	}
	parcel := "SYN-WALK-PARCEL-" + id
	if err := w.parcels.declare("SYN-WALK-REQUEST-"+id, parcel, postal); err != nil {
		return priced{}, err
	}
	result, err := w.formalHandle.Handle(ctx, application.FormEvaluationFromRequestCommand{
		Tenant:     tenant,
		Request:    reference,
		Scope:      scopes[0],
		Scopes:     scopes,
		Direction:  domain.PricingDirectionSell,
		Purpose:    domain.PricingPurposeCustomerCharge,
		BasisAt:    basisAt,
		Sources:    ports.EligibleSourceReferences{Parcel: parcel},
		Settlement: cny,
		Evidence:   domain.EvidenceSynthetic,
	})
	if err != nil {
		return priced{}, err
	}
	if result.Outcome != application.RequestEvaluationFormed || !result.HasEvaluation {
		return priced{}, fmt.Errorf("结果 %s，原因 %s，缺 %v，问过 %v", result.Outcome, result.Reason, result.Missing, result.ScopeTrail)
	}
	return pricedOf(result.Evaluation, result.ScopeTrail)
}

// declaredParcel 是两条编排共用的那一件包裹：与演示动线提交的那一件同重同尺寸，只有收件邮编按线路换。
type declaredParcel struct {
	parcel     string
	weight     domain.Weight
	dimensions domain.Dimensions
	route      domain.PostalRoute
}

// declaredParcels 同时当受理前估价读申报的口与正式评价造输入的口，两边读到的是同一件。
type declaredParcels struct {
	mu        sync.Mutex
	byRequest map[string]declaredParcel
}

func (parcels *declaredParcels) declare(request, parcel, postal string) error {
	weightValue, err := domain.ParseDecimal("1.50")
	if err != nil {
		return err
	}
	weight, err := domain.NewWeight(weightValue, domain.WeightUnitKilogram)
	if err != nil {
		return err
	}
	sides := make([]domain.Decimal, 0, 3)
	for _, side := range []string{"20", "15", "10"} {
		value, err := domain.ParseDecimal(side)
		if err != nil {
			return err
		}
		sides = append(sides, value)
	}
	dimensions, err := domain.NewDimensions(sides[0], sides[1], sides[2], domain.LengthUnitCentimeter)
	if err != nil {
		return err
	}
	route, err := domain.NewPostalRoute("200000", postal)
	if err != nil {
		return err
	}
	parcels.mu.Lock()
	defer parcels.mu.Unlock()
	parcels.byRequest[request] = declaredParcel{parcel: parcel, weight: weight, dimensions: dimensions, route: route}
	return nil
}

func (parcels *declaredParcels) LoadSubmissionDeclaration(
	_ context.Context,
	_ domain.TenantID,
	shipmentRequest string,
	_ string,
) (ports.SubmissionDeclaration, bool, error) {
	parcels.mu.Lock()
	defer parcels.mu.Unlock()
	declared, found := parcels.byRequest[shipmentRequest]
	if !found {
		return ports.SubmissionDeclaration{}, false, nil
	}
	weight, dimensions, route := declared.weight, declared.dimensions, declared.route
	return ports.SubmissionDeclaration{
		Parcels: []ports.DeclaredParcel{{Parcel: declared.parcel, Weight: &weight, Dimensions: &dimensions}},
		Route:   &route,
	}, true, nil
}

// ResolvePricingInput 照生产上按包裹造快照的形状：只带邮编路线，不带分区，分区留给卡绑的目录去解。
func (parcels *declaredParcels) ResolvePricingInput(_ context.Context, query ports.PricingInputQuery) (ports.PricingInputResolution, error) {
	parcels.mu.Lock()
	defer parcels.mu.Unlock()
	for _, declared := range parcels.byRequest {
		if declared.parcel != query.Sources.Parcel {
			continue
		}
		packageID, err := domain.NewPackageID(declared.parcel)
		if err != nil {
			return ports.PricingInputResolution{}, err
		}
		subject, err := domain.NewAcceptedPackageSubject(packageID)
		if err != nil {
			return ports.PricingInputResolution{}, err
		}
		dimensions := declared.dimensions
		input, err := domain.NewPostalPricingInputSnapshot(
			query.Tenant, query.Scope, subject, declared.route, declared.weight, &dimensions, query.BasisAt)
		if err != nil {
			return ports.PricingInputResolution{}, err
		}
		if settlement, ok := query.SettlementCurrency(); ok {
			if input, err = input.WithSettlementCurrency(settlement); err != nil {
				return ports.PricingInputResolution{}, err
			}
		}
		return ports.PricingInputResolution{Outcome: ports.PricingInputResolved, Input: input}, nil
	}
	return ports.PricingInputResolution{Outcome: ports.PricingInputUnavailable, Missing: []string{"parcel " + query.Sources.Parcel}}, nil
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

// memoryEvaluations 让正式评价不落进演示库。
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

func text(value string) *string { return &value }

func describe(value *string) string {
	if value == nil {
		return "（不带）"
	}
	return *value
}

func contains(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}
