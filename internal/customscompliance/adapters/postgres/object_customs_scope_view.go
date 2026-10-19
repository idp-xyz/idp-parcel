package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// ObjectCustomsScopeView 实现 ports.ObjectCustomsScopeView：按对象引用反查本上下文自己的申报单元与案件两本册子
// （ADR-0206 决定四；先例 ParcelDeclarationFactsView）。
//
//   - 当前单元：currentDeclarationUnitCTE，与 ParcelDeclarationFactsView 用同一份；程序取单元行自己的 procedure_ref。
//   - 在案未成单元：案件的包裹关联含该对象，而该案下没有任何当前单元含它——对象受关务管、范围还没成形。
//
// 只读，走 ReadExecutor。
type ObjectCustomsScopeView struct {
	db *bentopg.DB
}

func NewObjectCustomsScopeView(db *bentopg.DB) (*ObjectCustomsScopeView, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &ObjectCustomsScopeView{db: db}, nil
}

var _ ports.ObjectCustomsScopeView = (*ObjectCustomsScopeView)(nil)

func (view *ObjectCustomsScopeView) LoadObjectCustomsScope(
	ctx context.Context,
	tenant domain.TenantID,
	object domain.DeclaredParcelReference,
) (domain.ObjectCustomsScope, error) {
	none := domain.ObjectCustomsScope{}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return none, fmt.Errorf("load object customs scope: %w", err)
	}

	rows, err := querier.Query(ctx,
		`WITH `+currentDeclarationUnitCTE+`
		 SELECT 'UNIT' AS kind, cu.unit_id AS ref, cu.procedure_ref
		   FROM current_unit cu
		 UNION ALL
		 SELECT 'CASE' AS kind, c.case_id AS ref, c.procedure_ref
		   FROM customs_compliance.customs_case c
		  WHERE c.tenant_id = $1
		    AND c.parcels @> jsonb_build_array(jsonb_build_object('parcel', $2::text))
		    AND NOT EXISTS (SELECT 1 FROM current_unit cu WHERE cu.case_id = c.case_id)
		 ORDER BY kind, ref`,
		tenant.String(), object.String(),
	)
	if err != nil {
		return none, fmt.Errorf("load object customs scope: %w", err)
	}
	defer rows.Close()

	var scope domain.ObjectCustomsScope
	for rows.Next() {
		var kind, ref, procedureRaw string
		if err := rows.Scan(&kind, &ref, &procedureRaw); err != nil {
			return none, fmt.Errorf("load object customs scope: %w", err)
		}
		procedure, err := domain.NewCustomsProcedureReference(procedureRaw)
		if err != nil {
			return none, fmt.Errorf("load object customs scope: %w", err)
		}
		switch kind {
		case "UNIT":
			unit, err := domain.NewDecisionScopeReference(ref)
			if err != nil {
				return none, fmt.Errorf("load object customs scope: %w", err)
			}
			scope.Units = append(scope.Units, domain.UnitScope{Scope: unit, Procedure: procedure})
		case "CASE":
			scope.CasesWithoutUnit = append(scope.CasesWithoutUnit, procedure)
		}
	}
	if err := rows.Err(); err != nil {
		return none, fmt.Errorf("load object customs scope: %w", err)
	}
	return scope, nil
}
