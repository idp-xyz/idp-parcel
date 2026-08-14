package postgres_test

import (
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

func newReadinessView(t *testing.T) (*adapter.ReadinessView, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	view, err := adapter.NewReadinessView(fixture.db)
	if err != nil {
		t.Fatalf("构造就绪读口：%v", err)
	}
	return view, fixture
}

func loadReadiness(t *testing.T, view *adapter.ReadinessView, tenant, unit string) (domain.ReadinessJudgment, bool) {
	t.Helper()
	judgment, found, err := view.LoadReadiness(t.Context(),
		viewValue(t, domain.NewTenantID, tenant),
		viewValue(t, domain.NewDeclarationUnitID, unit))
	if err != nil {
		t.Fatalf("读就绪：%v", err)
	}
	return judgment, found
}

// 未登记就是未配置：编排据此停在未决，绝不能被读成「已就绪」。
func TestReadinessIsUnconfiguredWhenNothingWasRegistered(t *testing.T) {
	view, _ := newReadinessView(t)
	if _, found := loadReadiness(t, view, "tenant-a", "unit-1"); found {
		t.Fatal("没有登记却答出了就绪判断")
	}
}

func TestReadinessInForceIsReadBackWithItsBasis(t *testing.T) {
	view, fixture := newReadinessView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.readiness_judgment
			(tenant_id, unit_id, basis_ref, judged_at)
		 VALUES ('tenant-a', 'unit-1', 'readiness-check/v1', $1)`, viewBaseAt)

	judgment, found := loadReadiness(t, view, "tenant-a", "unit-1")
	if !found || !judgment.Effective() {
		t.Fatalf("有效就绪没读回：found=%v effective=%v", found, judgment.Effective())
	}
	if judgment.Basis().String() != "readiness-check/v1" || !judgment.JudgedAt().Equal(viewBaseAt) {
		t.Fatalf("依据或时间走样：basis=%s judgedAt=%s", judgment.Basis(), judgment.JudgedAt())
	}
}

// 这一格是本适配器存在的理由：`不再就绪`必须以 found=true + 失效读回。谎报成未配置
// 会把「重新取得就绪」错指成「等实例参数」；谎报成仍有效则直接放行一次不该发生的提交。
func TestRevokedReadinessIsReportedAsFoundButNoLongerEffective(t *testing.T) {
	view, fixture := newReadinessView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.readiness_judgment
			(tenant_id, unit_id, basis_ref, judged_at, revoked_by, revoked_at)
		 VALUES ('tenant-a', 'unit-1', 'readiness-check/v1', $1, 'DOSSIER_CHANGED', $2)`,
		viewBaseAt, viewBaseAt.Add(time.Hour))

	judgment, found := loadReadiness(t, view, "tenant-a", "unit-1")
	if !found {
		t.Fatal("失效就绪被谎报成未配置")
	}
	if judgment.Effective() {
		t.Fatal("已撤销的就绪仍报有效")
	}
	// 原判断保留：撤销不是删除。
	if judgment.Basis().String() != "readiness-check/v1" || !judgment.JudgedAt().Equal(viewBaseAt) {
		t.Fatalf("撤销抹掉了原判断：basis=%s judgedAt=%s", judgment.Basis(), judgment.JudgedAt())
	}
}

func TestReadinessOfAnotherTenantIsInvisible(t *testing.T) {
	view, fixture := newReadinessView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.readiness_judgment
			(tenant_id, unit_id, basis_ref, judged_at)
		 VALUES ('tenant-a', 'unit-1', 'readiness-check/v1', $1)`, viewBaseAt)

	if _, found := loadReadiness(t, view, "tenant-b", "unit-1"); found {
		t.Fatal("跨租户可见")
	}
}

func TestReadinessChecksPinTheRevocationShape(t *testing.T) {
	_, fixture := newReadinessView(t)
	fixture.rejects(t, "只有撤销原因没有撤销时间",
		`INSERT INTO customs_compliance.readiness_judgment
			(tenant_id, unit_id, basis_ref, judged_at, revoked_by)
		 VALUES ('t', 'u', 'b', now(), 'CAUSE')`)
	fixture.rejects(t, "撤销早于形成",
		`INSERT INTO customs_compliance.readiness_judgment
			(tenant_id, unit_id, basis_ref, judged_at, revoked_by, revoked_at)
		 VALUES ('t', 'u', 'b', $1, 'CAUSE', $2)`, viewBaseAt, viewBaseAt.Add(-time.Hour))
}
