package postgres_test

import (
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

func newSubmissionIndex(t *testing.T) (*adapter.SubmissionIndex, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	index, err := adapter.NewSubmissionIndex(fixture.db)
	if err != nil {
		t.Fatalf("构造提交索引：%v", err)
	}
	return index, fixture
}

// seedSubmission 直接写提交行：本索引读的是申报链自己的表，因此夹具也从那张表播种。
// is_current 显式给 true——迁移 0012 撤掉了列默认，播种方与写入方同责。
func seedSubmission(t *testing.T, fixture *viewFixture, tenant, unit, version string) {
	t.Helper()
	fixture.seed(t,
		`INSERT INTO customs_compliance.declaration_submission
			(tenant_id, unit_id, procedure_ref, version_id, content_digest,
			 members, dossier_ref, roles_ref, readiness_basis, authority_ref,
			 is_current, fixed_at, recorded_at)
		 VALUES ($1, $2, 'EXPORT/GENERAL', $3, 'digest-1',
			 '["parcel-1"]', 'dossier/v1', 'roles/v1', 'readiness/v1', 'authority/v1',
			 true, $4, $4)`,
		tenant, unit, version, viewBaseAt)
}

func TestSubmissionIndexFindsAVersionThatWasActuallySubmitted(t *testing.T) {
	index, fixture := newSubmissionIndex(t)
	seedSubmission(t, fixture, "tenant-a", "unit-1", "version-1")

	found, err := index.FindSubmission(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"),
		viewValue(t, domain.NewSubmissionVersionID, "version-1"))
	if err != nil || !found {
		t.Fatalf("已提交版本查不到：err=%v found=%v", err, found)
	}
}

// 归属不上的响应必须答 false 而不是报错：留存不猜那条路要走得通。
func TestSubmissionIndexReportsAnUnknownVersionAsNotFound(t *testing.T) {
	index, fixture := newSubmissionIndex(t)
	seedSubmission(t, fixture, "tenant-a", "unit-1", "version-1")

	found, err := index.FindSubmission(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-a"),
		viewValue(t, domain.NewSubmissionVersionID, "version-nope"))
	if err != nil || found {
		t.Fatalf("不存在的版本被认了：err=%v found=%v", err, found)
	}
}

func TestSubmissionIndexDoesNotSeeAnotherTenantsVersion(t *testing.T) {
	index, fixture := newSubmissionIndex(t)
	seedSubmission(t, fixture, "tenant-a", "unit-1", "version-1")

	found, err := index.FindSubmission(t.Context(),
		viewValue(t, domain.NewTenantID, "tenant-b"),
		viewValue(t, domain.NewSubmissionVersionID, "version-1"))
	if err != nil || found {
		t.Fatalf("跨租户可见：err=%v found=%v", err, found)
	}
}
