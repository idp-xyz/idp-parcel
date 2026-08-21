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

// CustomerChargeSpec 是形成一笔客户费用所需的全部输入。原币金额、合同结算币金额与
// 换算依据是从它引用的那一个 SELL 评价采用来的一组（CONTEXT：每条费用分别保存原币
// 金额、合同结算币金额及换算依据），不是可各自另取的三件。
type CustomerChargeSpec struct {
	ID                 CustomerChargeID
	FeeItem            FeeItemReference
	Evaluation         SellEvaluationReference
	OriginalCurrency   CurrencyCode
	OriginalMinor      int64
	SettlementCurrency CurrencyCode
	SettlementMinor    int64
	Conversion         ConversionStepReference
	Stage              ChargeStage
	FormedAt           time.Time
}

// CustomerCharge 是普通客户费用：预估或暂估起步，确认条件满足后定格。费用只进不退
// ——确认后的金额变化由计价纠错或商业让利这两种调整表达（AT-SA-056：只接受这两种，
// 其他类型转交唯一创建用例），不改写费用本体。
type CustomerCharge struct {
	id                 CustomerChargeID
	feeItem            FeeItemReference
	evaluation         SellEvaluationReference
	originalCurrency   CurrencyCode
	originalMinor      int64
	settlementCurrency CurrencyCode
	settlementMinor    int64
	conversion         ConversionStepReference
	stage              ChargeStage
	confirmation       ConfirmationBasisReference
	formedAt           time.Time
	confirmedAt        time.Time
}

