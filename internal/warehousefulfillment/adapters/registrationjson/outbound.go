package registrationjson

import (
	"io"
	"strings"
	"time"

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
	AccessRequestID  *string                 `json:"accessRequestId"`
	Warehouse        *string                 `json:"warehouse"`
	Dispatch         *string                 `json:"dispatch"`
	Service          *string                 `json:"service"`
	Recipient        *string                 `json:"recipient"`
	Address          *string                 `json:"address"`
	Lines            *[]outboundLineDocument `json:"lines"`
	PlatformStocking *bool                   `json:"platformStocking"`
	PartialShipment  *string                 `json:"partialShipment"`
}

// OwnerOutboundFields 是货主口译出的出库请求。货主与来源请求键由信封填。
type OwnerOutboundFields struct {
	AccessRequestID  string
	Warehouse        domain.WarehouseCode
	Dispatch         domain.DispatchMethod
	Service          string
	Recipient        string
	Address          string
	Lines            []domain.OutboundLine
	PlatformStocking bool
	PartialShipment  domain.PartialShipment
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
	fields.PlatformStocking = markedPlatformStocking(document.PlatformStocking)
	fields.PartialShipment, err = optionalPartialShipment(document.PartialShipment)
	if err != nil {
		return none, err
	}
	return fields, nil
}

type operatorOutboundDocument struct {
	carriedIdentity
	DecisionID       *string                 `json:"decisionId"`
	OwnerAccount     *string                 `json:"ownerAccount"`
	Basis            *string                 `json:"basis"`
	Warehouse        *string                 `json:"warehouse"`
	Dispatch         *string                 `json:"dispatch"`
	Service          *string                 `json:"service"`
	Recipient        *string                 `json:"recipient"`
	Address          *string                 `json:"address"`
	Lines            *[]outboundLineDocument `json:"lines"`
	PlatformStocking *bool                   `json:"platformStocking"`
	PartialShipment  *string                 `json:"partialShipment"`
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
	partial, err := optionalPartialShipment(document.PartialShipment)
	if err != nil {
		return none, err
	}
	return application.AcceptOutboundCommand{
		Owner: owner, Warehouse: fields.Warehouse, Origin: domain.OutboundOriginOperator, Key: key,
		Dispatch: fields.Dispatch, Service: fields.Service, Recipient: fields.Recipient, Address: fields.Address,
		Basis: *document.Basis, Lines: fields.Lines, PlatformStocking: markedPlatformStocking(document.PlatformStocking),
		PartialShipment: partial,
	}, nil
}

func markedPlatformStocking(value *bool) bool {
	return value != nil && *value
}

// optionalPartialShipment 读部分发货这一格：不带是没写明；带了就得是三个词之一，空串也不收——
// 没写明的写法是不带这一格，不是带一个空的。
func optionalPartialShipment(value *string) (domain.PartialShipment, error) {
	if value == nil {
		return domain.PartialShipmentUnset, nil
	}
	rule, err := domain.NewPartialShipment(*value)
	if err != nil {
		return domain.PartialShipmentUnset, malformed("partialShipment must be SHIP_COMPLETE, SHORT_SHIP or SPLIT_SHIP")
	}
	return rule, nil
}

type partialShipmentConsentDocument struct {
	carriedIdentity
	refusedOwner
	RequestID       *string `json:"requestId"`
	PartialShipment *string `json:"partialShipment"`
	Basis           *string `json:"basis"`
}

// OperatorPartialShipmentConsent 把代录的部分发货同意译成命令。租户与操作者由 Intake 填。
func OperatorPartialShipmentConsent(reader io.Reader) (application.RecordPartialShipmentConsentCommand, error) {
	var none application.RecordPartialShipmentConsentCommand
	document, err := decodeDocument[partialShipmentConsentDocument](reader, "partial shipment consent")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if len(document.OwnerAccount) > 0 {
		return none, malformed("ownerAccount must not be carried in the payload")
	}
	request, err := requiredText("requestId", document.RequestID, domain.NewOutboundRequestID)
	if err != nil {
		return none, err
	}
	if document.PartialShipment == nil {
		return none, malformed("partialShipment is required")
	}
	rule, err := optionalPartialShipment(document.PartialShipment)
	if err != nil {
		return none, err
	}
	if document.Basis == nil || strings.TrimSpace(*document.Basis) == "" {
		return none, malformed("basis is required")
	}
	return application.RecordPartialShipmentConsentCommand{Request: request, Rule: rule, Basis: strings.TrimSpace(*document.Basis)}, nil
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
	// PartialShipment 是拣不齐时怎么发的默认，与 Kind 各说各的；两格至少带一格。
	PartialShipment *string `json:"partialShipment"`
}

