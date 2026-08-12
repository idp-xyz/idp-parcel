// Package domain 承载运营结算的领域模型：运营结算余额、资金冻结，以及供其他上下文
// 消费的接受前财务控制结果。它不拥有委托决定，也不形成接受或拒绝。
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrBlankValue                = errors.New("settlement accounting: blank value")
	ErrInvalidSettlementScope    = errors.New("settlement accounting: invalid settlement scope")
	ErrInvalidOperationalBalance = errors.New("settlement accounting: invalid operational balance")
	ErrInvalidFreezeRequest      = errors.New("settlement accounting: invalid freeze request")
	ErrSettlementScopeMismatch   = errors.New("settlement accounting: balance belongs to another settlement scope")
	ErrControlRequestConflict    = errors.New("settlement accounting: control request identity carries different content")
	ErrFreezeNotFound            = errors.New("settlement accounting: freeze not found")
)

type requiredValue struct {
	value string
}

func newRequiredValue(name, value string) (requiredValue, error) {
	if strings.TrimSpace(value) == "" {
		return requiredValue{}, fmt.Errorf("%w: %s", ErrBlankValue, name)
	}
	return requiredValue{value: value}, nil
}

func (value requiredValue) String() string {
	return value.value
}

func (value requiredValue) valid() bool {
	return strings.TrimSpace(value.value) != ""
}

type LegalEntityReference struct{ requiredValue }

func NewLegalEntityReference(value string) (LegalEntityReference, error) {
	required, err := newRequiredValue("legal entity reference", value)
	return LegalEntityReference{required}, err
}

type SettlementAccountID struct{ requiredValue }

func NewSettlementAccountID(value string) (SettlementAccountID, error) {
	required, err := newRequiredValue("settlement account ID", value)
	return SettlementAccountID{required}, err
}

type CurrencyCode struct{ requiredValue }

func NewCurrencyCode(value string) (CurrencyCode, error) {
	required, err := newRequiredValue("currency code", value)
	return CurrencyCode{required}, err
}

type ControlRequestID struct{ requiredValue }

func NewControlRequestID(value string) (ControlRequestID, error) {
	required, err := newRequiredValue("control request ID", value)
	return ControlRequestID{required}, err
}

type BusinessAssociationReference struct{ requiredValue }

func NewBusinessAssociationReference(value string) (BusinessAssociationReference, error) {
	required, err := newRequiredValue("business association reference", value)
	return BusinessAssociationReference{required}, err
}

type FreezeID struct{ requiredValue }

type RestrictionReason struct{ requiredValue }

func NewRestrictionReason(value string) (RestrictionReason, error) {
	required, err := newRequiredValue("restriction reason", value)
	return RestrictionReason{required}, err
}

// SettlementScope 是一份余额的归属。责任法人、结算账户与币种三者都算在内，因为跨这
// 三者的余额默认不共用：拿一个作用域的钱去冻另一个作用域，正是这个类型要让它不可能
// 发生的错误。
type SettlementScope struct {
	legalEntity LegalEntityReference
	account     SettlementAccountID
	currency    CurrencyCode
}

func NewSettlementScope(
	legalEntity LegalEntityReference,
	account SettlementAccountID,
	currency CurrencyCode,
) (SettlementScope, error) {
	if !legalEntity.valid() || !account.valid() || !currency.valid() {
		return SettlementScope{}, ErrInvalidSettlementScope
	}
	return SettlementScope{legalEntity: legalEntity, account: account, currency: currency}, nil
}

func (scope SettlementScope) LegalEntity() LegalEntityReference {
	return scope.legalEntity
}

func (scope SettlementScope) Account() SettlementAccountID {
	return scope.account
}

func (scope SettlementScope) Currency() CurrencyCode {
	return scope.currency
}

func (scope SettlementScope) valid() bool {
	return scope.legalEntity.valid() && scope.account.valid() && scope.currency.valid()
}

// OperationalBalance 分开持有各个分量，而不是一个净值。CONTEXT 要求入账余额、当前
// 有效授信额度、冻结金额与已确认未结应收保持可分辨，一个净数字答不出「这笔钱为什么
// 不可用」。
type OperationalBalance struct {
	scope       SettlementScope
	postedMinor int64
	creditMinor int64
	frozenMinor int64
	unsettled   int64
}

