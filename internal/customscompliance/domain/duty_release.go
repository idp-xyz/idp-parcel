package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidDutyVerification = errors.New("customs compliance: invalid duty payment verification")
	ErrInvalidReleaseOutcome   = errors.New("customs compliance: invalid release outcome")
	ErrInvalidGateVerification = errors.New("customs compliance: invalid release gate verification")
)

// DutyCoverage 是税费付款核对的覆盖轴（无覆盖/部分覆盖/已覆盖）。
type DutyCoverage uint8

const (
	DutyCoverageInvalid DutyCoverage = iota
	CoverageNone
	CoveragePartial
	CoverageFull
)

func (coverage DutyCoverage) valid() bool {
	return coverage >= CoverageNone && coverage <= CoverageFull
}

func (coverage DutyCoverage) String() string {
	switch coverage {
	case CoverageNone:
		return "NONE"
	case CoveragePartial:
		return "PARTIAL"
	case CoverageFull:
		return "COVERED"
	default:
		return ""
	}
}

// DutyDelta 是差额轴（无差额/不足/超额/待确认）。
type DutyDelta uint8

const (
	DutyDeltaInvalid DutyDelta = iota
	DeltaNone
	DeltaShort
	DeltaExcess
	DeltaPending
)

func (delta DutyDelta) valid() bool {
	return delta >= DeltaNone && delta <= DeltaPending
}

func (delta DutyDelta) String() string {
	switch delta {
	case DeltaNone:
		return "NO_DELTA"
	case DeltaShort:
		return "SHORT"
	case DeltaExcess:
		return "EXCESS"
	case DeltaPending:
		return "PENDING"
	default:
		return ""
	}
}

// DutyFactValidity 是有效性轴（有效/失效/冲突/待确认）。
type DutyFactValidity uint8

const (
	DutyFactValidityInvalid DutyFactValidity = iota
	FundsFactValid
	FundsFactInvalidated
	FundsFactConflicting
	FundsFactPending
)

func (validity DutyFactValidity) valid() bool {
	return validity >= FundsFactValid && validity <= FundsFactPending
}

func (validity DutyFactValidity) String() string {
	switch validity {
	case FundsFactValid:
		return "VALID"
	case FundsFactInvalidated:
		return "INVALIDATED"
	case FundsFactConflicting:
		return "CONFLICTING"
	case FundsFactPending:
		return "PENDING"
	default:
		return ""
	}
}

// AssessedDutyReference 指名已接受的监管核定税费版本。
type AssessedDutyReference struct{ requiredValue }

func NewAssessedDutyReference(value string) (AssessedDutyReference, error) {
	required, err := newRequiredValue("assessed duty reference", value)
	return AssessedDutyReference{required}, err
}

// ExternalFundsFactReference 指名银行、支付或财务系统提供的外部资金事实。真实付款
// 归它们拥有，这里只引用参与核对。
type ExternalFundsFactReference struct{ requiredValue }

func NewExternalFundsFactReference(value string) (ExternalFundsFactReference, error) {
	required, err := newRequiredValue("external funds fact reference", value)
	return ExternalFundsFactReference{required}, err
}

// DutyPaymentVerification 是一次版本化的税费付款核对：覆盖、差额与有效性三轴分别
// 表达（CONTEXT 硬句 214 明禁实现为一组互斥总状态——三个独立枚举正是那半句的类型
// 面）。它不形成实际付款、客户回收或监管放行——类型上没有那些字段。
type DutyPaymentVerification struct {
	duty       AssessedDutyReference
	funds      ExternalFundsFactReference
	scope      DecisionScopeReference
	coverage   DutyCoverage
	delta      DutyDelta
	validity   DutyFactValidity
	verifiedAt time.Time
}

func VerifyDutyPayment(
	duty AssessedDutyReference,
	funds ExternalFundsFactReference,
	scope DecisionScopeReference,
	coverage DutyCoverage,
	delta DutyDelta,
	validity DutyFactValidity,
	verifiedAt time.Time,
) (DutyPaymentVerification, error) {
	if !duty.valid() || !funds.valid() || !scope.valid() ||
		!coverage.valid() || !delta.valid() || !validity.valid() ||
		verifiedAt.IsZero() {
		return DutyPaymentVerification{}, ErrInvalidDutyVerification
	}
	return DutyPaymentVerification{
		duty:       duty,
		funds:      funds,
		scope:      scope,
		coverage:   coverage,
		delta:      delta,
		validity:   validity,
		verifiedAt: verifiedAt.UTC(),
	}, nil
}

func (verification DutyPaymentVerification) Duty() AssessedDutyReference {
	return verification.duty
}

func (verification DutyPaymentVerification) Funds() ExternalFundsFactReference {
	return verification.funds
}

