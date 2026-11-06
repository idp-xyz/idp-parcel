package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// RuleCatalogue 实现 ports.RuleCatalogueRead：合规规则库的列表读面（ADR-0077）。
// 读的就是两本规则登记册本表，不是第二份数据；判断读口（JudgeCaseRequirement /
// LoadInterpretationRule）按键单点作答，这里按租户上列——两种读法各答各的问题，
// 谁也不为对方改形状。
//
// 两册按 ports 里的声明（CaseRequirementRulePages / InterpretationRulePages）取页，缺省序
// 合起来就是登记册键序：建案要求规则按（辖区，方向，程序）；解释规则按（层，辖区，生效
// 起点），同一选择键下的各版按生效起点升序——决胜键与排序同向（ADR-0144 决定三）。
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

// caseRequirementRulePages 把 ports.CaseRequirementRulePages 落到列上。q 在辖区、程序与依据上做字面包含。
var caseRequirementRulePages = ports.CaseRequirementRulePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"jurisdiction": "jurisdiction_ref"},
	Identity: []string{"jurisdiction_ref", "direction", "procedure_ref"},
	Filters:  map[string]string{"direction": "direction"},
	Keyword:  []string{"jurisdiction_ref", "procedure_ref", "basis"},
})

// scannedCaseRequirementRule 留着键（辖区、方向、程序）的原值：游标位置取自 ORDER BY 用的列，不取重建后的领域对象。
type scannedCaseRequirementRule struct {
	entry                                  ports.CaseRequirementRuleEntry
	jurisdiction, direction, procedureCode string
}

func (catalogue *RuleCatalogue) ListCaseRequirementRules(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.CaseRequirementRuleEntry], error) {
	fail := func(err error) (cataloguepage.Result[ports.CaseRequirementRuleEntry], error) {
		return cataloguepage.Result[ports.CaseRequirementRuleEntry]{}, fmt.Errorf("list case requirement rules: %w", err)
	}
	const source = "customs_compliance.case_requirement_rule"
	page, err := openPage(ctx, catalogue.db, caseRequirementRulePages, query, limit, source, "tenant_id = $1", tenant.String())
	if err != nil {
		return fail(err)
	}
	rows, err := page.rows(ctx, "jurisdiction_ref, direction, procedure_ref, required, basis", source)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]scannedCaseRequirementRule, 0, limit+1)
	for rows.Next() {
		var row scannedCaseRequirementRule
		var judgment ports.CaseRequirementJudgment
		if err := rows.Scan(
			&row.jurisdiction, &row.direction, &row.procedureCode,
			&judgment.Required, &judgment.Basis,
		); err != nil {
			return fail(err)
		}
		jurisdiction, err := domain.NewRegulatoryJurisdictionReference(row.jurisdiction)
		if err != nil {
			return fail(fmt.Errorf("rebuild case requirement rule: %w", err))
		}
		direction, err := manifestDirectionFrom(row.direction)
		if err != nil {
			return fail(fmt.Errorf("rebuild case requirement rule: %w", err))
		}
		procedure, err := domain.NewCustomsProcedureReference(row.procedureCode)
		if err != nil {
			return fail(fmt.Errorf("rebuild case requirement rule: %w", err))
		}
		row.entry = ports.CaseRequirementRuleEntry{
			Jurisdiction: jurisdiction,
			Direction:    direction,
			Procedure:    procedure,
			Judgment:     judgment,
		}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	result, err := cataloguepage.Finish(query, fetched, limit, page.total,
		func(row scannedCaseRequirementRule, _ string) cataloguepage.Position {
			return cataloguepage.Position{
				Value:    row.jurisdiction,
				Identity: []string{row.jurisdiction, row.direction, row.procedureCode},
			}
		})
	if err != nil {
		return fail(err)
	}
	return pageEntries(result, func(row scannedCaseRequirementRule) ports.CaseRequirementRuleEntry { return row.entry }), nil
}

// interpretationRulePages 把 ports.InterpretationRulePages 落到列上。q 在辖区与规则引用上做字面包含。
var interpretationRulePages = ports.InterpretationRulePages.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"layer": "result_layer", "appliesFrom": "applies_from"},
	Identity: []string{"result_layer", "jurisdiction_ref", "applies_from"},
	Filters:  map[string]string{"layer": "result_layer"},
	Keyword:  []string{"jurisdiction_ref", "rule_ref"},
})

// scannedInterpretationRule 留着键（结果层、辖区、生效起点）的原值，理由同 scannedCaseRequirementRule。
type scannedInterpretationRule struct {
	entry               ports.InterpretationRuleEntry
	layer, jurisdiction string
	appliesFrom         time.Time
}

func (catalogue *RuleCatalogue) ListInterpretationRules(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.InterpretationRuleEntry], error) {
	fail := func(err error) (cataloguepage.Result[ports.InterpretationRuleEntry], error) {
		return cataloguepage.Result[ports.InterpretationRuleEntry]{}, fmt.Errorf("list interpretation rules: %w", err)
	}
	const source = "customs_compliance.interpretation_rule"
	page, err := openPage(ctx, catalogue.db, interpretationRulePages, query, limit, source, "tenant_id = $1", tenant.String())
	if err != nil {
		return fail(err)
	}
	rows, err := page.rows(ctx, "result_layer, jurisdiction_ref, rule_ref, applies_from, applies_until", source)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]scannedInterpretationRule, 0, limit+1)
	for rows.Next() {
		var row scannedInterpretationRule
		var ruleRaw string
		var appliesUntil *time.Time
		if err := rows.Scan(&row.layer, &row.jurisdiction, &ruleRaw, &row.appliesFrom, &appliesUntil); err != nil {
			return fail(err)
		}
		layer, err := resultLayerFrom(row.layer)
		if err != nil {
			return fail(fmt.Errorf("rebuild interpretation rule: %w", err))
		}
		jurisdiction, err := domain.NewRegulatoryJurisdictionReference(row.jurisdiction)
		if err != nil {
			return fail(fmt.Errorf("rebuild interpretation rule: %w", err))
		}
		rule, err := domain.NewInterpretationRuleReference(ruleRaw)
		if err != nil {
			return fail(fmt.Errorf("rebuild interpretation rule: %w", err))
		}
		row.entry = ports.InterpretationRuleEntry{
			Layer:        layer,
			Jurisdiction: jurisdiction,
			Rule:         rule,
			AppliesFrom:  row.appliesFrom.UTC(),
		}
		// NULL 终点即开放版：零值照端口约定透出，不代填「无限远」的编造时刻。
		if appliesUntil != nil {
			row.entry.AppliesUntil = appliesUntil.UTC()
		}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}
	result, err := cataloguepage.Finish(query, fetched, limit, page.total,
		func(row scannedInterpretationRule, sortField string) cataloguepage.Position {
			appliesFrom := cataloguepage.FormatInstant(row.appliesFrom)
			value := row.layer
			if sortField == "appliesFrom" {
				value = appliesFrom
			}
			return cataloguepage.Position{Value: value, Identity: []string{row.layer, row.jurisdiction, appliesFrom}}
		})
	if err != nil {
		return fail(err)
	}
	return pageEntries(result, func(row scannedInterpretationRule) ports.InterpretationRuleEntry { return row.entry }), nil
}
