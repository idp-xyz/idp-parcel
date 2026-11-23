package registrationjson

import (
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

type returnLineDocument struct {
	GoodsCode *string `json:"goodsCode"`
	Unit      *string `json:"unit"`
	Quantity  *int64  `json:"quantity"`
}

func returnLines(documents *[]returnLineDocument) ([]ports.ReturnLine, error) {
	if documents == nil || len(*documents) == 0 {
		return nil, malformed("lines is required")
	}
	lines := make([]ports.ReturnLine, 0, len(*documents))
	for index, document := range *documents {
		goods, err := requiredText("lines.goodsCode", document.GoodsCode, domain.NewGoodsCode)
		if err != nil {
			return nil, err
		}
		unit, err := requiredText("lines.unit", document.Unit, domain.NewGoodsUnitName)
		if err != nil {
			return nil, err
		}
		if document.Quantity == nil || *document.Quantity < 1 {
			return nil, malformed("lines.quantity is required")
		}
		_ = index
		lines = append(lines, ports.ReturnLine{Goods: goods, Unit: unit, Quantity: *document.Quantity})
	}
	return lines, nil
}

type ownerReturnAdviceDocument struct {
	carriedIdentity
	refusedOwner
	AccessRequestID *string               `json:"accessRequestId"`
	Warehouse       *string               `json:"warehouse"`
	TrackingNumber  *string               `json:"trackingNumber"`
	Lines           *[]returnLineDocument `json:"lines"`
}

// ReadOwnerReturnAdvice 把货主提交的退货预报译成命令。货主与来源请求键由信封填。
func ReadOwnerReturnAdvice(reader io.Reader) (string, application.AcceptReturnCommand, error) {
	var none application.AcceptReturnCommand
	document, err := decodeDocument[ownerReturnAdviceDocument](reader, "return pre-advice")
	if err != nil {
		return "", none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return "", none, err
	}
	if err := document.refuseOwner(); err != nil {
		return "", none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return "", none, err
	}
	tracking, err := requiredText("trackingNumber", document.TrackingNumber, func(value string) (string, error) {
		if value == "" {
			return "", malformed("trackingNumber is required")
		}
		return value, nil
	})
	if err != nil {
		return "", none, err
	}
	lines, err := returnLines(document.Lines)
	if err != nil {
		return "", none, err
	}
	return rawAccessRequestID(document.AccessRequestID), application.AcceptReturnCommand{
		Warehouse: warehouse, Origin: domain.ReturnOriginCustomer, Tracking: tracking, Lines: lines,
	}, nil
}

type operatorReturnAdviceDocument struct {
	carriedIdentity
	OwnerAccount   *string               `json:"ownerAccount"`
	Basis          *string               `json:"basis"`
	Warehouse      *string               `json:"warehouse"`
	TrackingNumber *string               `json:"trackingNumber"`
	Lines          *[]returnLineDocument `json:"lines"`
}

// OperatorReturnAdvice 把代录的退货预报译成命令。来源请求键用依据引用。
func OperatorReturnAdvice(reader io.Reader) (application.AcceptReturnCommand, error) {
	var none application.AcceptReturnCommand
	document, err := decodeDocument[operatorReturnAdviceDocument](reader, "return pre-advice")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	owner, err := requiredText("ownerAccount", document.OwnerAccount, domain.NewOwnerAccountID)
	if err != nil {
		return none, err
	}
	basis, err := requiredText("basis", document.Basis, func(value string) (string, error) {
		if value == "" {
			return "", malformed("basis is required")
		}
		return value, nil
	})
	if err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	tracking, err := requiredText("trackingNumber", document.TrackingNumber, func(value string) (string, error) {
		if value == "" {
			return "", malformed("trackingNumber is required")
		}
		return value, nil
	})
	if err != nil {
		return none, err
	}
	lines, err := returnLines(document.Lines)
	if err != nil {
		return none, err
	}
	return application.AcceptReturnCommand{
		Owner: owner, Warehouse: warehouse, Origin: domain.ReturnOriginOperator, Key: basis, Tracking: tracking, Basis: basis, Lines: lines,
	}, nil
}

type ownerReturnDispositionDocument struct {
	carriedIdentity
	refusedOwner
	AccessRequestID *string `json:"accessRequestId"`
	FactID          *string `json:"factId"`
	Destination     *string `json:"destination"`
}

// ReadOwnerReturnDisposition 把货主的回库处置译成命令。决定标识用来源请求键。
func ReadOwnerReturnDisposition(reader io.Reader) (string, application.DisposeReturnCommand, error) {
	var none application.DisposeReturnCommand
	document, err := decodeDocument[ownerReturnDispositionDocument](reader, "return disposition")
	if err != nil {
		return "", none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return "", none, err
	}
	if err := document.refuseOwner(); err != nil {
		return "", none, err
	}
	fact, err := requiredText("factId", document.FactID, domain.NewReturnFactID)
	if err != nil {
		return "", none, err
	}
	destination, err := requiredText("destination", document.Destination, domain.NewReturnDestination)
	if err != nil {
		return "", none, err
	}
	return rawAccessRequestID(document.AccessRequestID), application.DisposeReturnCommand{
		Fact: fact, Destination: destination, Origin: domain.ReturnOriginCustomer,
	}, nil
}

type operatorReturnDispositionDocument struct {
	carriedIdentity
	refusedOwner
	Basis       *string `json:"basis"`
	FactID      *string `json:"factId"`
	Destination *string `json:"destination"`
}

// OperatorReturnDisposition 把代录的回库处置译成命令。货主取自到仓记录，载荷里再带即坏载荷。
func OperatorReturnDisposition(reader io.Reader) (application.DisposeReturnCommand, error) {
	var none application.DisposeReturnCommand
	document, err := decodeDocument[operatorReturnDispositionDocument](reader, "return disposition")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if err := document.refuseOwner(); err != nil {
		return none, err
	}
	basis, err := requiredText("basis", document.Basis, func(value string) (string, error) {
		if value == "" {
			return "", malformed("basis is required")
		}
		return value, nil
	})
	if err != nil {
		return none, err
	}
	fact, err := requiredText("factId", document.FactID, domain.NewReturnFactID)
	if err != nil {
		return none, err
	}
	destination, err := requiredText("destination", document.Destination, domain.NewReturnDestination)
	if err != nil {
		return none, err
	}
	decision, err := domain.NewReturnDecisionID(basis)
	if err != nil {
		return none, err
	}
	return application.DisposeReturnCommand{
		Decision: decision, Fact: fact, Destination: destination, Origin: domain.ReturnOriginOperator, Basis: basis,
	}, nil
}

type returnReceiptDocument struct {
	carriedIdentity
	FactID         *string `json:"factId"`
	Warehouse      *string `json:"warehouse"`
	Location       *string `json:"location"`
	TrackingNumber *string `json:"trackingNumber"`
	GoodsCode      *string `json:"goodsCode"`
	Unit           *string `json:"unit"`
	StockState     *string `json:"stockState"`
	Quantity       *int64  `json:"quantity"`
	OccurredAt     *string `json:"occurredAt"`
	ArrivalID      *string `json:"arrivalId"`
}

// ReturnReceiptCommand 把退货入库事实译成命令。租户、操作者与设备由作业事实信封填。
func ReturnReceiptCommand(reader io.Reader) (application.ReceiveReturnCommand, error) {
	var none application.ReceiveReturnCommand
	document, err := decodeDocument[returnReceiptDocument](reader, "return receipt")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	fact, err := requiredText("factId", document.FactID, domain.NewReturnFactID)
	if err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	location, err := requiredText("location", document.Location, domain.NewLocationCode)
	if err != nil {
		return none, err
	}
	tracking, err := requiredText("trackingNumber", document.TrackingNumber, func(value string) (string, error) {
		if value == "" {
			return "", malformed("trackingNumber is required")
		}
		return value, nil
	})
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
	state, err := requiredText("stockState", document.StockState, domain.NewStockState)
	if err != nil {
		return none, err
	}
	if document.Quantity == nil || *document.Quantity < 1 {
		return none, malformed("quantity is required")
	}
	occurred, err := optionalConfirmInstant("occurredAt", document.OccurredAt)
	if err != nil {
		return none, err
	}
	var arrival domain.UnclaimedArrivalID
	if document.ArrivalID != nil && *document.ArrivalID != "" {
		arrival, err = domain.NewUnclaimedArrivalID(*document.ArrivalID)
		if err != nil {
			return none, malformed("arrivalId must not be blank")
		}
	}
	return application.ReceiveReturnCommand{
		FactID: fact, Warehouse: warehouse, Location: location, Tracking: tracking, Goods: goods, Unit: unit,
		State: state, Quantity: *document.Quantity, OccurredAt: occurred, Arrival: arrival,
	}, nil
}
