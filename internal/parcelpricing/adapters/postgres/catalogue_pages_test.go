package postgres_test

import (
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证本上下文五册的 ADR-0144 读半边（票 catalogue-read-pagination-rollout/04）：按游标翻页不重不漏
// ——翻页途中新落的行排在游标之前就不再出现、排在之后就接着出现，同刻的行被页界切开也不丢；筛选、检索与换序都在库里做，
// total 随条件变；q 按字面包含匹配，% 与 _ 不当通配；空租户答空页。价卡与序列的登记时刻取库时钟，用例要的同刻与先后由池
// 事后写定。夹具全部为 SYN 合成值（S 级）。

var pageBase = time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)

// assertPage 断言一页的行（按序）、总数与有没有下一页。
func assertPage(t *testing.T, label string, keys []string, total int64, next string, want []string, wantTotal int64, wantMore bool) {
	t.Helper()
	if !reflect.DeepEqual(keys, want) || total != wantTotal || (next != "") != wantMore {
		t.Fatalf("%s：行 %v、总数 %d、有下一页 %v；想要 %v、%d、%v", label, keys, total, next != "", want, wantTotal, wantMore)
	}
}

func evaluationIDsOf(rows []ports.EvaluationCatalogueRow) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.EvaluationID)
	}
	return ids
}

func planKeysOf(rows []ports.PriceCardCatalogueRow) []string {
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, row.PlanID+"@"+row.PlanVersion)
	}
	return keys
}

func seriesKeysOf(rows []ports.ReferenceSeriesCatalogueRow) []string {
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, row.SeriesID+"@"+row.SeriesVersion)
	}
	return keys
}

func coverageKeysOf(rows []ports.ReferenceSeriesCoverageRow) []string {
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, row.SeriesID+"/"+row.Kind)
	}
	return keys
}

// pagedPricing 是价卡、序列与覆盖三册翻页用例的夹具：各读面与登记口共用一个库，池留给用例写定登记时刻。
type pagedPricing struct {
	pool       *pgxpool.Pool
	catalogue  *adapter.OperationsCatalogue
	cards      *adapter.PriceCards
	series     *adapter.ReferenceSeriesVersions
	reviews    *adapter.ReferenceSeriesReviews
	coverage   *adapter.ReferenceSeriesCoverage
	transactor bentoapp.Transactor
}

func newPagedPricing(t *testing.T) pagedPricing {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	fixture := pagedPricing{pool: pool, transactor: db.Transactor()}
	if fixture.catalogue, err = adapter.NewOperationsCatalogue(db); err != nil {
		t.Fatalf("构造目录读面：%v", err)
	}
	if fixture.cards, err = adapter.NewPriceCards(db); err != nil {
		t.Fatalf("构造价卡仓储：%v", err)
	}
	if fixture.series, err = adapter.NewReferenceSeriesVersions(db); err != nil {
		t.Fatalf("构造序列登记册：%v", err)
	}
	if fixture.reviews, err = adapter.NewReferenceSeriesReviews(db); err != nil {
		t.Fatalf("构造复核册：%v", err)
	}
	if fixture.coverage, err = adapter.NewReferenceSeriesCoverage(db); err != nil {
		t.Fatalf("构造覆盖读口：%v", err)
	}
	return fixture
}

// card 登一张卡，并把它的登记时刻写定在 pageBase 之后 offset 处。
func (fixture pagedPricing) card(t *testing.T, tenant, planID, version string, direction domain.PricingDirection, offset time.Duration) {
	t.Helper()
	registration := cardRegistration(t, tenant, catalogPlan(t, planID, version, direction, "10"))
	if outcome := registerCard(t, fixture.cards, fixture.transactor, t.Context(), registration); outcome != ports.PriceCardRegistered {
		t.Fatalf("登记 %s@%s outcome = %d", planID, version, outcome)
	}
	fixture.stamp(t, `UPDATE parcel_pricing.price_card_version SET registered_at = $4
		WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3`, tenant, planID, version, pageBase.Add(offset))
}

// seriesVersion 登一版序列并写定登记时刻，交回那份登记供复核用。
func (fixture pagedPricing) seriesVersion(
	t *testing.T,
	registration domain.ReferenceSeriesRegistration,
	offset time.Duration,
) domain.ReferenceSeriesRegistration {
	t.Helper()
	reference := registration.Reference()
	if outcome := registerSeries(t, fixture.series, fixture.transactor, t.Context(), registration); outcome != ports.ReferenceSeriesRegistered {
		t.Fatalf("登记 %s@%s outcome = %d", reference.ID(), reference.Version(), outcome)
	}
	fixture.stamp(t, `UPDATE parcel_pricing.reference_series_version SET registered_at = $4
		WHERE tenant_id = $1 AND series_id = $2 AND series_version = $3`,
		registration.Tenant().String(), reference.ID(), reference.Version(), pageBase.Add(offset))
	return registration
}

