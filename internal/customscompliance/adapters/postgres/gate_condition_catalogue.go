package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
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
// 排序按登记册三维键升序（范围、动作、边界；认定按前置条件引用），保证分页可重复。
// limit 非正是调用方编程错误：静默答一页会把「忘了传」变成一个没人决定过的页大小。
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

func (catalogue *GateConditionCatalogue) ListGateConditions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.GateConditionCatalogueEntry, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list gate conditions: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list gate conditions: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT catalog.scope_ref, catalog.action, catalog.boundary_ref, catalog.registered_at,
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
		        rule.not_a_precondition, rule.accept_coverage, rule.accept_delta, rule.accept_validity
		   FROM customs_compliance.gate_condition_catalog AS catalog
		   LEFT JOIN customs_compliance.gate_condition_duty_payment_rule AS rule
		     ON rule.tenant_id    = catalog.tenant_id
		    AND rule.scope_ref    = catalog.scope_ref
		    AND rule.action       = catalog.action
		    AND rule.boundary_ref = catalog.boundary_ref
		  WHERE catalog.tenant_id = $1
		  ORDER BY catalog.scope_ref, catalog.action, catalog.boundary_ref
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list gate conditions: %w", err)
	}
	defer rows.Close()

	entries := make([]ports.GateConditionCatalogueEntry, 0, limit)
	for rows.Next() {
		var (
			scopeRaw, actionRaw, boundaryRaw string
			registeredAt                     time.Time
			findingsJSON                     []byte
			// 规则行是 LEFT JOIN 来的：这一道尚未登规则时四列皆 NULL，上列如实透出空位（票 sa-cc/06）。
			notAPrecondition                      *bool
			coverageJSON, deltaJSON, validityJSON []byte
		)
		if err := rows.Scan(&scopeRaw, &actionRaw, &boundaryRaw, &registeredAt, &findingsJSON,
			&notAPrecondition, &coverageJSON, &deltaJSON, &validityJSON); err != nil {
			return nil, fmt.Errorf("list gate conditions: %w", err)
		}
		var dutyRule *domain.DutyPaymentGateRule
		if notAPrecondition != nil {
			rebuilt, err := rebuildDutyPaymentGateRule(*notAPrecondition, coverageJSON, deltaJSON, validityJSON)
			if err != nil {
				return nil, fmt.Errorf("rebuild gate condition catalogue: %w", err)
			}
			dutyRule = &rebuilt
		}
		scope, err := domain.NewDecisionScopeReference(scopeRaw)
		if err != nil {
			return nil, fmt.Errorf("rebuild gate condition catalogue: %w", err)
		}
		action, err := guardedActionFrom(actionRaw)
		if err != nil {
			return nil, fmt.Errorf("rebuild gate condition catalogue: %w", err)
		}
		boundary, err := domain.NewCustomsProcedureReference(boundaryRaw)
		if err != nil {
			return nil, fmt.Errorf("rebuild gate condition catalogue: %w", err)
		}
		var documents []gateFindingDocument
		if err := json.Unmarshal(findingsJSON, &documents); err != nil {
			return nil, fmt.Errorf("list gate conditions: 认定集解码：%w", err)
		}
		findings := make([]domain.PreconditionFinding, 0, len(documents))
		for _, document := range documents {
			finding, err := document.finding()
			if err != nil {
				return nil, fmt.Errorf("rebuild gate condition catalogue: %w", err)
			}
			findings = append(findings, finding)
		}
		entries = append(entries, ports.GateConditionCatalogueEntry{
			Scope:           scope,
			Action:          action,
			Boundary:        boundary,
			RegisteredAt:    registeredAt.UTC(),
			Findings:        findings,
			DutyPaymentRule: dutyRule,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list gate conditions: %w", err)
	}
	return entries, nil
}
