package domain

import (
	"errors"
	"slices"
)

// 本文件是供应商协议正文里「收费发生条件」一节的文法（ADR-0209）：协议按运输收费发生项的原因逐一声明计不计费，
// 计费的原因再声明数量形。
//
// 文法归本上下文，是因为条件是协议的内容：运输履约登记发生项时按它采用的那一版协议判，自己不立条件册——立了
// 就是第二个商业权威（UC-TF-004 输入行把「收费发生条件引用」指给本上下文）。原因词是运输履约的语言，本节只用
// 那四个字面量指名，不解释原因背后的事实怎么取。

var ErrInvalidChargeOccurrenceCondition = errors.New("party commercial: invalid charge occurrence condition")

// TransportChargeReason 是协议声明条件时指名的发生原因，与运输履约「运输收费发生项」的四个原因逐字相同。
// 封闭：协议声明一个运输履约形成不了的原因，等于约定一笔永远不会发生的费用。
type TransportChargeReason uint8

const (
	TransportChargeReasonInvalid TransportChargeReason = iota
	BookingChargeReason
	CancellationChargeReason
	FailedAttemptChargeReason
	ActualFulfillmentChargeReason
)

func (reason TransportChargeReason) valid() bool {
	return reason >= BookingChargeReason && reason <= ActualFulfillmentChargeReason
}

func (reason TransportChargeReason) String() string {
	switch reason {
	case BookingChargeReason:
		return "BOOKING"
	case CancellationChargeReason:
		return "CANCELLATION"
	case FailedAttemptChargeReason:
		return "FAILED_ATTEMPT"
	case ActualFulfillmentChargeReason:
		return "ACTUAL_FULFILLMENT"
	default:
		return ""
	}
}

// TransportChargeReasonNamed 按原词反查；集外与空串答 (零值, false)。
func TransportChargeReasonNamed(name string) (TransportChargeReason, bool) {
	return closedCodeNamed(TransportChargeReason.valid, TransportChargeReason.String, name)
}

// ChargeQuantityForm 是计费原因的数量形：一项发生项按什么计数、单位是什么。
//
// 按段、按尝试、按订舱三形按发生项的主要范围计一次，按件按成员计。数量形是协议的约定，不是登记方的声明：
// 同一段上的包裹逐件各登一项时，由登记方各报「数量 1、单位段」就会把一段算成多段（ADR-0207 越权风险点 4）。
type ChargeQuantityForm uint8

const (
	ChargeQuantityFormInvalid ChargeQuantityForm = iota
	PerSegmentQuantity
	PerPieceQuantity
	PerAttemptQuantity
	PerBookingQuantity
)

func (form ChargeQuantityForm) valid() bool {
	return form >= PerSegmentQuantity && form <= PerBookingQuantity
}

func (form ChargeQuantityForm) String() string {
	switch form {
	case PerSegmentQuantity:
		return "PER_SEGMENT"
	case PerPieceQuantity:
		return "PER_PIECE"
	case PerAttemptQuantity:
		return "PER_ATTEMPT"
	case PerBookingQuantity:
		return "PER_BOOKING"
	default:
		return ""
	}
}

// ChargeQuantityFormNamed 按原词反查；集外与空串答 (零值, false)。
func ChargeQuantityFormNamed(name string) (ChargeQuantityForm, bool) {
	return closedCodeNamed(ChargeQuantityForm.valid, ChargeQuantityForm.String, name)
}

// quantityFormsByReason 是每个原因允许声明的数量形（ADR-0209 决定二，产品策略）。按件对每个原因都成立；按范围计
// 的那一形只配主要范围就是那种东西的原因——失败尝试的范围是一次到场，实际履约的范围是一个段，订舱与取消的范围
// 是一次订舱。别处不另抄这张表：运输履约按数量形计数时不再判原因，它信这里发布时已判过。
var quantityFormsByReason = map[TransportChargeReason][]ChargeQuantityForm{
	BookingChargeReason:           {PerBookingQuantity, PerPieceQuantity},
	CancellationChargeReason:      {PerBookingQuantity, PerPieceQuantity},
	FailedAttemptChargeReason:     {PerAttemptQuantity, PerPieceQuantity},
	ActualFulfillmentChargeReason: {PerSegmentQuantity, PerPieceQuantity},
}

// ChargeOccurrenceCondition 是协议对一个原因的声明：计费并带数量形，或明确不计费。
//
// 「不计费」是一句声明，不是缺席：它说这份协议之下这种事实不产生外部成本，运输履约据此答「协议不计此原因」；
// 协议根本没提这个原因才是`未配置`。两者要人做的事相反，所以不折成一格。
type ChargeOccurrenceCondition struct {
	reason  TransportChargeReason
	charged bool
	form    ChargeQuantityForm
}

