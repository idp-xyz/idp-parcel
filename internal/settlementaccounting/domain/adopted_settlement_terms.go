package domain

import "fmt"

// AdoptedSettlementTerms 是一次已固定的商业解析采用的结算政策落在结算账户键上的那四维：责任法人、结算相对方、
// 结算币种、结算政策对象。包裹到结算账户的链（ADR-0190 决定三，与受理前财务控制同一条）在商业侧取到它，再到本上下文的
// 账户册上按键找账户。
//
// 收付方向不在这里：政策只说某个范围怎么结，向客户收还是向供应商付由提问的一方定，替提问方定死就成了第二处判断。
// 政策版本也不在这里，理由同 SettlementPolicyReference 头注。
type AdoptedSettlementTerms struct {
	legalEntity  LegalEntityReference
	counterparty SettlementCounterpartyReference
	currency     CurrencyCode
	policy       SettlementPolicyReference
}

// NewAdoptedSettlementTerms 四维全要：缺一维就拼不出账户键，而拼一把半截的键去查账户册只会答「没登记」，把翻译缺陷
// 说成租户没登。
func NewAdoptedSettlementTerms(
	legalEntity LegalEntityReference,
	counterparty SettlementCounterpartyReference,
	currency CurrencyCode,
	policy SettlementPolicyReference,
) (AdoptedSettlementTerms, error) {
	if !legalEntity.valid() || !counterparty.valid() || !currency.valid() || !policy.valid() {
		return AdoptedSettlementTerms{}, fmt.Errorf("%w: adopted settlement terms", ErrBlankValue)
	}
	return AdoptedSettlementTerms{
		legalEntity:  legalEntity,
		counterparty: counterparty,
		currency:     currency,
		policy:       policy,
	}, nil
}

func (terms AdoptedSettlementTerms) LegalEntity() LegalEntityReference { return terms.legalEntity }
func (terms AdoptedSettlementTerms) Counterparty() SettlementCounterpartyReference {
	return terms.counterparty
}
func (terms AdoptedSettlementTerms) Currency() CurrencyCode            { return terms.currency }
func (terms AdoptedSettlementTerms) Policy() SettlementPolicyReference { return terms.policy }

// AccountKey 按提问方给的收付方向拼出结算账户键。
func (terms AdoptedSettlementTerms) AccountKey(direction ChargeDirection) (SettlementAccountKey, error) {
	return NewSettlementAccountKey(terms.legalEntity, terms.counterparty, direction, terms.currency, terms.policy)
}
