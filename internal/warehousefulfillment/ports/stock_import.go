package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// StockImportStore 是建账导入册。Hold 与写入在没有环境事务时拒绝。
// 写入只追加待批准的一份，之后只把状态从待批准收成已入账或已驳回，不改各行。
type StockImportStore interface {
	Hold(ctx context.Context, tenant domain.TenantID, id domain.StockImportID) error
	Load(ctx context.Context, tenant domain.TenantID, id domain.StockImportID) (domain.StockImport, bool, error)
	SavePending(ctx context.Context, item domain.StockImport) error
	MarkPosted(ctx context.Context, item domain.StockImport) error
	MarkRejected(ctx context.Context, item domain.StockImport) error
	List(ctx context.Context, tenant domain.TenantID, limit int, query cataloguepage.Query) (cataloguepage.Result[domain.StockImport], error)
}
