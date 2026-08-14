package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidCustomerCharge = errors.New("settlement accounting: invalid customer charge")
	ErrChargeAlreadyFinal    = errors.New("settlement accounting: the charge is already confirmed")
	ErrInvalidAdjustment     = errors.New("settlement accounting: invalid charge adjustment")
)

// ChargeStage 是普通客户费用的三段封闭演进（UC-SA-002：费用可以经历预估、暂估和
// 确认）。预估与暂估不当作客户运营应收、最终应收应付或会计收入——那些结论在别的
// 对象上，这个枚举只说费用走到哪一段。
type ChargeStage uint8

const (
	ChargeStageInvalid ChargeStage = iota
	ChargeEstimated
	ChargeProvisional
	ChargeConfirmed
)

func (stage ChargeStage) String() string {
	switch stage {
	case ChargeEstimated:
		return "ESTIMATED"
	case ChargeProvisional:
		return "PROVISIONAL"
	case ChargeConfirmed:
		return "CONFIRMED"
	default:
		return ""
	}
}

// CustomerChargeID 是客户费用的标识。
type CustomerChargeID struct{ requiredValue }

func NewCustomerChargeID(value string) (CustomerChargeID, error) {
	required, err := newRequiredValue("customer charge ID", value)
	return CustomerChargeID{required}, err
}

// SellEvaluationReference 指名 parcel-pricing 的 SELL 方向 PricingEvaluation。
type SellEvaluationReference struct{ requiredValue }

func NewSellEvaluationReference(value string) (SellEvaluationReference, error) {
	required, err := newRequiredValue("sell evaluation reference", value)
	return SellEvaluationReference{required}, err
}

// ConfirmationBasisReference 指名费用类型的确认条件成立依据。
type ConfirmationBasisReference struct{ requiredValue }

func NewConfirmationBasisReference(value string) (ConfirmationBasisReference, error) {
	required, err := newRequiredValue("confirmation basis reference", value)
	return ConfirmationBasisReference{required}, err
}

// CustomerChargeSpec 是形成一笔客户费用所需的全部输入。
type CustomerChargeSpec struct {
	ID          CustomerChargeID
	FeeItem     FeeItemReference
	Evaluation  SellEvaluationReference
	Currency    CurrencyCode
	AmountMinor int64
	Stage       ChargeStage
	FormedAt    time.Time
}

// CustomerCharge 是普通客户费用：预估或暂估起步，确认条件满足后定格。费用只进不退
// ——确认后的金额变化由计价纠错或商业让利这两种调整表达（AT-SA-056：只接受这两种，
// 其他类型转交唯一创建用例），不改写费用本体。
type CustomerCharge struct {
	id           CustomerChargeID
	feeItem      FeeItemReference
	evaluation   SellEvaluationReference
	currency     CurrencyCode
	amountMinor  int64
	stage        ChargeStage
	confirmation ConfirmationBasisReference
	formedAt     time.Time
	confirmedAt  time.Time
}

// FormCustomerCharge 形成一笔预估或暂估费用。直接以`已确认`起步不允许——确认条件
// 的满足是一次显式判断，不是初值。
func FormCustomerCharge(spec CustomerChargeSpec) (CustomerCharge, error) {
	if !spec.ID.valid() ||
		!spec.FeeItem.valid() ||
		!spec.Evaluation.valid() ||
		!spec.Currency.valid() ||
		spec.AmountMinor <= 0 ||
		spec.FormedAt.IsZero() {
		return CustomerCharge{}, ErrInvalidCustomerCharge
	}
	if spec.Stage != ChargeEstimated && spec.Stage != ChargeProvisional {
		return CustomerCharge{}, ErrInvalidCustomerCharge
	}
	return CustomerCharge{
		id:          spec.ID,
		feeItem:     spec.FeeItem,
		evaluation:  spec.Evaluation,
		currency:    spec.Currency,
		amountMinor: spec.AmountMinor,
		stage:       spec.Stage,
		formedAt:    spec.FormedAt.UTC(),
	}, nil
}

func (charge CustomerCharge) ID() CustomerChargeID {
	return charge.id
}

func (charge CustomerCharge) FeeItem() FeeItemReference {
	return charge.feeItem
}

func (charge CustomerCharge) Evaluation() SellEvaluationReference {
	return charge.evaluation
}

func (charge CustomerCharge) Amount() (CurrencyCode, int64) {
	return charge.currency, charge.amountMinor
}

func (charge CustomerCharge) Stage() ChargeStage {
	return charge.stage
}

func (charge CustomerCharge) FormedAt() time.Time {
	return charge.formedAt
}

// Confirmation 只在已确认费用上给出。
func (charge CustomerCharge) Confirmation() (ConfirmationBasisReference, bool) {
	return charge.confirmation, charge.stage == ChargeConfirmed
}

// ConfirmedAt 只在已确认费用上给出。
func (charge CustomerCharge) ConfirmedAt() (time.Time, bool) {
	return charge.confirmedAt, charge.stage == ChargeConfirmed
}

