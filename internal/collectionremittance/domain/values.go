// Package domain 是代收与清分（`collection-remittance`）的领域模型。它拥有代收指令、
// 代收事实的接受判断、代收分户账及其记账、回汇批次、汇付主张与差异事项。
//
// 本包不依赖 HTTP 与 pgx：判断在这里，落库在 adapters。全部金额都是**客户的钱**——
// 受托保管口径，不是经营收入；COD 服务费与协议抵扣归 settlement-accounting，本包
// 不表达它们，也不在账内与本金净额。
//
// 汇率、回汇周期、手续费与支付通道不进本包：它们属实例半边，写成常量就是替租户拟
// 商业口径（判据见 CONTEXT 的「汇率、回汇周期、手续费与支付通道属运营企业的商业与
// 实例参数」那一句）。
package domain

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrBlankValue            = errors.New("collection remittance: blank value")
	ErrInvalidInstruction    = errors.New("collection remittance: invalid collection instruction")
	ErrInvalidCollectionFact = errors.New("collection remittance: invalid collection fact")
	ErrInvalidSubledger      = errors.New("collection remittance: invalid subledger")
	ErrInvalidPosting        = errors.New("collection remittance: invalid subledger posting")
	ErrInvalidBatch          = errors.New("collection remittance: invalid remittance batch")
	ErrInvalidDiscrepancy    = errors.New("collection remittance: invalid discrepancy item")

	// ErrPositionUnderfunded 是「该资金位置余额不够这笔记账扣」。它不是输入不合法：
	// 输入完全可能是对的，只是此刻账上还没有那么多钱——处置是等实收或先清分，不是
	// 改请求。透支后再补是本上下文明确不给的路径（CONTEXT「任一资金位置的余额不得
	// 为负」）。
	ErrPositionUnderfunded = errors.New("collection remittance: fund position has insufficient balance")

	// ErrConservationBroken 是分配守恒被破：非外部位置余额之和不等于入账之和。它只
	// 可能来自账面本身坏了（有人绕过记账写了余额，或记账被改写过），不来自单笔输入。
	ErrConservationBroken = errors.New("collection remittance: subledger allocation is not conserved")
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

// TenantID 指名租户。它是入参而不是登记内容——本上下文不做接入认证。
type TenantID struct{ requiredValue }

func NewTenantID(value string) (TenantID, error) {
	required, err := newRequiredValue("tenant ID", value)
	return TenantID{required}, err
}

// CustomerReference 指名货主客户，即代收本金的受托归属方。
type CustomerReference struct{ requiredValue }

func NewCustomerReference(value string) (CustomerReference, error) {
	required, err := newRequiredValue("customer reference", value)
	return CustomerReference{required}, err
}

// LegalEntityReference 指名承担代收义务的运营企业责任法人。
type LegalEntityReference struct{ requiredValue }

func NewLegalEntityReference(value string) (LegalEntityReference, error) {
	required, err := newRequiredValue("legal entity reference", value)
	return LegalEntityReference{required}, err
}

// CurrencyCode 指名币种。它在分户账键上，因此一本账内不发生换算——本包没有任何
// 折算入口，跨币种关系由两本分别成立的分户账加外部换算依据表达。
type CurrencyCode struct{ requiredValue }

func NewCurrencyCode(value string) (CurrencyCode, error) {
	required, err := newRequiredValue("currency code", value)
	return CurrencyCode{required}, err
}

// CollectionChannelReference 指名执行代收的渠道。真实渠道属实例半边，这里只是引用
// 词形，不内置任何渠道默认。
type CollectionChannelReference struct{ requiredValue }

func NewCollectionChannelReference(value string) (CollectionChannelReference, error) {
	required, err := newRequiredValue("collection channel reference", value)
	return CollectionChannelReference{required}, err
}

// CollectionInstructionID 指名一条代收指令。
type CollectionInstructionID struct{ requiredValue }

func NewCollectionInstructionID(value string) (CollectionInstructionID, error) {
	required, err := newRequiredValue("collection instruction ID", value)
	return CollectionInstructionID{required}, err
}

