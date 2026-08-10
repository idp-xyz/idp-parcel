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

// SubmissionVersion 是客户当前请求内容的不可覆盖记录。纠错形成新的提交版本，而不是
// 就地修改这一份。
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

// AcceptanceDecisionTask 记录一个提交版本上可续办的接受判断工作。它不是委托的领域
// 状态：任务未完成时委托仍为`已提交`，任务的建立本身也不构成接受。
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

// SubmitShipmentRequest 在放行的建单门禁之后建立一份`已提交`委托。它不形成接受或
// 拒绝。
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
