package domain

import (
	"errors"
	"time"
)

var ErrInvalidSafeHandoff = errors.New("parcel shipment: invalid safe handoff assessment")

type HandoffAttemptID struct{ requiredValue }

func NewHandoffAttemptID(value string) (HandoffAttemptID, error) {
	required, err := newRequiredValue("handoff attempt ID", value)
	return HandoffAttemptID{required}, err
}

type HandoffConfirmationReference struct{ requiredValue }

func NewHandoffConfirmationReference(value string) (HandoffConfirmationReference, error) {
	required, err := newRequiredValue("handoff confirmation reference", value)
	return HandoffConfirmationReference{required}, err
}

type HandoffQueryReference struct{ requiredValue }

func NewHandoffQueryReference(value string) (HandoffQueryReference, error) {
	required, err := newRequiredValue("handoff query reference", value)
	return HandoffQueryReference{required}, err
}

type HandoffObservation uint8

const (
	HandoffObservationInvalid HandoffObservation = iota
	HandoffObservationCompleteConfirmation
	HandoffObservationPartialConfirmation
	HandoffObservationTimedOut
	HandoffObservationQueryUnavailable
	HandoffObservationFailed
)

func (observation HandoffObservation) valid() bool {
	return observation >= HandoffObservationCompleteConfirmation && observation <= HandoffObservationFailed
}

func (observation HandoffObservation) String() string {
	switch observation {
	case HandoffObservationCompleteConfirmation:
		return "COMPLETE_CONFIRMATION"
	case HandoffObservationPartialConfirmation:
		return "PARTIAL_CONFIRMATION"
	case HandoffObservationTimedOut:
		return "TIMED_OUT"
	case HandoffObservationQueryUnavailable:
		return "QUERY_UNAVAILABLE"
	case HandoffObservationFailed:
		return "FAILED"
	default:
		return ""
	}
}

type SafeHandoffStatus uint8

const (
	SafeHandoffStatusInvalid SafeHandoffStatus = iota
	SafeHandoffConfirmed
	SafeHandoffUnresolved
)

func (status SafeHandoffStatus) String() string {
	switch status {
	case SafeHandoffConfirmed:
		return "CONFIRMED"
	case SafeHandoffUnresolved:
		return "UNRESOLVED"
	default:
		return ""
	}
}

type HandoffUnresolvedReason uint8

const (
	HandoffUnresolvedReasonInvalid HandoffUnresolvedReason = iota
	HandoffUnresolvedScopeMismatch
	HandoffUnresolvedPartialConfirmation
	HandoffUnresolvedTimedOut
	HandoffUnresolvedQueryUnavailable
	HandoffUnresolvedTargetFailure
)

func (reason HandoffUnresolvedReason) String() string {
	switch reason {
	case HandoffUnresolvedScopeMismatch:
		return "SCOPE_MISMATCH"
	case HandoffUnresolvedPartialConfirmation:
		return "PARTIAL_CONFIRMATION"
	case HandoffUnresolvedTimedOut:
		return "TIMED_OUT"
	case HandoffUnresolvedQueryUnavailable:
		return "QUERY_UNAVAILABLE"
	case HandoffUnresolvedTargetFailure:
		return "TARGET_FAILURE"
	default:
		return ""
	}
}

type SafeHandoffAssessmentSpec struct {
	AttemptID            HandoffAttemptID
	Scope                AdmissionScope
	TargetAuthority      ProductionAuthorityReference
	Observation          HandoffObservation
	ConfirmedScopeDigest AdmissionScopeDigest
	ConfirmationRef      HandoffConfirmationReference
	QueryRef             HandoffQueryReference
	ContinuationRef      OwnershipContinuationReference
	EffectiveAt          time.Time
	AssessedAt           time.Time
}

type SafeHandoffAssessment struct {
	attemptID            HandoffAttemptID
	scope                AdmissionScope
	targetAuthority      ProductionAuthorityReference
	observation          HandoffObservation
	confirmedScopeDigest AdmissionScopeDigest
	confirmationRef      HandoffConfirmationReference
	queryRef             HandoffQueryReference
	continuationRef      OwnershipContinuationReference
	effectiveAt          time.Time
	assessedAt           time.Time
	status               SafeHandoffStatus
	unresolvedReason     HandoffUnresolvedReason
}

