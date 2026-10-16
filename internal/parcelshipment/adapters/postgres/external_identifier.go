package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件是外部标识关联册（ADR-0184，迁移 0024）的真库面：写入随来源事实同笔发生，读面按号码答包裹。

// ExternalIdentifiers 是关联册的读面。写入不在这里：关联随它的来源事实形成，写入方是那件事实的仓储
// （今天是委托仓储保存已接受委托那一笔），见 insertExternalIdentifierRelations。
type ExternalIdentifiers struct {
	db *bentopg.DB
}

var _ ports.ParcelExternalIdentifierView = (*ExternalIdentifiers)(nil)

func NewExternalIdentifiers(db *bentopg.DB) (*ExternalIdentifiers, error) {
	if db == nil {
		return nil, fmt.Errorf("new external identifiers: db is required")
	}
	return &ExternalIdentifiers{db: db}, nil
}

// FindParcelsByExternalIdentifier 逐字比对号码，交回此刻适用、标识包裹的关联所指的包裹，去重。
//
// 不按委托状态过滤：关联只在委托接受时形成，接受后的取消、终局也不改它的适用状态（PS CONTEXT Rules）——
// 已取消的包裹到站仍要被认出来，节点才落得下`已取消`标记。ORDER BY 只为候选次序稳定，不表示谁优先。
func (view *ExternalIdentifiers) FindParcelsByExternalIdentifier(
	ctx context.Context,
	tenant domain.TenantID,
	number domain.ExternalIdentifierNumber,
) ([]domain.DeclaredParcelID, error) {
	if tenant.String() == "" || number.String() == "" {
		return nil, fmt.Errorf("find parcels by external identifier: tenant and number are required")
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("find parcels by external identifier: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT DISTINCT object_ref
		   FROM parcel_shipment.external_identifier_relation
		  WHERE tenant_id = $1
		    AND identifier_number = $2
		    AND object_kind = 'PARCEL'
		  ORDER BY object_ref`,
		tenant.String(),
		number.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("find parcels by external identifier: %w", err)
	}
	defer rows.Close()
	var parcels []domain.DeclaredParcelID
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("find parcels by external identifier: %w", err)
		}
		parcel, err := domain.NewDeclaredParcelID(raw)
		if err != nil {
			return nil, fmt.Errorf("find parcels by external identifier: %w", err)
		}
		parcels = append(parcels, parcel)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find parcels by external identifier: %w", err)
	}
	return parcels, nil
}

// insertExternalIdentifierRelations 在调用方的事务里写下一组关联。ON CONFLICT DO NOTHING 是重放语义：
// 已接受委托每次经仓储保存都会把同一组关联再交来一次，同键即同一条关联，不是冲突。
func insertExternalIdentifierRelations(
	ctx context.Context,
	executor bentopg.Executor,
	relations []domain.ExternalIdentifierRelation,
) error {
	for _, relation := range relations {
		if _, err := executor.Exec(ctx,
			`INSERT INTO parcel_shipment.external_identifier_relation
			     (tenant_id, identifier_kind, allocator, identifier_number,
			      object_kind, object_ref, scope, basis, formed_at)
			 VALUES ($1, $2, $3, $4, 'PARCEL', $5, $6, $7, $8)
			 ON CONFLICT (tenant_id, identifier_kind, allocator, identifier_number, object_ref) DO NOTHING`,
			relation.Tenant().String(),
			relation.Kind().String(),
			relation.Allocator().String(),
			relation.Number().String(),
			relation.Parcel().String(),
			relation.Scope().String(),
			relation.Basis().String(),
			relation.FormedAt().UTC(),
		); err != nil {
			return fmt.Errorf("insert external identifier relation: %w", err)
		}
	}
	return nil
}