func NewOperationalBalance(
	scope SettlementScope,
	postedMinor, creditMinor, frozenMinor, unsettledMinor int64,
) (OperationalBalance, error) {
	if !scope.valid() || creditMinor < 0 || frozenMinor < 0 || unsettledMinor < 0 {
		return OperationalBalance{}, ErrInvalidOperationalBalance
	}
	return OperationalBalance{
		scope:       scope,
		postedMinor: postedMinor,
		creditMinor: creditMinor,
		frozenMinor: frozenMinor,
		unsettled:   unsettledMinor,
	}, nil
}

func (balance OperationalBalance) Scope() SettlementScope {
	return balance.scope
}

// Available 是客户应收方向的可用余额：入账余额加当前有效授信额度，再扣除冻结金额与
// 已确认未结应收。
func (balance OperationalBalance) Available() int64 {
	return balance.postedMinor + balance.creditMinor - balance.frozenMinor - balance.unsettled
}

type FreezeRequest struct {
	requestID   ControlRequestID
	scope       SettlementScope
	amountMinor int64
	association BusinessAssociationReference
	requestedAt time.Time
}

// NewFreezeRequest 拒绝非正数金额。零金额冻结是一次什么也没占用、看上去却执行过的
// 控制，而 CONTEXT 明禁用它冒充「明确无控制」——那个答案由 `party-commercial` 以明确
// 的不适用依据提供。
func NewFreezeRequest(
	requestID ControlRequestID,
	scope SettlementScope,
	amountMinor int64,
	association BusinessAssociationReference,
	requestedAt time.Time,
) (FreezeRequest, error) {
	if !requestID.valid() || !scope.valid() || amountMinor <= 0 ||
		!association.valid() || requestedAt.IsZero() {
		return FreezeRequest{}, ErrInvalidFreezeRequest
	}
	return FreezeRequest{
		requestID:   requestID,
		scope:       scope,
		amountMinor: amountMinor,
		association: association,
		requestedAt: requestedAt.UTC(),
	}, nil
}

func (request FreezeRequest) RequestID() ControlRequestID {
	return request.requestID
}

func (request FreezeRequest) AmountMinor() int64 {
	return request.amountMinor
}

// FreezeStatus 是冻结可能结果的封闭集合。其中没有任何一个是接受判决：余额不足是本
// 上下文报告的业务限制，是否据此阻断委托由 `parcel-shipment` 决定。
type FreezeStatus uint8

const (
	FreezeStatusInvalid FreezeStatus = iota
	FreezeHeld
	FreezeReleased
	FreezeRestricted
)

func (status FreezeStatus) String() string {
	switch status {
	case FreezeHeld:
		return "HELD"
	case FreezeReleased:
		return "RELEASED"
	case FreezeRestricted:
		return "RESTRICTED"
	default:
		return ""
	}
}

type FundsFreeze struct {
	freezeID    FreezeID
	requestID   ControlRequestID
	scope       SettlementScope
	amountMinor int64
	association BusinessAssociationReference
	status      FreezeStatus
	frozenAt    time.Time
	releasedAt  time.Time
	reason      RestrictionReason
}

func (freeze FundsFreeze) FreezeID() FreezeID {
	return freeze.freezeID
}

func (freeze FundsFreeze) RequestID() ControlRequestID {
	return freeze.requestID
}

func (freeze FundsFreeze) AmountMinor() int64 {
	return freeze.amountMinor
}

func (freeze FundsFreeze) Status() FreezeStatus {
	return freeze.status
}

func (freeze FundsFreeze) FrozenAt() time.Time {
	return freeze.frozenAt
}

func (freeze FundsFreeze) ReleasedAt() time.Time {
	return freeze.releasedAt
}

func (freeze FundsFreeze) Reason() RestrictionReason {
	return freeze.reason
}

// FreezeLedger 只增不删地记录冻结。释放只是给记录打上标记，原金额与原冻结时间原样
// 留着——冻结发生过这件事，不因释放而不曾发生。
type FreezeLedger struct {
	byRequest map[ControlRequestID]FreezeID
	byFreeze  map[FreezeID]FundsFreeze
	digests   map[ControlRequestID]string
	nextID    int
}

func NewFreezeLedger() *FreezeLedger {
	return &FreezeLedger{
		byRequest: make(map[ControlRequestID]FreezeID),
		byFreeze:  make(map[FreezeID]FundsFreeze),
		digests:   make(map[ControlRequestID]string),
	}
}

