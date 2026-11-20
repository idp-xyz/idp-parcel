package registrationjson

import (
	"io"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

type outboundLineDocument struct {
	GoodsCode  *string `json:"goodsCode"`
	Unit       *string `json:"unit"`
	StockState *string `json:"stockState"`
	Quantity   *int64  `json:"quantity"`
}

type ownerOutboundDocument struct {
	carriedIdentity
	refusedOwner
	AccessRequestID *string                 `json:"accessRequestId"`
	Warehouse       *string                 `json:"warehouse"`
	Dispatch        *string                 `json:"dispatch"`
	Service         *string                 `json:"service"`
	Recipient       *string                 `json:"recipient"`
	Address         *string                 `json:"address"`
	Lines           *[]outboundLineDocument `json:"lines"`
}

// OwnerOutboundFields 是货主口译出的出库请求。货主与来源请求键由信封填。
type OwnerOutboundFields struct {
	AccessRequestID string
	Warehouse       domain.WarehouseCode
	Dispatch        domain.DispatchMethod
	Service         string
	Recipient       string
	Address         string
	Lines           []domain.OutboundLine
}

// ReadOwnerOutbound 把货主提交的出库请求译成字段。
func ReadOwnerOutbound(reader io.Reader) (OwnerOutboundFields, error) {
	var none OwnerOutboundFields
	document, err := decodeDocument[ownerOutboundDocument](reader, "outbound request")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if len(document.OwnerAccount) > 0 {
		return none, malformed("ownerAccount must not be carried in the payload")
	}
	fields, err := outboundFields(document.Warehouse, document.Dispatch, document.Service, document.Recipient, document.Address, document.Lines)
	if err != nil {
		return none, err
	}
	fields.AccessRequestID = rawAccessRequestID(document.AccessRequestID)
	return fields, nil
}

type operatorOutboundDocument struct {
	carriedIdentity
	DecisionID   *string                 `json:"decisionId"`
	OwnerAccount *string                 `json:"ownerAccount"`
	Basis        *string                 `json:"basis"`
	Warehouse    *string                 `json:"warehouse"`
	Dispatch     *string                 `json:"dispatch"`
	Service      *string                 `json:"service"`
	Recipient    *string                 `json:"recipient"`
	Address      *string                 `json:"address"`
	Lines        *[]outboundLineDocument `json:"lines"`
}

// OperatorOutboundCommand 把代录的出库请求译成命令。租户与操作者由 Intake 填。
func OperatorOutboundCommand(reader io.Reader) (application.AcceptOutboundCommand, error) {
	var none application.AcceptOutboundCommand
	document, err := decodeDocument[operatorOutboundDocument](reader, "outbound request")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	key, err := requiredText("decisionId", document.DecisionID, domain.NewRequestKey)
	if err != nil {
		return none, err
	}
	owner, err := requiredText("ownerAccount", document.OwnerAccount, domain.NewOwnerAccountID)
	if err != nil {
		return none, err
	}
	if document.Basis == nil || *document.Basis == "" {
		return none, malformed("basis is required")
	}
	fields, err := outboundFields(document.Warehouse, document.Dispatch, document.Service, document.Recipient, document.Address, document.Lines)
	if err != nil {
		return none, err
	}
	return application.AcceptOutboundCommand{
		Owner: owner, Warehouse: fields.Warehouse, Origin: domain.OutboundOriginOperator, Key: key,
		Dispatch: fields.Dispatch, Service: fields.Service, Recipient: fields.Recipient, Address: fields.Address,
		Basis: *document.Basis, Lines: fields.Lines,
	}, nil
}

func outboundFields(warehouse, dispatch, service, recipient, address *string, lines *[]outboundLineDocument) (OwnerOutboundFields, error) {
	var none OwnerOutboundFields
	parsedWarehouse, err := requiredText("warehouse", warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	if dispatch == nil {
		return none, malformed("dispatch is required")
	}
	parsedDispatch, err := domain.NewDispatchMethod(*dispatch)
	if err != nil {
		return none, malformed("dispatch is not a dispatch method")
	}
	serviceText := ""
	if service != nil {
		serviceText = *service
	}
	if parsedDispatch == domain.DispatchProductNetwork && serviceText == "" {
		return none, malformed("service is required")
	}
	if recipient == nil || *recipient == "" || address == nil || *address == "" {
		return none, malformed("recipient and address are required")
	}
	if lines == nil || len(*lines) == 0 {
		return none, malformed("lines is required")
	}
	parsedLines := make([]domain.OutboundLine, 0, len(*lines))
	for _, item := range *lines {
		goods, err := requiredText("goodsCode", item.GoodsCode, domain.NewGoodsCode)
		if err != nil {
			return none, err
		}
		unit, err := requiredText("unit", item.Unit, domain.NewGoodsUnitName)
		if err != nil {
			return none, err
		}
		if item.StockState == nil {
			return none, malformed("stockState is required")
		}
		state, err := domain.NewStockState(*item.StockState)
		if err != nil {
			return none, malformed("stockState is not a stock state")
		}
		if item.Quantity == nil || *item.Quantity <= 0 {
			return none, malformed("quantity must be a positive integer")
		}
		line, err := domain.NewOutboundLine(domain.OutboundLineSpec{Goods: goods, Unit: unit, State: state, Quantity: *item.Quantity})
		if err != nil {
			return none, err
		}
		parsedLines = append(parsedLines, line)
	}
	return OwnerOutboundFields{
		Warehouse: parsedWarehouse, Dispatch: parsedDispatch, Service: serviceText,
		Recipient: *recipient, Address: *address, Lines: parsedLines,
	}, nil
}

type shortagePolicyDocument struct {
	carriedIdentity
	OwnerAccount *string `json:"ownerAccount"`
	Revision     *int    `json:"revision"`
	Kind         *string `json:"kind"`
}

// ShortagePolicyCommand 把缺货处置译成命令。租户与登记人由 Intake 填。
func ShortagePolicyCommand(reader io.Reader, tenant domain.TenantID, operator domain.Registrant) (application.RegisterShortagePolicyCommand, error) {
	var none application.RegisterShortagePolicyCommand
	document, err := decodeDocument[shortagePolicyDocument](reader, "shortage policy")
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
	if document.Revision == nil || *document.Revision < 1 {
		return none, malformed("revision must be a positive integer")
	}
	if document.Kind == nil {
		return none, malformed("kind is required")
	}
	kind, err := domain.NewShortagePolicyKind(*document.Kind)
	if err != nil {
		return none, malformed("kind must be PARTIAL or REJECT")
	}
	return application.RegisterShortagePolicyCommand{Tenant: tenant, Owner: owner, Revision: *document.Revision, Kind: kind, Operator: operator}, nil
}