type outboundSplitDocument struct {
	carriedIdentity
	refusedOwner
	AccessRequestID *string              `json:"accessRequestId"`
	RequestID       *string              `json:"requestId"`
	Parts           *[]outboundSplitPart `json:"parts"`
}

type outboundSplitPart struct {
	RequestKey *string                 `json:"requestKey"`
	Lines      *[]outboundLineDocument `json:"lines"`
}

// ReadOwnerOutboundSplit 把货主拆单译成命令。货主由信封填。
func ReadOwnerOutboundSplit(reader io.Reader) (string, application.SplitOutboundCommand, error) {
	var none application.SplitOutboundCommand
	document, err := decodeDocument[outboundSplitDocument](reader, "outbound split")
	if err != nil {
		return "", none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return "", none, err
	}
	if len(document.OwnerAccount) > 0 {
		return "", none, malformed("ownerAccount must not be carried in the payload")
	}
	command, err := splitCommand(document.RequestID, document.Parts, false)
	if err != nil {
		return "", none, err
	}
	return rawAccessRequestID(document.AccessRequestID), command, nil
}

type outboundSupplementDocument struct {
	carriedIdentity
	refusedOwner
	AccessRequestID *string `json:"accessRequestId"`
	RequestID       *string `json:"requestId"`
	ParcelSequence  *int    `json:"parcelSequence"`
}

// ReadOwnerOutboundSupplement 把货主补正译成命令。货主与请求键由信封填。
func ReadOwnerOutboundSupplement(reader io.Reader) (string, application.ResubmitCommand, error) {
	var none application.ResubmitCommand
	document, err := decodeDocument[outboundSupplementDocument](reader, "outbound supplement")
	if err != nil {
		return "", none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return "", none, err
	}
	if len(document.OwnerAccount) > 0 {
		return "", none, malformed("ownerAccount must not be carried in the payload")
	}
	if document.RequestID == nil {
		return "", none, malformed("requestId is required")
	}
	requestID, err := domain.NewOutboundRequestID(strings.TrimSpace(*document.RequestID))
	if err != nil {
		return "", none, malformed("requestId is required")
	}
	if document.ParcelSequence == nil || *document.ParcelSequence < 1 {
		return "", none, malformed("parcelSequence must be a positive integer")
	}
	return rawAccessRequestID(document.AccessRequestID), application.ResubmitCommand{
		Request: requestID, Sequence: *document.ParcelSequence,
	}, nil
}

type operatorSplitDocument struct {
	carriedIdentity
	refusedOwner
	RequestID *string              `json:"requestId"`
	Basis     *string              `json:"basis"`
	Parts     *[]outboundSplitPart `json:"parts"`
}

// OperatorOutboundSplit 把代录拆单译成命令。货主取自被拆的请求，载荷里再带货主即坏载荷。
func OperatorOutboundSplit(reader io.Reader) (application.SplitOutboundCommand, error) {
	document, err := decodeDocument[operatorSplitDocument](reader, "outbound split")
	if err != nil {
		return application.SplitOutboundCommand{}, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return application.SplitOutboundCommand{}, err
	}
	if len(document.OwnerAccount) > 0 {
		return application.SplitOutboundCommand{}, malformed("ownerAccount must not be carried in the payload")
	}
	if document.Basis == nil || *document.Basis == "" {
		return application.SplitOutboundCommand{}, malformed("basis is required")
	}
	command, err := splitCommand(document.RequestID, document.Parts, true)
	if err != nil {
		return application.SplitOutboundCommand{}, err
	}
	command.Basis = *document.Basis
	command.Origin = domain.OutboundOriginOperator
	return command, nil
}

