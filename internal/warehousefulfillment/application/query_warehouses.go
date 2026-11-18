package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// QueryWarehousesHandler 是仓册的查阅用例：本租户各仓的当前修订（分页、可按是否停用筛）与一个仓的修订史。它只读已落册的
// 修订，不承诺读到在途的写；空册答空列表，不答未配置（ADR-0077 决定四，ADR-0375 决定五）。
//
// 读不动原样上抛，不折成空册：空册是「没登记」，读不动是「不知道」。
type QueryWarehousesHandler struct {
	catalogue ports.WarehouseCatalogueRead
	history   ports.WarehouseHistoryRead
}

func NewQueryWarehousesHandler(
	catalogue ports.WarehouseCatalogueRead,
	history ports.WarehouseHistoryRead,
) *QueryWarehousesHandler {
	return &QueryWarehousesHandler{catalogue: catalogue, history: history}
}

// ListWarehouses 答一页各仓的当前修订。查询对象由接入侧按 ports.WarehousePages 解出。
func (handler *QueryWarehousesHandler) ListWarehouses(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.WarehouseRow], error) {
	page, err := handler.catalogue.ListWarehouses(ctx, tenant, limit, query)
	if err != nil {
		return cataloguepage.Result[ports.WarehouseRow]{}, fmt.Errorf("list warehouses: %w", err)
	}
	return page, nil
}

// WarehouseRevisions 答一个仓册上的全部修订，按修订号升序。没登记过的仓与别的租户的同编码仓同形：空。
func (handler *QueryWarehousesHandler) WarehouseRevisions(
	ctx context.Context,
	tenant domain.TenantID,
	code domain.WarehouseCode,
) ([]ports.WarehouseRow, error) {
	rows, err := handler.history.ListWarehouseRevisions(ctx, tenant, code)
	if err != nil {
		return nil, fmt.Errorf("list warehouse revisions: %w", err)
	}
	return rows, nil
}
