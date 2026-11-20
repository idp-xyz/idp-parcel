package identity_test

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

func TestReceiptDiscrepancyIdentitiesMintPrefixedIDs(t *testing.T) {
	identities, err := identity.NewReceiptDiscrepancyIdentities()
	if err != nil {
		t.Fatal(err)
	}
	first, err := identities.NextReceiptDiscrepancyID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := identities.NextReceiptDiscrepancyID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first.String(), "RDP-") || !strings.HasPrefix(second.String(), "RDP-") || first == second {
		t.Fatalf("标识 %s、%s", first, second)
	}
	again, err := domain.NewReceiptDiscrepancyID(first.String())
	if err != nil || again != first {
		t.Fatalf("领域构造器 %s %v", first, err)
	}
}
