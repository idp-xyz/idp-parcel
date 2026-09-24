package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件是 NetworkCatalog 的运营查阅上列半边（ports.OperationsCatalogRead，ADR-0077）：
// 上列读的就是这份目录库，不是第二份数据（先例：visibilityexception 的 Projections
// 同一适配器一并实现运营读面）。
//
// 与 LoadDefinitionsAt 的分工：那边按 asOf 选版、拒歧义、带修订，是判断依据的取数口；
// 这边按族列版本行**原文**——含已闭区间的历史版、尚未生效的未来版与整条调整历史链，
// 不选版不折叠，两版区间重叠在这里不是错误（上列如实透出，修目录的人正需要看见它们）。
// 修订行不参与：「登记过与否」的分辨器属证据端口语义（ADR-0052），目录上列这一格
// 空表本身就是内容（ADR-0077 Decision 四），空族如实答空列表。
//
// 翻页、排序与筛选照 ADR-0144：按「排序维值 + 行标识」取游标之后的行，决胜键是主键里租户之后的列、
// 与排序同向；缺省序与其理由写在 ports 的各族声明上。total 与本页用同一组筛选与 q 条件另数一次。
// q 在各族的这些列上做不分大小写的字面包含匹配：节点 node_code；连接 connection_code、
// from_node_code、to_node_code；线路 line_code；服务区域 area_code；服务日历 target_code；
// 调整 adjustment_code、target_code、source；路由策略 strategy_code。limit 非正是调用方编程
// 错误——静默答一页会把「忘了传」变成一个没人决定过的页大小。
var _ ports.OperationsCatalogRead = (*NetworkCatalog)(nil)

func (catalog *NetworkCatalog) ListNodeVersions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (ports.CatalogPage[ports.NodeDefinitionVersion], error) {
	return listPage(ctx, catalog, nodeFamily, tenant, limit, query)
}

func (catalog *NetworkCatalog) ListConnectionVersions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (ports.CatalogPage[ports.ConnectionDefinitionVersion], error) {
	return listPage(ctx, catalog, connectionFamily, tenant, limit, query)
}

func (catalog *NetworkCatalog) ListLineVersions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (ports.CatalogPage[ports.LineDefinitionVersion], error) {
	return listPage(ctx, catalog, lineFamily, tenant, limit, query)
}

func (catalog *NetworkCatalog) ListServiceAreaVersions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (ports.CatalogPage[ports.ServiceAreaDefinitionVersion], error) {
	return listPage(ctx, catalog, serviceAreaFamily, tenant, limit, query)
}

func (catalog *NetworkCatalog) ListServiceCalendarVersions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (ports.CatalogPage[ports.ServiceCalendarDefinitionVersion], error) {
	return listPage(ctx, catalog, serviceCalendarFamily, tenant, limit, query)
}

