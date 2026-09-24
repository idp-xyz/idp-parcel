package main

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// 本文件证注册号类型登记进程口的两半（票 legal-entity-profile/01）：
// 翻译纪律——批文逐字段过领域构造门，未知字段、集外层、编不过的格式、形状不对的国家 / 地区与缺席
// 的时点在触库前拒收，一格不代填；
// 批推进——对真库端到端跑 register-registration-number-types，证登记、整批重放、同修订异内容冲突、
// 内容更正、改层被拒、停用从未登记的类型报请人看。

const registrationNumberTypeBatchJSON = `{
  "tenantId": "tenant-1",
  "types": [
    {
      "countryCode": "XA", "typeCode": "SYN-LIFETIME", "revision": 1,
      "name": "合成终身注册号", "layer": "IDENTITY", "format": "SYN-[0-9]{6}",
      "basis": "SYN-BASIS-01", "effectiveFrom": "2026-01-01T00:00:00Z"
    },
    {
      "countryCode": "XA", "typeCode": "SYN-TAX", "revision": 1,
      "name": "合成税务登记号", "layer": "PROFILE", "format": "SYN-TAX-[0-9]{4}",
      "basis": "SYN-BASIS-02", "effectiveFrom": "2026-01-01T00:00:00Z"
    }
  ],
  "deactivations": [
    {"countryCode": "XA", "typeCode": "SYN-TAX", "revision": 2, "basis": "SYN-BASIS-RETIRE", "at": "2026-02-01T00:00:00Z"}
  ]
}`

func singleRegistrationNumberType(revision, layer, format string) string {
	return `{
  "tenantId": "tenant-1",
  "types": [{
    "countryCode": "XA", "typeCode": "SYN-LIFETIME", "revision": ` + revision + `,
    "name": "合成终身注册号", "layer": "` + layer + `", "format": "` + format + `",
    "basis": "SYN-BASIS-r` + revision + `", "effectiveFrom": "2026-01-01T00:00:00Z"
  }]
}`
}

