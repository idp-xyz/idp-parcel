package registrationjson

import (
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

type freezeDocument struct {
	carriedIdentity
	DecisionID     *string `json:"decisionId"`
	OwnerAccount   *string `json:"ownerAccount"`
	Warehouse      *string `json:"warehouse"`
	CartonNumber   *string `json:"cartonNumber"`
	LocationCode   *string `json:"locationCode"`
	GoodsCode      *string `json:"goodsCode"`
	Unit           *string `json:"unit"`
	StockState     *string `json:"stockState"`
	LabelAlias     *string `json:"labelAlias"`
	BatchNumber    *string `json:"batchNumber"`
	ExpiryDate     *string `json:"expiryDate"`
	InboundBatchID *string `json:"inboundBatchId"`
	Quantity       *int64  `json:"quantity"`
	Reason         *string `json:"reason"`
}

// FreezeCommand 把冻结译成命令。整箱只带箱号；商品必须点名入库批次。原因必带。
func FreezeCommand(reader io.Reader) (application.FreezeCommand, error) {
	var none application.FreezeCommand
	document, err := decodeDocument[freezeDocument](reader, "stock freeze")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	decision, err := requiredText("decisionId", document.DecisionID, domain.NewRequestKey)
	if err != nil {
		return none, err
	}
	owner, err := requiredText("ownerAccount", document.OwnerAccount, domain.NewOwnerAccountID)
	if err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	if document.Reason == nil || *document.Reason == "" {
		return none, malformed("reason is required")
	}
	command := application.FreezeCommand{
		DecisionID: decision, Owner: owner, Warehouse: warehouse, Reason: *document.Reason,
	}
	if document.CartonNumber != nil && *document.CartonNumber != "" {
		carton, err := domain.NewCartonNumber(*document.CartonNumber)
		if err != nil {
			return none, malformed("cartonNumber must not be blank")
		}
		if document.Quantity != nil || document.LocationCode != nil || document.GoodsCode != nil || document.InboundBatchID != nil {
			return none, malformed("a carton freeze does not carry a cell or a quantity")
		}
		command.Target = domain.FreezeCarton
		command.Carton = carton
		return command, nil
	}
	if document.InboundBatchID == nil || *document.InboundBatchID == "" {
		return none, malformed("inboundBatchId is required")
	}
	if document.Quantity == nil || *document.Quantity <= 0 {
		return none, malformed("quantity must be a positive integer")
	}
	location, err := requiredText("locationCode", document.LocationCode, domain.NewLocationCode)
	if err != nil {
		return none, err
	}
	goods, err := requiredText("goodsCode", document.GoodsCode, domain.NewGoodsCode)
	if err != nil {
		return none, err
	}
	unit, err := requiredText("unit", document.Unit, domain.NewGoodsUnitName)
	if err != nil {
		return none, err
	}
	if document.StockState == nil {
		return none, malformed("stockState is required")
	}
	state, err := domain.NewStockState(*document.StockState)
	if err != nil {
		return none, malformed("stockState is not a stock state")
	}
	alias, err := optionalDomain("labelAlias", document.LabelAlias, domain.NewGoodsAliasCode)
	if err != nil {
		return none, err
	}
	batch, err := optionalDomain("batchNumber", document.BatchNumber, domain.NewBatchNumber)
	if err != nil {
		return none, err
	}
	inbound, err := requiredText("inboundBatchId", document.InboundBatchID, domain.NewInboundBatchID)
	if err != nil {
		return none, err
	}
	var expiry domain.ExpiryDate
	if document.ExpiryDate != nil && *document.ExpiryDate != "" {
		expiry, err = domain.NewExpiryDate(*document.ExpiryDate)
		if err != nil {
			return none, malformed("expiryDate must be a calendar date")
		}
	}
	cell, err := domain.NewStockCell(domain.StockCellSpec{
		Location: location, Goods: goods, Unit: unit, State: state, Alias: alias, InboundBatch: inbound, BatchNumber: batch, Expiry: expiry,
	})
	if err != nil {
		return none, malformed("the stock cell is incomplete")
	}
	command.Target = domain.FreezeGoods
	command.Cell = cell
	command.Quantity = *document.Quantity
	return command, nil
}

type unfreezeDocument struct {
	carriedIdentity
	refusedOwner
	FreezeID *string `json:"freezeId"`
	Note     *string `json:"note"`
}

// UnfreezeCommand 把解冻译成命令。货主与格不从载荷收。
func UnfreezeCommand(reader io.Reader) (application.UnfreezeCommand, error) {
	var none application.UnfreezeCommand
	document, err := decodeDocument[unfreezeDocument](reader, "stock unfreeze")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if len(document.OwnerAccount) > 0 {
		return none, malformed("ownerAccount must not be carried in the payload; the owner is taken from the freeze")
	}
	id, err := requiredText("freezeId", document.FreezeID, domain.NewFreezeID)
	if err != nil {
		return none, err
	}
	note := ""
	if document.Note != nil {
		note = *document.Note
	}
	return application.UnfreezeCommand{ID: id, Note: note}, nil
}
