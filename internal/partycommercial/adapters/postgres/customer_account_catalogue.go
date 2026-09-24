package postgres

import (
	"context"
	"fmt"
	"sort"
	"strings"
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

type accountKeyColumn struct {
	column string
	kind   cataloguepage.ValueKind
}

// 声明里的可排维、筛选维落到 listed 的列上；行标识是 account_id。
var (
	customerAccountSortColumns = map[string]accountKeyColumn{
		"registeredAt":  {"recorded_at", cataloguepage.Instant},
		"effectiveFrom": {"effective_from", cataloguepage.Instant},
		"accountId":     {"account_id", cataloguepage.Text},
	}
	customerAccountIdentity       = accountKeyColumn{"account_id", cataloguepage.Text}
	customerAccountFilterColumns  = map[string]string{"status": "status", "customerPartyId": "customer_party_id"}
	customerAccountKeywordColumns = []string{"account_id", "customer_party_id", "party_name"}
)

// ListCustomerAccounts 上列货主客户账户的最新修订：先按筛选与 q 数总数，再取游标之后的 limit+1 行，多出的一行
// 只用来判有没有下一页。
func (catalogue *OperationsCatalogue) ListCustomerAccounts(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (ports.CataloguePage[ports.CustomerAccountRow], error) {
	fail := func(err error) (ports.CataloguePage[ports.CustomerAccountRow], error) {
		return ports.CataloguePage[ports.CustomerAccountRow]{}, fmt.Errorf("list customer accounts: %w", err)
	}
	if err := requirePositiveLimit("list customer accounts", limit); err != nil {
		return ports.CataloguePage[ports.CustomerAccountRow]{}, err
	}
	primary, mapped := customerAccountSortColumns[query.Sort.Field]
	if !mapped {
		return fail(fmt.Errorf("排序维 %q 没有落到列上（查询对象须由 ports.CustomerAccountCatalogue 解出）", query.Sort.Field))
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return fail(err)
	}
	where, args, err := customerAccountConditions(tenant, query)
	if err != nil {
		return fail(err)
	}

	var total int64
	if err := querier.QueryRow(ctx, customerAccountListed+" SELECT count(*) FROM listed WHERE "+where, args...).Scan(&total); err != nil {
		return fail(err)
	}

	keys := []accountKeyColumn{primary, customerAccountIdentity}
	direction, comparison := "ASC", ">"
	if query.Sort.Descending {
		direction, comparison = "DESC", "<"
	}
	if query.After != nil {
		values := append([]string{query.After.Value}, query.After.Identity...)
		if len(values) != len(keys) {
			return fail(fmt.Errorf("游标位置有 %d 格，排序键有 %d 列", len(values), len(keys)))
		}
		columns := make([]string, len(keys))
		placeholders := make([]string, len(keys))
		for index, key := range keys {
			value, err := accountTypedValue(key.kind, values[index])
			if err != nil {
				return fail(err)
			}
			args = append(args, value)
			columns[index], placeholders[index] = key.column, fmt.Sprintf("$%d", len(args))
		}
		where += fmt.Sprintf(" AND (%s) %s (%s)", strings.Join(columns, ", "), comparison, strings.Join(placeholders, ", "))
	}
	args = append(args, limit+1)
	rows, err := querier.Query(ctx, fmt.Sprintf(
		`%s SELECT tenant_id, account_id, customer_party_id, party_name,
		           revision, basis_ref, effective_from,
		           deactivated_at, deactivation_basis, recorded_at, status
		      FROM listed
		     WHERE %s
		     ORDER BY %s %s, %s %s
		     LIMIT $%d`,
		customerAccountListed, where, primary.column, direction, customerAccountIdentity.column, direction, len(args),
	), args...)
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

	page, more := cataloguepage.Trim(fetched, limit)
	result := ports.CataloguePage[ports.CustomerAccountRow]{Rows: page, Total: total}
	if more {
		last := page[len(page)-1]
		next, err := query.CursorAfter(cataloguepage.Position{
			Value:    customerAccountSortValue(last, query.Sort.Field),
			Identity: []string{last.AccountID},
		})
		if err != nil {
			return fail(err)
		}
		result.Next = next
	}
	return result, nil
}

// customerAccountConditions 是筛选维与 q 的条件，总数与取页共用同一组；$1 是租户，已落在 listed 的前半句里。
// 筛选维内为或（= ANY）、维间为与。
func customerAccountConditions(tenant domain.TenantID, query cataloguepage.Query) (string, []any, error) {
	clauses := []string{"TRUE"}
	args := []any{tenant.String()}
	dimensions := make([]string, 0, len(query.Filters))
	for dimension := range query.Filters {
		dimensions = append(dimensions, dimension)
	}
	sort.Strings(dimensions)
	for _, dimension := range dimensions {
		column, mapped := customerAccountFilterColumns[dimension]
		if !mapped {
			return "", nil, fmt.Errorf("筛选维 %q 没有落到列上", dimension)
		}
		args = append(args, query.Filters[dimension])
		clauses = append(clauses, fmt.Sprintf("%s = ANY($%d)", column, len(args)))
	}
	if query.Keyword != "" {
		args = append(args, "%"+accountLikeLiteral.Replace(query.Keyword)+"%")
		matches := make([]string, len(customerAccountKeywordColumns))
		for index, column := range customerAccountKeywordColumns {
			matches[index] = fmt.Sprintf("%s ILIKE $%d", column, len(args))
		}
		clauses = append(clauses, "("+strings.Join(matches, " OR ")+")")
	}
	return strings.Join(clauses, " AND "), args, nil
}

// accountLikeLiteral 让 q 在 ILIKE 里只做字面包含：% 与 _ 是通配符，反斜杠是 LIKE 的缺省转义符。
var accountLikeLiteral = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func accountTypedValue(kind cataloguepage.ValueKind, value string) (any, error) {
	switch kind {
	case cataloguepage.Instant:
		return cataloguepage.ParseInstant(value)
	case cataloguepage.Integer:
		return cataloguepage.ParseInteger(value)
	default:
		return value, nil
	}
}

// customerAccountSortValue 给出一行在某个可排维上的值，按声明的 ValueKind 规整；本页末行的游标从这里取。
func customerAccountSortValue(row ports.CustomerAccountRow, field string) string {
	switch field {
	case "registeredAt":
		return cataloguepage.FormatInstant(row.RegisteredAt)
	case "effectiveFrom":
		return cataloguepage.FormatInstant(row.EffectiveFrom)
	default:
		return row.AccountID
	}
}
