package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件让 ShipmentRequests 同时实现 ports.AcceptedServiceProductView（ADR-0378 决定三）：与其余按（租户，包裹）答问的读口共用
// findAcceptedRequestCoveringParcel 那段取行 + 重建的路，答问在读回的聚合上做（AcceptedServiceProductFor）。申报的产品随提交版本
// 落在快照里，不加列。

var _ ports.AcceptedServiceProductView = (*ShipmentRequests)(nil)

// LoadAcceptedServiceProduct 按（租户，声明包裹身份）答覆盖它的已接受委托申报的服务产品。零行答没有产品；多于一行是歧义 error。
func (repository *ShipmentRequests) LoadAcceptedServiceProduct(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (domain.DeclaredServiceProduct, error) {
	request, found, err := repository.findAcceptedRequestCoveringParcel(ctx, tenant, parcel)
	if err != nil {
		return domain.DeclaredServiceProduct{}, fmt.Errorf("load accepted service product: %w", err)
	}
	if !found {
		return domain.DeclaredServiceProduct{}, nil
	}
	return request.AcceptedServiceProductFor(parcel), nil
}
