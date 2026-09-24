package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// ErrAmbiguousNetworkCatalog 说明目录在同一时点对同一身份有两个适用版本。
//
// 它是错误而不是业务答案（先例：VE 目录的 ErrAmbiguousCatalog）：按任一条挑一个都是
// 替网络定义的责任方作了它没作的决定。未闭区间那一半由部分唯一索引在库上挡住，本错
// 兜的是已闭区间的重叠——数据坏了要修目录，与`未配置`是两条续办路，不压成一格。
var ErrAmbiguousNetworkCatalog = errors.New(
	"network routing postgres: 网络目录在同一时点有多个适用版本")

// NetworkCatalog 是版本化网络目录的存取口（票 01 的机制四件）。它拥有定义原语的
// 版本行与目录修订，不做任何评估、过滤或排序——那一层属 PAR-NET-14，在它存在之前
// 三个证据视图不读本目录（护栏：三口取数侧不接，NetworkDefinitions 照旧作答）。
type NetworkCatalog struct {
	db *bentopg.DB
}

func NewNetworkCatalog(db *bentopg.DB) (*NetworkCatalog, error) {
	if db == nil {
		return nil, fmt.Errorf("network routing postgres: db is nil")
	}
	return &NetworkCatalog{db: db}, nil
}

// 登记口的端口契约用编译期钉住——接口漂移在构建时暴露（先例：parcel-pricing-register
// 对两个登记用例的钉法）。行类型与封闭枚举的定义在 ports 侧（登记用例要拿它们表达
// 受理门，而应用层不得依赖适配器），本文件只留读侧快照与七个写方法的实现。
var _ ports.NetworkCatalogRegistry = (*NetworkCatalog)(nil)

// NetworkCatalogSnapshot 是一次取回：七类定义在 asOf 的适用行与它们共同来自的修订。
// 事实与修订同版由单条语句担保（单语句单快照），不靠调用方两次取回再对号。
type NetworkCatalogSnapshot struct {
	Nodes        []ports.NodeDefinitionVersion
	Connections  []ports.ConnectionDefinitionVersion
	Lines        []ports.LineDefinitionVersion
	ServiceAreas []ports.ServiceAreaDefinitionVersion
	Calendars    []ports.ServiceCalendarDefinitionVersion
	Adjustments  []ports.AvailabilityAdjustmentStatement
	Strategies   []ports.RouteStrategyDefinitionVersion
	Revision     domain.NetworkViewRevision
}