func splitCommand(requestID *string, parts *[]outboundSplitPart, operator bool) (application.SplitOutboundCommand, error) {
	var none application.SplitOutboundCommand
	parent, err := requiredText("requestId", requestID, domain.NewOutboundRequestID)
	if err != nil {
		return none, err
	}
	if parts == nil || len(*parts) < 2 {
		return none, malformed("parts is required")
	}
	parsed := make([]application.SplitOutboundPart, 0, len(*parts))
	for _, part := range *parts {
		key, err := requiredText("requestKey", part.RequestKey, domain.NewRequestKey)
		if err != nil {
			return none, err
		}
		lines, err := splitLines(part.Lines)
		if err != nil {
			return none, err
		}
		parsed = append(parsed, application.SplitOutboundPart{Key: key, Lines: lines})
	}
	origin := domain.OutboundOriginCustomer
	if operator {
		origin = domain.OutboundOriginOperator
	}
	return application.SplitOutboundCommand{Origin: origin, Parent: parent, Parts: parsed}, nil
}

func splitLines(lines *[]outboundLineDocument) ([]domain.OutboundLine, error) {
	fields, err := outboundFields(stringPtr("SYN-WH-01"), stringPtr("PRODUCT_NETWORK"), stringPtr("SYN-SERVICE"), stringPtr("收件人"), stringPtr("地址"), lines)
	if err != nil {
		return nil, err
	}
	return fields.Lines, nil
}

func stringPtr(value string) *string { return &value }

type allocationStrategyDocument struct {
	carriedIdentity
	Revision *int `json:"revision"`
}

// AllocationStrategyCommand 把采用参考分配策略译成命令。
func AllocationStrategyCommand(reader io.Reader, tenant domain.TenantID, operator domain.Registrant) (application.AdoptStrategyCommand, error) {
	var none application.AdoptStrategyCommand
	document, err := decodeDocument[allocationStrategyDocument](reader, "allocation strategy")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if document.Revision == nil || *document.Revision < 1 {
		return none, malformed("revision must be a positive integer")
	}
	return application.AdoptStrategyCommand{Tenant: tenant, Revision: *document.Revision, Operator: operator}, nil
}

type waveDocument struct {
	carriedIdentity
	WaveID    *string   `json:"waveId"`
	Warehouse *string   `json:"warehouse"`
	Requests  *[]string `json:"requests"`
	Prefetch  *bool     `json:"prefetch"`
	Packaging *string   `json:"packagingMaterial"`
}

// ReleaseWaveCommand 把放波次译成命令。
func ReleaseWaveCommand(reader io.Reader) (application.ReleaseWaveCommand, error) {
	var none application.ReleaseWaveCommand
	document, err := decodeDocument[waveDocument](reader, "wave")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	id, err := requiredText("waveId", document.WaveID, domain.NewWaveID)
	if err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	if document.Requests == nil || len(*document.Requests) == 0 {
		return none, malformed("requests is required")
	}
	requests := make([]domain.OutboundRequestID, 0, len(*document.Requests))
	for _, raw := range *document.Requests {
		request, err := domain.NewOutboundRequestID(raw)
		if err != nil {
			return none, malformed("requests must not be blank")
		}
		requests = append(requests, request)
	}
	prefetch := false
	if document.Prefetch != nil {
		prefetch = *document.Prefetch
	}
	var packaging domain.PackagingMaterialCode
	if document.Packaging != nil && *document.Packaging != "" {
		packaging, err = domain.NewPackagingMaterialCode(*document.Packaging)
		if err != nil {
			return none, malformed("packagingMaterial is not a code")
		}
	}
	return application.ReleaseWaveCommand{ID: id, Warehouse: warehouse, Requests: requests, Prefetch: prefetch, Packaging: packaging}, nil
}

type waveWithdrawalDocument struct {
	carriedIdentity
	WaveID   *string   `json:"waveId"`
	Requests *[]string `json:"requests"`
}