func (fixture pagedPricing) stamp(t *testing.T, statement string, arguments ...any) {
	t.Helper()
	tag, err := fixture.pool.Exec(t.Context(), statement, arguments...)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("写定登记时刻：%v（%d 行）", err, tag.RowsAffected())
	}
}

// exchangeRegistration 是一版带口径的汇率序列登记，用来与燃油序列分出种类一维。
func exchangeRegistration(t *testing.T, tenant, seriesID, version string) domain.ReferenceSeriesRegistration {
	t.Helper()
	policy, err := domain.NewVersionReference(
		domain.ArtifactCommercialPolicy, "SYN-PRC-FX-POLICY", "v1", "sha256:syn-fx-policy")
	if err != nil {
		t.Fatalf("构造口径引用：%v", err)
	}
	registration, err := domain.NewReferenceSeriesRegistration(domain.ReferenceSeriesRegistrationSpec{
		Tenant:           evaluationValue(t, domain.NewTenantID, tenant),
		Kind:             domain.ReferenceSeriesExchangeRate,
		Reference:        seriesReference(t, seriesID, version),
		SourceIdentifier: "SYN-FINANCE/usd-cny-daily",
		Registrant:       "SYN-PRC-SERIES-REGISTRAR",
		QuoteBasis:       policy,
		Periods: []domain.SeriesPeriodValue{
			registerPeriod(t, registerWeekOne, registerWeekTwo, "7.2", "SYN-EVIDENCE/fx-2026-W32"),
		},
	})
	if err != nil {
		t.Fatalf("构造汇率登记：%v", err)
	}
	return registration
}

func seedEvaluations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := t.Context()
	insertEvaluationRow(t, pool, ctx, "SYN-EVAL-001", "SYN-TENANT-01", "COMPLETED", pageBase)
	insertEvaluationRow(t, pool, ctx, "SYN-EVAL-002", "SYN-TENANT-01", "FAILED", pageBase.Add(time.Hour))
	insertEvaluationRow(t, pool, ctx, "SYN-EVAL-003", "SYN-TENANT-01", "CONFLICT", pageBase.Add(3*time.Hour))
	insertEvaluationRow(t, pool, ctx, "SYN-EVAL-004", "SYN-TENANT-01", "COMPLETED", pageBase.Add(3*time.Hour))
	insertEvaluationRow(t, pool, ctx, "SYN-EVAL-005", "SYN-TENANT-01", "UNRATABLE", pageBase.Add(4*time.Hour))
	insertEvaluationRow(t, pool, ctx, "SYN-EVAL-THEIRS", "SYN-TENANT-02", "COMPLETED", pageBase.Add(4*time.Hour))
}

// Covers: ADR-0144 决定一、三、五——评价册按落册时刻倒序、同刻按评价号倒序翻页。翻页途中新落两行：比游标新的那行不再出现，
// 与游标同刻而标识排在其后的那行在下一页出现；total 是每一问当时的精确总数，他租的不算；末页没有 next。
func TestEvaluationPagesWalkWithoutRepeatOrGap(t *testing.T) {
	catalogue, pool := newEvaluationCatalogue(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "SYN-TENANT-01")
	seedEvaluations(t, pool)

	first, err := catalogue.ListEvaluations(ctx, tenant, 2, firstPage(t, ports.EvaluationPages))
	if err != nil {
		t.Fatalf("第一页：%v", err)
	}
	assertPage(t, "第一页", evaluationIDsOf(first.Rows), first.Total, first.Next,
		[]string{"SYN-EVAL-005", "SYN-EVAL-004"}, 5, true)

	insertEvaluationRow(t, pool, ctx, "SYN-EVAL-006", "SYN-TENANT-01", "PENDING", pageBase.Add(5*time.Hour))
	insertEvaluationRow(t, pool, ctx, "SYN-EVAL-000", "SYN-TENANT-01", "COMPLETED", pageBase.Add(3*time.Hour))

	second, err := catalogue.ListEvaluations(ctx, tenant, 2, nextPage(t, ports.EvaluationPages, url.Values{}, first.Next))
	if err != nil {
		t.Fatalf("第二页：%v", err)
	}
	assertPage(t, "第二页", evaluationIDsOf(second.Rows), second.Total, second.Next,
		[]string{"SYN-EVAL-003", "SYN-EVAL-000"}, 7, true)

	third, err := catalogue.ListEvaluations(ctx, tenant, 2, nextPage(t, ports.EvaluationPages, url.Values{}, second.Next))
	if err != nil {
		t.Fatalf("第三页：%v", err)
	}
	assertPage(t, "第三页", evaluationIDsOf(third.Rows), third.Total, third.Next,
		[]string{"SYN-EVAL-002", "SYN-EVAL-001"}, 7, false)
}

