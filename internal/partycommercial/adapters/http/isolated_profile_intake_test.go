package commercialhttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
)

// 票 legal-entity-profile/05 的传输层用例：资料登记口按 ADR-0091 逐口放进隔离写准入。载荷与受控 CLI
// `register-legal-entity-profiles` 的一项同形、租户取注入值；形状不对即 MALFORMED_REQUEST，缺不缺交给用例判。

func legalEntityProfileRegistrationRequest(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/commercial-legal-entity-profile-registrations", strings.NewReader(body))
}

const isolatedProfileItem = `{
	"legalEntityId":"SYN-LE-01","revision":2,"basis":"SYN-PROFILE-BASIS-02","effectiveFrom":"2027-01-01T00:00:00Z",
	"registeredAddress":{"country":"CN","lines":["合成市演示路 1 号"]},
	"taxRegistrationNumbers":[{"typeCode":"SYN-CN-TAX","number":"SYN-CN-TAX-0001"}],
	"invoiceTitle":"合成抬头",
	"contacts":[{"name":"合成联系人","email":"syn@example.invalid"}]
}`

// Covers: 资料登记口译命令——租户取注入值，各格逐字来自载荷；抬头缺席即不带开票资料，地址缺席照样译出、由用例答`未受理`。
func TestIsolatedIntakeTranslatesALegalEntityProfileRegistration(t *testing.T) {
	intake := isolatedIdentityIntakeForTest(t)
	command, err := intake.IntakeLegalEntityProfileRegistration(context.Background(),
		legalEntityProfileRegistrationRequest(`{"profiles":[`+isolatedProfileItem+`]}`))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if command.Tenant.String() != isolatedIdentityTenant || command.Entity.String() != "SYN-LE-01" || command.Revision != 2 ||
		command.Basis.String() != "SYN-PROFILE-BASIS-02" || !command.EffectiveFrom.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("命令头 = %+v", command)
	}
	if command.Address.Country().String() != "CN" || len(command.Address.Lines()) != 1 {
		t.Fatalf("注册地址 = %+v", command.Address)
	}
	if len(command.TaxNumbers) != 1 || command.TaxNumbers[0].TypeCode().String() != "SYN-CN-TAX" ||
		command.TaxNumbers[0].Number().String() != "SYN-CN-TAX-0001" {
		t.Fatalf("税务登记号 = %+v", command.TaxNumbers)
	}
	if command.Invoicing == nil || command.Invoicing.Title().String() != "合成抬头" {
		t.Fatalf("开票资料 = %+v", command.Invoicing)
	}
	if len(command.Contacts) != 1 || command.Contacts[0].Name() != "合成联系人" || command.Contacts[0].Email() != "syn@example.invalid" {
		t.Fatalf("联系人 = %+v", command.Contacts)
	}

	bare, err := intake.IntakeLegalEntityProfileRegistration(context.Background(), legalEntityProfileRegistrationRequest(
		`{"profiles":[{"legalEntityId":"SYN-LE-01","revision":1,"basis":"SYN-PROFILE-BASIS-01","effectiveFrom":"2026-01-02T00:00:00Z"}]}`))
	if err != nil {
		t.Fatalf("只带必需头格：%v", err)
	}
	if bare.Invoicing != nil || len(bare.TaxNumbers) != 0 || len(bare.Contacts) != 0 || len(bare.Address.Lines()) != 0 {
		t.Fatalf("缺席的格不该被补上：%+v", bare)
	}
}

// Covers: 形状错各拒一条、都答 MALFORMED_REQUEST——带 tenantId（值同开关也拒）、零项与多项、未知键，以及给了却立不住的格。
func TestIsolatedIntakeRefusesMalformedLegalEntityProfiles(t *testing.T) {
	intake := isolatedIdentityIntakeForTest(t)
	item := func(from, to string) string {
		changed := strings.Replace(isolatedProfileItem, from, to, 1)
		if changed == isolatedProfileItem {
			t.Fatalf("用例自身写错：%q 不在基准项里", from)
		}
		return `{"profiles":[` + changed + `]}`
	}
	cases := map[string]string{
		"带 tenantId": `{"tenantId":"` + isolatedIdentityTenant + `","profiles":[` + isolatedProfileItem + `]}`,
		"零项":         `{"profiles":[]}`,
		"两项":         `{"profiles":[` + isolatedProfileItem + `,` + isolatedProfileItem + `]}`,
		"未知键":        `{"profiles":[` + isolatedProfileItem + `],"extra":1}`,
		"法人标识空":      item(`"legalEntityId":"SYN-LE-01"`, `"legalEntityId":""`),
		"依据空":        item(`"basis":"SYN-PROFILE-BASIS-02"`, `"basis":""`),
		"小写国家":       item(`"country":"CN"`, `"country":"cn"`),
		"空地址行":       item(`"lines":["合成市演示路 1 号"]`, `"lines":[]`),
		"税号缺类型":      item(`"typeCode":"SYN-CN-TAX",`, ``),
		"空抬头":        item(`"invoiceTitle":"合成抬头"`, `"invoiceTitle":""`),
		"联系人没名字":     item(`"name":"合成联系人",`, ``),
	}
	for name, body := range cases {
		if _, err := intake.IntakeLegalEntityProfileRegistration(context.Background(),
			legalEntityProfileRegistrationRequest(body)); !errors.Is(err, commercialhttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}
}
