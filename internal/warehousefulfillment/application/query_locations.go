package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// QueryLocationsHandler 是库位册的查阅用例：本租户各库位的当前修订（分页、可按仓编码、库区编码与是否停用筛）与一个库位的修订史。
// 它只读已落册的修订，不承诺读到在途的写；空册答空列表，不答未配置（ADR-0077 决定四，ADR-0375 决定五）。
//
// 读不动原样上抛，不折成空册：空册是「没登记」，读不动是「不知道」。
type QueryLocationsHandler struct {
	catalogue ports.LocationCatalogueRead
	history   ports.LocationHistoryRead
}

func NewQueryLocationsHandler(catalogue ports.LocationCatalogueRead, history ports.LocationHistoryRead) *QueryLocationsHandler {
	return &QueryLocationsHandler{catalogue: catalogue, history: history}
}

// ListLocations 答一页各库位的当前修订。查询对象由接入侧按 ports.LocationPages 解出。
func (handler *QueryLocationsHandler) ListLocations(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.LocationRow], error) {
	page, err := handler.catalogue.ListLocations(ctx, tenant, limit, query)
	if err != nil {
		return cataloguepage.Result[ports.LocationRow]{}, fmt.Errorf("list locations: %w", err)
	}
	return page, nil
}

// LocationRevisions 答一个库位册上的全部修订，按修订号升序。没登记过的库位与别的租户的同编码库位同形：空。
func (handler *QueryLocationsHandler) LocationRevisions(
	ctx context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.LocationCode,
) ([]ports.LocationRow, error) {
	rows, err := handler.history.ListLocationRevisions(ctx, tenant, warehouse, code)
	if err != nil {
		return nil, fmt.Errorf("list location revisions: %w", err)
	}
	return rows, nil
}
