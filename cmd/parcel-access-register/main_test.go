package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipostgres "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件证操作者册受控登记口的两半（票 operator-channel/01 做什么第 3 条）：翻译纪律——批文逐字段过领域
// 构造门，未知字段、缺件、预留格与未知格、倒置区间在触库前拒收，一格不代填；批推进——对真库端到端跑
// operator-register → operator-grant → operator-revoke，证落定、整批重放、撤销生效，以及跨租户主体、
// 未登记主体、不存在的授予、同标识异内容各报请人看且册上不变。

const syntheticIssuer = "https://syn-issuer-01.example.invalid"

func freshMigratedDSN(t *testing.T) string {
	t.Helper()
	dsn := pgtest.FreshDatabase(t)
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("连接测试库：%v", err)
	}
	if err := migrate.Run(ctx, conn, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("施加迁移计划：%v", err)
	}
	if err := conn.Close(ctx); err != nil {
		t.Fatalf("关闭迁移连接：%v", err)
	}
	return dsn
}

func batchFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("写批文：%v", err)
	}
	return path
}

func runCLI(t *testing.T, dsn string, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, func(key string) string {
		if key == envDatabaseDSN {
			return dsn
		}
		return ""
	}, &out, &errOut)
	t.Logf("stdout:\n%s", out.String())
	if errOut.Len() > 0 {
		t.Logf("stderr:\n%s", errOut.String())
	}
	return code, out.String()
}

func standingOf(t *testing.T, dsn, sub string) accessidentity.OperatorStanding {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("开池核对：%v", err)
	}
	t.Cleanup(pool.Close)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registry, err := aipostgres.NewOperatorRegistry(db)
	if err != nil {
		t.Fatalf("构造操作者册：%v", err)
	}
	subject, err := accessidentity.NewOperatorSubject(syntheticIssuer, sub)
	if err != nil {
		t.Fatalf("主体：%v", err)
	}
	standing, found, err := registry.FindOperator(t.Context(), subject)
	if err != nil || !found {
		t.Fatalf("%s 在册：found %v, err %v", sub, found, err)
	}
	return standing
}

const operatorBatch = `{
  "tenantId": "SYN-TENANT-01",
  "operators": [
    {"issuer": "` + syntheticIssuer + `", "subject": "SYN-OPERATOR-01", "basis": "SYN-OPERATOR-BASIS-01"},
    {"issuer": "` + syntheticIssuer + `", "subject": "SYN-OPERATOR-02", "basis": "SYN-OPERATOR-BASIS-02"}
  ]
}`

const grantBatch = `{
  "tenantId": "SYN-TENANT-01",
  "grants": [
    {"grantId": "SYN-GRANT-01", "issuer": "` + syntheticIssuer + `", "subject": "SYN-OPERATOR-01",
     "capabilityFace": "REGISTRY_CONFIGURATION_WRITE", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "SYN-GRANT-BASIS-01"},
    {"grantId": "SYN-GRANT-02", "issuer": "` + syntheticIssuer + `", "subject": "SYN-OPERATOR-01",
     "capabilityFace": "MASTER_DATA_AND_OPERATIONS_READ", "effectiveStartsAt": "2026-01-01T00:00:00Z",
     "effectiveEndsAt": "2026-07-01T00:00:00Z", "basis": "SYN-GRANT-BASIS-02"},
    {"grantId": "SYN-GRANT-03", "issuer": "` + syntheticIssuer + `", "subject": "SYN-OPERATOR-02",
     "capabilityFace": "REGISTRY_CONFIGURATION_WRITE", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "SYN-GRANT-BASIS-03"}
  ]
}`

const revocationBatch = `{
  "tenantId": "SYN-TENANT-01",
  "revocations": [
    {"grantId": "SYN-GRANT-03", "revokedAt": "2026-03-01T00:00:00Z", "basis": "SYN-REVOKE-BASIS-01"}
  ]
}`