// Covers: ADR-0144 决定三、四——评价册的筛选、检索与换序都在库里做：status 维内取或、与 q 叠加取且，total 随条件变；q 去掉首尾
// 空白后不分大小写地按字面包含匹配评价号与两份摘要，% 与 _ 不当通配；正序时决胜键随之正序；空租户答空页、总数 0。
func TestEvaluationPagesFilterSearchAndSortInTheDatabase(t *testing.T) {
	catalogue, pool := newEvaluationCatalogue(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "SYN-TENANT-01")
	seedEvaluations(t, pool)

	list := func(label string, values url.Values, want ...string) {
		t.Helper()
		page, err := catalogue.ListEvaluations(ctx, tenant, 10, pageQuery(t, ports.EvaluationPages, values))
		if err != nil {
			t.Fatalf("%s：%v", label, err)
		}
		assertPage(t, label, evaluationIDsOf(page.Rows), page.Total, page.Next, append([]string{}, want...), int64(len(want)), false)
	}
	list("失败或冲突", url.Values{"status": {"FAILED", "CONFLICT"}}, "SYN-EVAL-003", "SYN-EVAL-002")
	list("按方案摘要检索", url.Values{"q": {"PLAN-syn-eval-004"}}, "SYN-EVAL-004")
	list("按语义摘要检索", url.Values{"q": {"semantic-SYN-EVAL-002"}}, "SYN-EVAL-002")
	list("按评价号检索", url.Values{"q": {" eval-00 "}},
		"SYN-EVAL-005", "SYN-EVAL-004", "SYN-EVAL-003", "SYN-EVAL-002", "SYN-EVAL-001")
	list("条件叠加", url.Values{"status": {"COMPLETED"}, "q": {"eval-001"}}, "SYN-EVAL-001")
	list("百分号按字面", url.Values{"q": {"%"}})
	list("下划线按字面", url.Values{"q": {"_"}})
	list("正序", url.Values{"sort": {"recordedAt"}},
		"SYN-EVAL-001", "SYN-EVAL-002", "SYN-EVAL-003", "SYN-EVAL-004", "SYN-EVAL-005")

	empty, err := catalogue.ListEvaluations(ctx, evaluationValue(t, domain.NewTenantID, "SYN-TENANT-03"), 10,
		firstPage(t, ports.EvaluationPages))
	if err != nil {
		t.Fatalf("空租户：%v", err)
	}
	assertPage(t, "空租户", evaluationIDsOf(empty.Rows), empty.Total, empty.Next, []string{}, 0, false)
}

func seedPriceCards(t *testing.T, fixture pagedPricing) {
	t.Helper()
	fixture.card(t, "SYN-TENANT-01", "SYN-PLAN-A", "v1", domain.PricingDirectionSell, 0)
	fixture.card(t, "SYN-TENANT-01", "SYN-PLAN-B", "v1", domain.PricingDirectionBuy, time.Hour)
	fixture.card(t, "SYN-TENANT-01", "SYN-PLAN-C", "v1", domain.PricingDirectionSell, 2*time.Hour)
	fixture.card(t, "SYN-TENANT-01", "SYN-PLAN-D", "v1", domain.PricingDirectionSell, 2*time.Hour)
	fixture.card(t, "SYN-TENANT-01", "SYN-PLAN-A", "v2", domain.PricingDirectionSell, 3*time.Hour)
	fixture.card(t, "SYN-TENANT-02", "SYN-PLAN-THEIRS", "v1", domain.PricingDirectionSell, 3*time.Hour)
}

// Covers: ADR-0144 决定一、三、五——价卡目录按登记时刻倒序、同刻按（方案、版本）倒序翻页；翻页途中新登的版本照游标前后各归其位，
// 不重不漏；他租的卡不进总数。
func TestPriceCardPagesWalkWithoutRepeatOrGap(t *testing.T) {
	fixture := newPagedPricing(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "SYN-TENANT-01")
	seedPriceCards(t, fixture)

	first, err := fixture.catalogue.ListPriceCards(ctx, tenant, 2, firstPage(t, ports.PriceCardPages))
	if err != nil {
		t.Fatalf("第一页：%v", err)
	}
	assertPage(t, "第一页", planKeysOf(first.Rows), first.Total, first.Next, []string{"SYN-PLAN-A@v2", "SYN-PLAN-D@v1"}, 5, true)

	fixture.card(t, "SYN-TENANT-01", "SYN-PLAN-E", "v1", domain.PricingDirectionSell, 4*time.Hour)
	fixture.card(t, "SYN-TENANT-01", "SYN-PLAN-C", "v2", domain.PricingDirectionSell, 2*time.Hour)

	second, err := fixture.catalogue.ListPriceCards(ctx, tenant, 2, nextPage(t, ports.PriceCardPages, url.Values{}, first.Next))
	if err != nil {
		t.Fatalf("第二页：%v", err)
	}
	assertPage(t, "第二页", planKeysOf(second.Rows), second.Total, second.Next, []string{"SYN-PLAN-C@v2", "SYN-PLAN-C@v1"}, 7, true)

	third, err := fixture.catalogue.ListPriceCards(ctx, tenant, 2, nextPage(t, ports.PriceCardPages, url.Values{}, second.Next))
	if err != nil {
		t.Fatalf("第三页：%v", err)
	}
	assertPage(t, "第三页", planKeysOf(third.Rows), third.Total, third.Next, []string{"SYN-PLAN-B@v1", "SYN-PLAN-A@v1"}, 7, false)
}

