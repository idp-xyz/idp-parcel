package postgres_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 证「还没出账的调整」那条读法对对账单调整行的逐笔探查经迁移 0063 建的 GIN 索引，不是逐笔扫整张对账单。
//
// EXPLAIN 的是读口自己发出的那条语句与那组参数：经连接池的查询追踪原样记下，不在测试里另抄一份 SQL——抄的那份会在查询改动时
// 留在这里变旧，而计划照旧好看。不动任何计划器设置，证的是规划器在这份数据上自己选了什么。夹具全为合成（S 级）。

const (
	unbilledPlanTenant      = "SYN-TENANT-UNBILLED-PLAN"
	unbilledPlanAccount     = "SYN-ACCOUNT-UNBILLED-PLAN-1"
	unbilledPlanStatements  = 20000
	unbilledPlanAdjustments = 3000
	// 前这么多笔调整已经写在某张未作废对账单的调整行里；其余的还没出账。
	unbilledPlanBilled = 1000

	adjustmentLinesIndex = "customer_statement_adjustment_lines_gin"
)

// planQueryRecorder 记下经它那只连接池发出的每一条语句与参数。
type planQueryRecorder struct {
	mutex   sync.Mutex
	queries []planRecordedQuery
}

type planRecordedQuery struct {
	sql  string
	args []any
}

func (recorder *planQueryRecorder) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	recorder.queries = append(recorder.queries, planRecordedQuery{sql: data.SQL, args: data.Args})
	return ctx
}

func (recorder *planQueryRecorder) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (recorder *planQueryRecorder) take() []planRecordedQuery {
	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()
	taken := recorder.queries
	recorder.queries = nil
	return taken
}

// plannedNode 是 EXPLAIN (FORMAT JSON) 计划树上的一个节点；只取断言要看的几格。
type plannedNode struct {
	NodeType     string        `json:"Node Type"`
	RelationName string        `json:"Relation Name"`
	IndexName    string        `json:"Index Name"`
	Plans        []plannedNode `json:"Plans"`
}

func walkPlannedNodes(node plannedNode, visit func(plannedNode)) {
	visit(node)
	for _, child := range node.Plans {
		walkPlannedNodes(child, visit)
	}
}

func plannedSummary(plan plannedNode) string {
	var parts []string
	walkPlannedNodes(plan, func(node plannedNode) {
		part := node.NodeType
		if node.RelationName != "" {
			part += " on " + node.RelationName
		}
		if node.IndexName != "" {
			part += " using " + node.IndexName
		}
		parts = append(parts, part)
	})
	return strings.Join(parts, " → ")
}

