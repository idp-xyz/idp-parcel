package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证参考复核职责规则（0014；ADR-0262 决定一）：规则一租户一条，重放与冲突分格；序列与目录的
// 人工复核经真编排、真登记册与真复核册按这一条判——没登答未配置、规则要求不同主体时登记者本人答须换人、不要求时照落并进
// 在用、要求授予而复核者没有答复核者不合格；拒下的三格都不落复核行，那一版也就不进在用。夹具全部为 SYN 合成值（S 级）。

const (
	dutyRuleGrant      = "SYN-GRANT-REFERENCE-REVIEW"
	dutyRuleRegistrant = "SYN-PRC-SERIES-REGISTRAR"
	dutyRuleReviewer   = "SYN-PRC-SERIES-REVIEWER"
)

// reviewRule 与 reviewSubject 是本包复核夹具共用的两件：要求不同主体、不要求授予，正是演示租户登的那一条的形状。
func reviewRule(t *testing.T, tenant string) domain.ReferenceReviewDutyRule {
	t.Helper()
	rule, err := domain.NewReferenceReviewDutyRule(evaluationValue(t, domain.NewTenantID, tenant), true, domain.OperatorGrant{})
	if err != nil {
		t.Fatalf("构造参考复核职责规则：%v", err)
	}
	return rule
}

func reviewSubject(t *testing.T, reference string, grants ...string) domain.OperatorSubject {
	t.Helper()
	held := make([]domain.OperatorGrant, 0, len(grants))
	for _, name := range grants {
		held = append(held, evaluationValue(t, domain.NewOperatorGrant, name))
	}
	subject, err := domain.NewOperatorSubject(reference, held)
	if err != nil {
		t.Fatalf("构造复核者：%v", err)
	}
	return subject
}

type dutyRuleStore struct {
	db         *bentopg.DB
	pool       *pgxpool.Pool
	rules      *adapter.ReferenceReviewDutyRules
	transactor bentoapp.Transactor
}

func newDutyRuleStore(t *testing.T) dutyRuleStore {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	rules, err := adapter.NewReferenceReviewDutyRules(db)
	if err != nil {
		t.Fatalf("构造参考复核职责规则册：%v", err)
	}
	return dutyRuleStore{db: db, pool: pool, rules: rules, transactor: db.Transactor()}
}

func (store dutyRuleStore) save(t *testing.T, rule domain.ReferenceReviewDutyRule) ports.ReferenceReviewDutyRuleSaveOutcome {
	t.Helper()
	var outcome ports.ReferenceReviewDutyRuleSaveOutcome
	if err := store.transactor.WithinTransaction(t.Context(), func(ctx context.Context) (err error) {
		outcome, err = store.rules.SaveReferenceReviewDutyRule(ctx, rule)
		return err
	}); err != nil {
		t.Fatalf("事务内登记规则：%v", err)
	}
	return outcome
}

func (store dutyRuleStore) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var count int
	if err := store.pool.QueryRow(t.Context(), query, args...).Scan(&count); err != nil {
		t.Fatalf("数行：%v", err)
	}
	return count
}