// Freeze 为一次控制请求占用资金。同一请求重复到达返回原冻结；同一请求身份携带不同
// 内容形成冲突，既不复用原冻结也不再占用资金。超出可用余额报告为业务限制而非错误——
// 那是一个调用方必须能据以行动的业务答案，写成 error 会逼他当技术故障处置。
func (ledger *FreezeLedger) Freeze(request FreezeRequest, balance OperationalBalance) (FundsFreeze, error) {
	if request.scope != balance.scope {
		return FundsFreeze{}, ErrSettlementScopeMismatch
	}

	digest := freezeDigest(request)
	if existingID, exists := ledger.byRequest[request.requestID]; exists {
		if ledger.digests[request.requestID] != digest {
			return FundsFreeze{}, ErrControlRequestConflict
		}
		return ledger.byFreeze[existingID], nil
	}

	if request.amountMinor > balance.Available() {
		reason, err := NewRestrictionReason("AVAILABLE_BALANCE_INSUFFICIENT")
		if err != nil {
			return FundsFreeze{}, err
		}
		return FundsFreeze{
			requestID:   request.requestID,
			scope:       request.scope,
			amountMinor: request.amountMinor,
			association: request.association,
			status:      FreezeRestricted,
			reason:      reason,
		}, nil
	}

	ledger.nextID++
	freeze := FundsFreeze{
		freezeID:    FreezeID{requiredValue{value: fmt.Sprintf("FRZ-%04d", ledger.nextID)}},
		requestID:   request.requestID,
		scope:       request.scope,
		amountMinor: request.amountMinor,
		association: request.association,
		status:      FreezeHeld,
		frozenAt:    request.requestedAt,
	}
	ledger.byRequest[request.requestID] = freeze.freezeID
	ledger.byFreeze[freeze.freezeID] = freeze
	ledger.digests[request.requestID] = digest
	return freeze, nil
}

// Release 把一笔已冻结资金放开。对已释放的冻结再次释放返回与首次相同的答案，包括
// 原释放时间：响应丢失后的重试不得看起来像发生了第二次释放。
func (ledger *FreezeLedger) Release(freezeID FreezeID, releasedAt time.Time) (FundsFreeze, error) {
	freeze, found := ledger.byFreeze[freezeID]
	if !found {
		return FundsFreeze{}, ErrFreezeNotFound
	}
	if freeze.status == FreezeReleased {
		return freeze, nil
	}
	if releasedAt.IsZero() || releasedAt.Before(freeze.frozenAt) {
		return FundsFreeze{}, ErrInvalidFreezeRequest
	}
	freeze.status = FreezeReleased
	freeze.releasedAt = releasedAt.UTC()
	ledger.byFreeze[freezeID] = freeze
	return freeze, nil
}

func (ledger *FreezeLedger) Lookup(freezeID FreezeID) (FundsFreeze, bool) {
	freeze, found := ledger.byFreeze[freezeID]
	return freeze, found
}

// FindByRequest 按原控制请求身份找回冻结。释放按原业务关联认领，而调用方手里只有当初的
// 请求身份——FreezeID 是本账本签发的内部编号，不随控制结果离开本上下文重建。
//
// `业务限制`的记录不入账本（Freeze 对超额只交回结果不登记），因此这里找不到它——那次控制
// 本就没有占用资金，也就没有可释放的东西。
func (ledger *FreezeLedger) FindByRequest(requestID ControlRequestID) (FundsFreeze, bool) {
	freezeID, found := ledger.byRequest[requestID]
	if !found {
		return FundsFreeze{}, false
	}
	freeze, found := ledger.byFreeze[freezeID]
	return freeze, found
}

func (ledger *FreezeLedger) HeldCount() int {
	return ledger.countWith(FreezeHeld)
}

func (ledger *FreezeLedger) ReleaseCount() int {
	return ledger.countWith(FreezeReleased)
}

func (ledger *FreezeLedger) countWith(status FreezeStatus) int {
	count := 0
	for _, freeze := range ledger.byFreeze {
		if freeze.status == status {
			count++
		}
	}
	return count
}

func freezeDigest(request FreezeRequest) string {
	return strings.Join([]string{
		request.scope.legalEntity.String(),
		request.scope.account.String(),
		request.scope.currency.String(),
		fmt.Sprint(request.amountMinor),
		request.association.String(),
	}, "\x00")
}
