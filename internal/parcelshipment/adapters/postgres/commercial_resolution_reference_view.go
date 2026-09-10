package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件让 ShipmentRequests 同时实现 ports.CommercialResolutionReferenceView（票 ps-port-remainder/07；ADR-0133
// 决定二）：与 DeliveryPlaceReferenceView 共用 findAcceptedRequestCoveringParcel 那段取行 + 重建的路，答问各自在读回的
// 聚合上做——回指由 domain 的 CommercialResolutionReferenceFor 从接受决定上取，SQL 里不解释三格里的任何一格。
//
// 回指列今天随接受决定快照落库（snapshot.decision.basis.resolutionId），CommercialResolutionKeyStore 与接受决定的
// postgres 形状不改；不加迁移、不加列。

var _ ports.CommercialResolutionReferenceView = (*ShipmentRequests)(nil)

// LoadCommercialResolutionReference 按（租户，声明包裹身份）答委托接受时固定的商业解析回指。
//
// 零行答「没有」（统一不可见结果）；恰一行由聚合答——`已接受`而决定缺回指是 error 不是「没有」；多于一行是歧义 error。
func (repository *ShipmentRequests) LoadCommercialResolutionReference(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (domain.CommercialResolutionID, bool, error) {
	none := domain.CommercialResolutionID{}
	request, found, err := repository.findAcceptedRequestCoveringParcel(ctx, tenant, parcel)
	if err != nil {
		return none, false, fmt.Errorf("load commercial resolution reference: %w", err)
	}
	if !found {
		return none, false, nil
	}
	reference, present, err := request.CommercialResolutionReferenceFor(parcel)
	if err != nil {
		return none, false, fmt.Errorf("load commercial resolution reference: %w", err)
	}
	return reference, present, nil
}
