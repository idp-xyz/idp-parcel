package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// Clock 给登记口取登记时刻。用例不自己读系统时钟，测试才摆得出确定的时刻。
type Clock interface {
	Now() time.Time
}

// WarehouseSaveOutcome 是一笔仓修订在持久化面的落点（ADR-0031）：`已落库`是新写入的一笔，`已在册`是同键同内容的
// 重放，`内容冲突`是同键异内容。业务格不用 error 表达，事务因此保持可用。比的只是完整内容，不比登记责任方与登记时刻。
type WarehouseSaveOutcome uint8

const (
	WarehouseSaveOutcomeInvalid WarehouseSaveOutcome = iota
	WarehouseRevisionSaved
	WarehouseRevisionAlreadyRegistered
	WarehouseRevisionContentConflict
)

func (outcome WarehouseSaveOutcome) String() string {
	switch outcome {
	case WarehouseRevisionSaved:
		return "SAVED"
	case WarehouseRevisionAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case WarehouseRevisionContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

// WarehouseRegistry 是仓登记册的写入面：键 = 租户 + 仓编码 + 修订号，修订只追加、不覆盖。
//
// 读不动走 error，不折成 found=false：把一次该重试的故障答成「从未登记」，登记方就会从修订 1 重登一个本来就在册的仓。
// 登记用例在同一个环境事务里调用两个读方法与保存；保存在没有环境事务时拒绝。
type WarehouseRegistry interface {
	// LoadLatestWarehouse 取在册的最新修订，写入用例据它核修订号连续与种类没有改。
	LoadLatestWarehouse(
		ctx context.Context,
		tenant domain.TenantID,
		code domain.WarehouseCode,
	) (domain.WarehouseRevision, bool, error)
	// LoadWarehouseRevision 按（租户，仓编码，修订号）取在册的任何一笔，不只是最新那一笔：对旧修订的原样重投仍是重放。
	LoadWarehouseRevision(
		ctx context.Context,
		tenant domain.TenantID,
		code domain.WarehouseCode,
		revision int,
	) (domain.WarehouseRevision, bool, error)
	SaveWarehouseRevision(
		ctx context.Context,
		revision domain.WarehouseRevision,
	) (WarehouseSaveOutcome, error)
}

// WarehouseRow 是仓册读面上的一行，供列表（各仓的当前修订）与修订史共用。可缺的两格带 Has 标记：逐字保存的取值
// 不会是空串，但「没登记」「没指名」另给一格比靠空串约定更不容易读错。
type WarehouseRow struct {
	WarehouseCode       string
	Revision            int
	Name                string
	Kind                string
	CountryCode         string
	AddressText         string
	BusinessTimeZone    string
	HasBusinessTimeZone bool
	ColocatedNode       string
	HasColocatedNode    bool
	Deactivated         bool
	Registrant          string
	RegisteredAt        time.Time
}

// WarehouseCatalogueRead 是仓册的列表读口（ADR-0077）：本租户各仓的当前修订，可按是否停用筛，按 WarehousePages 分页
// （ADR-0144）。租户在签名上、不取自查询；limit 非正拒；空册答空页，不答未配置。
type WarehouseCatalogueRead interface {
	ListWarehouses(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[WarehouseRow], error)
}

// WarehouseHistoryRead 按一个仓交回册上的全部修订，按修订号升序。不在册与别的租户的同编码仓同形：零行、无错——
// 读口不泄露别的租户有没有这个仓。一条修订链就是要全部交出，不分页。
type WarehouseHistoryRead interface {
	ListWarehouseRevisions(
		ctx context.Context,
		tenant domain.TenantID,
		code domain.WarehouseCode,
	) ([]WarehouseRow, error)
}
