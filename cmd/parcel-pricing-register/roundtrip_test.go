package main

import (
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 走受控登记口的复核整条链（ADR-0262 决定一）：装配经 assemble、事务经真 Transactor，测的是随
// 产品发出的那条接线。CLI 的复核者只是批文里的 reviewer 串、授予集为空，与操作者面走同一道门。夹具全部 SYN（S 级）。

// Covers: AT-PP-014、AT-PP-015 的受控登记口那一半——规则未登记时序列与目录复核答未配置（2）、不落复核行；规则登记一租户一条，
// 同内容重放答 0、异内容冲突答 2；规则要求不同主体时登记者本人复核答须换人（2），另一个人才落；规则要求某一格授予时，经
// CLI 复核不论复核者是谁一律答复核者不合格（2）、不落复核行。
func TestTheControlledReviewsGoThroughTheTenantRuleAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	wired, err := assemble(db)
	if err != nil {
		t.Fatalf("装配：%v", err)
	}
	ctx := t.Context()
	run := func(kind, raw string) (string, int) {
		if kind == kindReferenceReviewDutyRule {
			return executeReviewDutyRule(ctx, []byte(raw), wired.rules, db.Transactor())
		}
		return execute(ctx, kind, []byte(raw), wired.cards, wired.series, wired.reviewer, wired.catalogues, db.Transactor())
	}
	count := func(table string) int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM parcel_pricing.`+table).Scan(&n); err != nil {
			t.Fatalf("数 %s：%v", table, err)
		}
		return n
	}

	if message, code := run(kindReferenceSeries, string(seriesSnapshot(t))); code != exitRegistered {
		t.Fatalf("登序列：%q %d", message, code)
	}
	if message, code := run(kindReferenceCatalogue, string(catalogueSnapshot(t))); code != exitRegistered {
		t.Fatalf("登目录：%q %d", message, code)
	}
	seriesReview := func(reviewer string) string {
		return `{"tenant":"tenant-1","seriesId":"SYN-PRC-FUEL-WEEKLY","seriesVersion":"v1","reviewer":"` + reviewer +
			`","decision":"APPROVED","basis":"SYN-REVIEW/逐期核对","reviewedAt":"2026-08-20T09:00:00Z"}`
	}
	catalogueReview := func(reviewer string) string {
		return `{"tenant":"tenant-1","catalogueId":"SYN-CAT-ZONE-CLI","catalogueVersion":"v1","reviewer":"` + reviewer +
			`","decision":"APPROVED","basis":"SYN-REVIEW/逐条对照","reviewedAt":"2026-01-02T00:00:00Z"}`
	}

	// 规则未登记：未配置，一行不落。
	if message, code := run(kindReferenceSeriesReview, seriesReview("SYN-PRC-SERIES-REVIEWER")); code != exitGovernance || !strings.Contains(message, "NOT_CONFIGURED") {
		t.Fatalf("未登规则复核序列：%q %d", message, code)
	}
	if message, code := run(kindReferenceCatalogueReview, catalogueReview("SYN-CAT-REVIEWER")); code != exitGovernance || !strings.Contains(message, "NOT_CONFIGURED") {
		t.Fatalf("未登规则复核目录：%q %d", message, code)
	}
	if count("reference_series_review") != 0 || count("reference_catalogue_review") != 0 {
		t.Fatal("未配置落了复核行")
	}

	// 要求某一格授予：CLI 证明不了授予，不论复核者是谁一律复核者不合格。
	if message, code := run(kindReferenceReviewDutyRule, `{"tenant":"tenant-1","distinctSubjects":true,"requiredGrant":"SYN-GRANT-REFERENCE-REVIEW"}`); code != exitRegistered || !strings.Contains(message, "RECORDED") {
		t.Fatalf("登要求授予的规则：%q %d", message, code)
	}
	for _, reviewer := range []string{"SYN-PRC-SERIES-REVIEWER", "SYN-PRC-SERIES-REGISTRAR"} {
		if message, code := run(kindReferenceSeriesReview, seriesReview(reviewer)); code != exitGovernance || !strings.Contains(message, "REVIEWER_NOT_QUALIFIED") {
			t.Fatalf("要求授予时 %s 经 CLI 复核序列：%q %d", reviewer, message, code)
		}
	}
	if message, code := run(kindReferenceCatalogueReview, catalogueReview("SYN-CAT-REVIEWER")); code != exitGovernance || !strings.Contains(message, "REVIEWER_NOT_QUALIFIED") {
		t.Fatalf("要求授予时经 CLI 复核目录：%q %d", message, code)
	}
	if count("reference_series_review") != 0 || count("reference_catalogue_review") != 0 {
		t.Fatal("复核者不合格落了复核行")
	}

	// 一租户一条：同内容重放答 0，异内容冲突答 2、原行不被顶替。
	if message, code := run(kindReferenceReviewDutyRule, `{"tenant":"tenant-1","distinctSubjects":true,"requiredGrant":"SYN-GRANT-REFERENCE-REVIEW"}`); code != exitRegistered || !strings.Contains(message, "ALREADY_REGISTERED") {
		t.Fatalf("同内容再登：%q %d", message, code)
	}
	if message, code := run(kindReferenceReviewDutyRule, `{"tenant":"tenant-1","distinctSubjects":true,"requiredGrant":null}`); code != exitGovernance || !strings.Contains(message, "CONTENT_CONFLICT") {
		t.Fatalf("异内容再登：%q %d", message, code)
	}
	if count("reference_review_duty_rule") != 1 {
		t.Fatal("规则册不是一租户一条")
	}

	// 另一租户登要求不同主体、不要求授予的那一条（演示租户的形状）：登记者本人答须换人，另一个人才落、进在用。
	if message, code := run(kindReferenceSeries, string(seriesSnapshotFor(t, "tenant-2"))); code != exitRegistered {
		t.Fatalf("登第二租户序列：%q %d", message, code)
	}
	if message, code := run(kindReferenceReviewDutyRule, `{"tenant":"tenant-2","distinctSubjects":true,"requiredGrant":null}`); code != exitRegistered {
		t.Fatalf("登第二租户规则：%q %d", message, code)
	}
	tenantTwo := func(reviewer string) string {
		return strings.Replace(seriesReview(reviewer), `"tenant-1"`, `"tenant-2"`, 1)
	}
	if message, code := run(kindReferenceSeriesReview, tenantTwo("SYN-PRC-SERIES-REGISTRAR")); code != exitGovernance || !strings.Contains(message, "NEEDS_ANOTHER_REVIEWER") {
		t.Fatalf("登记者本人经 CLI 复核：%q %d", message, code)
	}
	if count("reference_series_review") != 0 {
		t.Fatal("须换人落了复核行")
	}
	if message, code := run(kindReferenceSeriesReview, tenantTwo("SYN-PRC-SERIES-REVIEWER")); code != exitRegistered || !strings.Contains(message, "RECORDED") {
		t.Fatalf("另一个人经 CLI 复核：%q %d", message, code)
	}
	reviews, err := adapter.NewReferenceSeriesReviews(db)
	if err != nil {
		t.Fatalf("构造复核册：%v", err)
	}
	tenant, err := domain.NewTenantID("tenant-2")
	if err != nil {
		t.Fatal(err)
	}
	if inForce, outcome, err := reviews.ResolveInForce(ctx, tenant, domain.ReferenceSeriesFuelRate, "SYN-PRC-FUEL-WEEKLY",
		time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)); err != nil || outcome != ports.SeriesVersionInForce || inForce.Version() != "v1" {
		t.Fatalf("经 CLI 复核通过的那一版没进在用：%v outcome=%d err=%v", inForce, outcome, err)
	}
}

