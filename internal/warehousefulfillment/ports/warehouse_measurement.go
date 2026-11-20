package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// WarehouseMeasurementRead 读已落册的仓测各版，返回按发生次序从早到晚排列的切片。
// 没有测过答空切片、无错；读不动走 error，不折成未测量。
type WarehouseMeasurementRead interface {
	ListForGoods(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		goods domain.GoodsCode,
	) ([]domain.WarehouseMeasurement, error)
	ListForGoodsBatch(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		goods []domain.GoodsCode,
	) (map[domain.GoodsCode][]domain.WarehouseMeasurement, error)
	ListForGoodsLevel(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		goods domain.GoodsCode,
		level domain.GoodsUnitName,
	) ([]domain.WarehouseMeasurement, error)
	ListForCarton(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		warehouse domain.WarehouseCode,
		carton domain.CartonNumber,
	) ([]domain.WarehouseMeasurement, error)
}

// WarehouseMeasurementStore 是仓测册的读写口。HoldFact 与 Save 在没有环境事务时拒绝。
// Save 用 ON CONFLICT DO NOTHING：唯一冲突不打坏当前事务；锁内仍出现重复事实身份时返回 inserted=false，调用方回滚。
type WarehouseMeasurementStore interface {
	WarehouseMeasurementRead
	HoldFact(ctx context.Context, tenant domain.TenantID, factID domain.MeasurementFactID) error
	LoadByFact(ctx context.Context, tenant domain.TenantID, factID domain.MeasurementFactID) (domain.WarehouseMeasurement, bool, error)
	Save(ctx context.Context, measurement domain.WarehouseMeasurement) (inserted bool, err error)
}

// CartonOnHandLookup 问一箱此刻在这个货主这个仓的整箱账上是不是在库（余额为正）。
// 仓测按箱测时在事实到达服务端那一刻核，不按设备发生时刻（UC-WF-002）。真实现是整箱账。
type CartonOnHandLookup interface {
	CartonOnHand(
		ctx context.Context,
		tenant domain.TenantID,
		owner domain.OwnerAccountID,
		warehouse domain.WarehouseCode,
		carton domain.CartonNumber,
	) (bool, error)
}
