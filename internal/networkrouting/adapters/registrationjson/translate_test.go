package registrationjson_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// 在线口那一路（票 operator-channel/04）：租户取操作者信封给的，批文带 tenant_id 即拒；受控批量口那一路租户取批文。

func TestNetworkTranslationTakesTheTenantFromItsSource(t *testing.T) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	online, err := registrationjson.NodeVersionFromJSONForTenant([]byte(`{"code": "SYN-NODE-A", "version": 1, "business_timezone": "Asia/Shanghai", "effective_from": "2026-09-01T00:00:00Z"}`), tenant)
	if err != nil || online.TenantID != tenant || online.Node.Code != "SYN-NODE-A" || online.Node.HasEffectiveTo {
		t.Fatalf("online node = %+v, err = %v", online, err)
	}
	batch, err := registrationjson.NodeVersionFromJSON([]byte(`{"tenant_id": "SYN-TENANT-09", "code": "SYN-NODE-A", "version": 1, "business_timezone": "Asia/Shanghai", "effective_from": "2026-09-01T00:00:00Z"}`))
	if err != nil || batch.TenantID.String() != "SYN-TENANT-09" {
		t.Fatalf("batch node tenant = %q, err = %v", batch.TenantID.String(), err)
	}
}

func TestOnlineTranslationRefusesASelfReportedTenantOnEveryNetworkFamily(t *testing.T) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	families := map[string]func([]byte) error{
		"node": func(raw []byte) error {
			_, err := registrationjson.NodeVersionFromJSONForTenant(raw, tenant)
			return err
		},
		"connection": func(raw []byte) error {
			_, err := registrationjson.ConnectionVersionFromJSONForTenant(raw, tenant)
			return err
		},
		"line": func(raw []byte) error {
			_, err := registrationjson.LineVersionFromJSONForTenant(raw, tenant)
			return err
		},
		"service area": func(raw []byte) error {
			_, err := registrationjson.ServiceAreaVersionFromJSONForTenant(raw, tenant)
			return err
		},
		"service calendar": func(raw []byte) error {
			_, err := registrationjson.ServiceCalendarVersionFromJSONForTenant(raw, tenant)
			return err
		},
		"availability adjustment": func(raw []byte) error {
			_, err := registrationjson.AvailabilityAdjustmentFromJSONForTenant(raw, tenant)
			return err
		},
		"route strategy": func(raw []byte) error {
			_, err := registrationjson.RouteStrategyVersionFromJSONForTenant(raw, tenant)
			return err
		},
	}
	for name, translate := range families {
		for _, raw := range []string{`{"tenant_id": "SYN-TENANT-02"}`, `{"tenant_id": null}`} {
			if err := translate([]byte(raw)); !errors.Is(err, registrationjson.ErrSelfReportedTenant) {
				t.Fatalf("%s with %s: err = %v, want ErrSelfReportedTenant", name, raw, err)
			}
		}
	}
}