// Covers: ADR-0144 决定三、四与越权风险点 1——价卡目录按 planId 排时同一方案的各版挨在一起；direction 维、q（方案标识、版本与
// 适用范围，不分大小写、按字面）与 total 都在库里算；空租户答空页。
func TestPriceCardPagesFilterSearchAndSortInTheDatabase(t *testing.T) {
	fixture := newPagedPricing(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "SYN-TENANT-01")
	seedPriceCards(t, fixture)

	list := func(label string, values url.Values, want ...string) {
		t.Helper()
		page, err := fixture.catalogue.ListPriceCards(ctx, tenant, 10, pageQuery(t, ports.PriceCardPages, values))
		if err != nil {
			t.Fatalf("%s：%v", label, err)
		}
		assertPage(t, label, planKeysOf(page.Rows), page.Total, page.Next, append([]string{}, want...), int64(len(want)), false)
	}
	list("按方案正序", url.Values{"sort": {"planId"}}, "SYN-PLAN-A@v1", "SYN-PLAN-A@v2", "SYN-PLAN-B@v1", "SYN-PLAN-C@v1", "SYN-PLAN-D@v1")
	list("按方案倒序", url.Values{"sort": {"-planId"}}, "SYN-PLAN-D@v1", "SYN-PLAN-C@v1", "SYN-PLAN-B@v1", "SYN-PLAN-A@v2", "SYN-PLAN-A@v1")
	list("只看采购", url.Values{"direction": {"BUY"}}, "SYN-PLAN-B@v1")
	list("采购或销售", url.Values{"direction": {"SELL", "BUY"}},
		"SYN-PLAN-A@v2", "SYN-PLAN-D@v1", "SYN-PLAN-C@v1", "SYN-PLAN-B@v1", "SYN-PLAN-A@v1")
	list("按方案检索", url.Values{"q": {"plan-a"}}, "SYN-PLAN-A@v2", "SYN-PLAN-A@v1")
	list("按版本检索", url.Values{"q": {"V2"}}, "SYN-PLAN-A@v2")
	list("按适用范围检索", url.Values{"q": {"scope-1"}},
		"SYN-PLAN-A@v2", "SYN-PLAN-D@v1", "SYN-PLAN-C@v1", "SYN-PLAN-B@v1", "SYN-PLAN-A@v1")
	list("条件叠加", url.Values{"direction": {"SELL"}, "q": {"plan-c"}}, "SYN-PLAN-C@v1")
	list("百分号按字面", url.Values{"q": {"%"}})

	empty, err := fixture.catalogue.ListPriceCards(ctx, evaluationValue(t, domain.NewTenantID, "SYN-TENANT-03"), 10,
		firstPage(t, ports.PriceCardPages))
	if err != nil {
		t.Fatalf("空租户：%v", err)
	}
	assertPage(t, "空租户", planKeysOf(empty.Rows), empty.Total, empty.Next, []string{}, 0, false)
}

