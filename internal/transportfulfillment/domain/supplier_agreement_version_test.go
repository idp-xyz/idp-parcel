package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: 票 demo-chain-gaps/22——协议版本两格分开收、原样交回，任一格空或含 NUL 都在构造期拒（登记方改输入，
// 不落到核对口的未决）。
func TestASupplierAgreementVersionKeepsItsTwoPartsApart(t *testing.T) {
	agreement, err := domain.NewSupplierAgreementVersion("SYN-SUPPLIER-TRUNK-CN-SG-01", "v1")
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if agreement.Object() != "SYN-SUPPLIER-TRUNK-CN-SG-01" || agreement.Version() != "v1" {
		t.Fatalf("两格 = %q / %q", agreement.Object(), agreement.Version())
	}

	for name, parts := range map[string][2]string{
		"对象空":    {" ", "v1"},
		"版本空":    {"SYN-SUPPLIER-TRUNK-CN-SG-01", ""},
		"对象含NUL": {"SYN-\x00", "v1"},
	} {
		_, err := domain.NewSupplierAgreementVersion(parts[0], parts[1])
		if !errors.Is(err, domain.ErrBlankValue) && !errors.Is(err, domain.ErrValueContainsNUL) {
			t.Fatalf("%s：err = %v，want 构造期拒", name, err)
		}
	}
}
