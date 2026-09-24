package registrationjson_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 在线口那一路（票 operator-channel/04）：租户取操作者信封给的，批文带 tenantId 即拒；形状照受控批量口那一份译装。

func TestOnlineTranslationTakesTheEnvelopeTenant(t *testing.T) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{
		"version": "SYN-MAP-V1",
		"approvedBy": "SYN-approver-1",
		"effectiveFrom": "2026-09-01T00:00:00Z",
		"entries": [
			{"source": "PARCEL_SHIPMENT", "factKind": "SYN_KIND_DELIVERED", "milestone": "SYN-MILESTONE-DELIVERED"}
		]
	}`)
	command, err := registrationjson.MilestoneMappingFromJSONForTenant(raw, tenant)
	if err != nil {
		t.Fatalf("online translation: %v", err)
	}
	if command.TenantID != tenant || command.Header.Version != "SYN-MAP-V1" || len(command.Entries) != 1 {
		t.Fatalf("command = tenant %q, header %+v, %d entries", command.TenantID.String(), command.Header, len(command.Entries))
	}
}

func TestOnlineTranslationRefusesASelfReportedTenantOnEveryFace(t *testing.T) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	faces := map[string]func([]byte) error{
		"milestone mapping": func(raw []byte) error {
			_, err := registrationjson.MilestoneMappingFromJSONForTenant(raw, tenant)
			return err
		},
		"triage rules": func(raw []byte) error {
			_, err := registrationjson.TriageRulesFromJSONForTenant(raw, tenant)
			return err
		},
		"notification policy": func(raw []byte) error {
			_, err := registrationjson.NotificationPolicyFromJSONForTenant(raw, tenant)
			return err
		},
		"claim eligibility": func(raw []byte) error {
			_, err := registrationjson.ClaimEligibilityFromJSONForTenant(raw, tenant)
			return err
		},
		"claim authorization": func(raw []byte) error {
			_, err := registrationjson.ClaimAuthorizationFromJSONForTenant(raw, tenant)
			return err
		},
		"disclosure policy": func(raw []byte) error {
			_, err := registrationjson.DisclosurePolicyFromJSONForTenant(raw, tenant)
			return err
		},
		"exception disclosure rules": func(raw []byte) error {
			_, err := registrationjson.ExceptionDisclosureRulesFromJSONForTenant(raw, tenant)
			return err
		},
		"conflict signal rule": func(raw []byte) error {
			_, err := registrationjson.ConflictSignalRuleFromJSONForTenant(raw, tenant)
			return err
		},
	}
	for name, translate := range faces {
		for _, raw := range []string{`{"tenantId": "SYN-TENANT-02", "version": "v"}`, `{"tenantId": null, "version": "v"}`} {
			if err := translate([]byte(raw)); !errors.Is(err, registrationjson.ErrSelfReportedTenant) {
				t.Fatalf("%s with %s: err = %v, want ErrSelfReportedTenant", name, raw, err)
			}
		}
	}
}
