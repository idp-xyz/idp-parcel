package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber/docnumberpg"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件证单据编号规则册的受控登记口（ADR-0085 决定一；ADR-0355 决定六、七）：演示种子那一版单据编号时区照 seed.sh
// 原样登进真库；退出码按恢复动作分格；与 parcel-api 的登记口是同一个编排、同一本册。夹具全是 SYN 合成值（S 级）。

var demoTimeZoneSeed = filepath.Join("..", "..", "scripts", "demo-seeds", "data", "document-numbering", "01-numbering-time-zone-syn-tenant-01.json")

func newRegistrar(t *testing.T, secrets docnumber.ScrambleSecrets) (registrar, *bentopg.DB) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	built, err := buildRegistrar(db, secrets)
	if err != nil {
		t.Fatalf("装配登记口：%v", err)
	}
	return built, db
}

// Covers: 演示判据——种子里演示租户那一版单据编号时区照 seed.sh 原样登进真库答已登记、重灌答已有结果（退出码都是 0，
// seed.sh 重跑不断）；登进去的那一版按时点读得回，登记人记的是本口。证据层级只记 S。
func TestTheDemoTimeZoneSeedRegistersOnARealDatabase(t *testing.T) {
	built, db := newRegistrar(t, docnumber.SecretTable{})
	raw, err := os.ReadFile(demoTimeZoneSeed)
	if err != nil {
		t.Fatalf("读演示种子：%v", err)
	}
	ctx := t.Context()
	if message, code := execute(ctx, kindTimeZone, raw, built); code != exitRegistered || !strings.Contains(message, "VERSION_REGISTERED") {
		t.Fatalf("首灌答 %q（%d）", message, code)
	}
	if message, code := execute(ctx, kindTimeZone, raw, built); code != exitRegistered || !strings.Contains(message, "VERSION_EXISTING") {
		t.Fatalf("重灌答 %q（%d）", message, code)
	}

	store, err := docnumberpg.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	version, found, err := store.EffectiveTimeZoneVersion(ctx, "SYN-TENANT-01", time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC))
	if err != nil || !found || version.TimeZone != "Asia/Shanghai" || version.RegisteredBy != registeredBy {
		t.Fatalf("登进去的那一版 = %+v %v %v", version, found, err)
	}
	if _, found, _ := store.EffectiveTimeZoneVersion(ctx, "SYN-TENANT-01", time.Date(2025, 12, 31, 15, 59, 0, 0, time.UTC)); found {
		t.Fatal("生效时刻之前就读得到这一版")
	}
}

// Covers: ADR-0029 退出码分格——同一生效时刻改登另一个时区答 2（版本只增不改，要人另选生效时刻）；不认识的时区、缺生效
// 时刻、未知字段、未知登记族答 1；规则族：首登 0，用加扰而该租户没配密钥答 1，配了答 0。
func TestExitCodesFollowTheRecoveryAction(t *testing.T) {
	built, _ := newRegistrar(t, docnumber.SecretTable{"SYN-TENANT-02": "syn-scramble-secret-0123456789abcdef"})
	ctx := t.Context()

	cases := []struct {
		name string
		kind string
		raw  string
		code int
	}{
		{"time zone", kindTimeZone, `{"tenantId":"SYN-TENANT-01","timeZone":"Asia/Shanghai","effectiveFrom":"2026-01-01T00:00:00+08:00"}`, exitRegistered},
		{"another zone at the same moment", kindTimeZone, `{"tenantId":"SYN-TENANT-01","timeZone":"Asia/Singapore","effectiveFrom":"2026-01-01T00:00:00+08:00"}`, exitTaken},
		{"unknown zone", kindTimeZone, `{"tenantId":"SYN-TENANT-01","timeZone":"Mars/Olympus","effectiveFrom":"2026-02-01T00:00:00Z"}`, exitUsage},
		{"no effective moment", kindTimeZone, `{"tenantId":"SYN-TENANT-01","timeZone":"Asia/Shanghai"}`, exitUsage},
		{"unknown field", kindTimeZone, `{"tenantId":"SYN-TENANT-01","timeZone":"Asia/Shanghai","effectiveFrom":"2026-02-01T00:00:00Z","zone":"x"}`, exitUsage},
		{"no tenant", kindTimeZone, `{"timeZone":"Asia/Shanghai","effectiveFrom":"2026-02-01T00:00:00Z"}`, exitUsage},
		{"rule", kindRule, `{"tenantId":"SYN-TENANT-01","documentType":"BOOKING","template":"SYN-BK-{yyyy}{mm}-{seq:5}","effectiveFrom":"2026-02-01T00:00:00Z"}`, exitRegistered},
		{"undeclared type", kindRule, `{"tenantId":"SYN-TENANT-01","documentType":"INVOICE","template":"INV-{seq:5}","effectiveFrom":"2026-02-01T00:00:00Z"}`, exitUsage},
		{"scrambled without a secret", kindRule, `{"tenantId":"SYN-TENANT-01","documentType":"CLAIM","template":"CLM-{seqx:6}","effectiveFrom":"2026-02-01T00:00:00Z"}`, exitUsage},
		{"scrambled with a secret", kindRule, `{"tenantId":"SYN-TENANT-02","documentType":"CLAIM","template":"CLM-{seqx:6}","effectiveFrom":"2026-02-01T00:00:00Z"}`, exitRegistered},
		{"unknown kind", "invoice", `{}`, exitUsage},
	}
	for _, testCase := range cases {
		if message, code := execute(ctx, testCase.kind, []byte(testCase.raw), built); code != testCase.code {
			t.Errorf("%s: 答 %q（%d），要 %d", testCase.name, message, code, testCase.code)
		}
	}
}
