package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证序列版本复核与在用解析（ADR-0099 决定二、三；票
// pricing-reference-series-operations/02）：复核行只增不改、同键重复按内容译成幂等或冲突、
// 复核不在册版本被外键拦成一格答案；在用版本 = 评价形成时刻之前复核通过的最新版本，退回与
// 未来的复核不算，跨租户不可见，种类不合单独一格。夹具全部为 SYN 合成序列（S 级）。

var (
	reviewAtOne   = time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	reviewAtTwo   = time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)
	reviewAtThree = time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC)
)

type reviewFixture struct {
	versions   *adapter.ReferenceSeriesVersions
	reviews    *adapter.ReferenceSeriesReviews
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newReviewRegister(t *testing.T) (*adapter.ReferenceSeriesVersions, *adapter.ReferenceSeriesReviews, bentoapp.Transactor) {
	t.Helper()
	fixture := newReviewFixture(t)
	return fixture.versions, fixture.reviews, fixture.transactor
}

func newReviewFixture(t *testing.T) reviewFixture {
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
	return reviewFixture{versions: versions, reviews: reviews, transactor: db.Transactor(), pool: pool}
}

func seriesReview(t *testing.T, registration domain.ReferenceSeriesRegistration, reviewer string, at time.Time, decision domain.SeriesReviewDecision) domain.SeriesReview {
	t.Helper()
	review, err := domain.NewSeriesReview(registration, reviewer, at, decision, "SYN-REVIEW/逐期核对公布记录")
	if err != nil {
		t.Fatalf("构造复核：%v", err)
	}
	return review
}

func recordReview(t *testing.T, reviews *adapter.ReferenceSeriesReviews, transactor bentoapp.Transactor, ctx context.Context, review domain.SeriesReview) ports.ReferenceSeriesReviewOutcome {
	t.Helper()
	var outcome ports.ReferenceSeriesReviewOutcome
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		saved, err := reviews.Record(txCtx, review)
		outcome = saved
		return err
	}); err != nil {
		t.Fatalf("事务内记录复核失败：%v", err)
	}
	return outcome
}

