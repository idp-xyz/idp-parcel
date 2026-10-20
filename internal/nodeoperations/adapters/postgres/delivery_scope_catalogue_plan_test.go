package postgres_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证送站预报与送站交接单两条查阅列表的各种读法经迁移 0021 各自那条索引读册表（票 ps-external-mark-relations/15）。
//
// EXPLAIN 的是读口自己发出的那条语句与那组参数：经连接池的查询追踪原样记下，不在测试里另抄一份 SQL——抄的那份会在查询改动时
// 留在这里变旧，而计划照旧好看。参数随语句绑定，规划器拿得到实参，「参数为空即不设这一界」那几支在规划时就折掉，计划与读口执行
// 时的定制计划同形。夹具全为合成（S 级）。

const (
	planTenant      = "SYN-TENANT-PSXM15"
	planOtherTenant = "SYN-TENANT-PSXM15-OTHER"
	planNodes       = 8
	// 每个租户每本册的行数。量取到四种读法上规划器都自己选了索引（带节点两种实测索引那条计划的估价不到顺序扫描的一半），再多只
	// 拖慢用例。
	planRowsPerTenant = 2000
	planPageSize      = 50

	preAdviceNodeIndex      = "delivery_pre_advice_by_node_expected_arrival"
	preAdviceTenantIndex    = "delivery_pre_advice_by_expected_arrival"
	handoverNoteNodeIndex   = "delivery_handover_note_by_node_opening"
	handoverNoteTenantIndex = "delivery_handover_note_by_opening"
)

var planEpoch = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

func planNodeName(index int) string {
	return fmt.Sprintf("SYN-NODE-PSXM15-%d", index)
}

// queryRecorder 记下经它那只连接池发出的每一条语句与参数。
type queryRecorder struct {
	mu      sync.Mutex
	queries []recordedQuery
}

type recordedQuery struct {
	sql  string
	args []any
}

func (recorder *queryRecorder) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.queries = append(recorder.queries, recordedQuery{sql: data.SQL, args: data.Args})
	return ctx
}

func (recorder *queryRecorder) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (recorder *queryRecorder) take() []recordedQuery {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	taken := recorder.queries
	recorder.queries = nil
	return taken
}

type cataloguePlanFixture struct {
	plain     *pgxpool.Pool
	recorder  *queryRecorder
	catalogue *adapter.DeliveryScopeCatalogue
}

// newCataloguePlanFixture 在同一个测试库上开第二只带追踪的连接池给读口用；灌数与 EXPLAIN 走原池，不混进记下的语句。
func newCataloguePlanFixture(t *testing.T) *cataloguePlanFixture {
	t.Helper()
	plain := pgtest.Pool(t)
	config := plain.Config()
	recorder := &queryRecorder{}
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
	catalogue, err := adapter.NewDeliveryScopeCatalogue(db)
	if err != nil {
		t.Fatal(err)
	}
	return &cataloguePlanFixture{plain: plain, recorder: recorder, catalogue: catalogue}
}

