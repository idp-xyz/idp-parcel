package main

import (
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

func mustInstant(t *testing.T, value string) time.Time {
	t.Helper()
	instant, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("解析时刻 %q：%v", value, err)
	}
	return instant
}

// 本文件对真实 PostgreSQL 16 证 buildRegistrar 装配的整条登记链（隔离合成 S）：八本
// 册子各自贯通「译装 → 用例 → 真库」，重放与内容冲突在真库上分得开，撤销走状态推进
// 且原判断留在行内，义务明细撞上库的外键防线时答未决。本口与写口适配器各持一份私有
// 词表映射，本用例同时把两份钉在迁移 CHECK 的同一词表上。
func TestCustomsRegisterVerticalOnRealPostgres(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registrar, err := buildRegistrar(db)
	if err != nil {
		t.Fatalf("装配登记口：%v", err)
	}
	ctx := t.Context()

	mustExecute := func(command string, raw string, wantCode int, wantAnswer string) string {
		t.Helper()
		message, code := execute(ctx, command, []byte(raw), registrar)
		if code != wantCode {
			t.Fatalf("%s 退出码 = %d（%s），要 %d", command, code, message, wantCode)
		}
		if wantAnswer != "" && !strings.Contains(message, wantAnswer) {
			t.Fatalf("%s 答复 = %q，要含 %s", command, message, wantAnswer)
		}
		return message
	}

	// 就绪：登记 → 重放 → 换依据冲突 → 撤销 → 再撤。
	readiness := func(basis string) string {
		return `{
			"tenantId": "SYN-T1", "unitId": "SYN-UNIT-1",
			"basisRef": "` + basis + `", "judgedAt": "2026-08-24T01:00:00Z"
		}`
	}
	mustExecute(commandReadinessRegister, readiness("SYN-BASIS-1"), exitRegistered, "REGISTERED")
	mustExecute(commandReadinessRegister, readiness("SYN-BASIS-1"), exitRegistered, "EXISTING")
	mustExecute(commandReadinessRegister, readiness("SYN-BASIS-2"), exitConflict, "CONTENT_CONFLICT")
	revoke := `{
		"tenantId": "SYN-T1", "unitId": "SYN-UNIT-1",
		"cause": "SYN-RULE-CHANGE", "at": "2026-08-24T02:00:00Z"
	}`
	mustExecute(commandReadinessRevoke, revoke, exitRegistered, "REVOKED")
	mustExecute(commandReadinessRevoke, revoke, exitRegistered, "ALREADY_REVOKED")

	// 撤销是状态推进不是删除：行还在、原依据原样、失效可见（CONTEXT「原判断保留，但不得继续支持实际提交」「提交授权与就绪判断分别形成和失效」）。
	readinessView, err := adapter.NewReadinessView(db)
	if err != nil {
		t.Fatalf("构造就绪读口：%v", err)
	}
	tenant, err := domain.NewTenantID("SYN-T1")
	if err != nil {
		t.Fatalf("构造租户标识：%v", err)
	}
	unit, err := domain.NewDeclarationUnitID("SYN-UNIT-1")
	if err != nil {
		t.Fatalf("构造单元标识：%v", err)
	}
	judgment, found, err := readinessView.LoadReadiness(ctx, tenant, unit)
	if err != nil || !found {
		t.Fatalf("撤销后的判断不在册（found=%v err=%v）——撤销变成了删除", found, err)
	}
	if judgment.Effective() {
		t.Fatalf("撤销没落到行上")
	}
	if judgment.Basis().String() != "SYN-BASIS-1" {
		t.Fatalf("撤销顶掉了原依据：%q", judgment.Basis())
	}

	// 授权：与就绪分轨分表，各自形成和失效。
	grant := `{
		"tenantId": "SYN-T1", "unitId": "SYN-UNIT-1",
		"authorityRef": "SYN-AUTH-1", "grantedAt": "2026-08-24T01:00:00Z"
	}`
	mustExecute(commandAuthorityGrant, grant, exitRegistered, "REGISTERED")
	authorityRevoke := `{
		"tenantId": "SYN-T1", "unitId": "SYN-UNIT-1",
		"cause": "SYN-AUTH-LAPSE", "at": "2026-08-24T03:00:00Z"
	}`
	mustExecute(commandAuthorityRevoke, authorityRevoke, exitRegistered, "REVOKED")

	// 解释规则：按（辖区，法定生效起点）登记版本；同键重放是已存在、换规则是冲突；
	// 换版是登记更晚起点的新版，旧区间的评估时点仍解析回旧版（ADR-0070 支点场景）。
	rule := func(ref, appliesFrom string) string {
		return `{
			"tenantId": "SYN-T1", "resultLayer": "RELEASE_RESULT",
			"jurisdictionRef": "SYN-JURIS-DE",
			"ruleRef": "` + ref + `", "appliesFrom": "` + appliesFrom + `"
		}`
	}
	mustExecute(commandInterpretationRule, rule("SYN-RULE-1", "2026-08-01T00:00:00Z"), exitRegistered, "REGISTERED")
	mustExecute(commandInterpretationRule, rule("SYN-RULE-1", "2026-08-01T00:00:00Z"), exitRegistered, "EXISTING")
	mustExecute(commandInterpretationRule, rule("SYN-RULE-2", "2026-08-01T00:00:00Z"), exitConflict, "CONTENT_CONFLICT")
	mustExecute(commandInterpretationRule, rule("SYN-RULE-2", "2026-08-20T00:00:00Z"), exitRegistered, "REGISTERED")
	ruleView, err := adapter.NewInterpretationRuleView(db)
	if err != nil {
		t.Fatalf("构造解释规则读口：%v", err)
	}
	jurisdiction, err := domain.NewRegulatoryJurisdictionReference("SYN-JURIS-DE")
	if err != nil {
		t.Fatalf("构造辖区引用：%v", err)
	}
	earlyRule, foundEarly, err := ruleView.LoadInterpretationRule(ctx, tenant,
		domain.ReleaseResultLayer, jurisdiction, mustInstant(t, "2026-08-10T00:00:00Z"))
	if err != nil || !foundEarly || earlyRule.String() != "SYN-RULE-1" {
		t.Fatalf("换版后旧区间时点没解析回旧版：err=%v found=%v rule=%s", err, foundEarly, earlyRule)
	}

	// 义务：目录先行，明细带区间；同项换区间是冲突；无目录的明细撞外键防线答未决。
	catalog := `{
		"tenantId": "SYN-T1", "caseRef": "SYN-CASE-1",
		"registeredAt": "2026-08-24T01:00:00Z"
	}`
	mustExecute(commandObligationCatalog, catalog, exitRegistered, "REGISTERED")
	item := func(appliesFrom string) string {
		return `{
			"tenantId": "SYN-T1", "caseRef": "SYN-CASE-1",
			"obligation": "SYN-DUTY-SETTLE", "scope": "SYN-SCOPE-1",
			"state": "HANDED_OVER", "basis": "SYN-BASIS-1", "handedTo": "SYN-BROKER-1",
			"appliesFrom": "` + appliesFrom + `"
		}`
	}
	mustExecute(commandObligationItem, item("2026-08-24T01:00:00Z"), exitRegistered, "REGISTERED")
	mustExecute(commandObligationItem, item("2026-08-24T01:00:00Z"), exitRegistered, "EXISTING")
	// 换更早的区间起点：按新起点盘点时在册那份还不适用、盘不出来，是冲突不是重放
	// （A 半边同名用例钉的语义；更晚的起点若落在在册区间内且内容全同则算重放）。
	mustExecute(commandObligationItem, item("2026-08-01T00:00:00Z"), exitConflict, "CONTENT_CONFLICT")
	orphanItem := `{
		"tenantId": "SYN-T1", "caseRef": "SYN-CASE-NEVER",
		"obligation": "SYN-DUTY-SETTLE", "scope": "SYN-SCOPE-1",
		"state": "CONCLUDED", "basis": "SYN-BASIS-1",
		"appliesFrom": "2026-08-24T01:00:00Z"
	}`
	mustExecute(commandObligationItem, orphanItem, exitUndecided, "")

	// 门禁：目录先行，判断绑定动作与边界；同键异判断是冲突（CONTEXT「不能复用于其他动作或监管边界」）。
	gateCatalog := `{
		"tenantId": "SYN-T1", "scopeRef": "SYN-SCOPE-1", "action": "OUTBOUND_RELEASE",
		"boundaryRef": "SYN-PROC-EXPORT", "registeredAt": "2026-08-24T01:00:00Z"
	}`
	mustExecute(commandGateCatalog, gateCatalog, exitRegistered, "REGISTERED")
	gateFinding := func(state string) string {
		return `{
			"tenantId": "SYN-T1", "scopeRef": "SYN-SCOPE-1", "action": "OUTBOUND_RELEASE",
			"boundaryRef": "SYN-PROC-EXPORT", "preconditionRef": "SYN-PRE-DUTY-PAID",
			"state": "` + state + `"
		}`
	}
	mustExecute(commandGateFinding, gateFinding("MET"), exitRegistered, "REGISTERED")
	mustExecute(commandGateFinding, gateFinding("MET"), exitRegistered, "EXISTING")
	mustExecute(commandGateFinding, gateFinding("CONFLICTING"), exitConflict, "CONTENT_CONFLICT")

	// 建案要求规则（第六本）：「不要求」带依据落册、重放、翻面冲突。
	requirement := func(required string) string {
		return `{
			"tenantId": "SYN-T1", "jurisdictionRef": "SYN-JURIS-DE", "direction": "EXPORT",
			"procedureRef": "SYN-PROC-EXPORT", "required": ` + required + `,
			"basis": "SYN-CONTRACT-NO-CASE-V1"
		}`
	}
	mustExecute(commandCaseRequirement, requirement("false"), exitRegistered, "REGISTERED")
	mustExecute(commandCaseRequirement, requirement("false"), exitRegistered, "EXISTING")
	mustExecute(commandCaseRequirement, requirement("true"), exitConflict, "CONTENT_CONFLICT")

	// 口岸目录与申报路径目录（第七、八本）：版本代数同解释规则——重放已存在、错序
	// 冲突、换版是登记更晚起点的新版，旧区间时点仍解析回旧版三维。
	candidatePort := func(appliesFrom string) string {
		return `{
			"tenantId": "SYN-T1", "portRef": "SYN-PORT-HAM",
			"appliesFrom": "` + appliesFrom + `"
		}`
	}
	mustExecute(commandCandidatePort, candidatePort("2026-08-01T00:00:00Z"), exitRegistered, "REGISTERED")
	mustExecute(commandCandidatePort, candidatePort("2026-08-01T00:00:00Z"), exitRegistered, "EXISTING")
	mustExecute(commandCandidatePort, candidatePort("2026-07-01T00:00:00Z"), exitConflict, "CONTENT_CONFLICT")

	declarationPath := func(mode, appliesFrom string) string {
		return `{
			"tenantId": "SYN-T1", "pathRef": "SYN-PATH-HAM-IMPORT", "portRef": "SYN-PORT-HAM",
			"direction": "IMPORT", "declarationMode": "` + mode + `",
			"appliesFrom": "` + appliesFrom + `"
		}`
	}
	mustExecute(commandDeclarationPath, declarationPath("SYN-MODE-GENERAL", "2026-08-01T00:00:00Z"), exitRegistered, "REGISTERED")
	mustExecute(commandDeclarationPath, declarationPath("SYN-MODE-GENERAL", "2026-08-01T00:00:00Z"), exitRegistered, "EXISTING")
	mustExecute(commandDeclarationPath, declarationPath("SYN-MODE-SIMPLIFIED", "2026-08-01T00:00:00Z"), exitConflict, "CONTENT_CONFLICT")
	mustExecute(commandDeclarationPath, declarationPath("SYN-MODE-SIMPLIFIED", "2026-08-20T00:00:00Z"), exitRegistered, "REGISTERED")

	portsPathsView, err := adapter.NewPortsPathsPointView(db)
	if err != nil {
		t.Fatalf("构造口岸路径读口：%v", err)
	}
	pathRef, err := domain.NewDeclarationPathReference("SYN-PATH-HAM-IMPORT")
	if err != nil {
		t.Fatalf("构造路径引用：%v", err)
	}
	earlyPath, foundEarlyPath, err := portsPathsView.LoadDeclarationPath(ctx, tenant, pathRef,
		mustInstant(t, "2026-08-10T00:00:00Z"))
	if err != nil || !foundEarlyPath || earlyPath.Route.Mode().String() != "SYN-MODE-GENERAL" {
		t.Fatalf("换版后旧区间时点没解析回旧版三维：err=%v found=%v entry=%+v",
			err, foundEarlyPath, earlyPath)
	}
}