// WithdrawWaveCommand 把放出后撤单译成命令。
func WithdrawWaveCommand(reader io.Reader) (application.WithdrawWaveCommand, error) {
	var none application.WithdrawWaveCommand
	document, err := decodeDocument[waveWithdrawalDocument](reader, "wave withdrawal")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	id, err := requiredText("waveId", document.WaveID, domain.NewWaveID)
	if err != nil {
		return none, err
	}
	if document.Requests == nil || len(*document.Requests) == 0 {
		return none, malformed("requests is required")
	}
	requests := make([]domain.OutboundRequestID, 0, len(*document.Requests))
	for _, raw := range *document.Requests {
		request, err := domain.NewOutboundRequestID(raw)
		if err != nil {
			return none, malformed("requests must not be blank")
		}
		requests = append(requests, request)
	}
	return application.WithdrawWaveCommand{ID: id, Requests: requests}, nil
}

type shortPickDisposalDocument struct {
	carriedIdentity
	RequestID *string `json:"requestId"`
}

// ShipPickedCommand 把按已拣数量发货译成命令。载荷只点名请求：能不能少发照货主的规则，不由这一次提交说。
func ShipPickedCommand(reader io.Reader) (application.ShipPickedCommand, error) {
	var none application.ShipPickedCommand
	document, err := decodeDocument[shortPickDisposalDocument](reader, "short shipment")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	request, err := requiredText("requestId", document.RequestID, domain.NewOutboundRequestID)
	if err != nil {
		return none, err
	}
	return application.ShipPickedCommand{Request: request}, nil
}

// SplitShortageCommand 把拆出缺货部分译成命令。载荷只点名请求：拆多少由没拣到的预占定。
func SplitShortageCommand(reader io.Reader) (application.SplitShortageCommand, error) {
	var none application.SplitShortageCommand
	document, err := decodeDocument[shortPickDisposalDocument](reader, "shortage split")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	request, err := requiredText("requestId", document.RequestID, domain.NewOutboundRequestID)
	if err != nil {
		return none, err
	}
	return application.SplitShortageCommand{Request: request}, nil
}

type pickDocument struct {
	carriedIdentity
	FactID    *string `json:"factId"`
	TaskID    *string `json:"taskId"`
	Warehouse *string `json:"warehouse"`
	Quantity  *int64  `json:"quantity"`
	Target    *string `json:"targetLocation"`
}

// PickCommand 把拣货事实译成命令。
func PickCommand(reader io.Reader) (application.PickCommand, error) {
	var none application.PickCommand
	document, err := decodeDocument[pickDocument](reader, "pick")
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
	task, err := requiredText("taskId", document.TaskID, domain.NewPickTaskID)
	if err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	if document.Quantity == nil || *document.Quantity < 1 {
		return none, malformed("quantity must be a positive integer")
	}
	target, err := requiredText("targetLocation", document.Target, domain.NewLocationCode)
	if err != nil {
		return none, err
	}
	return application.PickCommand{FactID: fact, TaskID: task, Warehouse: warehouse, Quantity: *document.Quantity, Target: target}, nil
}

type checkDocument struct {
	carriedIdentity
	FactID    *string `json:"factId"`
	RequestID *string `json:"requestId"`
	Warehouse *string `json:"warehouse"`
	Observed  *int64  `json:"observed"`
}

// CheckCommand 把出库复核译成命令。
func CheckCommand(reader io.Reader) (application.CheckCommand, error) {
	var none application.CheckCommand
	document, err := decodeDocument[checkDocument](reader, "outbound check")
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
	request, err := requiredText("requestId", document.RequestID, domain.NewOutboundRequestID)
	if err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	if document.Observed == nil || *document.Observed < 0 {
		return none, malformed("observed must not be negative")
	}
	return application.CheckCommand{FactID: fact, Request: request, Warehouse: warehouse, Observed: *document.Observed}, nil
}

type packLineDocument struct {
	Sequence   *int    `json:"sequence"`
	Packaging  *string `json:"packagingMaterial"`
	Weight     *string `json:"weight"`
	WeightUnit *string `json:"weightUnit"`
	Length     *string `json:"length"`
	Width      *string `json:"width"`
	Height     *string `json:"height"`
	Dimension  *string `json:"dimensionUnit"`
	GoodsCode  *string `json:"goodsCode"`
	Unit       *string `json:"unit"`
	Quantity   *int64  `json:"quantity"`
}

