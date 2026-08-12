package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidClaimAmount        = errors.New("settlement accounting: invalid customer claim amount")
	ErrInvalidRecoveryReceivable = errors.New("settlement accounting: invalid recovery receivable")
	ErrInvalidAcknowledgement    = errors.New("settlement accounting: invalid recovery acknowledgement")
	ErrInvalidAmountAdjustment   = errors.New("settlement accounting: invalid claim amount adjustment")
)

// ClaimItemReference 指名 visibility-exception 拥有的客户索赔项。SA 只引用——资格、
// 通知、对方响应与责任结论都在那边。
type ClaimItemReference struct{ requiredValue }

func NewClaimItemReference(value string) (ClaimItemReference, error) {
	required, err := newRequiredValue("claim item reference", value)
	return ClaimItemReference{required}, err
}

// ResponsibilityConclusionReference 指名 VE 责任结论的版本。没有责任结论就没有金额
// ——结算保持待判断，不形成赔付或追偿（AT-SA-144）。
type ResponsibilityConclusionReference struct{ requiredValue }

func NewResponsibilityConclusionReference(value string) (ResponsibilityConclusionReference, error) {
	required, err := newRequiredValue("responsibility conclusion reference", value)
	return ResponsibilityConclusionReference{required}, err
}

// AmountRuleVersionReference 指名采用的限额/比例/免赔金额规则版本（AT-SA-147：保存
// 采用版本和计算展开，金额可与主张不同）。
type AmountRuleVersionReference struct{ requiredValue }

func NewAmountRuleVersionReference(value string) (AmountRuleVersionReference, error) {
	required, err := newRequiredValue("amount rule version reference", value)
	return AmountRuleVersionReference{required}, err
}

// CustomerClaimAmountKind 是客户方向金额的封闭二值：赔付义务（应付/贷记）与索赔费用
// 退款（对既有费用追加贷记）。计价纠错与商业让利不在其中——那归 UC-SA-002，两族不混
// （AT-SA-153）。
type CustomerClaimAmountKind uint8

const (
	CustomerClaimAmountKindInvalid CustomerClaimAmountKind = iota
	CustomerCompensationPayable
	ClaimChargeRefund
)

func (kind CustomerClaimAmountKind) valid() bool {
	return kind == CustomerCompensationPayable || kind == ClaimChargeRefund
}

func (kind CustomerClaimAmountKind) String() string {
	switch kind {
	case CustomerCompensationPayable:
		return "COMPENSATION_PAYABLE"
	case ClaimChargeRefund:
		return "CLAIM_CHARGE_REFUND"
	default:
		return ""
	}
}

// CustomerClaimAmountID 是客户赔付/索赔退款金额的稳定身份。
type CustomerClaimAmountID struct{ requiredValue }

func NewCustomerClaimAmountID(value string) (CustomerClaimAmountID, error) {
	required, err := newRequiredValue("customer claim amount ID", value)
	return CustomerClaimAmountID{required}, err
}

// CustomerClaimAmountSpec 是形成一笔客户方向索赔金额所需的全部输入。
type CustomerClaimAmountSpec struct {
	ID             CustomerClaimAmountID
	Kind           CustomerClaimAmountKind
	ClaimItem      ClaimItemReference
	Responsibility ResponsibilityConclusionReference
	RuleVersion    AmountRuleVersionReference
	LegalEntity    LegalEntityReference
	OriginalCharge CustomerChargeID
	Currency       CurrencyCode
	AmountMinor    int64
	Period         BillingPeriodReference
	FormedAt       time.Time
}

// CustomerClaimAmount 是运营企业对客户的赔付义务或索赔费用退款（UC-SA-007 唯一创建）。
// 它不是客户应收、不是已付款——那些在 UC-SA-005 的映射与核销上；它也不携带任何追偿
// 字段：客户赔付与追偿分层并行，追偿未决不阻塞赔付、也不与赔付净额抵销（AT-SA-148）。
type CustomerClaimAmount struct {
	id             CustomerClaimAmountID
	kind           CustomerClaimAmountKind
	claimItem      ClaimItemReference
	responsibility ResponsibilityConclusionReference
	ruleVersion    AmountRuleVersionReference
	legalEntity    LegalEntityReference
	originalCharge CustomerChargeID
	currency       CurrencyCode
	amountMinor    int64
	period         BillingPeriodReference
	formedAt       time.Time
}