// 规则一租户一条：没登即不在（复核门据此答`未配置`）；首登入册、读回原样；同内容再登是重放；异内容答冲突、原行不被顶替；
// 别的租户各登各的，两格都不要求的那一条读回仍是两格都不要求。登记是写：无事务即拒。
func TestTheReferenceReviewDutyRuleIsRegisteredOncePerTenant(t *testing.T) {
	store := newDutyRuleStore(t)
	tenant := evaluationValue(t, domain.NewTenantID, "SYN-TENANT-01")
	if _, found, err := store.rules.LoadReferenceReviewDutyRule(t.Context(), tenant); err != nil || found {
		t.Fatalf("没登的规则：found=%v err=%v", found, err)
	}
	grant := evaluationValue(t, domain.NewOperatorGrant, dutyRuleGrant)
	strict, err := domain.NewReferenceReviewDutyRule(tenant, true, grant)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.rules.SaveReferenceReviewDutyRule(t.Context(), strict); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务登记规则：err = %v，want ErrTransactionRequired", err)
	}
	if outcome := store.save(t, strict); outcome != ports.ReferenceReviewDutyRuleSaved {
		t.Fatalf("首登 = %v", outcome)
	}
	loaded, found, err := store.rules.LoadReferenceReviewDutyRule(t.Context(), tenant)
	if got, required := loaded.RequiredGrant(); err != nil || !found || !loaded.RequiresDistinctSubjects() || !required || got != grant {
		t.Fatalf("读回 = %+v found=%v err=%v", loaded, found, err)
	}
	if outcome := store.save(t, strict); outcome != ports.ReferenceReviewDutyRuleAlreadyRegistered {
		t.Fatalf("同内容再登 = %v", outcome)
	}
	for name, changed := range map[string]domain.ReferenceReviewDutyRule{
		"不同主体一格变了": mustDutyRule(t, tenant, false, grant),
		"授予一格变了":   mustDutyRule(t, tenant, true, domain.OperatorGrant{}),
	} {
		if outcome := store.save(t, changed); outcome != ports.ReferenceReviewDutyRuleContentConflict {
			t.Fatalf("%s：异内容再登 = %v", name, outcome)
		}
	}
	if kept, _, _ := store.rules.LoadReferenceReviewDutyRule(t.Context(), tenant); !kept.RequiresDistinctSubjects() {
		t.Fatal("冲突顶替了原行")
	}
	if n := store.count(t, `SELECT count(*) FROM parcel_pricing.reference_review_duty_rule`); n != 1 {
		t.Fatalf("册上 %d 行，want 1", n)
	}

	other := evaluationValue(t, domain.NewTenantID, "SYN-TENANT-02")
	if outcome := store.save(t, mustDutyRule(t, other, false, domain.OperatorGrant{})); outcome != ports.ReferenceReviewDutyRuleSaved {
		t.Fatalf("别的租户首登 = %v", outcome)
	}
	read, found, err := store.rules.LoadReferenceReviewDutyRule(t.Context(), other)
	if _, required := read.RequiredGrant(); err != nil || !found || read.RequiresDistinctSubjects() || required || read.Tenant() != other {
		t.Fatalf("两格都不要求的规则读回 = %+v found=%v err=%v", read, found, err)
	}
}

func mustDutyRule(t *testing.T, tenant domain.TenantID, distinct bool, grant domain.OperatorGrant) domain.ReferenceReviewDutyRule {
	t.Helper()
	rule, err := domain.NewReferenceReviewDutyRule(tenant, distinct, grant)
	if err != nil {
		t.Fatalf("构造规则：%v", err)
	}
	return rule
}

// manualReviews 用真适配器装序列与目录两只复核编排，读同一本规则册——装法与 parcel-api、受控登记口一致。
type manualReviews struct {
	store      dutyRuleStore
	versions   *adapter.ReferenceSeriesVersions
	catalogues *adapter.ReferenceCatalogueVersions
	series     *application.ReviewReferenceSeriesHandler
	catalogue  *application.ReviewReferenceCatalogueHandler
	seriesIn   *adapter.ReferenceSeriesReviews
	catalogIn  *adapter.ReferenceCatalogueReviews
}

func newManualReviews(t *testing.T) manualReviews {
	t.Helper()
	store := newDutyRuleStore(t)
	versions, err := adapter.NewReferenceSeriesVersions(store.db)
	if err != nil {
		t.Fatal(err)
	}
	seriesReviews, err := adapter.NewReferenceSeriesReviews(store.db)
	if err != nil {
		t.Fatal(err)
	}
	catalogues, err := adapter.NewReferenceCatalogueVersions(store.db)
	if err != nil {
		t.Fatal(err)
	}
	catalogueReviews, err := adapter.NewReferenceCatalogueReviews(store.db)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := adapter.NewPriceCards(store.db)
	if err != nil {
		t.Fatal(err)
	}
	outboxStore, err := outbox.NewStore(store.db)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := adapter.NewOutboxEvaluationResumptionHandoffs(store.db, outboxStore, fixedClockAt(reviewAtThree))
	if err != nil {
		t.Fatal(err)
	}
	return manualReviews{
		store: store, versions: versions, catalogues: catalogues, seriesIn: seriesReviews, catalogIn: catalogueReviews,
		series: application.NewReviewReferenceSeriesHandler(application.ReviewReferenceSeriesDeps{
			Versions: versions, Reviews: seriesReviews, DutyRules: store.rules, Clock: fixedClockAt(reviewAtThree),
			Registered: registered, PricedScopes: cards,
		}),
		catalogue: application.NewReviewReferenceCatalogueHandler(application.ReviewReferenceCatalogueDeps{
			Versions: catalogues, Reviews: catalogueReviews, DutyRules: store.rules, Clock: fixedClockAt(reviewAtThree),
			Registered: registered, PricedScopes: cards,
		}),
	}
}

