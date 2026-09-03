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

// 本文件对真实 PostgreSQL 16 证覆盖地平线读口（票 pricing-reference-series-operations/05
// 第 1 项）：在用版本按给定时刻从复核派生而不是从登记派生、末期无上界与有上界分得开、
// 「等人复核」与「等登记方更正」两笔欠账分开计数、租户之间互不可见、limit 数的是序列条数。
// 夹具全部为 SYN 合成序列（S 级），不影射任何真实序列。

func newSeriesCoverage(t *testing.T) (*adapter.ReferenceSeriesVersions, *adapter.ReferenceSeriesReviews, *adapter.ReferenceSeriesCoverage, bentoapp.Transactor) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	versions, err := adapter.NewReferenceSeriesVersions(db)
	if err != nil {
		t.Fatalf("构造序列登记册：%v", err)
	}
	reviews, err := adapter.NewReferenceSeriesReviews(db)
	if err != nil {
		t.Fatalf("构造复核册：%v", err)
	}
	coverage, err := adapter.NewReferenceSeriesCoverage(db)
	if err != nil {
		t.Fatalf("构造覆盖读口：%v", err)
	}
	return versions, reviews, coverage, db.Transactor()
}

// coverageOf 取指定序列那一行。找不到就让用例响亮失败——「答了空列表」与「答了别的序列」
// 是两回事，用下标取会把后者读成前者。
func coverageOf(t *testing.T, rows []ports.ReferenceSeriesCoverageRow, seriesID string) ports.ReferenceSeriesCoverageRow {
	t.Helper()
	for _, row := range rows {
		if row.SeriesID == seriesID {
			return row
		}
	}
	t.Fatalf("覆盖列面里没有 %s：%+v", seriesID, rows)
	return ports.ReferenceSeriesCoverageRow{}
}

