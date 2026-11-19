package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// WarehouseLookup 是别的册核「指名的仓已登记」要的那一点读法：在册任何一笔修订即已登记，停用的仓也算——登记册只追加，
// 一个仓登记过就不会再「不在册」。它只读不写，所以拿它的用例碰不到仓的写入面。真实现是仓登记册本身
// （WarehouseRegistry 的这一个读方法）。
//
// 读不动走 error，不折成 found=false：把一次该重试的故障答成「查无所指」，登记方就会被打发去登记一个本来就在册的仓。
type WarehouseLookup interface {
	LoadLatestWarehouse(
		ctx context.Context,
		tenant domain.TenantID,
		code domain.WarehouseCode,
	) (domain.WarehouseRevision, bool, error)
}

// ZoneRegistry 是库区登记册的写入面：键 = 租户 + 仓编码 + 库区编码 + 修订号，修订只追加、不覆盖。
//
// 读不动走 error，不折成 found=false，理由同 WarehouseRegistry。登记用例在同一个环境事务里调用两个读方法与保存；保存在没有
// 环境事务时拒绝。库区编码在仓内才唯一：按（租户，仓编码，库区编码）读到，就同时是「库区已登记」与「库区属于这个仓」，别的
// 册核所指的库区时用 LoadLatestZone 即可，不必另有读法。
type ZoneRegistry interface {
	// LoadLatestZone 取在册的最新修订，写入用例据它核修订号连续与用途没有改。
	LoadLatestZone(
		ctx context.Context,
		tenant domain.TenantID,
		warehouse domain.WarehouseCode,
		code domain.ZoneCode,
	) (domain.ZoneRevision, bool, error)
	// LoadZoneRevision 按（租户，仓编码，库区编码，修订号）取在册的任何一笔，不只是最新那一笔：对旧修订的原样重投仍是重放。
	LoadZoneRevision(
		ctx context.Context,
		tenant domain.TenantID,
		warehouse domain.WarehouseCode,
		code domain.ZoneCode,
		revision int,
	) (domain.ZoneRevision, bool, error)
	SaveZoneRevision(
		ctx context.Context,
		revision domain.ZoneRevision,
	) (RegistrySaveOutcome, error)
}

// ZoneRow 是库区册读面上的一行，供列表（各库区的当前修订）与修订史共用。
type ZoneRow struct {
	WarehouseCode string
	ZoneCode      string
	Revision      int
	Name          string
	Usage         string
	Deactivated   bool
	Registrant    string
	RegisteredAt  time.Time
}

// ZoneCatalogueRead 是库区册的列表读口（ADR-0077）：本租户各库区的当前修订，可按仓编码与是否停用筛，按 ZonePages 分页
// （ADR-0144）。租户在签名上、不取自查询；limit 非正拒；空册答空页，不答未配置。
type ZoneCatalogueRead interface {
	ListZones(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ZoneRow], error)
}

// ZoneHistoryRead 按一个库区交回册上的全部修订，按修订号升序。不在册与别的租户的同编码库区同形：零行、无错——读口不泄露
// 别的租户有没有这个库区。一条修订链就是要全部交出，不分页。
type ZoneHistoryRead interface {
	ListZoneRevisions(
		ctx context.Context,
		tenant domain.TenantID,
		warehouse domain.WarehouseCode,
		code domain.ZoneCode,
	) ([]ZoneRow, error)
}
