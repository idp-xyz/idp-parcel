package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidAdvanceAssessment = errors.New("settlement accounting: invalid advance assessment")
	// ErrAdvanceNotEstablished：客户代垫回收只立在**已成立**的实际代垫上——代垫与回收
	// 分层，未成立、待判断或冲突的范围形成不了回收（UC-SA-001 结果契约）。
	ErrAdvanceNotEstablished     = errors.New("settlement accounting: the actual advance is not established")
	ErrInvalidAdvanceRecovery    = errors.New("settlement accounting: invalid customer advance recovery")
	ErrInvalidRecoveryAdjustment = errors.New("settlement accounting: invalid recovery adjustment")
)

// TaxObligationReference 指名 customs-compliance 拥有的监管税费义务。SA 只引用其核定
// 范围与金额快照，不重算税费。
type TaxObligationReference struct{ requiredValue }

func NewTaxObligationReference(value string) (TaxObligationReference, error) {
	required, err := newRequiredValue("tax obligation reference", value)
	return TaxObligationReference{required}, err
}

// AdvancePayerReference 指名实际付款方（运营责任法人、客户、收件人……）。付款方是
// 代垫判断的核心轴：客户或收件人直接付款时代垫不成立（AT-SA-006）。
type AdvancePayerReference struct{ requiredValue }

func NewAdvancePayerReference(value string) (AdvancePayerReference, error) {
	required, err := newRequiredValue("advance payer reference", value)
	return AdvancePayerReference{required}, err
}

// AdvanceResponsibilityReference 指名运营企业付款责任的依据。
type AdvanceResponsibilityReference struct{ requiredValue }

func NewAdvanceResponsibilityReference(value string) (AdvanceResponsibilityReference, error) {
	required, err := newRequiredValue("advance responsibility reference", value)
	return AdvanceResponsibilityReference{required}, err
}

// AssessmentBasisReference 指名负向依据、待判断缺口或冲突范围。
type AssessmentBasisReference struct{ requiredValue }

func NewAssessmentBasisReference(value string) (AssessmentBasisReference, error) {
	required, err := newRequiredValue("assessment basis reference", value)
	return AssessmentBasisReference{required}, err
}

// AdvanceAssessmentID 是实际代垫判断的稳定身份。
type AdvanceAssessmentID struct{ requiredValue }

func NewAdvanceAssessmentID(value string) (AdvanceAssessmentID, error) {
	required, err := newRequiredValue("advance assessment ID", value)
	return AdvanceAssessmentID{required}, err
}

// AdvanceAssessmentVersion 是判断版本：税费或资金更正形成新判断版本，不按接收顺序覆盖。
type AdvanceAssessmentVersion struct{ requiredValue }

func NewAdvanceAssessmentVersion(value string) (AdvanceAssessmentVersion, error) {
	required, err := newRequiredValue("advance assessment version", value)
	return AdvanceAssessmentVersion{required}, err
}

// AdvanceVerdict 是实际代垫判断的封闭四值（UC-SA-001 结果契约）：成立、不成立、待判断、
// 冲突。部分付款不形成吞并全部范围的「部分代垫」状态——各金额范围分别判。
type AdvanceVerdict uint8

const (
	AdvanceVerdictInvalid AdvanceVerdict = iota
	AdvanceEstablished
	AdvanceNotEstablishedVerdict
	AdvanceUndecided
	AdvanceConflicting
)

func (verdict AdvanceVerdict) valid() bool {
	return verdict >= AdvanceEstablished && verdict <= AdvanceConflicting
}

func (verdict AdvanceVerdict) String() string {
	switch verdict {
	case AdvanceEstablished:
		return "ESTABLISHED"
	case AdvanceNotEstablishedVerdict:
		return "NOT_ESTABLISHED"
	case AdvanceUndecided:
		return "UNDECIDED"
	case AdvanceConflicting:
		return "CONFLICTING"
	default:
		return ""
	}
}

