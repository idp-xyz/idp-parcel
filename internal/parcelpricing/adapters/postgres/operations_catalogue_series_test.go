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

// 本文件对真实 PostgreSQL 16 证序列目录逐版本读面新加的两组字段（票
// pricing-reference-series-operations/08 件①）：复核事实照复核册计数与取最近一条、期次照
// 快照经领域重建门转写、登记时声明的引用 digest 照实透出；无复核的版本两个计数为零而不是
// 缺字段；limit 数的是版本数不是连接后的行数。**「在用」不在这些字段里**——目录页没有
// 正当的时刻源，在用归覆盖读口（票 04 那条 owner 裁决）。夹具全部为 SYN 合成序列（S 级）。

func newSeriesCatalogueWithReviews(t *testing.T) (
	*adapter.OperationsCatalogue,
	*adapter.ReferenceSeriesVersions,
	*adapter.ReferenceSeriesReviews,
	bentoapp.Transactor,
) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalogue, err := adapter.NewOperationsCatalogue(db)
	if err != nil {
		t.Fatalf("构造目录读面：%v", err)
	}
	versions, err := adapter.NewReferenceSeriesVersions(db)
	if err != nil {
		t.Fatalf("构造序列登记册：%v", err)
	}
	reviews, err := adapter.NewReferenceSeriesReviews(db)
	if err != nil {
		t.Fatalf("构造复核册：%v", err)
	}
	return catalogue, versions, reviews, db.Transactor()
}

func seriesRowOf(t *testing.T, rows []ports.ReferenceSeriesCatalogueRow, seriesID, version string) ports.ReferenceSeriesCatalogueRow {
	t.Helper()
	for _, row := range rows {
		if row.SeriesID == seriesID && row.SeriesVersion == version {
			return row
		}
	}
	t.Fatalf("目录里没有 %s@%s：%+v", seriesID, version, rows)
	return ports.ReferenceSeriesCatalogueRow{}
}

