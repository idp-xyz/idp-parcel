package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidAcceptanceCheck    = errors.New("parcel shipment: invalid acceptance check")
	ErrInvalidAcceptanceDecision = errors.New("parcel shipment: invalid acceptance decision")
	ErrDecisionAlreadyFormed     = errors.New("parcel shipment: a decision already exists for this submission version")
)

type AcceptanceDecisionID struct{ requiredValue }

func NewAcceptanceDecisionID(value string) (AcceptanceDecisionID, error) {
	required, err := newRequiredValue("acceptance decision ID", value)
	return AcceptanceDecisionID{required}, err
}

// CheckReason is the structured reason a check failed or could not be settled.
// It is a reference rather than free text so rejections stay countable by cause.
type CheckReason struct{ requiredValue }

func NewCheckReason(value string) (CheckReason, error) {
	required, err := newRequiredValue("check reason", value)
	return CheckReason{required}, err
}

// AcceptanceCheckGroup is the closed set of validation groups the use case
// declares. The rule over them is uniform — every applicable group must pass —
// so the set is complete here even though only some groups have producers yet.
type AcceptanceCheckGroup uint8

const (
	AcceptanceCheckGroupInvalid AcceptanceCheckGroup = iota
	CustomerRelationshipCheck
	LegalEntityAndContractCheck
	ProductAndServiceCheck
	MemberBaselineCheck
	RequiredDocumentCheck
	PreAcceptanceFinancialControlCheck
	NetworkReachabilityCheck
)

func (group AcceptanceCheckGroup) valid() bool {
	return group >= CustomerRelationshipCheck && group <= NetworkReachabilityCheck
}

func (group AcceptanceCheckGroup) String() string {
	switch group {
	case CustomerRelationshipCheck:
		return "CUSTOMER_RELATIONSHIP"
	case LegalEntityAndContractCheck:
		return "LEGAL_ENTITY_AND_CONTRACT"
	case ProductAndServiceCheck:
		return "PRODUCT_AND_SERVICE"
	case MemberBaselineCheck:
		return "MEMBER_BASELINE"
	case RequiredDocumentCheck:
		return "REQUIRED_DOCUMENT"
	case PreAcceptanceFinancialControlCheck:
		return "PRE_ACCEPTANCE_FINANCIAL_CONTROL"
	case NetworkReachabilityCheck:
		return "NETWORK_REACHABILITY"
	default:
		return ""
	}
}

type CheckOutcome uint8

const (
	CheckOutcomeInvalid CheckOutcome = iota
	CheckPassed
	CheckFailed
	CheckUndetermined
)

func (outcome CheckOutcome) valid() bool {
	return outcome >= CheckPassed && outcome <= CheckUndetermined
}

func (outcome CheckOutcome) String() string {
	switch outcome {
	case CheckPassed:
		return "PASSED"
	case CheckFailed:
		return "FAILED"
	case CheckUndetermined:
		return "UNDETERMINED"
	default:
		return ""
	}
}

// AcceptanceCheck is one group's result. A zero parcel ID means the check is
// scoped to the whole submission version; a set one means it bears on that
// member alone. Anything other than a pass must carry its reason.
type AcceptanceCheck struct {
	group    AcceptanceCheckGroup
	parcelID DeclaredParcelID
	outcome  CheckOutcome
	reason   CheckReason
}

func NewAcceptanceCheck(
	group AcceptanceCheckGroup,
	parcelID DeclaredParcelID,
	outcome CheckOutcome,
	reason CheckReason,
) (AcceptanceCheck, error) {
	if !group.valid() || !outcome.valid() {
		return AcceptanceCheck{}, ErrInvalidAcceptanceCheck
	}
	if outcome != CheckPassed && !reason.valid() {
		return AcceptanceCheck{}, ErrInvalidAcceptanceCheck
	}
	return AcceptanceCheck{group: group, parcelID: parcelID, outcome: outcome, reason: reason}, nil
}

func (check AcceptanceCheck) Group() AcceptanceCheckGroup {
	return check.group
}

func (check AcceptanceCheck) DeclaredParcelID() DeclaredParcelID {
	return check.parcelID
}

func (check AcceptanceCheck) Outcome() CheckOutcome {
	return check.outcome
}

func (check AcceptanceCheck) Reason() CheckReason {
	return check.reason
}

// ManualReviewState records whether the adopted rule package demanded a human
// review. Review gates acceptance but never substitutes for a hard rule, so a
// completed review cannot turn a failure into an acceptance.
type ManualReviewState uint8

const (
	ManualReviewStateInvalid ManualReviewState = iota
	ManualReviewNotRequired
	ManualReviewRequired
	ManualReviewCompleted
)

func (state ManualReviewState) valid() bool {
	return state >= ManualReviewNotRequired && state <= ManualReviewCompleted
}

// AcceptanceBaseline is the uncoverable member set fixed at acceptance. It
// always covers the complete declared membership of the submission version:
// this product does not accept members individually, so a partial baseline
// could only mean a rule was skipped.
type AcceptanceBaseline struct {
	declaredParcelIDs []DeclaredParcelID
	submissionVersion SubmissionVersionID
	fixedAt           time.Time
}

func (baseline AcceptanceBaseline) DeclaredParcelIDs() []DeclaredParcelID {
	return append([]DeclaredParcelID(nil), baseline.declaredParcelIDs...)
}

func (baseline AcceptanceBaseline) SubmissionVersionID() SubmissionVersionID {
	return baseline.submissionVersion
}

func (baseline AcceptanceBaseline) FixedAt() time.Time {
	return baseline.fixedAt
}