// Covers: 票 operator-channel/01 完成判据「CLI 端到端一条」——三个子命令在真库上登记主体、授予、撤销，整批重放
// 以原结果回答；撤销自撤销时刻起生效、区间外不生效；乙租户登甲的主体、授未登记的主体、撤不存在的授予、
// 同标识异内容，各报请人看（退出码 2、答复名如实回显），册上不因它们变。
func TestOperatorRegisterGrantRevokeEndToEnd(t *testing.T) {
	dsn := freshMigratedDSN(t)
	for _, step := range []struct {
		name, subcommand, body string
	}{
		{"登主体", "operator-register", operatorBatch},
		{"授予", "operator-grant", grantBatch},
		{"撤销", "operator-revoke", revocationBatch},
	} {
		path := batchFile(t, step.body)
		if code, out := runCLI(t, dsn, step.subcommand, "-input", path); code != exitLanded || !strings.Contains(out, "RECORDED") {
			t.Fatalf("%s：exit = %d, want %d 且回显 RECORDED", step.name, code, exitLanded)
		}
		if code, out := runCLI(t, dsn, step.subcommand, "-input", path); code != exitLanded || !strings.Contains(out, "ALREADY_REGISTERED") {
			t.Fatalf("%s重放：exit = %d, want %d 且回显 ALREADY_REGISTERED", step.name, code, exitLanded)
		}
	}

	march := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	write := accessidentity.CapabilityRegistryConfigurationWrite
	read := accessidentity.CapabilityMasterDataAndOperationsRead
	first := standingOf(t, dsn, "SYN-OPERATOR-01")
	if !first.HoldsAt(write, march) || !first.HoldsAt(read, march) || first.HoldsAt(read, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("SYN-OPERATOR-01 现状 = %+v, want 两格在三月生效、查阅读到七月起不生效", first)
	}
	second := standingOf(t, dsn, "SYN-OPERATOR-02")
	if !second.HoldsAt(write, march.Add(-time.Second)) || second.HoldsAt(write, march) {
		t.Fatalf("SYN-OPERATOR-02 现状 = %+v, want 撤销时刻前生效、起不生效", second)
	}

	for _, refused := range []struct {
		name, subcommand, body, outcome string
	}{
		{"乙租户登甲的主体", "operator-register",
			`{"tenantId": "SYN-TENANT-02", "operators": [{"issuer": "` + syntheticIssuer + `", "subject": "SYN-OPERATOR-01", "basis": "SYN-OPERATOR-BASIS-09"}]}`,
			"SUBJECT_BOUND_TO_ANOTHER_TENANT"},
		{"乙租户授甲的主体", "operator-grant",
			`{"tenantId": "SYN-TENANT-02", "grants": [{"grantId": "SYN-GRANT-09", "issuer": "` + syntheticIssuer + `", "subject": "SYN-OPERATOR-01",
			  "capabilityFace": "REGISTRY_CONFIGURATION_WRITE", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "SYN-GRANT-BASIS-09"}]}`,
			"OPERATOR_NOT_REGISTERED"},
		{"撤不存在的授予", "operator-revoke",
			`{"tenantId": "SYN-TENANT-01", "revocations": [{"grantId": "SYN-GRANT-GHOST", "revokedAt": "2026-03-01T00:00:00Z", "basis": "SYN-REVOKE-BASIS-09"}]}`,
			"GRANT_NOT_REGISTERED"},
		{"同标识异区间", "operator-grant",
			`{"tenantId": "SYN-TENANT-01", "grants": [{"grantId": "SYN-GRANT-01", "issuer": "` + syntheticIssuer + `", "subject": "SYN-OPERATOR-01",
			  "capabilityFace": "REGISTRY_CONFIGURATION_WRITE", "effectiveStartsAt": "2026-02-01T00:00:00Z", "basis": "SYN-GRANT-BASIS-01"}]}`,
			"CONTENT_CONFLICT"},
	} {
		code, out := runCLI(t, dsn, refused.subcommand, "-input", batchFile(t, refused.body))
		if code != exitAttention || !strings.Contains(out, refused.outcome) {
			t.Fatalf("%s：exit = %d, want %d 且回显 %s", refused.name, code, exitAttention, refused.outcome)
		}
	}
	if after := standingOf(t, dsn, "SYN-OPERATOR-01"); after.Binding().TenantID() != "SYN-TENANT-01" ||
		len(after.Grants()) != 2 || !after.HoldsAt(write, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("被拒之后 SYN-OPERATOR-01 现状 = %+v, want 仍绑甲租户、仍是原两笔授予", after)
	}
}

