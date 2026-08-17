package partycommercial

import (
	"errors"
	"testing"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func TestUnknownServiceProductFormIsNotAbsorbed(t *testing.T) {
	_, err := translateForm(pcdomain.ServiceProductForm(255))
	if !errors.Is(err, ErrUntranslatableAnswer) {
		t.Fatalf("unknown form err = %v, want ErrUntranslatableAnswer — default 吸收了新形态", err)
	}
}
