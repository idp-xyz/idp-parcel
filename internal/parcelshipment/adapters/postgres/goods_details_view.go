package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件让 ShipmentRequests 同时实现 ports.GoodsDetailsView（票 declared-goods-lines/07）。读的是同一张表的同一份快照：货物明细
// 随提交版本与客户原始资料版本落库，读回过重建门，锚与那一版由聚合自己答（CurrentGoodsDetails、GoodsDetailsAt）——不加列
// 不加迁移，SQL 里没有第二套采用判断。

var _ ports.GoodsDetailsView = (*ShipmentRequests)(nil)

func (repository *ShipmentRequests) LoadCurrentGoodsDetails(
	ctx context.Context,
	tenant domain.TenantID,
	shipmentRequestID domain.ShipmentRequestID,
) (domain.GoodsDetailsResolution, bool, error) {
	request, found, err := repository.findAcceptedRequest(ctx, tenant, shipmentRequestID)
	if err != nil || !found {
		return domain.GoodsDetailsResolution{}, false, wrapGoodsDetailsView(err)
	}
	resolution, found, err := request.CurrentGoodsDetails()
	return resolution, found, wrapGoodsDetailsView(err)
}

func (repository *ShipmentRequests) LoadGoodsDetailsAt(
	ctx context.Context,
	tenant domain.TenantID,
	shipmentRequestID domain.ShipmentRequestID,
	anchor domain.SourceDataVersionAnchor,
) (domain.GoodsDetails, bool, error) {
	request, found, err := repository.findAcceptedRequest(ctx, tenant, shipmentRequestID)
	if err != nil || !found {
		return domain.GoodsDetails{}, false, wrapGoodsDetailsView(err)
	}
	goods, found := request.GoodsDetailsAt(anchor)
	return goods, found, nil
}

func wrapGoodsDetailsView(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("load goods details: %w", err)
}

// findAcceptedRequest 走（租户 + 委托标识）唯一键取一份`已接受`的委托并重建。零行与别的状态同答 false：统一不可见结果，
// 不区分不存在、他租户与还没接受。
func (repository *ShipmentRequests) findAcceptedRequest(
	ctx context.Context,
	tenant domain.TenantID,
	shipmentRequestID domain.ShipmentRequestID,
) (domain.ShipmentRequest, bool, error) {
	none := domain.ShipmentRequest{}
	if tenant.String() == "" || shipmentRequestID.String() == "" {
		return none, false, fmt.Errorf("tenant and shipment request are required")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, err
	}
	var revision int64
	var state uint8
	var raw []byte
	var numbering documentNumberingColumns
	err = querier.QueryRow(ctx,
		`SELECT revision, state, snapshot, document_number, document_number_absence
		   FROM parcel_shipment.shipment_request
		  WHERE tenant_id = $1
		    AND shipment_request_id = $2
		    AND state = $3`,
		tenant.String(), shipmentRequestID.String(), uint8(domain.ShipmentRequestAccepted),
	).Scan(&revision, &state, &raw, &numbering.number, &numbering.absence)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, err
	}
	var document requestDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return none, false, fmt.Errorf("快照不是本适配器写下的形状：%w", err)
	}
	spec, err := document.rehydrationSpec(revision, domain.ShipmentRequestState(state), numbering)
	if err != nil {
		return none, false, err
	}
	request, err := domain.RehydrateShipmentRequest(spec)
	if err != nil {
		return none, false, err
	}
	return request, true, nil
}
