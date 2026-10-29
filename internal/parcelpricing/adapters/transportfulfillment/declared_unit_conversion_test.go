package transportfulfillment_test

import (
	"testing"
	"time"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// kilogramTablePlan 是一张按 KG 查表、0.5 KG 向上进位的合成价卡，[0,1) 5、[1,3) 9、[3,100) 15。
func kilogramTablePlan(t *testing.T) ppdomain.PricingPlanVersion {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	decimal := func(raw string) ppdomain.Decimal {
		value, err := ppdomain.ParseDecimal(raw)
		must(err)
		return value
	}
	weightOf := func(raw string) ppdomain.Weight {
		value, err := ppdomain.NewWeight(decimal(raw), ppdomain.WeightUnitKilogram)
		must(err)
		return value
	}
	currency, err := ppdomain.NewCurrency("USD")
	must(err)
	var entries []ppdomain.RateEntry
	for index, band := range [][3]string{{"0", "1", "5"}, {"1", "3", "9"}, {"3", "100", "15"}} {
		id, err := ppdomain.NewRateEntryID("entry-" + string(rune('a'+index)))
		must(err)
		amount, err := ppdomain.NewMoney(decimal(band[2]), currency)
		must(err)
		entry, err := ppdomain.NewRateEntry(id, "Z1", weightOf(band[0]), weightOf(band[1]), amount)
		must(err)
		entries = append(entries, entry)
	}
	period, err := ppdomain.NewEffectivePeriod(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	must(err)
	reference := func(kind ppdomain.ArtifactKind, id string) ppdomain.VersionReference {
		value, err := ppdomain.NewVersionReferenceIdentity(kind, id, "v1")
		must(err)
		return value
	}
	table, err := ppdomain.NewRateTableVersion(reference(ppdomain.ArtifactRateTable, "table-1"), ppdomain.RateTableFamilyWeightZone, currency, ppdomain.WeightUnitKilogram, period, entries)
	must(err)
	rounding, err := ppdomain.NewWeightRoundingPolicy(ppdomain.RoundingCeiling, weightOf("0.5"))
	must(err)
	policy, err := ppdomain.NewPricingWeightPolicy(reference(ppdomain.ArtifactWeightPolicy, "weight-1"), ppdomain.PricingWeightActualOnly, rounding, nil)
	must(err)
	scope, err := ppdomain.NewPricingScopeID("SYN-SCOPE")
	must(err)
	code, err := ppdomain.NewChargeCode("BASE_FREIGHT")
	must(err)
	plan, err := ppdomain.NewPricingPlanVersion(reference(ppdomain.ArtifactPricingPlan, "plan-1"), scope,
		ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge, code, period, table, policy, nil, ppdomain.PricingPlanStructures{})
	must(err)
	return plan
}

// Covers: ADR-0346 决定一——申报单位是磅、价卡按千克查：解析器把 5 LB 原样（不换算）交进输入快照，
// 评价那一层换成 2.26796185 KG、进位到 2.5 KG、落 [1,3) 档；此前这一格答单位不一致冲突。
func TestADeclaredPoundWeightReachesTheEvaluationUnconvertedAndIsPricedInKilograms(t *testing.T) {
	resolver := resolverWith(t, membersOf(t, objectRef), declaredUnit(t, "5", "LB"), destinationPostal(t, "100115"), nil)
	resolved := resolve(t, resolver, ppdomain.PricingPurposeCustomerCharge)
	if weight := resolved.Input.ActualWeight(); weight.Value().String() != "5" || weight.Unit() != ppdomain.WeightUnitPound {
		t.Fatalf("解析器不该换算：%s %s", weight.Value(), weight.Unit())
	}
	if resolved.Outcome != ppports.PricingInputResolved {
		t.Fatalf("outcome %s", resolved.Outcome)
	}

	// 解析器给的是邮编路线、分区要靠绑了目录的卡去解；这张合成卡没绑目录，所以只补一个分区，其余（实重、主体、
	// 业务时点）都取解析器交出的那一份。
	subject, _ := resolved.Input.Subject()
	input, err := ppdomain.NewPricingInputSnapshot(resolved.Input.TenantID(), resolved.Input.Scope(), subject, "Z1",
		resolved.Input.ActualWeight(), nil, resolved.Input.BusinessAt())
	if err != nil {
		t.Fatal(err)
	}
	id, err := ppdomain.NewEvaluationID("eval-declared-lb")
	if err != nil {
		t.Fatal(err)
	}
	request, err := ppdomain.NewEvaluationRequest(id, kilogramTablePlan(t), input, ppdomain.EvidenceSynthetic)
	if err != nil {
		t.Fatal(err)
	}
	evaluation := ppdomain.EvaluatePricing(request)
	if evaluation.Status() != ppdomain.EvaluationCompleted {
		t.Fatalf("status %s issues %v", evaluation.Status(), evaluation.Issues())
	}
	if total, _ := evaluation.Total(); total.Amount().String() != "9" {
		t.Fatalf("total = %s, want 9", total.Amount())
	}
	steps := evaluation.UnitConversions()
	if len(steps) != 1 || steps[0].FromUnit() != "LB" || steps[0].ToUnit() != "KG" || steps[0].Converted()[0].String() != "2.26796185" {
		t.Fatalf("steps = %+v", steps)
	}
}
