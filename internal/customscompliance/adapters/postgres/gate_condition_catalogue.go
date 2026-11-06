package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// GateConditionCatalogue 实现 ports.GateConditionCatalogueRead：门禁条件目录与认定
// 两表的列表读面（ADR-0077，票 admin-web-page-wiring-frontier/06）。点读
// GateConditionView 按三维键单点作答伺候门禁编排，这里按租户上列伺候查阅——两种
// 读法各答各的问题，谁也不为对方改形状（判据同 CaseRegisterCatalogue）。
//
// 目录行与认定父子两表**一条语句取回**：ReadExecutor 不保证两条语句同一快照，分次取
// 会拼出从未同时存在的父子状态。认定走相关子查询 json_agg 各聚各的（多族 LEFT JOIN
// 笛卡尔积的教训照抄 CaseRegisterCatalogue 注释）。空认定集如实交回空切片——那是
// 「此动作在此边界本就不受门禁」的一格，与目录整行缺席（未登记 → 未决）含义相反，
// 端上不折。
//
// 按 ports.GateConditionPages 取页，缺省序是登记册三维键升序（范围、动作、边界）；认定
// 在一行之内按前置条件引用排。limit 非正是调用方编程错误：静默答一页会把「忘了传」
// 变成一个没人决定过的页大小。
type GateConditionCatalogue struct {
	db *bentopg.DB
}

func NewGateConditionCatalogue(db *bentopg.DB) (*GateConditionCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &GateConditionCatalogue{db: db}, nil
}

var _ ports.GateConditionCatalogueRead = (*GateConditionCatalogue)(nil)

// gateFindingDocument 是认定在 json_agg 里的临时词形。
type gateFindingDocument struct {
	Precondition string `json:"precondition"`
	State        string `json:"state"`
}

func (document gateFindingDocument) finding() (domain.PreconditionFinding, error) {
	precondition, err := domain.NewPreconditionReference(document.Precondition)
	if err != nil {
		return domain.PreconditionFinding{}, err
	}
	// 封闭三值经点读同一张译表（preconditionStateOf）：集外取值上抛，不折第四格
	// ——静默吸收会让旁路写入的坏数据穿进查阅答案。
	state, err := preconditionStateOf(document.State)
	if err != nil {
		return domain.PreconditionFinding{}, err
	}
	return domain.PreconditionFinding{Precondition: precondition, State: state}, nil
}

// gateConditionPages 把 ports.GateConditionPages 落到目录表的列上。q 在范围与边界引用上做字面包含。
var gateConditionPages = ports.GateConditionPages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"scope": "catalog.scope_ref", "registeredAt": "catalog.registered_at"},
	Identity: []string{"catalog.scope_ref", "catalog.action", "catalog.boundary_ref"},
	Filters:  map[string]string{"action": "catalog.action"},
	Keyword:  []string{"catalog.scope_ref", "catalog.boundary_ref"},
})

// scannedGateCondition 留着三维键与登记时刻的原值：游标位置取自 ORDER BY 用的列，不取重建后的领域对象。
type scannedGateCondition struct {
	entry                   ports.GateConditionCatalogueEntry
	scope, action, boundary string
	registeredAt            time.Time
}

