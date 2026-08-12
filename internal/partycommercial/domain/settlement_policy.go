package domain

import (
	"errors"
	"strings"
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

// ChargeScopeReference 标明一种结算方式适用的服务或费用范围。正是它让同一货主
// 可以同时存在预付与账期业务，而两者互不借用对方的约定。
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

// SettlementMethod 有意只封闭在两个取值。第三个取值——客户级默认值——正是本上下文
// 禁止的：未命中的范围必须报出`无适用依据`，而不是回落到某种通行做法。
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

// SettlementApplicability 是一种结算方式覆盖的六个维度。六个都在其中，是因为本
// 上下文禁止借宽泛的客户关系跨责任法人、相对方、收付方向或币种自动归集：一份政策
// 只回答它自己指名的那个精确范围。
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

func (applicability SettlementApplicability) LegalEntity() LegalEntityReference {
	return applicability.legalEntity
}

func (applicability SettlementApplicability) Counterparty() CounterpartyReference {
	return applicability.counterparty
}

func (applicability SettlementApplicability) Contract() CommercialVersionLabel {
	return applicability.contract
}

func (applicability SettlementApplicability) ChargeScope() ChargeScopeReference {
	return applicability.chargeScope
}

func (applicability SettlementApplicability) Currency() CurrencyCode {
	return applicability.currency
}

func (applicability SettlementApplicability) Effective() EffectiveInterval {
	return applicability.effective
}

func (applicability SettlementApplicability) covers(query SettlementQuery) bool {
	return applicability.legalEntity == query.legalEntity &&
		applicability.counterparty == query.counterparty &&
		applicability.contract == query.contract &&
		applicability.chargeScope == query.chargeScope &&
		applicability.currency == query.currency &&
		applicability.effective.Contains(query.at)
}

// SettlementPolicy 是一个结算政策版本的正文：在哪个范围内适用哪种结算方式。
// 版本承载发布相关的不变量，本类型承载这次发布说了什么。
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

// SettlementSelector 是解析键上请求结算依据时的结算专属维度（ADR-0044，镜像 PriceDirection
// 的纪律：请求结算依据必填，其余请求必缺）。法人与时点不在其中——键上已有 LegalEntityCandidate
// 与锚点，重复携带就允许两者不一致。
type SettlementSelector struct {
	Counterparty CounterpartyReference
	Contract     CommercialVersionLabel
	ChargeScope  ChargeScopeReference
	Currency     CurrencyCode
}

func (selector SettlementSelector) declared() bool {
	return selector.Counterparty.valid() &&
		selector.Contract.valid() &&
		selector.ChargeScope.valid() &&
		selector.Currency.valid()
}

func (selector SettlementSelector) empty() bool {
	return !selector.Counterparty.valid() &&
		!selector.Contract.valid() &&
		!selector.ChargeScope.valid() &&
		!selector.Currency.valid()
}

func (selector SettlementSelector) fingerprint() string {
	return strings.Join([]string{
		selector.Counterparty.String(),
		selector.Contract.String(),
		selector.ChargeScope.String(),
		selector.Currency.String(),
	}, "\x00")
}

// SettlementQuery 是调用方需要结算方式的那个精确范围。它同时带上时点，因为已经
// 离开有效区间的政策不再作答。
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

// ResolveSettlementPolicy 回答一个精确范围由哪种结算方式覆盖。
//
// 两条政策覆盖同一范围即形成`适用冲突`，即使它们结论一致；本上下文点名的是危险的
// 那种：预付与账期同时命中一个费用范围。任选其一等于静默替客户决定要不要先付钱，
// 所以重叠如实报出，交由商业依据的所有方去更正。零候选同样报出而不取默认——
// 客户级兜底正是不许用来填这个缺口的东西。
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
