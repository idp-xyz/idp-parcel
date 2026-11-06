package postgres

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件把货主客户账户目录读端口（票 admin-write-faces/04）挂在 OperationsCatalogue 上：
// 客户与合同页「客户账户」签的供数面。与另两册身份目录同住一个类型，因为三册读的是同一份
// 登记册（0015）、用同一个 domain.IdentityLifecycle 导出状态；端口分立是页面所有权的事，
// 不是实现要分家。
//
// 翻页、排序与筛选照 ADR-0144（票 catalogue-read-pagination/02）：先把每个账户折叠到最新修订、导出状态、
// 左连接参与方名称（customerAccountListed 里的 listed），再在它上面按筛选与 q 数总数、按「排序维值 + 账户号」
// 取游标之后的行——状态是导出列，筛选只能落在折叠之后。决胜键账户号与排序同向；缺省序与其理由写在
// ports.CustomerAccountCatalogue 上。q 在 account_id、customer_party_id、party_name 三列上做不分大小写的
// 字面包含匹配。
var _ ports.CustomerAccountCatalogueRead = (*OperationsCatalogue)(nil)

// customerAccountListed 是总数与取页共用的前半句。
//
// status 在 SQL 里按 now() 导出，是 domain.IdentityLifecycle.StatusAt 的逐字镜像，与法人册、
// 身份本体册那两条 CASE 一字不差：停用判断在先（生效前撤下的登记自撤下时点起即已停用），
// 其次生效时点。三册用的是同一个生命周期，判据不该有第二种写法。
//
// 悬空的客户参与方（参与方册上查无此人）不过滤：账户行是登记册上的事实，照常上列，只是名称
// 缺席——过滤掉等于替读面遮住一次写入门失败。
const customerAccountListed = `WITH account AS (
        SELECT DISTINCT ON (account_id) *
          FROM party_commercial.customer_account_registration
         WHERE tenant_id = $1
         ORDER BY account_id, revision DESC
     ), party AS (
        SELECT DISTINCT ON (party_id) party_id, party_name
          FROM party_commercial.business_party_registration
         WHERE tenant_id = $1
         ORDER BY party_id, revision DESC
     ), listed AS (
        SELECT account.tenant_id, account.account_id, account.customer_party_id,
               party.party_name,
               account.revision, account.basis_ref, account.effective_from,
               account.deactivated_at, account.deactivation_basis, account.recorded_at,
               CASE
                   WHEN account.deactivated_at IS NOT NULL AND account.deactivated_at <= now()
                       THEN 'DEACTIVATED'
                   WHEN account.effective_from <= now() THEN 'EFFECTIVE'
                   ELSE 'REGISTERED'
               END AS status
          FROM account
          LEFT JOIN party ON party.party_id = account.customer_party_id
     )`

// customerAccountPages 把 ports.CustomerAccountCatalogue 落到 listed 导出的列上；行标识是 account_id。
var customerAccountPages = ports.CustomerAccountCatalogue.MustBind(cataloguepage.Columns{
	Sorts:    map[string]string{"registeredAt": "recorded_at", "effectiveFrom": "effective_from", "accountId": "account_id"},
	Identity: []string{"account_id"},
	Filters:  map[string]string{"status": "status", "customerPartyId": "customer_party_id"},
	Keyword:  []string{"account_id", "customer_party_id", "party_name"},
})

// ListCustomerAccounts 上列货主客户账户的最新修订：先按筛选与 q 数总数，再取游标之后的 limit+1 行，多出的一行
// 只用来判有没有下一页。
func (catalogue *OperationsCatalogue) ListCustomerAccounts(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.CustomerAccountRow], error) {
	fail := func(err error) (cataloguepage.Result[ports.CustomerAccountRow], error) {
		return cataloguepage.Result[ports.CustomerAccountRow]{}, fmt.Errorf("list customer accounts: %w", err)
	}
	if err := requirePositiveLimit("list customer accounts", limit); err != nil {
		return cataloguepage.Result[ports.CustomerAccountRow]{}, err
	}
	// 租户 $1 已落在 listed 的前半句里，listed 上不另带固定条件。
	plan, err := customerAccountPages.Plan(query, "", []any{tenant.String()}, limit)
	if err != nil {
		return fail(err)
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}

	var total int64
	if err := querier.QueryRow(ctx, customerAccountListed+" "+plan.CountSQL("listed"), plan.Args...).Scan(&total); err != nil {
		return fail(err)
	}
	rows, err := querier.Query(ctx, customerAccountListed+" "+plan.PageSQL(
		`tenant_id, account_id, customer_party_id, party_name,
		 revision, basis_ref, effective_from,
		 deactivated_at, deactivation_basis, recorded_at, status`, "listed"), plan.PageArgs...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]ports.CustomerAccountRow, 0, limit+1)
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
			return fail(err)
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
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}

	page, err := cataloguepage.Finish(query, fetched, limit, total, customerAccountPosition)
	if err != nil {
		return fail(err)
	}
	return page, nil
}

// customerAccountPosition 给出一行在排序维上的位置，按声明的 ValueKind 规整。登记时刻、生效时刻与账户号都是从 listed
// 的列上原样扫进行里的值，与 ORDER BY 用的是同一份（cataloguepage.Finish 头注）。
func customerAccountPosition(row ports.CustomerAccountRow, sortField string) cataloguepage.Position {
	value := row.AccountID
	switch sortField {
	case "registeredAt":
		value = cataloguepage.FormatInstant(row.RegisteredAt)
	case "effectiveFrom":
		value = cataloguepage.FormatInstant(row.EffectiveFrom)
	}
	return cataloguepage.Position{Value: value, Identity: []string{row.AccountID}}
}
