package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// RuleCatalogue 实现 ports.RuleCatalogueRead：合规规则库的列表读面（ADR-0077）。
// 读的就是两本规则登记册本表，不是第二份数据；判断读口（JudgeCaseRequirement /
// LoadInterpretationRule）按键单点作答，这里按租户上列——两种读法各答各的问题，
// 谁也不为对方改形状。
//
// 排序按登记册键升序，保证分页可重复：建案要求规则按（辖区，方向，程序）；解释规则
// 按（层，辖区），同一选择键内按生效起点倒序——当前与最近的版本在前，历史版本随后。
// limit 非正是调用方编程错误：静默答一页会把「忘了传」变成一个没人决定过的页大小
// （判据与运营追踪列表读口同款）。
type RuleCatalogue struct {
	db *bentopg.DB
}

func NewRuleCatalogue(db *bentopg.DB) (*RuleCatalogue, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &RuleCatalogue{db: db}, nil
}

var _ ports.RuleCatalogueRead = (*RuleCatalogue)(nil)

func (catalogue *RuleCatalogue) ListCaseRequirementRules(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.CaseRequirementRuleEntry, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list case requirement rules: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list case requirement rules: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT jurisdiction_ref, direction, procedure_ref, required, basis
		   FROM customs_compliance.case_requirement_rule
		  WHERE tenant_id = $1
		  ORDER BY jurisdiction_ref, direction, procedure_ref
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list case requirement rules: %w", err)
	}
	defer rows.Close()

	entries := make([]ports.CaseRequirementRuleEntry, 0, limit)
	for rows.Next() {
		var jurisdictionRaw, directionRaw, procedureRaw string
		var judgment ports.CaseRequirementJudgment
		if err := rows.Scan(
			&jurisdictionRaw, &directionRaw, &procedureRaw,
			&judgment.Required, &judgment.Basis,
		); err != nil {
			return nil, fmt.Errorf("list case requirement rules: %w", err)
		}
		jurisdiction, err := domain.NewRegulatoryJurisdictionReference(jurisdictionRaw)
		if err != nil {
			return nil, fmt.Errorf("rebuild case requirement rule: %w", err)
		}
		direction, err := manifestDirectionFrom(directionRaw)
		if err != nil {
			return nil, fmt.Errorf("rebuild case requirement rule: %w", err)
		}
		procedure, err := domain.NewCustomsProcedureReference(procedureRaw)
		if err != nil {
			return nil, fmt.Errorf("rebuild case requirement rule: %w", err)
		}
		entries = append(entries, ports.CaseRequirementRuleEntry{
			Jurisdiction: jurisdiction,
			Direction:    direction,
			Procedure:    procedure,
			Judgment:     judgment,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list case requirement rules: %w", err)
	}
	return entries, nil
}

func (catalogue *RuleCatalogue) ListInterpretationRules(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.InterpretationRuleEntry, error) {
	if limit < 1 {
		return nil, fmt.Errorf("list interpretation rules: limit must be positive, got %d", limit)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list interpretation rules: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT result_layer, jurisdiction_ref, rule_ref, applies_from, applies_until
		   FROM customs_compliance.interpretation_rule
		  WHERE tenant_id = $1
		  ORDER BY result_layer, jurisdiction_ref, applies_from DESC
		  LIMIT $2`,
		tenant.String(), limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list interpretation rules: %w", err)
	}
	defer rows.Close()

	entries := make([]ports.InterpretationRuleEntry, 0, limit)
	for rows.Next() {
		var layerRaw, jurisdictionRaw, ruleRaw string
		var appliesFrom time.Time
		var appliesUntil *time.Time
		if err := rows.Scan(&layerRaw, &jurisdictionRaw, &ruleRaw, &appliesFrom, &appliesUntil); err != nil {
			return nil, fmt.Errorf("list interpretation rules: %w", err)
		}
		layer, err := resultLayerFrom(layerRaw)
		if err != nil {
			return nil, fmt.Errorf("rebuild interpretation rule: %w", err)
		}
		jurisdiction, err := domain.NewRegulatoryJurisdictionReference(jurisdictionRaw)
		if err != nil {
			return nil, fmt.Errorf("rebuild interpretation rule: %w", err)
		}
		rule, err := domain.NewInterpretationRuleReference(ruleRaw)
		if err != nil {
			return nil, fmt.Errorf("rebuild interpretation rule: %w", err)
		}
		entry := ports.InterpretationRuleEntry{
			Layer:        layer,
			Jurisdiction: jurisdiction,
			Rule:         rule,
			AppliesFrom:  appliesFrom.UTC(),
		}
		// NULL 终点即开放版：零值照端口约定透出，不代填「无限远」的编造时刻。
		if appliesUntil != nil {
			entry.AppliesUntil = appliesUntil.UTC()
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list interpretation rules: %w", err)
	}
	return entries, nil
}
