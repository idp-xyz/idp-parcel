package postgres

import (
	"encoding/json"
	"strings"
	"testing"
)

// 本文件证 LEP-4 的快照纪律（ADR-0359 决定三、四，沿 ADR-0267 最低可表达版本）：不带收款账户、多语言抬头与主抬头语言的
// 快照字节与摘要一字不变；带任一格就必须是 LEP-4，LEP-3 以下带了它们拒读，LEP-4 不带它们也拒读；LEP-4 原样读回；
// 没有主抬头却带多语言抬头的快照是坏数据。

const lep3Snapshot = `{"tenant":"SYN-TENANT-01","legalEntity":"SYN-LE-01","revision":1,"basis":"SYN-BASIS","effectiveFrom":"2026-01-01T00:00:00Z","address":{"country":"CN","lines":["SYN 地址"]},"taxRegistrationNumbers":[],"invoiceTitle":"SYN 抬头","contacts":[{"name":"SYN 联系人","socialAccounts":[{"platform":"微信","account":"SYN-WX-01"}]}],"shape":"LEP-3"}`

// Covers: AT-PC-078——旧行读作缺席且字节不变：LEP-3 快照重放仍是 LEP-3，摘要前缀不升；读回的修订不带收款账户与多语言抬头。
func TestSnapshotsWithoutCrossBorderDetailsKeepTheirShapeAndBytes(t *testing.T) {
	revision, err := legalEntityProfileFromSnapshot([]byte(lep3Snapshot))
	if err != nil {
		t.Fatal(err)
	}
	document := documentOfLegalEntityProfile(revision)
	raw, _ := json.Marshal(document)
	if string(raw) != lep3Snapshot || document.Shape != "LEP-3" {
		t.Fatalf("LEP-3 字节或形状改变：%s", raw)
	}
	content := revision.Content()
	invoicing, _ := content.Invoicing()
	if content.Supplement().BankAccounts != nil || invoicing.TitleLanguage() != "" || invoicing.LocalizedTitles() != nil {
		t.Fatal("旧行被回填了跨境明细")
	}
}

// Covers: AT-PC-078——带收款账户或多语言抬头的快照须标 LEP-4：三格各自单独出现都升到 LEP-4；标 LEP-3 却带它们、标 LEP-4
// 却不带它们都拒读；LEP-4 原样读回，摘要前缀为 LEP-4。
func TestCrossBorderDetailsRequireTheirShapeAndRoundTrip(t *testing.T) {
	withBank := strings.Replace(lep3Snapshot, `,"shape":"LEP-3"}`, `,"bankAccounts":[{"holderName":"SYN 户名","bankName":"SYN 开户行","accountNumber":"SYN-CNY-0001","currency":"CNY"}],"shape":"LEP-4"}`, 1)
	withTitles := strings.Replace(lep3Snapshot, `,"shape":"LEP-3"}`, `,"invoiceTitleLanguage":"zh-Hans","invoiceTitles":[{"language":"en","title":"SYN Title"}],"shape":"LEP-4"}`, 1)
	withLanguageOnly := strings.Replace(lep3Snapshot, `,"shape":"LEP-3"}`, `,"invoiceTitleLanguage":"zh-Hans","shape":"LEP-4"}`, 1)
	for name, valid := range map[string]string{"收款账户": withBank, "多语言抬头": withTitles, "只标主抬头语言": withLanguageOnly} {
		revision, err := legalEntityProfileFromSnapshot([]byte(valid))
		if err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		document := documentOfLegalEntityProfile(revision)
		raw, _ := json.Marshal(document)
		if string(raw) != valid || document.Shape != "LEP-4" {
			t.Fatalf("%s：LEP-4 读回丢字段或形状不对：%s", name, raw)
		}
	}
	for name, invalid := range map[string]string{
		"LEP-3 带收款账户":  strings.Replace(withBank, `"shape":"LEP-4"`, `"shape":"LEP-3"`, 1),
		"无标记带多语言抬头":    strings.Replace(withTitles, `,"shape":"LEP-4"`, "", 1),
		"LEP-4 不带新格":   strings.Replace(lep3Snapshot, "LEP-3", "LEP-4", 1),
		"LEP-4 带空收款账户": strings.Replace(lep3Snapshot, `,"shape":"LEP-3"}`, `,"bankAccounts":[],"shape":"LEP-4"}`, 1),
		"收款账户缺币种":      strings.Replace(withBank, `,"currency":"CNY"`, "", 1),
		"收款账户同币种同账号":   strings.Replace(withBank, `"bankAccounts":[`, `"bankAccounts":[{"holderName":"SYN 户名","bankName":"SYN 开户行","accountNumber":"SYN-CNY-0001","currency":"CNY"},`, 1),
		"多语言抬头语言重复":    strings.Replace(withTitles, `"invoiceTitles":[`, `"invoiceTitles":[{"language":"EN","title":"SYN Other"},`, 1),
		"多语言抬头撞主抬头语言":  strings.Replace(withTitles, `"language":"en"`, `"language":"zh-hans"`, 1),
		"没有主抬头却带多语言抬头": strings.Replace(withTitles, `"invoiceTitle":"SYN 抬头",`, "", 1),
		"形状未知":         strings.Replace(lep3Snapshot, "LEP-3", "LEP-5", 1),
	} {
		if _, err := legalEntityProfileFromSnapshot([]byte(invalid)); err == nil {
			t.Fatalf("%s：被接收", name)
		}
		var document legalEntityProfileDocument
		if err := json.Unmarshal([]byte(invalid), &document); err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		if err := document.checkShape(); err == nil {
			t.Fatalf("%s：历史读口的形状门放行了", name)
		}
	}
}
