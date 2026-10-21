package postgres_test

import (
	"context"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// Covers: AT-CC-446（持久化半边）——登记一版凭证时交来的操作证据随行落库（customs_compliance 0026），列表读面原样透出；不记登记人
// 的一路落 NULL、读回为空；同键再登不顶替先到那一版的操作证据；空白串由库上那道 CHECK 拒。
func TestACredentialVersionKeepsItsOperationEvidence(t *testing.T) {
	registry, _, fixture := newCredentialRegistry(t)
	catalogue, err := adapter.NewCredentialCatalogue(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := viewValue(t, domain.NewTenantID, "tenant-a")
	first := "OPERATOR/https://id.syn.example/dex#SYN-CUSTOMS-01"
	attributed := synCredential(t, "SYN-HOLDER-01", 1)
	if outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterCredential(ctx, tenant, attributed, first)
	}); err != nil || outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("登记：%v %v", outcome, err)
	}
	if outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterCredential(ctx, tenant, attributed, "OPERATOR/https://id.syn.example/dex#SYN-CUSTOMS-02")
	}); err != nil || outcome != ports.CaseConfigurationAlreadyRegistered {
		t.Fatalf("同键再登：%v %v", outcome, err)
	}
	plain, err := domain.RegisterCredential(
		viewValue(t, domain.NewCredentialID, "SYN-CRED-02"), attributed.Issuer(), attributed.Holder(), attributed.Procedure(),
		attributed.ValidFrom(), attributed.ValidTo(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if outcome, err := register(t, fixture, func(ctx context.Context) (ports.CaseConfigurationSaveOutcome, error) {
		return registry.RegisterCredential(ctx, tenant, plain, "")
	}); err != nil || outcome != ports.CaseConfigurationRegistered {
		t.Fatalf("不记登记人的一路：%v %v", outcome, err)
	}

	entries, err := catalogue.ListCredentials(t.Context(), tenant, 10)
	if err != nil || len(entries) != 2 {
		t.Fatalf("上列：%d %v", len(entries), err)
	}
	if entries[0].OperationEvidence != first || entries[1].OperationEvidence != "" {
		t.Fatalf("操作证据读回走样：%q / %q", entries[0].OperationEvidence, entries[1].OperationEvidence)
	}
	var nulls int
	if err := fixture.pool.QueryRow(t.Context(), `SELECT count(*) FROM customs_compliance.regulatory_credential
		 WHERE credential_id = 'SYN-CRED-02' AND operation_evidence IS NULL`).Scan(&nulls); err != nil || nulls != 1 {
		t.Fatalf("不记登记人的一版没落成 NULL：%d %v", nulls, err)
	}
	if _, err := fixture.pool.Exec(t.Context(), `UPDATE customs_compliance.regulatory_credential
		 SET operation_evidence = ' ' WHERE credential_id = 'SYN-CRED-02'`); err == nil {
		t.Fatal("空白的操作证据溜进了凭证行")
	}
}
