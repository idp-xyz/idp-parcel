package postgres

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件把参与方身份目录读端口（ADR-0077）挂在 OperationsCatalogue 上：集团与法人、
// 业务参与方两页的供数面。上列对象是各身份/关系的**最新修订**；修订史是登记册的
// 证据面，不是目录的行。
var _ ports.PartyIdentityCatalogueRead = (*OperationsCatalogue)(nil)

// ListGroupLegalEntities 上列责任法人的最新修订，左连接参与方册的最新修订取名称。
//
// status 在 SQL 里按 now() 导出，是 domain.IdentityLifecycle.StatusAt 的逐字镜像：
// 停用判断在先（生效前撤下的登记自撤下时点起即已停用），其次生效时点。目录答的是
// 「装载时点的状态」，行上同时携带全部生命周期事实，消费方要别的时点自己判。
func (catalogue *OperationsCatalogue) ListGroupLegalEntities(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.GroupLegalEntityRow, error) {
	if err := requirePositiveLimit("list group legal entities", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list group legal entities: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT entity.tenant_id, entity.legal_entity_id, entity.party_id,
		        party.party_name,
		        entity.revision, entity.basis_ref, entity.effective_from,
		        entity.deactivated_at, entity.deactivation_basis, entity.recorded_at,
		        CASE
		            WHEN entity.deactivated_at IS NOT NULL AND entity.deactivated_at <= now()
		                THEN 'DEACTIVATED'
		            WHEN entity.effective_from <= now() THEN 'EFFECTIVE'
		            ELSE 'REGISTERED'
		        END AS status
		   FROM (
		        SELECT DISTINCT ON (legal_entity_id) *
		          FROM party_commercial.legal_entity_registration
		         WHERE tenant_id = $1
		         ORDER BY legal_entity_id, revision DESC
		   ) AS entity
		   LEFT JOIN (
		        SELECT DISTINCT ON (party_id) party_id, party_name
		          FROM party_commercial.business_party_registration
		         WHERE tenant_id = $1
		         ORDER BY party_id, revision DESC
		   ) AS party ON party.party_id = entity.party_id
		  ORDER BY entity.recorded_at DESC, entity.legal_entity_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list group legal entities: %w", err)
	}
	defer rows.Close()

	catalogueRows := make([]ports.GroupLegalEntityRow, 0, limit)
	for rows.Next() {
		var row ports.GroupLegalEntityRow
		var partyName *string
		var deactivatedAt *time.Time
		var deactivationBasis *string
		if err := rows.Scan(
			&row.TenantID, &row.LegalEntityID, &row.PartyID,
			&partyName,
			&row.Revision, &row.Basis, &row.EffectiveFrom,
			&deactivatedAt, &deactivationBasis, &row.RegisteredAt,
			&row.Status,
		); err != nil {
			return nil, fmt.Errorf("list group legal entities: %w", err)
		}
		if partyName != nil {
			row.PartyName = *partyName
			row.HasPartyName = true
		}
		if deactivatedAt != nil {
			row.DeactivatedAt = *deactivatedAt
			row.HasDeactivation = true
			if deactivationBasis != nil {
				row.DeactivationBasis = *deactivationBasis
			}
		}
		catalogueRows = append(catalogueRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list group legal entities: %w", err)
	}
	return catalogueRows, nil
}

// ListPartyRelationships 上列参与方关系的最新修订，双方名称从参与方册左连接转写。
//
// status 照存的关系状态事实转写（CANDIDATE/EFFECTIVE/EXPIRED/REVOKED/SUPERSEDED），
// 不随装载时钟走：关系是否仍在有效区间内由消费方对区间判断，目录不代答——与身份
// 三册的时点导出状态刻意不同，两种状态的来源不同（关系状态经登记的真转换到达，
// 身份状态是生效/停用时点的推导）。
func (catalogue *OperationsCatalogue) ListPartyRelationships(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.PartyRelationshipRow, error) {
	if err := requirePositiveLimit("list party relationships", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list party relationships: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT relation.tenant_id, relation.relationship_id, relation.revision,
		        relation.holder_party_id, holder.party_name,
		        relation.counterparty_party_id, counterparty.party_name,
		        relation.role, relation.scope_ref, relation.basis_ref, relation.status,
		        relation.effective_starts_at, relation.effective_ends_at,
		        relation.ended_at, relation.end_basis, relation.successor_party_id,
		        relation.recorded_at
		   FROM (
		        SELECT DISTINCT ON (relationship_id) *
		          FROM party_commercial.party_relationship_registration
		         WHERE tenant_id = $1
		         ORDER BY relationship_id, revision DESC
		   ) AS relation
		   LEFT JOIN (
		        SELECT DISTINCT ON (party_id) party_id, party_name
		          FROM party_commercial.business_party_registration
		         WHERE tenant_id = $1
		         ORDER BY party_id, revision DESC
		   ) AS holder ON holder.party_id = relation.holder_party_id
		   LEFT JOIN (
		        SELECT DISTINCT ON (party_id) party_id, party_name
		          FROM party_commercial.business_party_registration
		         WHERE tenant_id = $1
		         ORDER BY party_id, revision DESC
		   ) AS counterparty ON counterparty.party_id = relation.counterparty_party_id
		  ORDER BY relation.recorded_at DESC, relation.relationship_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list party relationships: %w", err)
	}
	defer rows.Close()

	catalogueRows := make([]ports.PartyRelationshipRow, 0, limit)
	for rows.Next() {
		var row ports.PartyRelationshipRow
		var holderName, counterpartyName *string
		var endsAt, endedAt *time.Time
		var endBasis, successor *string
		if err := rows.Scan(
			&row.TenantID, &row.RelationshipID, &row.Revision,
			&row.HolderID, &holderName,
			&row.CounterpartyID, &counterpartyName,
			&row.Role, &row.Scope, &row.Basis, &row.Status,
			&row.EffectiveStartsAt, &endsAt,
			&endedAt, &endBasis, &successor,
			&row.RegisteredAt,
		); err != nil {
			return nil, fmt.Errorf("list party relationships: %w", err)
		}
		if holderName != nil {
			row.HolderName = *holderName
			row.HasHolderName = true
		}
		if counterpartyName != nil {
			row.CounterpartyName = *counterpartyName
			row.HasCounterpartyName = true
		}
		if endsAt != nil {
			row.EffectiveEndsAt = *endsAt
			row.HasEffectiveEnd = true
		}
		if endedAt != nil {
			row.EndedAt = *endedAt
			row.HasEnd = true
		}
		if endBasis != nil {
			row.EndBasis = *endBasis
		}
		if successor != nil {
			row.SuccessorID = *successor
		}
		catalogueRows = append(catalogueRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list party relationships: %w", err)
	}
	return catalogueRows, nil
}
