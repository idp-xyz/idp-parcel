package domain

import "time"

// FreezeID 是一份冻结的身份，由本上下文签发。
type FreezeID struct{ requiredValue }

func NewFreezeID(value string) (FreezeID, error) {
	required, err := newRequiredValue("freeze ID", value)
	return FreezeID{required}, err
}

// FreezeStatus 是冻结还在，还是已经整份解冻。
type FreezeStatus uint8

const (
	FreezeStatusInvalid FreezeStatus = iota
	FreezeFrozen
	FreezeUnfrozen
)

func (status FreezeStatus) String() string {
	switch status {
	case FreezeFrozen:
		return "FROZEN"
	case FreezeUnfrozen:
		return "UNFROZEN"
	default:
		return ""
	}
}

// FreezeTarget 是冻的是一格商品还是一箱。
type FreezeTarget uint8

const (
	FreezeTargetInvalid FreezeTarget = iota
	FreezeGoods
	FreezeCarton
)

func (target FreezeTarget) String() string {
	switch target {
	case FreezeGoods:
		return "GOODS"
	case FreezeCarton:
		return "CARTON"
	default:
		return ""
	}
}

// Freeze 是一份已落册的冻结。解冻不另起一份，原冻结留着，数量回到可用量。
type Freeze struct {
	tenant     TenantID
	id         FreezeID
	decisionID RequestKey
	owner      OwnerAccountID
	warehouse  WarehouseCode
	target     FreezeTarget
	cell       StockCell
	carton     CartonNumber
	quantity   int64
	reason     string
	operator   Registrant
	frozenAt   time.Time
	status     FreezeStatus
	unfrozenBy Registrant
	unfrozenAt time.Time
	note       string
}

// FreezeSpec 是冻结成立时落册的内容。整箱不带格子，数量是一箱。
type FreezeSpec struct {
	Tenant     TenantID
	ID         FreezeID
	DecisionID RequestKey
	Owner      OwnerAccountID
	Warehouse  WarehouseCode
	Target     FreezeTarget
	Cell       StockCell
	Carton     CartonNumber
	Quantity   int64
	Reason     string
	Operator   Registrant
	FrozenAt   time.Time
}

func NewFreeze(spec FreezeSpec) (Freeze, error) {
	freeze := Freeze{
		tenant: spec.Tenant, id: spec.ID, decisionID: spec.DecisionID, owner: spec.Owner, warehouse: spec.Warehouse,
		target: spec.Target, cell: spec.Cell, carton: spec.Carton, quantity: spec.Quantity, reason: spec.Reason,
		operator: spec.Operator, frozenAt: normalizedInstant(spec.FrozenAt), status: FreezeFrozen,
	}
	if freeze.tenant.String() == "" || freeze.id.String() == "" || freeze.decisionID.String() == "" || freeze.owner.String() == "" ||
		freeze.warehouse.String() == "" || freeze.operator.String() == "" || freeze.frozenAt.IsZero() || freeze.reason == "" || freeze.quantity <= 0 {
		return Freeze{}, ErrBlankValue
	}
	switch freeze.target {
	case FreezeGoods:
		if freeze.cell.Location().String() == "" || freeze.cell.InboundBatch().String() == "" || freeze.carton.String() != "" {
			return Freeze{}, ErrBlankValue
		}
	case FreezeCarton:
		if freeze.carton.String() == "" || freeze.quantity != 1 || freeze.cell.Location().String() != "" {
			return Freeze{}, ErrBlankValue
		}
	default:
		return Freeze{}, ErrBlankValue
	}
	return freeze, nil
}

