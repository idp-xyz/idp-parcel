package domain

import "time"

// WarehouseOutboundSource 是以货主名义提交委托的来源字面。与小包托运那一侧同词，两边各写各的。
const WarehouseOutboundSource = "WAREHOUSE_OUTBOUND"

// DispatchMethod 是出库请求的发运方式，产品内置封闭三种。
type DispatchMethod uint8

const (
	DispatchMethodInvalid DispatchMethod = iota
	DispatchProductNetwork
	DispatchOwnerLabel
	DispatchPickup
)

func (method DispatchMethod) String() string {
	switch method {
	case DispatchProductNetwork:
		return "PRODUCT_NETWORK"
	case DispatchOwnerLabel:
		return "OWNER_LABEL"
	case DispatchPickup:
		return "PICKUP"
	default:
		return ""
	}
}

func NewDispatchMethod(word string) (DispatchMethod, error) {
	for _, method := range []DispatchMethod{DispatchProductNetwork, DispatchOwnerLabel, DispatchPickup} {
		if method.String() == word {
			return method, nil
		}
	}
	return DispatchMethodInvalid, newRequiredValueError("dispatch method")
}

func newRequiredValueError(name string) error {
	_, err := newRequiredValue(name, "")
	return err
}

// OutboundOrigin 是出库请求从哪一张口来。
type OutboundOrigin uint8

const (
	OutboundOriginInvalid OutboundOrigin = iota
	OutboundOriginCustomer
	OutboundOriginOperator
)

func (origin OutboundOrigin) String() string {
	switch origin {
	case OutboundOriginCustomer:
		return "CUSTOMER"
	case OutboundOriginOperator:
		return "OPERATOR"
	default:
		return ""
	}
}

func NewOutboundOrigin(word string) (OutboundOrigin, error) {
	for _, origin := range []OutboundOrigin{OutboundOriginCustomer, OutboundOriginOperator} {
		if origin.String() == word {
			return origin, nil
		}
	}
	return OutboundOriginInvalid, newRequiredValueError("outbound origin")
}

// OutboundStatus 是出库请求受理之后停在哪一格。已分配及以后不在这一段。
type OutboundStatus uint8

const (
	OutboundStatusInvalid OutboundStatus = iota
	OutboundReserved
	OutboundPartiallyReserved
	OutboundRejected
	OutboundPendingPolicy
	OutboundHandedOff
	OutboundSplit
	OutboundAllocated
	OutboundPicked
	OutboundChecked
	OutboundPacked
)

func (status OutboundStatus) String() string {
	switch status {
	case OutboundReserved:
		return "RESERVED"
	case OutboundPartiallyReserved:
		return "PARTIALLY_RESERVED"
	case OutboundRejected:
		return "REJECTED"
	case OutboundPendingPolicy:
		return "PENDING_POLICY"
	case OutboundHandedOff:
		return "HANDED_OFF"
	case OutboundSplit:
		return "SPLIT"
	case OutboundAllocated:
		return "ALLOCATED"
	case OutboundPicked:
		return "PICKED"
	case OutboundChecked:
		return "CHECKED"
	case OutboundPacked:
		return "PACKED"
	default:
		return ""
	}
}

func NewOutboundStatus(word string) (OutboundStatus, error) {
	for _, status := range []OutboundStatus{OutboundReserved, OutboundPartiallyReserved, OutboundRejected, OutboundPendingPolicy, OutboundHandedOff, OutboundSplit, OutboundAllocated, OutboundPicked, OutboundChecked, OutboundPacked} {
		if status.String() == word {
			return status, nil
		}
	}
	return OutboundStatusInvalid, newRequiredValueError("outbound status")
}

// ShortagePolicyKind 是可用量不够时的两种处置。用哪一种按货主登记，产品不替租户选定。
type ShortagePolicyKind uint8