// LoadDefinitionsAt 按判断时点取回目录。三格与证据视图同形（ADR-0052）：快照在场即
// 定义；第二格 false 即**这个租户从未登记过任何网络定义**（未配置）；error 只表示
// 依赖故障或目录数据坏了（如两版同时适用）。登记过与否由修订行分辨，不由「查出零行」
// 推断——登记过而 asOf 无适用版本时答 configured=true 带空族，那是业务事实不是空册。
//
// 整份快照与修订经**单条语句**取回：单语句单快照，事实与修订必然同版（票 01 件④，
// create_initial_route 的提交前重校靠这一条比对换代）。稳定六族按 [effective_from,
// effective_to) 含 asOf 选版——「新版本自明确生效时间起参与新判断」，生效当刻属新版；
// 临时调整族先取每条调整历史链的当前陈述（最大版本），再按其生效窗口判 asOf，机制与
// 稳定族不同，正是「稳定网络定义和临时网络可用性调整必须分离」的选版半边。
func (catalog *NetworkCatalog) LoadDefinitionsAt(
	ctx context.Context,
	tenant domain.TenantID,
	asOf time.Time,
) (NetworkCatalogSnapshot, bool, error) {
	none := NetworkCatalogSnapshot{}
	if tenant.String() == "" {
		return none, false, fmt.Errorf("load network catalog: tenant is required")
	}
	querier, err := catalog.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load network catalog: %w", err)
	}

	var revision *int64
	var nodesRaw, connectionsRaw, linesRaw, areasRaw, calendarsRaw, adjustmentsRaw, strategiesRaw []byte
	err = querier.QueryRow(ctx, `
SELECT
    (SELECT revision
       FROM network_routing.network_catalog_revision
      WHERE tenant_id = $1),
    (SELECT coalesce(jsonb_agg(jsonb_build_object(
            'code', node_code, 'version', version, 'timezone', business_timezone,
            'effective_from', effective_from, 'effective_to', effective_to
        ) ORDER BY node_code, version), '[]'::jsonb)
       FROM network_routing.logistics_node_version
      WHERE tenant_id = $1 AND effective_from <= $2
        AND (effective_to IS NULL OR effective_to > $2)),
    (SELECT coalesce(jsonb_agg(jsonb_build_object(
            'code', connection_code, 'version', version, 'from_node', from_node_code,
            'to_node', to_node_code, 'timezone', business_timezone,
            'effective_from', effective_from, 'effective_to', effective_to
        ) ORDER BY connection_code, version), '[]'::jsonb)
       FROM network_routing.network_connection_version
      WHERE tenant_id = $1 AND effective_from <= $2
        AND (effective_to IS NULL OR effective_to > $2)),
    (SELECT coalesce(jsonb_agg(jsonb_build_object(
            'code', line_code, 'version', version, 'segments', segments,
            'timezone', business_timezone, 'scope', applicable_scope,
            'effective_from', effective_from, 'effective_to', effective_to
        ) ORDER BY line_code, version), '[]'::jsonb)
       FROM network_routing.line_version
      WHERE tenant_id = $1 AND effective_from <= $2
        AND (effective_to IS NULL OR effective_to > $2)),
    (SELECT coalesce(jsonb_agg(jsonb_build_object(
            'code', area_code, 'version', version,
            'effective_from', effective_from, 'effective_to', effective_to
        ) ORDER BY area_code, version), '[]'::jsonb)
       FROM network_routing.service_area_version
      WHERE tenant_id = $1 AND effective_from <= $2
        AND (effective_to IS NULL OR effective_to > $2)),
    (SELECT coalesce(jsonb_agg(jsonb_build_object(
            'target_kind', target_kind, 'target_code', target_code, 'version', version,
            'effective_from', effective_from, 'effective_to', effective_to
        ) ORDER BY target_kind, target_code, version), '[]'::jsonb)
       FROM network_routing.service_calendar_version
      WHERE tenant_id = $1 AND effective_from <= $2
        AND (effective_to IS NULL OR effective_to > $2)),
    (SELECT coalesce(jsonb_agg(jsonb_build_object(
            'code', current.adjustment_code, 'version', current.version,
            'target_kind', current.target_kind, 'target_code', current.target_code,
            'kind', current.adjustment_kind, 'source', current.source,
            'effective_at', current.effective_at, 'lifted_at', current.lifted_at
        ) ORDER BY current.adjustment_code), '[]'::jsonb)
       FROM (SELECT DISTINCT ON (adjustment_code)
                    adjustment_code, version, target_kind, target_code,
                    adjustment_kind, source, effective_at, lifted_at
               FROM network_routing.availability_adjustment
              WHERE tenant_id = $1
              ORDER BY adjustment_code, version DESC) AS current
      WHERE current.effective_at <= $2
        AND (current.lifted_at IS NULL OR current.lifted_at > $2)),
    (SELECT coalesce(jsonb_agg(jsonb_build_object(
            'code', strategy_code, 'version', version, 'scope', applicable_scope,
            'effective_from', effective_from, 'effective_to', effective_to,
            'ranking_form', ranking_form
        ) ORDER BY strategy_code, version), '[]'::jsonb)
       FROM network_routing.route_strategy_version
      WHERE tenant_id = $1 AND effective_from <= $2
        AND (effective_to IS NULL OR effective_to > $2))`,
		tenant.String(), asOf,
	).Scan(&revision, &nodesRaw, &connectionsRaw, &linesRaw, &areasRaw,
		&calendarsRaw, &adjustmentsRaw, &strategiesRaw)
	if err != nil {
		return none, false, fmt.Errorf("load network catalog: %w", err)
	}
	if revision == nil {
		return none, false, nil
	}

	snapshot, err := rebuildCatalogSnapshot(*revision, nodesRaw, connectionsRaw,
		linesRaw, areasRaw, calendarsRaw, adjustmentsRaw, strategiesRaw)
	if err != nil {
		return none, false, fmt.Errorf("load network catalog: %w", err)
	}
	return snapshot, true, nil
}

