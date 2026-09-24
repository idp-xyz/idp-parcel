package main

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// 本文件证法人资料登记进程口的两半（票 legal-entity-profile/03）：
// 翻译纪律——批文逐字段过领域构造门，未知字段、缺注册地址、空地址行、形状不对的国家 / 地区、缺席的时点、给了空串的
// 抬头、没名字的联系人与缺类型的税号在触库前拒收，一格不代填；抬头缺席译成「不带开票资料」，不是空抬头；
// 批推进——对真库端到端跑 register-legal-entity-profiles，证两笔修订落定、整批重放、同修订异内容冲突，以及地址国家
// 对不上法人身份、法人未登记各报请人看。

const legalEntityProfileBatchJSON = `{
  "tenantId": "tenant-1",
  "profiles": [
    {
      "legalEntityId": "legal-acme", "revision": 1, "basis": "SYN-PROFILE-BASIS-01", "effectiveFrom": "2026-01-02T00:00:00Z",
      "registeredAddress": {"country": "XA", "lines": ["SYN 一号路 1 号"]},
      "taxRegistrationNumbers": [{"typeCode": "SYN-TAX", "number": "SYN-TAX-0001"}],
      "contacts": [{"name": "SYN 联系人", "email": "syn@example.invalid"}]
    },
    {
      "legalEntityId": "legal-acme", "revision": 2, "basis": "SYN-PROFILE-BASIS-02", "effectiveFrom": "2026-03-01T00:00:00Z",
      "registeredAddress": {"country": "XA", "lines": ["SYN 一号路 1 号"]},
      "taxRegistrationNumbers": [{"typeCode": "SYN-TAX", "number": "SYN-TAX-0001"}],
      "invoiceTitle": "SYN Acme 抬头"
    }
  ]
}`

// legalEntityProfileSetupTypes 与 legalEntityProfileSetupParties 给端到端用例铺底：XA 两层各一类型、一个带 XA 身份层的
// 法人。不复用注册号类型用例的批：那一批停用了税号类型，资料修订按生效时点判号会撞上「不在用」。
const legalEntityProfileSetupTypes = `{
  "tenantId": "tenant-1",
  "types": [
    {"countryCode": "XA", "typeCode": "SYN-LIFETIME", "revision": 1, "name": "合成终身注册号", "layer": "IDENTITY",
     "format": "SYN-[0-9]{6}", "basis": "SYN-BASIS-01", "effectiveFrom": "2026-01-01T00:00:00Z"},
    {"countryCode": "XA", "typeCode": "SYN-TAX", "revision": 1, "name": "合成税务登记号", "layer": "PROFILE",
     "format": "SYN-TAX-[0-9]{4}", "basis": "SYN-BASIS-02", "effectiveFrom": "2026-01-01T00:00:00Z"}
  ]
}`

const legalEntityProfileSetupParties = `{
  "tenantId": "tenant-1",
  "businessParties": [
    {"partyId": "party-acme", "name": "Acme 物流", "revision": 1, "basis": "REG/party-acme", "effectiveFrom": "2026-01-01T00:00:00Z"}
  ],
  "legalEntities": [
    {"legalEntityId": "legal-acme", "partyId": "party-acme", "revision": 1, "basis": "REG/legal-acme", "effectiveFrom": "2026-01-02T00:00:00Z",
     "registrationCountry": "XA", "lifetimeRegistrationNumbers": [{"typeCode": "SYN-LIFETIME", "number": "SYN-000001"}]}
  ]
}`

// validProfileRest 是一项资料里除法人、修订与依据之外的最小合格各格；拒收用例各在它上面改一处。
const validProfileRest = `"effectiveFrom": "2026-01-02T00:00:00Z", "registeredAddress": {"country": "XA", "lines": ["SYN 一号路 1 号"]}`

func profileBatchOf(rest string) string {
	return `{"tenantId": "tenant-1", "profiles": [{"legalEntityId": "legal-acme", "revision": 1, "basis": "SYN-PROFILE-BASIS-01", ` + rest + `}]}`
}

// singleLegalEntityProfile 造一份单项批：entity 的第 revision 笔，地址落在 country。
func singleLegalEntityProfile(entity, revision, country, line string) string {
	return `{
  "tenantId": "tenant-1",
  "profiles": [{
    "legalEntityId": "` + entity + `", "revision": ` + revision + `, "basis": "SYN-PROFILE-BASIS-r` + revision + `",
    "effectiveFrom": "2026-04-01T00:00:00Z",
    "registeredAddress": {"country": "` + country + `", "lines": ["` + line + `"]}
  }]
}`
}

