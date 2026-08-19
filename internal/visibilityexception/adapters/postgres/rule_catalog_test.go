package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var catalogBaseAt = time.Date(2026, 4, 10, 8, 0, 0, 0, time.UTC)

type catalogFixture struct {
	pool *pgxpool.Pool
	db   *bentopg.DB
}

func newCatalogFixture(t *testing.T) *catalogFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return &catalogFixture{pool: pool, db: db}
}

func (fixture *catalogFixture) mappingsFor(t *testing.T, tenant string) *adapter.MilestoneMappings {
	t.Helper()
	var tenantID domain.TenantID
	if tenant != "" {
		tenantID = projectionValue(t, domain.NewTenantID, tenant)
	}
	view, err := adapter.NewMilestoneMappings(fixture.db, tenantID)
	if err != nil {
		t.Fatalf("构造映射视图：%v", err)
	}
	return view
}

func (fixture *catalogFixture) triageFor(t *testing.T, tenant string) *adapter.TriageRules {
	t.Helper()
	var tenantID domain.TenantID
	if tenant != "" {
		tenantID = projectionValue(t, domain.NewTenantID, tenant)
	}
	view, err := adapter.NewTriageRules(fixture.db, tenantID)
	if err != nil {
		t.Fatalf("构造分诊视图：%v", err)
	}
	return view
}

// 目录内容属实例半边，尚无登记入口，因此用例直接写行——证的是视图读得对。
func (fixture *catalogFixture) publishMapping(t *testing.T, tenant, version string, from time.Time, to *time.Time) {
	t.Helper()
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.milestone_mapping_version
			(tenant_id, mapping_version, effective_from, effective_to, approved_by)
		 VALUES ($1, $2, $3, $4, 'tracking-ops')`,
		tenant, version, from, to); err != nil {
		t.Fatalf("发布映射版本 %s：%v", version, err)
	}
}

func (fixture *catalogFixture) addMappingEntry(t *testing.T, tenant, version, kind, milestone string) {
	t.Helper()
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.milestone_mapping_entry
			(tenant_id, mapping_version, source_context, source_fact_kind, milestone_ref)
		 VALUES ($1, $2, 'NODE_OPERATIONS', $3, $4)`,
		tenant, version, kind, milestone); err != nil {
		t.Fatalf("写入映射条目 %s：%v", kind, err)
	}
}

