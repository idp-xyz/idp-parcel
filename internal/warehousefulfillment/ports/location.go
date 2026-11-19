package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// ZoneLookup 是库位核「所属库区已登记且属于这个仓」要的那一点读法：键里带仓编码，按（租户，仓编码，库区编码）读到，就同时是
// 「库区已登记」与「库区属于这个仓」；库区编码在另一个仓下存在，在这个仓下照样读不到。它只读不写，所以拿它的用例碰不到库区的
// 写入面。真实现是库区登记册本身（ZoneRegistry 的这一个读方法）。
//
// 读不动走 error，不折成 found=false，理由同 WarehouseLookup。
type ZoneLookup interface {
	LoadLatestZone(
		ctx context.Context,
		tenant domain.TenantID,
		warehouse domain.WarehouseCode,
		code domain.ZoneCode,
	) (domain.ZoneRevision, bool, error)
}

// LocationTypeLookup 是库位核「带的库位类型已登记」与「新指名的库位类型是否已停用」要的那一点读法：读到就是已登记，读到的那一笔的
// Content().Deactivated() 就是它此刻是否停用。它只读不写。真实现是库位类型登记册本身（LocationTypeRegistry 的这一个读方法）。
//
// 读不动走 error，不折成 found=false，理由同 WarehouseLookup。
type LocationTypeLookup interface {
	LoadLatestLocationType(
		ctx context.Context,
		tenant domain.TenantID,
		code domain.LocationTypeCode,
	) (domain.LocationTypeRevision, bool, error)
}

// LocationRegistry 是库位登记册的写入面：键 = 租户 + 仓编码 + 库位编码 + 修订号，修订只追加、不覆盖。
//
// 读不动走 error，不折成 found=false，理由同 WarehouseRegistry。登记用例在同一个环境事务里调用读方法与保存；保存在没有环境事务时
// 拒绝。库位编码在仓内才唯一，所以键里带仓编码。
type LocationRegistry interface {
	// LoadLatestLocation 取在册的最新修订，写入用例据它核修订号连续、所属库区没有改与库位类型是不是新指名。
	LoadLatestLocation(
		ctx context.Context,
		tenant domain.TenantID,
		warehouse domain.WarehouseCode,
		code domain.LocationCode,
	) (domain.LocationRevision, bool, error)
	// LoadLocationRevision 按（租户，仓编码，库位编码，修订号）取在册的任何一笔，不只是最新那一笔：对旧修订的原样重投仍是重放。
	LoadLocationRevision(
		ctx context.Context,
		tenant domain.TenantID,
		warehouse domain.WarehouseCode,
		code domain.LocationCode,
		revision int,
	) (domain.LocationRevision, bool, error)
	SaveLocationRevision(
		ctx context.Context,
		revision domain.LocationRevision,
	) (RegistrySaveOutcome, error)
}

// LocationRow 是库位册读面上的一行，供列表（各库位的当前修订）与修订史共用。库位类型与优先级可缺：缺就是 nil，不是空串或 0——
// 「没有」与「有」是不同的内容。
type LocationRow struct {
	WarehouseCode    string
	LocationCode     string
	Revision         int
	ZoneCode         string
	LocationTypeCode *string
	Priority         *int32
	Deactivated      bool
	Registrant       string
	RegisteredAt     time.Time
}

// LocationCatalogueRead 是库位册的列表读口（ADR-0077）：本租户各库位的当前修订，可按仓编码、库区编码与是否停用筛，按 LocationPages
// 分页（ADR-0144）。租户在签名上、不取自查询；limit 非正拒；空册答空页，不答未配置。
type LocationCatalogueRead interface {
	ListLocations(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[LocationRow], error)
}

// LocationHistoryRead 按一个库位交回册上的全部修订，按修订号升序。不在册与别的租户的同编码库位同形：零行、无错——读口不泄露别的
// 租户有没有这个库位。一条修订链就是要全部交出，不分页。
type LocationHistoryRead interface {
	ListLocationRevisions(
		ctx context.Context,
		tenant domain.TenantID,
		warehouse domain.WarehouseCode,
		code domain.LocationCode,
	) ([]LocationRow, error)
}