type packDocument struct {
	carriedIdentity
	FactID    *string             `json:"factId"`
	RequestID *string             `json:"requestId"`
	Warehouse *string             `json:"warehouse"`
	Parcels   *[]packLineDocument `json:"parcels"`
}

// PackCommand 把打包事实译成命令。包材只记下文本，不查册。
func PackCommand(reader io.Reader) (application.PackCommand, error) {
	var none application.PackCommand
	document, err := decodeDocument[packDocument](reader, "pack")
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
	request, err := requiredText("requestId", document.RequestID, domain.NewOutboundRequestID)
	if err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	if document.Parcels == nil || len(*document.Parcels) == 0 {
		return none, malformed("parcels is required")
	}
	parcels := make([]application.PackParcelCommand, 0, len(*document.Parcels))
	for _, item := range *document.Parcels {
		if item.Sequence == nil || *item.Sequence < 1 {
			return none, malformed("sequence must be a positive integer")
		}
		if item.Quantity == nil || *item.Quantity < 1 {
			return none, malformed("quantity must be a positive integer")
		}
		goods, err := requiredText("goodsCode", item.GoodsCode, domain.NewGoodsCode)
		if err != nil {
			return none, err
		}
		unit, err := requiredText("unit", item.Unit, domain.NewGoodsUnitName)
		if err != nil {
			return none, err
		}
		text := func(name string, raw *string) (string, error) {
			if raw == nil || *raw == "" {
				return "", malformed("%s is required", name)
			}
			return *raw, nil
		}
		packaging, err := text("packagingMaterial", item.Packaging)
		if err != nil {
			return none, err
		}
		weight, err := text("weight", item.Weight)
		if err != nil {
			return none, err
		}
		weightUnit, err := text("weightUnit", item.WeightUnit)
		if err != nil {
			return none, err
		}
		length, err := text("length", item.Length)
		if err != nil {
			return none, err
		}
		width, err := text("width", item.Width)
		if err != nil {
			return none, err
		}
		height, err := text("height", item.Height)
		if err != nil {
			return none, err
		}
		dimension, err := text("dimensionUnit", item.Dimension)
		if err != nil {
			return none, err
		}
		parcels = append(parcels, application.PackParcelCommand{
			Sequence: *item.Sequence, Packaging: packaging, Weight: weight, WeightUnit: weightUnit,
			Length: length, Width: width, Height: height, Dimension: dimension,
			Goods: goods, Unit: unit, Quantity: *item.Quantity,
		})
	}
	return application.PackCommand{FactID: fact, Request: request, Warehouse: warehouse, Parcels: parcels}, nil
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
	if document.Kind == nil && document.PartialShipment == nil {
		return none, malformed("kind or partialShipment is required")
	}
	kind := domain.ShortagePolicyInvalid
	if document.Kind != nil {
		kind, err = domain.NewShortagePolicyKind(*document.Kind)
		if err != nil {
			return none, malformed("kind must be PARTIAL or REJECT")
		}
	}
	partial, err := optionalPartialShipment(document.PartialShipment)
	if err != nil {
		return none, err
	}
	return application.RegisterShortagePolicyCommand{
		Tenant: tenant, Owner: owner, Revision: *document.Revision, Kind: kind, PartialShipment: partial, Operator: operator,
	}, nil
}

type confirmDocument struct {
	carriedIdentity
	FactID           *string `json:"factId"`
	RequestID        *string `json:"requestId"`
	Warehouse        *string `json:"warehouse"`
	ParcelSequence   *int    `json:"parcelSequence"`
	Offline          *bool   `json:"offline"`
	OccurredAt       *string `json:"occurredAt"`
	ValidFrom        *string `json:"validFrom"`
	ValidUntil       *string `json:"validUntil"`
	ExternalNumber   *string `json:"externalNumber"`
	PickupParty      *string `json:"pickupParty"`
	PickupCredential *string `json:"pickupCredential"`
}

