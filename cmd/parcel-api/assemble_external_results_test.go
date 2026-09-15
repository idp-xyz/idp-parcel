package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	customsdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// externalResultOccurredAt 是外部结果的业务发生时刻；接收时刻取它之后，守住编排的
// 评估时点因果门（业务发生不得晚于接收）。
var externalResultOccurredAt = time.Date(2026, 8, 21, 8, 0, 0, 0, time.UTC)

// Covers: `/customs/external-results` 的第二参是真编排——结果登记册、提交索引、版本
// 化解释规则读口、辖区回指链（单元→案件）与 Outbox 意图交付在真实 PostgreSQL 上装得
// 起来。两个票面验收钉：「找不到原提交」分支留存不猜（真索引查过、原始响应落库、不交
// 意图、重放走已有记录证事务提交）；「解释规则未配置」分支如实未决（归属、时点与辖区
// 链全走通之后，真规则读口对未登记组合答未配置，编排停在指名的未决并携续办引用，什么
// 也不落库）。测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredExternalResultsAnswerHonestlyAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	results, err := buildExternalResultsOrchestration(db)
	if err != nil {
		t.Fatalf("装配外部结果编排：%v", err)
	}

	// 「找不到原提交」：声称一个提交链里不存在的版本。真索引查过答归属不上——原始
	// 响应连同其声称的版本留存，不猜测提交、不补造层次，也不交意图（留存的不是监管
	// 事实）。
	orphan := externalResultCommand(t, "SYN-RESULT-ORPHAN-1", "SYN-CC-VERSION-MISSING")
	unattributed, err := results.Handle(t.Context(), orphan)
	if err != nil {
		t.Fatalf("归属不上的外部结果：%v", err)
	}
	if got := unattributed.Outcome(); got != customsapp.ResultUnattributable {
		t.Fatalf("outcome = %v, want UNATTRIBUTABLE——归属不上要留存不猜，不是拒收也不是未决", got)
	}
	record, has := unattributed.Record()
	if !has || !record.Unattributable {
		t.Fatalf("留存记录缺席或没带归属不上标记：has=%v record=%+v", has, record)
	}
	if ref := unattributed.ResultHandoffReference(); ref != "" {
		t.Fatalf("留存的不是监管事实，不该交意图，却留了续办引用 %q", ref)
	}

	replayed, err := results.Handle(t.Context(), orphan)
	if err != nil {
		t.Fatalf("重放同一份留存：%v", err)
	}
	if got := replayed.Outcome(); got != customsapp.ResultExistingResult {
		t.Fatalf("outcome = %v, want EXISTING_RESULT——重放没走已有记录，留存事务没有提交", got)
	}

	// 「解释规则未配置」：把归属、评估时点与辖区回指链全部走通，让真规则读口来作答。
	// 该（租户,层,辖区,时点）组合无已登记版本，读口如实答未配置，编排停在指名的未决
	// ——这正是实例半边的留白：恢复动作是经登记口登参数，不是改代码。
	seedExternalResultChain(t, pool, "SYN-TENANT-1", "SYN-CC-UNIT-1", "SYN-CC-CASE-1", "SYN-CC-VERSION-1")
	attributed := externalResultCommand(t, "SYN-RESULT-1", "SYN-CC-VERSION-1")
	undecided, err := results.Handle(t.Context(), attributed)
	if err != nil {
		t.Fatalf("解释规则未配置不该以 error 交回（那是 5xx，不是未决）：%v", err)
	}
	if got := undecided.Outcome(); got != customsapp.ResultUndecided {
		t.Fatalf("outcome = %v, want RESULT_UNDECIDED——规则未登记既不是拒收也不是猜一个口径记下去", got)
	}
	if got := undecided.UndecidedReason(); got != customsapp.InterpretationRuleUnconfigured {
		t.Fatalf("reason = %v, want INTERPRETATION_RULE_UNCONFIGURED——未决要指名到缝", got)
	}
	if undecided.ContinuationReference() == "" {
		t.Fatal("未决没带续办引用，接续的人不知道从哪一步重放")
	}
	if _, has := undecided.Record(); has {
		t.Fatal("未决不该留任何记录——规则未配置时半份解释落库就是猜了口径")
	}
}