// catalogueSnapshot 折装一版目录 SYN-CAT-ZONE-CLI，登记责任方 SYN-CAT-REGISTRAR。
func catalogueSnapshot(t *testing.T) []byte {
	t.Helper()
	period, err := domain.NewEffectivePeriod(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("生效区间：%v", err)
	}
	origin, err := domain.NewPostalPrefixCatalogueOrigin([]string{"200"})
	if err != nil {
		t.Fatalf("始发维度：%v", err)
	}
	entry, err := domain.NewCatalogueEntry("018", domain.CategoryValue("Z1"))
	if err != nil {
		t.Fatalf("条目：%v", err)
	}
	reference, err := domain.NewVersionReference(domain.ArtifactReferenceCatalogue, "SYN-CAT-ZONE-CLI", "v1", "sha256:syn-cat")
	if err != nil {
		t.Fatalf("引用：%v", err)
	}
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	registration, err := domain.NewReferenceCatalogueRegistration(domain.ReferenceCatalogueRegistrationSpec{
		Tenant:           tenant,
		Kind:             domain.CatalogueKindZone,
		Reference:        reference,
		SourceIdentifier: "SYN-CARRIER/zone-bulletin",
		Registrant:       "SYN-CAT-REGISTRAR",
		Origin:           origin,
		PrefixLength:     3,
		Entries:          []domain.CatalogueEntry{entry},
		Period:           period,
	})
	if err != nil {
		t.Fatalf("目录登记：%v", err)
	}
	raw, err := domain.MarshalReferenceCatalogueRegistration(registration)
	if err != nil {
		t.Fatalf("折装目录快照：%v", err)
	}
	return raw
}