// Covers: ADR-0144 决定一、三、五——序列册按登记时刻倒序、同刻按（序列、版本）倒序翻页。翻页途中新登两版：比游标新的那版不再
// 出现，与游标同刻而标识排在其后的那版在下一页出现。复核事实只为本页各版去复核册里取，落在后面几页的版本复核计数与最近一次
// 复核照样完整，期次与引用 digest 照样转写。
func TestReferenceSeriesPagesFoldReviewsOnEveryPage(t *testing.T) {
	fixture := newPagedPricing(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "SYN-TENANT-01")
	older := fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-01", "SYN-PRC-FUEL-A", "v1"), 0)
	newer := fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-01", "SYN-PRC-FUEL-A", "v2"), time.Hour)
	fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-01", "SYN-PRC-FUEL-AB", "v1"), 2*time.Hour)
	fixture.seriesVersion(t, exchangeRegistration(t, "SYN-TENANT-01", "SYN-PRC-USD-CNY", "v1"), 2*time.Hour)
	fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-02", "SYN-PRC-FUEL-A", "v1"), 3*time.Hour)
	recordReview(t, fixture.reviews, fixture.transactor, ctx,
		seriesReview(t, older, "SYN-PRC-SERIES-REVIEWER", reviewAtOne, domain.SeriesReviewReturned))
	recordReview(t, fixture.reviews, fixture.transactor, ctx,
		seriesReview(t, older, "SYN-PRC-SERIES-REVIEWER-2", reviewAtTwo, domain.SeriesReviewApproved))
	recordReview(t, fixture.reviews, fixture.transactor, ctx,
		seriesReview(t, newer, "SYN-PRC-SERIES-REVIEWER", reviewAtThree, domain.SeriesReviewApproved))

	first, err := fixture.catalogue.ListReferenceSeries(ctx, tenant, 2, firstPage(t, ports.ReferenceSeriesPages))
	if err != nil {
		t.Fatalf("第一页：%v", err)
	}
	assertPage(t, "第一页", seriesKeysOf(first.Rows), first.Total, first.Next,
		[]string{"SYN-PRC-USD-CNY@v1", "SYN-PRC-FUEL-AB@v1"}, 4, true)
	for _, row := range first.Rows {
		if row.ReviewCount != 0 || row.HasReview {
			t.Fatalf("没人复核过的 %s 报了复核：%+v", row.SeriesID, row)
		}
	}

	fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-01", "SYN-PRC-FUEL-Z", "v1"), 4*time.Hour)
	fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-01", "SYN-PRC-FUEL-AA", "v1"), 2*time.Hour)

	second, err := fixture.catalogue.ListReferenceSeries(ctx, tenant, 2,
		nextPage(t, ports.ReferenceSeriesPages, url.Values{}, first.Next))
	if err != nil {
		t.Fatalf("第二页：%v", err)
	}
	assertPage(t, "第二页", seriesKeysOf(second.Rows), second.Total, second.Next,
		[]string{"SYN-PRC-FUEL-AA@v1", "SYN-PRC-FUEL-A@v2"}, 6, true)

	third, err := fixture.catalogue.ListReferenceSeries(ctx, tenant, 2,
		nextPage(t, ports.ReferenceSeriesPages, url.Values{}, second.Next))
	if err != nil {
		t.Fatalf("第三页：%v", err)
	}
	assertPage(t, "第三页", seriesKeysOf(third.Rows), third.Total, third.Next, []string{"SYN-PRC-FUEL-A@v1"}, 6, false)

	inserted, latest, earliest := second.Rows[0], second.Rows[1], third.Rows[0]
	if inserted.ReviewCount != 0 || inserted.HasReview {
		t.Fatalf("没人复核过的 %s 报了复核：%+v", inserted.SeriesID, inserted)
	}
	if latest.ReviewCount != 1 || latest.ApprovedReviewCount != 1 || !latest.LastReviewedAt.Equal(reviewAtThree) {
		t.Fatalf("v2 的复核事实 = %d/%d/%v", latest.ReviewCount, latest.ApprovedReviewCount, latest.LastReviewedAt)
	}
	if earliest.ReviewCount != 2 || earliest.ApprovedReviewCount != 1 || !earliest.LastReviewedAt.Equal(reviewAtTwo) ||
		earliest.LastReviewDecision != domain.SeriesReviewApproved.String() {
		t.Fatalf("v1 的复核事实 = %d/%d/%v/%q", earliest.ReviewCount, earliest.ApprovedReviewCount,
			earliest.LastReviewedAt, earliest.LastReviewDecision)
	}
	if len(earliest.Periods) != 2 || earliest.ReferenceDigest != older.Reference().Digest() {
		t.Fatalf("v1 的期次或引用 digest 没转写：%+v", earliest)
	}
}

// Covers: ADR-0144 决定四——seriesId 维按标识精确取（管理台「只看这一条序列」），q 按字面包含、会把同前缀的兄弟序列一并带进来；
// kind 维、q 与 total 随条件变；按 seriesId 排时同一序列的各版挨在一起；空租户答空页。
func TestReferenceSeriesPagesFilterSearchAndSortInTheDatabase(t *testing.T) {
	fixture := newPagedPricing(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "SYN-TENANT-01")
	fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-01", "SYN-PRC-FUEL-A", "v1"), 0)
	fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-01", "SYN-PRC-FUEL-A", "v2"), time.Hour)
	fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-01", "SYN-PRC-FUEL-AB", "v1"), 2*time.Hour)
	fixture.seriesVersion(t, exchangeRegistration(t, "SYN-TENANT-01", "SYN-PRC-USD-CNY", "v1"), 2*time.Hour)
	fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-02", "SYN-PRC-FUEL-A", "v1"), 3*time.Hour)

	list := func(label string, values url.Values, want ...string) {
		t.Helper()
		page, err := fixture.catalogue.ListReferenceSeries(ctx, tenant, 10, pageQuery(t, ports.ReferenceSeriesPages, values))
		if err != nil {
			t.Fatalf("%s：%v", label, err)
		}
		assertPage(t, label, seriesKeysOf(page.Rows), page.Total, page.Next, append([]string{}, want...), int64(len(want)), false)
	}
	list("只看这一条序列", url.Values{"seriesId": {"SYN-PRC-FUEL-A"}}, "SYN-PRC-FUEL-A@v2", "SYN-PRC-FUEL-A@v1")
	list("检索同一标识", url.Values{"q": {"syn-prc-fuel-a"}}, "SYN-PRC-FUEL-AB@v1", "SYN-PRC-FUEL-A@v2", "SYN-PRC-FUEL-A@v1")
	list("只看汇率", url.Values{"kind": {"EXCHANGE_RATE"}}, "SYN-PRC-USD-CNY@v1")
	list("按来源检索", url.Values{"q": {"usd-cny-DAILY"}}, "SYN-PRC-USD-CNY@v1")
	list("按登记方检索", url.Values{"q": {"series-registrar"}},
		"SYN-PRC-USD-CNY@v1", "SYN-PRC-FUEL-AB@v1", "SYN-PRC-FUEL-A@v2", "SYN-PRC-FUEL-A@v1")
	list("按序列正序", url.Values{"sort": {"seriesId"}},
		"SYN-PRC-FUEL-A@v1", "SYN-PRC-FUEL-A@v2", "SYN-PRC-FUEL-AB@v1", "SYN-PRC-USD-CNY@v1")
	list("条件叠加", url.Values{"kind": {"FUEL_RATE"}, "seriesId": {"SYN-PRC-FUEL-AB", "SYN-PRC-USD-CNY"}}, "SYN-PRC-FUEL-AB@v1")
	list("百分号按字面", url.Values{"q": {"%"}})

	empty, err := fixture.catalogue.ListReferenceSeries(ctx, evaluationValue(t, domain.NewTenantID, "SYN-TENANT-03"), 10,
		firstPage(t, ports.ReferenceSeriesPages))
	if err != nil {
		t.Fatalf("空租户：%v", err)
	}
	assertPage(t, "空租户", seriesKeysOf(empty.Rows), empty.Total, empty.Next, []string{}, 0, false)
}