// Covers: 票 sa-cc/34 判据 (5)——`/customs/external-results` 的真装配上，来源标识长到把交接信封的分区键
// （「租户 / 来源标识」可读串接）顶过 eventing.MaxPartitionKeyLength：交接口分格交出 ports.ErrHandoffEnvelopeRejected、
// 编排以 ErrExternalResultHandoffRejected 响亮报错，transactionalResults 随之回滚——结果行零、信封零；端点映成
// 400 HANDOFF_ENVELOPE_REJECTED 出队交给人。先用正常长度的来源标识在同一条链上走到「已记录 + 信封一行」，证明夹具
// 确实到得了交接那一步，超长那一格的空才是回滚证出来的。用替身 intake 只为绕过生产上尚未配置的通道认证
// （UnconfiguredIntake 答 403 在编排之前），编排与事务壳都是生产装配那一份。
func TestAnOverlongSourceIDRollsBackTheRecordAndTheEnvelopeAndAnswersFourHundred(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	results, err := buildExternalResultsOrchestration(db)
	if err != nil {
		t.Fatalf("装配外部结果编排：%v", err)
	}
	const tenant = "SYN-TENANT-1"
	seedExternalResultChain(t, pool, tenant, "SYN-CC-UNIT-1", "SYN-CC-CASE-1", "SYN-CC-VERSION-1")
	seedInterpretationRule(t, pool, tenant, customsdomain.ProcessDecisionLayer, "SYN-JURISDICTION-1", externalResultOccurredAt.Add(-48*time.Hour))

	recorded, err := results.Handle(t.Context(), externalResultCommand(t, "SYN-RESULT-SHORT-1", "SYN-CC-VERSION-1"))
	if err != nil || recorded.Outcome() != customsapp.ResultRecorded || recorded.ResultHandoffReference() != "" {
		t.Fatalf("对照格没走到「已记录且意图已交」：err=%v outcome=%v handoff=%q", err, recorded.Outcome(), recorded.ResultHandoffReference())
	}
	if got := countOutboxRows(t, pool, string(outboxintent.FingerprintEventID("external-result", tenant, "SYN-RESULT-SHORT-1"))); got != 1 {
		t.Fatalf("对照格 outbox 行数 = %d，want 1", got)
	}

	overlong := strings.Repeat("x", eventing.MaxPartitionKeyLength-len(tenant+"/")+1)
	if partitionKey := tenant + "/" + overlong; len(partitionKey) <= eventing.MaxPartitionKeyLength || len(overlong) > eventing.MaxSubjectLength {
		t.Fatalf("夹具没造对：分区键 %d 字节（上限 %d）、主体 %d 字节（上限 %d）",
			len(partitionKey), eventing.MaxPartitionKeyLength, len(overlong), eventing.MaxSubjectLength)
	}
	command := externalResultCommand(t, overlong, "SYN-CC-VERSION-1")

	_, err = results.Handle(t.Context(), command)
	if !errors.Is(err, customsapp.ErrExternalResultHandoffRejected) || !errors.Is(err, ccports.ErrHandoffEnvelopeRejected) {
		t.Fatalf("超长来源标识该以确定性拒收响亮报错、带原因：err = %v", err)
	}
	if got := countExternalResultRows(t, pool, tenant, overlong); got != 0 {
		t.Fatalf("回滚后结果行数 = %d，want 0——记录与信封该同生同灭", got)
	}
	if got := countOutboxRows(t, pool, string(outboxintent.FingerprintEventID("external-result", tenant, overlong))); got != 0 {
		t.Fatalf("回滚后 outbox 行数 = %d，want 0", got)
	}

	endpoint := customshttp.NewReceiveExternalResultEndpoint(commandIntake{command: command}, results)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/customs/external-results", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("端点状态 = %d，want %d：确定性拒收是调用方的错，留队重发只会再拒一次", response.Code, http.StatusBadRequest)
	}
	var problem struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil || problem.Error.Code != "HANDOFF_ENVELOPE_REJECTED" {
		t.Fatalf("问题体 = %+v（err=%v），want code HANDOFF_ENVELOPE_REJECTED", problem, err)
	}
}

// commandIntake 把一条既定命令当作通道请求的翻译结果交出，只为让端点用例绕过生产上尚未配置的通道认证。
type commandIntake struct {
	command customsapp.ReceiveExternalResultCommand
}

func (intake commandIntake) IntakeResult(context.Context, *http.Request) (customsapp.ReceiveExternalResultCommand, error) {
	return intake.command, nil
}

