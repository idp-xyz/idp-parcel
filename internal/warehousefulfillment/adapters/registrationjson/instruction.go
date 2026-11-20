package registrationjson

import (
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

type instructionExecutionDocument struct {
	carriedIdentity
	DeviceID       []byte                         `json:"deviceId"`
	Operator       []byte                         `json:"operator"`
	FactID         *string                        `json:"factId"`
	OccurredAt     *string                        `json:"occurredAt"`
	DeviceSequence *int64                         `json:"deviceSequence"`
	Warehouse      *string                        `json:"warehouse"`
	InstructionID  *string                        `json:"instructionId"`
	DoneQuantity   *int64                         `json:"doneQuantity"`
	Lines          *[]instructionExecutionLineDoc `json:"lines"`
}

type instructionExecutionLineDoc struct {
	Direction      *string `json:"direction"`
	LocationCode   *string `json:"locationCode"`
	GoodsCode      *string `json:"goodsCode"`
	Unit           *string `json:"unit"`
	StockState     *string `json:"stockState"`
	Quantity       *int64  `json:"quantity"`
	LabelAlias     *string `json:"labelAlias"`
	BatchNumber    *string `json:"batchNumber"`
	ExpiryDate     *string `json:"expiryDate"`
	InboundBatchID *string `json:"inboundBatchId"`
}

// InstructionExecutionCommand 把加工或换标执行译成命令。租户、设备与操作者由 Intake 填。
func InstructionExecutionCommand(reader io.Reader) (application.ExecuteInstructionCommand, error) {
	var none application.ExecuteInstructionCommand
	document, err := decodeDocument[instructionExecutionDocument](reader, "instruction execution")
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
	if document.DeviceSequence == nil || *document.DeviceSequence < 1 {
		return none, malformed("deviceSequence must be a positive integer")
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	instructionID, err := requiredText("instructionId", document.InstructionID, domain.NewInstructionID)
	if err != nil {
		return none, err
	}
	if document.DoneQuantity == nil {
		return none, malformed("doneQuantity is required")
	}
	if document.Lines == nil || len(*document.Lines) == 0 {
		return none, malformed("lines is required")
	}
	lines := make([]application.InstructionExecutionLine, 0, len(*document.Lines))
	for index, item := range *document.Lines {
		line, err := instructionExecutionLine(index, item)
		if err != nil {
			return none, err
		}
		lines = append(lines, line)
	}
	return application.ExecuteInstructionCommand{
		FactID: factID, OccurredAt: occurredAt, DeviceSequence: *document.DeviceSequence, Warehouse: warehouse,
		InstructionID: instructionID, DoneQuantity: *document.DoneQuantity, Lines: lines,
	}, nil
}

func instructionExecutionLine(index int, document instructionExecutionLineDoc) (application.InstructionExecutionLine, error) {
	path := "lines[" + itoa(index) + "]"
	if document.Direction == nil || (*document.Direction != "IN" && *document.Direction != "OUT") {
		return application.InstructionExecutionLine{}, malformed("%s.direction must be IN or OUT", path)
	}
	direction := domain.StockOut
	if *document.Direction == "IN" {
		direction = domain.StockIn
	}
	location, err := requiredText(path+".locationCode", document.LocationCode, domain.NewLocationCode)
	if err != nil {
		return application.InstructionExecutionLine{}, err
	}
	goods, err := requiredText(path+".goodsCode", document.GoodsCode, domain.NewGoodsCode)
	if err != nil {
		return application.InstructionExecutionLine{}, err
	}
	unit, err := requiredText(path+".unit", document.Unit, domain.NewGoodsUnitName)
	if err != nil {
		return application.InstructionExecutionLine{}, err
	}
	if document.StockState == nil {
		return application.InstructionExecutionLine{}, malformed("%s.stockState is required", path)
	}
	state, err := domain.NewStockState(*document.StockState)
	if err != nil {
		return application.InstructionExecutionLine{}, malformed("%s.stockState is not a stock state", path)
	}
	if document.Quantity == nil {
		return application.InstructionExecutionLine{}, malformed("%s.quantity is required", path)
	}
	alias, err := optionalDomain(path+".labelAlias", document.LabelAlias, domain.NewGoodsAliasCode)
	if err != nil {
		return application.InstructionExecutionLine{}, err
	}
	batch, err := optionalDomain(path+".batchNumber", document.BatchNumber, domain.NewBatchNumber)
	if err != nil {
		return application.InstructionExecutionLine{}, err
	}
	var expiry domain.ExpiryDate
	if document.ExpiryDate != nil && *document.ExpiryDate != "" {
		expiry, err = domain.NewExpiryDate(*document.ExpiryDate)
		if err != nil {
			return application.InstructionExecutionLine{}, malformed("%s.expiryDate must be a calendar date", path)
		}
	}
	named, err := optionalDomain(path+".inboundBatchId", document.InboundBatchID, domain.NewInboundBatchID)
	if err != nil {
		return application.InstructionExecutionLine{}, err
	}
	return application.InstructionExecutionLine{
		Direction: direction, Location: location, Goods: goods, Unit: unit, State: state, Alias: alias,
		Batch: batch, Expiry: expiry, NamedBatch: named, Quantity: *document.Quantity,
	}, nil
}

type unboxDocument struct {
	carriedIdentity
	DeviceID       []byte          `json:"deviceId"`
	Operator       []byte          `json:"operator"`
	FactID         *string         `json:"factId"`
	OccurredAt     *string         `json:"occurredAt"`
	DeviceSequence *int64          `json:"deviceSequence"`
	Warehouse      *string         `json:"warehouse"`
	OwnerAccount   *string         `json:"ownerAccount"`
	CartonNumber   *string         `json:"cartonNumber"`
	Lines          *[]unboxLineDoc `json:"lines"`
}

type unboxLineDoc struct {
	Observed     *string `json:"observed"`
	GoodsCode    *string `json:"goodsCode"`
	Unit         *string `json:"unit"`
	StockState   *string `json:"stockState"`
	Quantity     *int64  `json:"quantity"`
	LocationCode *string `json:"locationCode"`
	LabelAlias   *string `json:"labelAlias"`
	BatchNumber  *string `json:"batchNumber"`
	ExpiryDate   *string `json:"expiryDate"`
}

// UnboxCommand 把开箱译成命令。实点可以没有行。
func UnboxCommand(reader io.Reader) (application.UnboxCommand, error) {
	var none application.UnboxCommand
	document, err := decodeDocument[unboxDocument](reader, "unbox")
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
	if document.DeviceSequence == nil || *document.DeviceSequence < 1 {
		return none, malformed("deviceSequence must be a positive integer")
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	owner, err := requiredText("ownerAccount", document.OwnerAccount, domain.NewOwnerAccountID)
	if err != nil {
		return none, err
	}
	carton, err := requiredText("cartonNumber", document.CartonNumber, domain.NewCartonNumber)
	if err != nil {
		return none, err
	}
	var lines []ports.UnboxLine
	if document.Lines != nil {
		lines = make([]ports.UnboxLine, 0, len(*document.Lines))
		for index, item := range *document.Lines {
			line, err := unboxLine(index, item)
			if err != nil {
				return none, err
			}
			lines = append(lines, line)
		}
	}
	return application.UnboxCommand{
		FactID: factID, OccurredAt: occurredAt, DeviceSequence: *document.DeviceSequence, Warehouse: warehouse,
		Owner: owner, Carton: carton, Lines: lines,
	}, nil
}

func unboxLine(index int, document unboxLineDoc) (ports.UnboxLine, error) {
	path := "lines[" + itoa(index) + "]"
	if document.Quantity == nil || *document.Quantity <= 0 {
		return ports.UnboxLine{}, malformed("%s.quantity must be a positive integer", path)
	}
	goods, err := optionalDomain(path+".goodsCode", document.GoodsCode, domain.NewGoodsCode)
	if err != nil {
		return ports.UnboxLine{}, err
	}
	unit, err := optionalDomain(path+".unit", document.Unit, domain.NewGoodsUnitName)
	if err != nil {
		return ports.UnboxLine{}, err
	}
	var state domain.StockState
	if document.StockState != nil && *document.StockState != "" {
		state, err = domain.NewStockState(*document.StockState)
		if err != nil {
			return ports.UnboxLine{}, malformed("%s.stockState is not a stock state", path)
		}
	}
	location, err := optionalDomain(path+".locationCode", document.LocationCode, domain.NewLocationCode)
	if err != nil {
		return ports.UnboxLine{}, err
	}
	alias, err := optionalDomain(path+".labelAlias", document.LabelAlias, domain.NewGoodsAliasCode)
	if err != nil {
		return ports.UnboxLine{}, err
	}
	batch, err := optionalDomain(path+".batchNumber", document.BatchNumber, domain.NewBatchNumber)
	if err != nil {
		return ports.UnboxLine{}, err
	}
	var expiry domain.ExpiryDate
	if document.ExpiryDate != nil && *document.ExpiryDate != "" {
		expiry, err = domain.NewExpiryDate(*document.ExpiryDate)
		if err != nil {
			return ports.UnboxLine{}, malformed("%s.expiryDate must be a calendar date", path)
		}
	}
	observed := ""
	if document.Observed != nil {
		observed = *document.Observed
	}
	return ports.UnboxLine{
		Observed: observed, Goods: goods, Unit: unit, State: state, Alias: alias, Batch: batch, Expiry: expiry,
		Location: location, Quantity: *document.Quantity,
	}, nil
}
