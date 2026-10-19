package registrationjson_test

import (
	"errors"
	"strings"
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

// tenantKeyCaseVariants 是 encoding/json 会解进 `json:"tenantId"` 格的那些键：它匹配字段名不分大小写，拒自报只认
// 精确键时，变体就顶着认证租户落进别的租户（票 operator-channel/52）。
var tenantKeyCaseVariants = []string{"tenantId", "TenantId", "tenantID", "tenantid", "tenantiD", "TENANTID"}

const onlineCredentialBody = `{"credentialId":"SYN-CRED-0801","issuerRef":"SYN-AUTHORITY/customs-sg",` +
	`"holderRef":"SYN-HOLDER/broker-08","procedureRef":"SYN-PROCEDURE/import-general",` +
	`"validFrom":"2026-09-01T00:00:00Z","validTo":"2027-08-31T00:00:00Z","uses":12}`

func TestOnlineRegulatoryCredentialTranslationTakesTheInjectedTenant(t *testing.T) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	command, err := registrationjson.RegulatoryCredentialFromJSONForTenant([]byte(onlineCredentialBody), tenant)
	if err != nil {
		t.Fatalf("online credential: %v", err)
	}
	if command.TenantID != tenant || command.ID.String() != "SYN-CRED-0801" || command.Uses != 12 {
		t.Fatalf("command = tenant %q, id %q, uses %d", command.TenantID.String(), command.ID.String(), command.Uses)
	}
}

func TestOnlineRegulatoryCredentialTranslationRefusesEveryLetterCaseOfTheTenantKey(t *testing.T) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	rest := strings.TrimPrefix(onlineCredentialBody, "{")
	for _, key := range tenantKeyCaseVariants {
		command, err := registrationjson.RegulatoryCredentialFromJSONForTenant([]byte(`{"`+key+`":"SYN-TENANT-OTHER",`+rest), tenant)
		if !errors.Is(err, registrationjson.ErrSelfReportedTenant) {
			t.Fatalf("键 %q：err = %v，命令租户 %q；want ErrSelfReportedTenant", key, err, command.TenantID.String())
		}
	}
	// 与 tenantId 不同折叠的键 encoding/json 也不认，在线口照未知字段拒，不是放行。
	if _, err := registrationjson.RegulatoryCredentialFromJSONForTenant([]byte(`{"tenantİd":"SYN-TENANT-OTHER",`+rest), tenant); err == nil {
		t.Fatal("tenantİd 被放行")
	}
}

func TestOnlineTranslationRefusesEveryLetterCaseOfTheTenantKeyOnOperatorFaces(t *testing.T) {
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
		for _, key := range tenantKeyCaseVariants {
			if err := translate([]byte(`{"` + key + `": "SYN-TENANT-OTHER"}`)); !errors.Is(err, registrationjson.ErrSelfReportedTenant) {
				t.Fatalf("%s 键 %q：err = %v, want ErrSelfReportedTenant", name, key, err)
			}
		}
	}
}
