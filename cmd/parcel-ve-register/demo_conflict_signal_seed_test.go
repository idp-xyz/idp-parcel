package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	vepg "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 演示租户的信号规则、分诊规则与异常披露规则种子原件照原样灌进真库（证据 S），再拿生产读口判一次无法
// 裁决的冲突走到哪里。读的是 scripts/demo-seeds 里的文件本身——种子改了，这里先红。
//
// 冲突信号只在投影对已接受来源事实裁决不了时形成，来源可以是交接、交付或任何源上下文；而「关务相关异常只能
// 消费 customs-compliance 已经接受且范围明确的事实或判断」（CONTEXT）。所以参考配置要给它一个专用类型：
// 分诊与异常披露都按（类型、可信度）取条目，借用别类信号的类型（例如关务扣留），一次交接更正分叉就会沿
// 那一类的去处挂案、对客说成关务扣留。承重的是「专用」那道断言：冲突规则的类型在分诊与披露两本当前版里
// 只挂在冲突规则那一个可信度下。末尾的 CUSTOMS 子串只是种子命名习惯上的可读提示，不承重——不带这个词的
// 别类类型，只要在两本里还挂在别的可信度下，照样由专用断言拦下。
//
// 去处取人工复核：一处分叉说明的是哪一支当前有效尚无从确认（资料不足），同一来源的更正问题可能波及成批
// 包裹（可能重复），「资料不足、可能重复或关联不明必须人工分诊」（UC-VE-004）。对两个演示账户都明说暂不
// 披露：没有条目时披露决定停在未配置，那不是「不披露」。
func TestDemoConflictSignalSeedIsANeutralSourceConflictReviewedAndNotDisclosed(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	regs, err := buildRegistrars(db)
	if err != nil {
		t.Fatalf("装配登记口：%v", err)
	}
	ctx := t.Context()
	identity := channelIdentity{osUser: "syn-seed", hostname: "syn-seed-host"}

	seeds := filepath.Join("..", "..", "scripts", "demo-seeds", "data", "visibility")
	for _, seed := range []struct{ command, file string }{
		{commandTriageRules, "02-triage-rules-v1.json"},
		{commandExceptionDisclosureRules, "08-exception-disclosure-rules-v1.json"},
		{commandConflictSignalRule, "09-conflict-signal-rule-v1.json"},
		{commandDepartureGateSignalRule, "13-departure-gate-signal-rule-v1.json"},
	} {
		raw, err := os.ReadFile(filepath.Join(seeds, seed.file))
		if err != nil {
			t.Fatalf("读种子 %s：%v", seed.file, err)
		}
		if message, code := execute(ctx, seed.command, raw, identity, regs); code != exitRegistered {
			t.Fatalf("%s 退出码 = %d（%s），要 0", seed.file, code, message)
		}
	}

	tenant := mustVE(t, domain.NewTenantID, "SYN-TENANT-01")
	conflictRules, err := vepg.NewConflictSignalRules(db, tenant)
	if err != nil {
		t.Fatalf("构造冲突信号规则读口：%v", err)
	}
	rule, configured, err := conflictRules.ConflictSignalRule(ctx)
	if err != nil || !configured {
		t.Fatalf("冲突信号规则 = 已配置 %v、错 %v，要已配置", configured, err)
	}
	kind := rule.Kind.String()
	for _, catalogue := range []struct{ name, entries, versions string }{
		{"分诊规则", "visibility_exception.triage_rule_entry", "visibility_exception.triage_rule_version"},
		{"异常披露规则", "visibility_exception.exception_disclosure_rule_entry", "visibility_exception.exception_disclosure_rule_version"},
	} {
		if others := querySingle(t, db,
			`SELECT COALESCE(string_agg(DISTINCT entry.confidence_ref, ','), '')
			   FROM `+catalogue.entries+` entry
			   JOIN `+catalogue.versions+` version
			     ON version.tenant_id = entry.tenant_id AND version.rule_version = entry.rule_version
			  WHERE entry.tenant_id = $1 AND version.effective_to IS NULL
			    AND entry.signal_kind = $2 AND entry.confidence_ref <> $3`,
			tenant.String(), kind, rule.Confidence.String()); others != "" {
			t.Fatalf("%s里类型 %s 挂在冲突规则可信度 %s 以外的 %s 下：借用了别类信号的类型，或条目与冲突规则的可信度失配",
				catalogue.name, kind, rule.Confidence, others)
		}
	}
	// 出发门禁阻断说的是关务判了阻断，与冲突信号共用类型就共用分诊与披露去处。
	gateKind := querySingle(t, db, `SELECT signal_kind FROM visibility_exception.departure_gate_signal_rule WHERE tenant_id = $1`,
		tenant.String())
	if kind == gateKind {
		t.Fatalf("冲突信号类型与出发门禁阻断信号同为 %s", kind)
	}

	triage, err := vepg.NewTriageRules(db, tenant)
	if err != nil {
		t.Fatalf("构造分诊规则读口：%v", err)
	}
	answer, configured, err := triage.TriageSignal(ctx, ports.TriageQuery{
		Kind:       rule.Kind,
		Parcel:     mustVE(t, domain.NewTrackedParcelReference, "SYN-PCL-FORKED"),
		Rule:       rule.Rule,
		Confidence: rule.Confidence,
	})
	if err != nil || !configured || answer.Outcome != domain.ManualReviewRequired {
		t.Fatalf("冲突信号分诊 = %v（已配置 %v、错 %v），要人工复核", answer.Outcome, configured, err)
	}
	// 没登条目时读口同样答人工复核；去处要由参考配置明登，不靠「没命中」落进那一格。
	if outcome := querySingle(t, db,
		`SELECT entry.outcome
		   FROM visibility_exception.triage_rule_entry entry
		   JOIN visibility_exception.triage_rule_version version
		     ON version.tenant_id = entry.tenant_id AND version.rule_version = entry.rule_version
		  WHERE entry.tenant_id = $1 AND version.effective_to IS NULL
		    AND entry.signal_kind = $2 AND entry.confidence_ref = $3`,
		tenant.String(), kind, rule.Confidence.String()); outcome != "MANUAL_REVIEW" {
		t.Fatalf("分诊规则里冲突信号的条目 = %q，要明登 MANUAL_REVIEW", outcome)
	}

	disclosure, err := vepg.NewExceptionDisclosureRules(db, tenant)
	if err != nil {
		t.Fatalf("构造异常披露规则读口：%v", err)
	}
	for _, account := range []string{"SYN-ACCOUNT-01", "SYN-ACCOUNT-02"} {
		customer := mustVE(t, domain.NewCustomerAccountReference, account)
		decided, configured, err := disclosure.RuleForSignal(ctx, customer, rule.Kind, rule.Confidence)
		if err != nil || !configured {
			t.Fatalf("%s 的冲突信号披露规则 = 已配置 %v、错 %v，要明登", account, configured, err)
		}
		if decided.Disclosable || decided.AutoRelease {
			t.Fatalf("%s 的冲突信号可披露 %v、自动发布 %v，要都不", account, decided.Disclosable, decided.AutoRelease)
		}
	}

	if strings.Contains(kind, "CUSTOMS") {
		t.Fatalf("冲突信号类型 = %s：不分来源的冲突登在关务类类型下", kind)
	}
}

func mustVE[T any](t *testing.T, construct func(string) (T, error), value string) T {
	t.Helper()
	built, err := construct(value)
	if err != nil {
		t.Fatalf("构造 %s：%v", value, err)
	}
	return built
}

// querySingle 取一格；查无此行答空串，让断言说出「没登」而不是一句驱动层的错。
func querySingle(t *testing.T, db *bentopg.DB, query string, args ...any) string {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var value string
	err = querier.QueryRow(t.Context(), query, args...).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatalf("查（%s）：%v", query, err)
	}
	return value
}