func (catalogue *GateConditionCatalogue) ListGateConditions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.GateConditionCatalogueEntry], error) {
	fail := func(err error) (cataloguepage.Result[ports.GateConditionCatalogueEntry], error) {
		return cataloguepage.Result[ports.GateConditionCatalogueEntry]{}, fmt.Errorf("list gate conditions: %w", err)
	}
	// 规则表只为补列 LEFT JOIN 进来，每行至多一条，不改变行数：总数只数目录表。
	page, err := openPage(ctx, catalogue.db, gateConditionPages, query, limit,
		"customs_compliance.gate_condition_catalog AS catalog", "catalog.tenant_id = $1", tenant.String())
	if err != nil {
		return fail(err)
	}
	rows, err := page.rows(ctx,
		`catalog.scope_ref, catalog.action, catalog.boundary_ref, catalog.registered_at,
		        (SELECT COALESCE(
		                    json_agg(
		                        json_build_object(
		                            'precondition', finding.precondition_ref,
		                            'state',        finding.finding_state
		                        )
		                        ORDER BY finding.precondition_ref
		                    ),
		                    '[]'::json
		                )
		           FROM customs_compliance.gate_condition_finding AS finding
		          WHERE finding.tenant_id    = catalog.tenant_id
		            AND finding.scope_ref    = catalog.scope_ref
		            AND finding.action       = catalog.action
		            AND finding.boundary_ref = catalog.boundary_ref),
		        rule.not_a_precondition, rule.accept_coverage, rule.accept_delta, rule.accept_validity`,
		`customs_compliance.gate_condition_catalog AS catalog
		   LEFT JOIN customs_compliance.gate_condition_duty_payment_rule AS rule
		     ON rule.tenant_id    = catalog.tenant_id
		    AND rule.scope_ref    = catalog.scope_ref
		    AND rule.action       = catalog.action
		    AND rule.boundary_ref = catalog.boundary_ref`)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]scannedGateCondition, 0, limit+1)
	for rows.Next() {
		var (
			row          scannedGateCondition
			findingsJSON []byte
			// 规则行是 LEFT JOIN 来的：这一道尚未登规则时四列皆 NULL，上列如实透出空位（票 sa-cc/06）。
			notAPrecondition                      *bool
			coverageJSON, deltaJSON, validityJSON []byte
		)
		if err := rows.Scan(&row.scope, &row.action, &row.boundary, &row.registeredAt, &findingsJSON,
			&notAPrecondition, &coverageJSON, &deltaJSON, &validityJSON); err != nil {
			return fail(err)
		}
		var dutyRule *domain.DutyPaymentGateRule
		if notAPrecondition != nil {
			rebuilt, err := rebuildDutyPaymentGateRule(*notAPrecondition, coverageJSON, deltaJSON, validityJSON)
			if err != nil {
				return fail(fmt.Errorf("rebuild gate condition catalogue: %w", err))
			}
			dutyRule = &rebuilt
		}
		scope, err := domain.NewDecisionScopeReference(row.scope)
		if err != nil {
			return fail(fmt.Errorf("rebuild gate condition catalogue: %w", err))
		}
		action, err := guardedActionFrom(row.action)
		if err != nil {
			return fail(fmt.Errorf("rebuild gate condition catalogue: %w", err))
		}
		boundary, err := domain.NewCustomsProcedureReference(row.boundary)
		if err != nil {
			return fail(fmt.Errorf("rebuild gate condition catalogue: %w", err))
		}
		var documents []gateFindingDocument
		if err := json.Unmarshal(findingsJSON, &documents); err != nil {
			return fail(fmt.Errorf("认定集解码：%w", err))
		}
		findings := make([]domain.PreconditionFinding, 0, len(documents))
		for _, document := range documents {
			finding, err := document.finding()
			if err != nil {
				return fail(fmt.Errorf("rebuild gate condition catalogue: %w", err))
			}
			findings = append(findings, finding)
		}
		row.entry = ports.GateConditionCatalogueEntry{
			Scope:           scope,
			Action:          action,
			Boundary:        boundary,
			RegisteredAt:    row.registeredAt.UTC(),
			Findings:        findings,
			DutyPaymentRule: dutyRule,
		}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	result, err := cataloguepage.Finish(query, fetched, limit, page.total,
		func(row scannedGateCondition, sortField string) cataloguepage.Position {
			value := row.scope
			if sortField == "registeredAt" {
				value = cataloguepage.FormatInstant(row.registeredAt)
			}
			return cataloguepage.Position{Value: value, Identity: []string{row.scope, row.action, row.boundary}}
		})
	if err != nil {
		return fail(err)
	}
	return pageEntries(result,
		func(row scannedGateCondition) ports.GateConditionCatalogueEntry { return row.entry }), nil
}
