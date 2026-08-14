package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// CaseRequirementView 实现 ports.CaseRequirementView：判断此监管范围要不要建案。
//
// found=false 是「规则未登记」，不是「不要求」。库把依据列设成 NOT NULL 正为守住这条
// 分界：登记为不要求的那一行也说得出依据，因此「答否」与「没答」在数据上就分得开。
type CaseRequirementView struct {
	db *bentopg.DB
}

func NewCaseRequirementView(db *bentopg.DB) (*CaseRequirementView, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &CaseRequirementView{db: db}, nil
}

var _ ports.CaseRequirementView = (*CaseRequirementView)(nil)

func (view *CaseRequirementView) JudgeCaseRequirement(
	ctx context.Context,
	tenant domain.TenantID,
	jurisdiction domain.RegulatoryJurisdictionReference,
	direction domain.ManifestDirection,
	procedure domain.CustomsProcedureReference,
) (ports.CaseRequirementJudgment, bool, error) {
	directionText := direction.String()
	if directionText == "" {
		return ports.CaseRequirementJudgment{}, false,
			fmt.Errorf("judge case requirement: unknown manifest direction %d", direction)
	}

	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return ports.CaseRequirementJudgment{}, false, fmt.Errorf("judge case requirement: %w", err)
	}

	var judgment ports.CaseRequirementJudgment
	err = querier.QueryRow(ctx,
		`SELECT required, basis
		   FROM customs_compliance.case_requirement_rule
		  WHERE tenant_id = $1
		    AND jurisdiction_ref = $2
		    AND direction = $3
		    AND procedure_ref = $4`,
		tenant.String(), jurisdiction.String(), directionText, procedure.String(),
	).Scan(&judgment.Required, &judgment.Basis)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.CaseRequirementJudgment{}, false, nil
	}
	if err != nil {
		return ports.CaseRequirementJudgment{}, false, fmt.Errorf("judge case requirement: %w", err)
	}
	return judgment, true, nil
}
