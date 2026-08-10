// Package domain holds the settlement-accounting model: operational balances,
// funds freezes and the pre-acceptance financial control results other contexts
// consume. It owns no shipment decision and forms no acceptance or rejection.
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

// SettlementScope is what a balance belongs to. Legal entity, settlement account
// and currency are all part of it because balances across them are not shared by
// default: borrowing one scope's funds for another is the mistake this type
// exists to make impossible.
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

// OperationalBalance carries the components separately rather than one net
// figure. The context requires posted balance, credit limit, frozen amount and
// confirmed-unsettled receivable to stay distinguishable, so a single number
// could not answer why an amount is unavailable.
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

// Available is the receivable-direction available balance: posted plus the
// currently effective credit limit, less what is frozen and what is confirmed
// but unsettled.
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

// NewFreezeRequest refuses a non-positive amount. A zero-amount freeze would be
// a control that occupies nothing while looking like one was performed, and the
// context forbids using exactly that to stand in for "no control applies" — that
// answer belongs to party-commercial as an explicit inapplicability basis.
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

// FreezeStatus is the closed set of outcomes a freeze may have. None of them is
// an acceptance verdict: an insufficient balance is a business restriction this
// context reports, and whether it blocks a shipment is parcel-shipment's call.
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

// FreezeLedger records freezes without ever removing one. A release marks the
// record and leaves the original amount and time in place, because the freeze
// having happened is a fact its release does not undo.
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

// Freeze occupies funds for one control request. A repeat of the same request
// returns the original freeze; the same identity carrying different content is a
// conflict that neither reuses the original nor occupies more funds. An amount
// beyond the available balance is reported as a restriction rather than an
// error, because it is a business answer the caller must be able to act on.
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

// Release turns a held freeze loose. Releasing an already released freeze is the
// same answer as the first release, including its original time: a retry after a
// lost response must not look like a second release happened.
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
