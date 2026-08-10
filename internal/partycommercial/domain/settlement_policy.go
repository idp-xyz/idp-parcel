package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidSettlementApplicability = errors.New("party commercial: invalid settlement applicability")
	ErrInvalidSettlementPolicy        = errors.New("party commercial: invalid settlement policy")
	ErrInvalidSettlementQuery         = errors.New("party commercial: invalid settlement query")
	ErrNoApplicableSettlementPolicy   = errors.New("party commercial: no applicable settlement policy")
	ErrSettlementMethodConflict       = errors.New("party commercial: one scope is hit by both settlement methods")
)

type CounterpartyReference struct{ requiredValue }

func NewCounterpartyReference(value string) (CounterpartyReference, error) {
	required, err := newRequiredValue("counterparty reference", value)
	return CounterpartyReference{required}, err
}

// ChargeScopeReference names the service or charge range a settlement method
// applies to. It is what lets one customer run prepaid and terms business at the
// same time without either borrowing the other's arrangement.
type ChargeScopeReference struct{ requiredValue }

func NewChargeScopeReference(value string) (ChargeScopeReference, error) {
	required, err := newRequiredValue("charge scope reference", value)
	return ChargeScopeReference{required}, err
}

type CurrencyCode struct{ requiredValue }

func NewCurrencyCode(value string) (CurrencyCode, error) {
	required, err := newRequiredValue("currency code", value)
	return CurrencyCode{required}, err
}

// SettlementMethod is closed at two values on purpose. A third — a customer-level
// default — is exactly what the context forbids: an unmatched scope must report
// no applicable basis rather than fall back to a house style.
type SettlementMethod uint8

const (
	SettlementMethodInvalid SettlementMethod = iota
	PrepaidMethod
	TermsMethod
)

func (method SettlementMethod) valid() bool {
	return method >= PrepaidMethod && method <= TermsMethod
}

func (method SettlementMethod) String() string {
	switch method {
	case PrepaidMethod:
		return "PREPAID"
	case TermsMethod:
		return "TERMS"
	default:
		return ""
	}
}

// SettlementApplicability is the six-dimension range a settlement method covers.
// All six are part of it because the context forbids collecting across legal
// entity, counterparty, direction or currency through a broad customer
// relationship: a policy answers only for the exact range it names.
type SettlementApplicability struct {
	legalEntity  LegalEntityReference
	counterparty CounterpartyReference
	contract     CommercialVersionLabel
	chargeScope  ChargeScopeReference
	currency     CurrencyCode
	effective    EffectiveInterval
}

func NewSettlementApplicability(
	legalEntity LegalEntityReference,
	counterparty CounterpartyReference,
	contract CommercialVersionLabel,
	chargeScope ChargeScopeReference,
	currency CurrencyCode,
	effective EffectiveInterval,
) (SettlementApplicability, error) {
	if !legalEntity.valid() || !counterparty.valid() || !contract.valid() ||
		!chargeScope.valid() || !currency.valid() || !effective.valid() {
		return SettlementApplicability{}, ErrInvalidSettlementApplicability
	}
	return SettlementApplicability{
		legalEntity:  legalEntity,
		counterparty: counterparty,
		contract:     contract,
		chargeScope:  chargeScope,
		currency:     currency,
		effective:    effective,
	}, nil
}

func (applicability SettlementApplicability) covers(query SettlementQuery) bool {
	return applicability.legalEntity == query.legalEntity &&
		applicability.counterparty == query.counterparty &&
		applicability.contract == query.contract &&
		applicability.chargeScope == query.chargeScope &&
		applicability.currency == query.currency &&
		applicability.effective.Contains(query.at)
}

// SettlementPolicy is the content of one settlement policy version: which method
// applies, over which range. The version carries the release invariants; this
// type carries what the release says.
type SettlementPolicy struct {
	version       CommercialVersion
	method        SettlementMethod
	applicability SettlementApplicability
}

func NewSettlementPolicy(
	version CommercialVersion,
	method SettlementMethod,
	applicability SettlementApplicability,
) (SettlementPolicy, error) {
	if version.kind != SettlementPolicyObject ||
		version.status != CommercialVersionEffective ||
		!method.valid() {
		return SettlementPolicy{}, ErrInvalidSettlementPolicy
	}
	return SettlementPolicy{version: version, method: method, applicability: applicability}, nil
}

func (policy SettlementPolicy) Version() CommercialVersion {
	return policy.version
}

func (policy SettlementPolicy) Method() SettlementMethod {
	return policy.method
}

func (policy SettlementPolicy) Applicability() SettlementApplicability {
	return policy.applicability
}

// SettlementQuery is the exact range a caller needs a method for. It carries the
// instant as well, because a policy that has left its effective interval no
// longer answers.
type SettlementQuery struct {
	legalEntity  LegalEntityReference
	counterparty CounterpartyReference
	contract     CommercialVersionLabel
	chargeScope  ChargeScopeReference
	currency     CurrencyCode
	at           time.Time
}

func NewSettlementQuery(
	legalEntity LegalEntityReference,
	counterparty CounterpartyReference,
	contract CommercialVersionLabel,
	chargeScope ChargeScopeReference,
	currency CurrencyCode,
	at time.Time,
) (SettlementQuery, error) {
	if !legalEntity.valid() || !counterparty.valid() || !contract.valid() ||
		!chargeScope.valid() || !currency.valid() || at.IsZero() {
		return SettlementQuery{}, ErrInvalidSettlementQuery
	}
	return SettlementQuery{
		legalEntity:  legalEntity,
		counterparty: counterparty,
		contract:     contract,
		chargeScope:  chargeScope,
		currency:     currency,
		at:           at.UTC(),
	}, nil
}

// ResolveSettlementPolicy answers which method covers one exact range.
//
// Two policies covering the same range conflict even when they agree, but the
// case the context names is the dangerous one: prepaid and terms both hitting a
// single charge scope. Picking either would silently decide whether the customer
// pays up front, so the overlap is reported for the commercial owner to correct.
// Nothing matching is likewise reported rather than defaulted — a customer-level
// fallback is exactly what must not fill the gap.
func ResolveSettlementPolicy(policies []SettlementPolicy, query SettlementQuery) (SettlementPolicy, error) {
	matches := make([]SettlementPolicy, 0, 2)
	for _, policy := range policies {
		if policy.applicability.covers(query) {
			matches = append(matches, policy)
		}
	}

	switch len(matches) {
	case 0:
		return SettlementPolicy{}, ErrNoApplicableSettlementPolicy
	case 1:
		return matches[0], nil
	default:
		return SettlementPolicy{}, ErrSettlementMethodConflict
	}
}
