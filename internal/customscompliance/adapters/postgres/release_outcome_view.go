package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// ReleaseOutcomeView 实现 ports.ReleaseOutcomeView：按申报范围盘出放行事实（0015），连同那次接收保存的业务发生
// 或适用时间（external_result.result 的 occurredAt——CC 按已登记解释规则对这一层作出的时间判断，ADR-0197
// 决定四同一取法）。记了同层冲突的那份不交出：冲突双方都在库里，当前判断不因后到的一方改变。
type ReleaseOutcomeView struct {
	db *bentopg.DB
}

func NewReleaseOutcomeView(db *bentopg.DB) (*ReleaseOutcomeView, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &ReleaseOutcomeView{db: db}, nil
}

var _ ports.ReleaseOutcomeView = (*ReleaseOutcomeView)(nil)

func (view *ReleaseOutcomeView) ListReleaseOutcomes(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
) ([]domain.ReleaseReading, error) {
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list release outcomes: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT r.source_id, r.kind, r.authority_ref, r.scope_ref, r.condition, r.received_at,
		        (e.result ->> 'occurredAt')::timestamptz
		   FROM customs_compliance.release_outcome r
		   JOIN customs_compliance.external_result e
		     ON e.tenant_id = r.tenant_id AND e.source_id = r.source_id
		  WHERE r.tenant_id = $1 AND r.scope_ref = $2
		    AND NOT e.layer_conflict
		  ORDER BY r.source_id`,
		tenant.String(), scope.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list release outcomes: %w", err)
	}
	defer rows.Close()

	var readings []domain.ReleaseReading
	for rows.Next() {
		var (
			source, kindRaw, authorityRaw, scopeRaw, condition string
			receivedAt                                         time.Time
			occurredAt                                         *time.Time
		)
		if err := rows.Scan(&source, &kindRaw, &authorityRaw, &scopeRaw, &condition, &receivedAt, &occurredAt); err != nil {
			return nil, fmt.Errorf("list release outcomes: %w", err)
		}
		if occurredAt == nil {
			// 放行事实外键钉在可归属的分层事实上，分层事实必带业务时间；读不到就是行坏了，上抛不猜。
			return nil, fmt.Errorf("list release outcomes: %s carries no occurrence time", source)
		}
		kind, err := releaseKindFrom(kindRaw)
		if err != nil {
			return nil, fmt.Errorf("list release outcomes: %w", err)
		}
		authority, err := domain.NewRegulatoryAuthorityReference(authorityRaw)
		if err != nil {
			return nil, fmt.Errorf("list release outcomes: %w", err)
		}
		releaseScope, err := domain.NewDecisionScopeReference(scopeRaw)
		if err != nil {
			return nil, fmt.Errorf("list release outcomes: %w", err)
		}
		release, err := domain.ReceiveReleaseOutcome(kind, authority, releaseScope, condition, receivedAt)
		if err != nil {
			return nil, fmt.Errorf("list release outcomes: %w", err)
		}
		readings = append(readings, domain.ReleaseReading{Source: source, Release: release, OccurredAt: occurredAt.UTC()})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list release outcomes: %w", err)
	}
	return readings, nil
}
