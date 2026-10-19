package domain_test

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 收寄面来源文档的 NRS-2 字面量，独立于实现写下：比 NRS-1 多交付范围依据引用两格（ADR-0212 决定七），缺席时照样在、记空串。
const (
	preAdvisedSourceDocument = `{"shape":"NRS-2","unit":"UNIT-1","mark":"MARK-1","claim":"RECEIVED","evidence":"SIGN-1","refusal":"","delivered_by":"CUSTOMER-1","node":"NODE-1","occurred_at":"2026-03-05T02:11:12.345678901Z","scope_basis_kind":"DELIVERY_PRE_ADVICE","scope_basis_id":"PREADV-1"}`
	basislessSourceDocument  = `{"shape":"NRS-2","unit":"UNIT-1","mark":"MARK-1","claim":"RECEIVED","evidence":"SIGN-1","refusal":"","delivered_by":"CUSTOMER-1","node":"NODE-1","occurred_at":"2026-03-05T02:11:12.345678901Z","scope_basis_kind":"","scope_basis_id":""}`
)

func receivedSource(t *testing.T) domain.DeliverySource {
	t.Helper()
	return domain.DeliverySource{
		Unit:        mustValue(t, domain.NewHandlingUnitID, "UNIT-1"),
		Mark:        "MARK-1",
		Claim:       "RECEIVED",
		Evidence:    mustValue(t, domain.NewReceptionEvidenceReference, "SIGN-1"),
		DeliveredBy: mustValue(t, domain.NewDeliveringPartyReference, "CUSTOMER-1"),
		Node:        mustValue(t, domain.NewNodeReference, "NODE-1"),
		OccurredAt:  payloadInstant(),
	}
}

// Covers: ADR-0212 决定七「收寄来源文档多一格：依据种类与标识，可以没有；来源文档的形状版本随之升一版」——带依据的来源写成 NRS-2、
// 依据原样保全；没带依据的同样写 NRS-2，两格记空串、不省键。读回的与写下的逐格相同。
func TestDeliverySourceKeepsTheScopeBasisAsNRS2(t *testing.T) {
	basis, err := domain.NewDeliveryScopeBasisReference(domain.BasisDeliveryPreAdvice, "PREADV-1")
	if err != nil {
		t.Fatal(err)
	}
	for name, testCase := range map[string]struct {
		basis    domain.DeliveryScopeBasisReference
		document string
	}{
		"带预报":  {basis, preAdvisedSourceDocument},
		"没带依据": {domain.DeliveryScopeBasisReference{}, basislessSourceDocument},
	} {
		source := receivedSource(t)
		source.ScopeBasis = testCase.basis
		document, err := domain.CanonicalizeDeliverySource(source)
		if err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		if string(document) != testCase.document {
			t.Fatalf("%s：document = %s", name, document)
		}
		parsed, err := domain.ParseDeliverySource(document)
		if err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		want := source
		want.OccurredAt = payloadInstant().UTC()
		if parsed != want {
			t.Fatalf("%s：parsed = %+v, want %+v", name, parsed, want)
		}
	}
}

// Covers: ADR-0212 决定七「既有行不回填，读作没有依据」——NRS-1 的来源文档照旧读得回，依据是零值。
func TestAnNRS1SourceReadsAsHavingNoScopeBasis(t *testing.T) {
	parsed, err := domain.ParseDeliverySource([]byte(refusalSourceDocument))
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Preserved() || parsed.ScopeBasis.Present() || parsed.ScopeBasis != (domain.DeliveryScopeBasisReference{}) {
		t.Fatalf("NRS-1 读回 = %+v, want 来源在、没有依据", parsed)
	}
}

// Covers: 来源文档里的依据两格要么同缺、要么成对且种类在词表里：半格依据读回成「没有依据」会把一次带了依据的送站说成没带。
func TestAHalfOrUnknownScopeBasisInTheSourceIsRefused(t *testing.T) {
	for name, document := range map[string]string{
		"种类认不出":  strings.Replace(preAdvisedSourceDocument, `"DELIVERY_PRE_ADVICE"`, `"PRE_ADVICE"`, 1),
		"有种类没标识": strings.Replace(preAdvisedSourceDocument, `"PREADV-1"`, `""`, 1),
		"有标识没种类": strings.Replace(preAdvisedSourceDocument, `"DELIVERY_PRE_ADVICE"`, `""`, 1),
	} {
		if _, err := domain.ParseDeliverySource([]byte(document)); err == nil {
			t.Fatalf("%s：读回了 %s", name, document)
		}
	}
}
