package postgres

import (
	"context"
	"fmt"
	"strings"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ConsolidationUnitsHoldingMember 实现 ports.ConsolidationUnitsHoldingMemberView：按（租户，成员）列出来源事实层里曾以它为成员
// 移入的单元（ADR-0299）。只读，走 ReadExecutor。
//
// 取自集运作业的来源事实层（consolidation_fact），不取单元行的 members 列、也不取容纳索引：那两样是此刻的派生状态，移出之后
// 或单元关闭之后就看不到这件成员曾经在里面，而要补的正是已经出了袋的那一刻之前的移动。走 consolidation_fact_by_member
// （租户，成员，单元；迁移 0023）。
type ConsolidationUnitsHoldingMember struct {
	db *bentopg.DB
}

func NewConsolidationUnitsHoldingMember(db *bentopg.DB) (*ConsolidationUnitsHoldingMember, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	return &ConsolidationUnitsHoldingMember{db: db}, nil
}

var _ ports.ConsolidationUnitsHoldingMemberView = (*ConsolidationUnitsHoldingMember)(nil)

func (view *ConsolidationUnitsHoldingMember) ReadUnitsEverHoldingMember(
	ctx context.Context,
	tenant domain.TenantID,
	member string,
) ([]domain.ConsolidationUnitID, error) {
	if strings.TrimSpace(member) == "" {
		return nil, fmt.Errorf("read units holding member: %w", errBlankMembershipObject)
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("read units holding member: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT DISTINCT unit_id
		   FROM node_operations.consolidation_fact
		  WHERE tenant_id = $1 AND member_id = $2 AND action = $3
		  ORDER BY unit_id`,
		tenant.String(), member, domain.AddMemberAction.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("read units holding member: %w", err)
	}
	defer rows.Close()
	units := []domain.ConsolidationUnitID{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("read units holding member: %w", err)
		}
		unit, err := domain.NewConsolidationUnitID(raw)
		if err != nil {
			return nil, fmt.Errorf("read units holding member: unit: %w", err)
		}
		units = append(units, unit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read units holding member: %w", err)
	}
	return units, nil
}
