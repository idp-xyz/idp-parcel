package partycommercial_test

import (
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/partycommercial"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// Covers: 显式未配置——对任何键都答 found=false 且不报错，编排据此停在 CONTRACT_UNCONFIGURED 未决，
// 不默认可回收。
func TestTheUnconfiguredContractResponsibilityAnswersNotConfigured(t *testing.T) {
	tenant, _ := domain.NewTenantID("tenant-a")
	customer, _ := domain.NewRecoveryCustomerReference("customer-1")
	obligation, _ := domain.NewTaxObligationReference("duty-1")

	_, configured, err := adapter.UnconfiguredContractResponsibility{}.LoadContractResponsibility(
		t.Context(), tenant, customer, obligation)
	if err != nil {
		t.Fatalf("err = %v, want nil——未配置不是故障", err)
	}
	if configured {
		t.Fatal("未配置的合同责任目录答成了已配置")
	}
}