func (freeze Freeze) Tenant() TenantID         { return freeze.tenant }
func (freeze Freeze) ID() FreezeID             { return freeze.id }
func (freeze Freeze) DecisionID() RequestKey   { return freeze.decisionID }
func (freeze Freeze) Owner() OwnerAccountID    { return freeze.owner }
func (freeze Freeze) Warehouse() WarehouseCode { return freeze.warehouse }
func (freeze Freeze) Target() FreezeTarget     { return freeze.target }
func (freeze Freeze) Cell() StockCell          { return freeze.cell }
func (freeze Freeze) Carton() CartonNumber     { return freeze.carton }
func (freeze Freeze) Quantity() int64          { return freeze.quantity }
func (freeze Freeze) Reason() string           { return freeze.reason }
func (freeze Freeze) Operator() Registrant     { return freeze.operator }
func (freeze Freeze) FrozenAt() time.Time      { return freeze.frozenAt }
func (freeze Freeze) Status() FreezeStatus     { return freeze.status }
func (freeze Freeze) UnfrozenBy() Registrant   { return freeze.unfrozenBy }
func (freeze Freeze) UnfrozenAt() time.Time    { return freeze.unfrozenAt }
func (freeze Freeze) Note() string             { return freeze.note }

// SameFreeze 比的是提交内容。操作者与时刻不在重放里。
func (freeze Freeze) SameFreeze(spec FreezeSpec) bool {
	if freeze.owner != spec.Owner || freeze.warehouse != spec.Warehouse || freeze.target != spec.Target ||
		freeze.quantity != spec.Quantity || freeze.reason != spec.Reason || freeze.decisionID != spec.DecisionID {
		return false
	}
	if spec.Target == FreezeCarton {
		return freeze.carton == spec.Carton
	}
	return freeze.cell == spec.Cell
}

// Unfreeze 交回已解冻的副本。已解冻的不再进这里。
func (freeze Freeze) Unfreeze(operator Registrant, at time.Time, note string) (Freeze, error) {
	if freeze.status != FreezeFrozen || operator.String() == "" || at.IsZero() {
		return Freeze{}, ErrBlankValue
	}
	freeze.status = FreezeUnfrozen
	freeze.unfrozenBy = operator
	freeze.unfrozenAt = normalizedInstant(at)
	freeze.note = note
	return freeze, nil
}

// FreezeIssueKind 是冻结内容不合时指出的那一格。
type FreezeIssueKind uint8

const (
	FreezeIssueInvalid FreezeIssueKind = iota
	FreezeIssueLocationNotFound
	FreezeIssueGoodsNotRegistered
	FreezeIssueUnitNotDeclared
	FreezeIssueInboundBatchNotOnCell
	FreezeIssueCartonNotOnHand
)

func (kind FreezeIssueKind) String() string {
	switch kind {
	case FreezeIssueLocationNotFound:
		return "LOCATION_NOT_FOUND"
	case FreezeIssueGoodsNotRegistered:
		return "GOODS_NOT_REGISTERED"
	case FreezeIssueUnitNotDeclared:
		return "UNIT_NOT_DECLARED"
	case FreezeIssueInboundBatchNotOnCell:
		return "INBOUND_BATCH_NOT_ON_CELL"
	case FreezeIssueCartonNotOnHand:
		return "CARTON_NOT_ON_HAND"
	default:
		return ""
	}
}

type FreezeIssue struct{ kind FreezeIssueKind }

func NewFreezeIssue(kind FreezeIssueKind) FreezeIssue { return FreezeIssue{kind: kind} }

func (issue FreezeIssue) Kind() FreezeIssueKind { return issue.kind }

// AllocatableGoods 是这一格能不能被预占或冻结：存储或拣选里的良品，次品库区里的次品。待检与三种暂存都不可分配。
func AllocatableGoods(state StockState, usage ZoneUsage) bool {
	switch usage {
	case ZoneUsageStorage, ZoneUsagePicking:
		return state == StockStateGood
	case ZoneUsageDefective:
		return state == StockStateDefective
	default:
		return false
	}
}

// AllocatableCarton 是整箱能不能被整箱出的请求预占：只在存储库区。
func AllocatableCarton(usage ZoneUsage) bool {
	return usage == ZoneUsageStorage
}
