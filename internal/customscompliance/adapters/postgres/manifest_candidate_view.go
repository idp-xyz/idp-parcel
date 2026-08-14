package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// ManifestCandidateView 实现 ports.ManifestCandidateView：按监管程序与方向盘出可供
// 关联的申报单元候选。
//
// 这里没有 configured 那一格，也不该有：空清单就是「没有能匹配的」如实答案，引用保持
// 待关联（CONTEXT 生命周期 258）。读不回则是依赖故障，照原样上抛——两者绝不能合并，
// 因为把一次读取失败答成空清单，会让「查不到候选」被当成「确实没有候选」，而后者会
// 促成一份本该等待的引用被永久搁置。
type ManifestCandidateView struct {
	db *bentopg.DB
}

func NewManifestCandidateView(db *bentopg.DB) (*ManifestCandidateView, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &ManifestCandidateView{db: db}, nil
}

var _ ports.ManifestCandidateView = (*ManifestCandidateView)(nil)

// LoadAssociationCandidates 交回该程序与方向下的全部候选。范围逐行取回而不在这里
// 过滤：唯一匹配由领域 Associate 逐维比对判定，适配器先替它挑一遍就等于把那条
// 「恰一个才关联」的规则挪出领域，挪走之后零个与多个的分界也跟着没人守。
func (view *ManifestCandidateView) LoadAssociationCandidates(
	ctx context.Context,
	tenant domain.TenantID,
	procedure domain.CustomsProcedureReference,
	direction domain.ManifestDirection,
) ([]domain.AssociationCandidate, error) {
	directionText := direction.String()
	if directionText == "" {
		return nil, fmt.Errorf("load association candidates: unknown manifest direction %d", direction)
	}

	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("load association candidates: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT unit_id, scope_ref
		   FROM customs_compliance.association_candidate
		  WHERE tenant_id = $1 AND procedure_ref = $2 AND direction = $3
		  ORDER BY unit_id, scope_ref`,
		tenant.String(), procedure.String(), directionText,
	)
	if err != nil {
		return nil, fmt.Errorf("load association candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]domain.AssociationCandidate, 0)
	for rows.Next() {
		var unitID, scopeRef string
		if err := rows.Scan(&unitID, &scopeRef); err != nil {
			return nil, fmt.Errorf("load association candidates: %w", err)
		}
		unit, err := domain.NewDeclarationUnitID(unitID)
		if err != nil {
			return nil, fmt.Errorf("load association candidates: %w", err)
		}
		scope, err := domain.NewDecisionScopeReference(scopeRef)
		if err != nil {
			return nil, fmt.Errorf("load association candidates: %w", err)
		}
		// 程序与方向不从行里读回：它们是本次查询的等值条件，行内取值必然与入参相同。
		candidates = append(candidates, domain.AssociationCandidate{
			Unit:      unit,
			Procedure: procedure,
			Direction: direction,
			Scope:     scope,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load association candidates: %w", err)
	}
	return candidates, nil
}
