package postgres_test

import (
	"context"
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

// 本文件对真实 PostgreSQL 证计价参考目录登记册（ADR-0109 Decision 二）：登记行只增不改、映射更正走新版本
// 不静默替换、按计价基准时点与邮编路线解析且查不到只答「查过没查到」、复核四眼门与在用派生照序列那一套。
// 夹具全部为 SYN 合成目录（S 级），邮编与分区取值都是编出来的形状。

var (
	catalogueFrom = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	catalogueTo   = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
)

func newCatalogueRegisters(t *testing.T) (*adapter.ReferenceCatalogueVersions, *adapter.ReferenceCatalogueReviews, bentoapp.Transactor) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	versions, err := adapter.NewReferenceCatalogueVersions(db)
	if err != nil {
		t.Fatalf("构造目录登记册：%v", err)
	}
	reviews, err := adapter.NewReferenceCatalogueReviews(db)
	if err != nil {
		t.Fatalf("构造目录复核册：%v", err)
	}
	return versions, reviews, db.Transactor()
}

func catalogueReference(t *testing.T, id, version string) domain.VersionReference {
	t.Helper()
	reference, err := domain.NewVersionReferenceIdentity(domain.ArtifactReferenceCatalogue, id, version)
	if err != nil {
		t.Fatalf("构造目录引用：%v", err)
	}
	return reference
}

func zoneChartRegistration(t *testing.T, tenant, catalogueID, version string, entries map[string]string) domain.ReferenceCatalogueRegistration {
	t.Helper()
	period, err := domain.NewEffectivePeriod(catalogueFrom, catalogueTo)
	if err != nil {
		t.Fatalf("生效区间：%v", err)
	}
	origin, err := domain.NewPostalPrefixCatalogueOrigin([]string{"940"})
	if err != nil {
		t.Fatalf("始发维度：%v", err)
	}
	rows := make([]domain.CatalogueEntry, 0, len(entries))
	for prefix, value := range entries {
		entry, err := domain.NewCatalogueEntry(prefix, domain.CategoryValue(value))
		if err != nil {
			t.Fatalf("条目 %s：%v", prefix, err)
		}
		rows = append(rows, entry)
	}
	registration, err := domain.NewReferenceCatalogueRegistration(domain.ReferenceCatalogueRegistrationSpec{
		Tenant:           evaluationValue(t, domain.NewTenantID, tenant),
		Kind:             domain.CatalogueKindZone,
		Reference:        catalogueReference(t, catalogueID, version),
		SourceIdentifier: "SYN-CARRIER/zone-chart-2026",
		Registrant:       "SYN-CAT-REGISTRAR",
		Origin:           origin,
		PrefixLength:     3,
		Entries:          rows,
		Period:           period,
	})
	if err != nil {
		t.Fatalf("构造目录登记：%v", err)
	}
	return registration
}

func registerCatalogue(t *testing.T, register *adapter.ReferenceCatalogueVersions, transactor bentoapp.Transactor, ctx context.Context, registration domain.ReferenceCatalogueRegistration) ports.ReferenceCatalogueRegistrationOutcome {
	t.Helper()
	var outcome ports.ReferenceCatalogueRegistrationOutcome
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		saved, err := register.Register(txCtx, registration)
		outcome = saved
		return err
	}); err != nil {
		t.Fatalf("事务内登记失败：%v", err)
	}
	return outcome
}

func recordCatalogueReview(t *testing.T, reviews *adapter.ReferenceCatalogueReviews, transactor bentoapp.Transactor, ctx context.Context, review domain.CatalogueReview) ports.ReferenceCatalogueReviewOutcome {
	t.Helper()
	var outcome ports.ReferenceCatalogueReviewOutcome
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		saved, err := reviews.Record(txCtx, review)
		outcome = saved
		return err
	}); err != nil {
		t.Fatalf("事务内复核失败：%v", err)
	}
	return outcome
}

