package postgres_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var caseBaseAt = time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)

type caseFixture struct {
	pool *pgxpool.Pool
	db   *bentopg.DB
}

func newCaseFixture(t *testing.T) *caseFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return &caseFixture{pool: pool, db: db}
}

// viewFor 按给定租户串装配在场视图；空串即「租户未登记」。
func (fixture *caseFixture) viewFor(t *testing.T, tenant string) *adapter.ExceptionCases {
	t.Helper()
	var tenantID domain.TenantID
	if tenant != "" {
		tenantID = projectionValue(t, domain.NewTenantID, tenant)
	}
	view, err := adapter.NewExceptionCases(fixture.db, tenantID)
	if err != nil {
		t.Fatalf("构造在场视图：%v", err)
	}
	return view
}

// 案件仓储尚未落地（ExceptionCase 还没有快照/重建构造器），因此用例直接写行——
// 证的是本适配器读得对、库的 CHECK 守得住，不是某个还不存在的写入方。
func (fixture *caseFixture) insertCase(t *testing.T, tenant, caseID, phase string) {
	t.Helper()
	var firstResponse, closedAt *time.Time
	var conclusion *string
	switch phase {
	case "IN_PROGRESS":
		at := caseBaseAt.Add(time.Hour)
		firstResponse = &at
	case "CLOSED":
		at := caseBaseAt.Add(2 * time.Hour)
		closedAt = &at
		reason := "RESOLVED"
		conclusion = &reason
	}
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.exception_case
			(tenant_id, case_id, root_parcel, impact_scope, responsible_team,
			 phase, established_at, first_response, closed_at, conclusion)
		 VALUES ($1, $2, 'parcel-1', 'scope/v1', 'team-a', $3, $4, $5, $6, $7)`,
		tenant, caseID, phase, caseBaseAt, firstResponse, closedAt, conclusion); err != nil {
		t.Fatalf("写入案件 %s/%s：%v", tenant, phase, err)
	}
}

func TestCaseActiveFollowsPhase(t *testing.T) {
	fixture := newCaseFixture(t)
	view := fixture.viewFor(t, "tenant-a")

	for _, testCase := range []struct {
		phase  string
		active bool
	}{
		{"AWAITING_RESPONSE", true},
		{"IN_PROGRESS", true},
		{"CLOSED", false},
	} {
		caseID := "case-" + testCase.phase
		fixture.insertCase(t, "tenant-a", caseID, testCase.phase)

		active, found, err := view.CaseActive(t.Context(),
			projectionValue(t, domain.NewCaseID, caseID))
		if err != nil || !found {
			t.Fatalf("%s：err=%v found=%v", testCase.phase, err, found)
		}
		if active != testCase.active {
			t.Fatalf("%s 的在场判据 = %v，应为 %v", testCase.phase, active, testCase.active)
		}
	}
}

func TestAbsentCaseAndUnregisteredTenantAreBothNotFound(t *testing.T) {
	fixture := newCaseFixture(t)
	fixture.insertCase(t, "tenant-a", "case-1", "IN_PROGRESS")

	// 案件不在场：处置请求挂不上去，这正是安全方向。
	if _, found, err := fixture.viewFor(t, "tenant-a").CaseActive(t.Context(),
		projectionValue(t, domain.NewCaseID, "case-unknown")); err != nil || found {
		t.Fatalf("不存在的案件被认作在场：err=%v found=%v", err, found)
	}
	// 租户未登记：今天没有租户，因此任何案件都不在场，且不得报成依赖故障。
	if _, found, err := fixture.viewFor(t, "").CaseActive(t.Context(),
		projectionValue(t, domain.NewCaseID, "case-1")); err != nil || found {
		t.Fatalf("未登记租户下案件可见：err=%v found=%v", err, found)
	}
	// 跨租户不可见（ADR-0003）。
	if _, found, err := fixture.viewFor(t, "tenant-b").CaseActive(t.Context(),
		projectionValue(t, domain.NewCaseID, "case-1")); err != nil || found {
		t.Fatalf("跨租户案件可见：err=%v found=%v", err, found)
	}
}

func TestExceptionCaseChecksGuardTheAggregateInvariants(t *testing.T) {
	fixture := newCaseFixture(t)

	for _, testCase := range []struct {
		name   string
		values string
	}{
		{
			"已关闭却没有结论",
			`'t', 'c1', 'p', 's', 'team', 'CLOSED', now(), NULL, now(), NULL, NULL`,
		},
		{
			"未关闭却带了结论",
			`'t', 'c2', 'p', 's', 'team', 'IN_PROGRESS', now(), now(), NULL, 'RESOLVED', NULL`,
		},
		{
			"处理中却没有首次响应时间",
			`'t', 'c3', 'p', 's', 'team', 'IN_PROGRESS', now(), NULL, NULL, NULL, NULL`,
		},
		{
			"待响应却已有首次响应时间",
			`'t', 'c4', 'p', 's', 'team', 'AWAITING_RESPONSE', now(), now(), NULL, NULL, NULL`,
		},
		{
			"归并进自己",
			`'t', 'c5', 'p', 's', 'team', 'CLOSED', now(), NULL, now(), 'MERGED', 'c5'`,
		},
		{
			"归并却没有关闭",
			`'t', 'c6', 'p', 's', 'team', 'IN_PROGRESS', now(), now(), NULL, NULL, 'c-main'`,
		},
		{
			"三态之外的主状态",
			`'t', 'c7', 'p', 's', 'team', 'ESCALATED', now(), now(), NULL, NULL, NULL`,
		},
		{
			"关闭时间早于建立",
			`'t', 'c8', 'p', 's', 'team', 'CLOSED', now(), NULL, now() - interval '1 day', 'RESOLVED', NULL`,
		},
	} {
		if _, err := fixture.pool.Exec(t.Context(),
			`INSERT INTO visibility_exception.exception_case
				(tenant_id, case_id, root_parcel, impact_scope, responsible_team,
				 phase, established_at, first_response, closed_at, conclusion, merged_into)
			 VALUES (`+testCase.values+`)`); err == nil {
			t.Fatalf("库接受了「%s」的案件行", testCase.name)
		}
	}
}
