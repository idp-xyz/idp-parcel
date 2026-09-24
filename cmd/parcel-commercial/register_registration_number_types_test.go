package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
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

const adoptedRegistrationNumberTypeBatchJSON = `{
  "tenantId": "tenant-1",
  "types": [{
    "countryCode": "CN", "typeCode": "USCC", "revision": 1,
    "adopt": "party-commercial/registration-number-types/CN@1", "effectiveFrom": "2026-01-01T00:00:00Z"
  }]
}`

const adoptedRegistrationNumberTypeCitation = "REFCFG-1:party-commercial/registration-number-types/CN@1"

// capturingRegistrationNumberTypes 只收下交来的修订，供翻译纪律的用例核对落进登记册的是什么。
type capturingRegistrationNumberTypes struct {
	saved []pcdomain.RegistrationNumberTypeRegistration
}

func (registry *capturingRegistrationNumberTypes) SaveRegistrationNumberType(
	_ context.Context,
	registration pcdomain.RegistrationNumberTypeRegistration,
) (pcports.RegistrationNumberTypeSaveOutcome, error) {
	registry.saved = append(registry.saved, registration)
	return pcports.RegistrationNumberTypeRegistrySaved, nil
}

func (*capturingRegistrationNumberTypes) LoadLatestRegistrationNumberType(
	context.Context,
	pcdomain.TenantID,
	pcdomain.RegistrationCountryCode,
	pcdomain.RegistrationNumberTypeCode,
) (pcdomain.RegistrationNumberTypeRegistration, bool, error) {
	return pcdomain.RegistrationNumberTypeRegistration{}, false, nil
}

// Covers: ADR-0147 决定四——带 adopt 的一项是一次普通登记：名称、层与格式取自参考配置，依据格由入口
// 写成引用串；修订号与生效时点照旧由批文给。
func TestAnAdoptItemRegistersTheReferenceContentAndCitesItsVersion(t *testing.T) {
	commands, err := registrationNumberTypeBatchFromJSON([]byte(adoptedRegistrationNumberTypeBatchJSON))
	if err != nil {
		t.Fatalf("翻译采用批：%v", err)
	}
	wantLabel := "注册号类型 CN/USCC r1（采用 party-commercial/registration-number-types/CN@1）"
	if len(commands) != 1 || commands[0].label != wantLabel {
		t.Fatalf("命令 = %d 项，首项标签 %q, want %q", len(commands), commands[0].label, wantLabel)
	}

	registry := &capturingRegistrationNumberTypes{}
	result, err := commands[0].execute(context.Background(), pcapplication.NewRegisterRegistrationNumberTypeHandler(registry))
	if err != nil || result.Outcome() != pcapplication.RegistrationNumberTypeRegistered {
		t.Fatalf("执行采用项 = %s, %v", result.Outcome(), err)
	}
	if len(registry.saved) != 1 {
		t.Fatalf("落进登记册 %d 笔, want 1", len(registry.saved))
	}
	saved := registry.saved[0]
	if saved.Basis().String() != adoptedRegistrationNumberTypeCitation {
		t.Fatalf("依据 = %q, want %q", saved.Basis(), adoptedRegistrationNumberTypeCitation)
	}
	if saved.Name().String() != "统一社会信用代码" || saved.Layer() != pcdomain.RegistrationNumberIdentityLayer {
		t.Fatalf("名称 / 层 = %q / %s，应取自参考配置", saved.Name(), saved.Layer())
	}
	if saved.Revision() != 1 || !saved.Lifecycle().EffectiveFrom().Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("修订 / 生效 = %d / %s，应取自批文", saved.Revision(), saved.Lifecycle().EffectiveFrom())
	}
}