func (verification DutyPaymentVerification) Coverage() DutyCoverage {
	return verification.coverage
}

func (verification DutyPaymentVerification) Delta() DutyDelta {
	return verification.delta
}

func (verification DutyPaymentVerification) Validity() DutyFactValidity {
	return verification.validity
}

func (verification DutyPaymentVerification) VerifiedAt() time.Time {
	return verification.verifiedAt
}

// ReleaseKind 是放行结果的封闭三值（全部/部分/附条件）。
type ReleaseKind uint8

const (
	ReleaseKindInvalid ReleaseKind = iota
	FullRelease
	PartialRelease
	ConditionalRelease
)

func (kind ReleaseKind) valid() bool {
	return kind >= FullRelease && kind <= ConditionalRelease
}

func (kind ReleaseKind) String() string {
	switch kind {
	case FullRelease:
		return "FULL"
	case PartialRelease:
		return "PARTIAL"
	case ConditionalRelease:
		return "CONDITIONAL"
	default:
		return ""
	}
}

// ReleaseOutcome 是监管机构对明确范围作出的放行事实。构造要求监管来源引用——放行
// 不能由技术成功、业务受理、税费支付或内部合规解除推导：那些东西换不成监管来源
// 引用，构造期就进不来。部分与附条件放行必须带范围/条件说明；未被放行的范围继续
// 受监管门禁约束（那是门禁核对的事，这里不管）。
type CustomsReleaseOutcome struct {
	kind       ReleaseKind
	authority  RegulatoryAuthorityReference
	scope      DecisionScopeReference
	condition  string
	receivedAt time.Time
}

func ReceiveReleaseOutcome(
	kind ReleaseKind,
	authority RegulatoryAuthorityReference,
	scope DecisionScopeReference,
	condition string,
	receivedAt time.Time,
) (CustomsReleaseOutcome, error) {
	if !kind.valid() || !authority.valid() || !scope.valid() || receivedAt.IsZero() {
		return CustomsReleaseOutcome{}, ErrInvalidReleaseOutcome
	}
	if kind == ConditionalRelease && condition == "" {
		// 附条件放行说不出条件，与全部放行分不开。
		return CustomsReleaseOutcome{}, ErrInvalidReleaseOutcome
	}
	if kind == FullRelease && condition != "" {
		return CustomsReleaseOutcome{}, ErrInvalidReleaseOutcome
	}
	return CustomsReleaseOutcome{
		kind:       kind,
		authority:  authority,
		scope:      scope,
		condition:  condition,
		receivedAt: receivedAt.UTC(),
	}, nil
}

func (release CustomsReleaseOutcome) Kind() ReleaseKind {
	return release.kind
}

func (release CustomsReleaseOutcome) Authority() RegulatoryAuthorityReference {
	return release.authority
}

func (release CustomsReleaseOutcome) Scope() DecisionScopeReference {
	return release.scope
}

// Condition 只在附条件放行上给出。
func (release CustomsReleaseOutcome) Condition() (string, bool) {
	return release.condition, release.kind == ConditionalRelease
}

// ReceivedAt 是持久化重建的必需读口（判据同 ExternalResult 那三个读口）。
func (release CustomsReleaseOutcome) ReceivedAt() time.Time {
	return release.receivedAt
}

// GateConclusion 是放行门禁核对的封闭五值（CONTEXT「放行门禁核对」语言逐词）。
type GateConclusion uint8

const (
	GateConclusionInvalid GateConclusion = iota
	GateUnmet
	GatePartiallyMet
	GateMet
	GateConflicting
	GateNotApplicable
)

func (conclusion GateConclusion) valid() bool {
	return conclusion >= GateUnmet && conclusion <= GateNotApplicable
}

func (conclusion GateConclusion) String() string {
	switch conclusion {
	case GateUnmet:
		return "UNMET"
	case GatePartiallyMet:
		return "PARTIALLY_MET"
	case GateMet:
		return "MET"
	case GateConflicting:
		return "CONFLICTING"
	case GateNotApplicable:
		return "NOT_APPLICABLE"
	default:
		return ""
	}
}

// PreconditionReference 指名参与门禁核对的一项前置条件判断（税费付款核对、限制、
// 处置或其他监管事实）。
type PreconditionReference struct{ requiredValue }

func NewPreconditionReference(value string) (PreconditionReference, error) {
	required, err := newRequiredValue("precondition reference", value)
	return PreconditionReference{required}, err
}

// PreconditionState 是单项前置条件的判断三值：满足、未满足、事实冲突。没有「未知」
// 格——判断不出来的前置条件根本不该进折叠，那是证据装配问题不是门禁语义。
type PreconditionState uint8

const (
	PreconditionStateInvalid PreconditionState = iota
	PreconditionMet
	PreconditionUnmet
	PreconditionConflicting
)