type clockAt struct{ at time.Time }

func (clock clockAt) Now() time.Time { return clock.at }

func fixedClockAt(at time.Time) clockAt { return clockAt{at: at} }

func (reviews manualReviews) reviewSeries(t *testing.T, tenant, seriesID, reviewer string, grants ...string) application.ReviewReferenceSeriesOutcome {
	t.Helper()
	held := make([]domain.OperatorGrant, 0, len(grants))
	for _, name := range grants {
		held = append(held, evaluationValue(t, domain.NewOperatorGrant, name))
	}
	var outcome application.ReviewReferenceSeriesOutcome
	if err := reviews.store.transactor.WithinTransaction(t.Context(), func(ctx context.Context) (err error) {
		outcome, err = reviews.series.Handle(ctx, application.ReviewReferenceSeriesCommand{
			Tenant: evaluationValue(t, domain.NewTenantID, tenant), SeriesID: seriesID, SeriesVersion: "v1",
			Reviewer: reviewer, ReviewerGrants: held, Decision: domain.SeriesReviewApproved, Basis: "SYN-REVIEW/逐期核对公布记录",
			ReviewedAt: reviewAtOne,
		})
		return err
	}); err != nil {
		t.Fatalf("序列复核：%v", err)
	}
	return outcome
}

func (reviews manualReviews) reviewCatalogue(t *testing.T, tenant, catalogueID, reviewer string) application.ReviewReferenceCatalogueOutcome {
	t.Helper()
	var outcome application.ReviewReferenceCatalogueOutcome
	if err := reviews.store.transactor.WithinTransaction(t.Context(), func(ctx context.Context) (err error) {
		outcome, err = reviews.catalogue.Handle(ctx, application.ReviewReferenceCatalogueCommand{
			Tenant: evaluationValue(t, domain.NewTenantID, tenant), CatalogueID: catalogueID, CatalogueVersion: "v1",
			Reviewer: reviewer, Decision: domain.SeriesReviewApproved, Basis: "SYN-REVIEW/逐条对照公布表",
			ReviewedAt: catalogueFrom.Add(time.Hour),
		})
		return err
	}); err != nil {
		t.Fatalf("目录复核：%v", err)
	}
	return outcome
}

func (reviews manualReviews) seriesReviewRows(t *testing.T, tenant, seriesID string) int {
	return reviews.store.count(t, `SELECT count(*) FROM parcel_pricing.reference_series_review WHERE tenant_id = $1 AND series_id = $2`, tenant, seriesID)
}

func (reviews manualReviews) catalogueReviewRows(t *testing.T, tenant, catalogueID string) int {
	return reviews.store.count(t, `SELECT count(*) FROM parcel_pricing.reference_catalogue_review WHERE tenant_id = $1 AND catalogue_id = $2`, tenant, catalogueID)
}

func (reviews manualReviews) seriesInForce(t *testing.T, tenant, seriesID string) bool {
	t.Helper()
	_, outcome, err := reviews.seriesIn.ResolveInForce(t.Context(), evaluationValue(t, domain.NewTenantID, tenant),
		domain.ReferenceSeriesFuelRate, seriesID, reviewAtTwo)
	if err != nil {
		t.Fatalf("在用解析：%v", err)
	}
	return outcome == ports.SeriesVersionInForce
}

