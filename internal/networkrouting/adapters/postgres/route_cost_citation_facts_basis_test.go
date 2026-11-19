package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证逐段成本出处里的包裹事实依据在库里收封闭集（迁移 0036）：每一处会落出处行模型的 jsonb 列各有一条
// CHECK，封闭集之外的字面与非字符串在写入那一刻就被拒；出处缺依据那一格（缺席即客户声明）、没有出处数组、数组为空、列为 NULL
// 照收。封闭集取出处读得回的那几格依据（ParcelFactsBasis.PricesACandidate 为真的），领域认得却不进出处的依据也在库里被拒；
// 两边对不齐时本文件的用例会红。

// sqlExecutor 是单条连接与连接池共有的写口：约束行为用例走连接池，迁移用例要停在某一份迁移之前，走单条连接。
type sqlExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// costCitationColumn 是一处会落逐段成本出处行模型（costCitationRow）的 jsonb 列。
type costCitationColumn struct {
	// name 是「表.列」，table 是它所在的表。
	name, table string
	constraint  string
	// arrayKey 是出处数组在该列文档里的键：计划里叫 cost_citations，改善依据里叫 superseded_cost_citations。
	arrayKey string
	// members 是该列文档里出处数组以外必带的成员：决定册要求新计划本体的 version 与平铺的新计划版本一致。
	members  string
	nullable bool
	// insert 落一行合规的记录，被约束的那一格取 document；document 为 nil 即该列为 NULL。label 保证行之间互不撞键。
	insert func(ctx context.Context, db sqlExecutor, label string, document *string) error
}

// document 把出处数组（原样的 JSON 文本）装成该列的一份文档；citations 为空串即文档里不带出处数组。
func (column costCitationColumn) document(citations string) string {
	members := column.members
	if citations != "" {
		if members != "" {
			members += ","
		}
		members += fmt.Sprintf("%q:%s", column.arrayKey, citations)
	}
	return "{" + members + "}"
}

// costCitationColumns 是适配器里 json.Marshal 进 planRow 与改善依据行模型的全部落点。
var costCitationColumns = []costCitationColumn{
	{
		name: "initial_route.plan", table: "initial_route",
		constraint: "initial_route_plan_facts_basis_closed",
		arrayKey:   "cost_citations", nullable: true,
		insert: insertInitialRoutePlan,
	},
	{
		name: "route_reassessment.new_plan", table: "route_reassessment",
		constraint: "route_reassessment_new_plan_facts_basis_closed",
		arrayKey:   "cost_citations", nullable: true,
		insert: insertReassessmentNewPlan,
	},
	{
		name: "route_reassessment.improvement_basis", table: "route_reassessment",
		constraint: "route_reassessment_improvement_basis_facts_basis_closed",
		arrayKey:   "superseded_cost_citations", nullable: true,
		insert: insertReassessmentImprovementBasis,
	},
	{
		name: "authorized_reroute_decision.new_plan", table: "authorized_reroute_decision",
		constraint: "authorized_reroute_decision_new_plan_facts_basis_closed",
		arrayKey:   "cost_citations", members: `"version":"RPV-CC-NEW"`,
		insert: insertDecisionNewPlan,
	},
}

// costCitation 造一条出处；factsBasis 是依据那一格的原样 JSON 文本，空串即不带 facts_basis 键。
func costCitation(factsBasis string) string {
	members := `"ordinal":1,"evaluation_id":"SYN-EVAL-1","plan_reference":"SYN-PLAN-COST@1","comparison_amount":"42.00","comparison_currency":"CNY"`
	if factsBasis != "" {
		members += `,"facts_basis":` + factsBasis
	}
	return "{" + members + "}"
}

func citationArray(citations ...string) string {
	return "[" + strings.Join(citations, ",") + "]"
}