// Confirm 在确认条件满足时定格费用（UC-SA-002 结果契约「费用已确认」）。确认依据
// 必备——没有依据的确认与预估阶段的静默转正分不开；已确认不再确认第二次；确认不
// 改金额，金额变化走调整。
func (charge CustomerCharge) Confirm(
	basis ConfirmationBasisReference,
	at time.Time,
) (CustomerCharge, error) {
	if charge.stage == ChargeConfirmed {
		return CustomerCharge{}, ErrChargeAlreadyFinal
	}
	if !basis.valid() || at.IsZero() || at.Before(charge.formedAt) {
		return CustomerCharge{}, ErrInvalidCustomerCharge
	}
	confirmed := charge
	confirmed.stage = ChargeConfirmed
	confirmed.confirmation = basis
	confirmed.confirmedAt = at.UTC()
	return confirmed, nil
}

// AdjustmentKind 是普通客户费用调整的封闭二值（UC-SA-002 硬句：本用例只对普通费用
// 形成计价纠错或商业让利调整；赔付、索赔退款、追偿与供应商贷项各归其唯一创建用例，
// 在这里没有格——AT-SA-056 的「拒绝无语义调整」靠这个封闭集合落地）。
type AdjustmentKind uint8

const (
	AdjustmentKindInvalid AdjustmentKind = iota
	PricingCorrection
	CommercialConcession
)

func (kind AdjustmentKind) valid() bool {
	return kind == PricingCorrection || kind == CommercialConcession
}

func (kind AdjustmentKind) String() string {
	switch kind {
	case PricingCorrection:
		return "PRICING_CORRECTION"
	case CommercialConcession:
		return "COMMERCIAL_CONCESSION"
	default:
		return ""
	}
}

// AdjustmentDirection 是调整的借/贷方向。
type AdjustmentDirection uint8

const (
	AdjustmentDirectionInvalid AdjustmentDirection = iota
	AdjustmentDebit
	AdjustmentCredit
)

func (direction AdjustmentDirection) valid() bool {
	return direction == AdjustmentDebit || direction == AdjustmentCredit
}

func (direction AdjustmentDirection) String() string {
	switch direction {
	case AdjustmentDebit:
		return "DEBIT"
	case AdjustmentCredit:
		return "CREDIT"
	default:
		return ""
	}
}

// AdjustmentAuthorityReference 指名调整的证据或授权（纠错的证据、让利的有效商业
// 授权）。
type AdjustmentAuthorityReference struct{ requiredValue }

func NewAdjustmentAuthorityReference(value string) (AdjustmentAuthorityReference, error) {
	required, err := newRequiredValue("adjustment authority reference", value)
	return AdjustmentAuthorityReference{required}, err
}

// ChargeAdjustmentID 是调整的标识。
type ChargeAdjustmentID struct{ requiredValue }

func NewChargeAdjustmentID(value string) (ChargeAdjustmentID, error) {
	required, err := newRequiredValue("charge adjustment ID", value)
	return ChargeAdjustmentID{required}, err
}

// ChargeAdjustmentSpec 是形成一笔费用调整所需的全部输入。
type ChargeAdjustmentSpec struct {
	ID          ChargeAdjustmentID
	Charge      CustomerChargeID
	Kind        AdjustmentKind
	Direction   AdjustmentDirection
	Authority   AdjustmentAuthorityReference
	Currency    CurrencyCode
	AmountMinor int64
	FormedAt    time.Time
}

// ChargeAdjustment 是对既有普通客户费用的追加调整：原费用不改写、已发布对账单不
// 改写——调整是新对象带方向，纳入后续账期由 UC-SA-003 处理（AT-SA-055）。
type ChargeAdjustment struct {
	id          ChargeAdjustmentID
	charge      CustomerChargeID
	kind        AdjustmentKind
	direction   AdjustmentDirection
	authority   AdjustmentAuthorityReference
	currency    CurrencyCode
	amountMinor int64
	formedAt    time.Time
}

// FormChargeAdjustment 形成一笔调整。种类封闭二值、证据/授权必备、方向必备——
// 「人工提交『冲销』但未说明语义」在种类这一格就被拒（AT-SA-056）。
func FormChargeAdjustment(spec ChargeAdjustmentSpec) (ChargeAdjustment, error) {
	if !spec.ID.valid() ||
		!spec.Charge.valid() ||
		!spec.Kind.valid() ||
		!spec.Direction.valid() ||
		!spec.Authority.valid() ||
		!spec.Currency.valid() ||
		spec.AmountMinor <= 0 ||
		spec.FormedAt.IsZero() {
		return ChargeAdjustment{}, ErrInvalidAdjustment
	}
	return ChargeAdjustment{
		id:          spec.ID,
		charge:      spec.Charge,
		kind:        spec.Kind,
		direction:   spec.Direction,
		authority:   spec.Authority,
		currency:    spec.Currency,
		amountMinor: spec.AmountMinor,
		formedAt:    spec.FormedAt.UTC(),
	}, nil
}

func (adjustment ChargeAdjustment) ID() ChargeAdjustmentID {
	return adjustment.id
}

func (adjustment ChargeAdjustment) Charge() CustomerChargeID {
	return adjustment.charge
}

func (adjustment ChargeAdjustment) Kind() AdjustmentKind {
	return adjustment.kind
}

func (adjustment ChargeAdjustment) Direction() AdjustmentDirection {
	return adjustment.direction
}

func (adjustment ChargeAdjustment) Authority() AdjustmentAuthorityReference {
	return adjustment.authority
}

func (adjustment ChargeAdjustment) Amount() (CurrencyCode, int64) {
	return adjustment.currency, adjustment.amountMinor
}