// Covers: AT-PP-014 真库——租户没登参考复核职责规则，操作者复核一版手工登记的序列与一版目录都答未配置、不落复核行，那一版
// 不进在用；不按默认双人也不按默认单人放行：登记者本人与另一个人复核答的是同一格。
func TestAManualReviewWithoutATenantRuleIsNotConfiguredAgainstARealDatabase(t *testing.T) {
	reviews := newManualReviews(t)
	registerSeries(t, reviews.versions, reviews.store.transactor, t.Context(), fuelRegistration(t, "tenant-a", "SYN-PRC-FUEL-RULE", "v1"))
	catalogue := zoneChartRegistration(t, "tenant-a", "SYN-CAT-ZONE-RULE", "v1", map[string]string{"902": "Z4"})
	if err := reviews.store.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, err := reviews.catalogues.Register(ctx, catalogue)
		return err
	}); err != nil {
		t.Fatalf("登目录：%v", err)
	}

	for _, reviewer := range []string{dutyRuleRegistrant, dutyRuleReviewer} {
		if outcome := reviews.reviewSeries(t, "tenant-a", "SYN-PRC-FUEL-RULE", reviewer); outcome != application.SeriesReviewNotConfigured {
			t.Fatalf("%s 复核序列 = %s，want NOT_CONFIGURED", reviewer, outcome)
		}
		if outcome := reviews.reviewCatalogue(t, "tenant-a", "SYN-CAT-ZONE-RULE", reviewer); outcome != application.CatalogueReviewNotConfigured {
			t.Fatalf("%s 复核目录 = %s，want NOT_CONFIGURED", reviewer, outcome)
		}
	}
	if n := reviews.seriesReviewRows(t, "tenant-a", "SYN-PRC-FUEL-RULE"); n != 0 {
		t.Fatalf("未配置落了 %d 条序列复核", n)
	}
	if n := reviews.catalogueReviewRows(t, "tenant-a", "SYN-CAT-ZONE-RULE"); n != 0 {
		t.Fatalf("未配置落了 %d 条目录复核", n)
	}
	if reviews.seriesInForce(t, "tenant-a", "SYN-PRC-FUEL-RULE") {
		t.Fatal("未配置的那一版进了在用")
	}
}

