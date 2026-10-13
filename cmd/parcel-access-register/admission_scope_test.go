package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipostgres "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// 本文件证准入范围对照册受控登记口的两半（票 operator-channel/19 做什么第 1 件）：翻译纪律——集合外的能力或事实类型、
// 带 SYN- 前缀的能力词、缺件与倒置区间在触库前拒收，一格不代填；批推进——对真库端到端跑
// admission-scope-mapping-register → admission-scope-mapping-end，同键重叠请人看，换版首尾相接登得进去。

const admissionScopeMappingBatch = `{
  "tenantId": "SYN-TENANT-01",
  "mappings": [
    {"mappingId": "SYN-MAP-LOAD-V1", "capability": "OPERATION_DECISION", "factKind": "LOAD_ASSIGNMENT",
     "objectScope": "SYN-PILOT-SCOPE/operation-decisions@v1", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "SYN-MAP-BASIS-01"},
    {"mappingId": "SYN-MAP-FUNDS-V1", "capability": "EXTERNAL_FACT", "factKind": "EXTERNAL_FUNDS_FACT",
     "objectScope": "SYN-PILOT-SCOPE/external-facts@v1", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "SYN-MAP-BASIS-02"}
  ]
}`

const admissionScopeMappingNextVersionBatch = `{
  "tenantId": "SYN-TENANT-01",
  "mappings": [
    {"mappingId": "SYN-MAP-LOAD-V2", "capability": "OPERATION_DECISION", "factKind": "LOAD_ASSIGNMENT",
     "objectScope": "SYN-PILOT-SCOPE/operation-decisions@v2", "effectiveStartsAt": "2026-03-01T00:00:00Z", "basis": "SYN-MAP-BASIS-03"}
  ]
}`

const admissionScopeMappingEndBatch = `{
  "tenantId": "SYN-TENANT-01",
  "ends": [{"mappingId": "SYN-MAP-LOAD-V1", "endedAt": "2026-03-01T00:00:00Z", "basis": "SYN-MAP-END-BASIS-01"}]
}`

func mappingsOf(t *testing.T, dsn string, requirement accessidentity.AdmissionRequirement) []accessidentity.RecordedAdmissionScopeMapping {
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
	mappings, err := aipostgres.NewAdmissionScopeMappings(db)
	if err != nil {
		t.Fatalf("构造对照册：%v", err)
	}
	found, err := mappings.FindAdmissionScopeMappings(t.Context(), "SYN-TENANT-01", requirement)
	if err != nil {
		t.Fatalf("读对照：%v", err)
	}
	return found
}

// Covers: 对照的登记、整批重放、同键重叠请人看（退出 2、册上不变），结束旧行之后自切换时刻起的新行登得进去；读回的
// 两行在切换前后各解出各自的对象范围。
func TestAdmissionScopeMappingRegisterAndEndEndToEnd(t *testing.T) {
	dsn := freshMigratedDSN(t)
	mappingsFile := batchFile(t, admissionScopeMappingBatch)
	nextFile := batchFile(t, admissionScopeMappingNextVersionBatch)
	endFile := batchFile(t, admissionScopeMappingEndBatch)

	if code, out := runCLI(t, dsn, "admission-scope-mapping-register", "-input", mappingsFile); code != exitLanded || strings.Count(out, "：RECORDED") != 2 {
		t.Fatalf("首登：exit = %d", code)
	}
	if code, out := runCLI(t, dsn, "admission-scope-mapping-register", "-input", mappingsFile); code != exitLanded || strings.Count(out, "：ALREADY_REGISTERED") != 2 {
		t.Fatalf("整批重放：exit = %d", code)
	}
	if code, out := runCLI(t, dsn, "admission-scope-mapping-register", "-input", nextFile); code != exitAttention || !strings.Contains(out, "：MAPPING_OVERLAPS") {
		t.Fatalf("旧行未结束时登新版：exit = %d", code)
	}
	if code, out := runCLI(t, dsn, "admission-scope-mapping-end", "-input", endFile); code != exitLanded || !strings.Contains(out, "：RECORDED") {
		t.Fatalf("结束旧行：exit = %d", code)
	}
	if code, out := runCLI(t, dsn, "admission-scope-mapping-register", "-input", nextFile); code != exitLanded || !strings.Contains(out, "：RECORDED") {
		t.Fatalf("切换时刻起登新版：exit = %d", code)
	}
	unknownEnd := batchFile(t, `{"tenantId": "SYN-TENANT-01", "ends": [{"mappingId": "SYN-MAP-NONE", "endedAt": "2026-03-01T00:00:00Z", "basis": "b"}]}`)
	if code, out := runCLI(t, dsn, "admission-scope-mapping-end", "-input", unknownEnd); code != exitAttention || !strings.Contains(out, "：MAPPING_NOT_REGISTERED") {
		t.Fatalf("结束未登的行：exit = %d", code)
	}

	load := accessidentity.AdmissionRequirement{
		Capability: accessidentity.CapabilityOperationDecision.String(),
		FactKind:   accessidentity.DecisionLoadAssignment.String(),
	}
	found := mappingsOf(t, dsn, load)
	for at, want := range map[time.Time]string{
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC): "SYN-PILOT-SCOPE/operation-decisions@v1",
		time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC): "SYN-PILOT-SCOPE/operation-decisions@v2",
	} {
		scope, resolved, err := accessidentity.ResolveAdmissionObjectScope(found, at)
		if err != nil || !resolved || scope != want {
			t.Fatalf("%s 解出 %q（%v, %v），want %q", at, scope, resolved, err, want)
		}
	}
}

