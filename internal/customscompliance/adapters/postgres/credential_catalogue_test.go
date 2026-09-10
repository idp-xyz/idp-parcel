package postgres_test

import (
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

func newCredentialCatalogue(t *testing.T) (*adapter.CredentialCatalogue, *viewFixture) {
	t.Helper()
	fixture := newViewFixture(t)
	catalogue, err := adapter.NewCredentialCatalogue(fixture.db)
	if err != nil {
		t.Fatalf("构造凭证册读口：%v", err)
	}
	return catalogue, fixture
}

func listCredentials(
	t *testing.T,
	catalogue *adapter.CredentialCatalogue,
	tenant string,
	limit int,
) []ports.CredentialCatalogueEntry {
	t.Helper()
	entries, err := catalogue.ListCredentials(t.Context(),
		viewValue(t, domain.NewTenantID, tenant), limit)
	if err != nil {
		t.Fatalf("上列凭证册：%v", err)
	}
	return entries
}

// Covers: 票 sa-cc/10 重点 ⑤ — 必填口构造期拒 nil，不等到第一个请求才发现装配错了。
func TestCredentialCatalogueRefusesANilDB(t *testing.T) {
	if _, err := adapter.NewCredentialCatalogue(nil); err == nil {
		t.Fatal("nil db 被接受了")
	}
}

// Covers: ADR-0077 Decision 四 — 空册如实答空列表，不是错误也不折成未配置。
func TestEmptyCredentialRegisterAnswersEmptyList(t *testing.T) {
	catalogue, _ := newCredentialCatalogue(t)
	if entries := listCredentials(t, catalogue, "tenant-a", 10); len(entries) != 0 {
		t.Fatalf("空库上列出 %d 版凭证", len(entries))
	}
}

// Covers: 0014 自注 — 一行即一版不可变凭证，七件逐格如实读回；uses 为零译成「来源未
// 提供」而不是「零次可用」；登记时间与有效期是两回事各自透出；跨租户不可见；按凭证
// 身份排序稳定。
func TestCredentialCatalogueListsRegisteredVersionsPerTenant(t *testing.T) {
	catalogue, fixture := newCredentialCatalogue(t)
	validTo := viewBaseAt.Add(30 * 24 * time.Hour)
	registeredAt := viewBaseAt.Add(-time.Hour)
	fixture.seed(t,
		`INSERT INTO customs_compliance.regulatory_credential
			(tenant_id, credential_id, issuer_ref, holder_ref, procedure_ref,
			 valid_from, valid_to, uses, registered_at)
		 VALUES
			('tenant-a', 'SYN-CRED-02', 'SYN-AUTHORITY-01', 'SYN-HOLDER-02', 'SYN-PROC-01', $1, $2, 0, $3),
			('tenant-a', 'SYN-CRED-01', 'SYN-AUTHORITY-01', 'SYN-HOLDER-01', 'SYN-PROC-01', $1, $2, 3, $3),
			('tenant-b', 'SYN-CRED-09', 'SYN-AUTHORITY-09', 'SYN-HOLDER-09', 'SYN-PROC-09', $1, $2, 1, $3)`,
		viewBaseAt, validTo, registeredAt)

	entries := listCredentials(t, catalogue, "tenant-a", 10)
	if len(entries) != 2 {
		t.Fatalf("上列了 %d 版凭证，要 2 版：%+v", len(entries), entries)
	}
	quota := entries[0].Credential
	if quota.ID().String() != "SYN-CRED-01" || quota.Issuer().String() != "SYN-AUTHORITY-01" ||
		quota.Holder().String() != "SYN-HOLDER-01" || quota.Procedure().String() != "SYN-PROC-01" ||
		!quota.ValidFrom().Equal(viewBaseAt) || !quota.ValidTo().Equal(validTo) {
		t.Fatalf("首版凭证七件走样：%+v", quota)
	}
	if uses, provided := quota.Uses(); !provided || uses != 3 {
		t.Fatalf("有额度的凭证读成未提供：uses=%d provided=%v", uses, provided)
	}
	if !entries[0].RegisteredAt.Equal(registeredAt) {
		t.Fatalf("登记时间走样（它是登记动作的时钟，不是有效期起点）：%v", entries[0].RegisteredAt)
	}

	// uses 为零在领域约定为「来源未提供次数额度」，读口把它翻成第二个返回值为假，
	// 不让消费方把「未提供」读成「额度已用尽」。
	unprovided := entries[1].Credential
	if unprovided.ID().String() != "SYN-CRED-02" {
		t.Fatalf("凭证身份序走样：%+v", unprovided)
	}
	if uses, provided := unprovided.Uses(); provided || uses != 0 {
		t.Fatalf("未提供额度的凭证读成有额度：uses=%d provided=%v", uses, provided)
	}
	for _, entry := range entries {
		if entry.Credential.ID().String() == "SYN-CRED-09" {
			t.Fatal("跨租户可见")
		}
	}
}

// Covers: ADR-0077 Decision 五 — limit 非正拒；limit 截断行数而不是静默全量。
func TestCredentialCatalogueGuardsItsLimit(t *testing.T) {
	catalogue, fixture := newCredentialCatalogue(t)
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	if _, err := catalogue.ListCredentials(t.Context(), tenant, 0); err == nil {
		t.Fatal("limit=0 的凭证上列被接受了")
	}
	if _, err := catalogue.ListCredentials(t.Context(), tenant, -1); err == nil {
		t.Fatal("limit=-1 的凭证上列被接受了")
	}

	fixture.seed(t,
		`INSERT INTO customs_compliance.regulatory_credential
			(tenant_id, credential_id, issuer_ref, holder_ref, procedure_ref,
			 valid_from, valid_to, uses, registered_at)
		 VALUES
			('tenant-a', 'SYN-CRED-01', 'SYN-AUTHORITY-01', 'SYN-HOLDER-01', 'SYN-PROC-01', $1, $2, 0, $1),
			('tenant-a', 'SYN-CRED-02', 'SYN-AUTHORITY-01', 'SYN-HOLDER-01', 'SYN-PROC-01', $1, $2, 0, $1),
			('tenant-a', 'SYN-CRED-03', 'SYN-AUTHORITY-01', 'SYN-HOLDER-01', 'SYN-PROC-01', $1, $2, 0, $1)`,
		viewBaseAt, viewBaseAt.Add(24*time.Hour))
	if entries := listCredentials(t, catalogue, "tenant-a", 2); len(entries) != 2 {
		t.Fatalf("limit=2 却上列了 %d 版", len(entries))
	}
}