// ActualAdvanceAssessmentSpec 是形成一次实际代垫判断所需的全部输入。
type ActualAdvanceAssessmentSpec struct {
	ID             AdvanceAssessmentID
	Obligation     TaxObligationReference
	Verdict        AdvanceVerdict
	FundsFact      FundsFactReference
	Payer          AdvancePayerReference
	Responsibility AdvanceResponsibilityReference
	Basis          AssessmentBasisReference
	Currency       CurrencyCode
	AmountMinor    int64
	Version        AdvanceAssessmentVersion
	JudgedAt       time.Time
}

// ActualAdvanceAssessment 是运营企业是否实际替他方付款的逐金额范围判断（UC-SA-001）。
// 税费核定归 customs-compliance、真实付款归银行/支付系统——本类型只引用两者并判成立
// 与否，没有任何重算税费或制造付款的入口；预付余额、单独的付款事实或单独的核定都进
// 不了「成立」那一格（AT-SA-002/003/004）。
type ActualAdvanceAssessment struct {
	id             AdvanceAssessmentID
	obligation     TaxObligationReference
	verdict        AdvanceVerdict
	fundsFact      FundsFactReference
	payer          AdvancePayerReference
	responsibility AdvanceResponsibilityReference
	basis          AssessmentBasisReference
	currency       CurrencyCode
	amountMinor    int64
	version        AdvanceAssessmentVersion
	judgedAt       time.Time
}

// AssessActualAdvance 逐格校验四值各自的完备性：
//   - `成立`必须同时有资金事实引用、付款方与付款责任依据——三者缺一，说的就不是
//     「运营企业已实际替他方付款」；不携带负向/缺口依据；
//   - `不成立`必须带负向依据——资料缺失伪装不成立被这格拒绝（那是待判断）；
//   - `待判断`/`冲突`必须带缺口或冲突依据。
func AssessActualAdvance(spec ActualAdvanceAssessmentSpec) (ActualAdvanceAssessment, error) {
	if !spec.ID.valid() ||
		!spec.Obligation.valid() ||
		!spec.Verdict.valid() ||
		!spec.Currency.valid() ||
		spec.AmountMinor <= 0 ||
		!spec.Version.valid() ||
		spec.JudgedAt.IsZero() {
		return ActualAdvanceAssessment{}, ErrInvalidAdvanceAssessment
	}
	if spec.Verdict == AdvanceEstablished {
		if !spec.FundsFact.valid() || !spec.Payer.valid() || !spec.Responsibility.valid() {
			return ActualAdvanceAssessment{}, ErrInvalidAdvanceAssessment
		}
		if spec.Basis.valid() {
			return ActualAdvanceAssessment{}, ErrInvalidAdvanceAssessment
		}
	} else if !spec.Basis.valid() {
		return ActualAdvanceAssessment{}, ErrInvalidAdvanceAssessment
	}
	return ActualAdvanceAssessment{
		id:             spec.ID,
		obligation:     spec.Obligation,
		verdict:        spec.Verdict,
		fundsFact:      spec.FundsFact,
		payer:          spec.Payer,
		responsibility: spec.Responsibility,
		basis:          spec.Basis,
		currency:       spec.Currency,
		amountMinor:    spec.AmountMinor,
		version:        spec.Version,
		judgedAt:       spec.JudgedAt.UTC(),
	}, nil
}

func (assessment ActualAdvanceAssessment) ID() AdvanceAssessmentID {
	return assessment.id
}

func (assessment ActualAdvanceAssessment) Obligation() TaxObligationReference {
	return assessment.obligation
}

func (assessment ActualAdvanceAssessment) Verdict() AdvanceVerdict {
	return assessment.verdict
}

// FundsFact 只在成立时必然给出——成立的代垫必须锚在真实付款事实引用上。
func (assessment ActualAdvanceAssessment) FundsFact() (FundsFactReference, bool) {
	if !assessment.fundsFact.valid() {
		return FundsFactReference{}, false
	}
	return assessment.fundsFact, true
}

