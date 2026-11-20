package registrationjson

import (
	"encoding/json"
	"io"
	"strconv"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// receiptConfirmationDocument 是收货确认的线格式。租户、操作者与设备不在载荷里：前两格取自信封，设备取自作业事实请求头。
// 这三格的键只为拒而存在，键在场即坏载荷，不看值。数量是 JSON 数字：商品代录里的数值是 JSON 字符串，是为了避开二进制浮点；
// 收货数量是正整数，整数没有那一格要躲的舍入，用 JSON 数字。不为正留给核心判收货内容不合，译装不提前拒。
type receiptConfirmationDocument struct {
	carriedIdentity
	DeviceID    json.RawMessage        `json:"deviceId"`
	Operator    json.RawMessage        `json:"operator"`
	FactID      *string                `json:"factId"`
	OccurredAt  *string                `json:"occurredAt"`
	Warehouse   *string                `json:"warehouse"`
	PreAdviceID *string                `json:"preAdviceId"`
	Shape       *string                `json:"shape"`
	Lines       *[]receiptLineDocument `json:"lines"`
}

type receiptLineDocument struct {
	Observed              *string `json:"observed"`
	Unit                  *string `json:"unit"`
	StockState            *string `json:"stockState"`
	Quantity              *int64  `json:"quantity"`
	BatchNumber           *string `json:"batchNumber"`
	ExpiryDate            *string `json:"expiryDate"`
	LabelAlias            *string `json:"labelAlias"`
	LocationCode          *string `json:"locationCode"`
	CartonNumber          *string `json:"cartonNumber"`
	WarehouseCartonNumber *string `json:"warehouseCartonNumber"`
	Damaged               *bool   `json:"damaged"`
}

func (document receiptConfirmationDocument) refuseCarriedActors() error {
	if err := document.refuseCarriedIdentity(); err != nil {
		return err
	}
	if len(document.DeviceID) > 0 {
		return malformed("deviceId must not be carried in the payload; the device comes from the work-fact headers, not from the request")
	}
	if len(document.Operator) > 0 {
		return malformed("operator must not be carried in the payload; the operator comes from the work-fact envelope, not from the request")
	}
	return nil
}

// ReceiptConfirmationCommand 把收货确认载荷译成命令。租户、操作者与设备留空，由 Intake 在认证之后填上。
func ReceiptConfirmationCommand(reader io.Reader) (application.ConfirmReceiptCommand, error) {
	var none application.ConfirmReceiptCommand
	document, err := decodeDocument[receiptConfirmationDocument](reader, "receipt confirmation")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedActors(); err != nil {
		return none, err
	}
	factID, err := requiredText("factId", document.FactID, domain.NewReceiptFactID)
	if err != nil {
		return none, err
	}
	occurredAt, err := requiredInstant("occurredAt", document.OccurredAt)
	if err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	preAdvice, err := requiredText("preAdviceId", document.PreAdviceID, domain.NewPreAdviceID)
	if err != nil {
		return none, err
	}
	shape, err := requiredClosedWord("shape", "inbound mode", document.Shape, domain.NewInboundMode)
	if err != nil {
		return none, err
	}
	if shape == domain.InboundByCarton {
		cartonLines, err := cartonReceiptLines(document.Lines)
		if err != nil {
			return none, err
		}
		return application.ConfirmReceiptCommand{
			FactID: factID, OccurredAt: occurredAt, Warehouse: warehouse, PreAdvice: preAdvice, Shape: shape, CartonLines: cartonLines,
		}, nil
	}
	lines, err := receiptLines(document.Lines)
	if err != nil {
		return none, err
	}
	return application.ConfirmReceiptCommand{
		FactID: factID, OccurredAt: occurredAt, Warehouse: warehouse, PreAdvice: preAdvice, Shape: shape, Lines: lines,
	}, nil
}

func receiptLines(documents *[]receiptLineDocument) ([]domain.ReceiptLine, error) {
	if documents == nil {
		return nil, malformed("lines is required")
	}
	lines := make([]domain.ReceiptLine, 0, len(*documents))
	for index, document := range *documents {
		line, err := receiptLine(index, document)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func cartonReceiptLines(documents *[]receiptLineDocument) ([]domain.CartonReceiptLine, error) {
	if documents == nil {
		return nil, malformed("lines is required")
	}
	lines := make([]domain.CartonReceiptLine, 0, len(*documents))
	for index, document := range *documents {
		line, err := cartonReceiptLine(index, document)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func receiptLine(index int, document receiptLineDocument) (domain.ReceiptLine, error) {
	path := "lines[" + strconv.Itoa(index) + "]"
	var none domain.ReceiptLine
	if document.CartonNumber != nil || document.WarehouseCartonNumber != nil || document.Damaged != nil {
		return none, malformed("%s is a carton line", path)
	}
	if document.Observed == nil {
		return none, malformed("%s.observed is required", path)
	}
	if document.Quantity == nil {
		return none, malformed("%s.quantity is required", path)
	}
	unit, err := requiredText(path+".unit", document.Unit, domain.NewGoodsUnitName)
	if err != nil {
		return none, err
	}
	state, err := requiredClosedWord(path+".stockState", "stock state", document.StockState, domain.NewStockState)
	if err != nil {
		return none, err
	}
	location, err := requiredText(path+".locationCode", document.LocationCode, domain.NewLocationCode)
	if err != nil {
		return none, err
	}
	batch, err := optionalBatch(path+".batchNumber", document.BatchNumber)
	if err != nil {
		return none, err
	}
	expiry, err := optionalExpiry(path+".expiryDate", document.ExpiryDate)
	if err != nil {
		return none, err
	}
	alias, err := optionalAlias(path+".labelAlias", document.LabelAlias)
	if err != nil {
		return none, err
	}
	line, err := domain.NewReceiptLine(*document.Observed, unit, state, *document.Quantity, batch, expiry, alias, location)
	if err != nil {
		return none, malformed("%s is not a receivable line", path)
	}
	return line, nil
}

func cartonReceiptLine(index int, document receiptLineDocument) (domain.CartonReceiptLine, error) {
	path := "lines[" + strconv.Itoa(index) + "]"
	var none domain.CartonReceiptLine
	if document.Observed != nil || document.Unit != nil || document.StockState != nil || document.Quantity != nil ||
		document.BatchNumber != nil || document.ExpiryDate != nil || document.LabelAlias != nil {
		return none, malformed("%s is a goods line", path)
	}
	number, err := requiredText(path+".cartonNumber", document.CartonNumber, domain.NewCartonNumber)
	if err != nil {
		return none, err
	}
	location, err := requiredText(path+".locationCode", document.LocationCode, domain.NewLocationCode)
	if err != nil {
		return none, err
	}
	var warehouse domain.CartonNumber
	if document.WarehouseCartonNumber != nil {
		warehouse, err = textCell(path+".warehouseCartonNumber", *document.WarehouseCartonNumber, domain.NewCartonNumber)
		if err != nil {
			return none, err
		}
	}
	damaged := false
	if document.Damaged != nil {
		damaged = *document.Damaged
	}
	line, err := domain.NewCartonReceiptLine(number, warehouse, damaged, location)
	if err != nil {
		return none, malformed("%s is not a receivable carton line", path)
	}
	return line, nil
}

func optionalAlias(path string, raw *string) (domain.GoodsAliasCode, error) {
	if raw == nil {
		return domain.GoodsAliasCode{}, nil
	}
	return textCell(path, *raw, domain.NewGoodsAliasCode)
}
