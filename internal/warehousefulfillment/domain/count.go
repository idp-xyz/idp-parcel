package domain

import "time"

// CountID 是一份盘点的身份，由本上下文签发。
type CountID struct{ requiredValue }

func NewCountID(value string) (CountID, error) {
	required, err := newRequiredValue("count ID", value)
	return CountID{required}, err
}

// CountStatus 是盘点走到哪一步。
type CountStatus uint8

const (
	CountStatusInvalid CountStatus = iota
	CountPending
	CountCounting
	CountCounted
	CountVoid
)

func (status CountStatus) String() string {
	switch status {
	case CountPending:
		return "PENDING"
	case CountCounting:
		return "COUNTING"
	case CountCounted:
		return "COUNTED"
	case CountVoid:
		return "VOID"
	default:
		return ""
	}
}

// Count 是一份盘点。所涉库位与商品范围在创建时固定。
type Count struct {
	tenant     TenantID
	id         CountID
	decisionID RequestKey
	warehouse  WarehouseCode
	locations  []LocationCode
	owner      OwnerAccountID
	goods      []GoodsCode
	note       string
	operator   Registrant
	createdAt  time.Time
	status     CountStatus
	startedBy  Registrant
	startedAt  time.Time
	finishedBy Registrant
	finishedAt time.Time
	voidReason string
}

// CountSpec 是创建一份待盘点时的内容。货主与商品都空表示盘所涉库位上的全部商品库存。
type CountSpec struct {
	Tenant     TenantID
	ID         CountID
	DecisionID RequestKey
	Warehouse  WarehouseCode
	Locations  []LocationCode
	Owner      OwnerAccountID
	Goods      []GoodsCode
	Note       string
	Operator   Registrant
	CreatedAt  time.Time
}

func NewCount(spec CountSpec) (Count, error) {
	count := Count{
		tenant: spec.Tenant, id: spec.ID, decisionID: spec.DecisionID, warehouse: spec.Warehouse,
		locations: append([]LocationCode(nil), spec.Locations...), owner: spec.Owner,
		goods: append([]GoodsCode(nil), spec.Goods...), note: spec.Note, operator: spec.Operator,
		createdAt: normalizedInstant(spec.CreatedAt), status: CountPending,
	}
	if count.tenant.String() == "" || count.id.String() == "" || count.decisionID.String() == "" || count.warehouse.String() == "" ||
		count.operator.String() == "" || count.createdAt.IsZero() || len(count.locations) == 0 {
		return Count{}, ErrBlankValue
	}
	if count.owner.String() == "" && len(count.goods) > 0 {
		return Count{}, ErrBlankValue
	}
	return count, nil
}

func (count Count) Tenant() TenantID         { return count.tenant }
func (count Count) ID() CountID              { return count.id }
func (count Count) DecisionID() RequestKey   { return count.decisionID }
func (count Count) Warehouse() WarehouseCode { return count.warehouse }
func (count Count) Locations() []LocationCode {
	return append([]LocationCode(nil), count.locations...)
}
func (count Count) Owner() OwnerAccountID  { return count.owner }
func (count Count) Goods() []GoodsCode     { return append([]GoodsCode(nil), count.goods...) }
func (count Count) Note() string           { return count.note }
func (count Count) Operator() Registrant   { return count.operator }
func (count Count) StartedBy() Registrant  { return count.startedBy }
func (count Count) FinishedBy() Registrant { return count.finishedBy }
func (count Count) CreatedAt() time.Time   { return count.createdAt }
func (count Count) Status() CountStatus    { return count.status }
func (count Count) StartedAt() time.Time   { return count.startedAt }
func (count Count) FinishedAt() time.Time  { return count.finishedAt }
func (count Count) VoidReason() string     { return count.voidReason }

// Covers 回答这一格在不在盘点范围内。没限定货主与商品时，所涉库位上的货都在范围内。
func (count Count) Covers(owner OwnerAccountID, goods GoodsCode, location LocationCode) bool {
	found := false
	for _, item := range count.locations {
		if item == location {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	if count.owner.String() == "" {
		return true
	}
	if count.owner != owner {
		return false
	}
	if len(count.goods) == 0 {
		return true
	}
	for _, code := range count.goods {
		if code == goods {
			return true
		}
	}
	return false
}

// SameCount 比创建时提交的范围。操作者与时刻不在重放里。
func (count Count) SameCount(spec CountSpec) bool {
	if count.warehouse != spec.Warehouse || count.owner != spec.Owner || count.note != spec.Note || count.decisionID != spec.DecisionID {
		return false
	}
	if len(count.locations) != len(spec.Locations) || len(count.goods) != len(spec.Goods) {
		return false
	}
	for _, location := range spec.Locations {
		if !containsLocation(count.locations, location) {
			return false
		}
	}
	for _, goods := range spec.Goods {
		if !containsGoods(count.goods, goods) {
			return false
		}
	}
	return true
}

func containsLocation(locations []LocationCode, location LocationCode) bool {
	for _, item := range locations {
		if item == location {
			return true
		}
	}
	return false
}

func containsGoods(goods []GoodsCode, code GoodsCode) bool {
	for _, item := range goods {
		if item == code {
			return true
		}
	}
	return false
}

// Start 进入盘点中。已在盘点中的不进这里。
func (count Count) Start(operator Registrant, at time.Time) (Count, error) {
	if count.status != CountPending || operator.String() == "" || at.IsZero() {
		return Count{}, ErrBlankValue
	}
	count.status = CountCounting
	count.startedBy = operator
	count.startedAt = normalizedInstant(at)
	return count, nil
}

// Finish 进入已盘点。只从盘点中来。
func (count Count) Finish(operator Registrant, at time.Time) (Count, error) {
	if count.status != CountCounting || operator.String() == "" || at.IsZero() {
		return Count{}, ErrBlankValue
	}
	count.status = CountCounted
	count.finishedBy = operator
	count.finishedAt = normalizedInstant(at)
	return count, nil
}

// Void 进入已作废。已盘点的不进这里。
func (count Count) Void(operator Registrant, at time.Time, reason string) (Count, error) {
	if (count.status != CountPending && count.status != CountCounting) || operator.String() == "" || at.IsZero() {
		return Count{}, ErrBlankValue
	}
	count.status = CountVoid
	count.finishedBy = operator
	count.finishedAt = normalizedInstant(at)
	count.voidReason = reason
	return count, nil
}

// CountLock 是开始时加在一格上的作业锁定。数量是没被冻结的那一部分。
type CountLock struct {
	Owner    OwnerAccountID
	Location LocationCode
	Cell     StockCell
	Quantity int64
}