// seed 给两个租户各灌同形的一批预报与交接单，节点轮着取，业务时刻每行往后错 17 分钟（一日约 85 行，一个节点一日约 11 行）；
// 本租户每张交接单名下两件成员，让列表行上的成员计数有东西可数。
//
// 走一条 INSERT … SELECT generate_series，不逐行过两本册的写口：这里证的是读的计划，写口的判断有各自的用例；读回仍过还原门（列表
// 照常调用），灌出一行还原不了的，列表会报错。灌完即 ANALYZE：生产表的统计由 autovacuum 维护，刚灌的表等不到它，不 ANALYZE
// 时规划器按默认选择率估，证的就不是这张表的计划。
func (fixture *cataloguePlanFixture) seed(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	statements := []struct {
		sql  string
		args []any
	}{
		{
			sql: `INSERT INTO node_operations.delivery_pre_advice
				(tenant_id, pre_advice_id, customer_account_id, node_ref, expected_arrival_at, parcel_numbers, delivering_party,
				 source_channel, source_request_key, content_digest, formed_at)
			 SELECT tenant.id, format('SYN-PREADV-PSXM15-%s', lpad(g::text, 6, '0')), 'SYN-ACCOUNT-PSXM15-1',
			        'SYN-NODE-PSXM15-' || (g % $3), $4::timestamptz + g * interval '17 minutes',
			        ARRAY[format('SYN-NUMBER-PSXM15-%s', g)], NULL,
			        'https://id.syn.example/dex#syn-customer-client-psxm15', format('SYN-REQUEST-PSXM15-%s', g),
			        format('sha256:psxm15-%s', g), $4::timestamptz
			   FROM generate_series(1, $5::int) AS g, (VALUES ($1::text), ($2::text)) AS tenant(id)`,
			args: []any{planTenant, planOtherTenant, planNodes, planEpoch, planRowsPerTenant},
		},
		{
			sql: `INSERT INTO node_operations.delivery_handover_note
				(tenant_id, note_id, node_ref, delivering_party, claimed_account_ids, opened_at)
			 SELECT tenant.id, format('SYN-HN-PSXM15-%s', lpad(g::text, 6, '0')), 'SYN-NODE-PSXM15-' || (g % $3),
			        'SYN-DRIVER-PSXM15', ARRAY['SYN-ACCOUNT-PSXM15-1'], $4::timestamptz + g * interval '17 minutes'
			   FROM generate_series(1, $5::int) AS g, (VALUES ($1::text), ($2::text)) AS tenant(id)`,
			args: []any{planTenant, planOtherTenant, planNodes, planEpoch, planRowsPerTenant},
		},
		{
			sql: `INSERT INTO node_operations.delivery_handover_member
				(tenant_id, membership_kind, fact_id, note_id, unit_id, formed_at)
			 SELECT note.tenant_id, 'RECEPTION', format('SYN-SCAN-PSXM15-%s-%s', note.note_id, unit.n), note.note_id,
			        format('SYN-UNIT-PSXM15-%s-%s', note.note_id, unit.n), note.opened_at + unit.n * interval '1 minute'
			   FROM node_operations.delivery_handover_note AS note, generate_series(1, 2) AS unit(n)
			  WHERE note.tenant_id = $1`,
			args: []any{planTenant},
		},
	}
	for _, statement := range statements {
		if _, err := fixture.plain.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("灌合成行：%v", err)
		}
	}
	if _, err := fixture.plain.Exec(ctx,
		`ANALYZE node_operations.delivery_pre_advice, node_operations.delivery_handover_note, node_operations.delivery_handover_member`,
	); err != nil {
		t.Fatalf("ANALYZE：%v", err)
	}
}

// listQuery 交回读口这一次调用发出的第一条语句——列表本身；其后按这一页标识取撤回、关闭与结论的那几条不在本票里。
func (fixture *cataloguePlanFixture) listQuery(t *testing.T, relation string) recordedQuery {
	t.Helper()
	queries := fixture.recorder.take()
	if len(queries) == 0 {
		t.Fatal("读口没有发出任何语句")
	}
	if !strings.Contains(queries[0].sql, "FROM node_operations."+relation) {
		t.Fatalf("读口发出的第一条语句不是 %s 上的列表：%s", relation, queries[0].sql)
	}
	return queries[0]
}

// explainedNode 是 EXPLAIN (FORMAT JSON) 计划树上的一个节点；只取断言要看的几格。
type explainedNode struct {
	NodeType     string          `json:"Node Type"`
	RelationName string          `json:"Relation Name"`
	IndexName    string          `json:"Index Name"`
	Plans        []explainedNode `json:"Plans"`
}

