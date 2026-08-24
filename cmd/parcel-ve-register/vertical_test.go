package main

import (
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证 buildRegistrars 装配的整条登记链（隔离合成 S）：
// 六条命令各自贯通「翻译 → 用例 → 真库 → 留痕」，同版本号重放答版本不可覆盖、
// 区间重叠被写入侧防重叠拦下、缺件拒绝不落行不留痕。目录内容全为 SYN- 合成值，
// 只证「目录可填且归类」，不进任何生产装配。
func TestVERegisterVerticalOnRealPostgres(t *testing.T) {
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
	identity := channelIdentity{osUser: "OPSHOST\\operator-a", hostname: "ops-host-01"}

	countRows := func(query string, args ...any) int {
		t.Helper()
		querier, err := db.ReadExecutor(ctx)
		if err != nil {
			t.Fatalf("取读执行器：%v", err)
		}
		var count int
		if err := querier.QueryRow(ctx, query, args...).Scan(&count); err != nil {
			t.Fatalf("数行（%s）：%v", query, err)
		}
		return count
	}
	countTraces := func(command string) int {
		return countRows(
			`SELECT count(*) FROM visibility_exception.channel_execution WHERE command = $1`,
			command)
	}

	// 六命令各自贯通，逐张目录表点数。
	steps := []struct {
		command    string
		input      string
		tableCount func() int
	}{
		{
			command: commandMilestoneMapping,
			input: `{"tenantId":"SYN-TEN-VE15","version":"SYN-MAP-V1","approvedBy":"SYN-approver-1",
				"effectiveFrom":"2026-09-01T00:00:00Z",
				"entries":[{"source":"PARCEL_SHIPMENT","factKind":"SYN_KIND_DELIVERED",
				"milestone":"SYN-MILESTONE-DELIVERED"}]}`,
			tableCount: func() int {
				return countRows(
					`SELECT count(*) FROM visibility_exception.milestone_mapping_version WHERE tenant_id = $1`,
					"SYN-TEN-VE15")
			},
		},
		{
			command: commandTriageRules,
			input: `{"tenantId":"SYN-TEN-VE15","version":"SYN-TRI-V1","approvedBy":"SYN-approver-1",
				"effectiveFrom":"2026-09-01T00:00:00Z",
				"entries":[{"kind":"SYN-SIGNAL-STALL","confidence":"SYN-CONF-HIGH",
				"outcome":"MANUAL_REVIEW"}]}`,
			tableCount: func() int {
				return countRows(
					`SELECT count(*) FROM visibility_exception.triage_rule_version WHERE tenant_id = $1`,
					"SYN-TEN-VE15")
			},
		},
		{
			command: commandNotificationPolicy,
			input: `{"tenantId":"SYN-TEN-VE15","policy":"SYN-DISC-POLICY-1","channel":"SYN-CHANNEL-EMAIL",
				"deadlineAfter":"72h","obligation":"SYN-OBLIGATION-1","approvedBy":"SYN-approver-1"}`,
			tableCount: func() int {
				return countRows(
					`SELECT count(*) FROM visibility_exception.notification_policy WHERE tenant_id = $1`,
					"SYN-TEN-VE15")
			},
		},
		{
			command: commandClaimEligibility,
			input: `{"tenantId":"SYN-TEN-VE15","version":"SYN-ELIG-V1","approvedBy":"SYN-approver-1",
				"contract":"SYN-CONTRACT-SCOPE-1","coveredKinds":["SYN-CLAIM-LOSS"]}`,
			tableCount: func() int {
				return countRows(
					`SELECT count(*) FROM visibility_exception.claim_contract_scope WHERE tenant_id = $1`,
					"SYN-TEN-VE15")
			},
		},
		{
			command: commandClaimAuthorization,
			input: `{"tenantId":"SYN-TEN-VE15","version":"SYN-AUTH-V1","approvedBy":"SYN-approver-1",
				"customer":"SYN-CUSTOMER-1","applicants":["SYN-APPLICANT-1"]}`,
			tableCount: func() int {
				return countRows(
					`SELECT count(*) FROM visibility_exception.claim_authorization_catalogue WHERE tenant_id = $1`,
					"SYN-TEN-VE15")
			},
		},
		{
			command: commandDisclosurePolicy,
			input: `{"tenantId":"SYN-TEN-VE15","version":"SYN-DISC-V1","approvedBy":"SYN-approver-1",
				"effectiveFrom":"2026-09-01T00:00:00Z",
				"entries":[{"customer":"SYN-CUSTOMER-1",
				"milestones":{"state":"SHOWN","content":"SYN-CONTENT-MILESTONES"},
				"eta":{"state":"PENDING_CONFIRMATION"},"final":{"state":"NOT_DISCLOSED"},
				"note":{"state":"SHOWN","content":"SYN-CONTENT-NOTE"}}]}`,
			tableCount: func() int {
				return countRows(
					`SELECT count(*) FROM visibility_exception.disclosure_policy_version WHERE tenant_id = $1`,
					"SYN-TEN-VE15")
			},
		},
	}
	for _, step := range steps {
		if message, code := execute(ctx, step.command, []byte(step.input), identity, regs); code != exitRegistered {
			t.Fatalf("%s 退出码 = %d（%s）", step.command, code, message)
		}
		if count := step.tableCount(); count != 1 {
			t.Fatalf("%s 落行 = %d，要 1", step.command, count)
		}
		if count := countTraces(step.command); count != 1 {
			t.Fatalf("%s 留痕 = %d，要 1", step.command, count)
		}
	}

	// 同版本号重放：登记册不比对内容，一律答版本不可覆盖（2）；原行未被顶替，不留痕。
	replay := steps[0]
	if message, code := execute(ctx, replay.command, []byte(replay.input), identity, regs); code != exitGovernance ||
		!strings.Contains(message, "VERSION_NOT_OVERWRITABLE") {
		t.Fatalf("重放 = %d（%s），要 2 且指名版本不可覆盖", code, message)
	}
	if count := replay.tableCount(); count != 1 {
		t.Fatalf("重放后行数 = %d，要 1（原行未被顶替）", count)
	}
	if count := countTraces(replay.command); count != 1 {
		t.Fatalf("重放留痕 = %d，要 1（拒绝不留痕）", count)
	}

	// 区间重叠：已闭区间撞上在册未闭版本（已闭区间不触发接续，补历史必须落在空档里），
	// 写入侧防重叠拦下（2），不落行。
	overlapping := `{"tenantId":"SYN-TEN-VE15","version":"SYN-MAP-V2","approvedBy":"SYN-approver-1",
		"effectiveFrom":"2026-08-01T00:00:00Z","effectiveTo":"2026-09-15T00:00:00Z",
		"entries":[{"source":"PARCEL_SHIPMENT","factKind":"SYN_KIND_DELIVERED",
		"milestone":"SYN-MILESTONE-DELIVERED"}]}`
	if message, code := execute(ctx, commandMilestoneMapping, []byte(overlapping), identity, regs); code != exitGovernance ||
		!strings.Contains(message, "VERSION_OVERLAPS_EXISTING") {
		t.Fatalf("重叠登记 = %d（%s），要 2 且指名区间重叠", code, message)
	}
	if count := countRows(
		`SELECT count(*) FROM visibility_exception.milestone_mapping_version WHERE tenant_id = $1`,
		"SYN-TEN-VE15"); count != 1 {
		t.Fatalf("重叠登记后版本行 = %d，要 1", count)
	}

	// 接续闭合（ADR-0068）：登记更晚起点的未闭新版是「发布新的当前版本」，放行且给
	// 前版补上终点——这是登记册最常见的正当动作，不是重叠。
	succession := `{"tenantId":"SYN-TEN-VE15","version":"SYN-MAP-V3","approvedBy":"SYN-approver-1",
		"effectiveFrom":"2026-10-01T00:00:00Z",
		"entries":[{"source":"PARCEL_SHIPMENT","factKind":"SYN_KIND_DELIVERED",
		"milestone":"SYN-MILESTONE-DELIVERED"}]}`
	if message, code := execute(ctx, commandMilestoneMapping, []byte(succession), identity, regs); code != exitRegistered {
		t.Fatalf("接续新版 = %d（%s），要 0", code, message)
	}
	if count := countRows(
		`SELECT count(*) FROM visibility_exception.milestone_mapping_version
		  WHERE tenant_id = $1 AND mapping_version = $2 AND effective_to = $3::timestamptz`,
		"SYN-TEN-VE15", "SYN-MAP-V1", "2026-10-01T00:00:00Z"); count != 1 {
		t.Fatalf("前版没被接续闭合到新版起点")
	}
	if count := countTraces(commandMilestoneMapping); count != 2 {
		t.Fatalf("接续后映射留痕 = %d，要 2（两次已登记）", count)
	}

	// 缺件拒绝：空条目集是用例门的 ENTRIES_MISSING（1），不落行、不留痕。
	refused := `{"tenantId":"SYN-TEN-VE15-REFUSED","version":"SYN-TRI-V1","approvedBy":"SYN-approver-1",
		"effectiveFrom":"2026-09-01T00:00:00Z","entries":[]}`
	if message, code := execute(ctx, commandTriageRules, []byte(refused), identity, regs); code != exitUsage ||
		!strings.Contains(message, "ENTRIES_MISSING") {
		t.Fatalf("缺件拒绝 = %d（%s），要 1 且指名 ENTRIES_MISSING", code, message)
	}
	if count := countRows(
		`SELECT count(*) FROM visibility_exception.triage_rule_version WHERE tenant_id = $1`,
		"SYN-TEN-VE15-REFUSED"); count != 0 {
		t.Fatalf("拒绝后落了行：%d", count)
	}
	if count := countTraces(commandTriageRules); count != 1 {
		t.Fatalf("拒绝后留痕 = %d，要 1（只有先前那次已登记的）", count)
	}
}
