package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// FindRecheckableInHandoverNote 按（租户，交接单）找名下的重核对象（ADR-0212 决定六）：成员表按交接单取作业实物（索引
// delivery_handover_member_by_note，迁移 0013），收寄册按作业实物取（部分索引 reception_intake_unit_idx，迁移 0005——谓词里的
// `intake IS NOT NULL` 与它的部分条件逐字同形，规划器才认得它），再筛开立节点上当前有效版本属于重核对象的行：判据与
// FindRecheckableByMark 逐格相同（待识别、不记冲突或来由是只因歧义信号、来源已保全、不带关联、有判断版本、没有后继），只是不按号认。
//
// 收寄带来的与归入的成员都算：两种都是交接单名下的实物。这件实物后来又归入了别的交接单，这里照样交回——重核按它存下的依据重算
// 范围，判的是它最后归入的那一张，这张交接单的范围对它不起作用，重核答判断不变。
func (repository *Receptions) FindRecheckableInHandoverNote(
	ctx context.Context,
	tenant domain.TenantID,
	note domain.DeliveryHandoverNoteID,
) ([]ports.ReceptionRecord, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("find recheckable receptions in handover note: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT `+receptionColumns+`
		   FROM node_operations.reception AS r
		  WHERE r.tenant_id = $1
		    AND r.intake IS NOT NULL
		    AND (r.intake ->> 'unit') IN (
		        SELECT member.unit_id
		          FROM node_operations.delivery_handover_member AS member
		         WHERE member.tenant_id = $1 AND member.note_id = $2)
		    AND (r.intake ->> 'node') = (
		        SELECT note.node_ref
		          FROM node_operations.delivery_handover_note AS note
		         WHERE note.tenant_id = $1 AND note.note_id = $2)
		    AND r.kind = 'PENDING_IDENTIFICATION'
		    AND (NOT r.identity_conflict OR r.identity_conflict_cause = 'AMBIGUITY_ONLY')
		    AND r.source_document IS NOT NULL
		    AND r.intake ->> 'association' IS NULL
		    AND r.judgment_version IS NOT NULL
		    AND NOT EXISTS (
		        SELECT 1
		          FROM node_operations.reception AS successor
		         WHERE successor.tenant_id = r.tenant_id
		           AND successor.predecessor_version = r.judgment_version)
		  ORDER BY r.source_id`,
		tenant.String(),
		note.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("find recheckable receptions in handover note: %w", err)
	}
	records, err := collectReceptions(rows, tenant)
	if err != nil {
		return nil, fmt.Errorf("find recheckable receptions in handover note: %w", err)
	}
	return records, nil
}