func insertInitialRoutePlan(ctx context.Context, db sqlExecutor, label string, document *string) error {
	if document == nil {
		_, err := db.Exec(ctx,
			`INSERT INTO network_routing.initial_route
				(tenant_id, customer_account_id, shipment_request_id, acceptance_baseline,
				 declared_parcel_id, service_purpose, conclusion, plan_version, plan, no_route)
			 VALUES ('SYN-TENANT-CC', 'SYN-CUSTOMER-CC', 'SYN-REQUEST-CC', 'baseline-v1',
			         $1, 'LAST_MILE_DELIVERY', 'NO_CURRENT_ROUTE', NULL, NULL, '{}'::jsonb)`, label)
		return err
	}
	_, err := db.Exec(ctx,
		`INSERT INTO network_routing.initial_route
			(tenant_id, customer_account_id, shipment_request_id, acceptance_baseline,
			 declared_parcel_id, service_purpose, conclusion, plan_version, plan, no_route)
		 VALUES ('SYN-TENANT-CC', 'SYN-CUSTOMER-CC', 'SYN-REQUEST-CC', 'baseline-v1',
		         $1, 'LAST_MILE_DELIVERY', 'ROUTE_FORMED', $2, $3::jsonb, NULL)`,
		label, "RPV-"+label, document)
	return err
}

// insertLapsedReassessment 落一行不带新计划的复核：它是授权改路决定指回的那一行，也是各列为 NULL 时的载体。
func insertLapsedReassessment(ctx context.Context, db sqlExecutor, correlation string) error {
	_, err := db.Exec(ctx,
		`INSERT INTO network_routing.route_reassessment
			(tenant_id, correlation_id, customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose,
			 conclusion, reviewed_plan, lapse_basis, candidate_state,
			 trigger_occurred_at, reassessed_at)
		 VALUES ('SYN-TENANT-CC', $1, 'SYN-CUSTOMER-CC', 'SYN-REQUEST-CC',
		         'baseline-v1', 'SYN-PARCEL-CC', 'LAST_MILE_DELIVERY',
		         'PLAN_LAPSED', 'RPV-0001', 'closure-7', 'NO_QUALIFIED_CANDIDATES',
		         now(), now())`, correlation)
	return err
}

// insertReroutedReassessment 落一行`已改路`：失效依据与改善依据恰有其一，新计划与决定都在场。
func insertReroutedReassessment(
	ctx context.Context, db sqlExecutor, correlation string, lapseBasis, newPlan, improvementBasis *string,
) error {
	_, err := db.Exec(ctx,
		`INSERT INTO network_routing.route_reassessment
			(tenant_id, correlation_id, customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose,
			 conclusion, reviewed_plan, lapse_basis, candidate_state, reroute_state,
			 new_plan, decision, improvement_basis, trigger_occurred_at, reassessed_at)
		 VALUES ('SYN-TENANT-CC', $1, 'SYN-CUSTOMER-CC', 'SYN-REQUEST-CC',
		         'baseline-v1', 'SYN-PARCEL-CC', 'LAST_MILE_DELIVERY',
		         'REROUTED', 'RPV-0001', $2, 'CANDIDATES_AVAILABLE', 'AUTOMATIC_ALLOWED',
		         $3::jsonb, '{}'::jsonb, $4::jsonb, now(), now())`,
		correlation, lapseBasis, newPlan, improvementBasis)
	return err
}

func insertReassessmentNewPlan(ctx context.Context, db sqlExecutor, label string, document *string) error {
	if document == nil {
		return insertLapsedReassessment(ctx, db, label)
	}
	lapse := "closure-7"
	return insertReroutedReassessment(ctx, db, label, &lapse, document, nil)
}

func insertReassessmentImprovementBasis(ctx context.Context, db sqlExecutor, label string, document *string) error {
	emptyPlan := `{}`
	lapse := "closure-7"
	if document == nil {
		return insertReroutedReassessment(ctx, db, label, &lapse, &emptyPlan, nil)
	}
	return insertReroutedReassessment(ctx, db, label, nil, &emptyPlan, document)
}