// FormCustomerClaimAmount 依据责任结论与金额规则形成客户方向金额。索赔费用退款必须
// 指名被贷记的原费用；赔付义务不得指名——指了就把赔付混成了费用调整（两族分离，
// AT-SA-153）。
func FormCustomerClaimAmount(spec CustomerClaimAmountSpec) (CustomerClaimAmount, error) {
	if !spec.ID.valid() ||
		!spec.Kind.valid() ||
		!spec.ClaimItem.valid() ||
		!spec.Responsibility.valid() ||
		!spec.RuleVersion.valid() ||
		!spec.LegalEntity.valid() ||
		!spec.Currency.valid() ||
		spec.AmountMinor <= 0 ||
		!spec.Period.valid() ||
		spec.FormedAt.IsZero() {
		return CustomerClaimAmount{}, ErrInvalidClaimAmount
	}
	if spec.Kind == ClaimChargeRefund && !spec.OriginalCharge.valid() {
		return CustomerClaimAmount{}, ErrInvalidClaimAmount
	}
	if spec.Kind == CustomerCompensationPayable && spec.OriginalCharge.valid() {
		return CustomerClaimAmount{}, ErrInvalidClaimAmount
	}
	return CustomerClaimAmount{
		id:             spec.ID,
		kind:           spec.Kind,
		claimItem:      spec.ClaimItem,
		responsibility: spec.Responsibility,
		ruleVersion:    spec.RuleVersion,
		legalEntity:    spec.LegalEntity,
		originalCharge: spec.OriginalCharge,
		currency:       spec.Currency,
		amountMinor:    spec.AmountMinor,
		period:         spec.Period,
		formedAt:       spec.FormedAt.UTC(),
	}, nil
}

func (amount CustomerClaimAmount) ID() CustomerClaimAmountID {
	return amount.id
}

func (amount CustomerClaimAmount) Kind() CustomerClaimAmountKind {
	return amount.kind
}

func (amount CustomerClaimAmount) ClaimItem() ClaimItemReference {
	return amount.claimItem
}

func (amount CustomerClaimAmount) Responsibility() ResponsibilityConclusionReference {
	return amount.responsibility
}

func (amount CustomerClaimAmount) RuleVersion() AmountRuleVersionReference {
	return amount.ruleVersion
}

func (amount CustomerClaimAmount) LegalEntity() LegalEntityReference {
	return amount.legalEntity
}

// OriginalCharge 只在索赔费用退款上给出——退款贷记的是既有费用，赔付义务没有它。
func (amount CustomerClaimAmount) OriginalCharge() (CustomerChargeID, bool) {
	if !amount.originalCharge.valid() {
		return CustomerChargeID{}, false
	}
	return amount.originalCharge, true
}

func (amount CustomerClaimAmount) Amount() (CurrencyCode, int64) {
	return amount.currency, amount.amountMinor
}

func (amount CustomerClaimAmount) Period() BillingPeriodReference {
	return amount.period
}

func (amount CustomerClaimAmount) FormedAt() time.Time {
	return amount.formedAt
}

// RecoveryMatterReference 指名 VE 拥有的追偿事项。
type RecoveryMatterReference struct{ requiredValue }

func NewRecoveryMatterReference(value string) (RecoveryMatterReference, error) {
	required, err := newRequiredValue("recovery matter reference", value)
	return RecoveryMatterReference{required}, err
}

// RecoveryCounterpartyReference 指名追偿相对方（供应商、实际承运商或保险方）。
type RecoveryCounterpartyReference struct{ requiredValue }

func NewRecoveryCounterpartyReference(value string) (RecoveryCounterpartyReference, error) {
	required, err := newRequiredValue("recovery counterparty reference", value)
	return RecoveryCounterpartyReference{required}, err
}

// RecoveryReceivableID 是应追偿金额的稳定身份。
type RecoveryReceivableID struct{ requiredValue }

func NewRecoveryReceivableID(value string) (RecoveryReceivableID, error) {
	required, err := newRequiredValue("recovery receivable ID", value)
	return RecoveryReceivableID{required}, err
}

// RecoveryReceivableSpec 是形成一笔应追偿金额所需的全部输入。
type RecoveryReceivableSpec struct {
	ID             RecoveryReceivableID
	Matter         RecoveryMatterReference
	Responsibility ResponsibilityConclusionReference
	Counterparty   RecoveryCounterpartyReference
	RuleVersion    AmountRuleVersionReference
	LegalEntity    LegalEntityReference
	Currency       CurrencyCode
	AmountMinor    int64
	FormedAt       time.Time
}

