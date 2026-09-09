package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidAdmissionScope              = errors.New("parcel shipment: invalid admission scope")
	ErrInvalidProductionOwnershipDecision = errors.New("parcel shipment: invalid production ownership decision")
	ErrInvalidFutureSubmissionGate        = errors.New("parcel shipment: invalid future submission gate input")
)

// AdmissionScope 是生产归属决定所用的不可变范围快照。它刻意与 PayloadDigest 分开：
// 请求内容与治理范围的答案是两类不同的事实。
type AdmissionScope struct {
	reference AdmissionScopeReference
	digest    AdmissionScopeDigest
}

type AdmissionScopeReference struct{ requiredValue }

func NewAdmissionScopeReference(value string) (AdmissionScopeReference, error) {
	required, err := newRequiredValue("admission scope reference", value)
	return AdmissionScopeReference{required}, err
}

type AdmissionScopeDigest struct{ requiredValue }

func NewAdmissionScopeDigest(value string) (AdmissionScopeDigest, error) {
	required, err := newRequiredValue("admission scope digest", value)
	return AdmissionScopeDigest{required}, err
}

func NewAdmissionScope(
	reference AdmissionScopeReference,
	digest AdmissionScopeDigest,
) (AdmissionScope, error) {
	if !reference.valid() || !digest.valid() {
		return AdmissionScope{}, ErrInvalidAdmissionScope
	}
	return AdmissionScope{reference: reference, digest: digest}, nil
}

func (scope AdmissionScope) Reference() AdmissionScopeReference {
	return scope.reference
}

func (scope AdmissionScope) Digest() AdmissionScopeDigest {
	return scope.digest
}

func (scope AdmissionScope) valid() bool {
	return scope.reference.valid() && scope.digest.valid()
}

type ProductionOwnershipDecisionID struct{ requiredValue }

func NewProductionOwnershipDecisionID(value string) (ProductionOwnershipDecisionID, error) {
	required, err := newRequiredValue("production ownership decision ID", value)
	return ProductionOwnershipDecisionID{required}, err
}

type ProductionOwnershipRuleVersion struct{ requiredValue }

func NewProductionOwnershipRuleVersion(value string) (ProductionOwnershipRuleVersion, error) {
	required, err := newRequiredValue("production ownership rule version", value)
	return ProductionOwnershipRuleVersion{required}, err
}

type ProductionOwnershipRevision struct{ requiredValue }

func NewProductionOwnershipRevision(value string) (ProductionOwnershipRevision, error) {
	required, err := newRequiredValue("production ownership revision", value)
	return ProductionOwnershipRevision{required}, err
}

type ProductionAuthorityReference struct{ requiredValue }

func NewProductionAuthorityReference(value string) (ProductionAuthorityReference, error) {
	required, err := newRequiredValue("other production authority reference", value)
	return ProductionAuthorityReference{required}, err
}

type OwnershipContinuationReference struct{ requiredValue }

func NewOwnershipContinuationReference(value string) (OwnershipContinuationReference, error) {
	required, err := newRequiredValue("ownership continuation reference", value)
	return OwnershipContinuationReference{required}, err
}

type OwnershipSuspensionReference struct{ requiredValue }

func NewOwnershipSuspensionReference(value string) (OwnershipSuspensionReference, error) {
	required, err := newRequiredValue("ownership suspension reference", value)
	return OwnershipSuspensionReference{required}, err
}

type ProductionAuthorityKind uint8

const (
	ProductionAuthorityInvalid ProductionAuthorityKind = iota
	ProductionAuthorityIDPParcel
	ProductionAuthorityOther
	ProductionAuthorityUnresolved
)

func (authority ProductionAuthorityKind) valid() bool {
	return authority >= ProductionAuthorityIDPParcel && authority <= ProductionAuthorityUnresolved
}

func (authority ProductionAuthorityKind) String() string {
	switch authority {
	case ProductionAuthorityIDPParcel:
		return "IDP_PARCEL"
	case ProductionAuthorityOther:
		return "OTHER"
	case ProductionAuthorityUnresolved:
		return "UNRESOLVED"
	default:
		return ""
	}
}

type AdmissionControl uint8

const (
	AdmissionControlInvalid AdmissionControl = iota
	AdmissionControlOpen
	AdmissionControlPaused
)

func (control AdmissionControl) valid() bool {
	return control >= AdmissionControlOpen && control <= AdmissionControlPaused
}

func (control AdmissionControl) String() string {
	switch control {
	case AdmissionControlOpen:
		return "OPEN"
	case AdmissionControlPaused:
		return "PAUSED"
	default:
		return ""
	}
}