// ConfirmCommand 把出库确认或自提交付译成命令。事实种类由口绑上，不从载荷取。
func ConfirmCommand(reader io.Reader, kind string) (application.ConfirmCommand, error) {
	var none application.ConfirmCommand
	document, err := decodeDocument[confirmDocument](reader, "outbound confirmation")
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
	request, err := requiredText("requestId", document.RequestID, domain.NewOutboundRequestID)
	if err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	if document.ParcelSequence == nil || *document.ParcelSequence < 1 {
		return none, malformed("parcelSequence must be a positive integer")
	}
	occurred, err := requiredConfirmInstant("occurredAt", document.OccurredAt)
	if err != nil {
		return none, err
	}
	from, err := optionalConfirmInstant("validFrom", document.ValidFrom)
	if err != nil {
		return none, err
	}
	until, err := optionalConfirmInstant("validUntil", document.ValidUntil)
	if err != nil {
		return none, err
	}
	offline := document.Offline != nil && *document.Offline
	return application.ConfirmCommand{
		FactID: fact, Request: request, Sequence: *document.ParcelSequence, Warehouse: warehouse, Kind: kind, Offline: offline,
		OccurredAt: occurred, ValidFrom: from, ValidUntil: until,
		ExternalNumber: textOrEmpty(document.ExternalNumber), PickupParty: textOrEmpty(document.PickupParty),
		PickupCredential: textOrEmpty(document.PickupCredential),
	}, nil
}

func requiredConfirmInstant(name string, raw *string) (time.Time, error) {
	if raw == nil || *raw == "" {
		return time.Time{}, malformed("%s is required", name)
	}
	at, err := time.Parse(time.RFC3339, *raw)
	if err != nil {
		return time.Time{}, malformed("%s must be RFC3339", name)
	}
	return at, nil
}

func optionalConfirmInstant(name string, raw *string) (time.Time, error) {
	if raw == nil || *raw == "" {
		return time.Time{}, nil
	}
	return requiredConfirmInstant(name, raw)
}

func textOrEmpty(raw *string) string {
	if raw == nil {
		return ""
	}
	return *raw
}

type ownerLabelDocument struct {
	carriedIdentity
	refusedOwner
	RequestID      *string `json:"requestId"`
	ParcelSequence *int    `json:"parcelSequence"`
	ExternalNumber *string `json:"externalNumber"`
}

// OperatorOwnerLabel 把代录的货主面单译成命令。货主取自出库请求，载荷里再带货主即坏载荷。
func OperatorOwnerLabel(reader io.Reader) (application.RegisterOwnerLabelCommand, error) {
	var none application.RegisterOwnerLabelCommand
	document, err := decodeDocument[ownerLabelDocument](reader, "owner label")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if len(document.OwnerAccount) > 0 {
		return none, malformed("ownerAccount must not be carried in the payload")
	}
	request, err := requiredText("requestId", document.RequestID, domain.NewOutboundRequestID)
	if err != nil {
		return none, err
	}
	if document.ParcelSequence == nil || *document.ParcelSequence < 1 {
		return none, malformed("parcelSequence must be a positive integer")
	}
	if document.ExternalNumber == nil || strings.TrimSpace(*document.ExternalNumber) == "" {
		return none, malformed("externalNumber is required")
	}
	return application.RegisterOwnerLabelCommand{
		Request: request, Sequence: *document.ParcelSequence, ExternalNumber: strings.TrimSpace(*document.ExternalNumber),
	}, nil
}

type correctionDocument struct {
	carriedIdentity
	FactID     *string `json:"factId"`
	OccurredAt *string `json:"occurredAt"`
	Invalidate *bool   `json:"invalidate"`
}

// CorrectConfirmationCommand 把出库确认更正译成命令。改时点与判不成立二选一。
func CorrectConfirmationCommand(reader io.Reader) (application.CorrectConfirmationCommand, error) {
	var none application.CorrectConfirmationCommand
	document, err := decodeDocument[correctionDocument](reader, "outbound confirmation correction")
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
	invalidate := document.Invalidate != nil && *document.Invalidate
	occurred, err := optionalConfirmInstant("occurredAt", document.OccurredAt)
	if err != nil {
		return none, err
	}
	return application.CorrectConfirmationCommand{FactID: fact, OccurredAt: occurred, Invalidate: invalidate}, nil
}

type replenishmentDocument struct {
	carriedIdentity
	FactID *string `json:"factId"`
}