// RecoveryReceivable 是运营企业有权向对方主张的应收金额（AT-SA-149：责任条件满足即可
// 独立形成，不等待对方响应）。类型上没有已认可、已到账或已核销字段——「应追偿≠认可
// ≠到账」是结构性的；认可另立对象，真实到账归 UC-SA-005。
type RecoveryReceivable struct {
	id             RecoveryReceivableID
	matter         RecoveryMatterReference
	responsibility ResponsibilityConclusionReference
	counterparty   RecoveryCounterpartyReference
	ruleVersion    AmountRuleVersionReference
	legalEntity    LegalEntityReference
	currency       CurrencyCode
	amountMinor    int64
	formedAt       time.Time
}

func FormRecoveryReceivable(spec RecoveryReceivableSpec) (RecoveryReceivable, error) {
	if !spec.ID.valid() ||
		!spec.Matter.valid() ||
		!spec.Responsibility.valid() ||
		!spec.Counterparty.valid() ||
		!spec.RuleVersion.valid() ||
		!spec.LegalEntity.valid() ||
		!spec.Currency.valid() ||
		spec.AmountMinor <= 0 ||
		spec.FormedAt.IsZero() {
		return RecoveryReceivable{}, ErrInvalidRecoveryReceivable
	}
	return RecoveryReceivable{
		id:             spec.ID,
		matter:         spec.Matter,
		responsibility: spec.Responsibility,
		counterparty:   spec.Counterparty,
		ruleVersion:    spec.RuleVersion,
		legalEntity:    spec.LegalEntity,
		currency:       spec.Currency,
		amountMinor:    spec.AmountMinor,
		formedAt:       spec.FormedAt.UTC(),
	}, nil
}

func (receivable RecoveryReceivable) ID() RecoveryReceivableID {
	return receivable.id
}

func (receivable RecoveryReceivable) Matter() RecoveryMatterReference {
	return receivable.matter
}

func (receivable RecoveryReceivable) Responsibility() ResponsibilityConclusionReference {
	return receivable.responsibility
}

func (receivable RecoveryReceivable) Counterparty() RecoveryCounterpartyReference {
	return receivable.counterparty
}

func (receivable RecoveryReceivable) Amount() (CurrencyCode, int64) {
	return receivable.currency, receivable.amountMinor
}

func (receivable RecoveryReceivable) FormedAt() time.Time {
	return receivable.formedAt
}

// CounterpartyResponseReference 指名 VE 已接收并判定有效的对方响应或结论版本。
type CounterpartyResponseReference struct{ requiredValue }

func NewCounterpartyResponseReference(value string) (CounterpartyResponseReference, error) {
	required, err := newRequiredValue("counterparty response reference", value)
	return CounterpartyResponseReference{required}, err
}

// ResponseStanding 是可形成认可金额的对方立场封闭二值：全部接受或部分接受。要求补充、
// 审核中、拒绝与无响应**没有格**——它们形成不了认可金额，封闭集合就是那道门
// （UC-SA-007 结果契约「追偿认可金额」行）。
type ResponseStanding uint8

const (
	ResponseStandingInvalid ResponseStanding = iota
	ResponseAccepted
	ResponsePartiallyAccepted
)

func (standing ResponseStanding) valid() bool {
	return standing == ResponseAccepted || standing == ResponsePartiallyAccepted
}

func (standing ResponseStanding) String() string {
	switch standing {
	case ResponseAccepted:
		return "ACCEPTED"
	case ResponsePartiallyAccepted:
		return "PARTIALLY_ACCEPTED"
	default:
		return ""
	}
}

// AcknowledgementID 是追偿认可金额的稳定身份。
type AcknowledgementID struct{ requiredValue }

func NewAcknowledgementID(value string) (AcknowledgementID, error) {
	required, err := newRequiredValue("acknowledgement ID", value)
	return AcknowledgementID{required}, err
}

// RecoveryAcknowledgement 是对方明确接受责任的认可范围金额（AT-SA-150）。认可不是
// 到账也不是核销——类型上没有任何收款或核销字段，真实资金只由 UC-SA-005 映射；完整
// 应追偿金额保留，未认可范围以差额可见。
type RecoveryAcknowledgement struct {
	id                AcknowledgementID
	receivable        RecoveryReceivableID
	response          CounterpartyResponseReference
	standing          ResponseStanding
	currency          CurrencyCode
	acknowledgedMinor int64
	receivableMinor   int64
	acknowledgedAt    time.Time
}

