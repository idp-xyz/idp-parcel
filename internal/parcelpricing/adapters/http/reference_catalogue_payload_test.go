package pricinghttp_test

import (
	"errors"
	"strings"
	"testing"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

const zoneChartPayload = `{
  "catalogueId": "SYN-CAT-ZONE",
  "catalogueVersion": "v1",
  "kind": "ZONE",
  "sourceIdentifier": "SYN-CARRIER/zone-chart-2026",
  "origin": {"scope": "POSTAL_PREFIXES", "prefixes": ["940", "941"]},
  "prefixLength": 3,
  "entries": [{"prefix": "902", "value": "Z4"}, {"prefix": "100", "value": "Z8"}],
  "effectiveFrom": "2026-01-01T00:00:00Z",
  "effectiveTo": "2027-01-01T00:00:00Z"
}`

// Covers: ADR-0101 决定八自裁的模板导入形——一份 JSON 带整张表；身份从信封来，每一格过领域构造器。
func TestReferenceCataloguePayloadBecomesARegistrationWithTheEnvelopeIdentity(t *testing.T) {
	payload, err := pricinghttp.DecodeReferenceCataloguePayload(strings.NewReader(zoneChartPayload))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	tenant, _ := domain.NewTenantID("tenant-1")
	command, err := payload.RegistrationCommand(tenant, "SYN-CAT-REGISTRAR")
	if err != nil {
		t.Fatalf("registration command: %v", err)
	}
	registration := command.Registration
	if registration.Kind() != domain.CatalogueKindZone || registration.Registrant() != "SYN-CAT-REGISTRAR" || registration.PrefixLength() != 3 || len(registration.Entries()) != 2 {
		t.Fatalf("registration shape: %#v", registration)
	}
	if registration.Origin().Scope() != domain.CatalogueOriginPostalPrefixes || len(registration.Origin().Prefixes()) != 2 {
		t.Fatalf("origin: %#v", registration.Origin())
	}

	if _, err := payload.RegistrationCommand(domain.TenantID{}, "SYN-CAT-REGISTRAR"); !errors.Is(err, pricinghttp.ErrOperatorIdentityMissing) {
		t.Fatalf("missing tenant: %v", err)
	}
}

// Covers: 载荷里不许出现身份键，未知键一律拒；始发维度不在封闭集拒；条目与粒度不符由领域门拒——三种都是
// ErrMalformedRequest，同一份内容重发不会变好。
func TestReferenceCataloguePayloadRejectsSelfReportedIdentityAndIllFormedTables(t *testing.T) {
	withIdentity := strings.Replace(zoneChartPayload, `"kind": "ZONE",`, `"kind": "ZONE", "tenant": "tenant-1",`, 1)
	if _, err := pricinghttp.DecodeReferenceCataloguePayload(strings.NewReader(withIdentity)); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
		t.Fatalf("self-reported tenant accepted: %v", err)
	}

	tenant, _ := domain.NewTenantID("tenant-1")
	badOrigin := strings.Replace(zoneChartPayload, `"scope": "POSTAL_PREFIXES"`, `"scope": "ORIGIN_ZONE"`, 1)
	payload, err := pricinghttp.DecodeReferenceCataloguePayload(strings.NewReader(badOrigin))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, err := payload.RegistrationCommand(tenant, "SYN-CAT-REGISTRAR"); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
		t.Fatalf("origin outside the closed set accepted: %v", err)
	}

	mixedLength := strings.Replace(zoneChartPayload, `{"prefix": "100", "value": "Z8"}`, `{"prefix": "10001", "value": "Z8"}`, 1)
	payload, err = pricinghttp.DecodeReferenceCataloguePayload(strings.NewReader(mixedLength))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, err := payload.RegistrationCommand(tenant, "SYN-CAT-REGISTRAR"); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
		t.Fatalf("entry longer than the declared prefix length accepted: %v", err)
	}
}
