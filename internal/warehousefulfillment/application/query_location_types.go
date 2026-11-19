package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// QueryLocationTypesHandler 是库位类型册的查阅用例：本租户各库位类型的当前修订（分页、可按是否停用筛）与一个库位类型的修订史。它只读
// 已落册的修订，不承诺读到在途的写；空册答空列表，不答未配置（ADR-0077 决定四，ADR-0375 决定五）。
//
// 读不动原样上抛，不折成空册：空册是「没登记」，读不动是「不知道」。
type QueryLocationTypesHandler struct {
	catalogue ports.LocationTypeCatalogueRead
	history   ports.LocationTypeHistoryRead
}

func NewQueryLocationTypesHandler(
	catalogue ports.LocationTypeCatalogueRead,
	history ports.LocationTypeHistoryRead,
) *QueryLocationTypesHandler {
	return &QueryLocationTypesHandler{catalogue: catalogue, history: history}
}

// ListLocationTypes 答一页各库位类型的当前修订。查询对象由接入侧按 ports.LocationTypePages 解出。
func (handler *QueryLocationTypesHandler) ListLocationTypes(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.LocationTypeRow], error) {
	page, err := handler.catalogue.ListLocationTypes(ctx, tenant, limit, query)
	if err != nil {
		return cataloguepage.Result[ports.LocationTypeRow]{}, fmt.Errorf("list location types: %w", err)
	}
	return page, nil
}

// LocationTypeRevisions 答一个库位类型册上的全部修订，按修订号升序。没登记过的库位类型与别的租户的同编码库位类型同形：空。
func (handler *QueryLocationTypesHandler) LocationTypeRevisions(
	ctx context.Context,
	tenant domain.TenantID,
	code domain.LocationTypeCode,
) ([]ports.LocationTypeRow, error) {
	rows, err := handler.history.ListLocationTypeRevisions(ctx, tenant, code)
	if err != nil {
		return nil, fmt.Errorf("list location type revisions: %w", err)
	}
	return rows, nil
}