// ParcelReference 指名代收义务所覆盖的包裹。包裹身份归 parcel-shipment，这里只引用。
type ParcelReference struct{ requiredValue }

func NewParcelReference(value string) (ParcelReference, error) {
	required, err := newRequiredValue("parcel reference", value)
	return ParcelReference{required}, err
}

// ServiceRequirementReference 指名客户代收服务要求快照。快照归 parcel-shipment 拥有，
// 代收指令引用它作为义务成立的依据；没有它就没有受托关系，因此它必填。
type ServiceRequirementReference struct{ requiredValue }

func NewServiceRequirementReference(value string) (ServiceRequirementReference, error) {
	required, err := newRequiredValue("service requirement reference", value)
	return ServiceRequirementReference{required}, err
}

// CustodyBasisReference 指名受托保管依据，即 party-commercial 拥有的代收商业责任。
type CustodyBasisReference struct{ requiredValue }

func NewCustodyBasisReference(value string) (CustodyBasisReference, error) {
	required, err := newRequiredValue("custody basis reference", value)
	return CustodyBasisReference{required}, err
}

// CollectionFactID 指名一条代收事实。
type CollectionFactID struct{ requiredValue }

func NewCollectionFactID(value string) (CollectionFactID, error) {
	required, err := newRequiredValue("collection fact ID", value)
	return CollectionFactID{required}, err
}

// EvidenceReference 指名代收事实的来源证据。履约侧代收证据、渠道代收报告与渠道回款
// 通知归 transport-fulfillment，真实到账事实归银行或支付系统；本上下文只引用。
type EvidenceReference struct{ requiredValue }

func NewEvidenceReference(value string) (EvidenceReference, error) {
	required, err := newRequiredValue("evidence reference", value)
	return EvidenceReference{required}, err
}

// PostingID 指名一笔分户账记账。
type PostingID struct{ requiredValue }

func NewPostingID(value string) (PostingID, error) {
	required, err := newRequiredValue("posting ID", value)
	return PostingID{required}, err
}

// BasisReference 指名一笔记账所依据的那一行：代收事实、回汇批次、差异事项或被冲正
// 的原记账。依据种类由 PostingBasisKind 声明，两者成对才说得清依据指向哪张册子。
type BasisReference struct{ requiredValue }

func NewBasisReference(value string) (BasisReference, error) {
	required, err := newRequiredValue("basis reference", value)
	return BasisReference{required}, err
}

// RemittanceBatchID 指名一个回汇批次。
type RemittanceBatchID struct{ requiredValue }

func NewRemittanceBatchID(value string) (RemittanceBatchID, error) {
	required, err := newRequiredValue("remittance batch ID", value)
	return RemittanceBatchID{required}, err
}

// DiscrepancyItemID 指名一项差异事项。
type DiscrepancyItemID struct{ requiredValue }

func NewDiscrepancyItemID(value string) (DiscrepancyItemID, error) {
	required, err := newRequiredValue("discrepancy item ID", value)
	return DiscrepancyItemID{required}, err
}

// Money 是一笔带币种的正金额。零金额构造不出：本上下文的每一笔登记与记账都表达一次
// 真实的资金主张，零额记账在账上什么也没说，却会让「已处置」看起来成立。
type Money struct {
	currency    CurrencyCode
	amountMinor int64
}

func NewMoney(currency CurrencyCode, amountMinor int64) (Money, error) {
	if !currency.valid() {
		return Money{}, fmt.Errorf("%w: currency code", ErrBlankValue)
	}
	if amountMinor <= 0 {
		return Money{}, fmt.Errorf("%w: amount must be positive, got %d", ErrBlankValue, amountMinor)
	}
	return Money{currency: currency, amountMinor: amountMinor}, nil
}

func (money Money) Currency() CurrencyCode { return money.currency }

func (money Money) AmountMinor() int64 { return money.amountMinor }

func (money Money) valid() bool { return money.currency.valid() && money.amountMinor > 0 }
