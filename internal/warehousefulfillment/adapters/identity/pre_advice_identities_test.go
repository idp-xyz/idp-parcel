package identity_test

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

func TestPreAdviceIdentitiesMintPrefixedIDs(t *testing.T) {
	identities, err := identity.NewPreAdviceIdentities()
	if err != nil {
		t.Fatal(err)
	}
	first, err := identities.NextPreAdviceID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := identities.NextPreAdviceID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first.String(), "IPA-") || !strings.HasPrefix(second.String(), "IPA-") || first == second {
		t.Fatalf("标识 %s、%s", first, second)
	}
	again, err := domain.NewPreAdviceID(first.String())
	if err != nil || again != first {
		t.Fatalf("领域构造器 %s %v", first, err)
	}
}
