package ports_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func TestSellPriceCardPathOutcomeNamesEachValue(t *testing.T) {
	if got := ports.SellPriceCardPathOutcomeInvalid.String(); got != "" {
		t.Fatalf("零值 = %q", got)
	}
	for _, outcome := range []ports.SellPriceCardPathOutcome{
		ports.SellPriceCardNotConfigured, ports.SellPriceCardInForce, ports.SellPriceCardConflict,
	} {
		if outcome.String() == "" {
			t.Fatalf("结果 %d 没有词", outcome)
		}
	}
}