// TestCoverageAnswersAnEmptyListForATenantWithNothingRegistered 证空册如实答空列表：
// 没有序列是一个正常答案，不是故障，也不是「未配置」（ADR-0077 Decision 四）。
func TestCoverageAnswersAnEmptyListForATenantWithNothingRegistered(t *testing.T) {
	_, _, coverage, _ := newSeriesCoverage(t)
	ctx := t.Context()

	rows, err := coverage.ListReferenceSeriesCoverage(
		ctx, evaluationValue(t, domain.NewTenantID, "tenant-empty"), reviewAtTwo, 10)
	if err != nil {
		t.Fatalf("空册报错：%v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("空册答了 %d 行：%+v", len(rows), rows)
	}
}

// TestCoverageReportsTheInForceVersionAndItsHorizon 证覆盖摘要答的是「哪一版在用、它盖到
// 什么时候」：在用从复核派生（登记了但没复核的那一版不算），末期止点照实转写，未复核的
// 那一版记在欠账里。
func TestCoverageReportsTheInForceVersionAndItsHorizon(t *testing.T) {
	versions, reviews, coverage, transactor := newSeriesCoverage(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	first := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-HORIZON", "v1")
	second := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-HORIZON", "v2")
	registerSeries(t, versions, transactor, ctx, first)
	registerSeries(t, versions, transactor, ctx, second)
	recordReview(t, reviews, transactor, ctx,
		seriesReview(t, first, "SYN-PRC-SERIES-REVIEWER", reviewAtOne, domain.SeriesReviewApproved))

	rows, err := coverage.ListReferenceSeriesCoverage(ctx, tenant, reviewAtTwo, 10)
	if err != nil {
		t.Fatalf("取覆盖列面：%v", err)
	}
	row := coverageOf(t, rows, "SYN-PRC-FUEL-HORIZON")

	if row.Kind != domain.ReferenceSeriesFuelRate.String() {
		t.Fatalf("种类转写变形：%q", row.Kind)
	}
	if !row.HasInForceVersion || row.InForceVersion != "v1" {
		t.Fatalf("在用版本 = %q/%v，想要 v1——v2 还没复核", row.InForceVersion, row.HasInForceVersion)
	}
	if !row.HasInForceEffectiveTo || !row.InForceEffectiveTo.Equal(registerWeekThree) {
		t.Fatalf("末期止点 = %v/%v，想要 %v", row.InForceEffectiveTo, row.HasInForceEffectiveTo, registerWeekThree)
	}
	if row.RegisteredVersionCount != 2 {
		t.Fatalf("登记版本数 = %d，想要 2", row.RegisteredVersionCount)
	}
	if row.UnreviewedVersionCount != 1 {
		t.Fatalf("未复核版本数 = %d，想要 1（v2）", row.UnreviewedVersionCount)
	}
	if !row.HasReview || !row.LastReviewedAt.Equal(reviewAtOne) || row.LastReviewDecision != domain.SeriesReviewApproved.String() {
		t.Fatalf("最近一次复核 = %v/%q/%v", row.LastReviewedAt, row.LastReviewDecision, row.HasReview)
	}
}

// TestCoverageTellsAnOpenEndedVersionFromABoundedOne 证末期无上界不被折成一个时刻：
// 那一版没有终点，与「终点恰好是零时刻」是两回事，显式布尔让两者分得开。
func TestCoverageTellsAnOpenEndedVersionFromABoundedOne(t *testing.T) {
	versions, reviews, coverage, transactor := newSeriesCoverage(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	openEnded := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-OPEN", "v1",
		registerPeriod(t, registerWeekOne, registerWeekTwo, "0.22", "SYN-EVIDENCE/fuel-2026-W32"),
		registerPeriod(t, registerWeekTwo, time.Time{}, "0.24", "SYN-EVIDENCE/fuel-2026-W33"))
	registerSeries(t, versions, transactor, ctx, openEnded)
	recordReview(t, reviews, transactor, ctx,
		seriesReview(t, openEnded, "SYN-PRC-SERIES-REVIEWER", reviewAtOne, domain.SeriesReviewApproved))

	rows, err := coverage.ListReferenceSeriesCoverage(ctx, tenant, reviewAtTwo, 10)
	if err != nil {
		t.Fatalf("取覆盖列面：%v", err)
	}
	row := coverageOf(t, rows, "SYN-PRC-FUEL-OPEN")
	if !row.HasInForceVersion {
		t.Fatal("无上界那一版没被选为在用")
	}
	if row.HasInForceEffectiveTo {
		t.Fatalf("无上界却报了止点 %v", row.InForceEffectiveTo)
	}
	if !row.InForceEffectiveFrom.Equal(registerWeekOne) {
		t.Fatalf("起点 = %v，想要 %v", row.InForceEffectiveFrom, registerWeekOne)
	}
}

// TestCoverageSeparatesWaitingForAReviewerFromWaitingForTheRegistrant 证两笔欠账不折成
// 一格：一条复核都没有的版本等的是复核人，复核过但至今未通过的等的是登记方去更正。
// 折成一个「未通过数」会让两种续办动作在页面上长同一张脸。
func TestCoverageSeparatesWaitingForAReviewerFromWaitingForTheRegistrant(t *testing.T) {
	versions, reviews, coverage, transactor := newSeriesCoverage(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	approved := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-DEBTS", "v1")
	returned := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-DEBTS", "v2")
	untouched := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-DEBTS", "v3")
	registerSeries(t, versions, transactor, ctx, approved)
	registerSeries(t, versions, transactor, ctx, returned)
	registerSeries(t, versions, transactor, ctx, untouched)
	recordReview(t, reviews, transactor, ctx,
		seriesReview(t, approved, "SYN-PRC-SERIES-REVIEWER", reviewAtOne, domain.SeriesReviewApproved))
	recordReview(t, reviews, transactor, ctx,
		seriesReview(t, returned, "SYN-PRC-SERIES-REVIEWER", reviewAtTwo, domain.SeriesReviewReturned))

	rows, err := coverage.ListReferenceSeriesCoverage(ctx, tenant, reviewAtThree, 10)
	if err != nil {
		t.Fatalf("取覆盖列面：%v", err)
	}
	row := coverageOf(t, rows, "SYN-PRC-FUEL-DEBTS")
	if row.UnreviewedVersionCount != 1 {
		t.Fatalf("未复核版本数 = %d，想要 1（v3 一条复核都没有）", row.UnreviewedVersionCount)
	}
	if row.ReturnedVersionCount != 1 {
		t.Fatalf("退回未通过版本数 = %d，想要 1（v2 复核过但没通过）", row.ReturnedVersionCount)
	}
	if !row.HasInForceVersion || row.InForceVersion != "v1" {
		t.Fatalf("在用 = %q，想要 v1——退回不进在用", row.InForceVersion)
	}
	if !row.LastReviewedAt.Equal(reviewAtTwo) || row.LastReviewDecision != domain.SeriesReviewReturned.String() {
		t.Fatalf("最近一次复核 = %v/%q，想要那条退回——它也是「有人看过」", row.LastReviewedAt, row.LastReviewDecision)
	}
}

// TestCoverageResolvesInForceAtTheGivenMoment 证在用是按传入时刻派生的结论而不是一个
// 状态列：同一份数据，问复核之前得「无在用」，问复核之后得那一版；两次的登记与欠账计数
// 一模一样——变的只有在用那一格。
func TestCoverageResolvesInForceAtTheGivenMoment(t *testing.T) {
	versions, reviews, coverage, transactor := newSeriesCoverage(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	registration := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-MOMENT", "v1")
	registerSeries(t, versions, transactor, ctx, registration)
	recordReview(t, reviews, transactor, ctx,
		seriesReview(t, registration, "SYN-PRC-SERIES-REVIEWER", reviewAtTwo, domain.SeriesReviewApproved))

	before, err := coverage.ListReferenceSeriesCoverage(ctx, tenant, reviewAtOne, 10)
	if err != nil {
		t.Fatalf("取覆盖列面（复核前）：%v", err)
	}
	earlier := coverageOf(t, before, "SYN-PRC-FUEL-MOMENT")
	if earlier.HasInForceVersion {
		t.Fatalf("复核时刻之前就在用了：%q", earlier.InForceVersion)
	}
	if earlier.RegisteredVersionCount != 1 || earlier.UnreviewedVersionCount != 0 {
		t.Fatalf("复核前的计数变形：登记 %d，未复核 %d", earlier.RegisteredVersionCount, earlier.UnreviewedVersionCount)
	}

	after, err := coverage.ListReferenceSeriesCoverage(ctx, tenant, reviewAtThree, 10)
	if err != nil {
		t.Fatalf("取覆盖列面（复核后）：%v", err)
	}
	later := coverageOf(t, after, "SYN-PRC-FUEL-MOMENT")
	if !later.HasInForceVersion || later.InForceVersion != "v1" {
		t.Fatalf("复核之后应在用 v1：%q/%v", later.InForceVersion, later.HasInForceVersion)
	}
	if later.RegisteredVersionCount != earlier.RegisteredVersionCount ||
		later.UnreviewedVersionCount != earlier.UnreviewedVersionCount {
		t.Fatal("换一个时刻问，登记与欠账计数不该跟着变")
	}
}

// TestCoverageIsScopedToTheTenant 证作用域是身份的一部分（ADR-0003）：另一个租户登记的
// 同名序列不出现在本租户的覆盖列面里。
func TestCoverageIsScopedToTheTenant(t *testing.T) {
	versions, reviews, coverage, transactor := newSeriesCoverage(t)
	ctx := t.Context()

	mine := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-SCOPED", "v1")
	theirs := fuelRegistration(t, "tenant-b", "SYN-PRC-FUEL-SCOPED", "v1")
	registerSeries(t, versions, transactor, ctx, mine)
	registerSeries(t, versions, transactor, ctx, theirs)
	recordReview(t, reviews, transactor, ctx,
		seriesReview(t, theirs, "SYN-PRC-SERIES-REVIEWER", reviewAtOne, domain.SeriesReviewApproved))

	rows, err := coverage.ListReferenceSeriesCoverage(
		ctx, evaluationValue(t, domain.NewTenantID, "tenant-a"), reviewAtTwo, 10)
	if err != nil {
		t.Fatalf("取覆盖列面：%v", err)
	}
	row := coverageOf(t, rows, "SYN-PRC-FUEL-SCOPED")
	if row.HasInForceVersion {
		t.Fatalf("看见了别的租户的复核：在用 %q", row.InForceVersion)
	}
	if row.UnreviewedVersionCount != 1 {
		t.Fatalf("未复核版本数 = %d，想要 1——本租户那一版确实没人复核过", row.UnreviewedVersionCount)
	}
}

// TestCoverageLimitCountsSeriesNotVersions 证 limit 数的是序列条数：一条多版序列不该把
// 整页占满，调用方问的是「我有几条序列、各自还剩多少」。
func TestCoverageLimitCountsSeriesNotVersions(t *testing.T) {
	versions, _, coverage, transactor := newSeriesCoverage(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	for _, version := range []string{"v1", "v2", "v3"} {
		registerSeries(t, versions, transactor, ctx,
			fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-PAGE-A", version))
	}
	registerSeries(t, versions, transactor, ctx,
		fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-PAGE-B", "v1"))

	rows, err := coverage.ListReferenceSeriesCoverage(ctx, tenant, reviewAtTwo, 1)
	if err != nil {
		t.Fatalf("取覆盖列面：%v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("limit=1 交回 %d 行：%+v", len(rows), rows)
	}
	if rows[0].RegisteredVersionCount != 3 {
		t.Fatalf("那一条序列的版本数 = %d，想要 3——limit 切的是序列不是版本", rows[0].RegisteredVersionCount)
	}
}

// TestCoverageRefusesAMissingMomentOrPageSize 证两个缺参被挡在读口上而不是静默答一个
// 没人决定过的取值：零时刻在领域里恒答「没有在用版本」，放它进来会让每一行都显示未在用，
// 而那是调用方忘了传时刻，不是登记册的事实。
func TestCoverageRefusesAMissingMomentOrPageSize(t *testing.T) {
	_, _, coverage, _ := newSeriesCoverage(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	if _, err := coverage.ListReferenceSeriesCoverage(ctx, tenant, time.Time{}, 10); err == nil {
		t.Fatal("零时刻被接受")
	}
	if _, err := coverage.ListReferenceSeriesCoverage(ctx, tenant, reviewAtTwo, 0); err == nil {
		t.Fatal("零页大小被接受")
	}
}
