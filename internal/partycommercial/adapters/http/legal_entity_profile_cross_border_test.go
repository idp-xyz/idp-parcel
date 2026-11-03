package commercialhttp

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// Covers: AT-PC-078——在线口的写入翻译与两个读口的答复体都带收款账户、主抬头语言与多语言抬头（ADR-0359 决定三、四）；
// 币种目录外、同（币种，账号）、同语言、没主抬头却带翻译的载荷都在译装处答形状错（400），一个字节不进用例。
func TestCrossBorderProfileDetailsTranslateThroughWriteAndBothReadShapes(t *testing.T) {
	const raw = `{"legalEntityId":"SYN-LE-01","revision":1,"basis":"SYN-BASIS","effectiveFrom":"2026-01-01T00:00:00Z","registeredAddress":{"country":"CN","lines":["SYN 注册地址"]},"invoiceTitle":"合成运营方一号（演示）","invoiceTitleLanguage":"zh-Hans","invoiceTitles":[{"language":"en","title":"SYN Operator One (Demo)"}],"bankAccounts":[{"label":"人民币对公","holderName":"SYN 户名","bankName":"SYN 开户行","accountNumber":"SYN-CNY-0001","swiftCode":"SYNBCNSH","currency":"CNY"},{"holderName":"SYN 户名","bankName":"SYN 开户行","accountNumber":"SYN-USD-0001","currency":"USD","bankAddress":"SYN 开户行地址","note":"SYN 备注"}]}`
	var document legalEntityProfileDocument
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		t.Fatal(err)
	}
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	command, err := document.command(tenant)
	if err != nil {
		t.Fatal(err)
	}
	if len(command.Supplement.BankAccounts) != 2 || command.Invoicing.TitleLanguage() != "zh-Hans" || len(command.Invoicing.LocalizedTitles()) != 1 {
		t.Fatalf("写入翻译丢跨境明细：%+v", command)
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
	if body.InvoiceTitleLanguage != "zh-Hans" || len(body.InvoiceTitles) != 1 || body.InvoiceTitles[0].Language != "en" || len(body.BankAccounts) != 2 || body.BankAccounts[0].SwiftCode != "SYNBCNSH" || body.BankAccounts[1].BankAddress != "SYN 开户行地址" {
		t.Fatalf("解析转写丢字段：%+v", body)
	}
	history := legalEntityProfileRevisionBodyOf(ports.LegalEntityProfileRevisionRow{HasInvoicing: true, InvoiceTitle: "合成运营方一号（演示）", InvoiceTitleLanguage: "zh-Hans", InvoiceTitles: []domain.LocalizedInvoiceTitle{{Language: "en", Title: "SYN Operator One (Demo)"}}, BankAccounts: []domain.LegalEntityBankAccount{{HolderName: "SYN 户名", BankName: "SYN 开户行", AccountNumber: "SYN-CNY-0001", Currency: "CNY"}}, RegisteredAt: time.Now()})
	if history.InvoiceTitleLanguage != body.InvoiceTitleLanguage || history.InvoiceTitles[0].Title != body.InvoiceTitles[0].Title || history.BankAccounts[0].AccountNumber != body.BankAccounts[0].AccountNumber {
		t.Fatalf("历史转写丢字段：%+v", history)
	}
	// 旧行与缺席：没带这三格的行体不出现这三个键，页面分得出「没登」与「登了空的」。
	encoded, _ := json.Marshal(legalEntityProfileRevisionBodyOf(ports.LegalEntityProfileRevisionRow{HasInvoicing: true, InvoiceTitle: "SYN 抬头", RegisteredAt: time.Now()}))
	for _, key := range []string{"invoiceTitleLanguage", "invoiceTitles", "bankAccounts"} {
		if strings.Contains(string(encoded), `"`+key+`"`) {
			t.Fatalf("缺席的 %s 出现在答复体里：%s", key, encoded)
		}
	}

	for name, replacement := range map[string][2]string{
		"币种在目录外":      {`"currency":"USD"`, `"currency":"XXX"`},
		"币种形状不对":      {`"currency":"USD"`, `"currency":"usd"`},
		"同币种同账号":      {`"accountNumber":"SYN-USD-0001","currency":"USD"`, `"accountNumber":"SYN-CNY-0001","currency":"CNY"`},
		"收款账户缺户名":     {`"holderName":"SYN 户名","bankName":"SYN 开户行","accountNumber":"SYN-USD-0001"`, `"bankName":"SYN 开户行","accountNumber":"SYN-USD-0001"`},
		"多语言抬头撞主抬头语言": {`"language":"en"`, `"language":"ZH-HANS"`},
		"语言标签形状不对":    {`"language":"en"`, `"language":"en_US"`},
		"翻译抬头空白":      {`"title":"SYN Operator One (Demo)"`, `"title":" "`},
		"没主抬头却带翻译":    {`"invoiceTitle":"合成运营方一号（演示）",`, ``},
	} {
		var invalid legalEntityProfileDocument
		if err := json.Unmarshal([]byte(strings.Replace(raw, replacement[0], replacement[1], 1)), &invalid); err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		if _, err := invalid.command(tenant); !errors.Is(err, ErrMalformedRequest) {
			t.Fatalf("%s：应答形状错，得 %v", name, err)
		}
	}
}
