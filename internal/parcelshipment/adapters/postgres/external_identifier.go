package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件是外部标识关联册（ADR-0184，迁移 0024、0025）的真库面：写入随来源事实同笔发生，读面按号码答包裹。

// ExternalIdentifiers 是关联册的读面。写入不在这里：关联与适用变化随它的来源事实形成，写入方是那件事实的仓储
// （委托仓储保存已接受委托、面单交易仓储保存交易），见 insertExternalIdentifierRelations 与
// insertExternalIdentifierChanges。
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

// FindParcelsByExternalIdentifier 逐字比对号码，交回此刻适用、标识包裹的关联所指的包裹，去重。已有终态（已作废、
// 已被替代）的关联不算——它的号只留历史，不再标识谁。
//
// 不按委托状态过滤：关联只随它的来源事实写下（客户包裹件号随委托接受，渠道包裹件号随面单交易包裹结果受理），接受
// 后的取消、终局也不改它的适用状态（PS CONTEXT Rules）——已取消的包裹到站仍要被认出来，节点才落得下`已取消`标记。ORDER BY 只为候选次序稳定，不表示谁优先。
//
// 歧义信号与候选在同一条语句里取：同一个快照里，一份委托要么还是`已提交`（算信号），要么已接受、关联已在册（是
// 候选），因为接受与关联同一笔事务写下。信号只认`已提交`，取值由领域常量传参，SQL 只比较不拥有编号；谓词与迁移
// 0026 的部分索引同形。客户包裹件号只在快照里（ADR-0184 决定四），按包含查，不展开数组。
func (view *ExternalIdentifiers) FindParcelsByExternalIdentifier(
	ctx context.Context,
	tenant domain.TenantID,
	number domain.ExternalIdentifierNumber,
) (ports.ExternalIdentifierParcels, error) {
	if tenant.String() == "" || number.String() == "" {
		return ports.ExternalIdentifierParcels{}, fmt.Errorf("find parcels by external identifier: tenant and number are required")
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ExternalIdentifierParcels{}, fmt.Errorf("find parcels by external identifier: %w", err)
	}
	var raws []string
	var declared bool
	if err := querier.QueryRow(ctx,
		`SELECT ARRAY(
		        SELECT DISTINCT relation.object_ref
		          FROM parcel_shipment.external_identifier_relation AS relation
		         WHERE relation.tenant_id = $1
		           AND relation.identifier_number = $2
		           AND relation.object_kind = 'PARCEL'
		           AND NOT EXISTS (
		               SELECT 1
		                 FROM parcel_shipment.external_identifier_relation_change AS change
		                WHERE change.tenant_id = relation.tenant_id
		                  AND change.identifier_kind = relation.identifier_kind
		                  AND change.allocator = relation.allocator
		                  AND change.identifier_number = relation.identifier_number
		                  AND change.object_ref = relation.object_ref
		                  AND change.basis = relation.basis)
		         ORDER BY relation.object_ref),
		        EXISTS (
		        SELECT 1
		          FROM parcel_shipment.shipment_request AS request
		         WHERE request.tenant_id = $1
		           AND request.state = $3
		           AND (request.snapshot -> 'customerParcelNumbers')
		               @> jsonb_build_array(jsonb_build_object('number', $2::text)))`,
		tenant.String(),
		number.String(),
		uint8(domain.ShipmentRequestSubmitted),
	).Scan(&raws, &declared); err != nil {
		return ports.ExternalIdentifierParcels{}, fmt.Errorf("find parcels by external identifier: %w", err)
	}
	answer := ports.ExternalIdentifierParcels{DeclaredBySubmittedRequest: declared}
	for _, raw := range raws {
		parcel, err := domain.NewDeclaredParcelID(raw)
		if err != nil {
			return ports.ExternalIdentifierParcels{}, fmt.Errorf("find parcels by external identifier: %w", err)
		}
		answer.Parcels = append(answer.Parcels, parcel)
	}
	return answer, nil
}

// insertExternalIdentifierRelations 在调用方的事务里写下一组关联。键含形成依据（ADR-0185）：别的来源事实再签同一个
// 号是另一条关联。ON CONFLICT DO NOTHING 是重放语义：来源事实每次经仓储保存都会把同一组关联再交来一次，同键即同一
// 条关联，不是冲突。
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
			 ON CONFLICT (tenant_id, identifier_kind, allocator, identifier_number, object_ref, basis) DO NOTHING`,
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

// insertExternalIdentifierChanges 在调用方的事务里写下一组适用状态变化。每一格按（租户，类型，形成依据，包裹）选中
// 原关联再写变化行：替代那一笔手里没有原交易的号。选不中（原交易那件包裹没受理、没形成关联）就什么都不写。
// ON CONFLICT DO NOTHING 让终态一次：交易每次保存都把同一组变化再交来，同键即同一次变化；先作废、后又被替代的
// 那条关联，先到的终态立住。
func insertExternalIdentifierChanges(
	ctx context.Context,
	executor bentopg.Executor,
	changes []domain.ExternalIdentifierChange,
) error {
	for _, change := range changes {
		var successorAllocator, successorNumber, successorBasis *string
		if successor, present := change.Successor(); present {
			allocator, number, basis := successor.Allocator().String(), successor.Number().String(), successor.Basis().String()
			successorAllocator, successorNumber, successorBasis = &allocator, &number, &basis
		}
		if _, err := executor.Exec(ctx,
			`INSERT INTO parcel_shipment.external_identifier_relation_change
			     (tenant_id, identifier_kind, allocator, identifier_number, object_ref, basis,
			      change_kind, successor_allocator, successor_number, successor_basis, cause, changed_at)
			 SELECT relation.tenant_id, relation.identifier_kind, relation.allocator,
			        relation.identifier_number, relation.object_ref, relation.basis,
			        $5, $6, $7, $8, $9, $10
			   FROM parcel_shipment.external_identifier_relation AS relation
			  WHERE relation.tenant_id = $1
			    AND relation.identifier_kind = $2
			    AND relation.basis = $3
			    AND relation.object_ref = $4
			 ON CONFLICT (tenant_id, identifier_kind, allocator, identifier_number, object_ref, basis) DO NOTHING`,
			change.Tenant().String(),
			change.IdentifierKind().String(),
			change.FormedBy().String(),
			change.Parcel().String(),
			change.ChangeKind().String(),
			successorAllocator,
			successorNumber,
			successorBasis,
			change.Cause().String(),
			change.ChangedAt().UTC(),
		); err != nil {
			return fmt.Errorf("insert external identifier change: %w", err)
		}
	}
	return nil
}
