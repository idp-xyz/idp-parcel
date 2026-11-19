package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// LocationTypeRegistry 是库位类型登记册的写入面：键 = 租户 + 库位类型编码 + 修订号，修订只追加、不覆盖。库位类型按租户登记、
// 各仓共用，所以键里没有仓编码。
//
// 读不动走 error，不折成 found=false，理由同 WarehouseRegistry。登记用例在同一个环境事务里调用读方法与保存；保存在没有环境事务时
// 拒绝。别的册核「库位类型已登记」与「库位类型是否停用」时用 LoadLatestLocationType 即可，不必另有读法：读到就是已登记，
// 读到的那一笔的 Content().Deactivated() 就是它此刻是否停用。
type LocationTypeRegistry interface {
	// LoadLatestLocationType 取在册的最新修订，写入用例据它核修订号连续。
	LoadLatestLocationType(
		ctx context.Context,
		tenant domain.TenantID,
		code domain.LocationTypeCode,
	) (domain.LocationTypeRevision, bool, error)
	// LoadLocationTypeRevision 按（租户，库位类型编码，修订号）取在册的任何一笔，不只是最新那一笔：对旧修订的原样重投仍是重放。
	LoadLocationTypeRevision(
		ctx context.Context,
		tenant domain.TenantID,
		code domain.LocationTypeCode,
		revision int,
	) (domain.LocationTypeRevision, bool, error)
	SaveLocationTypeRevision(
		ctx context.Context,
		revision domain.LocationTypeRevision,
	) (RegistrySaveOutcome, error)
}

// LocationTypeRow 是库位类型册读面上的一行，供列表（各库位类型的当前修订）与修订史共用。
type LocationTypeRow struct {
	LocationTypeCode string
	Revision         int
	Name             string
	Deactivated      bool
	Registrant       string
	RegisteredAt     time.Time
}

// LocationTypeCatalogueRead 是库位类型册的列表读口（ADR-0077）：本租户各库位类型的当前修订，可按是否停用筛，按 LocationTypePages
// 分页（ADR-0144）。租户在签名上、不取自查询；limit 非正拒；空册答空页，不答未配置。
type LocationTypeCatalogueRead interface {
	ListLocationTypes(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[LocationTypeRow], error)
}

// LocationTypeHistoryRead 按一个库位类型交回册上的全部修订，按修订号升序。不在册与别的租户的同编码库位类型同形：零行、无错——
// 读口不泄露别的租户有没有这个库位类型。一条修订链就是要全部交出，不分页。
type LocationTypeHistoryRead interface {
	ListLocationTypeRevisions(
		ctx context.Context,
		tenant domain.TenantID,
		code domain.LocationTypeCode,
	) ([]LocationTypeRow, error)
}
