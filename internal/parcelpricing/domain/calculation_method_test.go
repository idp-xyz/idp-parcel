package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// Covers: CONTEXT「计算方法」—「一条评价费用行金额的产生方式。首发取值闭合，只有定额、
// 查表、按基数百分比，以及在其中两者之间取较大值」：它只定义一次，覆盖任意评价费用行。
// 从表里查出的基础费率与从表里查出的附加费是同一种方法；两套并行的取值集合会让同一个词
// 各自漂移，使两份解释无法互相对读。
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