func insertDecisionNewPlan(ctx context.Context, db sqlExecutor, label string, document *string) error {
	if err := insertLapsedReassessment(ctx, db, label); err != nil {
		return err
	}
	_, err := db.Exec(ctx,
		`INSERT INTO network_routing.authorized_reroute_decision
			(tenant_id, suggestion_correlation_id, customer_account_id, shipment_request_id,
			 acceptance_baseline, declared_parcel_id, service_purpose,
			 chosen_candidate, original_plan, new_plan_version, new_plan,
			 decided_by, decision_basis, decided_at)
		 VALUES ('SYN-TENANT-CC', $1, 'SYN-CUSTOMER-CC', 'SYN-REQUEST-CC',
		         'baseline-v1', 'SYN-PARCEL-CC', 'LAST_MILE_DELIVERY',
		         'candidate-1', 'RPV-0001', 'RPV-CC-NEW', $2::jsonb,
		         'OPERATOR/https://issuer.example#ops-7', 'SYN-APPROVAL-CC', now())`,
		label, document)
	return err
}

// requireCheckViolation 要求写入被指名的那一条 CHECK 拒掉。只看「报了错」不够：jsonpath 里的结构错误也会报错，却不是约束
// 在拒；CHECK 对 NULL 放行，约束若写成会算出 NULL 就会无声放过——所以认 SQLSTATE 与约束名。
func requireCheckViolation(t *testing.T, err error, constraint, what string) {
	t.Helper()
	var refusal *pgconn.PgError
	if !errors.As(err, &refusal) || refusal.Code != "23514" || refusal.ConstraintName != constraint {
		t.Fatalf("%s：want 违反 CHECK %s，实得 %v", what, constraint, err)
	}
}

type citationCase struct {
	name      string
	citations string
}

// Covers: 每一处落出处的列：列为 NULL、没有出处数组、数组为空、出处缺依据那一格（缺席即客户声明）照收；依据在场就必须是封闭集
// 里的字符串——退役的「并用」、认不出的字面、空串（不算缺席）、大小写或空白不对的字面拒收，非字符串（数字、对象、JSON null、
// 布尔、数组）同样拒收：jsonpath 的 lax 模式里不同类型的比较算 unknown、过滤器当假，只写「不等于各字面」会把它们放过去，这里
// 逐一钉住。领域认得的各个依据收不收，由 TestTheCostCitationColumnsAcceptEveryFactsBasisThatCanPriceACandidateAndRefuseTheRest 遍历 Go 全集来证，
// 这里不再各抄一遍。
func TestACostCitationColumnAcceptsWhatReadsBackAndRefusesEveryOtherFactsBasis(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := t.Context()
	one := func(factsBasis string) string { return citationArray(costCitation(factsBasis)) }

	accepted := []citationCase{
		{"the document carries no citation array", ""},
		{"the citation array is empty", `[]`},
		{"a citation names no facts basis", one("")},
		{"a citation naming no facts basis sits among cited ones", citationArray(costCitation(""), costCitation(`"PHYSICAL"`))},
	}
	refused := []citationCase{
		{"the retired physical-with-declaration literal", one(`"PHYSICAL_WITH_DECLARATION"`)},
		{"an unknown literal", one(`"LAST_WEIGHED"`)},
		{"an empty string, which does not count as an absent key", one(`""`)},
		{"a literal in lower case", one(`"physical"`)},
		{"a literal with a leading blank", one(`" PHYSICAL"`)},
		{"a number", one(`1`)},
		{"an object", one(`{"basis":"PHYSICAL"}`)},
		{"a JSON null", one(`null`)},
		{"a boolean true", one(`true`)},
		{"a boolean false", one(`false`)},
		{"an array holding a closed-set literal", one(`["PHYSICAL"]`)},
		{"an empty array", one(`[]`)},
		{"an outside literal after a closed-set one", citationArray(costCitation(`"PHYSICAL"`), costCitation(`"LAST_WEIGHED"`))},
		{"a non-string after a citation naming no facts basis", citationArray(costCitation(""), costCitation(`2`))},
	}

	for _, column := range costCitationColumns {
		t.Run(column.name, func(t *testing.T) {
			if column.nullable {
				if err := column.insert(ctx, pool, column.name+"/null-column", nil); err != nil {
					t.Fatalf("列为 NULL 应落得了库：%v", err)
				}
			}
			for _, test := range accepted {
				document := column.document(test.citations)
				if err := column.insert(ctx, pool, column.name+"/accepted/"+test.name, &document); err != nil {
					t.Errorf("%s 应落得了库：%v", test.name, err)
				}
			}
			for _, test := range refused {
				document := column.document(test.citations)
				err := column.insert(ctx, pool, column.name+"/refused/"+test.name, &document)
				requireCheckViolation(t, err, column.constraint, test.name)
			}
		})
	}
}

