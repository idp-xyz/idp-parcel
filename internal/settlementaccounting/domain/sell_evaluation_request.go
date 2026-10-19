package domain

import (
	"errors"
	"fmt"
	"time"
)

var ErrInvalidSellEvaluationRequest = errors.New("settlement accounting: invalid sell evaluation request")

// SellEvaluationRequestID 是 SELL 评价请求的铸造身份（ADR-0190 决定一）。与 EvaluationRequestID 分成两种类型：
// BUY 的请求身份一字不动，两种 ID 若共一个类型，SELL 消费门按回指读请求时拿 BUY 的 ID 来查也编译得过。
type SellEvaluationRequestID struct{ requiredValue }

func NewSellEvaluationRequestID(value string) (SellEvaluationRequestID, error) {
	required, err := newRequiredValue("sell evaluation request ID", value)
	return SellEvaluationRequestID{required}, err
}

// ParcelReference 指名一件包裹，取 parcel-shipment 的包裹标识原样。本上下文不解析它的形，只拿它当自然键的
// 一维与回查键。
type ParcelReference struct{ requiredValue }

func NewParcelReference(value string) (ParcelReference, error) {
	required, err := newRequiredValue("parcel reference", value)
	return ParcelReference{required}, err
}

// SellEvaluationRequestNaturalKey 是 SELL 评价请求的自然键：同一件包裹在同一计费时点只发起一次（ADR-0190 决定二
// 「一件一次」）。租户在登记册键上另带。它只守幂等——账户、币种、计价范围与来源事实都不进键：同一件包裹在同一
// 时点再来一次，带的成分即便不同也是重放，答`已存在`，不另起一份。
type SellEvaluationRequestNaturalKey struct {
	Parcel ParcelReference
	Moment ParcelBillingMoment
}

// SellEvaluationRequestSpec 是提交一份 SELL 评价请求所需的全部输入。
//
// SettlementCurrency 是登记请求时从结算账户行快照下来的结算币种。账户行是不可变元组（ADR-0158），快照与册上永远
// 一致；计价建快照时取请求上这一格，不必再跨上下文去读账户册。
//
// Source 是来源事实引用：`有效网络收寄已采用`那一格是收寄采用键，`有效交付已终局`那一格是终局键。两种键在本上下文
// 都只是引用，所以共一个类型；它是计价输入的来源事实，不是评价本身（SourceFactReference 头注）。
type SellEvaluationRequestSpec struct {
	ID                 SellEvaluationRequestID
	Parcel             ParcelReference
	Account            SettlementAccountID
	SettlementCurrency CurrencyCode
	PricingScope       EstimatePricingScope
	Moment             ParcelBillingMoment
	Source             SourceFactReference
	PricingBasisAt     time.Time
}

// SellEvaluationRequest 是结算向 parcel-pricing 提出的一次 SELL·客户费用评价：哪件包裹、哪个结算账户与它的结算币种、
// 按哪个计价范围、在哪一计费时点、凭哪件来源事实、以哪一刻为计价基准。类型上没有金额、换算或价卡——那些整组归
// 评价（ADR-0190 决定一），请求只记「问了什么」。
type SellEvaluationRequest struct {
	id                 SellEvaluationRequestID
	naturalKey         SellEvaluationRequestNaturalKey
	account            SettlementAccountID
	settlementCurrency CurrencyCode
	pricingScope       EstimatePricingScope
	source             SourceFactReference
	pricingBasisAt     time.Time
}

// SubmitSellEvaluationRequest 形成一份 SELL 评价请求。每一格都必备：计价按（租户、计价范围、SELL、客户费用、计价基准
// 时点）选卡，缺范围或基准时点就选不了卡；缺结算币种，评价无从换算到结算币种；缺账户，形成的费用落不到结算身份上；
// 缺来源事实，评价的输入无从追溯。一份缺格的请求登进册，只会让按 ID 回读的消费者落空。
func SubmitSellEvaluationRequest(spec SellEvaluationRequestSpec) (SellEvaluationRequest, error) {
	if !spec.ID.valid() || !spec.Parcel.valid() || !spec.Account.valid() || !spec.SettlementCurrency.valid() ||
		!spec.PricingScope.valid() || !spec.Moment.valid() || !spec.Source.valid() || spec.PricingBasisAt.IsZero() {
		return SellEvaluationRequest{}, ErrInvalidSellEvaluationRequest
	}
	return SellEvaluationRequest{
		id:                 spec.ID,
		naturalKey:         SellEvaluationRequestNaturalKey{Parcel: spec.Parcel, Moment: spec.Moment},
		account:            spec.Account,
		settlementCurrency: spec.SettlementCurrency,
		pricingScope:       spec.PricingScope,
		source:             spec.Source,
		pricingBasisAt:     spec.PricingBasisAt.UTC(),
	}, nil
}

func (request SellEvaluationRequest) ID() SellEvaluationRequestID { return request.id }

func (request SellEvaluationRequest) Parcel() ParcelReference { return request.naturalKey.Parcel }

func (request SellEvaluationRequest) Account() SettlementAccountID { return request.account }

func (request SellEvaluationRequest) SettlementCurrency() CurrencyCode {
	return request.settlementCurrency
}

func (request SellEvaluationRequest) PricingScope() EstimatePricingScope { return request.pricingScope }

func (request SellEvaluationRequest) Moment() ParcelBillingMoment { return request.naturalKey.Moment }

func (request SellEvaluationRequest) Source() SourceFactReference { return request.source }

func (request SellEvaluationRequest) PricingBasisAt() time.Time { return request.pricingBasisAt }

func (request SellEvaluationRequest) NaturalKey() SellEvaluationRequestNaturalKey {
	return request.naturalKey
}

