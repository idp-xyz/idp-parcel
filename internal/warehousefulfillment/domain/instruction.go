package domain

import (
	"slices"
	"time"
	"unicode/utf8"
)

// InstructionKind 是加工或商品换标。加工与拆解都固定一份组成版本。
type InstructionKind uint8

const (
	InstructionKindInvalid InstructionKind = iota
	InstructionProcess
	InstructionDisassemble
	InstructionRelabel
)

func (kind InstructionKind) String() string {
	switch kind {
	case InstructionProcess:
		return "PROCESS"
	case InstructionDisassemble:
		return "DISASSEMBLE"
	case InstructionRelabel:
		return "RELABEL"
	default:
		return ""
	}
}

// InstructionStatus 是指令的状态。一份指令只执行一次。
type InstructionStatus uint8

const (
	InstructionStatusInvalid InstructionStatus = iota
	InstructionPending
	InstructionDone
	InstructionVoid
)

func (status InstructionStatus) String() string {
	switch status {
	case InstructionPending:
		return "PENDING"
	case InstructionDone:
		return "DONE"
	case InstructionVoid:
		return "VOID"
	default:
		return ""
	}
}

// InstructionOrigin 是指令从哪一侧来，作废时也用它记下是哪一侧作的。
type InstructionOrigin uint8

const (
	InstructionOriginInvalid InstructionOrigin = iota
	InstructionOriginCustomer
	InstructionOriginOperator
)

func (origin InstructionOrigin) String() string {
	switch origin {
	case InstructionOriginCustomer:
		return "CUSTOMER"
	case InstructionOriginOperator:
		return "OPERATOR"
	default:
		return ""
	}
}

// InstructionID 是本上下文签发的指令标识。
type InstructionID struct{ requiredValue }

func NewInstructionID(value string) (InstructionID, error) {
	required, err := newRequiredValue("instruction ID", value)
	return InstructionID{required}, err
}

// InstructionComponent 是组成版本里的一个组件：每 1 个成品基本单位要多少个组件基本单位。
type InstructionComponent struct {
	goods    GoodsCode
	quantity int64
}

func NewInstructionComponent(goods GoodsCode, quantity int64) (InstructionComponent, error) {
	if !goods.valid() || quantity <= 0 {
		return InstructionComponent{}, ErrBlankValue
	}
	return InstructionComponent{goods: goods, quantity: quantity}, nil
}

func (component InstructionComponent) Goods() GoodsCode { return component.goods }
func (component InstructionComponent) Quantity() int64  { return component.quantity }

// Instruction 是已经受理的加工或换标指令。组成在受理时固定，之后的商品修订不回改它。
type Instruction struct {
	tenant        TenantID
	id            InstructionID
	owner         OwnerAccountID
	warehouse     WarehouseCode
	kind          InstructionKind
	status        InstructionStatus
	origin        InstructionOrigin
	goods         GoodsCode
	unit          GoodsUnitName
	quantity      int64
	fromAlias     GoodsAliasCode
	toAlias       GoodsAliasCode
	batch         BatchNumber
	expiry        ExpiryDate
	reference     string
	requestKey    RequestKey
	decisionID    RequestKey
	operator      Registrant
	reason        string
	basis         string
	namedRevision int
	fixedRevision int
	components    []InstructionComponent
	doneQuantity  int64
	executionFact InternalJobFactID
	voidOrigin    InstructionOrigin
	voidReason    string
	voidOperator  Registrant
	withdrawalKey RequestKey
	voidedAt      time.Time
	receivedAt    time.Time
}

// InstructionSpec 是受理时构造一份待加工或待换标指令的输入。
type InstructionSpec struct {
	Tenant        TenantID
	ID            InstructionID
	Owner         OwnerAccountID
	Warehouse     WarehouseCode
	Kind          InstructionKind
	Origin        InstructionOrigin
	Goods         GoodsCode
	Unit          GoodsUnitName
	Quantity      int64
	FromAlias     GoodsAliasCode
	ToAlias       GoodsAliasCode
	Batch         BatchNumber
	Expiry        ExpiryDate
	Reference     string
	RequestKey    RequestKey
	DecisionID    RequestKey
	Operator      Registrant
	Reason        string
	Basis         string
	NamedRevision int
	FixedRevision int
	Components    []InstructionComponent
	ReceivedAt    time.Time
}

func NewPendingInstruction(spec InstructionSpec) (Instruction, error) {
	if !spec.Tenant.valid() || !spec.ID.valid() || !spec.Owner.valid() || !spec.Warehouse.valid() ||
		spec.Kind.String() == "" || spec.Origin.String() == "" || !spec.Goods.valid() || spec.Quantity <= 0 ||
		spec.FixedRevision < 1 || spec.ReceivedAt.IsZero() {
		return Instruction{}, ErrBlankValue
	}
	if spec.Kind == InstructionRelabel && spec.Unit.String() == "" {
		return Instruction{}, ErrBlankValue
	}
	if spec.Origin == InstructionOriginCustomer && !spec.RequestKey.valid() {
		return Instruction{}, ErrBlankValue
	}
	if spec.Origin == InstructionOriginOperator && (!spec.DecisionID.valid() || spec.Reason == "") {
		return Instruction{}, ErrBlankValue
	}
	for _, text := range []string{spec.Reference, spec.Reason, spec.Basis} {
		if text != "" && (!utf8.ValidString(text) || containsNUL(text)) {
			return Instruction{}, ErrUnstorableValue
		}
	}
	return Instruction{
		tenant: spec.Tenant, id: spec.ID, owner: spec.Owner, warehouse: spec.Warehouse, kind: spec.Kind,
		status: InstructionPending, origin: spec.Origin, goods: spec.Goods, unit: spec.Unit, quantity: spec.Quantity,
		fromAlias: spec.FromAlias, toAlias: spec.ToAlias, batch: spec.Batch, expiry: spec.Expiry, reference: spec.Reference,
		requestKey: spec.RequestKey, decisionID: spec.DecisionID, operator: spec.Operator, reason: spec.Reason, basis: spec.Basis,
		namedRevision: spec.NamedRevision, fixedRevision: spec.FixedRevision, components: slices.Clone(spec.Components),
		receivedAt: normalizedInstant(spec.ReceivedAt),
	}, nil
}

