package postgres_test

import (
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

func newAuthorityView(t *testing.T) (*adapter.SubmissionAuthorityView, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	view, err := adapter.NewSubmissionAuthorityView(fixture.db)
	if err != nil {
		t.Fatalf("构造授权读口：%v", err)
	}
	return view, fixture
}

func loadAuthority(t *testing.T, view *adapter.SubmissionAuthorityView, tenant, unit string) (domain.SubmissionAuthorization, bool) {
	t.Helper()
	authorization, found, err := view.LoadSubmissionAuthority(t.Context(),
		viewValue(t, domain.NewTenantID, tenant),
		viewValue(t, domain.NewDeclarationUnitID, unit))
	if err != nil {
		t.Fatalf("读授权：%v", err)
	}
	return authorization, found
}

func TestSubmissionAuthorityIsUnconfiguredWhenNothingWasGranted(t *testing.T) {
	view, _ := newAuthorityView(t)
	if _, found := loadAuthority(t, view, "tenant-a", "unit-1"); found {
		t.Fatal("没有授予却答出了授权")
	}
}

func TestGrantedSubmissionAuthorityIsReadBackWithItsBasis(t *testing.T) {
	view, fixture := newAuthorityView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.submission_authority
			(tenant_id, unit_id, authority_ref, granted_at)
		 VALUES ('tenant-a', 'unit-1', 'submit-authority/v1', $1)`, viewBaseAt)

	authorization, found := loadAuthority(t, view, "tenant-a", "unit-1")
	if !found || !authorization.Effective() {
		t.Fatalf("有效授权没读回：found=%v effective=%v", found, authorization.Effective())
	}
	if authorization.Authority().String() != "submit-authority/v1" {
		t.Fatalf("授权依据走样：%s", authorization.Authority())
	}
}

func TestRevokedSubmissionAuthorityIsReportedAsFoundButNoLongerEffective(t *testing.T) {
	view, fixture := newAuthorityView(t)
	fixture.seed(t,
		`INSERT INTO customs_compliance.submission_authority
			(tenant_id, unit_id, authority_ref, granted_at, revoked_by, revoked_at)
		 VALUES ('tenant-a', 'unit-1', 'submit-authority/v1', $1, 'MANDATE_WITHDRAWN', $2)`,
		viewBaseAt, viewBaseAt.Add(time.Hour))

	authorization, found := loadAuthority(t, view, "tenant-a", "unit-1")
	if !found {
		t.Fatal("失效授权被谎报成未配置")
	}
	if authorization.Effective() {
		t.Fatal("已撤销的授权仍报有效")
	}
	if !authorization.GrantedAt().Equal(viewBaseAt) {
		t.Fatalf("撤销抹掉了原授予时间：%s", authorization.GrantedAt())
	}
}

// 两条轨分表存放，因此一条的登记不会替另一条作答（CONTEXT「提交授权与就绪判断分别形成和失效」）。就绪在场顶替不了
// 授权，这一条把它钉在读口层面。
func TestReadinessAndSubmissionAuthorityAnswerIndependently(t *testing.T) {
	fixture := newViewFixture(t)
	readiness, err := adapter.NewReadinessView(fixture.db)
	if err != nil {
		t.Fatalf("构造就绪读口：%v", err)
	}
	authority, err := adapter.NewSubmissionAuthorityView(fixture.db)
	if err != nil {
		t.Fatalf("构造授权读口：%v", err)
	}
	// 只登记就绪，不登记授权。
	fixture.seed(t,
		`INSERT INTO customs_compliance.readiness_judgment
			(tenant_id, unit_id, basis_ref, judged_at)
		 VALUES ('tenant-a', 'unit-1', 'readiness-check/v1', $1)`, viewBaseAt)

	if _, found := loadReadiness(t, readiness, "tenant-a", "unit-1"); !found {
		t.Fatal("就绪没读回")
	}
	if _, found := loadAuthority(t, authority, "tenant-a", "unit-1"); found {
		t.Fatal("就绪的登记替授权作了答")
	}
}

func TestSubmissionAuthorityChecksPinTheRevocationShape(t *testing.T) {
	_, fixture := newAuthorityView(t)
	fixture.rejects(t, "只有撤销时间没有撤销原因",
		`INSERT INTO customs_compliance.submission_authority
			(tenant_id, unit_id, authority_ref, granted_at, revoked_at)
		 VALUES ('t', 'u', 'a', now(), now())`)
	fixture.rejects(t, "撤销早于授予",
		`INSERT INTO customs_compliance.submission_authority
			(tenant_id, unit_id, authority_ref, granted_at, revoked_by, revoked_at)
		 VALUES ('t', 'u', 'a', $1, 'CAUSE', $2)`, viewBaseAt, viewBaseAt.Add(-time.Hour))
}
