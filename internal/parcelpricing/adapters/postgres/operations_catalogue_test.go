package postgres_test

import (
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证主数据目录读面(ADR-0077,票 master-data-wiring/02):
// 两册检索列面照实转写(区间/口径/更正/证据等级的可缺席字段不代填)、跨租户不可见、
// 空租户答空列表、limit 生效且非正拒。夹具全部为 SYN-PRC 合成登记(S 级)。

// newPricingCatalogue 与登记侧共用同一个池:pgtest.Pool 每次调用都建全新的库,
// 各建各的就各看各的。
func newPricingCatalogue(t *testing.T) (
	*adapter.OperationsCatalogue,
	*adapter.PriceCards,
	*adapter.ReferenceSeriesVersions,
	bentoapp.Transactor,
) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB:%v", err)
	}
	catalogue, err := adapter.NewOperationsCatalogue(db)
	if err != nil {
		t.Fatalf("构造目录读面:%v", err)
	}
	cards, err := adapter.NewPriceCards(db)
	if err != nil {
		t.Fatalf("构造价卡仓储:%v", err)
	}
	register, err := adapter.NewReferenceSeriesVersions(db)
	if err != nil {
		t.Fatalf("构造序列登记册:%v", err)
	}
	return catalogue, cards, register, db.Transactor()
}

// Covers: 价卡目录照列转写——方向两向并见(方向隔离是评价装载的谓词,不是目录的),
// 证据索引与授权引用照登透出,有界适用期读回;他租的卡不进本租户的列表,空租户答空。
func TestPriceCardCatalogueTranscribesTheColumnFace(t *testing.T) {
	catalogue, cards, _, transactor := newPricingCatalogue(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	sell := catalogPlan(t, "plan-sell", "v1", domain.PricingDirectionSell, "10")
	registerCard(t, cards, transactor, ctx, cardRegistration(t, "tenant-a", sell))
	registerCard(t, cards, transactor, ctx,
		cardRegistration(t, "tenant-a", catalogPlan(t, "plan-buy", "v1", domain.PricingDirectionBuy, "7")))
	registerCard(t, cards, transactor, ctx,
		cardRegistration(t, "tenant-b", catalogPlan(t, "plan-theirs", "v1", domain.PricingDirectionSell, "9")))

	rows, err := catalogue.ListPriceCards(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列价卡:%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("上列 %d 行,want 2(他租的卡不得进本租户列表)", len(rows))
	}
	byPlan := map[string]ports.PriceCardCatalogueRow{}
	for _, row := range rows {
		byPlan[row.PlanID] = row
	}
	if _, bled := byPlan["plan-theirs"]; bled {
		t.Fatal("他租的卡进了本租户的目录")
	}

	sold := byPlan["plan-sell"]
	if sold.PlanVersion != "v1" || sold.Direction != "SELL" || sold.Purpose != "CUSTOMER_CHARGE" ||
		sold.Scope != "scope-1" {
		t.Fatalf("检索列面变形:%+v", sold)
	}
	if sold.Canonicalization != sell.CanonicalizationVersion() || sold.ContentDigest != sell.ContentDigest() {
		t.Fatalf("摘要列变形:%+v", sold)
	}
	if sold.SourceFileName != "SYN-PRC-CARD-260820.xlsx" || sold.SourceFileSHA256 != catalogSourceSHA {
		t.Fatalf("证据索引变形:%+v", sold)
	}
	if sold.AuthorizationID != "SYN-PRC-GRANT" || sold.AuthorizationVersion != "v1" ||
		sold.PublicationApprover != "SYN-PRC-GOVERNANCE" {
		t.Fatalf("授权与批准责任列变形:%+v", sold)
	}
	if !sold.EffectiveFrom.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) ||
		!sold.HasEffectiveTo ||
		!sold.EffectiveTo.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("适用期变形:%+v", sold)
	}
	if sold.RegisteredAt.IsZero() {
		t.Fatal("登记时间没透出")
	}

	if bought := byPlan["plan-buy"]; bought.Direction != "BUY" || bought.Purpose != "SUPPLIER_COST" {
		t.Fatalf("BUY 卡不完整:%+v", bought)
	}

	empty, err := catalogue.ListPriceCards(ctx, evaluationValue(t, domain.NewTenantID, "tenant-empty"), 10)
	if err != nil {
		t.Fatalf("空租户上列:%v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("空租户答了 %d 行,want 0", len(empty))
	}
}

