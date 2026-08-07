package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// CONTEXT defines 计算方法 once, over any evaluation charge line, with a closed
// set of four values. A base rate read from a table and a surcharge read from a
// table are the same method; two separate value sets would let the same words
// drift apart and make one explanation unreadable against another.
func TestOneCalculationMethodSetSpansBaseLinesAndSurchargeRules(t *testing.T) {
	plan := syntheticPlan(t, "method-set", domain.PricingDirectionSell, domain.PricingPurposeCustomerCharge, "10", domain.PricingWeightActualOnly, nil)
	baseLine := evaluate(t, "eval-method-set", plan, syntheticInput(t, "1", "Z1")).ChargeLines()[0]
	if baseLine.Method() != domain.ChargeMethodTableLookup {
		t.Fatalf("base line method = %q, want the shared table lookup method", baseLine.Method())
	}
	if tableLookupCalculation(t).Method() != domain.ChargeMethodTableLookup {
		t.Fatal("a surcharge read from a table names a different method than a base rate read from a table")
	}
	if fixedAmountCalculation(t, "10").Method() != domain.ChargeMethodFixedAmount {
		t.Fatal("a fixed-amount surcharge names a different method than a fixed charge line")
	}
}