// seedInterpretationRule 直插一版解释规则，让真读口对该（租户, 层, 辖区, 时点）答已配置；规则引用是合成字面，
// 编排只把它记进结果、不解读它。
func seedInterpretationRule(t *testing.T, pool *pgxpool.Pool, tenant string, layer customsdomain.ResultLayer, jurisdiction string, appliesFrom time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO customs_compliance.interpretation_rule
			(tenant_id, result_layer, jurisdiction_ref, applies_from, rule_ref)
		 VALUES ($1, $2, $3, $4, 'SYN-INTERPRET/process/v1')`,
		tenant, layer.String(), jurisdiction, appliesFrom,
	); err != nil {
		t.Fatalf("植入解释规则：%v", err)
	}
}

func countExternalResultRows(t *testing.T, pool *pgxpool.Pool, tenant, sourceID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM customs_compliance.external_result WHERE tenant_id = $1 AND source_id = $2`,
		tenant, sourceID,
	).Scan(&count); err != nil {
		t.Fatalf("统计结果行：%v", err)
	}
	return count
}

func countOutboxRows(t *testing.T, pool *pgxpool.Pool, eventID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`, eventID,
	).Scan(&count); err != nil {
		t.Fatalf("统计 outbox 行：%v", err)
	}
	return count
}

func externalResultCommand(t *testing.T, sourceID, claimedVersion string) customsapp.ReceiveExternalResultCommand {
	t.Helper()
	return customsapp.ReceiveExternalResultCommand{
		TenantID:       mustValue(t, customsdomain.NewTenantID, "SYN-TENANT-1"),
		SourceID:       sourceID,
		Layer:          customsdomain.ProcessDecisionLayer,
		Role:           "SYN-AUTHORITY-1",
		RawSemantics:   "SYN-RAW-RELEASED",
		ClaimedVersion: claimedVersion,
		Attempt:        1,
		Scope:          "SYN-CC-UNIT-1",
		OccurredAt:     externalResultOccurredAt,
		ReceivedAt:     externalResultOccurredAt.Add(time.Hour),
	}
}

// seedExternalResultChain 直插归属与辖区回指链三行事实：提交索引读的权威提交表、
// 结果范围指名的申报单元、单元回指的案件（辖区在案件本体上）。夹具从库这一层造事实
// ——本测试证的是装配对这三张权威表的消费，不是各写入方的行为。
func seedExternalResultChain(t *testing.T, pool *pgxpool.Pool, tenant, unit, caseID, version string) {
	t.Helper()
	formedAt := externalResultOccurredAt.Add(-24 * time.Hour)

	// is_current 显式给 true——迁移 0012 撤掉了列默认，播种方与写入方同责。
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO customs_compliance.declaration_submission
			(tenant_id, unit_id, procedure_ref, version_id, content_digest,
			 members, dossier_ref, roles_ref, readiness_basis, authority_ref,
			 is_current, fixed_at, recorded_at)
		 VALUES ($1, $2, 'EXPORT/GENERAL', $3, 'SYN-DIGEST-1',
			 '["SYN-PARCEL-1"]', 'SYN-DOSSIER/v1', 'SYN-ROLES/v1', 'SYN-READINESS/v1', 'SYN-AUTHORITY/v1',
			 true, $4, $4)`,
		tenant, unit, version, formedAt,
	); err != nil {
		t.Fatalf("植入申报提交：%v", err)
	}

	// 案件先于单元：declaration_unit 的案件维带外键（declaration_unit_case_exists），
	// 单元只回指在册案件。
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO customs_compliance.customs_case
			(tenant_id, jurisdiction_ref, direction, procedure_ref, obligation_ref,
			 case_id, parcels, roles, established_at)
		 VALUES ($1, 'SYN-JURISDICTION-1', 'EXPORT', 'EXPORT/GENERAL', 'SYN-OBLIGATION-1',
			 $2, $3::jsonb, '[]'::jsonb, $4)`,
		tenant, caseID,
		`[{"parcel":"SYN-PARCEL-1","customer":"SYN-CUSTOMER-1","sourceRef":"SYN-SOURCE-1"}]`,
		formedAt,
	); err != nil {
		t.Fatalf("植入案件：%v", err)
	}

	if _, err := pool.Exec(context.Background(),
		`INSERT INTO customs_compliance.declaration_unit
			(tenant_id, unit_id, case_id, procedure_ref, members, formed_at)
		 VALUES ($1, $2, $3, 'EXPORT/GENERAL', '["SYN-PARCEL-1"]', $4)`,
		tenant, unit, caseID, formedAt,
	); err != nil {
		t.Fatalf("植入申报单元：%v", err)
	}
}
