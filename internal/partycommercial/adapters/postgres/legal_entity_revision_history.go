package postgres

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件把责任法人修订历史读端口（票 admin-web-group-legal-entities/03）挂在 OperationsCatalogue
// 上而不另立适配器：它读的是 ListGroupLegalEntities 同一张 legal_entity_registration 表、守的是
// 同一条租户作用域纪律（ADR-0003 隔离边界落在 SQL 条件上），差别只在少了 DISTINCT ON——目录取
// 每身份最新一笔，历史取一身份全部。同表同作用域挂同一只，判据同 PS 复核队列读口挂在委托查阅
// 适配器上那条；行形状不同这一点由 ports 那侧另立行类型表达，不由适配器分只表达。
var _ ports.LegalEntityRevisionHistoryRead = (*OperationsCatalogue)(nil)

// ListLegalEntityRevisions 交回一个法人登记册上的全部修订，按修订号升序。
//
// 不带 LIMIT：一个法人的修订链是要全部交出的证据面，截断的历史不是历史；修订号由写入用例
// 守连续（首笔 1、此后 +1），链长受登记节奏约束，不是无界集合。序按 revision 而不按
// recorded_at：修订号是登记方声明的次序，落库时刻只是它到达的次序——两者通常一致，不一致时
// 算数的是前者。不导出 status：每一笔各有自己的生效与停用时点，给历史上的每一笔算「此刻的
// 状态」会让被顶替的旧笔各自显出一格状态（理由在 ports.LegalEntityRevisionRow）。
func (catalogue *OperationsCatalogue) ListLegalEntityRevisions(
	ctx context.Context,
	tenant domain.TenantID,
	entity domain.LegalEntityReference,
) ([]ports.LegalEntityRevisionRow, error) {
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list legal entity revisions: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT tenant_id, legal_entity_id, party_id, revision, basis_ref, effective_from,
		        deactivated_at, deactivation_basis, recorded_at
		   FROM party_commercial.legal_entity_registration
		  WHERE tenant_id = $1 AND legal_entity_id = $2
		  ORDER BY revision ASC`,
		tenant.String(),
		entity.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list legal entity revisions: %w", err)
	}
	defer rows.Close()

	// 不在册时也交回空切片而不是 nil：传输层据此编成 []，与 ListGroupLegalEntities 空册答空数组同款。
	revisions := make([]ports.LegalEntityRevisionRow, 0)
	for rows.Next() {
		var row ports.LegalEntityRevisionRow
		var deactivatedAt *time.Time
		var deactivationBasis *string
		if err := rows.Scan(
			&row.TenantID, &row.LegalEntityID, &row.PartyID, &row.Revision, &row.Basis, &row.EffectiveFrom,
			&deactivatedAt, &deactivationBasis, &row.RegisteredAt,
		); err != nil {
			return nil, fmt.Errorf("list legal entity revisions: %w", err)
		}
		if deactivatedAt != nil {
			row.DeactivatedAt = *deactivatedAt
			row.HasDeactivation = true
			if deactivationBasis != nil {
				row.DeactivationBasis = *deactivationBasis
			}
		}
		revisions = append(revisions, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list legal entity revisions: %w", err)
	}
	return revisions, nil
}