// rebuildCatalogSnapshot 把 jsonb 聚合译回类型化快照。
func rebuildCatalogSnapshot(
	revision int64,
	nodesRaw, connectionsRaw, linesRaw, areasRaw, calendarsRaw, adjustmentsRaw, strategiesRaw []byte,
) (NetworkCatalogSnapshot, error) {
	none := NetworkCatalogSnapshot{}

	viewRevision, err := domain.NewNetworkViewRevision(strconv.FormatInt(revision, 10))
	if err != nil {
		return none, err
	}
	snapshot := NetworkCatalogSnapshot{Revision: viewRevision}

	var nodes []nodeVersionRow
	if err := json.Unmarshal(nodesRaw, &nodes); err != nil {
		return none, fmt.Errorf("译回节点族：%w", err)
	}
	if err := refuseAmbiguity("节点", nodes, func(row nodeVersionRow) string {
		return row.Code
	}); err != nil {
		return none, err
	}
	for _, row := range nodes {
		snapshot.Nodes = append(snapshot.Nodes, ports.NodeDefinitionVersion{
			Code: row.Code, Version: row.Version, BusinessTimezone: row.Timezone,
			EffectiveFrom: row.EffectiveFrom, EffectiveTo: timeOf(row.EffectiveTo),
			HasEffectiveTo: row.EffectiveTo != nil,
		})
	}

	var connections []connectionVersionRow
	if err := json.Unmarshal(connectionsRaw, &connections); err != nil {
		return none, fmt.Errorf("译回连接族：%w", err)
	}
	if err := refuseAmbiguity("网络连接", connections, func(row connectionVersionRow) string {
		return row.Code
	}); err != nil {
		return none, err
	}
	for _, row := range connections {
		snapshot.Connections = append(snapshot.Connections, ports.ConnectionDefinitionVersion{
			Code: row.Code, Version: row.Version, FromNode: row.FromNode, ToNode: row.ToNode,
			BusinessTimezone: row.Timezone,
			EffectiveFrom:    row.EffectiveFrom, EffectiveTo: timeOf(row.EffectiveTo),
			HasEffectiveTo: row.EffectiveTo != nil,
		})
	}

	var lines []lineVersionRow
	if err := json.Unmarshal(linesRaw, &lines); err != nil {
		return none, fmt.Errorf("译回线路族：%w", err)
	}
	if err := refuseAmbiguity("线路", lines, func(row lineVersionRow) string {
		return row.Code
	}); err != nil {
		return none, err
	}
	for _, row := range lines {
		snapshot.Lines = append(snapshot.Lines, ports.LineDefinitionVersion{
			Code: row.Code, Version: row.Version, Segments: row.Segments,
			BusinessTimezone: row.Timezone, ApplicableScope: row.Scope,
			EffectiveFrom: row.EffectiveFrom, EffectiveTo: timeOf(row.EffectiveTo),
			HasEffectiveTo: row.EffectiveTo != nil,
		})
	}

	var areas []areaVersionRow
	if err := json.Unmarshal(areasRaw, &areas); err != nil {
		return none, fmt.Errorf("译回服务区域族：%w", err)
	}
	if err := refuseAmbiguity("服务区域", areas, func(row areaVersionRow) string {
		return row.Code
	}); err != nil {
		return none, err
	}
	for _, row := range areas {
		snapshot.ServiceAreas = append(snapshot.ServiceAreas, ports.ServiceAreaDefinitionVersion{
			Code: row.Code, Version: row.Version,
			EffectiveFrom: row.EffectiveFrom, EffectiveTo: timeOf(row.EffectiveTo),
			HasEffectiveTo: row.EffectiveTo != nil,
		})
	}

	var calendars []calendarVersionRow
	if err := json.Unmarshal(calendarsRaw, &calendars); err != nil {
		return none, fmt.Errorf("译回服务日历族：%w", err)
	}
	if err := refuseAmbiguity("服务日历", calendars, func(row calendarVersionRow) string {
		return row.TargetKind + "/" + row.TargetCode
	}); err != nil {
		return none, err
	}
	for _, row := range calendars {
		kind, err := ports.CatalogTargetKindFrom(row.TargetKind)
		if err != nil {
			return none, err
		}
		snapshot.Calendars = append(snapshot.Calendars, ports.ServiceCalendarDefinitionVersion{
			TargetKind: kind, TargetCode: row.TargetCode, Version: row.Version,
			EffectiveFrom: row.EffectiveFrom, EffectiveTo: timeOf(row.EffectiveTo),
			HasEffectiveTo: row.EffectiveTo != nil,
		})
	}

	var adjustments []adjustmentRow
	if err := json.Unmarshal(adjustmentsRaw, &adjustments); err != nil {
		return none, fmt.Errorf("译回可用性调整族：%w", err)
	}
	for _, row := range adjustments {
		kind, err := ports.CatalogTargetKindFrom(row.TargetKind)
		if err != nil {
			return none, err
		}
		adjustmentKind, err := ports.AvailabilityAdjustmentKindFrom(row.Kind)
		if err != nil {
			return none, err
		}
		snapshot.Adjustments = append(snapshot.Adjustments, ports.AvailabilityAdjustmentStatement{
			Code: row.Code, Version: row.Version, TargetKind: kind, TargetCode: row.TargetCode,
			Kind: adjustmentKind, Source: row.Source,
			EffectiveAt: row.EffectiveAt, LiftedAt: timeOf(row.LiftedAt),
			HasLiftedAt: row.LiftedAt != nil,
		})
	}

	var strategies []strategyVersionRow
	if err := json.Unmarshal(strategiesRaw, &strategies); err != nil {
		return none, fmt.Errorf("译回路由策略族：%w", err)
	}
	if err := refuseAmbiguity("路由策略", strategies, func(row strategyVersionRow) string {
		return row.Code
	}); err != nil {
		return none, err
	}
	for _, row := range strategies {
		form, err := rankingFormOf(row.RankingForm)
		if err != nil {
			return none, fmt.Errorf("译回路由策略族：%w", err)
		}
		snapshot.Strategies = append(snapshot.Strategies, ports.RouteStrategyDefinitionVersion{
			Code: row.Code, Version: row.Version, ApplicableScope: row.Scope, RankingForm: form,
			EffectiveFrom: row.EffectiveFrom, EffectiveTo: timeOf(row.EffectiveTo),
			HasEffectiveTo: row.EffectiveTo != nil,
		})
	}

	return snapshot, nil
}

