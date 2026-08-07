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

// AdmissionScope is the immutable scope snapshot used by a production
// ownership decision. It is intentionally separate from PayloadDigest: a
// request payload and a governance scope answer are different facts.
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

// ProductionOwnershipDecisionSpec is the input to the immutable decision.
// Optional references are represented by their zero values and are validated
// against the selected authority/control dimensions.
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
	return validAuthorityDetails(spec) && validAdmissionDetails(spec)
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

// EvaluateFutureSubmissionGate derives the future-submission result without
// creating a request, task, event, or persistence record.
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
