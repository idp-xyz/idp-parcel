package domain

import "slices"

// 采用协议声明的收费发生条件在本上下文的样子（ADR-0209）。条件的文法与「哪个原因能配哪个数量形」归 party-commercial：
// 协议发布时那一侧已判过，这里只按协议快照照办——没声明答`未配置`，声明不计费答「协议不计此原因」，计费按数量形
// 数出数量与单位。本上下文不立条件册，立了就是第二个商业权威（ADR-0207 候选与反方）。

// ChargeQuantityForm 是协议给一个计费原因约定的数量形。词与 party-commercial 那一侧逐字相同，由消费侧适配器按字面
// 搬过来。
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

// ParseChargeQuantityForm 按字面认数量形，大小写不折叠。
func ParseChargeQuantityForm(value string) (ChargeQuantityForm, error) {
	for form := PerSegmentQuantity; form <= PerBookingQuantity; form++ {
		if form.String() == value {
			return form, nil
		}
	}
	return ChargeQuantityFormInvalid, ErrInvalidChargeOccurrence
}

// unitWord 是每个数量形落在发生项上的单位。单位由数量形定，不由登记方报：同一形在册上只有一种写法，结算与审计
// 按单位认「按什么计的」才认得准。
func (form ChargeQuantityForm) unitWord() string {
	switch form {
	case PerSegmentQuantity:
		return "SEGMENT"
	case PerPieceQuantity:
		return "PIECE"
	case PerAttemptQuantity:
		return "ATTEMPT"
	case PerBookingQuantity:
		return "BOOKING"
	default:
		return ""
	}
}

// CountsTheScope 答这一形是不是按主要范围计一次（按段、按尝试、按订舱）。这样计的发生项盖住整个范围：同一范围上
// 不论成员怎么分，只能有一项——否则同一段上的包裹逐件各登一项，一段就被算成多段（ADR-0207 越权风险点 4）。
func (form ChargeQuantityForm) CountsTheScope() bool {
	return form == PerSegmentQuantity || form == PerAttemptQuantity || form == PerBookingQuantity
}

// Measure 按数量形数出一项的数量与单位：按范围计的三形是 1，按件是成员数。成员已由事实那一半判过非空、不重。
func (form ChargeQuantityForm) Measure(members []CarriedObjectReference) (int64, QuantityUnitReference, error) {
	if !form.valid() || len(members) == 0 {
		return 0, QuantityUnitReference{}, ErrInvalidChargeOccurrence
	}
	unit := QuantityUnitReference{requiredValue{value: form.unitWord()}}
	if form == PerPieceQuantity {
		return int64(len(members)), unit, nil
	}
	return 1, unit, nil
}

// ScopeCountingUnits 是按范围计的那几形落在册上的单位。登记前问「这个范围是不是已经整体计过」时按单位认册上的项：
// 断言期由登记方声明的单位只要是这几个词之一，同样算整体计过。
func ScopeCountingUnits() []QuantityUnitReference {
	units := make([]QuantityUnitReference, 0, 3)
	for form := PerSegmentQuantity; form <= PerBookingQuantity; form++ {
		if form.CountsTheScope() {
			units = append(units, QuantityUnitReference{requiredValue{value: form.unitWord()}})
		}
	}
	return units
}

// AgreementChargeCondition 是采用协议对一个原因的声明：计费并带数量形，或明确不计费。
type AgreementChargeCondition struct {
	charged bool
	form    ChargeQuantityForm
}

// NewChargedAgreementCondition 立一条计费声明。
func NewChargedAgreementCondition(form ChargeQuantityForm) (AgreementChargeCondition, error) {
	if !form.valid() {
		return AgreementChargeCondition{}, ErrInvalidChargeOccurrence
	}
	return AgreementChargeCondition{charged: true, form: form}, nil
}

// NewUnchargedAgreementCondition 立一条不计费声明。
func NewUnchargedAgreementCondition() AgreementChargeCondition {
	return AgreementChargeCondition{}
}

func (condition AgreementChargeCondition) Charged() bool { return condition.charged }

// QuantityForm 只在计费的声明上给出。
func (condition AgreementChargeCondition) QuantityForm() (ChargeQuantityForm, bool) {
	return condition.form, condition.charged
}

// AgreementChargeConditions 是一版协议声明的全部条件，按原因各至多一条。零值即整节没声明。
type AgreementChargeConditions struct {
	reasons    []ChargeOccurrenceReason
	conditions []AgreementChargeCondition
}

// DeclareAgreementChargeCondition 把一个原因的声明加进来，交回新的一份；同一原因两条、原因集外即拒。
func (conditions AgreementChargeConditions) DeclareAgreementChargeCondition(
	reason ChargeOccurrenceReason,
	condition AgreementChargeCondition,
) (AgreementChargeConditions, error) {
	if !reason.valid() || slices.Contains(conditions.reasons, reason) {
		return AgreementChargeConditions{}, ErrInvalidChargeOccurrence
	}
	return AgreementChargeConditions{
		reasons:    append(slices.Clone(conditions.reasons), reason),
		conditions: append(slices.Clone(conditions.conditions), condition),
	}, nil
}

// For 答协议对这个原因的声明；没声明答 false。
func (conditions AgreementChargeConditions) For(reason ChargeOccurrenceReason) (AgreementChargeCondition, bool) {
	index := slices.Index(conditions.reasons, reason)
	if index < 0 {
		return AgreementChargeCondition{}, false
	}
	return conditions.conditions[index], true
}
