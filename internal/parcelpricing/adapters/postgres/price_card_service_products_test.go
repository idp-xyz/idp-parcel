package postgres_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// newPriceCardsOnPool 同 newPriceCards，另交出底下的连接池：要绕过领域直接改列的用例用它。
func newPriceCardsOnPool(t *testing.T) (*adapter.PriceCards, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalog, err := adapter.NewPriceCards(db)
	if err != nil {
		t.Fatalf("构造价卡仓储：%v", err)
	}
	return catalog, db.Transactor(), pool
}

// 本文件对真实 PostgreSQL 16 证售价卡的适用服务产品（ADR-0378 决定二、四、五）：登记写列、读回与快照核对；内容冲突照摘要判；
// 在用解析按指名优先分两层，同一层多于一张答适用冲突；试算的卡集先取同一层、层内照旧并列；收尾不看产品。夹具全为合成价卡，
// 证据只记 S。

const (
	productFBA     = "SYN-PROD-CN-SG-FBA"
	productExpress = "SYN-PROD-CN-SG-EXPRESS"
)

func named(t *testing.T, plan domain.PricingPlanVersion, products ...string) domain.PricingPlanVersion {
	t.Helper()
	references := make([]domain.ServiceProductReference, 0, len(products))
	for _, product := range products {
		references = append(references, evaluationValue(t, domain.NewServiceProductReference, product))
	}
	declared, err := plan.WithServiceProducts(references...)
	if err != nil {
		t.Fatalf("声明适用产品：%v", err)
	}
	return declared
}

func productOf(t *testing.T, value string) domain.ServiceProductReference {
	t.Helper()
	if value == "" {
		return domain.ServiceProductReference{}
	}
	return evaluationValue(t, domain.NewServiceProductReference, value)
}

func sellCard(t *testing.T, planID string, products ...string) domain.PricingPlanVersion {
	t.Helper()
	plan := catalogPlan(t, planID, "v1", domain.PricingDirectionSell, "10")
	if len(products) == 0 {
		return plan
	}
	return named(t, plan, products...)
}

func resolveForProduct(t *testing.T, catalog *adapter.PriceCards, product string) ports.PriceCardInForceResolution {
	t.Helper()
	resolution, err := catalog.ResolveInForce(t.Context(),
		evaluationValue(t, domain.NewTenantID, "tenant-a"), evaluationValue(t, domain.NewPricingScopeID, "scope-1"),
		domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, catalogAsOf, productOf(t, product))
	if err != nil {
		t.Fatalf("解析在用价卡（%q）：%v", product, err)
	}
	return resolution
}

func loadForProduct(t *testing.T, catalog *adapter.PriceCards, product string) ([]domain.PricingPlanVersion, error) {
	t.Helper()
	return catalog.LoadApplicable(t.Context(), evaluationValue(t, domain.NewTenantID, "tenant-a"), domain.PricingDirectionSell,
		evaluationValue(t, domain.NewPricingScopeID, "scope-1"), catalogAsOf, productOf(t, product))
}

func assertInForce(t *testing.T, resolution ports.PriceCardInForceResolution, planID, label string) {
	t.Helper()
	if resolution.Outcome != ports.PriceCardVersionInForce || resolution.Plan.Reference().ID() != planID {
		t.Fatalf("%s：%s %s %v，want 恰选中 %s", label, resolution.Outcome, resolution.Plan.Reference().ID(), resolution.Candidates, planID)
	}
}

