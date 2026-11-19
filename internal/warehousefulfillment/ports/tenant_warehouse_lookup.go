package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// TenantWarehouseLookup 是不指名仓的册（库位类型、包材）核「本租户有没有登记过仓」要的那一点读法：在册任何一个仓的任何一笔修订
// 即算登记过，停用的仓也算——登记册只追加，登记过就不会再「一个都没有」。它与 WarehouseLookup 分开：那一个问的是一个指名的仓，
// 这一个问的是整个租户；分开则只读一个仓的用例拿不到「整租户」的读法，反之亦然。真实现是仓登记册本身。
//
// 读不动走 error，不折成 false：把一次该重试的故障答成「一个仓都没登记」，登记方就会被告知未配置、被打发去登记一个本来就在册
// 的仓（CONTEXT「主数据登记」，ADR-0375 决定五）。
type TenantWarehouseLookup interface {
	HasAnyWarehouse(ctx context.Context, tenant domain.TenantID) (bool, error)
}