// seedUnbilledPlanRows 灌一批合成行：unbilledPlanAdjustments 笔已确认费用与各挂其上的一笔调整；unbilledPlanStatements 张对账单，
// 每四张里一张带两笔调整行，带调整行的前面那批指向 adj-0 起的真实调整（共 unbilledPlanBilled 笔已出账），其余指向调整册上没有的标识。
// 走 INSERT … SELECT generate_series，不逐行过写口：这里证的是读的计划，写口的判断有各自的用例。灌完即 ANALYZE：生产表的统计由
// autovacuum 维护，刚灌的表等不到它，不 ANALYZE 时规划器按默认选择率估，证的就不是这张表的计划。
func seedUnbilledPlanRows(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := t.Context()
	statements := []struct {
		sql  string
		args []any
	}{
		{
			sql: `INSERT INTO settlement_accounting.customer_charge
				(tenant_id, charge_id, fee_item, evaluation_ref, settlement_currency, settlement_minor, original_currency, original_minor,
				 stage, confirmation_basis, formed_at, confirmed_at, recorded_at,
				 responsible_entity, counterparty_ref, charge_direction, settlement_account_id, contract_basis, primary_charging_scope, source_fact_ref)
			 SELECT $1, 'SYN-CHARGE-PLAN-' || g, 'BASE_FREIGHT', 'SYN-EVAL-PLAN-' || g, 'CNY', 1000, 'CNY', 1000,
			        'CONFIRMED', 'DELIVERY_FINALIZED/syn-plan', now(), now(), now(),
			        'SYN-ENTITY-PLAN', 'SYN-COUNTERPARTY-PLAN', 'RECEIVABLE', $2, 'SYN-CONTRACT-PLAN', 'SYN-SCOPE-PLAN', 'SYN-SOURCE-PLAN'
			   FROM generate_series(0, $3::int - 1) AS g`,
			args: []any{unbilledPlanTenant, unbilledPlanAccount, unbilledPlanAdjustments},
		},
		{
			sql: `INSERT INTO settlement_accounting.charge_adjustment
				(tenant_id, adjustment_id, charge_id, kind, direction, evaluation_ref, original_currency, original_minor,
				 settlement_currency, settlement_minor, formed_at, recorded_at)
			 SELECT $1, 'SYN-ADJ-PLAN-' || g, 'SYN-CHARGE-PLAN-' || g, 'PRICING_CORRECTION', 'DEBIT', 'SYN-EVAL-PLAN-NEW', 'CNY', 100,
			        'CNY', 100, now() + g * interval '1 second', now()
			   FROM generate_series(0, $2::int - 1) AS g`,
			args: []any{unbilledPlanTenant, unbilledPlanAdjustments},
		},
		{
			sql: `INSERT INTO settlement_accounting.customer_statement
				(tenant_id, statement_number, account_id, period_ref, currency, total_minor, content_digest, lines, adjustment_lines,
				 published_at, recorded_at)
			 SELECT $1, 'SYN-STMT-PLAN-' || g, 'SYN-ACCOUNT-PLAN-OTHER-' || (g % 50), 'SYN-PERIOD-' || g, 'CNY', 100, 'SYN-DIGEST-' || g,
			        jsonb_build_array(jsonb_build_object('charge', 'SYN-CHARGE-PLAN-OTHER-' || g, 'amount_minor', 100)),
			        CASE WHEN g % 4 <> 0 THEN '[]'::jsonb
			             WHEN g / 4 < $2::int / 2 THEN jsonb_build_array(
			                  jsonb_build_object('adjustment', 'SYN-ADJ-PLAN-' || ((g / 4) * 2), 'charge', 'SYN-CHARGE-PLAN-' || ((g / 4) * 2),
			                                     'direction', 'DEBIT', 'amount_minor', 100),
			                  jsonb_build_object('adjustment', 'SYN-ADJ-PLAN-' || ((g / 4) * 2 + 1), 'charge', 'SYN-CHARGE-PLAN-' || ((g / 4) * 2 + 1),
			                                     'direction', 'DEBIT', 'amount_minor', 100))
			             ELSE jsonb_build_array(
			                  jsonb_build_object('adjustment', 'SYN-ADJ-ELSEWHERE-' || (g * 2), 'charge', 'SYN-CHARGE-PLAN-OTHER-' || g,
			                                     'direction', 'DEBIT', 'amount_minor', 100),
			                  jsonb_build_object('adjustment', 'SYN-ADJ-ELSEWHERE-' || (g * 2 + 1), 'charge', 'SYN-CHARGE-PLAN-OTHER-' || g,
			                                     'direction', 'DEBIT', 'amount_minor', 100))
			        END,
			        now(), now()
			   FROM generate_series(0, $3::int - 1) AS g`,
			args: []any{unbilledPlanTenant, unbilledPlanBilled, unbilledPlanStatements},
		},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("灌合成行：%v", err)
		}
	}
	if _, err := pool.Exec(ctx, `ANALYZE settlement_accounting.customer_charge, settlement_accounting.charge_adjustment,
		settlement_accounting.customer_statement, settlement_accounting.subsequent_inclusion`); err != nil {
		t.Fatalf("ANALYZE：%v", err)
	}
}

