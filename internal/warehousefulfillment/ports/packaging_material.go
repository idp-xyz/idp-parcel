package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// PackagingMaterialRegistry 是包材登记册的写入面：键 = 租户 + 包材编码 + 修订号，修订只追加、不覆盖。包材按租户登记、各仓共用，所以键里
// 没有仓编码。
//
// 读不动走 error，不折成 found=false，理由同 WarehouseRegistry。登记用例在同一个环境事务里调用读方法与保存；保存在没有环境事务时拒绝。
type PackagingMaterialRegistry interface {
	// LoadLatestPackagingMaterial 取在册的最新修订，写入用例据它核修订号连续。
	LoadLatestPackagingMaterial(
		ctx context.Context,
		tenant domain.TenantID,
		code domain.PackagingMaterialCode,
	) (domain.PackagingMaterialRevision, bool, error)
	// LoadPackagingMaterialRevision 按（租户，包材编码，修订号）取在册的任何一笔，不只是最新那一笔：对旧修订的原样重投仍是重放。
	LoadPackagingMaterialRevision(
		ctx context.Context,
		tenant domain.TenantID,
		code domain.PackagingMaterialCode,
		revision int,
	) (domain.PackagingMaterialRevision, bool, error)
	SavePackagingMaterialRevision(
		ctx context.Context,
		revision domain.PackagingMaterialRevision,
	) (RegistrySaveOutcome, error)
}

// PackagingMaterialRow 是包材册读面上的一行，供列表（各包材的当前修订）与修订史共用。四个数值是去掉多余前导零与尾零的写法（Measure 的
// 规范写法），与登记口回显的同一份；单位是登记时的码，原样。
type PackagingMaterialRow struct {
	PackagingMaterialCode string
	Revision              int
	Name                  string
	Length                string
	Width                 string
	Height                string
	DimensionsUnit        string
	Weight                string
	WeightUnit            string
	Deactivated           bool
	Registrant            string
	RegisteredAt          time.Time
}

// PackagingMaterialCatalogueRead 是包材册的列表读口（ADR-0077）：本租户各包材的当前修订，可按是否停用筛，按 PackagingMaterialPages 分页
// （ADR-0144）。租户在签名上、不取自查询；limit 非正拒；空册答空页，不答未配置。
type PackagingMaterialCatalogueRead interface {
	ListPackagingMaterials(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[PackagingMaterialRow], error)
}

// PackagingMaterialHistoryRead 按一个包材交回册上的全部修订，按修订号升序。不在册与别的租户的同编码包材同形：零行、无错——读口不泄露别的
// 租户有没有这个包材。一条修订链就是要全部交出，不分页。
type PackagingMaterialHistoryRead interface {
	ListPackagingMaterialRevisions(
		ctx context.Context,
		tenant domain.TenantID,
		code domain.PackagingMaterialCode,
	) ([]PackagingMaterialRow, error)
}