// Covers: ADR-0144 决定一、三、五——覆盖册一条序列（标识 + 种类）一行，按序列标识升序翻页；翻页途中新登的序列照游标前后各归其位；
// 每页只为本页各序列取明细，版本数与在用版本照样完整；total 数的是序列不是版本。
func TestCoveragePagesWalkSeriesInIdentifierOrder(t *testing.T) {
	fixture := newPagedPricing(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "SYN-TENANT-01")
	fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-01", "SYN-PRC-FUEL-B", "v1"), 0)
	approved := fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-01", "SYN-PRC-FUEL-C", "v1"), 0)
	fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-01", "SYN-PRC-FUEL-C", "v2"), time.Hour)
	fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-01", "SYN-PRC-FUEL-D", "v1"), 0)
	fixture.seriesVersion(t, exchangeRegistration(t, "SYN-TENANT-01", "SYN-PRC-USD-CNY", "v1"), 0)
	fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-02", "SYN-PRC-FUEL-THEIRS", "v1"), 0)
	recordReview(t, fixture.reviews, fixture.transactor, ctx,
		seriesReview(t, approved, "SYN-PRC-SERIES-REVIEWER", reviewAtOne, domain.SeriesReviewApproved))

	first, err := fixture.coverage.ListReferenceSeriesCoverage(ctx, tenant, reviewAtTwo, 2,
		firstPage(t, ports.ReferenceSeriesCoveragePages))
	if err != nil {
		t.Fatalf("第一页：%v", err)
	}
	assertPage(t, "第一页", coverageKeysOf(first.Rows), first.Total, first.Next,
		[]string{"SYN-PRC-FUEL-B/FUEL_RATE", "SYN-PRC-FUEL-C/FUEL_RATE"}, 4, true)
	if row := first.Rows[1]; row.RegisteredVersionCount != 2 || row.UnreviewedVersionCount != 1 ||
		!row.HasInForceVersion || row.InForceVersion != "v1" {
		t.Fatalf("FUEL-C 的明细不完整：%+v", row)
	}

	fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-01", "SYN-PRC-FUEL-A", "v1"), 0)
	fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-01", "SYN-PRC-FUEL-E", "v1"), 0)

	second, err := fixture.coverage.ListReferenceSeriesCoverage(ctx, tenant, reviewAtTwo, 2,
		nextPage(t, ports.ReferenceSeriesCoveragePages, url.Values{}, first.Next))
	if err != nil {
		t.Fatalf("第二页：%v", err)
	}
	assertPage(t, "第二页", coverageKeysOf(second.Rows), second.Total, second.Next,
		[]string{"SYN-PRC-FUEL-D/FUEL_RATE", "SYN-PRC-FUEL-E/FUEL_RATE"}, 6, true)

	third, err := fixture.coverage.ListReferenceSeriesCoverage(ctx, tenant, reviewAtTwo, 2,
		nextPage(t, ports.ReferenceSeriesCoveragePages, url.Values{}, second.Next))
	if err != nil {
		t.Fatalf("第三页：%v", err)
	}
	assertPage(t, "第三页", coverageKeysOf(third.Rows), third.Total, third.Next,
		[]string{"SYN-PRC-USD-CNY/EXCHANGE_RATE"}, 6, false)
}

