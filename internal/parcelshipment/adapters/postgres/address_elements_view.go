package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件让 ShipmentRequests 同时实现 ports.AddressElementsView（pp-seams/03 裁决 2 / 4 与裁决 7 追裁）：写侧仓储与「地址要素」
// 读口是同一只适配器，读的是同一张表的同一份快照。**不加列不加迁移**：要素落在提交版本文档的 `elements` 子段与资料版本
// 文档的 `content` 子段（pp-seams/05），五格全由聚合自己译出——早于那票的快照没有子段，读回零值、如实答「要素缺席」。
//
// 取行 + 重建那段路在 findAcceptedRequestCoveringParcel（与其余按（租户，包裹）答问的读口共用）。**它只是找到那一行的路，
// 不是成员集合的权威**：成员归属由接受基线自己回答，两段的锚由 domain 的共用内部步骤解析，五格由 AddressElementsFor 译出。
// SQL 里没有第二套派生规则。

var _ ports.AddressElementsView = (*ShipmentRequests)(nil)

// LoadAddressElements 按（租户，声明包裹身份）答寄件与收件两段地址要素的封闭五格。
//
// 零行命中两段直接答「无」——按统一不可见结果，不区分不存在、他租户与未授权；仅`已提交`的委托不在部分索引里，其成员同样
// 落这一格。多于一行是读面坏了（ADR-0060 的歧义），上抛而不挑一份。
func (repository *ShipmentRequests) LoadAddressElements(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (domain.ShipmentAddressElements, error) {
	none := domain.ShipmentAddressElements{}
	request, found, err := repository.findAcceptedRequestCoveringParcel(ctx, tenant, parcel)
	if err != nil {
		return none, fmt.Errorf("load address elements: %w", err)
	}
	if !found {
		return domain.NoShipmentAddressElements(), nil
	}
	answer, err := request.AddressElementsFor(parcel)
	if err != nil {
		return none, fmt.Errorf("load address elements: %w", err)
	}
	return answer, nil
}
