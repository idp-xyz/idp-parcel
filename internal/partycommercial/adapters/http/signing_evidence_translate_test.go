package commercialhttp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func TestSigningEvidencePayloadRefusesSelfReportedIdentity(t *testing.T) {
	if err := refuseSelfReportedRecorder(json.RawMessage(`"op-1"`)); err == nil || !strings.Contains(err.Error(), "recordedBy") {
		t.Fatalf("recordedBy err = %v", err)
	}
	if err := refuseSelfReportedTenant(json.RawMessage(`"tenant-1"`)); err == nil {
		t.Fatal("tenantId 在场却被收下了")
	}
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	recorder, err := domain.NewOperatorSubjectReference("op-1")
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	at := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	evidence, err := signingEvidenceFromDocument(signingEvidenceDocument{
		EvidenceID: "evidence-1", Kind: "CUSTOMER_CONTRACT", ObjectID: "contract-1", Version: "v1",
		SignedOn: "2026-11-01", FileName: "signed.pdf", SHA256: strings.Repeat("ab", 32),
		LibraryIndex: "EVD-1", Source: "OFFLINE_SIGNED", RecordedAt: &at,
	}, tenant, recorder)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if evidence.Tenant().String() != "tenant-1" || evidence.RecordedBy().String() != "op-1" {
		t.Fatalf("身份没有取调用方给的：%s %s", evidence.Tenant(), evidence.RecordedBy())
	}
	if _, err := signingEvidenceFromDocument(signingEvidenceDocument{
		EvidenceID: "evidence-1", Kind: "CREDIT_POLICY", ObjectID: "contract-1", Version: "v1",
		SignedOn: "2026-11-01", FileName: "signed.pdf", SHA256: strings.Repeat("ab", 32),
		LibraryIndex: "EVD-1", Source: "OFFLINE_SIGNED", RecordedAt: &at,
	}, tenant, recorder); err == nil {
		t.Fatal("别的类别被收下了")
	}

	if _, err := signingEvidenceRequirementFromDocument(signingEvidenceRequirementDocument{}, tenant); err == nil {
		t.Fatal("两个布尔缺席却被收下了")
	}
	contract, agreement := false, true
	rule, err := signingEvidenceRequirementFromDocument(signingEvidenceRequirementDocument{
		RequiresCustomerContract: &contract, RequiresSupplierAgreement: &agreement,
	}, tenant)
	if err != nil || rule.RequiresCustomerContract() || !rule.RequiresSupplierAgreement() {
		t.Fatalf("要求 = %v %v", rule, err)
	}
}