// explain EXPLAIN 读口发出的那条语句，不动任何计划器设置：证的是规划器在这份数据上自己选了什么。
func (fixture *cataloguePlanFixture) explain(t *testing.T, query recordedQuery) explainedNode {
	t.Helper()
	var raw []byte
	if err := fixture.plain.QueryRow(t.Context(), `EXPLAIN (FORMAT JSON) `+query.sql, query.args...).Scan(&raw); err != nil {
		t.Fatalf("EXPLAIN：%v", err)
	}
	var explained []struct {
		Plan explainedNode `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &explained); err != nil || len(explained) != 1 {
		t.Fatalf("读不懂 EXPLAIN 输出（%v）：%s", err, raw)
	}
	return explained[0].Plan
}

func walkPlan(node explainedNode, visit func(explainedNode)) {
	visit(node)
	for _, child := range node.Plans {
		walkPlan(child, visit)
	}
}

// planSummary 把计划树压成一行，失败信息与日志里看得出计划器选了什么。
func planSummary(plan explainedNode) string {
	var parts []string
	walkPlan(plan, func(node explainedNode) {
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

// limitOverIndexScan 报告计划是不是「Limit 下直接一个按该索引的索引扫描」：按键序读满一页即停的那个形状。
func limitOverIndexScan(plan explainedNode, index string) bool {
	if plan.NodeType != "Limit" || len(plan.Plans) != 1 {
		return false
	}
	scan := plan.Plans[0]
	return (scan.NodeType == "Index Scan" || scan.NodeType == "Index Only Scan") && scan.IndexName == index
}

type listedRow struct {
	id   string
	node string
	at   time.Time
}

// checkListed 证灌量之后列表答的仍是筛选与次序本身：节点逐字、区间左闭右开、业务时刻倒序同刻按标识。
func checkListed(t *testing.T, rows []listedRow, filter ports.DeliveryScopeCatalogueFilter) {
	t.Helper()
	if len(rows) == 0 {
		t.Fatal("列表读回空页：灌的数据没落进这一筛选，证不到计划")
	}
	for index, row := range rows {
		if node := filter.Node.String(); node != "" && row.node != node {
			t.Errorf("%s 的节点 = %s, want %s", row.id, row.node, node)
		}
		if (!filter.From.IsZero() && row.at.Before(filter.From)) || (!filter.Until.IsZero() && !row.at.Before(filter.Until)) {
			t.Errorf("%s 的业务时刻 %v 不在〔%v, %v）", row.id, row.at, filter.From, filter.Until)
		}
		if index > 0 {
			previous := rows[index-1]
			if row.at.After(previous.at) || (row.at.Equal(previous.at) && row.id < previous.id) {
				t.Errorf("次序走样：%s(%v) 排在 %s(%v) 之后", row.id, row.at, previous.id, previous.at)
			}
		}
	}
}

// Covers: 票 ps-external-mark-relations/15「完成判据」与「裁定补」的用例——两个租户各灌同形的合成预报与交接单，读口两条列表的四种读法
// 发出的语句，规划器不改任何设置就经迁移 0021 为那种读法建的索引读册表，不是顺序扫描：带节点（节点与一日、只按节点）走（租户，节点，
// 业务时刻，标识）那条，不带节点（全租户一日、全租户）走（租户，业务时刻，标识）那条。不带区间的两种读法按键序读满一页即停，计划就是
// Limit 下一个索引扫描，没有排序——索引的列向与查询的排序对得上。灌量之后的答复照旧是筛选与次序本身。
func TestPreAdviceAndHandoverNoteListsReadThroughTheirIndexes(t *testing.T) {
	fixture := newCataloguePlanFixture(t)
	fixture.seed(t)
	ctx := t.Context()
	tenant := must(t, domain.NewTenantID, planTenant)
	node := must(t, domain.NewNodeReference, planNodeName(3))
	day := planEpoch.AddDate(0, 0, 9)

	// 带一日的两种读法只有十来行到八十来行，规划器可以取位图扫描再排这几十行，比按键序逐行回表便宜，所以只有不带区间的两种读法断言
	// 不排序。
	variants := []struct {
		name           string
		filter         ports.DeliveryScopeCatalogueFilter
		wholeTenant    bool
		orderedByIndex bool
	}{
		{name: "节点与一日", filter: ports.DeliveryScopeCatalogueFilter{Node: node, From: day, Until: day.AddDate(0, 0, 1)}},
		{name: "只按节点", filter: ports.DeliveryScopeCatalogueFilter{Node: node}, orderedByIndex: true},
		{name: "全租户一日", filter: ports.DeliveryScopeCatalogueFilter{From: day, Until: day.AddDate(0, 0, 1)}, wholeTenant: true},
		{name: "全租户", filter: ports.DeliveryScopeCatalogueFilter{}, wholeTenant: true, orderedByIndex: true},
	}
	lists := []struct {
		name        string
		relation    string
		nodeIndex   string
		tenantIndex string
		list        func(*testing.T, ports.DeliveryScopeCatalogueFilter) ([]listedRow, error)
	}{
		{
			name: "预报", relation: "delivery_pre_advice", nodeIndex: preAdviceNodeIndex, tenantIndex: preAdviceTenantIndex,
			list: func(t *testing.T, filter ports.DeliveryScopeCatalogueFilter) ([]listedRow, error) {
				states, err := fixture.catalogue.ListDeliveryPreAdvices(ctx, tenant, filter, planPageSize)
				rows := make([]listedRow, 0, len(states))
				for _, state := range states {
					rows = append(rows, listedRow{
						id: state.PreAdvice.ID().String(), node: state.PreAdvice.Node().String(), at: state.PreAdvice.ExpectedArrivalAt(),
					})
				}
				return rows, err
			},
		},
		{
			name: "交接单", relation: "delivery_handover_note", nodeIndex: handoverNoteNodeIndex, tenantIndex: handoverNoteTenantIndex,
			list: func(t *testing.T, filter ports.DeliveryScopeCatalogueFilter) ([]listedRow, error) {
				notes, err := fixture.catalogue.ListDeliveryHandoverNotes(ctx, tenant, filter, planPageSize)
				rows := make([]listedRow, 0, len(notes))
				for _, row := range notes {
					if row.MemberCount != 2 {
						t.Errorf("%s 的成员数 = %d, want 2", row.State.Note.ID(), row.MemberCount)
					}
					rows = append(rows, listedRow{
						id: row.State.Note.ID().String(), node: row.State.Note.Node().String(), at: row.State.Note.OpenedAt(),
					})
				}
				return rows, err
			},
		},
	}

	for _, list := range lists {
		for _, variant := range variants {
			t.Run(list.name+"/"+variant.name, func(t *testing.T) {
				fixture.recorder.take()
				rows, err := list.list(t, variant.filter)
				if err != nil {
					t.Fatalf("列表：%v", err)
				}
				checkListed(t, rows, variant.filter)
				if variant.filter.From.IsZero() && len(rows) != planPageSize {
					t.Errorf("不带区间读回 %d 行，want 满页 %d", len(rows), planPageSize)
				}

				index := list.nodeIndex
				if variant.wholeTenant {
					index = list.tenantIndex
				}
				plan := fixture.explain(t, fixture.listQuery(t, list.relation))
				summary := planSummary(plan)
				t.Logf("计划：%s", summary)
				var usesIndex, seqScan, sorted bool
				walkPlan(plan, func(node explainedNode) {
					usesIndex = usesIndex || node.IndexName == index
					seqScan = seqScan || (node.NodeType == "Seq Scan" && node.RelationName == list.relation)
					sorted = sorted || node.NodeType == "Sort" || node.NodeType == "Incremental Sort"
				})
				if seqScan || !usesIndex {
					t.Errorf("计划没有经 %s 读 %s：%s", index, list.relation, summary)
				}
				if variant.orderedByIndex {
					if sorted || !limitOverIndexScan(plan, index) {
						t.Errorf("不带区间的读不是 Limit 下按 %s 键序读满一页——索引的列向与查询的排序对不上：%s", index, summary)
					}
				}
			})
		}
	}
}