const (
	ShortagePolicyInvalid ShortagePolicyKind = iota
	ShortagePolicyPartial
	ShortagePolicyReject
)

func (kind ShortagePolicyKind) String() string {
	switch kind {
	case ShortagePolicyPartial:
		return "PARTIAL"
	case ShortagePolicyReject:
		return "REJECT"
	default:
		return ""
	}
}

func NewShortagePolicyKind(word string) (ShortagePolicyKind, error) {
	for _, kind := range []ShortagePolicyKind{ShortagePolicyPartial, ShortagePolicyReject} {
		if kind.String() == word {
			return kind, nil
		}
	}
	return ShortagePolicyInvalid, newRequiredValueError("shortage policy")
}

// OutboundRequestID 是一份出库请求的身份，由本上下文签发。
type OutboundRequestID struct{ requiredValue }

func NewOutboundRequestID(value string) (OutboundRequestID, error) {
	required, err := newRequiredValue("outbound request ID", value)
	return OutboundRequestID{required}, err
}

// OutboundLine 是货主交来的一行，还没展开组合商品。
type OutboundLine struct {
	goods    GoodsCode
	unit     GoodsUnitName
	state    StockState
	alias    GoodsAliasCode
	batch    BatchNumber
	expiry   ExpiryDate
	quantity int64
}

type OutboundLineSpec struct {
	Goods    GoodsCode
	Unit     GoodsUnitName
	State    StockState
	Alias    GoodsAliasCode
	Batch    BatchNumber
	Expiry   ExpiryDate
	Quantity int64
}

func NewOutboundLine(spec OutboundLineSpec) (OutboundLine, error) {
	if spec.Goods.String() == "" || spec.Unit.String() == "" || spec.State.String() == "" || spec.Quantity <= 0 {
		return OutboundLine{}, newRequiredValueError("outbound line")
	}
	return OutboundLine{
		goods: spec.Goods, unit: spec.Unit, state: spec.State, alias: spec.Alias,
		batch: spec.Batch, expiry: spec.Expiry, quantity: spec.Quantity,
	}, nil
}

func (line OutboundLine) Goods() GoodsCode      { return line.goods }
func (line OutboundLine) Unit() GoodsUnitName   { return line.unit }
func (line OutboundLine) State() StockState     { return line.state }
func (line OutboundLine) Alias() GoodsAliasCode { return line.alias }
func (line OutboundLine) Batch() BatchNumber    { return line.batch }
func (line OutboundLine) Expiry() ExpiryDate    { return line.expiry }
func (line OutboundLine) Quantity() int64       { return line.quantity }

func (line OutboundLine) same(other OutboundLine) bool {
	return line.goods == other.goods && line.unit == other.unit && line.state == other.state &&
		line.alias == other.alias && line.batch == other.batch && line.expiry == other.expiry &&
		line.quantity == other.quantity
}

// OutboundReservation 是一行展开之后实际占上的基本单位数量。Quantity 是占上的，Wanted 是想占的。
type OutboundReservation struct {
	Goods         GoodsCode
	State         StockState
	Alias         GoodsAliasCode
	Batch         BatchNumber
	Expiry        ExpiryDate
	Quantity      int64
	FromUnlabeled int64
	Wanted        int64
}

// OutboundRequest 是已经落册的出库请求。
type OutboundRequest struct {
	tenant       TenantID
	id           OutboundRequestID
	owner        OwnerAccountID
	warehouse    WarehouseCode
	origin       OutboundOrigin
	key          RequestKey
	dispatch     DispatchMethod
	service      string
	recipient    string
	address      string
	basis        string
	operator     string
	status       OutboundStatus
	parent       OutboundRequestID
	number       DocumentNumber
	absence      NumberingAbsence
	lines        []OutboundLine
	reservations []OutboundReservation
	receivedAt   time.Time
}

