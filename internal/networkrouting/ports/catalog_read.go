package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// OperationsCatalogRead 是版本化网络目录的运营查阅上列读面(ADR-0077 Decision 一):
// 给管理台网络目录与服务区域两页供数,按族列版本行原文——含已闭区间的历史版本与
// 尚未生效的未来版本,临时调整族列整条历史链。查阅不触发判断、决定或披露,所以它
// 接存储读面,不接应用编排(分界句沿 /shipment-request-views 先例)。
//
// 它与 LoadDefinitionsAt 的选版快照是两个读法:上列不按 asOf 选版、不折叠、形不成
// 判断依据,三个证据视图与解析层的护栏(ADR-0068 Decision 六)不因本端口松动。
// 「登记过与否」的分辨器(修订行)属证据端口语义(ADR-0052);目录上列这一格,空表
// 本身就是内容(ADR-0077 Decision 四)——所以本端口没有 configured 布尔,空族如实
// 答空列表,「未配置」属接入渠道,住在 Intake 缝里。
//
// 七个方法与 0008 的七张版本表一一对应,不并进 NetworkCatalogRegistry 写口——
// 扩写侧接口会拆全部写侧测试替身,伴生读端口另立(ADR-0077 Decision 五)。
// 租户维在方法签名上(同 Decision 五,仓储派);行类型透出的是库里已有的列,不多不少——
// 服务区域的覆盖与节点角色、路由策略的排序形态已有列并随行列出,日历内容与策略其余规则
// 正文的列还不存在。
//
// Limit 必须为正;每页多大由接入面按渠道契约裁决,读口只拒绝无意义的取值。
//
// 翻页、排序与筛选照 ADR-0144 决定六:query 是端点经 cataloguepage 按本族声明
// (catalog_query.go)解出的查询对象,各方法答本页行、下一游标与同一组条件下的总数。
type OperationsCatalogRead interface {
	ListNodeVersions(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (CatalogPage[NodeDefinitionVersion], error)
	ListConnectionVersions(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (CatalogPage[ConnectionDefinitionVersion], error)
	ListLineVersions(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (CatalogPage[LineDefinitionVersion], error)
	ListServiceAreaVersions(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (CatalogPage[ServiceAreaDefinitionVersion], error)
	ListServiceCalendarVersions(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (CatalogPage[ServiceCalendarDefinitionVersion], error)
	ListAvailabilityAdjustments(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (CatalogPage[AvailabilityAdjustmentStatement], error)
	ListRouteStrategyVersions(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (CatalogPage[RouteStrategyDefinitionVersion], error)
}
