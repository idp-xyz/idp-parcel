package postgres

import (
	"encoding/json"
	"strings"
	"testing"
)

// 旧规范化字节必须原样产生，否则升级构建后相同修订重放会被误答内容冲突。
func TestProfileLegacySnapshotKeepsCanonicalBytes(t *testing.T) {
	const legacy = `{"tenant":"SYN-TENANT-01","legalEntity":"SYN-LE-01","revision":1,"basis":"SYN-BASIS","effectiveFrom":"2026-01-01T00:00:00Z","address":{"country":"CN","lines":["SYN 注册地址"]},"taxRegistrationNumbers":[],"invoiceTitle":"SYN 抬头","contacts":[{"name":"SYN 联系人","email":"syn@example.invalid"}]}`
	revision, err := legalEntityProfileFromSnapshot([]byte(legacy))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(documentOfLegalEntityProfile(revision))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != legacy {
		t.Fatalf("旧规范化字节改变：\n%s\n%s", legacy, raw)
	}
}

func TestProfileSnapshotRejectsUnrecognizedOrMismatchedShape(t *testing.T) {
	const base = `{"tenant":"SYN-TENANT-01","legalEntity":"SYN-LE-01","revision":1,"basis":"SYN-BASIS","effectiveFrom":"2026-01-01T00:00:00Z","address":{"country":"CN","lines":["SYN 地址"]},"taxRegistrationNumbers":[],"contacts":[]}`
	for _, suffix := range []string{`,"shape":"LEP-99"}`, `,"shape":"LEP-2"}`, `,"representative":{"kind":"LEGAL_REPRESENTATIVE","name":"SYN 代表人"}}`} {
		if _, err := legalEntityProfileFromSnapshot([]byte(strings.TrimSuffix(base, "}") + suffix)); err == nil {
			t.Fatalf("接收了未识别或内容不符形状：%s", suffix)
		}
	}
	raw := strings.TrimSuffix(base, "}") + `,"shape":"LEP-2","representative":{"kind":"LEGAL_REPRESENTATIVE","name":"SYN 代表人"}}`
	revision, err := legalEntityProfileFromSnapshot([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if documentOfLegalEntityProfile(revision).Shape != "LEP-2" {
		t.Fatal("扩展资料未保留新形状")
	}
}

func TestSocialAccountsSnapshotRequiresItsShapeAndKeepsPreviousShapeBytes(t *testing.T) {
	const legacy = `{"tenant":"SYN-TENANT-01","legalEntity":"SYN-LE-01","revision":1,"basis":"SYN-BASIS","effectiveFrom":"2026-01-01T00:00:00Z","address":{"country":"CN","lines":["SYN 地址"]},"taxRegistrationNumbers":[],"contacts":[{"name":"SYN 联系人","department":"SYN 财务部"}],"shape":"LEP-2"}`
	revision, err := legalEntityProfileFromSnapshot([]byte(legacy))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(documentOfLegalEntityProfile(revision))
	if string(raw) != legacy {
		t.Fatalf("LEP-2 字节改变：%s", raw)
	}
	withAccounts := strings.Replace(legacy, `"department":"SYN 财务部"`, `"department":"SYN 财务部","socialAccounts":[{"platform":"微信","account":"SYN-WX-01"}]`, 1)
	for _, invalid := range []string{withAccounts, strings.Replace(withAccounts, `,"shape":"LEP-2"`, "", 1), strings.Replace(legacy, "LEP-2", "LEP-3", 1)} {
		if _, err := legalEntityProfileFromSnapshot([]byte(invalid)); err == nil {
			t.Fatal("形状内容不符被接收")
		}
	}
	valid := strings.Replace(withAccounts, "LEP-2", "LEP-3", 1)
	if _, err := legalEntityProfileFromSnapshot([]byte(strings.Replace(valid, `"account":"SYN-WX-01"`, `"account":" "`, 1))); err == nil {
		t.Fatal("有版本标记的空账号被接收")
	}
	revision, err = legalEntityProfileFromSnapshot([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(documentOfLegalEntityProfile(revision))
	if string(raw) != valid {
		t.Fatalf("LEP-3 读回丢字段：%s", raw)
	}
}
