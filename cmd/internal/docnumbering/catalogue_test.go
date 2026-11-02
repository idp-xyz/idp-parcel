package docnumbering

import (
	"testing"

	"go.idp.xyz/idp-platform-go/numbering"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
)

// Covers: ADR-0355 决定二、五、六——汇总出的恰是决定二那十一类，每类的默认模板与重置周期照决定五那张表，且各上下文
// 的声明汇得起来（代码不重、默认模板过模板检查、不用加扰）。表在这里抄一遍是证据不是第二份口径：声明改了而 ADR 没改，
// 这里会红，提醒改的人先去改决定。
func TestCatalogueHoldsTheElevenDocumentTypesOfTheDecision(t *testing.T) {
	want := map[docnumber.TypeCode]struct {
		template string
		reset    numbering.Granularity
		zone     docnumber.DateTimeZoneSource
	}{
		"SHIPMENT":               {"P{yyyy}{mm}{dd}{seq:6}", numbering.Daily, docnumber.DateTimeZoneFromTenant},
		"CUSTOMER_STATEMENT":     {"STMT-{yyyy}{mm}-{seq:5}", numbering.Monthly, docnumber.DateTimeZoneFromIssuer},
		"STATEMENT_DISPUTE":      {"DSP-{yyyy}{mm}-{seq:5}", numbering.Monthly, docnumber.DateTimeZoneFromIssuer},
		"AUDITED_PAYABLE":        {"AP-{yyyy}{mm}-{seq:5}", numbering.Monthly, docnumber.DateTimeZoneFromIssuer},
		"EXCEPTION_CASE":         {"EX-{yyyy}{mm}{dd}-{seq:4}", numbering.Daily, docnumber.DateTimeZoneFromTenant},
		"CLAIM":                  {"CLM-{yyyy}{mm}-{seq:5}", numbering.Monthly, docnumber.DateTimeZoneFromTenant},
		"CUSTOMS_CASE":           {"CC-{yyyy}{mm}{dd}-{seq:4}", numbering.Daily, docnumber.DateTimeZoneFromTenant},
		"DELIVERY_PRE_ADVICE":    {"PA-{yyyy}{mm}{dd}-{seq:5}", numbering.Daily, docnumber.DateTimeZoneFromIssuer},
		"DELIVERY_HANDOVER_NOTE": {"HO-{yyyy}{mm}{dd}-{seq:5}", numbering.Daily, docnumber.DateTimeZoneFromIssuer},
		"BOOKING":                {"BK-{yyyy}{mm}-{seq:5}", numbering.Monthly, docnumber.DateTimeZoneFromTenant},
		"TRANSPORT_COMMISSION":   {"TC-{yyyy}{mm}-{seq:5}", numbering.Monthly, docnumber.DateTimeZoneFromTenant},
	}

	catalogue, err := Catalogue()
	if err != nil {
		t.Fatalf("aggregate declarations: %v", err)
	}
	declarations := catalogue.Declarations()
	if len(declarations) != len(want) {
		t.Fatalf("aggregated %d types, the decision lists %d", len(declarations), len(want))
	}
	for _, declaration := range declarations {
		expected, listed := want[declaration.Code]
		if !listed {
			t.Errorf("%s is not a type of ADR-0355 决定二", declaration.Code)
			continue
		}
		if declaration.DefaultTemplate != expected.template {
			t.Errorf("%s default %q, the decision says %q", declaration.Code, declaration.DefaultTemplate, expected.template)
		}
		if declaration.DateTimeZone != expected.zone {
			t.Errorf("%s time zone source %s, want %s", declaration.Code, declaration.DateTimeZone, expected.zone)
		}
		template, err := numbering.Parse(declaration.DefaultTemplate)
		if err != nil {
			t.Fatalf("%s: %v", declaration.Code, err)
		}
		if template.Granularity() != expected.reset {
			t.Errorf("%s resets %s, the decision says %s", declaration.Code, template.Granularity(), expected.reset)
		}
	}
}