// FormCustomerCharge 形成一笔预估或暂估费用。直接以`已确认`起步不允许——确认条件
// 的满足是一次显式判断，不是初值。跨币种必备评价内换算步骤、同币种两额必须相等，
// 理由与供应商侧形成门一字不差：没有换算却造出第二个数，只可能是自行取汇率补算出来
// 的。客户费用受三件组约束由票 supplier-expected-cost-correction/04 裁定。
func FormCustomerCharge(spec CustomerChargeSpec) (CustomerCharge, error) {
	if !spec.ID.valid() ||
		!spec.FeeItem.valid() ||
		!spec.Evaluation.valid() ||
		!spec.OriginalCurrency.valid() ||
		spec.OriginalMinor <= 0 ||
		!spec.SettlementCurrency.valid() ||
		spec.SettlementMinor <= 0 ||
		spec.FormedAt.IsZero() {
		return CustomerCharge{}, ErrInvalidCustomerCharge
	}
	if spec.Stage != ChargeEstimated && spec.Stage != ChargeProvisional {
		return CustomerCharge{}, ErrInvalidCustomerCharge
	}
	if spec.OriginalCurrency != spec.SettlementCurrency && !spec.Conversion.valid() {
		return CustomerCharge{}, ErrConversionStepMissing
	}
	if spec.OriginalCurrency == spec.SettlementCurrency &&
		spec.OriginalMinor != spec.SettlementMinor {
		// 同币种两个金额不一致：没有换算却造出了第二个数。
		return CustomerCharge{}, ErrInvalidCustomerCharge
	}
	return CustomerCharge{
		id:                 spec.ID,
		feeItem:            spec.FeeItem,
		evaluation:         spec.Evaluation,
		originalCurrency:   spec.OriginalCurrency,
		originalMinor:      spec.OriginalMinor,
		settlementCurrency: spec.SettlementCurrency,
		settlementMinor:    spec.SettlementMinor,
		conversion:         spec.Conversion,
		stage:              spec.Stage,
		formedAt:           spec.FormedAt.UTC(),
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

func (charge CustomerCharge) OriginalAmount() (CurrencyCode, int64) {
	return charge.originalCurrency, charge.originalMinor
}

// SettlementAmount 是合同结算币的一对：对账单与应收派生用的是它。
func (charge CustomerCharge) SettlementAmount() (CurrencyCode, int64) {
	return charge.settlementCurrency, charge.settlementMinor
}

// Conversion 只在跨币种时给出：换算已由 parcel-pricing 在评价内完成，这里保存的是
// 换算依据，本上下文不重算也不改用其他汇率。
func (charge CustomerCharge) Conversion() (ConversionStepReference, bool) {
	return charge.conversion, charge.conversion.valid()
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

// CommercialAuthorizationReference 指名让利所依据的有效商业授权。纠错与让利的依据
// 分两格而不共用一格：两者语义不同（一个指评价、一个指授权），Kind 之外得有第二个
// 维分辨那个字符串指的是什么——与 CustomerCharge 把 evaluation 与 confirmation 分开
// 建同一条理由（票 supplier-expected-cost-correction/05 第三问裁定）。
type CommercialAuthorizationReference struct{ requiredValue }

func NewCommercialAuthorizationReference(value string) (CommercialAuthorizationReference, error) {
	required, err := newRequiredValue("commercial authorization reference", value)
	return CommercialAuthorizationReference{required}, err
}

// ChargeAdjustmentID 是调整的标识。
type ChargeAdjustmentID struct{ requiredValue }

func NewChargeAdjustmentID(value string) (ChargeAdjustmentID, error) {
	required, err := newRequiredValue("charge adjustment ID", value)
	return ChargeAdjustmentID{required}, err
}

// ChargeAdjustmentSpec 是形成一笔费用调整所需的全部输入。原币金额、结算币金额与
// 换算依据是同源一组（SA CONTEXT：费用调整同受三件组约束）：计价纠错类整组出自它
// 引用的那一个新 SELL 评价；商业让利类的金额与币种出自有效商业授权，授权以非结算币
// 表达让利时换算依据必须随授权内容一并给出——两类都不自行取汇率补算。
type ChargeAdjustmentSpec struct {
	ID                 ChargeAdjustmentID
	Charge             CustomerChargeID
	Kind               AdjustmentKind
	Direction          AdjustmentDirection
	Evaluation         SellEvaluationReference
	Authorization      CommercialAuthorizationReference
	OriginalCurrency   CurrencyCode
	OriginalMinor      int64
	SettlementCurrency CurrencyCode
	SettlementMinor    int64
	Conversion         ConversionStepReference
	FormedAt           time.Time
}

// ChargeAdjustment 是对既有普通客户费用的追加调整：原费用不改写、已发布对账单不
// 改写——调整是新对象带方向，纳入后续账期由 UC-SA-003 处理（AT-SA-055）。金额与
// 费用本体同构地带三件组：同一条链上费用能表达的跨币种，它的调整必须也能表达，
// 否则纠错这一步自己破坏审计链（票 supplier-expected-cost-correction/05 第一问裁定）。
type ChargeAdjustment struct {
	id                 ChargeAdjustmentID
	charge             CustomerChargeID
	kind               AdjustmentKind
	direction          AdjustmentDirection
	evaluation         SellEvaluationReference
	authorization      CommercialAuthorizationReference
	originalCurrency   CurrencyCode
	originalMinor      int64
	settlementCurrency CurrencyCode
	settlementMinor    int64
	conversion         ConversionStepReference
	formedAt           time.Time
}

// FormChargeAdjustment 形成一笔调整。种类封闭二值、方向必备——「人工提交『冲销』
// 但未说明语义」在种类这一格就被拒（AT-SA-056）。依据按种类各占一格且有此无彼：
// 纠错必挂新评价、让利必挂商业授权，填错格与两格齐填同样拒收。跨币种必备换算依据、
// 同币种两额必须相等，两道门与 FormCustomerCharge 一字不差。
func FormChargeAdjustment(spec ChargeAdjustmentSpec) (ChargeAdjustment, error) {
	if !spec.ID.valid() ||
		!spec.Charge.valid() ||
		!spec.Kind.valid() ||
		!spec.Direction.valid() ||
		!spec.OriginalCurrency.valid() ||
		spec.OriginalMinor <= 0 ||
		!spec.SettlementCurrency.valid() ||
		spec.SettlementMinor <= 0 ||
		spec.FormedAt.IsZero() {
		return ChargeAdjustment{}, ErrInvalidAdjustment
	}
	switch spec.Kind {
	case PricingCorrection:
		if !spec.Evaluation.valid() || spec.Authorization.valid() {
			return ChargeAdjustment{}, ErrInvalidAdjustment
		}
	case CommercialConcession:
		if !spec.Authorization.valid() || spec.Evaluation.valid() {
			return ChargeAdjustment{}, ErrInvalidAdjustment
		}
	}
	if spec.OriginalCurrency != spec.SettlementCurrency && !spec.Conversion.valid() {
		return ChargeAdjustment{}, ErrConversionStepMissing
	}
	if spec.OriginalCurrency == spec.SettlementCurrency &&
		spec.OriginalMinor != spec.SettlementMinor {
		// 同币种两个金额不一致：没有换算却造出了第二个数。
		return ChargeAdjustment{}, ErrInvalidAdjustment
	}
	return ChargeAdjustment{
		id:                 spec.ID,
		charge:             spec.Charge,
		kind:               spec.Kind,
		direction:          spec.Direction,
		evaluation:         spec.Evaluation,
		authorization:      spec.Authorization,
		originalCurrency:   spec.OriginalCurrency,
		originalMinor:      spec.OriginalMinor,
		settlementCurrency: spec.SettlementCurrency,
		settlementMinor:    spec.SettlementMinor,
		conversion:         spec.Conversion,
		formedAt:           spec.FormedAt.UTC(),
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

// Evaluation 只在计价纠错类上给出：三件组从这一个评价采用。
func (adjustment ChargeAdjustment) Evaluation() (SellEvaluationReference, bool) {
	return adjustment.evaluation, adjustment.kind == PricingCorrection
}

// Authorization 只在商业让利类上给出。
func (adjustment ChargeAdjustment) Authorization() (CommercialAuthorizationReference, bool) {
	return adjustment.authorization, adjustment.kind == CommercialConcession
}

func (adjustment ChargeAdjustment) OriginalAmount() (CurrencyCode, int64) {
	return adjustment.originalCurrency, adjustment.originalMinor
}

// SettlementAmount 是合同结算币的一对：对账单立单币核对与快照行用的是它。这个身份
// 原先只活在 CutStatementDraft 的比对里，现在类型自己说得出（票 05 第四问裁定）。
func (adjustment ChargeAdjustment) SettlementAmount() (CurrencyCode, int64) {
	return adjustment.settlementCurrency, adjustment.settlementMinor
}

// Conversion 只在跨币种时给出：纠错的换算在评价内完成、让利的随授权内容给出，本
// 上下文不重算也不改用其他汇率。
func (adjustment ChargeAdjustment) Conversion() (ConversionStepReference, bool) {
	return adjustment.conversion, adjustment.conversion.valid()
}