type OwnershipUnresolvedReason uint8

const (
	OwnershipUnresolvedReasonInvalid OwnershipUnresolvedReason = iota
	OwnershipUnresolvedAuthorityNotUnique
	OwnershipUnresolvedHandoffIncomplete
	OwnershipUnresolvedHandoffUnavailable
	OwnershipUnresolvedRuleUnavailable
)

func (reason OwnershipUnresolvedReason) valid() bool {
	return reason >= OwnershipUnresolvedAuthorityNotUnique && reason <= OwnershipUnresolvedRuleUnavailable
}

func (reason OwnershipUnresolvedReason) String() string {
	switch reason {
	case OwnershipUnresolvedAuthorityNotUnique:
		return "AUTHORITY_NOT_UNIQUE"
	case OwnershipUnresolvedHandoffIncomplete:
		return "HANDOFF_INCOMPLETE"
	case OwnershipUnresolvedHandoffUnavailable:
		return "HANDOFF_UNAVAILABLE"
	case OwnershipUnresolvedRuleUnavailable:
		return "RULE_UNAVAILABLE"
	default:
		return ""
	}
}

type OwnershipValidityInterval struct {
	validFrom  time.Time
	validUntil time.Time
}

func NewOwnershipValidityInterval(validFrom, validUntil time.Time) (OwnershipValidityInterval, error) {
	if validFrom.IsZero() || validUntil.IsZero() || !validFrom.Before(validUntil) {
		return OwnershipValidityInterval{}, ErrInvalidProductionOwnershipDecision
	}
	return OwnershipValidityInterval{
		validFrom:  validFrom,
		validUntil: validUntil,
	}, nil
}

func (interval OwnershipValidityInterval) ValidFrom() time.Time {
	return interval.validFrom
}

func (interval OwnershipValidityInterval) ValidUntil() time.Time {
	return interval.validUntil
}

func (interval OwnershipValidityInterval) Contains(at time.Time) bool {
	return !interval.validFrom.IsZero() &&
		!interval.validUntil.IsZero() &&
		!at.Before(interval.validFrom) &&
		at.Before(interval.validUntil)
}

func (interval OwnershipValidityInterval) valid() bool {
	return !interval.validFrom.IsZero() &&
		!interval.validUntil.IsZero() &&
		interval.validFrom.Before(interval.validUntil)
}

// ProductionOwnershipDecisionSpec 是形成不可变决定的输入。可选引用以零值表达，并按
// 所选的权威身份与准入控制两个维度校验其该有还是不该有。
type ProductionOwnershipDecisionSpec struct {
	DecisionID        ProductionOwnershipDecisionID
	Scope             AdmissionScope
	Authority         ProductionAuthorityKind
	OtherAuthorityRef ProductionAuthorityReference
	HandoffRef        HandoffConfirmationReference
	UnresolvedReason  OwnershipUnresolvedReason
	ContinuationRef   OwnershipContinuationReference
	AdmissionControl  AdmissionControl
	SuspensionRef     OwnershipSuspensionReference
	RuleVersion       ProductionOwnershipRuleVersion
	AsOf              time.Time
	Validity          OwnershipValidityInterval
	Revision          ProductionOwnershipRevision
	DecisionAt        time.Time
}

type ProductionOwnershipDecision struct {
	decisionID        ProductionOwnershipDecisionID
	scope             AdmissionScope
	authority         ProductionAuthorityKind
	otherAuthorityRef ProductionAuthorityReference
	handoffRef        HandoffConfirmationReference
	unresolvedReason  OwnershipUnresolvedReason
	continuationRef   OwnershipContinuationReference
	admissionControl  AdmissionControl
	suspensionRef     OwnershipSuspensionReference
	ruleVersion       ProductionOwnershipRuleVersion
	asOf              time.Time
	validity          OwnershipValidityInterval
	revision          ProductionOwnershipRevision
	decisionAt        time.Time
	// safeHandoff 是`其他权威`决定之后那一次交接尝试的评估（ADR-0128 决定四）。它与 handoffRef
	// 分格：handoffRef 是治理侧登记的停写证据，回答「前任停笔了没有」；这一格回答「这一笔范围
	// 交过去、对方确认了没有」。合并会让「有停写证据但交接失败」与「无停写证据」在记录上不可分。
	// 不进 Spec：形成决定的一方（治理读口的适配器）拿不到它，它在决定成立之后由编排的交接步记上。
	safeHandoff    SafeHandoffAssessment
	hasSafeHandoff bool
}