// AcknowledgeRecovery 依附应追偿金额形成认可金额。全部接受必须等额，部分接受必须
// 少于应追偿——两格的完备性互不借用；超过应追偿的「认可」不存在。
func AcknowledgeRecovery(
	receivable RecoveryReceivable,
	acknowledgement AcknowledgementID,
	response CounterpartyResponseReference,
	standing ResponseStanding,
	acknowledgedMinor int64,
	acknowledgedAt time.Time,
) (RecoveryAcknowledgement, error) {
	if !receivable.id.valid() || !acknowledgement.valid() || !response.valid() ||
		!standing.valid() || acknowledgedAt.IsZero() || acknowledgedAt.Before(receivable.formedAt) {
		return RecoveryAcknowledgement{}, ErrInvalidAcknowledgement
	}
	if acknowledgedMinor <= 0 || acknowledgedMinor > receivable.amountMinor {
		return RecoveryAcknowledgement{}, ErrInvalidAcknowledgement
	}
	if standing == ResponseAccepted && acknowledgedMinor != receivable.amountMinor {
		return RecoveryAcknowledgement{}, ErrInvalidAcknowledgement
	}
	if standing == ResponsePartiallyAccepted && acknowledgedMinor == receivable.amountMinor {
		return RecoveryAcknowledgement{}, ErrInvalidAcknowledgement
	}
	return RecoveryAcknowledgement{
		id:                acknowledgement,
		receivable:        receivable.id,
		response:          response,
		standing:          standing,
		currency:          receivable.currency,
		acknowledgedMinor: acknowledgedMinor,
		receivableMinor:   receivable.amountMinor,
		acknowledgedAt:    acknowledgedAt.UTC(),
	}, nil
}

func (acknowledgement RecoveryAcknowledgement) ID() AcknowledgementID {
	return acknowledgement.id
}

func (acknowledgement RecoveryAcknowledgement) Receivable() RecoveryReceivableID {
	return acknowledgement.receivable
}

func (acknowledgement RecoveryAcknowledgement) Response() CounterpartyResponseReference {
	return acknowledgement.response
}

func (acknowledgement RecoveryAcknowledgement) Standing() ResponseStanding {
	return acknowledgement.standing
}

func (acknowledgement RecoveryAcknowledgement) Amount() (CurrencyCode, int64) {
	return acknowledgement.currency, acknowledgement.acknowledgedMinor
}

// UnacknowledgedMinor 是未认可范围（AT-SA-150：完整应追偿保留，部分认可与未认可范围
// 分别可见）。
func (acknowledgement RecoveryAcknowledgement) UnacknowledgedMinor() int64 {
	return acknowledgement.receivableMinor - acknowledgement.acknowledgedMinor
}

func (acknowledgement RecoveryAcknowledgement) AcknowledgedAt() time.Time {
	return acknowledgement.acknowledgedAt
}

// ClaimAdjustmentReason 是追加金额调整的封闭三因：责任版本变化、金额规则更正、对方
// 认可范围变化。真实资金变化不在其中——那由 UC-SA-005 形成映射更正或核销撤销，不
// 改写金额（AT-SA-155）。
type ClaimAdjustmentReason uint8

const (
	ClaimAdjustmentReasonInvalid ClaimAdjustmentReason = iota
	ResponsibilityRevised
	AmountRuleCorrected
	AcknowledgementChanged
)

func (reason ClaimAdjustmentReason) valid() bool {
	return reason >= ResponsibilityRevised && reason <= AcknowledgementChanged
}

func (reason ClaimAdjustmentReason) String() string {
	switch reason {
	case ResponsibilityRevised:
		return "RESPONSIBILITY_REVISED"
	case AmountRuleCorrected:
		return "AMOUNT_RULE_CORRECTED"
	case AcknowledgementChanged:
		return "ACKNOWLEDGEMENT_CHANGED"
	default:
		return ""
	}
}

// AdjustedAmountKind 指名被调整的金额层：赔付/退款、应追偿或认可。
type AdjustedAmountKind uint8

const (
	AdjustedAmountKindInvalid AdjustedAmountKind = iota
	AdjustsCustomerClaimAmount
	AdjustsRecoveryReceivable
	AdjustsAcknowledgement
)

func (kind AdjustedAmountKind) valid() bool {
	return kind >= AdjustsCustomerClaimAmount && kind <= AdjustsAcknowledgement
}

