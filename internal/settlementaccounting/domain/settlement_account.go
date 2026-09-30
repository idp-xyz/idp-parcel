package domain

import "fmt"

// SettlementPolicyReference 回指商业侧一份结算政策。政策正文与版本生命周期不在本上下文。
type SettlementPolicyReference struct{ requiredValue }

func NewSettlementPolicyReference(value string) (SettlementPolicyReference, error) {
	required, err := newRequiredValue("settlement policy reference", value)
	return SettlementPolicyReference{required}, err
}

// ResponsibilityBasis 是这个账户得以成立的合同或责任依据。本上下文只保存引用，不解释它。
type ResponsibilityBasis struct{ requiredValue }

func NewResponsibilityBasis(value string) (ResponsibilityBasis, error) {
	required, err := newRequiredValue("responsibility basis", value)
	return ResponsibilityBasis{required}, err
}

// AccountStatementTerms 是 CONTEXT 要求每个结算账户写明的对账周期、业务时区、截单时刻与付款条件。
// 四格都是租户原文，本上下文不解析、不补默认：空着的账户立不起来，册上没有行就答未配置。
type AccountStatementTerms struct {
	cycle    requiredValue
	timeZone requiredValue
	cutoff   requiredValue
	payment  requiredValue
}

func NewAccountStatementTerms(cycle, timeZone, cutoff, payment string) (AccountStatementTerms, error) {
	parsedCycle, err := newRequiredValue("reconciliation cycle", cycle)
	if err != nil {
		return AccountStatementTerms{}, err
	}
	parsedZone, err := newRequiredValue("business time zone", timeZone)
	if err != nil {
		return AccountStatementTerms{}, err
	}
	parsedCutoff, err := newRequiredValue("cutoff", cutoff)
	if err != nil {
		return AccountStatementTerms{}, err
	}
	parsedPayment, err := newRequiredValue("payment terms", payment)
	if err != nil {
		return AccountStatementTerms{}, err
	}
	return AccountStatementTerms{
		cycle:    parsedCycle,
		timeZone: parsedZone,
		cutoff:   parsedCutoff,
		payment:  parsedPayment,
	}, nil
}

func (terms AccountStatementTerms) Cycle() string    { return terms.cycle.String() }
func (terms AccountStatementTerms) TimeZone() string { return terms.timeZone.String() }
func (terms AccountStatementTerms) Cutoff() string   { return terms.cutoff.String() }
func (terms AccountStatementTerms) Payment() string  { return terms.payment.String() }

func (terms AccountStatementTerms) same(other AccountStatementTerms) bool {
	return terms.cycle.String() == other.cycle.String() &&
		terms.timeZone.String() == other.timeZone.String() &&
		terms.cutoff.String() == other.cutoff.String() &&
		terms.payment.String() == other.payment.String()
}

// SettlementAccountKey 是「不得合并」的那一组：责任法人、结算相对方、收付方向、结算币种、结算政策。
// 对账周期与付款条件挂在账户上，不另开一把键——同一组再登一个账户就是把两个账户合成一个。
type SettlementAccountKey struct {
	legalEntity  LegalEntityReference
	counterparty SettlementCounterpartyReference
	direction    ChargeDirection
	currency     CurrencyCode
	policy       SettlementPolicyReference
}

func NewSettlementAccountKey(
	legalEntity LegalEntityReference,
	counterparty SettlementCounterpartyReference,
	direction ChargeDirection,
	currency CurrencyCode,
	policy SettlementPolicyReference,
) (SettlementAccountKey, error) {
	if !legalEntity.valid() || !counterparty.valid() || !direction.valid() || !currency.valid() || !policy.valid() {
		return SettlementAccountKey{}, fmt.Errorf("%w: settlement account key", ErrBlankValue)
	}
	return SettlementAccountKey{
		legalEntity:  legalEntity,
		counterparty: counterparty,
		direction:    direction,
		currency:     currency,
		policy:       policy,
	}, nil
}