func (instruction Instruction) Tenant() TenantID          { return instruction.tenant }
func (instruction Instruction) ID() InstructionID         { return instruction.id }
func (instruction Instruction) Owner() OwnerAccountID     { return instruction.owner }
func (instruction Instruction) Warehouse() WarehouseCode  { return instruction.warehouse }
func (instruction Instruction) Kind() InstructionKind     { return instruction.kind }
func (instruction Instruction) Status() InstructionStatus { return instruction.status }
func (instruction Instruction) Origin() InstructionOrigin { return instruction.origin }
func (instruction Instruction) Goods() GoodsCode          { return instruction.goods }
func (instruction Instruction) Unit() GoodsUnitName       { return instruction.unit }
func (instruction Instruction) Quantity() int64           { return instruction.quantity }
func (instruction Instruction) FromAlias() GoodsAliasCode { return instruction.fromAlias }
func (instruction Instruction) ToAlias() GoodsAliasCode   { return instruction.toAlias }
func (instruction Instruction) Batch() BatchNumber        { return instruction.batch }
func (instruction Instruction) Expiry() ExpiryDate        { return instruction.expiry }
func (instruction Instruction) Reference() string         { return instruction.reference }
func (instruction Instruction) RequestKey() RequestKey    { return instruction.requestKey }
func (instruction Instruction) DecisionID() RequestKey    { return instruction.decisionID }
func (instruction Instruction) Operator() Registrant      { return instruction.operator }
func (instruction Instruction) Reason() string            { return instruction.reason }
func (instruction Instruction) Basis() string             { return instruction.basis }
func (instruction Instruction) NamedRevision() int        { return instruction.namedRevision }
func (instruction Instruction) FixedRevision() int        { return instruction.fixedRevision }
func (instruction Instruction) Components() []InstructionComponent {
	return slices.Clone(instruction.components)
}
func (instruction Instruction) DoneQuantity() int64              { return instruction.doneQuantity }
func (instruction Instruction) ExecutionFact() InternalJobFactID { return instruction.executionFact }
func (instruction Instruction) VoidOrigin() InstructionOrigin    { return instruction.voidOrigin }
func (instruction Instruction) VoidReason() string               { return instruction.voidReason }
func (instruction Instruction) VoidOperator() Registrant         { return instruction.voidOperator }
func (instruction Instruction) WithdrawalKey() RequestKey        { return instruction.withdrawalKey }
func (instruction Instruction) VoidedAt() time.Time              { return instruction.voidedAt }
func (instruction Instruction) ReceivedAt() time.Time            { return instruction.receivedAt }

// SameSubmission 比受理时交来的内容，不比签发的标识、固定下来的修订号与受理时刻。
func (instruction Instruction) SameSubmission(spec InstructionSpec) bool {
	return instruction.owner == spec.Owner && instruction.warehouse == spec.Warehouse && instruction.kind == spec.Kind &&
		instruction.origin == spec.Origin && instruction.goods == spec.Goods && instruction.unit == spec.Unit &&
		instruction.quantity == spec.Quantity && instruction.fromAlias == spec.FromAlias && instruction.toAlias == spec.ToAlias &&
		instruction.batch == spec.Batch && instruction.expiry == spec.Expiry && instruction.reference == spec.Reference &&
		instruction.requestKey == spec.RequestKey && instruction.decisionID == spec.DecisionID &&
		instruction.operator == spec.Operator && instruction.reason == spec.Reason && instruction.basis == spec.Basis &&
		instruction.namedRevision == spec.NamedRevision
}

// WithDone 交回进入已加工或已换标的副本。
func (instruction Instruction) WithDone(done int64, fact InternalJobFactID) (Instruction, error) {
	if instruction.status != InstructionPending || done <= 0 || done > instruction.quantity || !fact.valid() {
		return Instruction{}, ErrBlankValue
	}
	instruction.doneQuantity = done
	instruction.executionFact = fact
	instruction.status = InstructionDone
	return instruction, nil
}

// WithVoid 交回已作废的副本。已作废再作废由调用方答已有结果，不进这里。
// 货主撤回带上来源请求键；运营作废带上操作者。没带的是零值。
func (instruction Instruction) WithVoid(origin InstructionOrigin, reason string, at time.Time, operator Registrant, withdrawal RequestKey) (Instruction, error) {
	if instruction.status != InstructionPending || origin.String() == "" || at.IsZero() {
		return Instruction{}, ErrBlankValue
	}
	if reason != "" && (!utf8.ValidString(reason) || containsNUL(reason)) {
		return Instruction{}, ErrUnstorableValue
	}
	instruction.status = InstructionVoid
	instruction.voidOrigin = origin
	instruction.voidReason = reason
	instruction.voidOperator = operator
	instruction.withdrawalKey = withdrawal
	instruction.voidedAt = normalizedInstant(at)
	return instruction, nil
}