func countLegalEntityProfileRows(t *testing.T, dsn, entity string) int {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("开池核对：%v", err)
	}
	defer pool.Close()

	var count int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM party_commercial.legal_entity_profile_revision WHERE tenant_id = $1 AND legal_entity_id = $2`,
		"tenant-1", entity,
	).Scan(&count); err != nil {
		t.Fatalf("统计资料修订行：%v", err)
	}
	return count
}

// Covers: 翻译产物按文件内次序排列，标签带法人与修订；抬头缺席译成不带开票资料，给了即带；税号与联系人逐项译入。
func TestLegalEntityProfileBatchTranslationKeepsOrder(t *testing.T) {
	commands, err := legalEntityProfileBatchFromJSON([]byte(legalEntityProfileBatchJSON))
	if err != nil {
		t.Fatalf("翻译完整批：%v", err)
	}
	wantLabels := []string{"法人资料 legal-acme r1", "法人资料 legal-acme r2"}
	if len(commands) != len(wantLabels) {
		t.Fatalf("命令数 = %d, want %d", len(commands), len(wantLabels))
	}
	for index, want := range wantLabels {
		if commands[index].label != want {
			t.Fatalf("第 %d 项标签 = %q, want %q", index+1, commands[index].label, want)
		}
	}
	first, second := commands[0].command, commands[1].command
	if first.Invoicing != nil {
		t.Fatal("r1 没给 invoiceTitle，应译成不带开票资料")
	}
	if second.Invoicing == nil || second.Invoicing.Title().String() != "SYN Acme 抬头" {
		t.Fatalf("r2 开票资料 = %+v, want 抬头「SYN Acme 抬头」", second.Invoicing)
	}
	if len(first.TaxNumbers) != 1 || len(first.Contacts) != 1 || len(second.Contacts) != 0 {
		t.Fatalf("税号 / 联系人条数 = %d / %d / %d, want 1 / 1 / 0", len(first.TaxNumbers), len(first.Contacts), len(second.Contacts))
	}
}

// Covers: 缺件与形状错在触库之前拒收，绝不代填——没有「默认地址」「当前时刻」，空抬头也不当缺席。
func TestLegalEntityProfileBatchTranslationRefusesBeforeTouchingTheDatabase(t *testing.T) {
	if _, err := legalEntityProfileBatchFromJSON([]byte(profileBatchOf(validProfileRest))); err != nil {
		t.Fatalf("基准项应译得出，否则下面的拒收证不了各自那一处：%v", err)
	}
	cases := map[string]string{
		"未知字段":   profileBatchOf(validProfileRest + `, "nickname": "x"`),
		"缺注册地址":  profileBatchOf(`"effectiveFrom": "2026-01-02T00:00:00Z"`),
		"空地址行":   profileBatchOf(`"effectiveFrom": "2026-01-02T00:00:00Z", "registeredAddress": {"country": "XA", "lines": []}`),
		"小写国家":   profileBatchOf(`"effectiveFrom": "2026-01-02T00:00:00Z", "registeredAddress": {"country": "xa", "lines": ["SYN 一号路 1 号"]}`),
		"缺生效时点":  profileBatchOf(`"registeredAddress": {"country": "XA", "lines": ["SYN 一号路 1 号"]}`),
		"空抬头":    profileBatchOf(validProfileRest + `, "invoiceTitle": ""`),
		"联系人没名字": profileBatchOf(validProfileRest + `, "contacts": [{"email": "syn@example.invalid"}]`),
		"税号缺类型":  profileBatchOf(validProfileRest + `, "taxRegistrationNumbers": [{"number": "SYN-TAX-0001"}]`),
		"空批":     `{"tenantId": "tenant-1"}`,
		"空数组":    `{"tenantId": "tenant-1", "profiles": []}`,
	}
	for name, body := range cases {
		if _, err := legalEntityProfileBatchFromJSON([]byte(body)); err == nil {
			t.Fatalf("%s：翻译应在触库前拒收", name)
		}
	}
}

// Covers: 票 legal-entity-profile/03 登记面的进程口半边，端到端于真库——两笔修订（先不带开票资料、后带）落定；重放整批
// 以原结果回答不追加行；同修订异内容报请人看；地址国家对不上法人身份、法人未登记各报请人看，都不落行。
func TestRegisterLegalEntityProfilesBatchLandsRepliesAndRefuses(t *testing.T) {
	dsn := freshMigratedDSN(t)
	if code := runCLI(t, dsn, "register-registration-number-types", "-input", batchFile(t, legalEntityProfileSetupTypes)); code != exitLanded {
		t.Fatalf("铺注册号类型 exit = %d, want %d", code, exitLanded)
	}
	if code := runCLI(t, dsn, "register-parties", "-input", batchFile(t, legalEntityProfileSetupParties)); code != exitLanded {
		t.Fatalf("铺法人 exit = %d, want %d", code, exitLanded)
	}

	batch := batchFile(t, legalEntityProfileBatchJSON)
	if code := runCLI(t, dsn, "register-legal-entity-profiles", "-input", batch); code != exitLanded {
		t.Fatalf("首批 exit = %d, want %d", code, exitLanded)
	}
	if code := runCLI(t, dsn, "register-legal-entity-profiles", "-input", batch); code != exitLanded {
		t.Fatalf("重放批 exit = %d, want %d", code, exitLanded)
	}
	if got := countLegalEntityProfileRows(t, dsn, "legal-acme"); got != 2 {
		t.Fatalf("legal-acme 资料修订行数 = %d, want 2（重放不追加）", got)
	}

	for name, body := range map[string]string{
		"同修订异内容":  singleLegalEntityProfile("legal-acme", "1", "XA", "SYN 二号路 2 号"),
		"地址国家对不上": singleLegalEntityProfile("legal-acme", "3", "XB", "SYN 境外路 1 号"),
		"法人未登记":   singleLegalEntityProfile("legal-ghost", "1", "XA", "SYN 一号路 1 号"),
	} {
		if code := runCLI(t, dsn, "register-legal-entity-profiles", "-input", batchFile(t, body)); code != exitAttention {
			t.Fatalf("%s exit = %d, want %d", name, code, exitAttention)
		}
	}
	if got := countLegalEntityProfileRows(t, dsn, "legal-acme"); got != 2 {
		t.Fatalf("legal-acme 资料修订行数 = %d, want 2（冲突与被拒都不落行）", got)
	}
	if got := countLegalEntityProfileRows(t, dsn, "legal-ghost"); got != 0 {
		t.Fatalf("legal-ghost 资料修订行数 = %d, want 0", got)
	}
}
