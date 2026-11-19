package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// QueryPackagingMaterialsHandler 是包材册的查阅用例：本租户各包材的当前修订（分页、可按是否停用筛）与一个包材的修订史。它只读已落册的
// 修订，不承诺读到在途的写；空册答空列表，不答未配置（ADR-0077 决定四，ADR-0375 决定五）。
//
// 读不动原样上抛，不折成空册：空册是「没登记」，读不动是「不知道」。
type QueryPackagingMaterialsHandler struct {
	catalogue ports.PackagingMaterialCatalogueRead
	history   ports.PackagingMaterialHistoryRead
}

func NewQueryPackagingMaterialsHandler(
	catalogue ports.PackagingMaterialCatalogueRead,
	history ports.PackagingMaterialHistoryRead,
) *QueryPackagingMaterialsHandler {
	return &QueryPackagingMaterialsHandler{catalogue: catalogue, history: history}
}

// ListPackagingMaterials 答一页各包材的当前修订。查询对象由接入侧按 ports.PackagingMaterialPages 解出。
func (handler *QueryPackagingMaterialsHandler) ListPackagingMaterials(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.PackagingMaterialRow], error) {
	page, err := handler.catalogue.ListPackagingMaterials(ctx, tenant, limit, query)
	if err != nil {
		return cataloguepage.Result[ports.PackagingMaterialRow]{}, fmt.Errorf("list packaging materials: %w", err)
	}
	return page, nil
}

// PackagingMaterialRevisions 答一个包材册上的全部修订，按修订号升序。没登记过的包材与别的租户的同编码包材同形：空。
func (handler *QueryPackagingMaterialsHandler) PackagingMaterialRevisions(
	ctx context.Context,
	tenant domain.TenantID,
	code domain.PackagingMaterialCode,
) ([]ports.PackagingMaterialRow, error) {
	rows, err := handler.history.ListPackagingMaterialRevisions(ctx, tenant, code)
	if err != nil {
		return nil, fmt.Errorf("list packaging material revisions: %w", err)
	}
	return rows, nil
}