func TestANamedPriceCardRegistersReplaysAndConflictsOnChangedProducts(t *testing.T) {
	catalog, transactor, pool := newPriceCardsOnPool(t)
	ctx := t.Context()

	card := sellCard(t, "plan-fba", productFBA, productExpress)
	if card.CanonicalizationVersion() != "PPC-7" {
		t.Fatalf("指名产品的卡写 %s", card.CanonicalizationVersion())
	}
	if outcome := registerCard(t, catalog, transactor, ctx, cardRegistration(t, "tenant-a", card)); outcome != ports.PriceCardRegistered {
		t.Fatalf("首登 = %d", outcome)
	}
	if outcome := registerCard(t, catalog, transactor, ctx, cardRegistration(t, "tenant-a", card)); outcome != ports.PriceCardAlreadyRegistered {
		t.Fatalf("重放 = %d，want 已登记", outcome)
	}
	changed := sellCard(t, "plan-fba", productFBA)
	if outcome := registerCard(t, catalog, transactor, ctx, cardRegistration(t, "tenant-a", changed)); outcome != ports.PriceCardContentConflict {
		t.Fatalf("同一标识改了适用产品 = %d，want 内容冲突", outcome)
	}

	found, ok, err := catalog.FindByReference(ctx, evaluationValue(t, domain.NewTenantID, "tenant-a"), card.Reference())
	if err != nil || !ok || found.ContentDigest() != card.ContentDigest() || len(found.ServiceProducts()) != 2 {
		t.Fatalf("按引用读回：ok=%v err=%v 产品=%v", ok, err, found.ServiceProducts())
	}
	content, ok, err := catalog.ReadPriceCardContent(ctx, evaluationValue(t, domain.NewTenantID, "tenant-a"), card.Reference())
	if err != nil || !ok {
		t.Fatalf("读价卡内容：ok=%v err=%v", ok, err)
	}
	var document struct {
		ServiceProducts []string `json:"serviceProducts"`
	}
	if err := json.Unmarshal(content.Document, &document); err != nil || len(document.ServiceProducts) != 2 ||
		document.ServiceProducts[0] != productExpress || document.ServiceProducts[1] != productFBA {
		t.Fatalf("内容文档的适用产品 = %v（%v）", document.ServiceProducts, err)
	}

	// 列与快照分岔即行被改过：改掉列面，读回门拒。
	if _, err := catalog.ResolveInForce(ctx, evaluationValue(t, domain.NewTenantID, "tenant-a"),
		evaluationValue(t, domain.NewPricingScopeID, "scope-1"), domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge,
		catalogAsOf, productOf(t, productFBA)); err != nil {
		t.Fatalf("改列之前读得回：%v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE parcel_pricing.price_card_version SET service_products = ARRAY[$1]::text[]
		WHERE tenant_id = 'tenant-a' AND plan_id = 'plan-fba'`, productFBA); err != nil {
		t.Fatalf("改列：%v", err)
	}
	if _, err := catalog.ResolveInForce(ctx, evaluationValue(t, domain.NewTenantID, "tenant-a"),
		evaluationValue(t, domain.NewPricingScopeID, "scope-1"), domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge,
		catalogAsOf, productOf(t, productFBA)); err == nil {
		t.Fatal("列面的适用产品与快照分岔，读回门放行了")
	}
}

func TestOnlySellCustomerChargeRowsMayCarryServiceProductsInTheRegister(t *testing.T) {
	catalog, transactor, pool := newPriceCardsOnPool(t)
	registerCard(t, catalog, transactor, t.Context(),
		cardRegistration(t, "tenant-a", catalogPlan(t, "plan-buy", "v1", domain.PricingDirectionBuy, "8")))
	if _, err := pool.Exec(t.Context(), `UPDATE parcel_pricing.price_card_version SET service_products = ARRAY['X']::text[]
		WHERE tenant_id = 'tenant-a' AND plan_id = 'plan-buy'`); err == nil {
		t.Fatal("BUY 行写进了适用产品：库上那一道检查没拦")
	}
}

// 两层选卡的各格（ADR-0378 决定二），每格一册干净的库。
func TestInForceSelectionByDeclaredProductHasTwoTiers(t *testing.T) {
	type cell struct {
		cards   []domain.PricingPlanVersion
		product string
		want    ports.PriceCardInForceOutcome
		planID  string
		count   int
	}
	cells := map[string]func(t *testing.T) cell{
		"指名一张即它": func(t *testing.T) cell {
			return cell{[]domain.PricingPlanVersion{sellCard(t, "plan-rest"), sellCard(t, "plan-fba", productFBA)}, productFBA, ports.PriceCardVersionInForce, "plan-fba", 0}
		},
		"指名两张冲突": func(t *testing.T) cell {
			return cell{[]domain.PricingPlanVersion{sellCard(t, "plan-rest"), sellCard(t, "plan-fba-a", productFBA), sellCard(t, "plan-fba-b", productFBA, productExpress)},
				productFBA, ports.PriceCardApplicabilityConflict, "", 2}
		},
		"只有不声明的": func(t *testing.T) cell {
			return cell{[]domain.PricingPlanVersion{sellCard(t, "plan-rest")}, productFBA, ports.PriceCardVersionInForce, "plan-rest", 0}
		},
		"申报了没人指名的产品，看不声明的": func(t *testing.T) cell {
			return cell{[]domain.PricingPlanVersion{sellCard(t, "plan-rest"), sellCard(t, "plan-fba", productFBA)}, productExpress, ports.PriceCardVersionInForce, "plan-rest", 0}
		},
		"不声明的两张冲突": func(t *testing.T) cell {
			return cell{[]domain.PricingPlanVersion{sellCard(t, "plan-rest-a"), sellCard(t, "plan-rest-b"), sellCard(t, "plan-express", productExpress)},
				productFBA, ports.PriceCardApplicabilityConflict, "", 2}
		},
		"指名别的产品不适用": func(t *testing.T) cell {
			return cell{[]domain.PricingPlanVersion{sellCard(t, "plan-express", productExpress)}, productFBA, ports.PriceCardNotConfigured, "", 0}
		},
		"零值只看不声明的": func(t *testing.T) cell {
			return cell{[]domain.PricingPlanVersion{sellCard(t, "plan-rest"), sellCard(t, "plan-fba", productFBA)}, "", ports.PriceCardVersionInForce, "plan-rest", 0}
		},
		"零值而只有指名卡，答未配置": func(t *testing.T) cell {
			return cell{[]domain.PricingPlanVersion{sellCard(t, "plan-fba", productFBA)}, "", ports.PriceCardNotConfigured, "", 0}
		},
	}
	for name, build := range cells {
		t.Run(name, func(t *testing.T) {
			catalog, transactor := newPriceCards(t)
			c := build(t)
			for _, card := range c.cards {
				if outcome := registerCard(t, catalog, transactor, t.Context(), cardRegistration(t, "tenant-a", card)); outcome != ports.PriceCardRegistered {
					t.Fatalf("登记 %s = %d", card.Reference().ID(), outcome)
				}
			}
			resolution := resolveForProduct(t, catalog, c.product)
			if resolution.Outcome != c.want || (c.planID != "" && resolution.Plan.Reference().ID() != c.planID) || len(resolution.Candidates) != c.count {
				t.Fatalf("outcome %s 方案 %q 候选 %v；want %s %q %d 个候选", resolution.Outcome, resolution.Plan.Reference().ID(), resolution.Candidates, c.want, c.planID, c.count)
			}
		})
	}
}

// 试算的卡集先取同一层，层内照旧：不同方案并列，同一方案两版才是错误（ADR-0152，协调方对票 14 的更正一）。
func TestLoadApplicableTakesTheSameTierAndKeepsTheTrialRulesWithinIt(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	for _, card := range []domain.PricingPlanVersion{
		sellCard(t, "plan-rest-a"), sellCard(t, "plan-rest-b"),
		sellCard(t, "plan-fba-a", productFBA), sellCard(t, "plan-fba-b", productFBA),
		sellCard(t, "plan-express", productExpress),
	} {
		registerCard(t, catalog, transactor, t.Context(), cardRegistration(t, "tenant-a", card))
	}
	for product, want := range map[string][]string{
		productFBA:      {"plan-fba-a", "plan-fba-b"},
		productExpress:  {"plan-express"},
		"":              {"plan-rest-a", "plan-rest-b"},
		"SYN-PROD-NONE": {"plan-rest-a", "plan-rest-b"},
	} {
		plans, err := loadForProduct(t, catalog, product)
		if err != nil {
			t.Fatalf("%q：%v", product, err)
		}
		var got []string
		for _, plan := range plans {
			got = append(got, plan.Reference().ID())
		}
		if len(got) != len(want) || (len(got) > 0 && (got[0] != want[0] || got[len(got)-1] != want[len(want)-1])) {
			t.Fatalf("%q：卡集 %v，want %v", product, got, want)
		}
	}

	// 同一方案两版都指名这个产品：层内照旧答错误，不挑一版。
	registerCard(t, catalog, transactor, t.Context(),
		cardRegistration(t, "tenant-a", named(t, catalogPlan(t, "plan-express", "v2", domain.PricingDirectionSell, "11"), productExpress)))
	if _, err := loadForProduct(t, catalog, productExpress); !errors.Is(err, ports.ErrAmbiguousPriceCard) {
		t.Fatalf("同一方案两版同指名：%v，want ErrAmbiguousPriceCard", err)
	}
}

// 收尾不看产品（ADR-0378 决定五）：同一方案的后继改了适用产品照样收尾；后继不再指名的产品自截止起由不声明的卡接，没有就答
// 未配置，不回到旧版。
func TestClosureIgnoresProductsAndADroppedProductMovesToTheRestOrNowhere(t *testing.T) {
	fixture := newClosureFixture(t)
	v1 := named(t, sellClosedPlan(t, "v1", closureJanuary, time.Time{}), productFBA, productExpress)
	v2 := named(t, sellClosedPlan(t, "v2", closureJune, time.Time{}), productFBA)
	fixture.register(t, "tenant-a", v1)
	fixture.register(t, "tenant-a", v2)

	at := closureSeptember
	resolve := func(product string) ports.PriceCardInForceResolution {
		resolution, err := fixture.catalog.ResolveInForce(t.Context(), evaluationValue(t, domain.NewTenantID, "tenant-a"),
			evaluationValue(t, domain.NewPricingScopeID, closureScope), domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge,
			at, productOf(t, product))
		if err != nil {
			t.Fatalf("解析（%q）：%v", product, err)
		}
		return resolution
	}
	if got := resolve(productFBA); got.Outcome != ports.PriceCardApplicabilityConflict {
		t.Fatalf("收尾之前同一方案两版同指名 FBA：%s，want 适用冲突", got.Outcome)
	}

	if result := fixture.cut(t, t.Context(), "tenant-a", "v1", "v2", "SYN-CUT-V1-FOR-V2"); result.Outcome() != application.PriceCardClosureRecorded {
		t.Fatalf("后继改了适用产品，收尾 = %s，want 已登记", result.Outcome())
	}
	if got := resolve(productFBA); got.Outcome != ports.PriceCardVersionInForce || got.Plan.Reference().Version() != "v2" {
		t.Fatalf("收尾之后 FBA：%s %v", got.Outcome, got.Plan.Reference())
	}
	if got := resolve(productExpress); got.Outcome != ports.PriceCardNotConfigured {
		t.Fatalf("后继放下的快递产品、册上没有不声明的卡：%s，want 未配置——不回到旧版", got.Outcome)
	}
	if got := resolve(""); got.Outcome != ports.PriceCardNotConfigured {
		t.Fatalf("零值而只有指名卡：%s", got.Outcome)
	}

	rest := sellClosedPlanFor(t, "SYN-PLAN-CLOSE-REST", "v1", closureJanuary, time.Time{})
	fixture.register(t, "tenant-a", rest)
	if got := resolve(productExpress); got.Outcome != ports.PriceCardVersionInForce || got.Plan.Reference().ID() != "SYN-PLAN-CLOSE-REST" {
		t.Fatalf("放下的快递产品有不声明的卡接：%s %v", got.Outcome, got.Plan.Reference())
	}
	before := closureJune.Add(-time.Hour)
	at = before
	if got := resolve(productExpress); got.Outcome != ports.PriceCardVersionInForce || got.Plan.Reference().Version() != "v1" {
		t.Fatalf("截止之前快递仍归旧版：%s %v", got.Outcome, got.Plan.Reference())
	}
}

func sellClosedPlan(t *testing.T, version string, starts, ends time.Time) domain.PricingPlanVersion {
	t.Helper()
	return sellClosedPlanFor(t, closurePlanID, version, starts, ends)
}

// sellClosedPlanFor 是 closedCardPlan 的 SELL·客户费用版：同一计价范围，方案标识由用例给。
func sellClosedPlanFor(t *testing.T, planID, version string, starts, ends time.Time) domain.PricingPlanVersion {
	t.Helper()
	buy := closedCardPlan(t, version, closureScope, starts, ends, domain.PricingPlanStructures{})
	reference, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, planID, version)
	if err != nil {
		t.Fatalf("构造方案引用：%v", err)
	}
	plan, err := domain.NewPricingPlanVersion(reference, buy.Scope(), domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge,
		buy.BaseChargeCode(), buy.EffectivePeriod(), buy.RateTable(), buy.WeightPolicy(), nil, domain.PricingPlanStructures{})
	if err != nil {
		t.Fatalf("构造售价卡 %s/%s：%v", planID, version, err)
	}
	return plan
}

// 价卡目录照列转写适用产品：指名卡交出排序后的产品，不声明的卡是空。
func TestThePriceCardCatalogueTranscribesTheServiceProductsColumn(t *testing.T) {
	catalogue, cards, _, transactor := newPricingCatalogue(t)
	registerCard(t, cards, transactor, t.Context(), cardRegistration(t, "tenant-a", sellCard(t, "plan-fba", productFBA, productExpress)))
	registerCard(t, cards, transactor, t.Context(), cardRegistration(t, "tenant-a", sellCard(t, "plan-rest")))
	rows, err := rowsOf(catalogue.ListPriceCards(t.Context(), evaluationValue(t, domain.NewTenantID, "tenant-a"), 10, firstPage(t, ports.PriceCardPages)))
	if err != nil {
		t.Fatalf("上列价卡：%v", err)
	}
	byPlan := map[string]ports.PriceCardCatalogueRow{}
	for _, row := range rows {
		byPlan[row.PlanID] = row
	}
	if got := byPlan["plan-fba"].ServiceProducts; len(got) != 2 || got[0] != productExpress || got[1] != productFBA {
		t.Fatalf("指名卡的适用产品 = %v", got)
	}
	if got := byPlan["plan-rest"].ServiceProducts; len(got) != 0 {
		t.Fatalf("不声明的卡的适用产品 = %v", got)
	}
}

// 同一方案 v1 不声明、v2 指名 X，两版同时在用（ADR-0378 决定五：后继可以改适用产品）：X 只看指名那一层，得 v2；别的产品与零值只看
// 不声明那一层，得 v1。两版落在不同的层，所以试算的卡集也不报同一方案两版。
func TestTwoVersionsOfOnePlanThatDifferInProductsLandInDifferentTiers(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	registerCard(t, catalog, transactor, t.Context(), cardRegistration(t, "tenant-a", catalogPlan(t, "plan-a", "v1", domain.PricingDirectionSell, "10")))
	registerCard(t, catalog, transactor, t.Context(),
		cardRegistration(t, "tenant-a", named(t, catalogPlan(t, "plan-a", "v2", domain.PricingDirectionSell, "11"), productFBA)))

	for product, want := range map[string]string{productFBA: "v2", productExpress: "v1", "": "v1"} {
		resolution := resolveForProduct(t, catalog, product)
		if resolution.Outcome != ports.PriceCardVersionInForce || resolution.Plan.Reference().Version() != want {
			t.Fatalf("在用解析（%q）：%s %v %v，want plan-a@%s", product, resolution.Outcome, resolution.Plan.Reference(), resolution.Candidates, want)
		}
		plans, err := loadForProduct(t, catalog, product)
		if err != nil || len(plans) != 1 || plans[0].Reference().Version() != want {
			t.Fatalf("试算卡集（%q）：%d 张 err = %v，want 恰 plan-a@%s、不报同一方案两版", product, len(plans), err, want)
		}
	}
}

// 指名 X 的卡过了适用期，X 落回不声明的那张卡：取层在选得中的行里取，过期的指名卡不挡路。
func TestAnExpiredNamedCardFallsThroughToTheUnnamedCard(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	registerCard(t, catalog, transactor, t.Context(),
		cardRegistration(t, "tenant-a", named(t, sellClosedPlanFor(t, "SYN-PLAN-FBA-OLD", "v1", closureJanuary, closureJune), productFBA)))
	registerCard(t, catalog, transactor, t.Context(),
		cardRegistration(t, "tenant-a", sellClosedPlanFor(t, "SYN-PLAN-REST", "v1", closureJanuary, time.Time{})))

	tenant, scope := evaluationValue(t, domain.NewTenantID, "tenant-a"), evaluationValue(t, domain.NewPricingScopeID, closureScope)
	for at, want := range map[time.Time]string{closureJune.Add(-time.Hour): "SYN-PLAN-FBA-OLD", closureSeptember: "SYN-PLAN-REST"} {
		resolution, err := catalog.ResolveInForce(t.Context(), tenant, scope, domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, at, productOf(t, productFBA))
		if err != nil || resolution.Outcome != ports.PriceCardVersionInForce || resolution.Plan.Reference().ID() != want {
			t.Fatalf("%s：%s %v err = %v，want %s", at.Format(time.RFC3339), resolution.Outcome, resolution.Plan.Reference(), err, want)
		}
		plans, err := catalog.LoadApplicable(t.Context(), tenant, domain.PricingDirectionSell, scope, at, productOf(t, productFBA))
		if err != nil || len(plans) != 1 || plans[0].Reference().ID() != want {
			t.Fatalf("%s 试算卡集：%d 张 err = %v，want 恰 %s", at.Format(time.RFC3339), len(plans), err, want)
		}
	}
}