// Party 交回付款方引用。方法名避开「pay」——结构防线把带 pay 的方法当成制造付款的入口。
func (assessment ActualAdvanceAssessment) Party() (AdvancePayerReference, bool) {
	if !assessment.payer.valid() {
		return AdvancePayerReference{}, false
	}
	return assessment.payer, true
}

func (assessment ActualAdvanceAssessment) Responsibility() (AdvanceResponsibilityReference, bool) {
	if !assessment.responsibility.valid() {
		return AdvanceResponsibilityReference{}, false
	}
	return assessment.responsibility, true
}

// Basis 在不成立/待判断/冲突时交回依据；成立没有它。
func (assessment ActualAdvanceAssessment) Basis() (AssessmentBasisReference, bool) {
	if !assessment.basis.valid() {
		return AssessmentBasisReference{}, false
	}
	return assessment.basis, true
}

func (assessment ActualAdvanceAssessment) Amount() (CurrencyCode, int64) {
	return assessment.currency, assessment.amountMinor
}

func (assessment ActualAdvanceAssessment) Version() AdvanceAssessmentVersion {
	return assessment.version
}

func (assessment ActualAdvanceAssessment) JudgedAt() time.Time {
	return assessment.judgedAt
}

// RecoveryCustomerReference 指名承担回收责任的货主客户账户。
type RecoveryCustomerReference struct{ requiredValue }

func NewRecoveryCustomerReference(value string) (RecoveryCustomerReference, error) {
	required, err := newRequiredValue("recovery customer reference", value)
	return RecoveryCustomerReference{required}, err
}

// ContractResponsibilityReference 指名客户最终承担责任的合同依据。回收不是从代垫
// 自动长出来的——没有合同依据就没有回收（UC-SA-001 步骤 5 的独立判断）。
type ContractResponsibilityReference struct{ requiredValue }

func NewContractResponsibilityReference(value string) (ContractResponsibilityReference, error) {
	required, err := newRequiredValue("contract responsibility reference", value)
	return ContractResponsibilityReference{required}, err
}

// AdvanceRecoveryID 是客户代垫回收的稳定身份。
type AdvanceRecoveryID struct{ requiredValue }

func NewAdvanceRecoveryID(value string) (AdvanceRecoveryID, error) {
	required, err := newRequiredValue("advance recovery ID", value)
	return AdvanceRecoveryID{required}, err
}

// CustomerAdvanceRecovery 是客户对已成立实际代垫承担的运营金额责任（UC-SA-001「客户
// 代垫回收已形成」）。它不表示客户已付款、已核销或已开票——那些在 UC-SA-005 与外部
// 财税系统；类型上没有任何收款或核销字段。
type CustomerAdvanceRecovery struct {
	id            AdvanceRecoveryID
	assessment    AdvanceAssessmentID
	customer      RecoveryCustomerReference
	contractBasis ContractResponsibilityReference
	account       SettlementAccountID
	currency      CurrencyCode
	amountMinor   int64
	formedAt      time.Time
}

