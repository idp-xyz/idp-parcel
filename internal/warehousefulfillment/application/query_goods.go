package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// QueryGoodsHandler 是商品册的查阅用例：按（货主，商品编码）读当前修订；按货主分页列出各商品的当前修订；一个商品的修订史；按（货主，一个码）找商品。
// 它只读已落册的修订，不承诺读到在途的写；没登记的商品、空册与别的货主的商品都答「没有」或空，不答未配置（ADR-0077 决定四，ADR-0375 决定五）。
// 每一种读都以货主为界：别的货主的商品读不到，与没登记同形。
//
// 读不动原样上抛，不折成「没有」：没有是「没登记」，读不动是「不知道」。
type QueryGoodsHandler struct {
	catalogue    ports.GoodsCatalogueRead
	current      ports.GoodsCurrentRead
	history      ports.GoodsHistoryRead
	measurements ports.WarehouseMeasurementRead
	clock        ports.Clock
}

func NewQueryGoodsHandler(
	catalogue ports.GoodsCatalogueRead,
	current ports.GoodsCurrentRead,
	history ports.GoodsHistoryRead,
	measurements ports.WarehouseMeasurementRead,
	clock ports.Clock,
) *QueryGoodsHandler {
	return &QueryGoodsHandler{
		catalogue: catalogue, current: current, history: history, measurements: measurements, clock: clock,
	}
}

// CurrentGoods 读一个商品的当前修订并附带各包装层级的仓测视图；没登记时第二个返回值为假。
func (handler *QueryGoodsHandler) CurrentGoods(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.GoodsCode,
) (ports.GoodsRow, bool, error) {
	row, found, err := handler.current.GetCurrentGoods(ctx, tenant, owner, code)
	if err != nil {
		return ports.GoodsRow{}, false, fmt.Errorf("get current goods: %w", err)
	}
	if !found {
		return ports.GoodsRow{}, false, nil
	}
	if err := handler.attachMeasurements(ctx, tenant, owner, &row); err != nil {
		return ports.GoodsRow{}, false, fmt.Errorf("get current goods: %w", err)
	}
	return row, true, nil
}

// ListGoods 答一页一个货主名下各商品的当前修订（含各包装层级仓测视图）。查询对象由接入侧按 ports.GoodsPages 解出。
func (handler *QueryGoodsHandler) ListGoods(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.GoodsRow], error) {
	page, err := handler.catalogue.ListGoods(ctx, tenant, owner, limit, query)
	if err != nil {
		return cataloguepage.Result[ports.GoodsRow]{}, fmt.Errorf("list goods: %w", err)
	}
	if len(page.Rows) == 0 || handler.measurements == nil {
		return page, nil
	}
	codes := make([]domain.GoodsCode, 0, len(page.Rows))
	for _, row := range page.Rows {
		code, err := domain.NewGoodsCode(row.GoodsCode)
		if err != nil {
			return cataloguepage.Result[ports.GoodsRow]{}, fmt.Errorf("list goods: %w", err)
		}
		codes = append(codes, code)
	}
	grouped, err := handler.measurements.ListForGoodsBatch(ctx, tenant, owner, codes)
	if err != nil {
		return cataloguepage.Result[ports.GoodsRow]{}, fmt.Errorf("list goods: %w", err)
	}
	for index := range page.Rows {
		page.Rows[index].WarehouseMeasurements = buildGoodsLevelMeasurements(page.Rows[index], grouped[codes[index]])
	}
	return page, nil
}

// GoodsRevisions 答一个商品册上的全部修订，按修订号升序。没登记过的商品、别的货主的同编码商品与别的租户的同形：空。
func (handler *QueryGoodsHandler) GoodsRevisions(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.GoodsCode,
) ([]ports.GoodsRow, error) {
	rows, err := handler.history.ListGoodsRevisions(ctx, tenant, owner, code)
	if err != nil {
		return nil, fmt.Errorf("list goods revisions: %w", err)
	}
	return rows, nil
}

// FindGoods 按（货主，一个码）找商品：码是商品编码，或是此刻有效的别名。按此刻生效的修订找，此刻取注入的时钟：
// 别名的有效区间止不含，区间已经结束的别名不再命中；还没开始的同样不命中。找不到时第二个返回值为假。
func (handler *QueryGoodsHandler) FindGoods(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	code domain.IdentifierCode,
) (ports.GoodsRow, bool, error) {
	row, found, err := handler.current.FindGoodsByCode(ctx, tenant, owner, code, handler.clock.Now())
	if err != nil {
		return ports.GoodsRow{}, false, fmt.Errorf("find goods by code: %w", err)
	}
	if !found {
		return ports.GoodsRow{}, false, nil
	}
	if err := handler.attachMeasurements(ctx, tenant, owner, &row); err != nil {
		return ports.GoodsRow{}, false, fmt.Errorf("find goods by code: %w", err)
	}
	return row, true, nil
}

func (handler *QueryGoodsHandler) attachMeasurements(
	ctx context.Context,
	tenant domain.TenantID,
	owner domain.OwnerAccountID,
	row *ports.GoodsRow,
) error {
	if handler.measurements == nil {
		return nil
	}
	goodsCode, err := domain.NewGoodsCode(row.GoodsCode)
	if err != nil {
		return err
	}
	recorded, err := handler.measurements.ListForGoods(ctx, tenant, owner, goodsCode)
	if err != nil {
		return err
	}
	row.WarehouseMeasurements = buildGoodsLevelMeasurements(*row, recorded)
	return nil
}
