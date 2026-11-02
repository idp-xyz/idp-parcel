package registrationjson_test

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// Covers: CONTEXT「口岸目录的一版可以带一个可选的显示名称」（登记形状半边，ADR-0353 决定二）——displayName 缺席或 null 即
// 没登；给了就原样随这一版登记；不合格整行拒（领域构造门的错误原样上抛），不截断不改写、也不落成没登。
func TestACandidatePortRowCarriesAnOptionalDisplayName(t *testing.T) {
	row := func(extra string) string {
		return `{"tenantId": "SYN-TENANT-01", "portRef": "SYN-PORT-SZX-01", "appliesFrom": "2026-01-01T00:00:00Z"` + extra + `}`
	}
	for label, extra := range map[string]string{"absent": "", "null": `, "displayName": null`} {
		command, err := registrationjson.CandidatePortFromJSON([]byte(row(extra)))
		if err != nil || command.DisplayName.Registered() {
			t.Fatalf("%s：err=%v name=%q，想要没登", label, err, command.DisplayName.String())
		}
	}
	command, err := registrationjson.CandidatePortFromJSON([]byte(row(`, "displayName": "深圳湾口岸"`)))
	if err != nil || command.DisplayName.String() != "深圳湾口岸" || command.Port.String() != "SYN-PORT-SZX-01" {
		t.Fatalf("带名称：err=%v command=%+v", err, command)
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	online, err := registrationjson.CandidatePortFromJSONForTenant(
		[]byte(`{"portRef": "SYN-PORT-SZX-01", "appliesFrom": "2026-01-01T00:00:00Z", "displayName": "深圳湾口岸"}`), tenant)
	if err != nil || online.DisplayName.String() != "深圳湾口岸" {
		t.Fatalf("在线口带名称：err=%v name=%q", err, online.DisplayName.String())
	}
	for _, bad := range []string{`""`, `" "`, `"深圳湾口岸 "`, `"深圳\t湾"`, `"深圳​湾"`,
		`"` + strings.Repeat("岸", domain.CustomsPortDisplayNameMaxRunes+1) + `"`} {
		if _, err := registrationjson.CandidatePortFromJSON([]byte(row(`, "displayName": ` + bad))); !errors.Is(err, domain.ErrCustomsPortDisplayNameMalformed) {
			t.Fatalf("名称 %s：err=%v，想要 ErrCustomsPortDisplayNameMalformed", bad, err)
		}
	}
}