// TestReferenceCatalogueRegisterAndResolveRoundTrip 证登记后按时点与邮编路线原样解析：命中前缀给出分区，
// 查不到的邮编答「查过没查到」且版本引用在，区间外与不在册的版本不适用。
func TestReferenceCatalogueRegisterAndResolveRoundTrip(t *testing.T) {
	register, _, transactor := newCatalogueRegisters(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")
	registration := zoneChartRegistration(t, "tenant-a", "SYN-CAT-ZONE", "v1", map[string]string{"902": "Z4", "100": "Z8"})

	if outcome := registerCatalogue(t, register, transactor, ctx, registration); outcome != ports.ReferenceCatalogueRegistered {
		t.Fatalf("首登 outcome = %d, 想要 ReferenceCatalogueRegistered", outcome)
	}
	route, err := domain.NewPostalRoute("94016", "90210")
	if err != nil {
		t.Fatalf("路线：%v", err)
	}
	reading, applicable, err := register.ResolveAt(ctx, tenant, catalogueReference(t, "SYN-CAT-ZONE", "v1"), catalogueFrom.Add(time.Hour), route)
	if err != nil || !applicable {
		t.Fatalf("区间内解析失败：applicable=%v err=%v", applicable, err)
	}
	if value, resolved := reading.Value(); !resolved || value != "Z4" {
		t.Fatalf("读数变形：%q %v", value, resolved)
	}

	unknown, err := domain.NewPostalRoute("94016", "33101")
	if err != nil {
		t.Fatalf("路线：%v", err)
	}
	consulted, applicable, err := register.ResolveAt(ctx, tenant, catalogueReference(t, "SYN-CAT-ZONE", "v1"), catalogueFrom.Add(time.Hour), unknown)
	if err != nil || !applicable {
		t.Fatalf("查不到的邮编不该让版本不适用：applicable=%v err=%v", applicable, err)
	}
	if _, resolved := consulted.Value(); resolved || consulted.Reference().Version() != "v1" {
		t.Fatalf("查过没查到的读数变形：%#v", consulted)
	}

	if _, applicable, err := register.ResolveAt(ctx, tenant, catalogueReference(t, "SYN-CAT-ZONE", "v1"), catalogueTo, route); err != nil || applicable {
		t.Fatalf("区间外时点不该适用：applicable=%v err=%v", applicable, err)
	}
	if _, applicable, err := register.ResolveAt(ctx, tenant, catalogueReference(t, "SYN-CAT-ZONE", "v9"), catalogueFrom.Add(time.Hour), route); err != nil || applicable {
		t.Fatalf("不在册的版本不该适用：applicable=%v err=%v", applicable, err)
	}

	loaded, found, err := register.LoadVersion(ctx, tenant, "SYN-CAT-ZONE", "v1")
	if err != nil || !found || loaded.ContentDigest() != registration.ContentDigest() {
		t.Fatalf("读回失败或摘要不一致：found=%v err=%v", found, err)
	}
}

// TestReferenceCatalogueRowsAreAppendOnly 证同键重复按（形状 + 摘要）译成结果代数：同摘要幂等，不同摘要
// 是内容冲突且原行不被顶替——映射更正只能走新版本。
func TestReferenceCatalogueRowsAreAppendOnly(t *testing.T) {
	register, _, transactor := newCatalogueRegisters(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-b")
	first := zoneChartRegistration(t, "tenant-b", "SYN-CAT-ZONE", "v1", map[string]string{"902": "Z4"})
	registerCatalogue(t, register, transactor, ctx, first)

	if outcome := registerCatalogue(t, register, transactor, ctx, first); outcome != ports.ReferenceCatalogueAlreadyRegistered {
		t.Fatalf("重放 outcome = %d, 想要 AlreadyRegistered", outcome)
	}
	retyped := zoneChartRegistration(t, "tenant-b", "SYN-CAT-ZONE", "v1", map[string]string{"902": "Z5"})
	if outcome := registerCatalogue(t, register, transactor, ctx, retyped); outcome != ports.ReferenceCatalogueContentConflict {
		t.Fatalf("同版本不同映射 outcome = %d, 想要 ContentConflict", outcome)
	}
	route, _ := domain.NewPostalRoute("94016", "90210")
	reading, _, err := register.ResolveAt(ctx, tenant, catalogueReference(t, "SYN-CAT-ZONE", "v1"), catalogueFrom.Add(time.Hour), route)
	if err != nil {
		t.Fatalf("解析：%v", err)
	}
	if value, _ := reading.Value(); value != "Z4" {
		t.Fatalf("原行被顶替成了 %q", value)
	}
}

// TestReferenceCatalogueReviewDerivesTheInForceVersion 证复核追加与在用派生：没登记 / 登了没复核 / 复核通过
// 三格分开答；退回不进在用；种类不合另答一格。
func TestReferenceCatalogueReviewDerivesTheInForceVersion(t *testing.T) {
	register, reviews, transactor := newCatalogueRegisters(t)
	ctx := t.Context()
	tenant := evaluationValue(t, domain.NewTenantID, "tenant-c")
	at := catalogueFrom.Add(48 * time.Hour)

	if _, outcome, err := reviews.ResolveInForce(ctx, tenant, domain.CatalogueKindZone, "SYN-CAT-ZONE", at); err != nil || outcome != ports.CatalogueHasNoRegisteredVersion {
		t.Fatalf("没登记：outcome=%d err=%v", outcome, err)
	}
	registration := zoneChartRegistration(t, "tenant-c", "SYN-CAT-ZONE", "v1", map[string]string{"902": "Z4"})
	registerCatalogue(t, register, transactor, ctx, registration)
	if _, outcome, err := reviews.ResolveInForce(ctx, tenant, domain.CatalogueKindZone, "SYN-CAT-ZONE", at); err != nil || outcome != ports.CatalogueHasNoApprovedVersion {
		t.Fatalf("登了没复核：outcome=%d err=%v", outcome, err)
	}
	if _, outcome, err := reviews.ResolveInForce(ctx, tenant, domain.CatalogueKindRemoteTier, "SYN-CAT-ZONE", at); err != nil || outcome != ports.CatalogueKindDisagrees {
		t.Fatalf("种类不合：outcome=%d err=%v", outcome, err)
	}

	unknownVersion := zoneChartRegistration(t, "tenant-c", "SYN-CAT-ZONE", "v7", map[string]string{"902": "Z4"})
	orphan, err := domain.NewCatalogueReview(unknownVersion, "SYN-CAT-REVIEWER", at.Add(-time.Hour), domain.SeriesReviewApproved, "SYN-REVIEW/never-registered")
	if err != nil {
		t.Fatalf("复核：%v", err)
	}
	if outcome := recordCatalogueReview(t, reviews, transactor, ctx, orphan); outcome != ports.ReferenceCatalogueReviewVersionUnknown {
		t.Fatalf("复核不在册版本 outcome = %d, 想要 VersionUnknown", outcome)
	}

	approved, err := domain.NewCatalogueReview(registration, "SYN-CAT-REVIEWER", at.Add(-time.Hour), domain.SeriesReviewApproved, "SYN-REVIEW/zone-v1")
	if err != nil {
		t.Fatalf("复核：%v", err)
	}
	if outcome := recordCatalogueReview(t, reviews, transactor, ctx, approved); outcome != ports.ReferenceCatalogueReviewRecorded {
		t.Fatalf("首条复核 outcome = %d", outcome)
	}
	if outcome := recordCatalogueReview(t, reviews, transactor, ctx, approved); outcome != ports.ReferenceCatalogueReviewAlreadyRecorded {
		t.Fatalf("重放复核 outcome = %d", outcome)
	}
	changed, _ := domain.NewCatalogueReview(registration, "SYN-CAT-REVIEWER", at.Add(-time.Hour), domain.SeriesReviewReturned, "SYN-REVIEW/zone-v1")
	if outcome := recordCatalogueReview(t, reviews, transactor, ctx, changed); outcome != ports.ReferenceCatalogueReviewConflict {
		t.Fatalf("同键不同结论 outcome = %d", outcome)
	}

	inForce, outcome, err := reviews.ResolveInForce(ctx, tenant, domain.CatalogueKindZone, "SYN-CAT-ZONE", at)
	if err != nil || outcome != ports.CatalogueVersionInForce || inForce.Version() != "v1" {
		t.Fatalf("在用：%v outcome=%d err=%v", inForce, outcome, err)
	}
	if _, outcome, err := reviews.ResolveInForce(ctx, tenant, domain.CatalogueKindZone, "SYN-CAT-ZONE", at.Add(-2*time.Hour)); err != nil || outcome != ports.CatalogueHasNoApprovedVersion {
		t.Fatalf("复核之前的时刻不该有在用：outcome=%d err=%v", outcome, err)
	}
}
