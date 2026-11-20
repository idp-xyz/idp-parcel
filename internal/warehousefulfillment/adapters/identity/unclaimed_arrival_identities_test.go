package identity_test

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

func TestUnclaimedArrivalIdentitiesMintPrefixedIDs(t *testing.T) {
	identities, err := identity.NewUnclaimedArrivalIdentities()
	if err != nil {
		t.Fatal(err)
	}
	first, err := identities.NextUnclaimedArrivalID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := identities.NextUnclaimedArrivalID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first.String(), "UCA-") || !strings.HasPrefix(second.String(), "UCA-") || first == second {
		t.Fatalf("标识 %s、%s", first, second)
	}
	again, err := domain.NewUnclaimedArrivalID(first.String())
	if err != nil || again != first {
		t.Fatalf("领域构造器 %s %v", first, err)
	}
}