// FormCustomerAdvanceRecovery 依附已成立的代垫判断形成回收。合同依据必备；回收金额
// 不得超过代垫金额——超出监管税费和合同责任范围的金额不得为了对平自动转为客户回收
// （UC-SA-001 明句）。
func FormCustomerAdvanceRecovery(
	assessment ActualAdvanceAssessment,
	recovery AdvanceRecoveryID,
	customer RecoveryCustomerReference,
	contractBasis ContractResponsibilityReference,
	account SettlementAccountID,
	amountMinor int64,
	formedAt time.Time,
) (CustomerAdvanceRecovery, error) {
	if !assessment.id.valid() {
		return CustomerAdvanceRecovery{}, ErrInvalidAdvanceRecovery
	}
	if assessment.verdict != AdvanceEstablished {
		return CustomerAdvanceRecovery{}, ErrAdvanceNotEstablished
	}
	if !recovery.valid() || !customer.valid() || !contractBasis.valid() || !account.valid() ||
		formedAt.IsZero() || formedAt.Before(assessment.judgedAt) {
		return CustomerAdvanceRecovery{}, ErrInvalidAdvanceRecovery
	}
	if amountMinor <= 0 || amountMinor > assessment.amountMinor {
		return CustomerAdvanceRecovery{}, ErrInvalidAdvanceRecovery
	}
	return CustomerAdvanceRecovery{
		id:            recovery,
		assessment:    assessment.id,
		customer:      customer,
		contractBasis: contractBasis,
		account:       account,
		currency:      assessment.currency,
		amountMinor:   amountMinor,
		formedAt:      formedAt.UTC(),
	}, nil
}

func (recovery CustomerAdvanceRecovery) ID() AdvanceRecoveryID {
	return recovery.id
}

func (recovery CustomerAdvanceRecovery) Assessment() AdvanceAssessmentID {
	return recovery.assessment
}

func (recovery CustomerAdvanceRecovery) Customer() RecoveryCustomerReference {
	return recovery.customer
}

func (recovery CustomerAdvanceRecovery) ContractBasis() ContractResponsibilityReference {
	return recovery.contractBasis
}

func (recovery CustomerAdvanceRecovery) Account() SettlementAccountID {
	return recovery.account
}

func (recovery CustomerAdvanceRecovery) Amount() (CurrencyCode, int64) {
	return recovery.currency, recovery.amountMinor
}

func (recovery CustomerAdvanceRecovery) FormedAt() time.Time {
	return recovery.formedAt
}

// RehydrateCustomerAdvanceRecoverySpec 是回收行在库里的样子。FormCustomerAdvanceRecovery
// 要一份已成立评估才能限量，而行里只有回收本身——评估裁决与金额上限是写入时已经判过的。
type RehydrateCustomerAdvanceRecoverySpec struct {
	ID            AdvanceRecoveryID
	Assessment    AdvanceAssessmentID
	Customer      RecoveryCustomerReference
	ContractBasis ContractResponsibilityReference
	Account       SettlementAccountID
	Currency      CurrencyCode
	AmountMinor   int64
	FormedAt      time.Time
}

// RehydrateCustomerAdvanceRecovery 验身份、合同依据、正金额与形成时刻。不重审评估
// 是否成立、也不重审金额是否超出代垫——那是形成门的事。
func RehydrateCustomerAdvanceRecovery(spec RehydrateCustomerAdvanceRecoverySpec) (CustomerAdvanceRecovery, error) {
	if !spec.ID.valid() ||
		!spec.Assessment.valid() ||
		!spec.Customer.valid() ||
		!spec.ContractBasis.valid() ||
		!spec.Account.valid() ||
		!spec.Currency.valid() ||
		spec.AmountMinor <= 0 ||
		spec.FormedAt.IsZero() {
		return CustomerAdvanceRecovery{}, ErrInvalidAdvanceRecovery
	}
	return CustomerAdvanceRecovery{
		id:            spec.ID,
		assessment:    spec.Assessment,
		customer:      spec.Customer,
		contractBasis: spec.ContractBasis,
		account:       spec.Account,
		currency:      spec.Currency,
		amountMinor:   spec.AmountMinor,
		formedAt:      spec.FormedAt.UTC(),
	}, nil
}

// RecoveryAdjustmentReason 是回收调整的封闭三因：税费更正、资金事实更正/撤销、客户
// 责任变化。真实收款不在其中——收款走 UC-SA-005 的映射与核销，不改写回收。
type RecoveryAdjustmentReason uint8

const (
	RecoveryAdjustmentReasonInvalid RecoveryAdjustmentReason = iota
	TaxAssessmentCorrected
	FundsFactRevised
	CustomerResponsibilityChanged
)