func (catalog *NetworkCatalog) ListAvailabilityAdjustments(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (ports.CatalogPage[ports.AvailabilityAdjustmentStatement], error) {
	return listPage(ctx, catalog, adjustmentFamily, tenant, limit, query)
}

func (catalog *NetworkCatalog) ListRouteStrategyVersions(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (ports.CatalogPage[ports.RouteStrategyDefinitionVersion], error) {
	return listPage(ctx, catalog, routeStrategyFamily, tenant, limit, query)
}

// catalogFamily 把一族在 ports 里的查询声明落到它那张版本表上：答复体字段名对到列，行标识对到主键里
// 租户之后的列（与声明的 Identity 同序同形）。
type catalogFamily[Row any] struct {
	operation string
	table     string
	selection string
	sorts     map[string]keyColumn
	identity  []keyColumn
	filters   map[string]string
	keyword   []string
	scan      func(pgx.Rows) (Row, error)
	// sortValues 与 identityOf 给出一行在各可排维上的值与它的标识，都按声明的 ValueKind 规整；
	// 本页末行的游标从这里取。
	sortValues func(Row) map[string]string
	identityOf func(Row) []string
}

type keyColumn struct {
	column string
	kind   cataloguepage.ValueKind
}

func codeAndVersion(codeColumn string) []keyColumn {
	return []keyColumn{{codeColumn, cataloguepage.Text}, {"version", cataloguepage.Integer}}
}

func codeVersion(code string, version int32) []string {
	return []string{code, cataloguepage.FormatInteger(int64(version))}
}

var byEffectiveFrom = keyColumn{"effective_from", cataloguepage.Instant}

var nodeFamily = catalogFamily[ports.NodeDefinitionVersion]{
	operation: "list node versions",
	table:     "network_routing.logistics_node_version",
	selection: "node_code, version, business_timezone, effective_from, effective_to",
	sorts: map[string]keyColumn{
		"code":          {"node_code", cataloguepage.Text},
		"effectiveFrom": byEffectiveFrom,
	},
	identity: codeAndVersion("node_code"),
	filters:  map[string]string{"code": "node_code"},
	keyword:  []string{"node_code"},
	scan: func(rows pgx.Rows) (ports.NodeDefinitionVersion, error) {
		var row ports.NodeDefinitionVersion
		var effectiveTo *time.Time
		if err := rows.Scan(&row.Code, &row.Version, &row.BusinessTimezone,
			&row.EffectiveFrom, &effectiveTo); err != nil {
			return row, err
		}
		row.EffectiveTo, row.HasEffectiveTo = timeOf(effectiveTo), effectiveTo != nil
		return row, nil
	},
	sortValues: func(row ports.NodeDefinitionVersion) map[string]string {
		return map[string]string{"code": row.Code, "effectiveFrom": cataloguepage.FormatInstant(row.EffectiveFrom)}
	},
	identityOf: func(row ports.NodeDefinitionVersion) []string { return codeVersion(row.Code, row.Version) },
}

var connectionFamily = catalogFamily[ports.ConnectionDefinitionVersion]{
	operation: "list connection versions",
	table:     "network_routing.network_connection_version",
	selection: "connection_code, version, from_node_code, to_node_code, business_timezone, effective_from, effective_to",
	sorts: map[string]keyColumn{
		"code":          {"connection_code", cataloguepage.Text},
		"fromNode":      {"from_node_code", cataloguepage.Text},
		"toNode":        {"to_node_code", cataloguepage.Text},
		"effectiveFrom": byEffectiveFrom,
	},
	identity: codeAndVersion("connection_code"),
	filters:  map[string]string{"code": "connection_code", "fromNode": "from_node_code", "toNode": "to_node_code"},
	keyword:  []string{"connection_code", "from_node_code", "to_node_code"},
	scan: func(rows pgx.Rows) (ports.ConnectionDefinitionVersion, error) {
		var row ports.ConnectionDefinitionVersion
		var effectiveTo *time.Time
		if err := rows.Scan(&row.Code, &row.Version, &row.FromNode, &row.ToNode,
			&row.BusinessTimezone, &row.EffectiveFrom, &effectiveTo); err != nil {
			return row, err
		}
		row.EffectiveTo, row.HasEffectiveTo = timeOf(effectiveTo), effectiveTo != nil
		return row, nil
	},
	sortValues: func(row ports.ConnectionDefinitionVersion) map[string]string {
		return map[string]string{
			"code": row.Code, "fromNode": row.FromNode, "toNode": row.ToNode,
			"effectiveFrom": cataloguepage.FormatInstant(row.EffectiveFrom),
		}
	},
	identityOf: func(row ports.ConnectionDefinitionVersion) []string { return codeVersion(row.Code, row.Version) },
}

var lineFamily = catalogFamily[ports.LineDefinitionVersion]{
	operation: "list line versions",
	table:     "network_routing.line_version",
	selection: "line_code, version, segments, business_timezone, applicable_scope, effective_from, effective_to",
	sorts: map[string]keyColumn{
		"code":          {"line_code", cataloguepage.Text},
		"effectiveFrom": byEffectiveFrom,
	},
	identity: codeAndVersion("line_code"),
	filters:  map[string]string{"code": "line_code"},
	keyword:  []string{"line_code"},
	scan: func(rows pgx.Rows) (ports.LineDefinitionVersion, error) {
		var row ports.LineDefinitionVersion
		var segmentsRaw []byte
		var effectiveTo *time.Time
		if err := rows.Scan(&row.Code, &row.Version, &segmentsRaw, &row.BusinessTimezone,
			&row.ApplicableScope, &row.EffectiveFrom, &effectiveTo); err != nil {
			return row, err
		}
		if err := json.Unmarshal(segmentsRaw, &row.Segments); err != nil {
			return row, fmt.Errorf("译回段链：%w", err)
		}
		row.EffectiveTo, row.HasEffectiveTo = timeOf(effectiveTo), effectiveTo != nil
		return row, nil
	},
	sortValues: func(row ports.LineDefinitionVersion) map[string]string {
		return map[string]string{"code": row.Code, "effectiveFrom": cataloguepage.FormatInstant(row.EffectiveFrom)}
	},
	identityOf: func(row ports.LineDefinitionVersion) []string { return codeVersion(row.Code, row.Version) },
}

var serviceAreaFamily = catalogFamily[ports.ServiceAreaDefinitionVersion]{
	operation: "list service area versions",
	table:     "network_routing.service_area_version",
	selection: "area_code, version, effective_from, effective_to, " +
		"coverage_country, coverage_postal_prefixes, origin_node_codes, destination_node_codes",
	sorts: map[string]keyColumn{
		"code":          {"area_code", cataloguepage.Text},
		"effectiveFrom": byEffectiveFrom,
	},
	identity: codeAndVersion("area_code"),
	filters:  map[string]string{"code": "area_code"},
	keyword:  []string{"area_code"},
	scan: func(rows pgx.Rows) (ports.ServiceAreaDefinitionVersion, error) {
		var row areaVersionRow
		var prefixes, origin, destination []byte
		if err := rows.Scan(&row.Code, &row.Version, &row.EffectiveFrom, &row.EffectiveTo,
			&row.CoverageCountry, &prefixes, &origin, &destination); err != nil {
			return ports.ServiceAreaDefinitionVersion{}, err
		}
		for _, column := range []struct {
			raw    []byte
			target *[]string
		}{{prefixes, &row.PostalPrefixes}, {origin, &row.OriginNodes}, {destination, &row.DestinationNodes}} {
			if column.raw == nil {
				continue
			}
			if err := json.Unmarshal(column.raw, column.target); err != nil {
				return ports.ServiceAreaDefinitionVersion{}, fmt.Errorf("译回服务区域覆盖：%w", err)
			}
		}
		return row.definition(), nil
	},
	sortValues: func(row ports.ServiceAreaDefinitionVersion) map[string]string {
		return map[string]string{"code": row.Code, "effectiveFrom": cataloguepage.FormatInstant(row.EffectiveFrom)}
	},
	identityOf: func(row ports.ServiceAreaDefinitionVersion) []string { return codeVersion(row.Code, row.Version) },
}

var serviceCalendarFamily = catalogFamily[ports.ServiceCalendarDefinitionVersion]{
	operation: "list service calendar versions",
	table:     "network_routing.service_calendar_version",
	selection: "target_kind, target_code, version, effective_from, effective_to",
	sorts: map[string]keyColumn{
		"targetKind":    {"target_kind", cataloguepage.Text},
		"targetCode":    {"target_code", cataloguepage.Text},
		"effectiveFrom": byEffectiveFrom,
	},
	identity: []keyColumn{
		{"target_kind", cataloguepage.Text}, {"target_code", cataloguepage.Text}, {"version", cataloguepage.Integer},
	},
	filters: map[string]string{"targetKind": "target_kind", "targetCode": "target_code"},
	keyword: []string{"target_code"},
	scan: func(rows pgx.Rows) (ports.ServiceCalendarDefinitionVersion, error) {
		var row ports.ServiceCalendarDefinitionVersion
		var kindRaw string
		var effectiveTo *time.Time
		if err := rows.Scan(&kindRaw, &row.TargetCode, &row.Version,
			&row.EffectiveFrom, &effectiveTo); err != nil {
			return row, err
		}
		kind, err := ports.CatalogTargetKindFrom(kindRaw)
		if err != nil {
			return row, err
		}
		row.TargetKind = kind
		row.EffectiveTo, row.HasEffectiveTo = timeOf(effectiveTo), effectiveTo != nil
		return row, nil
	},
	sortValues: func(row ports.ServiceCalendarDefinitionVersion) map[string]string {
		return map[string]string{
			"targetKind": row.TargetKind.String(), "targetCode": row.TargetCode,
			"effectiveFrom": cataloguepage.FormatInstant(row.EffectiveFrom),
		}
	},
	identityOf: func(row ports.ServiceCalendarDefinitionVersion) []string {
		return []string{row.TargetKind.String(), row.TargetCode, cataloguepage.FormatInteger(int64(row.Version))}
	},
}

var adjustmentFamily = catalogFamily[ports.AvailabilityAdjustmentStatement]{
	operation: "list availability adjustments",
	table:     "network_routing.availability_adjustment",
	selection: "adjustment_code, version, target_kind, target_code, adjustment_kind, source, effective_at, lifted_at",
	sorts: map[string]keyColumn{
		"code":        {"adjustment_code", cataloguepage.Text},
		"targetCode":  {"target_code", cataloguepage.Text},
		"effectiveAt": {"effective_at", cataloguepage.Instant},
	},
	identity: codeAndVersion("adjustment_code"),
	filters: map[string]string{
		"code": "adjustment_code", "targetKind": "target_kind", "targetCode": "target_code", "kind": "adjustment_kind",
	},
	keyword: []string{"adjustment_code", "target_code", "source"},
	scan: func(rows pgx.Rows) (ports.AvailabilityAdjustmentStatement, error) {
		var row ports.AvailabilityAdjustmentStatement
		var targetKindRaw, adjustmentKindRaw string
		var liftedAt *time.Time
		if err := rows.Scan(&row.Code, &row.Version, &targetKindRaw, &row.TargetCode,
			&adjustmentKindRaw, &row.Source, &row.EffectiveAt, &liftedAt); err != nil {
			return row, err
		}
		targetKind, err := ports.CatalogTargetKindFrom(targetKindRaw)
		if err != nil {
			return row, err
		}
		adjustmentKind, err := ports.AvailabilityAdjustmentKindFrom(adjustmentKindRaw)
		if err != nil {
			return row, err
		}
		row.TargetKind, row.Kind = targetKind, adjustmentKind
		row.LiftedAt, row.HasLiftedAt = timeOf(liftedAt), liftedAt != nil
		return row, nil
	},
	sortValues: func(row ports.AvailabilityAdjustmentStatement) map[string]string {
		return map[string]string{
			"code": row.Code, "targetCode": row.TargetCode,
			"effectiveAt": cataloguepage.FormatInstant(row.EffectiveAt),
		}
	},
	identityOf: func(row ports.AvailabilityAdjustmentStatement) []string { return codeVersion(row.Code, row.Version) },
}

var routeStrategyFamily = catalogFamily[ports.RouteStrategyDefinitionVersion]{
	operation: "list route strategy versions",
	table:     "network_routing.route_strategy_version",
	selection: "strategy_code, version, applicable_scope, effective_from, effective_to, ranking_form",
	sorts: map[string]keyColumn{
		"code":          {"strategy_code", cataloguepage.Text},
		"effectiveFrom": byEffectiveFrom,
	},
	identity: codeAndVersion("strategy_code"),
	filters:  map[string]string{"code": "strategy_code"},
	keyword:  []string{"strategy_code"},
	scan: func(rows pgx.Rows) (ports.RouteStrategyDefinitionVersion, error) {
		var row ports.RouteStrategyDefinitionVersion
		var effectiveTo *time.Time
		var form *string
		if err := rows.Scan(&row.Code, &row.Version, &row.ApplicableScope,
			&row.EffectiveFrom, &effectiveTo, &form); err != nil {
			return row, err
		}
		row.EffectiveTo, row.HasEffectiveTo = timeOf(effectiveTo), effectiveTo != nil
		var err error
		row.RankingForm, err = rankingFormOf(form)
		return row, err
	},
	sortValues: func(row ports.RouteStrategyDefinitionVersion) map[string]string {
		return map[string]string{"code": row.Code, "effectiveFrom": cataloguepage.FormatInstant(row.EffectiveFrom)}
	},
	identityOf: func(row ports.RouteStrategyDefinitionVersion) []string { return codeVersion(row.Code, row.Version) },
}

// listPage 是七族共用的取页：先按筛选与 q 数总数，再按「排序维值 + 标识」取游标之后的 limit+1 行，
// 多出的一行只用来判有没有下一页。
func listPage[Row any](
	ctx context.Context,
	catalog *NetworkCatalog,
	family catalogFamily[Row],
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (ports.CatalogPage[Row], error) {
	fail := func(err error) (ports.CatalogPage[Row], error) {
		return ports.CatalogPage[Row]{}, fmt.Errorf("%s: %w", family.operation, err)
	}
	querier, err := catalog.listQuerier(ctx, family.operation, limit)
	if err != nil {
		return ports.CatalogPage[Row]{}, err
	}
	primary, mapped := family.sorts[query.Sort.Field]
	if !mapped {
		return fail(fmt.Errorf("排序维 %q 没有落到列上（查询对象须由本族声明解出）", query.Sort.Field))
	}
	where, args, err := family.conditions(tenant, query)
	if err != nil {
		return fail(err)
	}

	var total int64
	if err := querier.QueryRow(ctx, "SELECT count(*) FROM "+family.table+" WHERE "+where, args...).Scan(&total); err != nil {
		return fail(err)
	}

	keys := append([]keyColumn{primary}, family.identity...)
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
			value, err := typedValue(key.kind, values[index])
			if err != nil {
				return fail(err)
			}
			args = append(args, value)
			columns[index], placeholders[index] = key.column, fmt.Sprintf("$%d", len(args))
		}
		where += fmt.Sprintf(" AND (%s) %s (%s)", strings.Join(columns, ", "), comparison, strings.Join(placeholders, ", "))
	}
	order := make([]string, len(keys))
	for index, key := range keys {
		order[index] = key.column + " " + direction
	}
	args = append(args, limit+1)
	rows, err := querier.Query(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT $%d",
		family.selection, family.table, where, strings.Join(order, ", "), len(args)), args...)
	if err != nil {
		return fail(err)
	}
	defer rows.Close()

	fetched := make([]Row, 0, limit+1)
	for rows.Next() {
		row, err := family.scan(rows)
		if err != nil {
			return fail(err)
		}
		fetched = append(fetched, row)
	}
	if err := rows.Err(); err != nil {
		return fail(err)
	}

	page, more := cataloguepage.Trim(fetched, limit)
	result := ports.CatalogPage[Row]{Rows: page, Total: total}
	if more {
		last := page[len(page)-1]
		next, err := query.CursorAfter(cataloguepage.Position{
			Value:    family.sortValues(last)[query.Sort.Field],
			Identity: family.identityOf(last),
		})
		if err != nil {
			return fail(err)
		}
		result.Next = next
	}
	return result, nil
}