type nodeVersionRow struct {
	Code          string     `json:"code"`
	Version       int32      `json:"version"`
	Timezone      string     `json:"timezone"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to"`
}

type connectionVersionRow struct {
	Code          string     `json:"code"`
	Version       int32      `json:"version"`
	FromNode      string     `json:"from_node"`
	ToNode        string     `json:"to_node"`
	Timezone      string     `json:"timezone"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to"`
}

type lineVersionRow struct {
	Code          string     `json:"code"`
	Version       int32      `json:"version"`
	Segments      []string   `json:"segments"`
	Timezone      string     `json:"timezone"`
	Scope         string     `json:"scope"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to"`
}

type areaVersionRow struct {
	Code          string     `json:"code"`
	Version       int32      `json:"version"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to"`
}

type calendarVersionRow struct {
	TargetKind    string     `json:"target_kind"`
	TargetCode    string     `json:"target_code"`
	Version       int32      `json:"version"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to"`
}

type adjustmentRow struct {
	Code        string     `json:"code"`
	Version     int32      `json:"version"`
	TargetKind  string     `json:"target_kind"`
	TargetCode  string     `json:"target_code"`
	Kind        string     `json:"kind"`
	Source      string     `json:"source"`
	EffectiveAt time.Time  `json:"effective_at"`
	LiftedAt    *time.Time `json:"lifted_at"`
}

type strategyVersionRow struct {
	Code          string     `json:"code"`
	Version       int32      `json:"version"`
	Scope         string     `json:"scope"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to"`
	RankingForm   *string    `json:"ranking_form"`
}

// rankingFormColumn 把排序形态写成列值：未声明落 NULL；族外的值在触库前拒——CHECK 也会拒，
// 但撞 CHECK 会把整个环境事务打进中止态，登记方得到的只是一段约束名。
func rankingFormColumn(form domain.RankingForm) (*string, error) {
	if form == domain.RankingFormUndeclared {
		return nil, nil
	}
	name := form.String()
	if name == "" {
		return nil, fmt.Errorf("%w: %d", domain.ErrUnknownRankingForm, form)
	}
	return &name, nil
}