// Covers: Go 侧与库约束对齐——遍历领域认得的每一个依据：能出价的（PricesACandidate 为真，出处读得回），每一处落出处的列都收；
// 不能出价的（领域认得、但构造门不让进出处，读回同样拒），每一处落出处的列都以指名的 CHECK 拒。Go 侧再加一个值而迁移没跟，
// 这里红。两类都要遍历到：一类遍历不到，对齐那一半就没有被证。
func TestTheCostCitationColumnsAcceptEveryFactsBasisThatCanPriceACandidateAndRefuseTheRest(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := t.Context()

	priced, unpriced := 0, 0
	for basis := domain.ParcelFactsDeclaration; basis.Valid(); basis++ {
		if _, err := domain.ParseParcelFactsBasis(basis.String()); err != nil {
			t.Fatalf("%s 领域自己读不回：%v", basis, err)
		}
		if basis.PricesACandidate() {
			priced++
		} else {
			unpriced++
		}
		for _, column := range costCitationColumns {
			document := column.document(citationArray(costCitation(strconv.Quote(basis.String()))))
			err := column.insert(ctx, pool, column.name+"/"+basis.String(), &document)
			if basis.PricesACandidate() {
				if err != nil {
					t.Errorf("%s：库不收出处读得回的依据 %s：%v", column.name, basis, err)
				}
				continue
			}
			requireCheckViolation(t, err, column.constraint, column.name+" 收了出处读不回的依据 "+basis.String())
		}
	}
	if priced == 0 || unpriced == 0 {
		t.Fatalf("遍历到能出价的 %d 个、不能出价的 %d 个，两类都要有，对齐才被证", priced, unpriced)
	}
}

// 约束定义里写进去的依据字面：pg_get_constraintdef 把 jsonpath 规整成 `@."facts_basis" == "字面"` 的样子。
var constraintFactsBasisLiteral = regexp.MustCompile(`== "([^"]*)"`)

// Covers: 库约束不收出处读不回的字面——约束定义里写着的每个字面，领域既要认得（ParseParcelFactsBasis），又要让它能进成本出处
// （PricesACandidate 为真）。Go 侧收窄（如退役一个值、或某个依据不再能出价）而迁移没跟时，这里红。与
// TestTheCostCitationColumnsAcceptEveryFactsBasisThatCanPriceACandidateAndRefuseTheRest 合起来，两边的集合相等。
func TestTheCostCitationConstraintsNameOnlyFactsBasesThatCanPriceACandidate(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := t.Context()

	for _, column := range costCitationColumns {
		var definition string
		if err := pool.QueryRow(ctx,
			`SELECT pg_get_constraintdef(oid) FROM pg_constraint
			  WHERE connamespace = 'network_routing'::regnamespace AND conname = $1`,
			column.constraint).Scan(&definition); err != nil {
			t.Fatalf("%s：读约束 %s 的定义：%v", column.name, column.constraint, err)
		}
		literals := constraintFactsBasisLiteral.FindAllStringSubmatch(definition, -1)
		if len(literals) == 0 {
			t.Fatalf("%s：约束定义里没取到任何依据字面，取法失效：%s", column.name, definition)
		}
		for _, match := range literals {
			basis, err := domain.ParseParcelFactsBasis(match[1])
			if err != nil {
				t.Errorf("%s：约束收 %q，领域读不回：%v", column.name, match[1], err)
				continue
			}
			if !basis.PricesACandidate() {
				t.Errorf("%s：约束收 %q，它没有出价、领域不让进成本出处，读回会拒", column.name, match[1])
			}
		}
	}
}