func (reason RecoveryAdjustmentReason) valid() bool {
	return reason >= TaxAssessmentCorrected && reason <= CustomerResponsibilityChanged
}

func (reason RecoveryAdjustmentReason) String() string {
	switch reason {
	case TaxAssessmentCorrected:
		return "TAX_ASSESSMENT_CORRECTED"
	case FundsFactRevised:
		return "FUNDS_FACT_REVISED"
	case CustomerResponsibilityChanged:
		return "CUSTOMER_RESPONSIBILITY_CHANGED"
	default:
		return ""
	}
}

// RecoveryAdjustmentID 是回收调整的稳定身份。
type RecoveryAdjustmentID struct{ requiredValue }

func NewRecoveryAdjustmentID(value string) (RecoveryAdjustmentID, error) {
	required, err := newRequiredValue("recovery adjustment ID", value)
	return RecoveryAdjustmentID{required}, err
}

// RecoveryAdjustmentSpec 是形成一笔回收调整所需的全部输入。
type RecoveryAdjustmentSpec struct {
	ID          RecoveryAdjustmentID
	Recovery    AdvanceRecoveryID
	Reason      RecoveryAdjustmentReason
	NewBasis    AssessmentBasisReference
	Direction   AdjustmentDirection
	Currency    CurrencyCode
	AmountMinor int64
	Period      BillingPeriodReference
	FormedAt    time.Time
}

// RecoveryAdjustment 是对已形成回收的追加借项或贷项（UC-SA-001「客户代垫回收调整
// 已形成」）。原判断、原回收与已发布对账单都不被它改写——它只带原因、新依据、差额
// 与适用账期。
type RecoveryAdjustment struct {
	id          RecoveryAdjustmentID
	recovery    AdvanceRecoveryID
	reason      RecoveryAdjustmentReason
	newBasis    AssessmentBasisReference
	direction   AdjustmentDirection
	currency    CurrencyCode
	amountMinor int64
	period      BillingPeriodReference
	formedAt    time.Time
}

func FormRecoveryAdjustment(spec RecoveryAdjustmentSpec) (RecoveryAdjustment, error) {
	if !spec.ID.valid() ||
		!spec.Recovery.valid() ||
		!spec.Reason.valid() ||
		!spec.NewBasis.valid() ||
		!spec.Direction.valid() ||
		!spec.Currency.valid() ||
		spec.AmountMinor <= 0 ||
		!spec.Period.valid() ||
		spec.FormedAt.IsZero() {
		return RecoveryAdjustment{}, ErrInvalidRecoveryAdjustment
	}
	return RecoveryAdjustment{
		id:          spec.ID,
		recovery:    spec.Recovery,
		reason:      spec.Reason,
		newBasis:    spec.NewBasis,
		direction:   spec.Direction,
		currency:    spec.Currency,
		amountMinor: spec.AmountMinor,
		period:      spec.Period,
		formedAt:    spec.FormedAt.UTC(),
	}, nil
}

func (adjustment RecoveryAdjustment) ID() RecoveryAdjustmentID {
	return adjustment.id
}

func (adjustment RecoveryAdjustment) Recovery() AdvanceRecoveryID {
	return adjustment.recovery
}

func (adjustment RecoveryAdjustment) Reason() RecoveryAdjustmentReason {
	return adjustment.reason
}

func (adjustment RecoveryAdjustment) NewBasis() AssessmentBasisReference {
	return adjustment.newBasis
}

func (adjustment RecoveryAdjustment) Direction() AdjustmentDirection {
	return adjustment.direction
}

func (adjustment RecoveryAdjustment) Amount() (CurrencyCode, int64) {
	return adjustment.currency, adjustment.amountMinor
}

func (adjustment RecoveryAdjustment) Period() BillingPeriodReference {
	return adjustment.period
}

func (adjustment RecoveryAdjustment) FormedAt() time.Time {
	return adjustment.formedAt
}