// Covers: 找「还没出账的调整」的查询对每笔候选调整去探对账单的调整行；迁移 0063 给调整行建了 GIN（jsonb_path_ops）索引，灌了两万张
// 对账单之后规划器不改任何设置就经它探，不是对每笔调整顺序扫整张对账单。答复仍是还没出账的那些：前一千笔已写在对账单调整行里的不交。
func TestTheUnbilledAdjustmentProbeReadsStatementsThroughTheAdjustmentLinesIndex(t *testing.T) {
	plain := pgtest.Pool(t)
	recorder := &planQueryRecorder{}
	config := plain.Config()
	config.ConnConfig.Tracer = recorder
	traced, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatalf("开带追踪的连接池：%v", err)
	}
	// 晚于 pgtest.Pool 登记，先于它的删库执行：库上还有连接时删不掉。
	t.Cleanup(traced.Close)
	db, err := bentopg.NewDB(traced, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	adjustments, err := adapter.NewChargeAdjustments(db)
	if err != nil {
		t.Fatal(err)
	}
	seedUnbilledPlanRows(t, plain)

	// 每隔十笔点名一笔费用：三百笔候选，其中前一百笔已出账、其余二百笔还没出账。
	var named []domain.CustomerChargeID
	for index := 0; index < unbilledPlanAdjustments; index += 10 {
		named = append(named, saValue(t, domain.NewCustomerChargeID, "SYN-CHARGE-PLAN-"+strconv.Itoa(index)))
	}
	recorder.take()
	listed, err := adjustments.ListUnbilledForCutoff(t.Context(), saTenant(t, unbilledPlanTenant),
		saValue(t, domain.NewSettlementAccountID, unbilledPlanAccount), saValue(t, domain.NewCurrencyCode, "CNY"), named, false)
	if err != nil {
		t.Fatalf("还没出账的调整：%v", err)
	}
	wantListed := (unbilledPlanAdjustments - unbilledPlanBilled) / 10
	if len(listed) != wantListed {
		t.Fatalf("交回 %d 笔，want 点名那三百笔里没出账的 %d 笔", len(listed), wantListed)
	}
	for _, record := range listed {
		suffix := strings.TrimPrefix(record.Key.Adjustment.String(), "SYN-ADJ-PLAN-")
		if number, err := strconv.Atoi(suffix); err != nil || number < unbilledPlanBilled {
			t.Fatalf("交回了已出账的调整 %s", record.Key.Adjustment)
		}
	}

	var probe planRecordedQuery
	for _, query := range recorder.take() {
		if strings.Contains(query.sql, "FROM settlement_accounting.charge_adjustment") &&
			strings.Contains(query.sql, "settlement_accounting.customer_statement") {
			probe = query
		}
	}
	if probe.sql == "" {
		t.Fatal("读口没有发出带对账单探查的那条语句")
	}
	var raw []byte
	if err := plain.QueryRow(t.Context(), `EXPLAIN (FORMAT JSON) `+probe.sql, probe.args...).Scan(&raw); err != nil {
		t.Fatalf("EXPLAIN：%v", err)
	}
	var explained []struct {
		Plan plannedNode `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &explained); err != nil || len(explained) != 1 {
		t.Fatalf("读不懂 EXPLAIN 输出（%v）：%s", err, raw)
	}
	plan := explained[0].Plan
	summary := plannedSummary(plan)
	t.Logf("计划：%s", summary)
	var usesIndex, scansStatements bool
	walkPlannedNodes(plan, func(node plannedNode) {
		usesIndex = usesIndex || node.IndexName == adjustmentLinesIndex
		scansStatements = scansStatements || (node.NodeType == "Seq Scan" && node.RelationName == "customer_statement")
	})
	if !usesIndex || scansStatements {
		t.Fatalf("对账单调整行的探查没有经 %s：%s", adjustmentLinesIndex, summary)
	}

	var definition string
	if err := plain.QueryRow(t.Context(),
		`SELECT indexdef FROM pg_indexes WHERE schemaname = 'settlement_accounting' AND indexname = $1`, adjustmentLinesIndex,
	).Scan(&definition); err != nil {
		t.Fatalf("读索引定义：%v", err)
	}
	if !strings.Contains(definition, "USING gin") || !strings.Contains(definition, "adjustment_lines jsonb_path_ops") {
		t.Fatalf("索引不是调整行上的 jsonb_path_ops GIN：%s", definition)
	}
}