// Covers: ADR-0180 决定四——能力与事实类型照认证方的词写；带 SYN- 前缀的能力词、集合外的决定种类、能力与事实类型配错族，
// 以及缺件、倒置区间、零值终点、未知字段，都在触库之前拒收，一格不代填。
func TestAdmissionScopeMappingTranslationRefusesBeforeTouchingTheDatabase(t *testing.T) {
	if _, err := admissionScopeMappingBatchFromJSON([]byte(admissionScopeMappingBatch)); err != nil {
		t.Fatalf("基准对照批应译得出，否则下面的拒收证不了各自那一处：%v", err)
	}
	if _, err := admissionScopeMappingEndBatchFromJSON([]byte(admissionScopeMappingEndBatch)); err != nil {
		t.Fatalf("基准结束批应译得出：%v", err)
	}
	item := func(rest string) string {
		return `{"tenantId": "SYN-TENANT-01", "mappings": [{"mappingId": "SYN-MAP-X", ` + rest + `}]}`
	}
	const scopeAndBasis = `"objectScope": "SYN-PILOT-SCOPE/x@v1", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "b"`
	mapping := func(raw []byte) error { _, err := admissionScopeMappingBatchFromJSON(raw); return err }
	ending := func(raw []byte) error { _, err := admissionScopeMappingEndBatchFromJSON(raw); return err }
	cases := map[string]struct {
		translate func([]byte) error
		body      string
	}{
		"合成能力词":      {mapping, item(`"capability": "SYN-CAP/operation-decision", "factKind": "LOAD_ASSIGNMENT", ` + scopeAndBasis)},
		"未知决定种类":     {mapping, item(`"capability": "OPERATION_DECISION", "factKind": "APPROVE_EVERYTHING", ` + scopeAndBasis)},
		"能力与事实类型配错族": {mapping, item(`"capability": "EXTERNAL_FACT", "factKind": "LOAD_ASSIGNMENT", ` + scopeAndBasis)},
		"缺对象范围":      {mapping, item(`"capability": "OPERATION_DECISION", "factKind": "LOAD_ASSIGNMENT", "effectiveStartsAt": "2026-01-01T00:00:00Z", "basis": "b"`)},
		"缺起点":        {mapping, item(`"capability": "OPERATION_DECISION", "factKind": "LOAD_ASSIGNMENT", "objectScope": "SYN-PILOT-SCOPE/x@v1", "basis": "b"`)},
		"倒置区间":       {mapping, item(`"capability": "OPERATION_DECISION", "factKind": "LOAD_ASSIGNMENT", ` + scopeAndBasis + `, "effectiveEndsAt": "2025-12-31T00:00:00Z"`)},
		"零值终点":       {mapping, item(`"capability": "OPERATION_DECISION", "factKind": "LOAD_ASSIGNMENT", ` + scopeAndBasis + `, "effectiveEndsAt": "0001-01-01T00:00:00Z"`)},
		"缺依据":        {mapping, item(`"capability": "OPERATION_DECISION", "factKind": "LOAD_ASSIGNMENT", "objectScope": "SYN-PILOT-SCOPE/x@v1", "effectiveStartsAt": "2026-01-01T00:00:00Z"`)},
		"对照批未知字段":    {mapping, item(`"capability": "OPERATION_DECISION", "factKind": "LOAD_ASSIGNMENT", ` + scopeAndBasis + `, "authority": "SYN-AUTH/x"`)},
		"对照批缺租户":     {mapping, `{"mappings": [{"mappingId": "SYN-MAP-X", "capability": "OPERATION_DECISION", "factKind": "LOAD_ASSIGNMENT", ` + scopeAndBasis + `}]}`},
		"对照批空数组":     {mapping, `{"tenantId": "SYN-TENANT-01", "mappings": []}`},
		"结束缺时刻":      {ending, `{"tenantId": "SYN-TENANT-01", "ends": [{"mappingId": "SYN-MAP-X", "basis": "b"}]}`},
		"结束缺依据":      {ending, `{"tenantId": "SYN-TENANT-01", "ends": [{"mappingId": "SYN-MAP-X", "endedAt": "2026-03-01T00:00:00Z"}]}`},
	}
	for name, probe := range cases {
		if err := probe.translate([]byte(probe.body)); err == nil {
			t.Fatalf("%s：翻译应在触库前拒收", name)
		}
	}
}

// Covers: ADR-0180 越权风险点 2——用法里写明一行对照会把这个租户该类生产写全部放进那一个对象范围，登之前就看得见。
func TestTheUsageStatesWhenAMappingMayBeRegistered(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(context.Background(), nil, func(string) string { return "" }, &out, &errOut); code != exitTechnical {
		t.Fatalf("无子命令：exit = %d", code)
	}
	if !strings.Contains(errOut.String(), "admission-scope-mapping-register：只在这个租户该（能力、事实类型）的生产写全部落在那一个对象范围时才登对照") {
		t.Fatalf("用法里没有登对照的前提：\n%s", errOut.String())
	}
}