func (key SettlementAccountKey) LegalEntity() LegalEntityReference { return key.legalEntity }
func (key SettlementAccountKey) Counterparty() SettlementCounterpartyReference {
	return key.counterparty
}
func (key SettlementAccountKey) Direction() ChargeDirection        { return key.direction }
func (key SettlementAccountKey) Currency() CurrencyCode            { return key.currency }
func (key SettlementAccountKey) Policy() SettlementPolicyReference { return key.policy }

func (key SettlementAccountKey) same(other SettlementAccountKey) bool {
	return key.legalEntity.String() == other.legalEntity.String() &&
		key.counterparty.String() == other.counterparty.String() &&
		key.direction == other.direction &&
		key.currency.String() == other.currency.String() &&
		key.policy.String() == other.policy.String()
}

// SettlementAccount 是一笔结算账户登记。固定属性写在键上；实际付款责任方只在与结算相对方不同时另存。
type SettlementAccount struct {
	id             SettlementAccountID
	key            SettlementAccountKey
	payer          SettlementCounterpartyReference
	payerDistinct  bool
	responsibility ResponsibilityBasis
	statement      AccountStatementTerms
}

// NewSettlementAccount 拒空白、拒无效收付方向、拒把付款责任方写成与相对方相同的一格。
// 相同就不另存：另存一个相同的值会让「未分别保存」和「保存了同一方」变成两套口径。
func NewSettlementAccount(
	id SettlementAccountID,
	key SettlementAccountKey,
	payer SettlementCounterpartyReference,
	payerDistinct bool,
	responsibility ResponsibilityBasis,
	statement AccountStatementTerms,
) (SettlementAccount, error) {
	if !id.valid() || !key.legalEntity.valid() || !responsibility.valid() || !statement.cycle.valid() {
		return SettlementAccount{}, fmt.Errorf("%w: settlement account", ErrBlankValue)
	}
	if payerDistinct {
		if !payer.valid() {
			return SettlementAccount{}, fmt.Errorf("%w: settlement account payer", ErrBlankValue)
		}
		if payer.String() == key.counterparty.String() {
			return SettlementAccount{}, fmt.Errorf("%w: payer duplicates counterparty", ErrBlankValue)
		}
	}
	return SettlementAccount{
		id:             id,
		key:            key,
		payer:          payer,
		payerDistinct:  payerDistinct,
		responsibility: responsibility,
		statement:      statement,
	}, nil
}

func (account SettlementAccount) ID() SettlementAccountID   { return account.id }
func (account SettlementAccount) Key() SettlementAccountKey { return account.key }
func (account SettlementAccount) Responsibility() ResponsibilityBasis {
	return account.responsibility
}
func (account SettlementAccount) Statement() AccountStatementTerms { return account.statement }

// Payer 在与结算相对方不同时交回那一方。第二返回值为 false 表示没有另行保存的付款责任方。
func (account SettlementAccount) Payer() (SettlementCounterpartyReference, bool) {
	if !account.payerDistinct {
		return SettlementCounterpartyReference{}, false
	}
	return account.payer, true
}

// SameRegistration 比的是登记内容，不含落库时刻。同一份再交一次是重放；任一格不同就是冲突，不覆盖。
func (account SettlementAccount) SameRegistration(other SettlementAccount) bool {
	if account.id.String() != other.id.String() || !account.key.same(other.key) {
		return false
	}
	if account.payerDistinct != other.payerDistinct {
		return false
	}
	if account.payerDistinct && account.payer.String() != other.payer.String() {
		return false
	}
	return account.responsibility.String() == other.responsibility.String() && account.statement.same(other.statement)
}

// ChargeDirectionFromName 把册上的收付方向词译回领域常量。词与 ChargeDirection.String 以及库上的 CHECK 同字。
func ChargeDirectionFromName(name string) (ChargeDirection, error) {
	switch name {
	case "RECEIVABLE":
		return ChargeReceivable, nil
	case "PAYABLE":
		return ChargePayable, nil
	default:
		return ChargeDirectionInvalid, fmt.Errorf("%w: charge direction %q", ErrBlankValue, name)
	}
}