func (kind AdjustedAmountKind) String() string {
	switch kind {
	case AdjustsCustomerClaimAmount:
		return "CUSTOMER_CLAIM_AMOUNT"
	case AdjustsRecoveryReceivable:
		return "RECOVERY_RECEIVABLE"
	case AdjustsAcknowledgement:
		return "ACKNOWLEDGEMENT"
	default:
		return ""
	}
}

// AdjustedAmountReference 指名被调整金额的身份。
type AdjustedAmountReference struct{ requiredValue }

func NewAdjustedAmountReference(value string) (AdjustedAmountReference, error) {
	required, err := newRequiredValue("adjusted amount reference", value)
	return AdjustedAmountReference{required}, err
}

// ClaimAmountAdjustmentID 是追加调整的稳定身份。
type ClaimAmountAdjustmentID struct{ requiredValue }

func NewClaimAmountAdjustmentID(value string) (ClaimAmountAdjustmentID, error) {
	required, err := newRequiredValue("claim amount adjustment ID", value)
	return ClaimAmountAdjustmentID{required}, err
}

// ClaimAmountAdjustmentSpec 是形成一笔追加金额调整所需的全部输入。
type ClaimAmountAdjustmentSpec struct {
	ID          ClaimAmountAdjustmentID
	TargetKind  AdjustedAmountKind
	Target      AdjustedAmountReference
	Reason      ClaimAdjustmentReason
	Basis       ResponsibilityConclusionReference
	Direction   AdjustmentDirection
	Currency    CurrencyCode
	AmountMinor int64
	Period      BillingPeriodReference
	FormedAt    time.Time
}

// ClaimAmountAdjustment 是对既有赔付/应追偿/认可金额的追加借项或贷项（AT-SA-152）。
// 原金额、已发布单据、真实收付和核销历史都不被它改写——它只带差额与新依据，适用
// 后续账期。
type ClaimAmountAdjustment struct {
	id          ClaimAmountAdjustmentID
	targetKind  AdjustedAmountKind
	target      AdjustedAmountReference
	reason      ClaimAdjustmentReason
	basis       ResponsibilityConclusionReference
	direction   AdjustmentDirection
	currency    CurrencyCode
	amountMinor int64
	period      BillingPeriodReference
	formedAt    time.Time
}

func FormClaimAmountAdjustment(spec ClaimAmountAdjustmentSpec) (ClaimAmountAdjustment, error) {
	if !spec.ID.valid() ||
		!spec.TargetKind.valid() ||
		!spec.Target.valid() ||
		!spec.Reason.valid() ||
		!spec.Basis.valid() ||
		!spec.Direction.valid() ||
		!spec.Currency.valid() ||
		spec.AmountMinor <= 0 ||
		!spec.Period.valid() ||
		spec.FormedAt.IsZero() {
		return ClaimAmountAdjustment{}, ErrInvalidAmountAdjustment
	}
	return ClaimAmountAdjustment{
		id:          spec.ID,
		targetKind:  spec.TargetKind,
		target:      spec.Target,
		reason:      spec.Reason,
		basis:       spec.Basis,
		direction:   spec.Direction,
		currency:    spec.Currency,
		amountMinor: spec.AmountMinor,
		period:      spec.Period,
		formedAt:    spec.FormedAt.UTC(),
	}, nil
}

func (adjustment ClaimAmountAdjustment) ID() ClaimAmountAdjustmentID {
	return adjustment.id
}

func (adjustment ClaimAmountAdjustment) TargetKind() AdjustedAmountKind {
	return adjustment.targetKind
}

func (adjustment ClaimAmountAdjustment) Target() AdjustedAmountReference {
	return adjustment.target
}

func (adjustment ClaimAmountAdjustment) Reason() ClaimAdjustmentReason {
	return adjustment.reason
}

func (adjustment ClaimAmountAdjustment) Basis() ResponsibilityConclusionReference {
	return adjustment.basis
}

func (adjustment ClaimAmountAdjustment) Direction() AdjustmentDirection {
	return adjustment.direction
}

func (adjustment ClaimAmountAdjustment) Amount() (CurrencyCode, int64) {
	return adjustment.currency, adjustment.amountMinor
}

func (adjustment ClaimAmountAdjustment) Period() BillingPeriodReference {
	return adjustment.period
}

func (adjustment ClaimAmountAdjustment) FormedAt() time.Time {
	return adjustment.formedAt
}
