package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidShipmentRequest     = errors.New("parcel shipment: invalid shipment request")
	ErrFutureSubmissionNotAllowed = errors.New("parcel shipment: future submission is not allowed")
)

type SubmissionVersionID struct{ requiredValue }

func NewSubmissionVersionID(value string) (SubmissionVersionID, error) {
	required, err := newRequiredValue("submission version ID", value)
	return SubmissionVersionID{required}, err
}

type AcceptanceDecisionTaskID struct{ requiredValue }

func NewAcceptanceDecisionTaskID(value string) (AcceptanceDecisionTaskID, error) {
	required, err := newRequiredValue("acceptance decision task ID", value)
	return AcceptanceDecisionTaskID{required}, err
}

type ShipmentRequestState uint8

const (
	ShipmentRequestStateInvalid ShipmentRequestState = iota
	ShipmentRequestSubmitted
	ShipmentRequestAccepted
	ShipmentRequestRejected
)

func (state ShipmentRequestState) String() string {
	switch state {
	case ShipmentRequestSubmitted:
		return "SUBMITTED"
	case ShipmentRequestAccepted:
		return "ACCEPTED"
	case ShipmentRequestRejected:
		return "REJECTED"
	default:
		return ""
	}
}

// SubmissionVersion is the uncoverable record of what the customer currently
// asks for. Correction produces a further version rather than editing this one.
type SubmissionVersion struct {
	versionID         SubmissionVersionID
	sourceSubmission  SourceSubmissionFingerprint
	declaredParcelIDs []DeclaredParcelID
	establishedAt     time.Time
}

func (version SubmissionVersion) VersionID() SubmissionVersionID {
	return version.versionID
}

func (version SubmissionVersion) SourceSubmission() SourceSubmissionFingerprint {
	return version.sourceSubmission
}

func (version SubmissionVersion) DeclaredParcelIDs() []DeclaredParcelID {
	return append([]DeclaredParcelID(nil), version.declaredParcelIDs...)
}

func (version SubmissionVersion) EstablishedAt() time.Time {
	return version.establishedAt
}

// AcceptanceDecisionTask tracks continuable acceptance work for one submission
// version. It is not a request state: while it is open the request stays
// submitted, and establishing it is not acceptance.
type AcceptanceDecisionTask struct {
	taskID              AcceptanceDecisionTaskID
	submissionVersionID SubmissionVersionID
	establishedAt       time.Time
	complete            bool
}

func (task AcceptanceDecisionTask) TaskID() AcceptanceDecisionTaskID {
	return task.taskID
}

func (task AcceptanceDecisionTask) SubmissionVersionID() SubmissionVersionID {
	return task.submissionVersionID
}

func (task AcceptanceDecisionTask) EstablishedAt() time.Time {
	return task.establishedAt
}

func (task AcceptanceDecisionTask) IsComplete() bool {
	return task.complete
}

type SubmitShipmentRequestSpec struct {
	Candidate   SubmissionCandidate
	Gate        FutureSubmissionGate
	VersionID   SubmissionVersionID
	TaskID      AcceptanceDecisionTaskID
	SubmittedAt time.Time
}

type ShipmentRequest struct {
	shipmentRequestID ShipmentRequestID
	batchID           SubmissionBatchID
	state             ShipmentRequestState
	currentVersion    SubmissionVersion
	acceptanceTask    AcceptanceDecisionTask
	submittedAt       time.Time
	decision          AcceptanceDecision
	decisionFormed    bool
	baseline          AcceptanceBaseline
	commitment        ExpectedCommitment
}

// SubmitShipmentRequest establishes a submitted request behind an allowed
// future-submission gate. It forms no acceptance or rejection.
func SubmitShipmentRequest(spec SubmitShipmentRequestSpec) (ShipmentRequest, error) {
	if !spec.Candidate.valid() ||
		!spec.VersionID.valid() ||
		!spec.TaskID.valid() ||
		spec.SubmittedAt.IsZero() {
		return ShipmentRequest{}, ErrInvalidShipmentRequest
	}
	if !spec.Gate.valid() {
		return ShipmentRequest{}, ErrInvalidFutureSubmissionGate
	}
	if !spec.Gate.IsAllowed() {
		return ShipmentRequest{}, ErrFutureSubmissionNotAllowed
	}

	return ShipmentRequest{
		shipmentRequestID: spec.Candidate.ShipmentRequestID(),
		batchID:           spec.Candidate.BatchID(),
		state:             ShipmentRequestSubmitted,
		currentVersion: SubmissionVersion{
			versionID:         spec.VersionID,
			sourceSubmission:  spec.Candidate.SourceSubmission(),
			declaredParcelIDs: spec.Candidate.DeclaredParcelIDs(),
			establishedAt:     spec.SubmittedAt,
		},
		acceptanceTask: AcceptanceDecisionTask{
			taskID:              spec.TaskID,
			submissionVersionID: spec.VersionID,
			establishedAt:       spec.SubmittedAt,
		},
		submittedAt: spec.SubmittedAt,
	}, nil
}

func (request ShipmentRequest) ShipmentRequestID() ShipmentRequestID {
	return request.shipmentRequestID
}

func (request ShipmentRequest) BatchID() SubmissionBatchID {
	return request.batchID
}

func (request ShipmentRequest) State() ShipmentRequestState {
	return request.state
}

func (request ShipmentRequest) CurrentSubmissionVersion() SubmissionVersion {
	return request.currentVersion
}

func (request ShipmentRequest) AcceptanceDecisionTask() AcceptanceDecisionTask {
	return request.acceptanceTask
}

func (request ShipmentRequest) SubmittedAt() time.Time {
	return request.submittedAt
}