// Covers: 缺件与形状错在触库之前拒收，绝不代填——没有缺省生效起点、没有空依据，预留的治理登记与未知格
// 各自拒收，倒置区间拒收。
func TestBatchTranslationRefusesBeforeTouchingTheDatabase(t *testing.T) {
	grantItem := func(rest string) string {
		return `{"tenantId": "SYN-TENANT-01", "grants": [{"grantId": "SYN-GRANT-01", "issuer": "` + syntheticIssuer +
			`", "subject": "SYN-OPERATOR-01", ` + rest + `}]}`
	}
	const validGrantRest = `"capabilityFace": "REGISTRY_CONFIGURATION_WRITE", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "SYN-GRANT-BASIS-01"`
	if _, err := grantBatchFromJSON([]byte(grantItem(validGrantRest))); err != nil {
		t.Fatalf("基准授予项应译得出，否则下面的拒收证不了各自那一处：%v", err)
	}
	if _, err := operatorBatchFromJSON([]byte(operatorBatch)); err != nil {
		t.Fatalf("基准主体批应译得出：%v", err)
	}
	if _, err := revocationBatchFromJSON([]byte(revocationBatch)); err != nil {
		t.Fatalf("基准撤销批应译得出：%v", err)
	}

	cases := map[string]struct {
		translate func([]byte) error
		body      string
	}{
		"主体批未知字段": {operators, `{"tenantId": "SYN-TENANT-01", "operators": [{"issuer": "i", "subject": "s", "basis": "b", "name": "张三"}]}`},
		"主体批缺租户":  {operators, `{"operators": [{"issuer": "i", "subject": "s", "basis": "b"}]}`},
		"主体批空数组":  {operators, `{"tenantId": "SYN-TENANT-01", "operators": []}`},
		"主体缺依据":   {operators, `{"tenantId": "SYN-TENANT-01", "operators": [{"issuer": "i", "subject": "s"}]}`},
		"主体缺 sub": {operators, `{"tenantId": "SYN-TENANT-01", "operators": [{"issuer": "i", "basis": "b"}]}`},
		"授予治理登记":  {grants, grantItem(`"capabilityFace": "GOVERNANCE_REGISTRATION", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "b"`)},
		"授予未知格":   {grants, grantItem(`"capabilityFace": "ANYTHING", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "b"`)},
		"授予缺起点":   {grants, grantItem(`"capabilityFace": "REGISTRY_CONFIGURATION_WRITE", "basis": "b"`)},
		"授予倒置区间":  {grants, grantItem(validGrantRest + `, "effectiveEndsAt": "2025-12-31T00:00:00Z"`)},
		"授予缺依据":   {grants, grantItem(`"capabilityFace": "REGISTRY_CONFIGURATION_WRITE", "effectiveStartsAt": "2026-01-01T00:00:00Z"`)},
		"授予批空数组":  {grants, `{"tenantId": "SYN-TENANT-01", "grants": []}`},
		"撤销缺时刻":   {revocations, `{"tenantId": "SYN-TENANT-01", "revocations": [{"grantId": "SYN-GRANT-01", "basis": "b"}]}`},
		"撤销缺依据":   {revocations, `{"tenantId": "SYN-TENANT-01", "revocations": [{"grantId": "SYN-GRANT-01", "revokedAt": "2026-03-01T00:00:00Z"}]}`},
		"撤销批未知字段": {revocations, `{"tenantId": "SYN-TENANT-01", "revocations": [{"grantId": "g", "revokedAt": "2026-03-01T00:00:00Z", "basis": "b", "cause": "x"}]}`},
	}
	for name, probe := range cases {
		if err := probe.translate([]byte(probe.body)); err == nil {
			t.Fatalf("%s：翻译应在触库前拒收", name)
		}
	}
}

func operators(raw []byte) error   { _, err := operatorBatchFromJSON(raw); return err }
func grants(raw []byte) error      { _, err := grantBatchFromJSON(raw); return err }
func revocations(raw []byte) error { _, err := revocationBatchFromJSON(raw); return err }

// Covers: 进程口的用法错误与缺连接串都算技术失败（退出码 1），不静默退 0。
func TestUsageMistakesAreTechnicalFailures(t *testing.T) {
	for name, args := range map[string][]string{
		"无子命令":    nil,
		"未知子命令":   {"operator-delete", "-input", "x.json"},
		"缺 input": {"operator-register"},
	} {
		if code, _ := runCLI(t, "", args...); code != exitTechnical {
			t.Fatalf("%s：exit = %d, want %d", name, code, exitTechnical)
		}
	}
	if code, _ := runCLI(t, "", "operator-register", "-input", batchFile(t, operatorBatch)); code != exitTechnical {
		t.Fatalf("缺连接串：exit = %d, want %d", code, exitTechnical)
	}
}