// Covers: AT-PP-015 真库——规则要求不同主体时登记者本人复核答须换人、不落复核行，另一个人复核才落；不要求不同主体的租户，
// 登记者本人复核照落、自复核时刻起在用；要求某一格授予时复核者没有它答复核者不合格、不落复核行，持有它才落。序列与目录同判。
func TestAManualReviewFollowsTheTenantRuleAgainstARealDatabase(t *testing.T) {
	reviews := newManualReviews(t)
	ctx := t.Context()
	for _, tenant := range []string{"tenant-a", "tenant-b", "tenant-c"} {
		registerSeries(t, reviews.versions, reviews.store.transactor, ctx, fuelRegistration(t, tenant, "SYN-PRC-FUEL-RULE", "v1"))
		catalogue := zoneChartRegistration(t, tenant, "SYN-CAT-ZONE-RULE", "v1", map[string]string{"902": "Z4"})
		if err := reviews.store.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			_, err := reviews.catalogues.Register(txCtx, catalogue)
			return err
		}); err != nil {
			t.Fatalf("登目录：%v", err)
		}
	}
	grant := evaluationValue(t, domain.NewOperatorGrant, dutyRuleGrant)
	reviews.store.save(t, mustDutyRule(t, evaluationValue(t, domain.NewTenantID, "tenant-a"), true, domain.OperatorGrant{}))
	reviews.store.save(t, mustDutyRule(t, evaluationValue(t, domain.NewTenantID, "tenant-b"), false, domain.OperatorGrant{}))
	reviews.store.save(t, mustDutyRule(t, evaluationValue(t, domain.NewTenantID, "tenant-c"), false, grant))

	// 要求不同主体：登记者本人答须换人、一行不落；另一个人复核才落、进在用。
	if outcome := reviews.reviewSeries(t, "tenant-a", "SYN-PRC-FUEL-RULE", dutyRuleRegistrant); outcome != application.SeriesReviewNeedsAnotherReviewer {
		t.Fatalf("要求不同主体、登记者本人复核序列 = %s", outcome)
	}
	catalogueRegistrant := "SYN-CAT-REGISTRAR"
	if outcome := reviews.reviewCatalogue(t, "tenant-a", "SYN-CAT-ZONE-RULE", catalogueRegistrant); outcome != application.CatalogueReviewNeedsAnotherReviewer {
		t.Fatalf("要求不同主体、登记者本人复核目录 = %s", outcome)
	}
	if reviews.seriesReviewRows(t, "tenant-a", "SYN-PRC-FUEL-RULE") != 0 || reviews.catalogueReviewRows(t, "tenant-a", "SYN-CAT-ZONE-RULE") != 0 ||
		reviews.seriesInForce(t, "tenant-a", "SYN-PRC-FUEL-RULE") {
		t.Fatal("须换人那一格落了复核行或进了在用")
	}
	if outcome := reviews.reviewSeries(t, "tenant-a", "SYN-PRC-FUEL-RULE", dutyRuleReviewer); outcome != application.SeriesReviewRecorded ||
		!reviews.seriesInForce(t, "tenant-a", "SYN-PRC-FUEL-RULE") {
		t.Fatalf("另一个人复核序列 = %s", outcome)
	}

	// 不要求不同主体：登记者本人复核照落，自复核时刻起在用。
	if outcome := reviews.reviewSeries(t, "tenant-b", "SYN-PRC-FUEL-RULE", dutyRuleRegistrant); outcome != application.SeriesReviewRecorded {
		t.Fatalf("不要求不同主体、登记者本人复核序列 = %s", outcome)
	}
	if outcome := reviews.reviewCatalogue(t, "tenant-b", "SYN-CAT-ZONE-RULE", catalogueRegistrant); outcome != application.CatalogueReviewRecorded {
		t.Fatalf("不要求不同主体、登记者本人复核目录 = %s", outcome)
	}
	if !reviews.seriesInForce(t, "tenant-b", "SYN-PRC-FUEL-RULE") {
		t.Fatal("单人复核通过的那一版没进在用")
	}
	inForce, outcome, err := reviews.catalogIn.ResolveInForce(ctx, evaluationValue(t, domain.NewTenantID, "tenant-b"), domain.CatalogueKindZone, "SYN-CAT-ZONE-RULE", catalogueFrom.Add(2*time.Hour))
	if err != nil || outcome != ports.CatalogueVersionInForce || inForce.Version() != "v1" {
		t.Fatalf("单人复核通过的目录没进在用：%v outcome=%d err=%v", inForce, outcome, err)
	}

	// 要求授予：没有它答复核者不合格、一行不落（目录口没有授予集，答的也是这一格）；持有它才落。
	if outcome := reviews.reviewSeries(t, "tenant-c", "SYN-PRC-FUEL-RULE", dutyRuleReviewer, "SYN-GRANT-OTHER"); outcome != application.SeriesReviewReviewerNotQualified {
		t.Fatalf("要求授予、复核者没有 = %s", outcome)
	}
	if outcome := reviews.reviewCatalogue(t, "tenant-c", "SYN-CAT-ZONE-RULE", dutyRuleReviewer); outcome != application.CatalogueReviewReviewerNotQualified {
		t.Fatalf("要求授予、目录复核者没有 = %s", outcome)
	}
	if reviews.seriesReviewRows(t, "tenant-c", "SYN-PRC-FUEL-RULE") != 0 || reviews.catalogueReviewRows(t, "tenant-c", "SYN-CAT-ZONE-RULE") != 0 {
		t.Fatal("复核者不合格那一格落了复核行")
	}
	if outcome := reviews.reviewSeries(t, "tenant-c", "SYN-PRC-FUEL-RULE", dutyRuleReviewer, dutyRuleGrant); outcome != application.SeriesReviewRecorded {
		t.Fatalf("要求授予、复核者持有 = %s", outcome)
	}
}