// TestInForceFollowsApprovalNotRegistration 证在用版本从复核派生：登了没复核答无通过版本；
// 复核通过后自复核时刻起在用；复核时刻之前问，仍然没有；解析出的引用能原样交给 ResolveAt。
func TestInForceFollowsApprovalNotRegistration(t *testing.T) {
	versions, reviews, transactor := newReviewRegister(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	registration := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-INFORCE", "v1")
	registerSeries(t, versions, transactor, ctx, registration)

	if _, outcome, err := reviews.ResolveInForce(ctx, tenant, domain.ReferenceSeriesFuelRate, "SYN-PRC-FUEL-INFORCE", reviewAtTwo); err != nil || outcome != ports.SeriesHasNoApprovedVersion {
		t.Fatalf("未复核就在用了：outcome=%d err=%v", outcome, err)
	}

	if outcome := recordReview(t, reviews, transactor, ctx, seriesReview(t, registration, "SYN-PRC-SERIES-REVIEWER", reviewAtOne, domain.SeriesReviewApproved)); outcome != ports.ReferenceSeriesReviewRecorded {
		t.Fatalf("复核 outcome = %d，想要 Recorded", outcome)
	}

	reference, outcome, err := reviews.ResolveInForce(ctx, tenant, domain.ReferenceSeriesFuelRate, "SYN-PRC-FUEL-INFORCE", reviewAtTwo)
	if err != nil || outcome != ports.SeriesVersionInForce || reference.Version() != "v1" {
		t.Fatalf("复核通过后应在用 v1：outcome=%d ref=%v err=%v", outcome, reference, err)
	}
	if reference != registration.Reference() {
		t.Fatalf("解析出的引用与登记引用不同：%v vs %v", reference, registration.Reference())
	}
	if _, outcome, err := reviews.ResolveInForce(ctx, tenant, domain.ReferenceSeriesFuelRate, "SYN-PRC-FUEL-INFORCE", reviewAtOne.Add(-time.Minute)); err != nil || outcome != ports.SeriesHasNoApprovedVersion {
		t.Fatalf("复核时刻之前不该在用：outcome=%d err=%v", outcome, err)
	}

	reading, found, err := versions.ResolveAt(ctx, tenant, reference, registerWeekOne.Add(time.Hour))
	if err != nil || !found || reading.Value().Value().String() != "0.22" {
		t.Fatalf("在用引用交给 ResolveAt 解析失败：found=%v err=%v", found, err)
	}
}

// TestInForcePicksTheLatestApprovedBeforeTheMoment 证两版先后通过时按评价形成时刻取版：
// 在 v2 通过之前问得 v1，之后问得 v2；既有评价冻结的是各自那一刻的答案。
func TestInForcePicksTheLatestApprovedBeforeTheMoment(t *testing.T) {
	versions, reviews, transactor := newReviewRegister(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	first := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-TWO", "v1")
	second := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-TWO", "v2",
		registerPeriod(t, registerWeekOne, registerWeekTwo, "0.22", "SYN-EVIDENCE/fuel-2026-W32"),
		registerPeriod(t, registerWeekTwo, registerWeekThree, "0.24", "SYN-EVIDENCE/fuel-2026-W33"),
		registerPeriod(t, registerWeekThree, time.Time{}, "0.25", "SYN-EVIDENCE/fuel-2026-W34"))
	registerSeries(t, versions, transactor, ctx, first)
	registerSeries(t, versions, transactor, ctx, second)
	recordReview(t, reviews, transactor, ctx, seriesReview(t, first, "SYN-PRC-SERIES-REVIEWER", reviewAtOne, domain.SeriesReviewApproved))
	recordReview(t, reviews, transactor, ctx, seriesReview(t, second, "SYN-PRC-SERIES-REVIEWER", reviewAtThree, domain.SeriesReviewApproved))

	between, outcome, err := reviews.ResolveInForce(ctx, tenant, domain.ReferenceSeriesFuelRate, "SYN-PRC-FUEL-TWO", reviewAtTwo)
	if err != nil || outcome != ports.SeriesVersionInForce || between.Version() != "v1" {
		t.Fatalf("v2 通过前应在用 v1：outcome=%d ref=%s err=%v", outcome, between.Version(), err)
	}
	after, outcome, err := reviews.ResolveInForce(ctx, tenant, domain.ReferenceSeriesFuelRate, "SYN-PRC-FUEL-TWO", reviewAtThree)
	if err != nil || outcome != ports.SeriesVersionInForce || after.Version() != "v2" {
		t.Fatalf("v2 通过时刻起应在用 v2：outcome=%d ref=%s err=%v", outcome, after.Version(), err)
	}
}

// TestReturnedReviewsNeverPutAVersionInForce 证退回不进在用，且通过之后的退回不撤销——取值
// 错误以更正版本处理，不靠撤复核。
func TestReturnedReviewsNeverPutAVersionInForce(t *testing.T) {
	versions, reviews, transactor := newReviewRegister(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	registration := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-RET", "v1")
	registerSeries(t, versions, transactor, ctx, registration)
	recordReview(t, reviews, transactor, ctx, seriesReview(t, registration, "SYN-PRC-SERIES-REVIEWER", reviewAtOne, domain.SeriesReviewReturned))
	if _, outcome, err := reviews.ResolveInForce(ctx, tenant, domain.ReferenceSeriesFuelRate, "SYN-PRC-FUEL-RET", reviewAtThree); err != nil || outcome != ports.SeriesHasNoApprovedVersion {
		t.Fatalf("只有退回却在用了：outcome=%d err=%v", outcome, err)
	}

	recordReview(t, reviews, transactor, ctx, seriesReview(t, registration, "SYN-PRC-SERIES-REVIEWER", reviewAtTwo, domain.SeriesReviewApproved))
	recordReview(t, reviews, transactor, ctx, seriesReview(t, registration, "SYN-PRC-SERIES-REVIEWER-2", reviewAtThree, domain.SeriesReviewReturned))
	reference, outcome, err := reviews.ResolveInForce(ctx, tenant, domain.ReferenceSeriesFuelRate, "SYN-PRC-FUEL-RET", reviewAtThree.Add(time.Hour))
	if err != nil || outcome != ports.SeriesVersionInForce || reference.Version() != "v1" {
		t.Fatalf("通过后的退回撤销了在用：outcome=%d err=%v", outcome, err)
	}
}

// TestInForceAnswersByRecoveryAction 证三个「没有」分格：无版本、种类不合、跨租户不可见。
func TestInForceAnswersByRecoveryAction(t *testing.T) {
	versions, reviews, transactor := newReviewRegister(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	if _, outcome, err := reviews.ResolveInForce(ctx, tenant, domain.ReferenceSeriesFuelRate, "SYN-PRC-NEVER", reviewAtOne); err != nil || outcome != ports.SeriesHasNoRegisteredVersion {
		t.Fatalf("从未登记的序列：outcome=%d err=%v", outcome, err)
	}

	registration := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-KIND", "v1")
	registerSeries(t, versions, transactor, ctx, registration)
	recordReview(t, reviews, transactor, ctx, seriesReview(t, registration, "SYN-PRC-SERIES-REVIEWER", reviewAtOne, domain.SeriesReviewApproved))
	if _, outcome, err := reviews.ResolveInForce(ctx, tenant, domain.ReferenceSeriesExchangeRate, "SYN-PRC-FUEL-KIND", reviewAtTwo); err != nil || outcome != ports.SeriesKindDisagrees {
		t.Fatalf("拿汇率种类问燃油序列：outcome=%d err=%v，想要 SeriesKindDisagrees", outcome, err)
	}
	other := evaluationValue(t, domain.NewTenantID, "tenant-b")
	if _, outcome, err := reviews.ResolveInForce(ctx, other, domain.ReferenceSeriesFuelRate, "SYN-PRC-FUEL-KIND", reviewAtTwo); err != nil || outcome != ports.SeriesHasNoRegisteredVersion {
		t.Fatalf("别的租户看见了该序列：outcome=%d err=%v", outcome, err)
	}
	if _, _, err := reviews.ResolveInForce(ctx, tenant, domain.ReferenceSeriesFuelRate, "SYN-PRC-FUEL-KIND", time.Time{}); err == nil {
		t.Fatal("零时刻被接受了")
	}
}

// TestReviewRecordAlgebra 证复核写口的结果代数：同键同内容是幂等重放；同键异内容是冲突且
// 原行不顶替；复核不在册版本答 VersionUnknown 而不是 error。
func TestReviewRecordAlgebra(t *testing.T) {
	versions, reviews, transactor := newReviewRegister(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	registration := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-ALG", "v1")
	registerSeries(t, versions, transactor, ctx, registration)

	approved := seriesReview(t, registration, "SYN-PRC-SERIES-REVIEWER", reviewAtOne, domain.SeriesReviewApproved)
	if outcome := recordReview(t, reviews, transactor, ctx, approved); outcome != ports.ReferenceSeriesReviewRecorded {
		t.Fatalf("首记 outcome = %d", outcome)
	}
	if outcome := recordReview(t, reviews, transactor, ctx, approved); outcome != ports.ReferenceSeriesReviewAlreadyRecorded {
		t.Fatalf("重记 outcome = %d，想要 AlreadyRecorded", outcome)
	}
	impostor, err := domain.NewSeriesReview(registration, "SYN-PRC-SERIES-REVIEWER", reviewAtOne, domain.SeriesReviewReturned, "同一刻改口")
	if err != nil {
		t.Fatalf("构造冒名复核：%v", err)
	}
	if outcome := recordReview(t, reviews, transactor, ctx, impostor); outcome != ports.ReferenceSeriesReviewConflict {
		t.Fatalf("同键异内容 outcome = %d，想要 Conflict", outcome)
	}
	if reference, outcome, err := reviews.ResolveInForce(ctx, tenant, domain.ReferenceSeriesFuelRate, "SYN-PRC-FUEL-ALG", reviewAtTwo); err != nil || outcome != ports.SeriesVersionInForce || reference.Version() != "v1" {
		t.Fatalf("冲突写入顶替了原行：outcome=%d err=%v", outcome, err)
	}

	unregistered := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-ALG", "v7")
	if outcome := recordReview(t, reviews, transactor, ctx, seriesReview(t, unregistered, "SYN-PRC-SERIES-REVIEWER", reviewAtOne, domain.SeriesReviewApproved)); outcome != ports.ReferenceSeriesReviewVersionUnknown {
		t.Fatalf("复核不在册版本 outcome = %d，想要 VersionUnknown", outcome)
	}
}

// TestReviewCheckConstraintsRejectImpossibleRows 证库层再守一遍形状：结论封闭、空复核人、
// 空依据、不在册版本都进不去。
func TestReviewCheckConstraintsRejectImpossibleRows(t *testing.T) {
	fixture := newReviewFixture(t)
	ctx := t.Context()
	registration := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-CHK", "v1")
	registerSeries(t, fixture.versions, fixture.transactor, ctx, registration)

	execRaw := func(sql string, args ...any) error {
		_, err := fixture.pool.Exec(ctx, sql, args...)
		return err
	}

	if err := execRaw(
		`INSERT INTO parcel_pricing.reference_series_review
			(tenant_id, series_id, series_version, reviewer, reviewed_at, decision, basis)
		 VALUES ('tenant-a', 'SYN-PRC-FUEL-CHK', 'v1', 'r', $1, 'MAYBE', 'b')`, reviewAtOne); err == nil {
		t.Fatal("一行「未知结论」溜进了复核册")
	}
	if err := execRaw(
		`INSERT INTO parcel_pricing.reference_series_review
			(tenant_id, series_id, series_version, reviewer, reviewed_at, decision, basis)
		 VALUES ('tenant-a', 'SYN-PRC-FUEL-CHK', 'v1', ' ', $1, 'APPROVED', 'b')`, reviewAtOne); err == nil {
		t.Fatal("一行「空复核人」溜进了复核册")
	}
	if err := execRaw(
		`INSERT INTO parcel_pricing.reference_series_review
			(tenant_id, series_id, series_version, reviewer, reviewed_at, decision, basis)
		 VALUES ('tenant-a', 'SYN-PRC-FUEL-CHK', 'v1', 'r', $1, 'APPROVED', '')`, reviewAtOne); err == nil {
		t.Fatal("一行「空依据」溜进了复核册")
	}
	if err := execRaw(
		`INSERT INTO parcel_pricing.reference_series_review
			(tenant_id, series_id, series_version, reviewer, reviewed_at, decision, basis)
		 VALUES ('tenant-a', 'SYN-PRC-FUEL-CHK', 'v9', 'r', $1, 'APPROVED', 'b')`, reviewAtOne); err == nil {
		t.Fatal("一行「不在册版本」溜进了复核册")
	}
}
