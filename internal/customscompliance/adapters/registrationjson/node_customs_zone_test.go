package registrationjson_test

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

func TestNodeCustomsZoneTranslationIsStrictAndHasNoDefaults(t *testing.T) {
	command, err := registrationjson.NodeCustomsZoneFromJSON([]byte(`{
		"tenantId": "SYN-TENANT-01", "nodeRef": "SYN-NODE-SZX-GATE", "zoneRef": "SYN-ZONE-CN",
		"appliesFrom": "2026-01-01T00:00:00Z"
	}`))
	if err != nil {
		t.Fatalf("译装：%v", err)
	}
	if command.Node.String() != "SYN-NODE-SZX-GATE" || command.Zone.String() != "SYN-ZONE-CN" || command.AppliesFrom.IsZero() {
		t.Fatalf("命令 = %+v", command)
	}

	for name, raw := range map[string]string{
		"缺起点":   `{"tenantId": "t", "nodeRef": "n", "zoneRef": "z"}`,
		"缺区域":   `{"tenantId": "t", "nodeRef": "n", "appliesFrom": "2026-01-01T00:00:00Z"}`,
		"缺节点":   `{"tenantId": "t", "zoneRef": "z", "appliesFrom": "2026-01-01T00:00:00Z"}`,
		"字段名打错": `{"tenantId": "t", "node": "n", "zoneRef": "z", "appliesFrom": "2026-01-01T00:00:00Z"}`,
	} {
		if _, err := registrationjson.NodeCustomsZoneFromJSON([]byte(raw)); err == nil {
			t.Fatalf("%s：该拒", name)
		}
	}
}

func TestOnlineNodeCustomsZoneTranslationTakesTheEnvelopeTenantOnly(t *testing.T) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	command, err := registrationjson.NodeCustomsZoneFromJSONForTenant([]byte(`{
		"nodeRef": "SYN-NODE-SIN-HUB", "zoneRef": "SYN-ZONE-SG", "appliesFrom": "2026-01-01T00:00:00Z"
	}`), tenant)
	if err != nil || command.TenantID != tenant {
		t.Fatalf("在线译装：%+v / %v", command, err)
	}
	for _, key := range []string{"tenantId", "TENANTID", "tenantid"} {
		raw := `{"` + key + `": "SYN-TENANT-02", "nodeRef": "n", "zoneRef": "z", "appliesFrom": "2026-01-01T00:00:00Z"}`
		_, err := registrationjson.NodeCustomsZoneFromJSONForTenant([]byte(raw), tenant)
		if !errors.Is(err, registrationjson.ErrSelfReportedTenant) && (err == nil || !strings.Contains(err.Error(), "unknown field")) {
			t.Fatalf("%s：自报租户要拒，err = %v", key, err)
		}
	}
}