func AssessSafeHandoff(spec SafeHandoffAssessmentSpec) (SafeHandoffAssessment, error) {
	if !spec.AttemptID.valid() ||
		!spec.Scope.valid() ||
		!spec.TargetAuthority.valid() ||
		!spec.Observation.valid() ||
		spec.AssessedAt.IsZero() ||
		!validHandoffEvidence(spec) {
		return SafeHandoffAssessment{}, ErrInvalidSafeHandoff
	}

	status := SafeHandoffUnresolved
	reason := unresolvedReasonFor(spec.Observation)
	if spec.Observation == HandoffObservationCompleteConfirmation {
		if spec.ConfirmedScopeDigest == spec.Scope.Digest() {
			status = SafeHandoffConfirmed
			reason = HandoffUnresolvedReasonInvalid
		} else {
			reason = HandoffUnresolvedScopeMismatch
		}
	}
	if status == SafeHandoffUnresolved && !spec.ContinuationRef.valid() {
		return SafeHandoffAssessment{}, ErrInvalidSafeHandoff
	}

	return SafeHandoffAssessment{
		attemptID:            spec.AttemptID,
		scope:                spec.Scope,
		targetAuthority:      spec.TargetAuthority,
		observation:          spec.Observation,
		confirmedScopeDigest: spec.ConfirmedScopeDigest,
		confirmationRef:      spec.ConfirmationRef,
		queryRef:             spec.QueryRef,
		continuationRef:      spec.ContinuationRef,
		effectiveAt:          spec.EffectiveAt,
		assessedAt:           spec.AssessedAt,
		status:               status,
		unresolvedReason:     reason,
	}, nil
}

func validHandoffEvidence(spec SafeHandoffAssessmentSpec) bool {
	switch spec.Observation {
	case HandoffObservationCompleteConfirmation:
		return spec.ConfirmedScopeDigest.valid() &&
			spec.ConfirmationRef.valid() &&
			spec.QueryRef.valid() &&
			!spec.EffectiveAt.IsZero()
	case HandoffObservationPartialConfirmation:
		return spec.ConfirmedScopeDigest.valid() &&
			spec.ConfirmationRef.valid() &&
			spec.EffectiveAt.IsZero()
	case HandoffObservationQueryUnavailable:
		return spec.ConfirmedScopeDigest.valid() &&
			spec.ConfirmationRef.valid() &&
			!spec.QueryRef.valid() &&
			spec.EffectiveAt.IsZero()
	case HandoffObservationTimedOut, HandoffObservationFailed:
		return !spec.ConfirmedScopeDigest.valid() &&
			!spec.ConfirmationRef.valid() &&
			!spec.QueryRef.valid() &&
			spec.EffectiveAt.IsZero()
	default:
		return false
	}
}

func unresolvedReasonFor(observation HandoffObservation) HandoffUnresolvedReason {
	switch observation {
	case HandoffObservationPartialConfirmation:
		return HandoffUnresolvedPartialConfirmation
	case HandoffObservationTimedOut:
		return HandoffUnresolvedTimedOut
	case HandoffObservationQueryUnavailable:
		return HandoffUnresolvedQueryUnavailable
	case HandoffObservationFailed:
		return HandoffUnresolvedTargetFailure
	default:
		return HandoffUnresolvedReasonInvalid
	}
}

func (assessment SafeHandoffAssessment) AttemptID() HandoffAttemptID {
	return assessment.attemptID
}

func (assessment SafeHandoffAssessment) Scope() AdmissionScope {
	return assessment.scope
}

func (assessment SafeHandoffAssessment) TargetAuthority() ProductionAuthorityReference {
	return assessment.targetAuthority
}

func (assessment SafeHandoffAssessment) Observation() HandoffObservation {
	return assessment.observation
}

func (assessment SafeHandoffAssessment) ConfirmedScopeDigest() (AdmissionScopeDigest, bool) {
	if !assessment.confirmedScopeDigest.valid() {
		return AdmissionScopeDigest{}, false
	}
	return assessment.confirmedScopeDigest, true
}

// ConfirmationReference 是`其他权威`归属决定必须引用的凭据。判定未决时即便目标方已经
// 回复也不给出它，这样部分确认或查不到的证据就不会被误当成已完成的交接。
func (assessment SafeHandoffAssessment) ConfirmationReference() (HandoffConfirmationReference, bool) {
	if !assessment.confirmationRef.valid() || assessment.status != SafeHandoffConfirmed {
		return HandoffConfirmationReference{}, false
	}
	return assessment.confirmationRef, true
}

func (assessment SafeHandoffAssessment) QueryReference() (HandoffQueryReference, bool) {
	if !assessment.queryRef.valid() {
		return HandoffQueryReference{}, false
	}
	return assessment.queryRef, true
}

func (assessment SafeHandoffAssessment) ContinuationReference() (OwnershipContinuationReference, bool) {
	if !assessment.continuationRef.valid() {
		return OwnershipContinuationReference{}, false
	}
	return assessment.continuationRef, true
}

func (assessment SafeHandoffAssessment) EffectiveAt() (time.Time, bool) {
	if assessment.effectiveAt.IsZero() || assessment.status != SafeHandoffConfirmed {
		return time.Time{}, false
	}
	return assessment.effectiveAt, true
}

func (assessment SafeHandoffAssessment) AssessedAt() time.Time {
	return assessment.assessedAt
}

func (assessment SafeHandoffAssessment) Status() SafeHandoffStatus {
	return assessment.status
}

func (assessment SafeHandoffAssessment) UnresolvedReason() (HandoffUnresolvedReason, bool) {
	if assessment.status != SafeHandoffUnresolved {
		return HandoffUnresolvedReasonInvalid, false
	}
	return assessment.unresolvedReason, true
}
