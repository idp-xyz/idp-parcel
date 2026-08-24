package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 第六本册子（case_requirement_rule）的写口。纪律同 case_config_registry.go 那五本：
// 一律不 UPSERT（ON CONFLICT DO NOTHING，同键已在册答`已登记`，内容比对归编排）、
// 走 RequireExecutor（无环境事务即拒）。本册没有撤销半边——建案要求规则不是逐单元
// 的判断，改规则走「先核对既有登记再决定续办」，不走状态推进。
//
// 依据列对「要求」与「不要求」都必填（迁移 CHECK 同句）：说不出依据的「不要求建案」
// 与「规则没登记」分不开，而两者的续办动作完全不同。

// CaseRequirementRegistrations 实现 ports.CaseRequirementRegistry。
type CaseRequirementRegistrations struct {
	db *bentopg.DB
}

func NewCaseRequirementRegistrations(db *bentopg.DB) (*CaseRequirementRegistrations, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &CaseRequirementRegistrations{db: db}, nil
}

var _ ports.CaseRequirementRegistry = (*CaseRequirementRegistrations)(nil)

func (registry *CaseRequirementRegistrations) RegisterCaseRequirementRule(
	ctx context.Context,
	tenant domain.TenantID,
	jurisdiction domain.RegulatoryJurisdictionReference,
	direction domain.ManifestDirection,
	procedure domain.CustomsProcedureReference,
	judgment ports.CaseRequirementJudgment,
) (ports.CaseConfigurationSaveOutcome, error) {
	// 封闭二向之外的取值是调用方编程错误，与「实例还没登记」是两回事（同读口那一句）。
	directionText := direction.String()
	if directionText == "" {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register case requirement rule: unknown manifest direction %d", direction)
	}

	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register case requirement rule: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.case_requirement_rule
			(tenant_id, jurisdiction_ref, direction, procedure_ref, required, basis)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (tenant_id, jurisdiction_ref, direction, procedure_ref) DO NOTHING`,
		tenant.String(),
		jurisdiction.String(),
		directionText,
		procedure.String(),
		judgment.Required,
		judgment.Basis,
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register case requirement rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}
