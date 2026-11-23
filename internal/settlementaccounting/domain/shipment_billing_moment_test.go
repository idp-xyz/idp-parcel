package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func TestShipmentBillingMomentNamesEveryBuiltInValue(t *testing.T) {
	if got := domain.ShipmentBillingMomentInvalid.String(); got != "" {
		t.Fatalf("零值 = %q", got)
	}
	for _, moment := range []domain.ShipmentBillingMoment{domain.ShipmentMembersIntakeAdopted} {
		if moment.String() == "" {
			t.Fatalf("时点 %d 没有词", moment)
		}
		parsed, err := domain.ShipmentBillingMomentFromName(moment.String())
		if err != nil || parsed != moment {
			t.Fatalf("译回 %s：%v %v", moment, parsed, err)
		}
	}
	if _, err := domain.ShipmentBillingMomentFromName("NETWORK_INTAKE_ADOPTED"); err == nil {
		t.Fatal("包裹计费时点的词不该被认成委托级时点")
	}
	if _, err := domain.ShipmentBillingMomentFromName("SHIPMENT_ACCEPTED"); err == nil {
		t.Fatal("委托已接受不该是计费时点")
	}
}
