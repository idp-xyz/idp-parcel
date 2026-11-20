package registrationjson

import (
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// internalJobDocument 是库内作业四个登记口共用的线格式。种类由口决定，不在载荷里。
// 租户、操作者与设备不在载荷里。数量不为正留给核心判作业内容不合。
type internalJobDocument struct {
	carriedIdentity
	DeviceID       []byte                `json:"deviceId"`
	Operator       []byte                `json:"operator"`
	FactID         *string               `json:"factId"`
	OccurredAt     *string               `json:"occurredAt"`
	DeviceSequence *int64                `json:"deviceSequence"`
	Warehouse      *string               `json:"warehouse"`
	Lines          *[]internalJobLineDoc `json:"lines"`
}

type internalJobLineDoc struct {
	OwnerAccount   *string `json:"ownerAccount"`
	GoodsCode      *string `json:"goodsCode"`
	Unit           *string `json:"unit"`
	StockState     *string `json:"stockState"`
	Quantity       *int64  `json:"quantity"`
	SourceLocation *string `json:"sourceLocation"`
	TargetLocation *string `json:"targetLocation"`
	ToUnit         *string `json:"toUnit"`
	ToQuantity     *int64  `json:"toQuantity"`
	ToState        *string `json:"toState"`
	BatchNumber    *string `json:"batchNumber"`
	ExpiryDate     *string `json:"expiryDate"`
	LabelAlias     *string `json:"labelAlias"`
	InboundBatchID *string `json:"inboundBatchId"`
	CartonNumber   *string `json:"cartonNumber"`
	Note           *string `json:"note"`
}

// InternalJobCommand 把库内作业载荷译成命令。租户、操作者与设备留空，由 Intake 在认证之后填上。
func InternalJobCommand(reader io.Reader, kind domain.InternalJobKind) (application.RegisterInternalJobCommand, error) {
	var none application.RegisterInternalJobCommand
	document, err := decodeDocument[internalJobDocument](reader, "internal job")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if len(document.DeviceID) > 0 || len(document.Operator) > 0 {
		return none, malformed("deviceId and operator must not be carried in the payload")
	}
	factID, err := requiredText("factId", document.FactID, domain.NewInternalJobFactID)
	if err != nil {
		return none, err
	}
	occurredAt, err := requiredInstant("occurredAt", document.OccurredAt)
	if err != nil {
		return none, err
	}
	if document.DeviceSequence == nil {
		return none, malformed("deviceSequence is required")
	}
	if *document.DeviceSequence < 1 {
		return none, malformed("deviceSequence must be a positive integer")
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	if document.Lines == nil || len(*document.Lines) == 0 {
		return none, malformed("lines is required")
	}
	lines := make([]domain.InternalJobLine, 0, len(*document.Lines))
	for index, item := range *document.Lines {
		line, err := internalJobLine(index, item)
		if err != nil {
			return none, err
		}
		lines = append(lines, line)
	}
	return application.RegisterInternalJobCommand{
		FactID: factID, Kind: kind, OccurredAt: occurredAt, DeviceSequence: *document.DeviceSequence,
		Warehouse: warehouse, Lines: lines,
	}, nil
}

func internalJobLine(index int, document internalJobLineDoc) (domain.InternalJobLine, error) {
	path := "lines[" + itoa(index) + "]"
	owner, err := requiredText(path+".ownerAccount", document.OwnerAccount, domain.NewOwnerAccountID)
	if err != nil {
		return domain.InternalJobLine{}, err
	}
	goods, err := optionalDomain(path+".goodsCode", document.GoodsCode, domain.NewGoodsCode)
	if err != nil {
		return domain.InternalJobLine{}, err
	}
	unit, err := optionalDomain(path+".unit", document.Unit, domain.NewGoodsUnitName)
	if err != nil {
		return domain.InternalJobLine{}, err
	}
	var state domain.StockState
	if document.StockState != nil {
		state, err = domain.NewStockState(*document.StockState)
		if err != nil {
			return domain.InternalJobLine{}, malformed("%s.stockState is not a stock state", path)
		}
	}
	source, err := optionalDomain(path+".sourceLocation", document.SourceLocation, domain.NewLocationCode)
	if err != nil {
		return domain.InternalJobLine{}, err
	}
	target, err := optionalDomain(path+".targetLocation", document.TargetLocation, domain.NewLocationCode)
	if err != nil {
		return domain.InternalJobLine{}, err
	}
	toUnit, err := optionalDomain(path+".toUnit", document.ToUnit, domain.NewGoodsUnitName)
	if err != nil {
		return domain.InternalJobLine{}, err
	}
	var toState domain.StockState
	if document.ToState != nil {
		toState, err = domain.NewStockState(*document.ToState)
		if err != nil {
			return domain.InternalJobLine{}, malformed("%s.toState is not a stock state", path)
		}
	}
	batch, err := optionalDomain(path+".batchNumber", document.BatchNumber, domain.NewBatchNumber)
	if err != nil {
		return domain.InternalJobLine{}, err
	}
	var expiry domain.ExpiryDate
	if document.ExpiryDate != nil && *document.ExpiryDate != "" {
		expiry, err = domain.NewExpiryDate(*document.ExpiryDate)
		if err != nil {
			return domain.InternalJobLine{}, malformed("%s.expiryDate must be a calendar date", path)
		}
	}
	alias, err := optionalDomain(path+".labelAlias", document.LabelAlias, domain.NewGoodsAliasCode)
	if err != nil {
		return domain.InternalJobLine{}, err
	}
	named, err := optionalDomain(path+".inboundBatchId", document.InboundBatchID, domain.NewInboundBatchID)
	if err != nil {
		return domain.InternalJobLine{}, err
	}
	carton, err := optionalDomain(path+".cartonNumber", document.CartonNumber, domain.NewCartonNumber)
	if err != nil {
		return domain.InternalJobLine{}, err
	}
	var quantity int64
	if document.Quantity != nil {
		quantity = *document.Quantity
	}
	var toQuantity int64
	if document.ToQuantity != nil {
		toQuantity = *document.ToQuantity
	}
	var note string
	if document.Note != nil {
		note = *document.Note
	}
	line, err := domain.NewInternalJobLine(domain.InternalJobLineSpec{
		Owner: owner, Goods: goods, Unit: unit, State: state, Alias: alias, Batch: batch, Expiry: expiry, NamedBatch: named,
		Source: source, Target: target, Quantity: quantity, ToUnit: toUnit, ToQuantity: toQuantity, ToState: toState,
		Carton: carton, Note: note,
	})
	if err != nil {
		return domain.InternalJobLine{}, malformed("%s carries a character that cannot be stored (NUL or invalid UTF-8)", path)
	}
	return line, nil
}

func optionalDomain[Value any](path string, raw *string, construct func(string) (Value, error)) (Value, error) {
	var none Value
	if raw == nil || *raw == "" {
		return none, nil
	}
	return textCell(path, *raw, construct)
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}

type conditionRestoreDocument struct {
	carriedIdentity
	Operator       []byte  `json:"operator"`
	DecisionID     *string `json:"decisionId"`
	Warehouse      *string `json:"warehouse"`
	OwnerAccount   *string `json:"ownerAccount"`
	LocationCode   *string `json:"locationCode"`
	GoodsCode      *string `json:"goodsCode"`
	Unit           *string `json:"unit"`
	StockState     *string `json:"stockState"`
	Quantity       *int64  `json:"quantity"`
	Reason         *string `json:"reason"`
	BatchNumber    *string `json:"batchNumber"`
	ExpiryDate     *string `json:"expiryDate"`
	LabelAlias     *string `json:"labelAlias"`
	InboundBatchID *string `json:"inboundBatchId"`
}

// ConditionRestoreCommand 把次品转良品载荷译成命令。租户与操作者留空。没带原因是坏载荷。
func ConditionRestoreCommand(reader io.Reader) (application.RestoreConditionCommand, error) {
	var none application.RestoreConditionCommand
	document, err := decodeDocument[conditionRestoreDocument](reader, "condition restore")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if len(document.Operator) > 0 {
		return none, malformed("operator must not be carried in the payload")
	}
	id, err := requiredText("decisionId", document.DecisionID, domain.NewConditionRestoreID)
	if err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	owner, err := requiredText("ownerAccount", document.OwnerAccount, domain.NewOwnerAccountID)
	if err != nil {
		return none, err
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
	if document.Quantity == nil {
		return none, malformed("quantity is required")
	}
	if document.Reason == nil || *document.Reason == "" {
		return none, malformed("reason is required")
	}
	var state domain.StockState
	if document.StockState != nil {
		state, err = domain.NewStockState(*document.StockState)
		if err != nil {
			return none, malformed("stockState is not a stock state")
		}
	}
	batch, err := optionalDomain("batchNumber", document.BatchNumber, domain.NewBatchNumber)
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
	alias, err := optionalDomain("labelAlias", document.LabelAlias, domain.NewGoodsAliasCode)
	if err != nil {
		return none, err
	}
	named, err := optionalDomain("inboundBatchId", document.InboundBatchID, domain.NewInboundBatchID)
	if err != nil {
		return none, err
	}
	return application.RestoreConditionCommand{
		ID: id, Warehouse: warehouse, Owner: owner, Location: location, Goods: goods, Unit: unit, State: state,
		Quantity: *document.Quantity, Reason: *document.Reason, Batch: batch, Expiry: expiry, Alias: alias, NamedBatch: named,
	}, nil
}