// rankingFormOf 译回列值。族外的词报错而不吸收成「未声明」：那是 CHECK 被后续迁移放宽而 Go 侧
// 没跟上，照「未声明」读会让一版声明过形态的策略在排序时答未配置。
func rankingFormOf(raw *string) (domain.RankingForm, error) {
	if raw == nil {
		return domain.RankingFormUndeclared, nil
	}
	return domain.RankingFormFrom(*raw)
}

// refuseAmbiguity 对一族适用行按身份查重（件②：两版同时适用交回错误不挑一个）。
// 行已按身份排序（SQL ORDER BY），相邻比较即可。调整族不经此检查——其历史链按最大
// 版本取当前陈述，同一身份恒一行。
func refuseAmbiguity[Row any](family string, rows []Row, identityOf func(Row) string) error {
	for index := 1; index < len(rows); index++ {
		identity := identityOf(rows[index])
		if identityOf(rows[index-1]) == identity {
			return fmt.Errorf("%w：%s %q", ErrAmbiguousNetworkCatalog, family, identity)
		}
	}
	return nil
}

func timeOf(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

// RegisterNodeVersion 追加一个物流节点版本。「永久变化形成新版本，不覆盖原版本」——
// 只 INSERT 不 UPDATE 内容；登记未闭区间的新版本时，同一身份此前的未闭版本按新版本的
// 生效时间**接续闭合**（补上 effective_to，不动其余任何列）：那不是改写历史，是「新
// 版本自明确生效时间起参与新判断」在旧版本区间上的那一半。登记已闭区间（补历史）不
// 触发接续。与所有目录写入一样，同一事务里目录修订 +1。
func (catalog *NetworkCatalog) RegisterNodeVersion(
	ctx context.Context,
	tenant domain.TenantID,
	row ports.NodeDefinitionVersion,
) error {
	executor, err := catalog.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("register node version: %w", err)
	}
	if !row.HasEffectiveTo {
		if _, err := executor.Exec(ctx,
			`UPDATE network_routing.logistics_node_version
			    SET effective_to = $3
			  WHERE tenant_id = $1 AND node_code = $2
			    AND effective_to IS NULL AND effective_from < $3`,
			tenant.String(), row.Code, row.EffectiveFrom.UTC(),
		); err != nil {
			return fmt.Errorf("register node version: 接续闭合前版本：%w", err)
		}
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO network_routing.logistics_node_version
			(tenant_id, node_code, version, business_timezone, effective_from, effective_to)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		tenant.String(), row.Code, row.Version, row.BusinessTimezone,
		row.EffectiveFrom.UTC(), optionalTime(row.EffectiveTo, row.HasEffectiveTo),
	); err != nil {
		return fmt.Errorf("register node version: %w", err)
	}
	return catalog.bumpRevision(ctx, executor, tenant)
}

// RegisterConnectionVersion 追加一条有向网络连接版本。版本纪律同 RegisterNodeVersion：
// 只增不改，登记未闭新版时前版按新版生效时间接续闭合，同一事务里目录修订 +1。
func (catalog *NetworkCatalog) RegisterConnectionVersion(
	ctx context.Context,
	tenant domain.TenantID,
	row ports.ConnectionDefinitionVersion,
) error {
	executor, err := catalog.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("register connection version: %w", err)
	}
	if !row.HasEffectiveTo {
		if _, err := executor.Exec(ctx,
			`UPDATE network_routing.network_connection_version
			    SET effective_to = $3
			  WHERE tenant_id = $1 AND connection_code = $2
			    AND effective_to IS NULL AND effective_from < $3`,
			tenant.String(), row.Code, row.EffectiveFrom.UTC(),
		); err != nil {
			return fmt.Errorf("register connection version: 接续闭合前版本：%w", err)
		}
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO network_routing.network_connection_version
			(tenant_id, connection_code, version, from_node_code, to_node_code,
			 business_timezone, effective_from, effective_to)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		tenant.String(), row.Code, row.Version, row.FromNode, row.ToNode,
		row.BusinessTimezone, row.EffectiveFrom.UTC(),
		optionalTime(row.EffectiveTo, row.HasEffectiveTo),
	); err != nil {
		return fmt.Errorf("register connection version: %w", err)
	}
	return catalog.bumpRevision(ctx, executor, tenant)
}