func NewProductionOwnershipDecision(spec ProductionOwnershipDecisionSpec) (ProductionOwnershipDecision, error) {
	if !spec.DecisionID.valid() ||
		!spec.Scope.valid() ||
		!spec.Authority.valid() ||
		!spec.AdmissionControl.valid() ||
		!spec.RuleVersion.valid() ||
		!spec.Validity.valid() ||
		!spec.Revision.valid() ||
		spec.AsOf.IsZero() ||
		spec.DecisionAt.IsZero() ||
		!spec.Validity.Contains(spec.AsOf) {
		return ProductionOwnershipDecision{}, ErrInvalidProductionOwnershipDecision
	}

	if !validAuthorityDetails(spec) || !validAdmissionDetails(spec) {
		return ProductionOwnershipDecision{}, ErrInvalidProductionOwnershipDecision
	}

	return ProductionOwnershipDecision{
		decisionID:        spec.DecisionID,
		scope:             spec.Scope,
		authority:         spec.Authority,
		otherAuthorityRef: spec.OtherAuthorityRef,
		handoffRef:        spec.HandoffRef,
		unresolvedReason:  spec.UnresolvedReason,
		continuationRef:   spec.ContinuationRef,
		admissionControl:  spec.AdmissionControl,
		suspensionRef:     spec.SuspensionRef,
		ruleVersion:       spec.RuleVersion,
		asOf:              spec.AsOf,
		validity:          spec.Validity,
		revision:          spec.Revision,
		decisionAt:        spec.DecisionAt,
	}, nil
}

func validAuthorityDetails(spec ProductionOwnershipDecisionSpec) bool {
	switch spec.Authority {
	case ProductionAuthorityIDPParcel:
		return !spec.OtherAuthorityRef.valid() &&
			!spec.HandoffRef.valid() &&
			!spec.UnresolvedReason.valid() &&
			!spec.ContinuationRef.valid()
	case ProductionAuthorityOther:
		return spec.OtherAuthorityRef.valid() &&
			spec.HandoffRef.valid() &&
			!spec.UnresolvedReason.valid() &&
			!spec.ContinuationRef.valid()
	case ProductionAuthorityUnresolved:
		return !spec.OtherAuthorityRef.valid() &&
			!spec.HandoffRef.valid() &&
			spec.UnresolvedReason.valid() &&
			spec.ContinuationRef.valid()
	default:
		return false
	}
}

func validAdmissionDetails(spec ProductionOwnershipDecisionSpec) bool {
	if spec.AdmissionControl == AdmissionControlPaused {
		return spec.SuspensionRef.valid()
	}
	return !spec.SuspensionRef.valid()
}

func (decision ProductionOwnershipDecision) DecisionID() ProductionOwnershipDecisionID {
	return decision.decisionID
}

func (decision ProductionOwnershipDecision) Scope() AdmissionScope {
	return decision.scope
}

func (decision ProductionOwnershipDecision) Authority() ProductionAuthorityKind {
	return decision.authority
}

func (decision ProductionOwnershipDecision) OtherAuthorityReference() (ProductionAuthorityReference, bool) {
	if !decision.otherAuthorityRef.valid() {
		return ProductionAuthorityReference{}, false
	}
	return decision.otherAuthorityRef, true
}

func (decision ProductionOwnershipDecision) HandoffReference() (HandoffConfirmationReference, bool) {
	if !decision.handoffRef.valid() {
		return HandoffConfirmationReference{}, false
	}
	return decision.handoffRef, true
}

func (decision ProductionOwnershipDecision) UnresolvedDetails() (OwnershipUnresolvedReason, OwnershipContinuationReference, bool) {
	if !decision.unresolvedReason.valid() || !decision.continuationRef.valid() {
		return 0, OwnershipContinuationReference{}, false
	}
	return decision.unresolvedReason, decision.continuationRef, true
}

// WithSafeHandoff 把一次交接尝试的评估记到`其他权威`决定上，交回新的决定值；原值不动。
//
// 只有`其他权威`有这一格：本产品自己承接的范围没有交接可言，权威未决的范围连交给谁都不知道
// （ADR-0128 决定二：归属先凭接管记录成立，成立之后才进入交接步）。评估必须是对这份决定的
// 范围、向这份决定指名的权威做的那一次——拿别的范围或别的对方的确认来给这份决定作数，与
// 没有确认一样。一次决定只记一次：交接尝试的身份由决定派生，第二次记上去的不可能是同一次。
func (decision ProductionOwnershipDecision) WithSafeHandoff(assessment SafeHandoffAssessment) (ProductionOwnershipDecision, error) {
	if !decision.valid() ||
		decision.hasSafeHandoff ||
		decision.authority != ProductionAuthorityOther ||
		!assessment.valid() ||
		assessment.scope != decision.scope ||
		assessment.targetAuthority != decision.otherAuthorityRef {
		return ProductionOwnershipDecision{}, ErrInvalidProductionOwnershipDecision
	}
	decision.safeHandoff = assessment
	decision.hasSafeHandoff = true
	return decision, nil
}

