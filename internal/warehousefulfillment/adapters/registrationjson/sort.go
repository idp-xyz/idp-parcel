package registrationjson

import (
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

type sortDocument struct {
	carriedIdentity
	FactID    *string `json:"factId"`
	Warehouse *string `json:"warehouse"`
	WaveID    *string `json:"waveId"`
	RequestID *string `json:"requestId"`
	GoodsCode *string `json:"goodsCode"`
	Quantity  *int64  `json:"quantity"`
	ToteCode  *string `json:"toteCode"`
}

// SortCommand 把分拣的一次投放译成命令。箱号必带：分拣就是把货放进这张请求的箱。
func SortCommand(reader io.Reader) (application.SortCommand, error) {
	var none application.SortCommand
	document, err := decodeDocument[sortDocument](reader, "sort put")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	fact, err := requiredText("factId", document.FactID, domain.NewPickFactID)
	if err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	wave, err := requiredText("waveId", document.WaveID, domain.NewWaveID)
	if err != nil {
		return none, err
	}
	request, err := requiredText("requestId", document.RequestID, domain.NewOutboundRequestID)
	if err != nil {
		return none, err
	}
	goods, err := requiredText("goodsCode", document.GoodsCode, domain.NewGoodsCode)
	if err != nil {
		return none, err
	}
	if document.Quantity == nil || *document.Quantity < 1 {
		return none, malformed("quantity must be a positive integer")
	}
	tote, err := requiredText("toteCode", document.ToteCode, domain.NewToteCode)
	if err != nil {
		return none, err
	}
	return application.SortCommand{
		FactID: fact, Warehouse: warehouse, Wave: wave, Request: request, Goods: goods, Quantity: *document.Quantity, Tote: tote,
	}, nil
}