// ApproveReplenishmentCommand 把回补批准译成命令。
func ApproveReplenishmentCommand(reader io.Reader) (application.ApproveReplenishmentCommand, error) {
	var none application.ApproveReplenishmentCommand
	document, err := decodeDocument[replenishmentDocument](reader, "outbound replenishment")
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
	return application.ApproveReplenishmentCommand{FactID: fact}, nil
}

type outboundCancellationDocument struct {
	carriedIdentity
	refusedOwner
	AccessRequestID *string `json:"accessRequestId"`
	RequestID       *string `json:"requestId"`
	ParcelSequence  *int    `json:"parcelSequence"`
}

// ReadOwnerOutboundCancellation 把货主截单译成命令。货主与请求键由信封填。
func ReadOwnerOutboundCancellation(reader io.Reader) (string, application.CutoffCommand, error) {
	var none application.CutoffCommand
	document, err := decodeDocument[outboundCancellationDocument](reader, "outbound cancellation")
	if err != nil {
		return "", none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return "", none, err
	}
	if len(document.OwnerAccount) > 0 {
		return "", none, malformed("ownerAccount must not be carried in the payload")
	}
	request, sequence, err := cutoffTarget(document.RequestID, document.ParcelSequence)
	if err != nil {
		return "", none, err
	}
	return rawAccessRequestID(document.AccessRequestID), application.CutoffCommand{Request: request, Sequence: sequence}, nil
}

type operatorCutoffDocument struct {
	carriedIdentity
	refusedOwner
	RequestID      *string `json:"requestId"`
	ParcelSequence *int    `json:"parcelSequence"`
	Basis          *string `json:"basis"`
}

// OperatorOutboundCutoff 把代录截单译成命令。货主取自被截的请求，依据引用必填。
func OperatorOutboundCutoff(reader io.Reader) (application.CutoffCommand, error) {
	var none application.CutoffCommand
	document, err := decodeDocument[operatorCutoffDocument](reader, "outbound cancellation")
	if err != nil {
		return none, err
	}
	if err := document.refuseCarriedIdentity(); err != nil {
		return none, err
	}
	if len(document.OwnerAccount) > 0 {
		return none, malformed("ownerAccount must not be carried in the payload")
	}
	if document.Basis == nil || strings.TrimSpace(*document.Basis) == "" {
		return none, malformed("basis is required")
	}
	request, sequence, err := cutoffTarget(document.RequestID, document.ParcelSequence)
	if err != nil {
		return none, err
	}
	return application.CutoffCommand{Request: request, Sequence: sequence, Basis: strings.TrimSpace(*document.Basis)}, nil
}

type returnDocument struct {
	carriedIdentity
	FactID    *string `json:"factId"`
	ReturnID  *string `json:"returnId"`
	Target    *string `json:"target"`
	Warehouse *string `json:"warehouse"`
}

// ReturnCommand 把回库事实译成命令。目标库位必须可分配，由编排核。
func ReturnCommand(reader io.Reader) (application.ReturnCommand, error) {
	var none application.ReturnCommand
	document, err := decodeDocument[returnDocument](reader, "return")
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
	returnID, err := requiredText("returnId", document.ReturnID, func(value string) (string, error) {
		if strings.TrimSpace(value) == "" {
			return "", malformed("returnId is required")
		}
		return strings.TrimSpace(value), nil
	})
	if err != nil {
		return none, err
	}
	target, err := requiredText("target", document.Target, domain.NewLocationCode)
	if err != nil {
		return none, err
	}
	warehouse, err := requiredText("warehouse", document.Warehouse, domain.NewWarehouseCode)
	if err != nil {
		return none, err
	}
	return application.ReturnCommand{FactID: fact, ReturnID: returnID, Target: target, Warehouse: warehouse}, nil
}

func cutoffTarget(requestID *string, sequence *int) (domain.OutboundRequestID, int, error) {
	var none domain.OutboundRequestID
	request, err := requiredText("requestId", requestID, domain.NewOutboundRequestID)
	if err != nil {
		return none, 0, err
	}
	if sequence == nil {
		return request, 0, nil
	}
	if *sequence < 1 {
		return none, 0, malformed("parcelSequence must be a positive integer")
	}
	return request, *sequence, nil
}