// SafeHandoff 交回记在这份决定上的交接评估；没记过即第二个返回值为假。它与 HandoffReference
// 各答各的问题，调用方不得拿其中一格去推另一格。
func (decision ProductionOwnershipDecision) SafeHandoff() (SafeHandoffAssessment, bool) {
	if !decision.hasSafeHandoff {
		return SafeHandoffAssessment{}, false
	}
	return decision.safeHandoff, true
}

func (decision ProductionOwnershipDecision) AdmissionControl() AdmissionControl {
	return decision.admissionControl
}

func (decision ProductionOwnershipDecision) SuspensionReference() (OwnershipSuspensionReference, bool) {
	if !decision.suspensionRef.valid() {
		return OwnershipSuspensionReference{}, false
	}
	return decision.suspensionRef, true
}

func (decision ProductionOwnershipDecision) RuleVersion() ProductionOwnershipRuleVersion {
	return decision.ruleVersion
}

func (decision ProductionOwnershipDecision) AsOf() time.Time {
	return decision.asOf
}

func (decision ProductionOwnershipDecision) Validity() OwnershipValidityInterval {
	return decision.validity
}

func (decision ProductionOwnershipDecision) Revision() ProductionOwnershipRevision {
	return decision.revision
}

func (decision ProductionOwnershipDecision) DecisionAt() time.Time {
	return decision.decisionAt
}

func (decision ProductionOwnershipDecision) valid() bool {
	if !decision.decisionID.valid() ||
		!decision.scope.valid() ||
		!decision.authority.valid() ||
		!decision.admissionControl.valid() ||
		!decision.ruleVersion.valid() ||
		!decision.validity.valid() ||
		!decision.revision.valid() ||
		decision.asOf.IsZero() ||
		decision.decisionAt.IsZero() ||
		!decision.validity.Contains(decision.asOf) {
		return false
	}
	spec := ProductionOwnershipDecisionSpec{
		DecisionID:        decision.decisionID,
		Scope:             decision.scope,
		Authority:         decision.authority,
		OtherAuthorityRef: decision.otherAuthorityRef,
		HandoffRef:        decision.handoffRef,
		UnresolvedReason:  decision.unresolvedReason,
		ContinuationRef:   decision.continuationRef,
		AdmissionControl:  decision.admissionControl,
		SuspensionRef:     decision.suspensionRef,
		RuleVersion:       decision.ruleVersion,
		AsOf:              decision.asOf,
		Validity:          decision.validity,
		Revision:          decision.revision,
		DecisionAt:        decision.decisionAt,
	}
	if !validAuthorityDetails(spec) || !validAdmissionDetails(spec) {
		return false
	}
	// 交接评估那一格若在场，必须与它所附的决定说的是同一次交接：同一范围、同一对方、且只挂在
	// `其他权威`上。WithSafeHandoff 在记入时守过这三条，这里再守一次是为了让一份决定值无论怎么
	// 拿到都能自证，而不是只在记入那一刻成立。
	if decision.hasSafeHandoff {
		return decision.authority == ProductionAuthorityOther &&
			decision.safeHandoff.valid() &&
			decision.safeHandoff.scope == decision.scope &&
			decision.safeHandoff.targetAuthority == decision.otherAuthorityRef
	}
	return true
}

type FutureSubmissionDisposition uint8

const (
	FutureSubmissionDispositionInvalid FutureSubmissionDisposition = iota
	FutureSubmissionAllowed
	FutureSubmissionBlocked
)

func (disposition FutureSubmissionDisposition) valid() bool {
	return disposition >= FutureSubmissionAllowed && disposition <= FutureSubmissionBlocked
}

func (disposition FutureSubmissionDisposition) String() string {
	switch disposition {
	case FutureSubmissionAllowed:
		return "ALLOWED"
	case FutureSubmissionBlocked:
		return "BLOCKED"
	default:
		return ""
	}
}

type FutureSubmissionBlockReason uint8

const (
	FutureSubmissionBlockReasonInvalid FutureSubmissionBlockReason = iota
	FutureSubmissionScopeMismatch
	FutureSubmissionDecisionStale
	FutureSubmissionOtherAuthority
	FutureSubmissionAuthorityUnresolved
	FutureSubmissionAdmissionPaused
)