// NewChargedOccurrenceCondition 声明这个原因计费、按 form 计数。数量形不配这个原因（ADR-0209 决定二那张表）即拒。
func NewChargedOccurrenceCondition(reason TransportChargeReason, form ChargeQuantityForm) (ChargeOccurrenceCondition, error) {
	if !reason.valid() || !form.valid() || !slices.Contains(quantityFormsByReason[reason], form) {
		return ChargeOccurrenceCondition{}, ErrInvalidChargeOccurrenceCondition
	}
	return ChargeOccurrenceCondition{reason: reason, charged: true, form: form}, nil
}

// NewUnchargedOccurrenceCondition 声明这个原因不计费。不计费没有数量形可带。
func NewUnchargedOccurrenceCondition(reason TransportChargeReason) (ChargeOccurrenceCondition, error) {
	if !reason.valid() {
		return ChargeOccurrenceCondition{}, ErrInvalidChargeOccurrenceCondition
	}
	return ChargeOccurrenceCondition{reason: reason}, nil
}

func (condition ChargeOccurrenceCondition) Reason() TransportChargeReason { return condition.reason }

func (condition ChargeOccurrenceCondition) Charged() bool { return condition.charged }

// QuantityForm 只在计费的声明上给出。
func (condition ChargeOccurrenceCondition) QuantityForm() (ChargeQuantityForm, bool) {
	if !condition.charged {
		return ChargeQuantityFormInvalid, false
	}
	return condition.form, true
}

func (condition ChargeOccurrenceCondition) valid() bool {
	if !condition.reason.valid() {
		return false
	}
	if !condition.charged {
		return condition.form == ChargeQuantityFormInvalid
	}
	return slices.Contains(quantityFormsByReason[condition.reason], condition.form)
}

// ChargeOccurrenceConditions 是一版协议声明的全部收费发生条件，按原因的声明顺序排好、每个原因至多一条。
//
// 至少一条：整节在场却一条不声明，与整节缺席说的是同一句话，一句话只许一种写法，否则同一份正文会折出两个摘要。
// 没声明的原因就是没声明，不由别的原因推出来。
type ChargeOccurrenceConditions struct {
	conditions []ChargeOccurrenceCondition
}

// NewChargeOccurrenceConditions 收拢各原因的声明。空、同一原因两条、某条立不住都拒。
func NewChargeOccurrenceConditions(conditions ...ChargeOccurrenceCondition) (ChargeOccurrenceConditions, error) {
	if len(conditions) == 0 {
		return ChargeOccurrenceConditions{}, ErrInvalidChargeOccurrenceCondition
	}
	ordered := make([]ChargeOccurrenceCondition, 0, len(conditions))
	seen := make(map[TransportChargeReason]struct{}, len(conditions))
	for _, condition := range conditions {
		if !condition.valid() {
			return ChargeOccurrenceConditions{}, ErrInvalidChargeOccurrenceCondition
		}
		if _, duplicate := seen[condition.reason]; duplicate {
			return ChargeOccurrenceConditions{}, ErrInvalidChargeOccurrenceCondition
		}
		seen[condition.reason] = struct{}{}
		ordered = append(ordered, condition)
	}
	slices.SortFunc(ordered, func(left, right ChargeOccurrenceCondition) int {
		return int(left.reason) - int(right.reason)
	})
	return ChargeOccurrenceConditions{conditions: ordered}, nil
}

// For 答协议对这个原因的声明；没声明答 false。
func (conditions ChargeOccurrenceConditions) For(reason TransportChargeReason) (ChargeOccurrenceCondition, bool) {
	for _, condition := range conditions.conditions {
		if condition.reason == reason {
			return condition, true
		}
	}
	return ChargeOccurrenceCondition{}, false
}

// All 按原因的声明顺序交回各条（副本）。
func (conditions ChargeOccurrenceConditions) All() []ChargeOccurrenceCondition {
	return append([]ChargeOccurrenceCondition(nil), conditions.conditions...)
}

// Equal 逐条比：两份声明各条相同即同一句话。各条已按原因排好，不必再排。
func (conditions ChargeOccurrenceConditions) Equal(other ChargeOccurrenceConditions) bool {
	return slices.Equal(conditions.conditions, other.conditions)
}

func (conditions ChargeOccurrenceConditions) valid() bool {
	return len(conditions.conditions) > 0
}
