package main

import "testing"

// Covers: AT-PC-078——受控批文带收款账户、主抬头语言与多语言抬头照原样译进命令（ADR-0359 决定三、四）；币种在目录外、
// 同（币种，账号）、同语言、没主抬头却带翻译、收款账户缺必填格都在触库前拒收，一格不代填。
func TestLegalEntityProfileBatchCarriesCrossBorderDetailsAndRefusesTheirFlaws(t *testing.T) {
	commands, err := legalEntityProfileBatchFromJSON([]byte(profileBatchOf(validProfileRest + `,
"invoiceTitle":"合成运营方一号（演示）","invoiceTitleLanguage":"zh-Hans",
"invoiceTitles":[{"language":"en","title":"SYN Operator One (Demo)"},{"language":"zh-Hant","title":"合成營運方一號（演示）"}],
"bankAccounts":[{"label":"人民币对公","holderName":"SYN 户名","bankName":"SYN 开户行","accountNumber":"SYN-CNY-0001","swiftCode":"SYNBCNSH","currency":"CNY"},
                {"holderName":"SYN 户名","bankName":"SYN 开户行","accountNumber":"SYN-USD-0001","currency":"USD"}]`)))
	if err != nil {
		t.Fatal(err)
	}
	command := commands[0].command
	if command.Invoicing == nil || command.Invoicing.TitleLanguage() != "zh-Hans" || len(command.Invoicing.LocalizedTitles()) != 2 || command.Invoicing.LocalizedTitles()[1].Language != "zh-Hant" ||
		len(command.Supplement.BankAccounts) != 2 || command.Supplement.BankAccounts[0].Label != "人民币对公" || command.Supplement.BankAccounts[1].Currency != "USD" {
		t.Fatalf("CLI 跨境明细丢失：%+v", command)
	}
	for name, body := range map[string]string{
		"币种在目录外":   profileBatchOf(validProfileRest + `, "bankAccounts":[{"holderName":"SYN","bankName":"SYN","accountNumber":"SYN-0001","currency":"XXX"}]`),
		"币种形状不对":   profileBatchOf(validProfileRest + `, "bankAccounts":[{"holderName":"SYN","bankName":"SYN","accountNumber":"SYN-0001","currency":"cny"}]`),
		"同币种同账号":   profileBatchOf(validProfileRest + `, "bankAccounts":[{"holderName":"SYN","bankName":"SYN","accountNumber":"SYN-0001","currency":"CNY"},{"holderName":"SYN 乙","bankName":"SYN 乙","accountNumber":"SYN-0001","currency":"CNY"}]`),
		"收款账户缺开户行": profileBatchOf(validProfileRest + `, "bankAccounts":[{"holderName":"SYN","accountNumber":"SYN-0001","currency":"CNY"}]`),
		"同语言两个抬头":  profileBatchOf(validProfileRest + `, "invoiceTitle":"SYN","invoiceTitles":[{"language":"en","title":"A"},{"language":"EN","title":"B"}]`),
		"翻译撞主抬头语言": profileBatchOf(validProfileRest + `, "invoiceTitle":"SYN","invoiceTitleLanguage":"en","invoiceTitles":[{"language":"en","title":"A"}]`),
		"语言标签形状不对": profileBatchOf(validProfileRest + `, "invoiceTitle":"SYN","invoiceTitleLanguage":"zh_Hans"`),
		"没主抬头却带翻译": profileBatchOf(validProfileRest + `, "invoiceTitles":[{"language":"en","title":"A"}]`),
		"没主抬头却标语言": profileBatchOf(validProfileRest + `, "invoiceTitleLanguage":"en"`),
	} {
		if _, err := legalEntityProfileBatchFromJSON([]byte(body)); err == nil {
			t.Fatalf("%s：翻译应在触库前拒收", name)
		}
	}
}
