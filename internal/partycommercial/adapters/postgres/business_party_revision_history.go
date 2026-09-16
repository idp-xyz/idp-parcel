package postgres

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件把业务参与方修订历史读端口（票 admin-web-group-legal-entities/12）挂在 OperationsCatalogue
// 上而不另立适配器，判据与 ListLegalEntityRevisions 挂同一只的那条一字不改：它读的是 ListBusinessParties
// 同一张 business_party_registration 表、守的是同一条租户作用域纪律（ADR-0003 隔离边界落在 SQL 条件上），
// 差别只在少了 DISTINCT ON——目录取每身份最新一笔，历史取一身份全部。
var _ ports.BusinessPartyRevisionHistoryRead = (*OperationsCatalogue)(nil)

// ListBusinessPartyRevisions 交回一个参与方登记册上的全部修订，按修订号升序。
//
// 不带 LIMIT、序按 revision 不按 recorded_at、不导出 status，三条取舍与理由同 ListLegalEntityRevisions：
// 修订链是要全部交出的证据面；修订号是登记方声明的次序，落库时刻只是它到达的次序；每一笔各有自己的
// 生效与停用时点，给历史上的每一笔算「此刻的状态」会让被顶替的旧笔各自显出一格状态。名称随每一笔
// 一起取：它登在本册自己的行上、随修订走（理由在 ports.BusinessPartyRevisionRow）。
func (catalogue *OperationsCatalogue) ListBusinessPartyRevisions(
	ctx context.Context,
	tenant domain.TenantID,
	party domain.PartyID,
) ([]ports.BusinessPartyRevisionRow, error) {
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list business party revisions: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT tenant_id, party_id, party_name, revision, basis_ref, effective_from,
		        deactivated_at, deactivation_basis, recorded_at
		   FROM party_commercial.business_party_registration
		  WHERE tenant_id = $1 AND party_id = $2
		  ORDER BY revision ASC`,
		tenant.String(),
		party.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("list business party revisions: %w", err)
	}
	defer rows.Close()

	// 不在册时也交回空切片而不是 nil：传输层据此编成 []，与 ListBusinessParties 空册答空数组同款。
	revisions := make([]ports.BusinessPartyRevisionRow, 0)
	for rows.Next() {
		var row ports.BusinessPartyRevisionRow
		var deactivatedAt *time.Time
		var deactivationBasis *string
		if err := rows.Scan(
			&row.TenantID, &row.PartyID, &row.PartyName, &row.Revision, &row.Basis, &row.EffectiveFrom,
			&deactivatedAt, &deactivationBasis, &row.RegisteredAt,
		); err != nil {
			return nil, fmt.Errorf("list business party revisions: %w", err)
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
		return nil, fmt.Errorf("list business party revisions: %w", err)
	}
	return revisions, nil
}
