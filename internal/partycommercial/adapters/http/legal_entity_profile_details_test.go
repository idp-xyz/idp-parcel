package commercialhttp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

func TestProfileDetailsTranslateThroughWriteAndBothReadShapes(t *testing.T) {
	const raw = `{"legalEntityId":"SYN-LE-01","revision":1,"basis":"SYN-BASIS","effectiveFrom":"2026-01-01T00:00:00Z","registeredAddress":{"country":"CN","lines":["SYN 注册地址"]},"representative":{"kind":"LEGAL_REPRESENTATIVE","name":"SYN 代表人"},"officeAddress":{"country":"SG","lines":["SYN 办公地址"]},"correspondenceAddress":{"country":"CN","lines":["SYN 通信地址"]},"contacts":[{"name":"SYN 联系人","department":"SYN 财务部","position":"SYN 经理","purposes":["FINANCE","BUSINESS"],"primaryFor":["FINANCE"],"socialAccounts":[{"platform":"微信","account":"SYN-WX-01"},{"platform":"Custom","account":"SYN-CUSTOM-01"}]}]}`
	var document legalEntityProfileDocument
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		t.Fatal(err)
	}
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	command, err := document.command(tenant)
	if err != nil {
		t.Fatal(err)
	}
	content, err := domain.NewLegalEntityProfileContentWithSupplement(command.Address, command.TaxNumbers, command.Invoicing, command.Contacts, command.Supplement)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := domain.NewLegalEntityProfileRevision(command.Tenant, command.Entity, command.Revision, command.Basis, command.EffectiveFrom, content)
	if err != nil {
		t.Fatal(err)
	}
	body := legalEntityProfileEffectiveRevisionBodyOf(revision)
	if body.Representative.Name != "SYN 代表人" || body.OfficeAddress.Country != "SG" || body.CorrespondenceAddress.Lines[0] != "SYN 通信地址" || body.Contacts[0].Position != "SYN 经理" || body.Contacts[0].PrimaryFor[0] != domain.ContactFinance || len(body.Contacts[0].SocialAccounts) != 2 || body.Contacts[0].SocialAccounts[0].Account != "SYN-WX-01" {
		t.Fatalf("解析转写丢字段：%+v", body)
	}
	history := legalEntityProfileRevisionBodyOf(ports.LegalEntityProfileRevisionRow{Representative: &ports.LegalEntityRepresentativeRow{Kind: "LEGAL_REPRESENTATIVE", Name: "SYN 代表人"}, OfficeAddress: &ports.LegalEntityAddressRow{Country: "SG", Lines: []string{"SYN 办公地址"}}, CorrespondenceAddress: &ports.LegalEntityAddressRow{Country: "CN", Lines: []string{"SYN 通信地址"}}, Contacts: []ports.LegalEntityContactRow{{Name: "SYN 联系人", Department: "SYN 财务部", Position: "SYN 经理", Purposes: []string{"FINANCE"}, PrimaryFor: []string{"FINANCE"}, SocialAccounts: []domain.ContactSocialAccount{{Platform: "微信", Account: "SYN-WX-01"}}}}, RegisteredAt: time.Now()})
	if history.Representative.Name != body.Representative.Name || history.OfficeAddress.Country != body.OfficeAddress.Country || history.Contacts[0].Department != body.Contacts[0].Department || history.Contacts[0].PrimaryFor[0] != domain.ContactFinance || history.Contacts[0].SocialAccounts[0].Account != "SYN-WX-01" {
		t.Fatalf("历史转写丢字段：%+v", history)
	}
	for _, replacement := range [][2]string{{`"account":"SYN-WX-01"`, `"account":" "`}, {`"platform":"微信"`, `"platform":""`}, {`"platform":"Custom","account":"SYN-CUSTOM-01"`, `"platform":"微信","account":"SYN-WX-01"`}, {`"kind":"LEGAL_REPRESENTATIVE"`, `"kind":"BUSINESS_OWNER"`}, {`"name":"SYN 代表人"`, `"name":""`}, {`"purposes":["FINANCE","BUSINESS"]`, `"purposes":["BUSINESS"]`}, {`"country":"SG"`, `"country":"sg"`}} {
		var invalid legalEntityProfileDocument
		if err := json.Unmarshal([]byte(strings.Replace(raw, replacement[0], replacement[1], 1)), &invalid); err != nil {
			t.Fatal(err)
		}
		if _, err := invalid.command(tenant); err == nil {
			t.Fatalf("无效输入被接收：%v", replacement)
		}
	}
}