func catalogFact(t *testing.T, factRef, kind string, occurredAt time.Time) domain.AcceptedSourceFact {
	t.Helper()
	fact, err := domain.NewAcceptedSourceFact(domain.AcceptedSourceFactSpec{
		Source:      domain.SourceNodeOperations,
		Parcel:      projectionValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		Fact:        projectionValue(t, domain.NewSourceFactReference, factRef),
		Kind:        projectionValue(t, domain.NewSourceFactKind, kind),
		Version:     projectionValue(t, domain.NewSourceFactVersion, "v1"),
		OccurredAt:  occurredAt,
		EffectiveAt: occurredAt.Add(time.Hour),
		ReceivedAt:  occurredAt.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("构造事实：%v", err)
	}
	return fact
}

// 空目录是首发唯一走得到的真实分支：没有租户就没有已发布映射版本。它必须与
// 「目录已配但这条没映射」分得开——前者等登记，后者是一次已作出的未归类判断。
func TestMissingMappingCatalogIsNotConfiguredRatherThanUnclassified(t *testing.T) {
	fixture := newCatalogFixture(t)
	fact := catalogFact(t, "scan/origin", "node-intake", catalogBaseAt)

	for _, tenant := range []string{"tenant-a", ""} {
		answer, configured, err := fixture.mappingsFor(t, tenant).ClassifyFact(t.Context(), fact)
		if err != nil || configured {
			t.Fatalf("租户 %q：err=%v configured=%v", tenant, err, configured)
		}
		if answer.Classified {
			t.Fatalf("租户 %q：空目录却交出了归类", tenant)
		}
	}
}

func TestMappingCatalogClassifiesHitAndLeavesMissUnclassified(t *testing.T) {
	fixture := newCatalogFixture(t)
	fixture.publishMapping(t, "tenant-a", "map/v1", catalogBaseAt.Add(-24*time.Hour), nil)
	fixture.addMappingEntry(t, "tenant-a", "map/v1", "node-intake", "PICKED_UP")
	view := fixture.mappingsFor(t, "tenant-a")

	answer, configured, err := view.ClassifyFact(t.Context(), catalogFact(t, "scan/origin", "node-intake", catalogBaseAt))
	if err != nil || !configured || !answer.Classified {
		t.Fatalf("命中：err=%v configured=%v classified=%v", err, configured, answer.Classified)
	}
	if answer.Milestone.String() != "PICKED_UP" || answer.Mapping.String() != "map/v1" {
		t.Fatalf("命中答复 = %s / %s", answer.Milestone, answer.Mapping)
	}

	// 一行覆盖同类型全部事实：不同事实引用、同一类型，必须命中同一里程碑。
	sameKind, configured, err := view.ClassifyFact(t.Context(), catalogFact(t, "scan/other", "node-intake", catalogBaseAt))
	if err != nil || !configured || !sameKind.Classified || sameKind.Milestone.String() != "PICKED_UP" {
		t.Fatalf("同类型另一引用：err=%v configured=%v classified=%v milestone=%s",
			err, configured, sameKind.Classified, sameKind.Milestone)
	}

	// 目录在场但这条类型没有可靠映射：如实未归类，且必须带上所依据的版本号——投影要能
	// 追溯「按哪版判的未归类」，而不是被强行映射成一个宽泛结果。
	answer, configured, err = view.ClassifyFact(t.Context(), catalogFact(t, "scan/origin", "unknown-kind", catalogBaseAt))
	if err != nil || !configured {
		t.Fatalf("未命中：err=%v configured=%v", err, configured)
	}
	if answer.Classified {
		t.Fatal("没有条目的事实被强行映射了")
	}
	if answer.Mapping.String() != "map/v1" {
		t.Fatalf("未归类没带版本号：%s", answer.Mapping)
	}
}

// 适用版本按事实的业务发生时间选：一条迟到很久才到达的事实属于它发生那天的版本。
func TestMappingVersionIsChosenByBusinessOccurrenceTime(t *testing.T) {
	fixture := newCatalogFixture(t)
	switchover := catalogBaseAt
	fixture.publishMapping(t, "tenant-a", "map/v1", switchover.Add(-30*24*time.Hour), &switchover)
	fixture.publishMapping(t, "tenant-a", "map/v2", switchover, nil)
	fixture.addMappingEntry(t, "tenant-a", "map/v1", "node-intake", "PICKED_UP")
	fixture.addMappingEntry(t, "tenant-a", "map/v2", "node-intake", "COLLECTED")
	view := fixture.mappingsFor(t, "tenant-a")

	old, _, err := view.ClassifyFact(t.Context(), catalogFact(t, "scan/origin", "node-intake", switchover.Add(-time.Hour)))
	if err != nil || old.Mapping.String() != "map/v1" || old.Milestone.String() != "PICKED_UP" {
		t.Fatalf("旧版事实按 %s 判成 %s（err=%v）", old.Mapping, old.Milestone, err)
	}
	fresh, _, err := view.ClassifyFact(t.Context(), catalogFact(t, "scan/origin", "node-intake", switchover.Add(time.Hour)))
	if err != nil || fresh.Mapping.String() != "map/v2" || fresh.Milestone.String() != "COLLECTED" {
		t.Fatalf("新版事实按 %s 判成 %s（err=%v）", fresh.Mapping, fresh.Milestone, err)
	}
}

// 两个版本同时适用时不得挑一个：CONTEXT 不许按最后到达或来源排名覆盖，挑一个
// 就是替商业责任方作了它没作的决定。
func TestOverlappingMappingVersionsAreRefusedNotRanked(t *testing.T) {
	fixture := newCatalogFixture(t)
	closed := catalogBaseAt.Add(60 * 24 * time.Hour)
	fixture.publishMapping(t, "tenant-a", "map/v1", catalogBaseAt.Add(-30*24*time.Hour), &closed)
	fixture.publishMapping(t, "tenant-a", "map/v2", catalogBaseAt.Add(-10*24*time.Hour), &closed)

	_, configured, err := fixture.mappingsFor(t, "tenant-a").
		ClassifyFact(t.Context(), catalogFact(t, "scan/origin", "node-intake", catalogBaseAt))
	if !errors.Is(err, adapter.ErrAmbiguousCatalog) {
		t.Fatalf("重叠版本没有报冲突：err=%v configured=%v", err, configured)
	}
}

// 未闭区间的版本至多一份，否则「当前适用版本」永远有两个答案。
func TestSecondOpenMappingVersionIsRejectedByTheIndex(t *testing.T) {
	fixture := newCatalogFixture(t)
	fixture.publishMapping(t, "tenant-a", "map/v1", catalogBaseAt, nil)
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.milestone_mapping_version
			(tenant_id, mapping_version, effective_from, effective_to, approved_by)
		 VALUES ('tenant-a', 'map/v2', $1, NULL, 'tracking-ops')`, catalogBaseAt); err == nil {
		t.Fatal("库接受了同租户第二份未闭区间的映射版本")
	}
	// 另一个租户的未闭区间版本不受影响（ADR-0003）。
	fixture.publishMapping(t, "tenant-b", "map/v1", catalogBaseAt, nil)
}

func TestMappingsOfAnotherTenantAreInvisible(t *testing.T) {
	fixture := newCatalogFixture(t)
	fixture.publishMapping(t, "tenant-a", "map/v1", catalogBaseAt.Add(-time.Hour), nil)
	fixture.addMappingEntry(t, "tenant-a", "map/v1", "node-intake", "PICKED_UP")

	if _, configured, err := fixture.mappingsFor(t, "tenant-b").
		ClassifyFact(t.Context(), catalogFact(t, "scan/origin", "node-intake", catalogBaseAt)); err != nil || configured {
		t.Fatalf("跨租户目录可见：err=%v configured=%v", err, configured)
	}
}

func (fixture *catalogFixture) publishTriage(t *testing.T, tenant, version string) {
	t.Helper()
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.triage_rule_version
			(tenant_id, rule_version, effective_from, effective_to, approved_by)
		 VALUES ($1, $2, now() - interval '1 hour', NULL, 'exception-ops')`,
		tenant, version); err != nil {
		t.Fatalf("发布分诊版本 %s：%v", version, err)
	}
}