func (state PreconditionState) valid() bool {
	return state >= PreconditionMet && state <= PreconditionConflicting
}

// String 是封闭三值的词形出口，库列与传输层同词（判据同 GuardedAction.String：
// 集外答空串，调用方以空串拒收，不给「未知」留第四格）。
func (state PreconditionState) String() string {
	switch state {
	case PreconditionMet:
		return "MET"
	case PreconditionUnmet:
		return "UNMET"
	case PreconditionConflicting:
		return "CONFLICTING"
	default:
		return ""
	}
}

// PreconditionFinding 是一项前置条件及其判断。
type PreconditionFinding struct {
	Precondition PreconditionReference
	State        PreconditionState
}

// FoldGateConclusion 把逐项前置条件判断折成门禁五值结论：任一冲突即整体冲突（冲突
// 压过满足与未满足——事实打架时说「部分满足」是把矛盾说成进度）；无冲突时全满足为
// 满足、全未满足为未满足、混合为部分满足；空清单即不适用（此动作在此边界本就不受
// 门禁）。逐项判断必须完整——判断不出的项不该送进来。
func FoldGateConclusion(findings []PreconditionFinding) (GateConclusion, error) {
	if len(findings) == 0 {
		return GateNotApplicable, nil
	}
	met, unmet := 0, 0
	for _, finding := range findings {
		if !finding.Precondition.valid() || !finding.State.valid() {
			return GateConclusionInvalid, ErrInvalidGateVerification
		}
		switch finding.State {
		case PreconditionConflicting:
			return GateConflicting, nil
		case PreconditionMet:
			met++
		case PreconditionUnmet:
			unmet++
		}
	}
	switch {
	case unmet == 0:
		return GateMet, nil
	case met == 0:
		return GateUnmet, nil
	default:
		return GatePartiallyMet, nil
	}
}

// ReleaseGateVerification 是针对明确申报范围、拟执行动作和适用监管边界的放行前置
// 条件版本化判断。动作绑定构造期固定——门禁判断不能复用于其他动作或监管边界
// （CONTEXT 硬句 216）；类型上没有放行字段——门禁满足不生成放行，放行结果仍由外部
// 事实接收。
type ReleaseGateVerification struct {
	scope         DecisionScopeReference
	action        GuardedAction
	boundary      CustomsProcedureReference
	preconditions []PreconditionReference
	conclusion    GateConclusion
	verifiedAt    time.Time
}

func VerifyReleaseGate(
	scope DecisionScopeReference,
	action GuardedAction,
	boundary CustomsProcedureReference,
	preconditions []PreconditionReference,
	conclusion GateConclusion,
	verifiedAt time.Time,
) (ReleaseGateVerification, error) {
	if !scope.valid() || !action.valid() || !boundary.valid() ||
		!conclusion.valid() || verifiedAt.IsZero() {
		return ReleaseGateVerification{}, ErrInvalidGateVerification
	}
	if conclusion != GateNotApplicable && len(preconditions) == 0 {
		// 说不出核对了哪些前置条件的门禁判断无从复核；只有`不适用`可以没有前置
		// 条件——那格的意思是这个动作在此边界本就不受门禁。
		return ReleaseGateVerification{}, ErrInvalidGateVerification
	}
	for _, precondition := range preconditions {
		if !precondition.valid() {
			return ReleaseGateVerification{}, ErrInvalidGateVerification
		}
	}
	return ReleaseGateVerification{
		scope:         scope,
		action:        action,
		boundary:      boundary,
		preconditions: append([]PreconditionReference(nil), preconditions...),
		conclusion:    conclusion,
		verifiedAt:    verifiedAt.UTC(),
	}, nil
}

func (gate ReleaseGateVerification) Scope() DecisionScopeReference {
	return gate.scope
}

func (gate ReleaseGateVerification) Action() GuardedAction {
	return gate.action
}

func (gate ReleaseGateVerification) Boundary() CustomsProcedureReference {
	return gate.boundary
}

func (gate ReleaseGateVerification) Preconditions() []PreconditionReference {
	return append([]PreconditionReference(nil), gate.preconditions...)
}

func (gate ReleaseGateVerification) Conclusion() GateConclusion {
	return gate.conclusion
}

func (gate ReleaseGateVerification) VerifiedAt() time.Time {
	return gate.verifiedAt
}

// AppliesTo 报告本门禁判断是否适用于给定动作与边界——判断不能复用于其他动作或
// 监管边界，消费方用这个读口核对而不是拿着结论到处贴。
func (gate ReleaseGateVerification) AppliesTo(action GuardedAction, boundary CustomsProcedureReference) bool {
	return gate.action == action && gate.boundary == boundary
}