// Covers: ADR-0144 决定三、四——同一标识登成两个种类各是一行，页界切在两行之间也不重不漏（决胜键是种类）；kind 维、q（序列
// 标识，不分大小写、按字面）与倒序都在库里做，total 随条件变；空租户答空页。
func TestCoveragePagesFilterSearchAndSortInTheDatabase(t *testing.T) {
	fixture := newPagedPricing(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "SYN-TENANT-01")
	fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-01", "SYN-PRC-FUEL-B", "v1"), 0)
	fixture.seriesVersion(t, fuelRegistration(t, "SYN-TENANT-01", "SYN-PRC-FUEL-C", "v1"), 0)
	fixture.seriesVersion(t, exchangeRegistration(t, "SYN-TENANT-01", "SYN-PRC-FUEL-C", "v2"), 0)
	fixture.seriesVersion(t, exchangeRegistration(t, "SYN-TENANT-01", "SYN-PRC-USD-CNY", "v1"), 0)

	walked := []string{}
	values := url.Values{}
	query := firstPage(t, ports.ReferenceSeriesCoveragePages)
	for {
		page, err := fixture.coverage.ListReferenceSeriesCoverage(ctx, tenant, reviewAtTwo, 1, query)
		if err != nil {
			t.Fatalf("逐行翻：%v", err)
		}
		if len(page.Rows) != 1 || page.Rows[0].RegisteredVersionCount != 1 || page.Total != 4 {
			t.Fatalf("逐行翻的这一页不对：%+v（总数 %d）", page.Rows, page.Total)
		}
		walked = append(walked, coverageKeysOf(page.Rows)...)
		if page.Next == "" {
			break
		}
		query = nextPage(t, ports.ReferenceSeriesCoveragePages, values, page.Next)
	}
	if want := []string{
		"SYN-PRC-FUEL-B/FUEL_RATE", "SYN-PRC-FUEL-C/EXCHANGE_RATE", "SYN-PRC-FUEL-C/FUEL_RATE", "SYN-PRC-USD-CNY/EXCHANGE_RATE",
	}; !reflect.DeepEqual(walked, want) {
		t.Fatalf("逐行翻出 %v，想要 %v", walked, want)
	}

	list := func(label string, values url.Values, want ...string) {
		t.Helper()
		page, err := fixture.coverage.ListReferenceSeriesCoverage(ctx, tenant, reviewAtTwo, 10,
			pageQuery(t, ports.ReferenceSeriesCoveragePages, values))
		if err != nil {
			t.Fatalf("%s：%v", label, err)
		}
		assertPage(t, label, coverageKeysOf(page.Rows), page.Total, page.Next, append([]string{}, want...), int64(len(want)), false)
	}
	list("只看汇率", url.Values{"kind": {"EXCHANGE_RATE"}}, "SYN-PRC-FUEL-C/EXCHANGE_RATE", "SYN-PRC-USD-CNY/EXCHANGE_RATE")
	list("按标识检索", url.Values{"q": {"fuel-c"}}, "SYN-PRC-FUEL-C/EXCHANGE_RATE", "SYN-PRC-FUEL-C/FUEL_RATE")
	list("倒序", url.Values{"sort": {"-seriesId"}},
		"SYN-PRC-USD-CNY/EXCHANGE_RATE", "SYN-PRC-FUEL-C/FUEL_RATE", "SYN-PRC-FUEL-C/EXCHANGE_RATE", "SYN-PRC-FUEL-B/FUEL_RATE")
	list("百分号按字面", url.Values{"q": {"%"}})

	empty, err := fixture.coverage.ListReferenceSeriesCoverage(ctx, evaluationValue(t, domain.NewTenantID, "SYN-TENANT-03"),
		reviewAtTwo, 10, firstPage(t, ports.ReferenceSeriesCoveragePages))
	if err != nil {
		t.Fatalf("空租户：%v", err)
	}
	assertPage(t, "空租户", coverageKeysOf(empty.Rows), empty.Total, empty.Next, []string{}, 0, false)
}

// seedDrafts 录四版草稿：两版同刻，一版带问题（`草稿`），其余已校验；另录一版他租的。
func seedDrafts(t *testing.T, store draftStore) {
	t.Helper()
	submitDraft(t, store, "SYN-TENANT-01", "SYN-PLAN-DRAFT-01", "SYN-CARD-ALPHA.xlsx", draftAt)
	submitDraft(t, store, "SYN-TENANT-01", "SYN-PLAN-DRAFT-02", "SYN-CARD-BRAVO.xlsx", draftAt.Add(time.Hour))
	if outcome := store.submit(t, problemDraftOf(t, "SYN-TENANT-01", "SYN-PLAN-DRAFT-03",
		draftSourceOf(t, "SYN-CARD-CHARLIE.xlsx", catalogSourceSHA), draftAt.Add(time.Hour),
		"https://id.syn.example/dex#SYN-OPERATOR-01")); outcome != ports.PriceCardDraftSaved {
		t.Fatalf("录入带问题的草稿 = %v", outcome)
	}
	submitDraft(t, store, "SYN-TENANT-01", "SYN-PLAN-DRAFT-04", "SYN-CARD-DELTA.xlsx", draftAt.Add(3*time.Hour))
	submitDraft(t, store, "SYN-TENANT-02", "SYN-PLAN-DRAFT-THEIRS", "SYN-CARD-ALPHA.xlsx", draftAt.Add(5*time.Hour))
}