// conditions 是租户、筛选维与 q 的条件，总数与取页共用同一组。筛选维内为或（= ANY）、维间为与。
func (family catalogFamily[Row]) conditions(tenant domain.TenantID, query cataloguepage.Query) (string, []any, error) {
	clauses := []string{"tenant_id = $1"}
	args := []any{tenant.String()}
	dimensions := make([]string, 0, len(query.Filters))
	for dimension := range query.Filters {
		dimensions = append(dimensions, dimension)
	}
	sort.Strings(dimensions)
	for _, dimension := range dimensions {
		column, mapped := family.filters[dimension]
		if !mapped {
			return "", nil, fmt.Errorf("筛选维 %q 没有落到列上", dimension)
		}
		args = append(args, query.Filters[dimension])
		clauses = append(clauses, fmt.Sprintf("%s = ANY($%d)", column, len(args)))
	}
	if query.Keyword != "" {
		args = append(args, "%"+likeLiteral.Replace(query.Keyword)+"%")
		matches := make([]string, len(family.keyword))
		for index, column := range family.keyword {
			matches[index] = fmt.Sprintf("%s ILIKE $%d", column, len(args))
		}
		clauses = append(clauses, "("+strings.Join(matches, " OR ")+")")
	}
	return strings.Join(clauses, " AND "), args, nil
}

// likeLiteral 让 q 在 ILIKE 里只做字面包含：% 与 _ 是通配符，反斜杠是 LIKE 的缺省转义符。
var likeLiteral = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func typedValue(kind cataloguepage.ValueKind, value string) (any, error) {
	switch kind {
	case cataloguepage.Instant:
		return cataloguepage.ParseInstant(value)
	case cataloguepage.Integer:
		return cataloguepage.ParseInteger(value)
	default:
		return value, nil
	}
}

// listQuerier 是七个上列方法共用的入口检查：limit 门禁加读执行器。
func (catalog *NetworkCatalog) listQuerier(
	ctx context.Context,
	operation string,
	limit int,
) (bentopg.Querier, error) {
	if limit < 1 {
		return nil, fmt.Errorf("%s: limit must be positive, got %d", operation, limit)
	}
	querier, err := catalog.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return querier, nil
}