func (fixture *catalogFixture) addTriageEntry(t *testing.T, tenant, version, kind, confidence, outcome string) {
	t.Helper()
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.triage_rule_entry
			(tenant_id, rule_version, signal_kind, confidence_ref, outcome)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenant, version, kind, confidence, outcome); err != nil {
		t.Fatalf("写入分诊条目：%v", err)
	}
}

func triageQuery(t *testing.T, kind, confidence string) ports.TriageQuery {
	t.Helper()
	return ports.TriageQuery{
		Kind:       projectionValue(t, domain.NewExceptionSignalKindReference, kind),
		Parcel:     projectionValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		Rule:       projectionValue(t, domain.NewSignalRuleVersionReference, "signal-rule/v1"),
		Confidence: projectionValue(t, domain.NewConfidenceReference, confidence),
	}
}

func TestMissingTriageCatalogIsNotConfigured(t *testing.T) {
	fixture := newCatalogFixture(t)
	for _, tenant := range []string{"tenant-a", ""} {
		_, configured, err := fixture.triageFor(t, tenant).
			TriageSignal(t.Context(), triageQuery(t, "CUSTOMS_HOLD", "HIGH"))
		if err != nil || configured {
			t.Fatalf("租户 %q：err=%v configured=%v", tenant, err, configured)
		}
	}
}

