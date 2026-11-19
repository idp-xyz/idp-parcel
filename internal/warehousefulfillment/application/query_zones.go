package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// QueryZonesHandler 是库区册的查阅用例：本租户各库区的当前修订（分页、可按仓编码与是否停用筛）与一个库区的修订史。它只读已落册的
// 修订，不承诺读到在途的写；空册答空列表，不答未配置（ADR-0077 决定四，ADR-0375 决定五）。
//
// 读不动原样上抛，不折成空册：空册是「没登记」，读不动是「不知道」。
type QueryZonesHandler struct {
	catalogue ports.ZoneCatalogueRead
	history   ports.ZoneHistoryRead
}

func NewQueryZonesHandler(catalogue ports.ZoneCatalogueRead, history ports.ZoneHistoryRead) *QueryZonesHandler {
	return &QueryZonesHandler{catalogue: catalogue, history: history}
}

// ListZones 答一页各库区的当前修订。查询对象由接入侧按 ports.ZonePages 解出。
func (handler *QueryZonesHandler) ListZones(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ZoneRow], error) {
	page, err := handler.catalogue.ListZones(ctx, tenant, limit, query)
	if err != nil {
		return cataloguepage.Result[ports.ZoneRow]{}, fmt.Errorf("list zones: %w", err)
	}
	return page, nil
}

// ZoneRevisions 答一个库区册上的全部修订，按修订号升序。没登记过的库区与别的租户的同编码库区同形：空。
func (handler *QueryZonesHandler) ZoneRevisions(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.ZoneCode,
) ([]ports.ZoneRow, error) {
	rows, err := handler.history.ListZoneRevisions(ctx, tenant, warehouse, code)
	if err != nil {
		return nil, fmt.Errorf("list zone revisions: %w", err)
	}
	return rows, nil
}