func countRegistrationNumberTypeRows(t *testing.T, dsn, typeCode string) int {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("开池核对：%v", err)
	}
	defer pool.Close()

	var count int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM party_commercial.registration_number_type_registration
		  WHERE tenant_id = $1 AND country_code = $2 AND type_code = $3`,
		"tenant-1", "XA", typeCode,
	).Scan(&count); err != nil {
		t.Fatalf("统计类型修订行：%v", err)
	}
	return count
}

// Covers: 翻译产物按文件内 types → deactivations 次序排列，标签带国家 / 地区、类型与修订。
func TestRegistrationNumberTypeBatchTranslationKeepsOrder(t *testing.T) {
	commands, err := registrationNumberTypeBatchFromJSON([]byte(registrationNumberTypeBatchJSON))
	if err != nil {
		t.Fatalf("翻译完整批：%v", err)
	}
	wantLabels := []string{
		"注册号类型 XA/SYN-LIFETIME r1",
		"注册号类型 XA/SYN-TAX r1",
		"停用注册号类型 XA/SYN-TAX r2",
	}
	if len(commands) != len(wantLabels) {
		t.Fatalf("命令数 = %d, want %d", len(commands), len(wantLabels))
	}
	for index, want := range wantLabels {
		if commands[index].label != want {
			t.Fatalf("第 %d 项标签 = %q, want %q", index+1, commands[index].label, want)
		}
	}
}

// Covers: 缺件与形状错在触库之前拒收，绝不代填默认——没有「默认层」「默认格式」「当前时刻」这回事。
func TestRegistrationNumberTypeBatchTranslationRefusesBeforeTouchingTheDatabase(t *testing.T) {
	cases := map[string]string{
		"未知字段":   strings.Replace(registrationNumberTypeBatchJSON, `"revision": 1,`, `"revision": 1, "extra": 1,`, 1),
		"集外层":    singleRegistrationNumberType("1", "TAX", "SYN-[0-9]{6}"),
		"空层":     singleRegistrationNumberType("1", "", "SYN-[0-9]{6}"),
		"编不过的格式": singleRegistrationNumberType("1", "IDENTITY", "["),
		"空格式":    singleRegistrationNumberType("1", "IDENTITY", ""),
		"小写国家":   strings.Replace(singleRegistrationNumberType("1", "IDENTITY", "SYN-[0-9]{6}"), `"XA"`, `"xa"`, 1),
		"缺生效时点":  strings.Replace(singleRegistrationNumberType("1", "IDENTITY", "SYN-[0-9]{6}"), `, "effectiveFrom": "2026-01-01T00:00:00Z"`, ``, 1),
		"缺停用时点": `{"tenantId": "tenant-1", "deactivations": [
		  {"countryCode": "XA", "typeCode": "SYN-TAX", "revision": 2, "basis": "SYN-BASIS-RETIRE"}]}`,
		"空批": `{"tenantId": "tenant-1"}`,
	}
	for name, body := range cases {
		if _, err := registrationNumberTypeBatchFromJSON([]byte(body)); err == nil {
			t.Fatalf("%s：翻译应在触库前拒收", name)
		}
	}
}

// Covers: 票 legal-entity-profile/01 登记面的进程口半边，端到端于真库——整批（两层各一类型 + 一次
// 停用）落定；重放整批以原结果回答不追加行；同修订异内容报请人看；内容更正占下一修订号；新修订
// 改层被拒；停用从未登记的类型报请人看。
func TestRegisterRegistrationNumberTypesBatchLandsRepliesAndRefuses(t *testing.T) {
	dsn := freshMigratedDSN(t)

	batch := batchFile(t, registrationNumberTypeBatchJSON)
	if code := runCLI(t, dsn, "register-registration-number-types", "-input", batch); code != exitLanded {
		t.Fatalf("首批 exit = %d, want %d", code, exitLanded)
	}
	if code := runCLI(t, dsn, "register-registration-number-types", "-input", batch); code != exitLanded {
		t.Fatalf("重放批 exit = %d, want %d", code, exitLanded)
	}
	if got := countRegistrationNumberTypeRows(t, dsn, "SYN-LIFETIME"); got != 1 {
		t.Fatalf("SYN-LIFETIME 行数 = %d, want 1（重放不追加）", got)
	}
	if got := countRegistrationNumberTypeRows(t, dsn, "SYN-TAX"); got != 2 {
		t.Fatalf("SYN-TAX 行数 = %d, want 2（登记 + 停用）", got)
	}

	conflicted := batchFile(t, singleRegistrationNumberType("1", "IDENTITY", "SYN-[0-9]{7}"))
	if code := runCLI(t, dsn, "register-registration-number-types", "-input", conflicted); code != exitAttention {
		t.Fatalf("冲突批 exit = %d, want %d", code, exitAttention)
	}

	corrected := batchFile(t, singleRegistrationNumberType("2", "IDENTITY", "SYN-[0-9]{8}"))
	if code := runCLI(t, dsn, "register-registration-number-types", "-input", corrected); code != exitLanded {
		t.Fatalf("更正批 exit = %d, want %d", code, exitLanded)
	}
	relayered := batchFile(t, singleRegistrationNumberType("3", "PROFILE", "SYN-[0-9]{8}"))
	if code := runCLI(t, dsn, "register-registration-number-types", "-input", relayered); code != exitAttention {
		t.Fatalf("改层批 exit = %d, want %d", code, exitAttention)
	}
	if got := countRegistrationNumberTypeRows(t, dsn, "SYN-LIFETIME"); got != 2 {
		t.Fatalf("SYN-LIFETIME 行数 = %d, want 2（更正落定、改层未落）", got)
	}

	ghost := batchFile(t, `{"tenantId": "tenant-1", "deactivations": [
	  {"countryCode": "XA", "typeCode": "SYN-GHOST", "revision": 2, "basis": "SYN-BASIS-RETIRE", "at": "2026-02-01T00:00:00Z"}]}`)
	if code := runCLI(t, dsn, "register-registration-number-types", "-input", ghost); code != exitAttention {
		t.Fatalf("停用从未登记的类型 exit = %d, want %d", code, exitAttention)
	}
}
