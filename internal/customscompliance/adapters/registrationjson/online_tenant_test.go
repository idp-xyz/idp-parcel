package registrationjson_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 在线口那一路（票 operator-channel/04）：租户取操作者信封给的，批文带 tenantId 即拒；形状照受控批量口那一份译装。

func TestOnlineGateCatalogTranslationTakesTheEnvelopeTenant(t *testing.T) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	command, err := registrationjson.GateCatalogFromJSONForTenant([]byte(`{
		"scopeRef": "scope-1", "action": "FINAL_DELIVERY", "boundaryRef": "boundary-1",
		"registeredAt": "2026-09-25T01:00:00Z"
	}`), tenant)
	if err != nil {
		t.Fatalf("online gate catalog: %v", err)
	}
	if command.TenantID != tenant || command.Scope.String() != "scope-1" {
		t.Fatalf("command = tenant %q, scope %q", command.TenantID.String(), command.Scope.String())
	}
}

func TestOnlineTranslationRefusesASelfReportedTenantOnEveryCustomsFace(t *testing.T) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	faces := map[string]func([]byte) error{
		"interpretation rule": func(raw []byte) error {
			_, err := registrationjson.InterpretationRuleFromJSONForTenant(raw, tenant)
			return err
		},
		"gate catalog": func(raw []byte) error {
			_, err := registrationjson.GateCatalogFromJSONForTenant(raw, tenant)
			return err
		},
		"candidate port": func(raw []byte) error {
			_, err := registrationjson.CandidatePortFromJSONForTenant(raw, tenant)
			return err
		},
		"declaration path": func(raw []byte) error {
			_, err := registrationjson.DeclarationPathFromJSONForTenant(raw, tenant)
			return err
		},
		"case requirement": func(raw []byte) error {
			_, err := registrationjson.CaseRequirementFromJSONForTenant(raw, tenant)
			return err
		},
		"duty collaboration": func(raw []byte) error {
			_, err := registrationjson.DutyCollaborationFromJSONForTenant(raw, tenant)
			return err
		},
		"duty payment verification": func(raw []byte) error {
			_, err := registrationjson.DutyPaymentVerificationFromJSONForTenant(raw, tenant)
			return err
		},
	}
	for name, translate := range faces {
		for _, raw := range []string{`{"tenantId": "SYN-TENANT-02"}`, `{"tenantId": null}`} {
			if err := translate([]byte(raw)); !errors.Is(err, registrationjson.ErrSelfReportedTenant) {
				t.Fatalf("%s with %s: err = %v, want ErrSelfReportedTenant", name, raw, err)
			}
		}
	}
}
