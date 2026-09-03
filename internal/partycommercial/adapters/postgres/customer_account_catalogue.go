package postgres

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件把货主客户账户目录读端口（票 admin-write-faces/04）挂在 OperationsCatalogue 上：
// 客户与合同页「客户账户」签的供数面。与另两册身份目录同住一个类型，因为三册读的是同一份
// 登记册（0015）、用同一个 domain.IdentityLifecycle 导出状态；端口分立是页面所有权的事，
// 不是实现要分家。
var _ ports.CustomerAccountCatalogueRead = (*OperationsCatalogue)(nil)

// ListCustomerAccounts 上列货主客户账户的最新修订，左连接参与方册的最新修订取客户参与方名称。
//
// status 在 SQL 里按 now() 导出，是 domain.IdentityLifecycle.StatusAt 的逐字镜像，与法人册、
// 身份本体册那两条 CASE 一字不差：停用判断在先（生效前撤下的登记自撤下时点起即已停用），
// 其次生效时点。三册用的是同一个生命周期，判据不该有第二种写法。
//
// 悬空的客户参与方（参与方册上查无此人）不过滤：账户行是登记册上的事实，照常上列，只是名称
// 缺席——过滤掉等于替读面遮住一次写入门失败。
func (catalogue *OperationsCatalogue) ListCustomerAccounts(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.CustomerAccountRow, error) {
	if err := requirePositiveLimit("list customer accounts", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list customer accounts: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT account.tenant_id, account.account_id, account.customer_party_id,
		        party.party_name,
		        account.revision, account.basis_ref, account.effective_from,
		        account.deactivated_at, account.deactivation_basis, account.recorded_at,
		        CASE
		            WHEN account.deactivated_at IS NOT NULL AND account.deactivated_at <= now()
		                THEN 'DEACTIVATED'
		            WHEN account.effective_from <= now() THEN 'EFFECTIVE'
		            ELSE 'REGISTERED'
		        END AS status
		   FROM (
		        SELECT DISTINCT ON (account_id) *
		          FROM party_commercial.customer_account_registration
		         WHERE tenant_id = $1
		         ORDER BY account_id, revision DESC
		   ) AS account
		   LEFT JOIN (
		        SELECT DISTINCT ON (party_id) party_id, party_name
		          FROM party_commercial.business_party_registration
		         WHERE tenant_id = $1
		         ORDER BY party_id, revision DESC
		   ) AS party ON party.party_id = account.customer_party_id
		  ORDER BY account.recorded_at DESC, account.account_id
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list customer accounts: %w", err)
	}
	defer rows.Close()

	catalogueRows := make([]ports.CustomerAccountRow, 0, limit)
	for rows.Next() {
		var row ports.CustomerAccountRow
		var partyName *string
		var deactivatedAt *time.Time
		var deactivationBasis *string
		if err := rows.Scan(
			&row.TenantID, &row.AccountID, &row.CustomerPartyID,
			&partyName,
			&row.Revision, &row.Basis, &row.EffectiveFrom,
			&deactivatedAt, &deactivationBasis, &row.RegisteredAt,
			&row.Status,
		); err != nil {
			return nil, fmt.Errorf("list customer accounts: %w", err)
		}
		if partyName != nil {
			row.CustomerPartyName = *partyName
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
		return nil, fmt.Errorf("list customer accounts: %w", err)
	}
	return catalogueRows, nil
}