// Covers: 序列册照列转写——燃油无口径且整版断言强度、汇率带口径且可复核、更正版
// 两件成对透出;首版的更正字段如实缺席;他租与空租户同价卡目录一条纪律。
func TestReferenceSeriesCatalogueTranscribesTheColumnFace(t *testing.T) {
	catalogue, _, register, transactor := newPricingCatalogue(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	fuel := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-WEEKLY", "v1")
	if outcome := registerSeries(t, register, transactor, ctx, fuel); outcome != ports.ReferenceSeriesRegistered {
		t.Fatalf("燃油首登 outcome = %d", outcome)
	}

	policy, err := domain.NewVersionReference(
		domain.ArtifactCommercialPolicy, "SYN-PRC-FX-POLICY", "v1", "sha256:syn-fx-policy")
	if err != nil {
		t.Fatalf("构造口径引用:%v", err)
	}
	fx, err := domain.NewReferenceSeriesRegistration(domain.ReferenceSeriesRegistrationSpec{
		Tenant:           tenant,
		Kind:             domain.ReferenceSeriesExchangeRate,
		Reference:        seriesReference(t, "SYN-PRC-USD-CNY", "v1"),
		SourceIdentifier: "SYN-FINANCE/usd-cny-daily",
		Registrant:       "SYN-PRC-SERIES-REGISTRAR",
		QuoteBasis:       policy,
		Periods: []domain.SeriesPeriodValue{
			registerPeriod(t, registerWeekOne, registerWeekTwo, "7.2", "SYN-EVIDENCE/fx-2026-W32"),
		},
	})
	if err != nil {
		t.Fatalf("构造汇率登记:%v", err)
	}
	if outcome := registerSeries(t, register, transactor, ctx, fx); outcome != ports.ReferenceSeriesRegistered {
		t.Fatalf("汇率首登 outcome = %d", outcome)
	}

	correction, err := domain.NewReferenceSeriesRegistration(domain.ReferenceSeriesRegistrationSpec{
		Tenant:           tenant,
		Kind:             domain.ReferenceSeriesFuelRate,
		Reference:        seriesReference(t, "SYN-PRC-FUEL-WEEKLY", "v2"),
		SourceIdentifier: "SYN-CARRIER/fuel-weekly-bulletin",
		Registrant:       "SYN-PRC-SERIES-REGISTRAR",
		Periods: []domain.SeriesPeriodValue{
			registerPeriod(t, registerWeekOne, registerWeekTwo, "0.23", "SYN-EVIDENCE/fuel-2026-W32-corrected"),
		},
		PriorVersion:    seriesReference(t, "SYN-PRC-FUEL-WEEKLY", "v1"),
		CorrectionBasis: "SYN-CORRECTION/fuel-w32-transcription",
	})
	if err != nil {
		t.Fatalf("构造更正登记:%v", err)
	}
	if outcome := registerSeries(t, register, transactor, ctx, correction); outcome != ports.ReferenceSeriesRegistered {
		t.Fatalf("更正登记 outcome = %d", outcome)
	}

	if outcome := registerSeries(t, register, transactor, ctx,
		fuelRegistration(t, "tenant-b", "SYN-PRC-FUEL-WEEKLY", "v1")); outcome != ports.ReferenceSeriesRegistered {
		t.Fatalf("他租登记 outcome = %d", outcome)
	}

	rows, err := catalogue.ListReferenceSeries(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列序列:%v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("上列 %d 行,want 3(他租的序列不得进本租户列表)", len(rows))
	}
	type seriesKey struct{ id, version string }
	byKey := map[seriesKey]ports.ReferenceSeriesCatalogueRow{}
	for _, row := range rows {
		byKey[seriesKey{row.SeriesID, row.SeriesVersion}] = row
	}

	fuelRow := byKey[seriesKey{"SYN-PRC-FUEL-WEEKLY", "v1"}]
	if fuelRow.Kind != "FUEL_RATE" || fuelRow.SourceIdentifier != "SYN-CARRIER/fuel-weekly-bulletin" ||
		fuelRow.Registrant != "SYN-PRC-SERIES-REGISTRAR" {
		t.Fatalf("燃油列面变形:%+v", fuelRow)
	}
	// 夹具第二期缺凭证:整版只有断言强度,目录照登记册汇总透出,不重算不粉饰。
	if fuelRow.EvidenceGrade != "ASSERTED" {
		t.Fatalf("燃油证据等级 = %q, want ASSERTED", fuelRow.EvidenceGrade)
	}
	if fuelRow.HasQuoteBasis {
		t.Fatal("燃油长出了口径(折扣系数写在卡上,无需口径)")
	}
	if fuelRow.IsCorrection || fuelRow.PriorVersion != "" || fuelRow.CorrectionBasis != "" {
		t.Fatalf("首版长出了更正关系:%+v", fuelRow)
	}
	if !fuelRow.EffectiveFrom.Equal(registerWeekOne) ||
		!fuelRow.HasEffectiveTo || !fuelRow.EffectiveTo.Equal(registerWeekThree) {
		t.Fatalf("燃油期次包络变形:%+v", fuelRow)
	}

	fxRow := byKey[seriesKey{"SYN-PRC-USD-CNY", "v1"}]
	if fxRow.Kind != "EXCHANGE_RATE" || fxRow.EvidenceGrade != "VERIFIABLE" {
		t.Fatalf("汇率列面变形:%+v", fxRow)
	}
	if !fxRow.HasQuoteBasis || fxRow.QuoteBasisID != "SYN-PRC-FX-POLICY" || fxRow.QuoteBasisVersion != "v1" {
		t.Fatalf("汇率口径没成对透出:%+v", fxRow)
	}

	corrected := byKey[seriesKey{"SYN-PRC-FUEL-WEEKLY", "v2"}]
	if !corrected.IsCorrection || corrected.PriorVersion != "v1" ||
		corrected.CorrectionBasis != "SYN-CORRECTION/fuel-w32-transcription" {
		t.Fatalf("更正关系没成对透出:%+v", corrected)
	}

	empty, err := catalogue.ListReferenceSeries(ctx, evaluationValue(t, domain.NewTenantID, "tenant-empty"), 10)
	if err != nil {
		t.Fatalf("空租户上列:%v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("空租户答了 %d 行,want 0", len(empty))
	}
}

// Covers: ADR-0077 Decision 五——limit 生效,非正拒(静默答一页会把缺参变成没人
// 决定过的页大小);两口同判据。
func TestPricingCatalogueAppliesTheLimitAndRejectsNonPositive(t *testing.T) {
	catalogue, cards, _, transactor := newPricingCatalogue(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	for _, planID := range []string{"plan-1", "plan-2", "plan-3"} {
		registerCard(t, cards, transactor, ctx,
			cardRegistration(t, "tenant-a", catalogPlan(t, planID, "v1", domain.PricingDirectionSell, "10")))
	}

	rows, err := catalogue.ListPriceCards(ctx, tenant, 2)
	if err != nil {
		t.Fatalf("带 limit 上列:%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("limit 2 交回 %d 行", len(rows))
	}

	if _, err := catalogue.ListPriceCards(ctx, tenant, 0); err == nil {
		t.Fatal("价卡目录 limit 0 未被拒")
	}
	if _, err := catalogue.ListReferenceSeries(ctx, tenant, -1); err == nil {
		t.Fatal("序列册 limit -1 未被拒")
	}
}