func (reason FutureSubmissionBlockReason) valid() bool {
	return reason >= FutureSubmissionScopeMismatch && reason <= FutureSubmissionAdmissionPaused
}

func (reason FutureSubmissionBlockReason) String() string {
	switch reason {
	case FutureSubmissionScopeMismatch:
		return "SCOPE_MISMATCH"
	case FutureSubmissionDecisionStale:
		return "DECISION_STALE"
	case FutureSubmissionOtherAuthority:
		return "OTHER_AUTHORITY"
	case FutureSubmissionAuthorityUnresolved:
		return "AUTHORITY_UNRESOLVED"
	case FutureSubmissionAdmissionPaused:
		return "ADMISSION_PAUSED"
	default:
		return ""
	}
}

type FutureSubmissionGate struct {
	decision            ProductionOwnershipDecision
	expectedScopeDigest AdmissionScopeDigest
	expectedRevision    ProductionOwnershipRevision
	evaluatedAt         time.Time
	disposition         FutureSubmissionDisposition
	blockReasons        []FutureSubmissionBlockReason
}

// EvaluateFutureSubmissionGate 派生未来建单门禁结果，不创建委托、接受判断任务、领域
// 事件或任何持久化记录。
func EvaluateFutureSubmissionGate(
	decision ProductionOwnershipDecision,
	expectedScopeDigest AdmissionScopeDigest,
	expectedRevision ProductionOwnershipRevision,
	evaluatedAt time.Time,
) (FutureSubmissionGate, error) {
	if !decision.valid() ||
		!expectedScopeDigest.valid() ||
		!expectedRevision.valid() ||
		evaluatedAt.IsZero() {
		return FutureSubmissionGate{}, ErrInvalidFutureSubmissionGate
	}

	reasons := make([]FutureSubmissionBlockReason, 0, 4)
	if decision.Scope().Digest() != expectedScopeDigest {
		reasons = append(reasons, FutureSubmissionScopeMismatch)
	}
	if decision.Revision() != expectedRevision ||
		evaluatedAt.Before(decision.DecisionAt()) ||
		!decision.Validity().Contains(evaluatedAt) {
		reasons = append(reasons, FutureSubmissionDecisionStale)
	}
	switch decision.Authority() {
	case ProductionAuthorityOther:
		reasons = append(reasons, FutureSubmissionOtherAuthority)
	case ProductionAuthorityUnresolved:
		reasons = append(reasons, FutureSubmissionAuthorityUnresolved)
	}
	if decision.AdmissionControl() == AdmissionControlPaused {
		reasons = append(reasons, FutureSubmissionAdmissionPaused)
	}

	disposition := FutureSubmissionAllowed
	if len(reasons) > 0 {
		disposition = FutureSubmissionBlocked
	}
	return FutureSubmissionGate{
		decision:            decision,
		expectedScopeDigest: expectedScopeDigest,
		expectedRevision:    expectedRevision,
		evaluatedAt:         evaluatedAt,
		disposition:         disposition,
		blockReasons:        reasons,
	}, nil
}

func (gate FutureSubmissionGate) Decision() ProductionOwnershipDecision {
	return gate.decision
}

func (gate FutureSubmissionGate) ExpectedScopeDigest() AdmissionScopeDigest {
	return gate.expectedScopeDigest
}

func (gate FutureSubmissionGate) ExpectedRevision() ProductionOwnershipRevision {
	return gate.expectedRevision
}

func (gate FutureSubmissionGate) EvaluatedAt() time.Time {
	return gate.evaluatedAt
}

func (gate FutureSubmissionGate) Disposition() FutureSubmissionDisposition {
	return gate.disposition
}

func (gate FutureSubmissionGate) IsAllowed() bool {
	return gate.disposition == FutureSubmissionAllowed
}

func (gate FutureSubmissionGate) BlockReasons() []FutureSubmissionBlockReason {
	return append([]FutureSubmissionBlockReason(nil), gate.blockReasons...)
}

func (gate FutureSubmissionGate) valid() bool {
	if !gate.decision.valid() ||
		!gate.expectedScopeDigest.valid() ||
		!gate.expectedRevision.valid() ||
		gate.evaluatedAt.IsZero() ||
		!gate.disposition.valid() {
		return false
	}
	for _, reason := range gate.blockReasons {
		if !reason.valid() {
			return false
		}
	}
	return (gate.disposition == FutureSubmissionAllowed && len(gate.blockReasons) == 0) ||
		(gate.disposition == FutureSubmissionBlocked && len(gate.blockReasons) > 0)
}