// Covers: 票 08 件①——复核事实按版本计数并取最近一条（同版本多条复核、退回之后再通过），
// 无复核的版本计数为零；期次起/止/值/凭证逐期转写，缺凭证与无上界各自可缺席；引用 digest
// 是登记时声明的那一个，不是 PRS 内容摘要。
func TestReferenceSeriesCatalogueCarriesReviewFactsAndPeriods(t *testing.T) {
	catalogue, versions, reviews, transactor := newSeriesCatalogueWithReviews(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	first := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-FACE", "v1",
		registerPeriod(t, registerWeekOne, registerWeekTwo, "0.22", "SYN-EVIDENCE/fuel-2026-W32"),
		registerPeriod(t, registerWeekTwo, time.Time{}, "0.24", ""))
	second := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-FACE", "v2",
		registerPeriod(t, registerWeekOne, registerWeekTwo, "0.23", "SYN-EVIDENCE/fuel-2026-W32-reissued"))
	registerSeries(t, versions, transactor, ctx, first)
	registerSeries(t, versions, transactor, ctx, second)
	recordReview(t, reviews, transactor, ctx,
		seriesReview(t, first, "SYN-PRC-SERIES-REVIEWER", reviewAtOne, domain.SeriesReviewReturned))
	recordReview(t, reviews, transactor, ctx,
		seriesReview(t, first, "SYN-PRC-SERIES-REVIEWER-2", reviewAtTwo, domain.SeriesReviewApproved))

	rows, err := catalogue.ListReferenceSeries(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列序列：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("上列 %d 行，想要 2（复核不该把一版撑成多行）", len(rows))
	}

	reviewed := seriesRowOf(t, rows, "SYN-PRC-FUEL-FACE", "v1")
	if reviewed.ReviewCount != 2 || reviewed.ApprovedReviewCount != 1 {
		t.Fatalf("v1 复核计数 = %d/%d，想要 2/1", reviewed.ReviewCount, reviewed.ApprovedReviewCount)
	}
	if !reviewed.HasReview || !reviewed.LastReviewedAt.Equal(reviewAtTwo) || reviewed.LastReviewDecision != domain.SeriesReviewApproved.String() {
		t.Fatalf("v1 最近一次复核 = %v/%q/%v，想要 %v/APPROVED/true",
			reviewed.LastReviewedAt, reviewed.LastReviewDecision, reviewed.HasReview, reviewAtTwo)
	}
	if reviewed.ReferenceDigest != first.Reference().Digest() {
		t.Fatalf("v1 引用 digest = %q，想要登记时声明的 %q", reviewed.ReferenceDigest, first.Reference().Digest())
	}
	if reviewed.ReferenceDigest == reviewed.ContentDigest {
		t.Fatal("引用 digest 与 PRS 内容摘要是两回事，读面把它们混成了一个")
	}
	if len(reviewed.Periods) != 2 {
		t.Fatalf("v1 期次数 = %d，想要 2", len(reviewed.Periods))
	}
	head := reviewed.Periods[0]
	if !head.StartsAt.Equal(registerWeekOne) || !head.HasEndsAt || !head.EndsAt.Equal(registerWeekTwo) ||
		head.Value != "0.22" || !head.HasEvidence || head.EvidenceRef != "SYN-EVIDENCE/fuel-2026-W32" {
		t.Fatalf("v1 首期转写变形：%+v", head)
	}
	tail := reviewed.Periods[1]
	if !tail.StartsAt.Equal(registerWeekTwo) || tail.HasEndsAt || tail.Value != "0.24" || tail.HasEvidence || tail.EvidenceRef != "" {
		t.Fatalf("v1 末期（无上界、缺凭证）转写变形：%+v", tail)
	}

	unreviewed := seriesRowOf(t, rows, "SYN-PRC-FUEL-FACE", "v2")
	if unreviewed.ReviewCount != 0 || unreviewed.ApprovedReviewCount != 0 || unreviewed.HasReview {
		t.Fatalf("v2 没有复核却报了 %d/%d/%v", unreviewed.ReviewCount, unreviewed.ApprovedReviewCount, unreviewed.HasReview)
	}
	if !unreviewed.LastReviewedAt.IsZero() || unreviewed.LastReviewDecision != "" {
		t.Fatalf("v2 无复核却带了最近一次复核：%v/%q", unreviewed.LastReviewedAt, unreviewed.LastReviewDecision)
	}
	if len(unreviewed.Periods) != 1 || unreviewed.Periods[0].Value != "0.23" {
		t.Fatalf("v2 期次转写变形：%+v", unreviewed.Periods)
	}
}

// Covers: ADR-0077 Decision 五在新形状下仍成立——limit 数的是版本数：一版带两条复核时
// limit 1 仍只交回一版，且那一版的复核计数完整。
func TestReferenceSeriesCatalogueLimitCountsVersionsNotJoinedRows(t *testing.T) {
	catalogue, versions, reviews, transactor := newSeriesCatalogueWithReviews(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")

	// 先登一版无复核的，再登带两条复核的：排序按登记时间倒序，limit 1 落在带复核的那一版上。
	// limit 若数的是连接后的行，这一版只会带回一条复核——计数就会变成 1。
	registerSeries(t, versions, transactor, ctx, fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-LIMIT", "v1"))
	latest := fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-LIMIT", "v2")
	registerSeries(t, versions, transactor, ctx, latest)
	recordReview(t, reviews, transactor, ctx,
		seriesReview(t, latest, "SYN-PRC-SERIES-REVIEWER", reviewAtOne, domain.SeriesReviewReturned))
	recordReview(t, reviews, transactor, ctx,
		seriesReview(t, latest, "SYN-PRC-SERIES-REVIEWER-2", reviewAtTwo, domain.SeriesReviewApproved))

	rows, err := catalogue.ListReferenceSeries(ctx, tenant, 1)
	if err != nil {
		t.Fatalf("带 limit 上列：%v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("limit 1 交回 %d 行", len(rows))
	}
	if rows[0].SeriesVersion != "v2" {
		t.Fatalf("limit 1 交回的是 %s，想要最后登记的 v2", rows[0].SeriesVersion)
	}
	if rows[0].ReviewCount != 2 || rows[0].ApprovedReviewCount != 1 {
		t.Fatalf("limit 之下复核计数 = %d/%d，想要 2/1——连接吃掉了版本", rows[0].ReviewCount, rows[0].ApprovedReviewCount)
	}
}
