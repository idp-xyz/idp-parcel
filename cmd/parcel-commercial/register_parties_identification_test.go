package main

import (
	"strings"
	"testing"
)

// 往来单位识别信息（ADR-0367）在受控 CLI 上的两半：翻译把四格逐字译进命令、形状不对在触库前拒；对真库，第二家单位登
// 同一个号或同一个外部编码答「请人看」，不落册。

const identifiedPartyBatchJSON = `{
  "tenantId": "tenant-1",
  "businessParties": [
    {"partyId": "party-gamma", "name": "Gamma 货代", "revision": 1, "basis": "REG/party-gamma", "effectiveFrom": "2026-01-01T00:00:00Z",
     "registrationCountry": "CN", "lifetimeRegistrationNumbers": [{"typeCode": "SYN-LIFETIME", "number": "SYN-000101"}],
     "externalReferences": [{"system": "SYN-ERP", "purpose": "CUSTOMER", "code": "C-0101"}]}
  ]
}`

func TestPartyBatchTranslationCarriesIdentification(t *testing.T) {
	if _, err := partyBatchFromJSON([]byte(identifiedPartyBatchJSON)); err != nil {
		t.Fatalf("翻译带识别信息的批：%v", err)
	}
	for name, body := range map[string]string{
		"用途不在词表": strings.Replace(identifiedPartyBatchJSON, `"CUSTOMER"`, `"VENDOR"`, 1),
		"国家小写":   strings.Replace(identifiedPartyBatchJSON, `"CN"`, `"cn"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := partyBatchFromJSON([]byte(body)); err == nil {
				t.Fatal("变形批被收下了")
			}
		})
	}
}

func TestRegisterPartiesRefusesASecondHolderOfANumberOrCode(t *testing.T) {
	dsn := freshMigratedDSN(t)
	if code := runCLI(t, dsn, "register-registration-number-types", "-input",
		batchFile(t, registrationNumberTypeBatchJSON)); code != exitLanded {
		t.Fatalf("注册号类型目录 exit = %d, want %d", code, exitLanded)
	}
	if code := runCLI(t, dsn, "register-parties", "-input", batchFile(t, identifiedPartyBatchJSON)); code != exitLanded {
		t.Fatalf("首家 exit = %d, want %d", code, exitLanded)
	}
	sameNumber := strings.NewReplacer(`party-gamma`, `party-delta`, `C-0101`, `C-0102`).Replace(identifiedPartyBatchJSON)
	if code := runCLI(t, dsn, "register-parties", "-input", batchFile(t, sameNumber)); code != exitAttention {
		t.Fatalf("同号第二家 exit = %d, want %d", code, exitAttention)
	}
	sameCode := strings.NewReplacer(`party-gamma`, `party-delta`, `SYN-000101`, `SYN-000102`).Replace(identifiedPartyBatchJSON)
	if code := runCLI(t, dsn, "register-parties", "-input", batchFile(t, sameCode)); code != exitAttention {
		t.Fatalf("同码第二家 exit = %d, want %d", code, exitAttention)
	}
	distinct := strings.NewReplacer(`party-gamma`, `party-delta`, `SYN-000101`, `SYN-000102`, `C-0101`, `C-0102`).Replace(identifiedPartyBatchJSON)
	if code := runCLI(t, dsn, "register-parties", "-input", batchFile(t, distinct)); code != exitLanded {
		t.Fatalf("号与码都不同的第二家 exit = %d, want %d", code, exitLanded)
	}
}