// RegisterLineVersion 追加一条线路版本。Segments 是连接身份的有序数组，序即语义
// （「线路由一个或多个网络连接按明确顺序组成」），原样落 jsonb；至少一段由库上
// CHECK 守。版本纪律同 RegisterNodeVersion。
func (catalog *NetworkCatalog) RegisterLineVersion(
	ctx context.Context,
	tenant domain.TenantID,
	row ports.LineDefinitionVersion,
) error {
	executor, err := catalog.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("register line version: %w", err)
	}
	segments, err := json.Marshal(row.Segments)
	if err != nil {
		return fmt.Errorf("register line version: 编码段链：%w", err)
	}
	if !row.HasEffectiveTo {
		if _, err := executor.Exec(ctx,
			`UPDATE network_routing.line_version
			    SET effective_to = $3
			  WHERE tenant_id = $1 AND line_code = $2
			    AND effective_to IS NULL AND effective_from < $3`,
			tenant.String(), row.Code, row.EffectiveFrom.UTC(),
		); err != nil {
			return fmt.Errorf("register line version: 接续闭合前版本：%w", err)
		}
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO network_routing.line_version
			(tenant_id, line_code, version, segments, business_timezone,
			 applicable_scope, effective_from, effective_to)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		tenant.String(), row.Code, row.Version, segments,
		row.BusinessTimezone, row.ApplicableScope, row.EffectiveFrom.UTC(),
		optionalTime(row.EffectiveTo, row.HasEffectiveTo),
	); err != nil {
		return fmt.Errorf("register line version: %w", err)
	}
	return catalog.bumpRevision(ctx, executor, tenant)
}

// RegisterServiceAreaVersion 追加一个服务区域版本。覆盖内容列未定（开放集，等
// PAR-NET-14 的形态），本方法只登版本与有效区间。版本纪律同 RegisterNodeVersion。
func (catalog *NetworkCatalog) RegisterServiceAreaVersion(
	ctx context.Context,
	tenant domain.TenantID,
	row ports.ServiceAreaDefinitionVersion,
) error {
	executor, err := catalog.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("register service area version: %w", err)
	}
	if !row.HasEffectiveTo {
		if _, err := executor.Exec(ctx,
			`UPDATE network_routing.service_area_version
			    SET effective_to = $3
			  WHERE tenant_id = $1 AND area_code = $2
			    AND effective_to IS NULL AND effective_from < $3`,
			tenant.String(), row.Code, row.EffectiveFrom.UTC(),
		); err != nil {
			return fmt.Errorf("register service area version: 接续闭合前版本：%w", err)
		}
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO network_routing.service_area_version
			(tenant_id, area_code, version, effective_from, effective_to)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenant.String(), row.Code, row.Version, row.EffectiveFrom.UTC(),
		optionalTime(row.EffectiveTo, row.HasEffectiveTo),
	); err != nil {
		return fmt.Errorf("register service area version: %w", err)
	}
	return catalog.bumpRevision(ctx, executor, tenant)
}

// RegisterServiceCalendarVersion 追加一个服务日历版本。身份是（适用对象类别+对象
// 身份）；业务时区不设列——日历的时区就是其适用对象的时区（「业务时区按各自节点/
// 线路解释」），营业日/节假日/服务窗口/截单内容列等 PAR-NET-14 定形后再扩。版本
// 纪律同 RegisterNodeVersion。
func (catalog *NetworkCatalog) RegisterServiceCalendarVersion(
	ctx context.Context,
	tenant domain.TenantID,
	row ports.ServiceCalendarDefinitionVersion,
) error {
	executor, err := catalog.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("register service calendar version: %w", err)
	}
	kind := row.TargetKind.String()
	if kind == "" {
		return fmt.Errorf("register service calendar version: 未知适用对象类别")
	}
	if !row.HasEffectiveTo {
		if _, err := executor.Exec(ctx,
			`UPDATE network_routing.service_calendar_version
			    SET effective_to = $4
			  WHERE tenant_id = $1 AND target_kind = $2 AND target_code = $3
			    AND effective_to IS NULL AND effective_from < $4`,
			tenant.String(), kind, row.TargetCode, row.EffectiveFrom.UTC(),
		); err != nil {
			return fmt.Errorf("register service calendar version: 接续闭合前版本：%w", err)
		}
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO network_routing.service_calendar_version
			(tenant_id, target_kind, target_code, version, effective_from, effective_to)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		tenant.String(), kind, row.TargetCode, row.Version, row.EffectiveFrom.UTC(),
		optionalTime(row.EffectiveTo, row.HasEffectiveTo),
	); err != nil {
		return fmt.Errorf("register service calendar version: %w", err)
	}
	return catalog.bumpRevision(ctx, executor, tenant)
}