// SellEvaluationRequestState 是请求册上的状态格（ADR-0190 决定五）。登记时是`已提交`；评价回来之后由形成那一半改记
// 其余各格之一：形成了预估客户费用是`已形成`，映不全或映到多个费用项目是`待判断`（AT-SA-174），评价不可计价或
// 适用冲突各记一格。状态留在册上，入账不重投之后仍可查。
type SellEvaluationRequestState uint8

const (
	SellEvaluationRequestStateInvalid SellEvaluationRequestState = iota
	SellRequestSubmitted
	SellRequestFormed
	SellRequestPendingJudgment
	SellRequestUnratable
	SellRequestConflict
)

func (state SellEvaluationRequestState) String() string {
	switch state {
	case SellRequestSubmitted:
		return "SUBMITTED"
	case SellRequestFormed:
		return "FORMED"
	case SellRequestPendingJudgment:
		return "PENDING_JUDGMENT"
	case SellRequestUnratable:
		return "UNRATABLE"
	case SellRequestConflict:
		return "CONFLICT"
	default:
		return ""
	}
}

// RecordsFormedCharge 报告这一格是否带形成的预估客户费用标识：只有`已形成`带，其余各格都不带。按包裹找当前有效的
// 预估客户费用就走这一对（状态、费用标识）；库上的 CHECK 守同一条。
func (state SellEvaluationRequestState) RecordsFormedCharge() bool {
	return state == SellRequestFormed
}

// SellEvaluationRequestStateFromName 把册上的状态词译回领域常量。词表外的拒：册上出现本上下文不认识的状态只可能是
// 适配器或迁移的 bug，读成某一格会让一份请求的去向看错。
func SellEvaluationRequestStateFromName(name string) (SellEvaluationRequestState, error) {
	for _, candidate := range []SellEvaluationRequestState{
		SellRequestSubmitted, SellRequestFormed, SellRequestPendingJudgment, SellRequestUnratable, SellRequestConflict,
	} {
		if candidate.String() == name {
			return candidate, nil
		}
	}
	return SellEvaluationRequestStateInvalid, fmt.Errorf("%w: sell evaluation request state %q", ErrInvalidSellEvaluationRequest, name)
}

// estimatedCustomerChargePrefix 是由 SELL 请求派生的预估客户费用标识的类别段。它不是铸造前缀，但照样以 Prefix
// 结尾，让全仓标识前缀门禁替它守唯一：费用标识与各上下文铸造的标识在日志与工单里混看，撞名的代价一样。
const estimatedCustomerChargePrefix = "SELLCC"

// EstimatedCustomerChargeID 是这份请求至多形成的那一笔预估客户费用的标识（ADR-0190 决定五「费用标识由 SELL 请求
// 标识确定性派生，同一请求至多一笔预估客户费用」）：类别段 `SELLCC`、一道横线、请求标识原文。
//
// 派生而不铸造：同一封评价已记录信封重投、或两拍并发，算出的都是同一个标识，费用库主键（租户、费用标识）当场挡住
// 第二笔，不必另立一张「请求 → 费用」对照表。拼接而不取指纹：请求标识由本上下文铸造、长度有界，拼出来仍有界，
// 读的人从费用标识一眼看得出它出自哪份请求。
func (request SellEvaluationRequest) EstimatedCustomerChargeID() CustomerChargeID {
	return CustomerChargeID{requiredValue{value: estimatedCustomerChargePrefix + "-" + request.id.String()}}
}

// SellEvaluationRequestConclusion 是形成那一半给一份请求下的结论：状态格，以及只在`已形成`时才有的那笔预估客户
// 费用标识。两格成对构造——拆开写就能写出「已形成却没有费用」或「待判断却带着费用」，库上的 CHECK 守的是同一条，
// 这里让它在写之前就过不了构造。`已提交`不是结论：那是登记时的起点，形成那一半不会把请求改回去。
type SellEvaluationRequestConclusion struct {
	state  SellEvaluationRequestState
	charge CustomerChargeID
}

// NewSellEvaluationRequestConclusion 收状态格与费用标识。`已形成`必须带费用标识，`待判断`、`不可计价`、`冲突`必须
// 不带（传零值）；`已提交`与词表外的状态一律拒。
func NewSellEvaluationRequestConclusion(
	state SellEvaluationRequestState,
	charge CustomerChargeID,
) (SellEvaluationRequestConclusion, error) {
	switch state {
	case SellRequestFormed, SellRequestPendingJudgment, SellRequestUnratable, SellRequestConflict:
	default:
		return SellEvaluationRequestConclusion{}, fmt.Errorf("%w: %q is not a conclusion", ErrInvalidSellEvaluationRequest, state.String())
	}
	if state.RecordsFormedCharge() != charge.valid() {
		return SellEvaluationRequestConclusion{}, fmt.Errorf("%w: %s with charge %q", ErrInvalidSellEvaluationRequest, state, charge.String())
	}
	return SellEvaluationRequestConclusion{state: state, charge: charge}, nil
}

func (conclusion SellEvaluationRequestConclusion) State() SellEvaluationRequestState {
	return conclusion.state
}

// FormedCharge 交回`已形成`时的费用标识；其余各格第二个返回值为假。
func (conclusion SellEvaluationRequestConclusion) FormedCharge() (CustomerChargeID, bool) {
	return conclusion.charge, conclusion.state.RecordsFormedCharge()
}

// Same 比两份结论的状态格与费用标识。册上已是同一份结论时，再记一次是重放。
func (conclusion SellEvaluationRequestConclusion) Same(state SellEvaluationRequestState, charge CustomerChargeID) bool {
	return conclusion.state == state && conclusion.charge.String() == charge.String()
}
