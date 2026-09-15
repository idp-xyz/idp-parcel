package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件让 ShipmentRequests 同时实现 ports.DeclaredMeasurementView（pp-seams/02 裁决 5）：写侧仓储与「申报测量」读口是
// 同一只适配器，读的是同一张表的同一份快照——画像随委托 `snapshot` 列的 versionDocument.Profiles 落库、读回过重建门，
// 本读口从重建后的聚合上取，**不加列不加迁移**。
//
// 取行 + 重建那段路在 findAcceptedRequestCoveringParcel（与 DeliveryPlaceReferenceView / CommercialResolutionReferenceView
// 共用）。**它只是找到那一行的路，不是成员集合的权威**：成员归属由聚合上的接受基线自己回答，锚由 domain 的共用内部步骤
// 解析，五格由 DeclaredMeasurementFor 译出。SQL 里没有第二套派生规则，也不解释五格里的任何一格。

var _ ports.DeclaredMeasurementView = (*ShipmentRequests)(nil)

// LoadDeclaredMeasurement 按（租户，声明包裹身份）答申报测量的封闭五格。
//
// 零行命中直接答「无」——按统一不可见结果，不区分不存在、他租户与未授权；仅`已提交`的委托不在部分索引里，其成员同样
// 落这一格：责任起点在接受之后。多于一行是读面坏了（ADR-0060 的歧义），上抛而不挑一份。
func (repository *ShipmentRequests) LoadDeclaredMeasurement(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (domain.DeclaredMeasurementResolution, error) {
	none := domain.DeclaredMeasurementResolution{}
	request, found, err := repository.findAcceptedRequestCoveringParcel(ctx, tenant, parcel)
	if err != nil {
		return none, fmt.Errorf("load declared measurement: %w", err)
	}
	if !found {
		return domain.NoDeclaredMeasurementResolution(), nil
	}
	resolution, err := request.DeclaredMeasurementFor(parcel)
	if err != nil {
		return none, fmt.Errorf("load declared measurement: %w", err)
	}
	return resolution, nil
}