func TestTriageCatalogAnswersHitAndSendsMissToManualReview(t *testing.T) {
	fixture := newCatalogFixture(t)
	fixture.publishTriage(t, "tenant-a", "triage/v1")
	fixture.addTriageEntry(t, "tenant-a", "triage/v1", "CUSTOMS_HOLD", "HIGH", "AUTO_ESTABLISH")
	fixture.addTriageEntry(t, "tenant-a", "triage/v1", "CUSTOMS_HOLD", "LOW", "NO_CASE")
	view := fixture.triageFor(t, "tenant-a")

	for _, testCase := range []struct {
		confidence string
		outcome    domain.TriageOutcome
	}{
		{"HIGH", domain.AutoEstablishCase},
		{"LOW", domain.NoCaseNeeded},
	} {
		answer, configured, err := view.TriageSignal(t.Context(), triageQuery(t, "CUSTOMS_HOLD", testCase.confidence))
		if err != nil || !configured {
			t.Fatalf("可信度 %s：err=%v configured=%v", testCase.confidence, err, configured)
		}
		if answer.Outcome != testCase.outcome {
			t.Fatalf("可信度 %s 判成 %s", testCase.confidence, answer.Outcome)
		}
		if answer.Rule.String() != "triage/v1" {
			t.Fatalf("答复没带分诊规则版本：%s", answer.Rule)
		}
	}

	// 目录已配而这一类没有条目：没命中就不具备自动建案条件，进人工复核，并带上
	// 所依据的版本——不是 found=false（那会被读成「等租户登记」）。
	answer, configured, err := view.TriageSignal(t.Context(), triageQuery(t, "UNKNOWN_KIND", "HIGH"))
	if err != nil || !configured {
		t.Fatalf("未命中：err=%v configured=%v", err, configured)
	}
	if answer.Outcome != domain.ManualReviewRequired || answer.Rule.String() != "triage/v1" {
		t.Fatalf("未命中判成 %s（版本 %s）", answer.Outcome, answer.Rule)
	}
}

func TestIncompleteTriageQueryIsNotAnswered(t *testing.T) {
	fixture := newCatalogFixture(t)
	fixture.publishTriage(t, "tenant-a", "triage/v1")
	fixture.addTriageEntry(t, "tenant-a", "triage/v1", "CUSTOMS_HOLD", "HIGH", "AUTO_ESTABLISH")

	// 缺可信度：信号必须保存可信度（CONTEXT 硬句），适配器不替它补一个默认值去查目录。
	incomplete := triageQuery(t, "CUSTOMS_HOLD", "HIGH")
	incomplete.Confidence = domain.ConfidenceReference{}
	if _, configured, err := fixture.triageFor(t, "tenant-a").
		TriageSignal(t.Context(), incomplete); err != nil || configured {
		t.Fatalf("缺维查询拿到了答复：err=%v configured=%v", err, configured)
	}
}

func TestTriageRuleEntryOutcomeIsAClosedSet(t *testing.T) {
	fixture := newCatalogFixture(t)
	fixture.publishTriage(t, "tenant-a", "triage/v1")
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.triage_rule_entry
			(tenant_id, rule_version, signal_kind, confidence_ref, outcome)
		 VALUES ('tenant-a', 'triage/v1', 'CUSTOMS_HOLD', 'HIGH', 'ESCALATE')`); err == nil {
		t.Fatal("库接受了四走向之外的分诊结论")
	}
	// 条目必须挂在已发布版本下，否则「按哪版判的」就说不清。
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.triage_rule_entry
			(tenant_id, rule_version, signal_kind, confidence_ref, outcome)
		 VALUES ('tenant-a', 'triage/v9', 'CUSTOMS_HOLD', 'HIGH', 'NO_CASE')`); err == nil {
		t.Fatal("库接受了挂在未发布版本下的分诊条目")
	}
}