// Covers: 采用项的内容只来自参考配置——批文再写名称、层、格式或依据即拒收，免得一份「采用」记录里
// 混着租户自己的格；版本未发布、参考配置里没有这一类型、国家 / 地区与参考配置的键不符、引用形状不对、
// 缺生效时点，都在触库前拒收。
func TestAnAdoptItemRefusesContentOfItsOwnAndReferencesThatDoNotResolve(t *testing.T) {
	adopt := func(from, to string) string {
		return strings.Replace(adoptedRegistrationNumberTypeBatchJSON, from, to, 1)
	}
	cases := map[string]string{
		"另写名称":       adopt(`"revision": 1,`, `"revision": 1, "name": "租户自拟",`),
		"另写层":        adopt(`"revision": 1,`, `"revision": 1, "layer": "IDENTITY",`),
		"另写格式":       adopt(`"revision": 1,`, `"revision": 1, "format": "[0-9]{18}",`),
		"另写依据":       adopt(`"revision": 1,`, `"revision": 1, "basis": "SYN-BASIS-01",`),
		"未发布的版本":     adopt(`CN@1`, `CN@2`),
		"参考配置里没有该类型": adopt(`"typeCode": "USCC"`, `"typeCode": "SYN-CN-LIFETIME"`),
		"国家与键不符":     adopt(`"countryCode": "CN"`, `"countryCode": "SG"`),
		"引用形状不对":     adopt(`party-commercial/registration-number-types/CN@1`, `CN@1`),
		"缺生效时点":      adopt(`, "effectiveFrom": "2026-01-01T00:00:00Z"`, ``),
	}
	for name, body := range cases {
		if _, err := registrationNumberTypeBatchFromJSON([]byte(body)); err == nil {
			t.Fatalf("%s：翻译应在触库前拒收", name)
		}
	}
}

// Covers: 票 product-strategy-boundary/03 完成判据，端到端于真库——参考配置经采用路径进入采用方的
// 注册号类型目录，依据格指向参考配置版本，按目录判号接受合格样例；重放以原结果回答；没采用的租户
// 同一国家 / 地区照旧答`未登记`。
func TestAdoptingAReferenceLandsItInTheAdoptersCatalogueOnly(t *testing.T) {
	dsn := freshMigratedDSN(t)

	batch := batchFile(t, adoptedRegistrationNumberTypeBatchJSON)
	for _, round := range []string{"首次采用", "重放"} {
		if code := runCLI(t, dsn, "register-registration-number-types", "-input", batch); code != exitLanded {
			t.Fatalf("%s exit = %d, want %d", round, code, exitLanded)
		}
	}

	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("开池核对：%v", err)
	}
	defer pool.Close()
	var basis string
	if err := pool.QueryRow(t.Context(),
		`SELECT basis_ref FROM party_commercial.registration_number_type_registration
		  WHERE tenant_id = $1 AND country_code = 'CN' AND type_code = 'USCC' AND revision = 1`,
		"tenant-1",
	).Scan(&basis); err != nil {
		t.Fatalf("读采用记录：%v", err)
	}
	if basis != adoptedRegistrationNumberTypeCitation {
		t.Fatalf("采用记录的依据 = %q, want %q", basis, adoptedRegistrationNumberTypeCitation)
	}

	db, cleanup, err := openDatabase(t.Context(), func(key string) string {
		if key == envDatabaseDSN {
			return dsn
		}
		return ""
	})
	if err != nil {
		t.Fatalf("开库：%v", err)
	}
	defer cleanup()
	lookup, err := pcpostgres.NewRegistrationNumberTypes(db)
	if err != nil {
		t.Fatalf("构造目录读口：%v", err)
	}
	country, _ := pcdomain.NewRegistrationCountryCode("CN")
	code, _ := pcdomain.NewRegistrationNumberTypeCode("USCC")
	number, _ := pcdomain.NewRegistrationNumber("91000000000000001X")
	at := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	for tenantValue, want := range map[string]pcdomain.RegistrationNumberCheckOutcome{
		"tenant-1": pcdomain.RegistrationNumberAccepted,
		"tenant-2": pcdomain.RegistrationCountryNotRegistered,
	} {
		tenant, _ := pcdomain.NewTenantID(tenantValue)
		catalogue, err := lookup.LoadRegistrationNumberTypeCatalogue(t.Context(), tenant, country)
		if err != nil {
			t.Fatalf("%s 取目录：%v", tenantValue, err)
		}
		check, err := catalogue.Check(code, pcdomain.RegistrationNumberIdentityLayer, number, at)
		if err != nil || check.Outcome() != want {
			t.Fatalf("%s 判号 = %s, %v, want %s", tenantValue, check.Outcome(), err, want)
		}
	}
}