// RegisterRouteStrategyVersion 追加一个路由策略版本：版本、范围、有效区间与这一版声明的排序
// 形态。版本纪律同 RegisterNodeVersion。
func (catalog *NetworkCatalog) RegisterRouteStrategyVersion(
	ctx context.Context,
	tenant domain.TenantID,
	row ports.RouteStrategyDefinitionVersion,
) error {
	executor, err := catalog.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("register route strategy version: %w", err)
	}
	form, err := rankingFormColumn(row.RankingForm)
	if err != nil {
		return fmt.Errorf("register route strategy version: %w", err)
	}
	if !row.HasEffectiveTo {
		if _, err := executor.Exec(ctx,
			`UPDATE network_routing.route_strategy_version
			    SET effective_to = $3
			  WHERE tenant_id = $1 AND strategy_code = $2
			    AND effective_to IS NULL AND effective_from < $3`,
			tenant.String(), row.Code, row.EffectiveFrom.UTC(),
		); err != nil {
			return fmt.Errorf("register route strategy version: 接续闭合前版本：%w", err)
		}
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO network_routing.route_strategy_version
			(tenant_id, strategy_code, version, applicable_scope, effective_from, effective_to, ranking_form)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		tenant.String(), row.Code, row.Version, row.ApplicableScope,
		row.EffectiveFrom.UTC(), optionalTime(row.EffectiveTo, row.HasEffectiveTo), form,
	); err != nil {
		return fmt.Errorf("register route strategy version: %w", err)
	}
	return catalog.bumpRevision(ctx, executor, tenant)
}

// RegisterAvailabilityAdjustment 追加一条临时可用性调整陈述。「调整的形成、变化和
// 解除历史必须保留」——同一调整身份的形成→变化→解除各成一行新版本，永不改写旧行，
// 也**不做接续闭合**：当前陈述由历史链最大版本认定，再按其生效窗口判 asOf。这与
// 稳定六族的区间选版是两套机制（「稳定网络定义和临时网络可用性调整必须分离」）。
// 与所有目录写入一样，同一事务里目录修订 +1。
func (catalog *NetworkCatalog) RegisterAvailabilityAdjustment(
	ctx context.Context,
	tenant domain.TenantID,
	row ports.AvailabilityAdjustmentStatement,
) error {
	executor, err := catalog.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("register availability adjustment: %w", err)
	}
	targetKind := row.TargetKind.String()
	if targetKind == "" {
		return fmt.Errorf("register availability adjustment: 未知适用对象类别")
	}
	adjustmentKind := row.Kind.String()
	if adjustmentKind == "" {
		return fmt.Errorf("register availability adjustment: 未知调整种类")
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO network_routing.availability_adjustment
			(tenant_id, adjustment_code, version, target_kind, target_code,
			 adjustment_kind, source, effective_at, lifted_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		tenant.String(), row.Code, row.Version, targetKind, row.TargetCode,
		adjustmentKind, row.Source, row.EffectiveAt.UTC(),
		optionalTime(row.LiftedAt, row.HasLiftedAt),
	); err != nil {
		return fmt.Errorf("register availability adjustment: %w", err)
	}
	return catalog.bumpRevision(ctx, executor, tenant)
}

// bumpRevision 在写入所在事务里把目录修订 +1（首笔置 1）。修订由登记册派生、随写入
// 同事务推进（ADR-0052），任何目录写入方法都必须以它收尾。
func (catalog *NetworkCatalog) bumpRevision(
	ctx context.Context,
	executor bentopg.Executor,
	tenant domain.TenantID,
) error {
	if _, err := executor.Exec(ctx,
		`INSERT INTO network_routing.network_catalog_revision (tenant_id, revision)
		 VALUES ($1, 1)
		 ON CONFLICT (tenant_id) DO UPDATE
		    SET revision = network_catalog_revision.revision + 1`,
		tenant.String(),
	); err != nil {
		return fmt.Errorf("推进目录修订：%w", err)
	}
	return nil
}

func optionalTime(value time.Time, present bool) *time.Time {
	if !present {
		return nil
	}
	utc := value.UTC()
	return &utc
}