func submitDraft(t *testing.T, store draftStore, tenant, planID, sourceName string, at time.Time) {
	t.Helper()
	plan := catalogPlan(t, planID, "v1", domain.PricingDirectionBuy, "30")
	draft := validatedDraftOf(t, tenant, plan, draftSourceOf(t, sourceName, catalogSourceSHA), at)
	if outcome := store.submit(t, draft); outcome != ports.PriceCardDraftSaved {
		t.Fatalf("录入 %s = %v", planID, outcome)
	}
}

func listDrafts(
	t *testing.T,
	store draftStore,
	tenant string,
	limit int,
	query cataloguepage.Query,
) cataloguepage.Result[domain.PriceCardDraft] {
	t.Helper()
	page, err := store.drafts.ListPriceCardDrafts(t.Context(), evaluationValue(t, domain.NewTenantID, tenant), limit, query)
	if err != nil {
		t.Fatalf("列草稿：%v", err)
	}
	return page
}

// Covers: ADR-0144 决定一、三、五——草稿册按录入时刻倒序、同刻按（方案、版本）倒序翻页；翻页途中新录的草稿照游标前后各归其位，
// 不重不漏；他租的不进总数。
func TestPriceCardDraftPagesWalkWithoutRepeatOrGap(t *testing.T) {
	store := newDraftStore(t)
	seedDrafts(t, store)

	first := listDrafts(t, store, "SYN-TENANT-01", 2, firstPage(t, ports.PriceCardDraftPages))
	assertPage(t, "第一页", planIDsOf(first.Rows), first.Total, first.Next,
		[]string{"SYN-PLAN-DRAFT-04", "SYN-PLAN-DRAFT-03"}, 4, true)

	submitDraft(t, store, "SYN-TENANT-01", "SYN-PLAN-DRAFT-05", "SYN-CARD-ECHO.xlsx", draftAt.Add(2*time.Hour))
	submitDraft(t, store, "SYN-TENANT-01", "SYN-PLAN-DRAFT-00", "SYN-CARD-ZULU.xlsx", draftAt.Add(-time.Hour))

	second := listDrafts(t, store, "SYN-TENANT-01", 2, nextPage(t, ports.PriceCardDraftPages, url.Values{}, first.Next))
	assertPage(t, "第二页", planIDsOf(second.Rows), second.Total, second.Next,
		[]string{"SYN-PLAN-DRAFT-02", "SYN-PLAN-DRAFT-01"}, 6, true)

	third := listDrafts(t, store, "SYN-TENANT-01", 2, nextPage(t, ports.PriceCardDraftPages, url.Values{}, second.Next))
	assertPage(t, "第三页", planIDsOf(third.Rows), third.Total, third.Next, []string{"SYN-PLAN-DRAFT-00"}, 6, false)
}

// Covers: ADR-0144 决定三、四、六——草稿册的 status 维从 Intake 挪进册声明后可重复（维内取或）；q 不分大小写、按字面匹配方案标识、
// 版本与源文件名；正序时决胜键随之正序；total 随条件变；空租户答空页。
func TestPriceCardDraftPagesFilterSearchAndSortInTheDatabase(t *testing.T) {
	store := newDraftStore(t)
	seedDrafts(t, store)
	store.advance(t, "SYN-TENANT-01", "SYN-PLAN-DRAFT-01", domain.PriceCardDraftStatusApproved)

	list := func(label string, values url.Values, want ...string) {
		t.Helper()
		page := listDrafts(t, store, "SYN-TENANT-01", 10, pageQuery(t, ports.PriceCardDraftPages, values))
		assertPage(t, label, planIDsOf(page.Rows), page.Total, page.Next, append([]string{}, want...), int64(len(want)), false)
	}
	list("待改或已批准", url.Values{"status": {"DRAFT", "APPROVED"}}, "SYN-PLAN-DRAFT-03", "SYN-PLAN-DRAFT-01")
	list("只看已校验", url.Values{"status": {"VALIDATED"}}, "SYN-PLAN-DRAFT-04", "SYN-PLAN-DRAFT-02")
	list("按源文件名检索", url.Values{"q": {"card-bravo"}}, "SYN-PLAN-DRAFT-02")
	list("按方案检索", url.Values{"q": {"plan-draft-0"}},
		"SYN-PLAN-DRAFT-04", "SYN-PLAN-DRAFT-03", "SYN-PLAN-DRAFT-02", "SYN-PLAN-DRAFT-01")
	list("条件叠加", url.Values{"status": {"VALIDATED", "APPROVED"}, "q": {"alpha"}}, "SYN-PLAN-DRAFT-01")
	list("正序", url.Values{"sort": {"submittedAt"}},
		"SYN-PLAN-DRAFT-01", "SYN-PLAN-DRAFT-02", "SYN-PLAN-DRAFT-03", "SYN-PLAN-DRAFT-04")
	list("百分号按字面", url.Values{"q": {"%"}})

	empty := listDrafts(t, store, "SYN-TENANT-03", 10, firstPage(t, ports.PriceCardDraftPages))
	assertPage(t, "空租户", planIDsOf(empty.Rows), empty.Total, empty.Next, []string{}, 0, false)
}