type OutboundRequestSpec struct {
	Tenant       TenantID
	ID           OutboundRequestID
	Owner        OwnerAccountID
	Warehouse    WarehouseCode
	Origin       OutboundOrigin
	Key          RequestKey
	Dispatch     DispatchMethod
	Service      string
	Recipient    string
	Address      string
	Basis        string
	Operator     string
	Status       OutboundStatus
	Parent       OutboundRequestID
	Number       DocumentNumber
	Absence      NumberingAbsence
	Lines        []OutboundLine
	Reservations []OutboundReservation
	ReceivedAt   time.Time
}

func NewOutboundRequest(spec OutboundRequestSpec) (OutboundRequest, error) {
	if spec.Tenant.String() == "" || spec.ID.String() == "" || spec.Owner.String() == "" || spec.Warehouse.String() == "" ||
		spec.Origin == OutboundOriginInvalid || spec.Key.String() == "" || spec.Dispatch == DispatchMethodInvalid ||
		spec.Status == OutboundStatusInvalid || spec.Recipient == "" || spec.Address == "" || spec.ReceivedAt.IsZero() ||
		len(spec.Lines) == 0 {
		return OutboundRequest{}, newRequiredValueError("outbound request")
	}
	lines := append([]OutboundLine(nil), spec.Lines...)
	reservations := append([]OutboundReservation(nil), spec.Reservations...)
	return OutboundRequest{
		tenant: spec.Tenant, id: spec.ID, owner: spec.Owner, warehouse: spec.Warehouse, origin: spec.Origin, key: spec.Key,
		dispatch: spec.Dispatch, service: spec.Service, recipient: spec.Recipient, address: spec.Address, basis: spec.Basis,
		operator: spec.Operator, status: spec.Status, parent: spec.Parent, number: spec.Number, absence: spec.Absence, lines: lines,
		reservations: reservations, receivedAt: spec.ReceivedAt,
	}, nil
}

func (request OutboundRequest) Tenant() TenantID          { return request.tenant }
func (request OutboundRequest) ID() OutboundRequestID     { return request.id }
func (request OutboundRequest) Owner() OwnerAccountID     { return request.owner }
func (request OutboundRequest) Warehouse() WarehouseCode  { return request.warehouse }
func (request OutboundRequest) Origin() OutboundOrigin    { return request.origin }
func (request OutboundRequest) Key() RequestKey           { return request.key }
func (request OutboundRequest) Dispatch() DispatchMethod  { return request.dispatch }
func (request OutboundRequest) Service() string           { return request.service }
func (request OutboundRequest) Recipient() string         { return request.recipient }
func (request OutboundRequest) Address() string           { return request.address }
func (request OutboundRequest) Basis() string             { return request.basis }
func (request OutboundRequest) Operator() string          { return request.operator }
func (request OutboundRequest) Status() OutboundStatus    { return request.status }
func (request OutboundRequest) Parent() OutboundRequestID { return request.parent }
func (request OutboundRequest) Lines() []OutboundLine {
	return append([]OutboundLine(nil), request.lines...)
}
func (request OutboundRequest) Reservations() []OutboundReservation {
	return append([]OutboundReservation(nil), request.reservations...)
}
func (request OutboundRequest) ReceivedAt() time.Time { return request.receivedAt }

func (request OutboundRequest) DocumentNumber() (DocumentNumber, bool) {
	if request.number.Assigned() {
		return request.number, true
	}
	return DocumentNumber{}, false
}

func (request OutboundRequest) NumberingAbsence() NumberingAbsence { return request.absence }

func (request OutboundRequest) SameSubmission(other OutboundRequest) bool {
	if request.warehouse != other.warehouse || request.dispatch != other.dispatch || request.service != other.service ||
		request.recipient != other.recipient || request.address != other.address || len(request.lines) != len(other.lines) {
		return false
	}
	for index := range request.lines {
		if !request.lines[index].same(other.lines[index]) {
			return false
		}
	}
	return true
}