// ExpectedCommitment is what the operator undertook at acceptance. It stores the
// basis in force at that moment rather than a computed date: the promised values
// live in the product and contract versions the basis names, so a later change
// to those versions cannot silently rewrite what was promised.
type ExpectedCommitment struct {
	basis    CommercialBasisSnapshot
	formedAt time.Time
}

func (commitment ExpectedCommitment) Basis() CommercialBasisSnapshot {
	return commitment.basis
}

func (commitment ExpectedCommitment) FormedAt() time.Time {
	return commitment.formedAt
}

type AcceptanceDecision struct {
	decisionID   AcceptanceDecisionID
	accepted     bool
	checks       []AcceptanceCheck
	basis        CommercialBasisSnapshot
	manualReview ManualReviewState
	decidedAt    time.Time
}

func (decision AcceptanceDecision) DecisionID() AcceptanceDecisionID {
	return decision.decisionID
}

func (decision AcceptanceDecision) Accepted() bool {
	return decision.accepted
}

func (decision AcceptanceDecision) Checks() []AcceptanceCheck {
	return append([]AcceptanceCheck(nil), decision.checks...)
}

func (decision AcceptanceDecision) FailedChecks() []AcceptanceCheck {
	failed := make([]AcceptanceCheck, 0, len(decision.checks))
	for _, check := range decision.checks {
		if check.outcome == CheckFailed {
			failed = append(failed, check)
		}
	}
	return failed
}

func (decision AcceptanceDecision) Basis() CommercialBasisSnapshot {
	return decision.basis
}

func (decision AcceptanceDecision) DecidedAt() time.Time {
	return decision.decidedAt
}

type AcceptanceDecisionSpec struct {
	DecisionID   AcceptanceDecisionID
	Checks       []AcceptanceCheck
	ManualReview ManualReviewState
	Basis        CommercialBasisSnapshot
	DecidedAt    time.Time
}

// Decide forms at most one acceptance or rejection for the current submission
// version. Three outcomes are possible and only two of them are decisions: a
// pass that cannot yet be settled leaves the request submitted with its
// acceptance task still open, because "not yet decided" is processing state
// rather than a lifecycle result.
//
// Any deterministic failure — on the version or on a single member — rejects the
// whole version. This product does not accept members individually, so there is
// no path here that keeps the good members and drops the bad one.
func (request ShipmentRequest) Decide(spec AcceptanceDecisionSpec) (ShipmentRequest, error) {
	// The already-decided case is checked first so a second attempt on an
	// accepted or rejected version says why it is refused, rather than reporting
	// a generic invalid state that reads like malformed input.
	if request.decisionFormed {
		return ShipmentRequest{}, ErrDecisionAlreadyFormed
	}
	if request.state != ShipmentRequestSubmitted {
		return ShipmentRequest{}, ErrInvalidShipmentRequest
	}
	if !spec.DecisionID.valid() || !spec.ManualReview.valid() || !spec.Basis.valid() || spec.DecidedAt.IsZero() {
		return ShipmentRequest{}, ErrInvalidAcceptanceDecision
	}

	failed, undetermined := 0, 0
	judged := make(map[DeclaredParcelID]struct{}, len(request.currentVersion.declaredParcelIDs))
	for _, check := range spec.Checks {
		if !check.group.valid() || !check.outcome.valid() {
			return ShipmentRequest{}, ErrInvalidAcceptanceCheck
		}
		switch check.outcome {
		case CheckFailed:
			failed++
		case CheckUndetermined:
			undetermined++
		}
		if check.parcelID.valid() && check.outcome == CheckPassed {
			judged[check.parcelID] = struct{}{}
		}
	}

	decision := AcceptanceDecision{
		decisionID:   spec.DecisionID,
		checks:       append([]AcceptanceCheck(nil), spec.Checks...),
		basis:        spec.Basis,
		manualReview: spec.ManualReview,
		decidedAt:    spec.DecidedAt,
	}

	if failed > 0 {
		request.state = ShipmentRequestRejected
		request.decision = decision
		request.decisionFormed = true
		request.acceptanceTask.complete = true
		return request, nil
	}
	if undetermined > 0 ||
		spec.ManualReview == ManualReviewRequired ||
		!request.everyMemberJudged(judged) {
		return request, nil
	}

	decision.accepted = true
	request.state = ShipmentRequestAccepted
	request.decision = decision
	request.decisionFormed = true
	request.acceptanceTask.complete = true
	request.baseline = AcceptanceBaseline{
		declaredParcelIDs: request.currentVersion.DeclaredParcelIDs(),
		submissionVersion: request.currentVersion.versionID,
		fixedAt:           spec.DecidedAt,
	}
	request.commitment = ExpectedCommitment{basis: spec.Basis, formedAt: spec.DecidedAt}
	return request, nil
}

// everyMemberJudged guards against accepting a version in which some declared
// member was never assessed. An unjudged member is not a passing member, and
// treating it as one would be member-level acceptance by omission.
func (request ShipmentRequest) everyMemberJudged(judged map[DeclaredParcelID]struct{}) bool {
	for _, parcelID := range request.currentVersion.declaredParcelIDs {
		if _, present := judged[parcelID]; !present {
			return false
		}
	}
	return true
}

func (request ShipmentRequest) AcceptanceDecision() (AcceptanceDecision, bool) {
	return request.decision, request.decisionFormed
}

func (request ShipmentRequest) AcceptanceBaseline() (AcceptanceBaseline, bool) {
	if len(request.baseline.declaredParcelIDs) == 0 {
		return AcceptanceBaseline{}, false
	}
	return request.baseline, true
}

func (request ShipmentRequest) ExpectedCommitment() (ExpectedCommitment, bool) {
	if request.commitment.formedAt.IsZero() {
		return ExpectedCommitment{}, false
	}
	return request.commitment, true
}
