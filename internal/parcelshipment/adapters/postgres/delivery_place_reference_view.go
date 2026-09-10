package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件让 ShipmentRequests 同时实现 ports.DeliveryPlaceReferenceView（票 ps-port-remainder/06；ADR-0130 决定二）：
// 写侧仓储与「收件地点引用」读口是同一只适配器，读的是同一张表的同一份快照（判据同 ChannelSelectionDecisions
// 兼 ChannelSelectionDecisionRead）。
//
// 取行 + 重建那段路在 findAcceptedRequestCoveringParcel（与 CommercialResolutionReferenceView 共用）：一条按（租户，
// 包裹）的查询走 `declared_parcel_ids` 上的部分 GIN 反查已接受委托，命中的行整份读回过重建门。**它只是找到那一行
// 的路，不是成员集合的权威**：成员归属由聚合上的接受基线自己回答（AcceptanceBaseline.covers），当前采用判断由
// CurrentSourceDataAdoption 派生，四格由 domain 的 DeliveryPlaceReferenceFor 译出。SQL 里没有第二套派生规则，也不
// 解释四格里的任何一格。不加迁移：既有索引够用，没有新列。

var _ ports.DeliveryPlaceReferenceView = (*ShipmentRequests)(nil)

// LoadDeliveryPlaceReference 按（租户，声明包裹身份）答收件地点引用的封闭四格。
//
// 零行命中直接答「没有收件地点」——按统一不可见结果，不区分不存在、他租户与未授权；仅`已提交`的委托不在部分索引
// 里，其成员同样落这一格：责任起点在接受之后。多于一行是读面坏了（ADR-0060 的歧义），上抛而不挑一份——四格里没有
// 一格是「不知道」，也没有一格容得下「两份里的某一份」。
func (repository *ShipmentRequests) LoadDeliveryPlaceReference(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (domain.DeliveryPlaceResolution, error) {
	none := domain.DeliveryPlaceResolution{}
	request, found, err := repository.findAcceptedRequestCoveringParcel(ctx, tenant, parcel)
	if err != nil {
		return none, fmt.Errorf("load delivery place reference: %w", err)
	}
	if !found {
		return domain.NoDeliveryPlaceResolution(), nil
	}
	resolution, err := request.DeliveryPlaceReferenceFor(parcel)
	if err != nil {
		return none, fmt.Errorf("load delivery place reference: %w", err)
	}
	return resolution, nil
}
