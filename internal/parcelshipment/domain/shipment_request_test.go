package domain_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

var submittedAt = time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)

func allowedGate(t *testing.T, scopeDigest, revision string) domain.FutureSubmissionGate {
	t.Helper()
	decision := ownershipDecision(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlOpen, scopeDigest, revision)
	gate, err := domain.EvaluateFutureSubmissionGate(
		decision,
		mustValue(t, domain.NewAdmissionScopeDigest, scopeDigest),
		mustValue(t, domain.NewProductionOwnershipRevision, revision),
		time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("evaluate gate: %v", err)
	}
	if !gate.IsAllowed() {
		t.Fatalf("fixture gate is blocked: %v", gate.BlockReasons())
	}
	return gate
}

func submitSpec(t *testing.T, parcelIDs ...string) domain.SubmitShipmentRequestSpec {
	t.Helper()
	declared := make([]domain.DeclaredParcelID, len(parcelIDs))
	for index, value := range parcelIDs {
		declared[index] = mustValue(t, domain.NewDeclaredParcelID, value)
	}
	candidate, err := domain.NewSubmissionCandidate(
		sourceFingerprint(t, "tenant-1", "customer-1", "source-a", "key-1", "digest-1"),
		mustValue(t, domain.NewSubmissionBatchID, "batch-1"),
		mustValue(t, domain.NewShipmentRequestID, "request-1"),
		declared,
	)
	if err != nil {
		t.Fatalf("new submission candidate: %v", err)
	}
	return domain.SubmitShipmentRequestSpec{
		Candidate:   candidate,
		Gate:        allowedGate(t, "scope-1", "rev-1"),
		VersionID:   mustValue(t, domain.NewSubmissionVersionID, "version-1"),
		TaskID:      mustValue(t, domain.NewAcceptanceDecisionTaskID, "task-1"),
		SubmittedAt: submittedAt,
	}
}

// Covers: UC-PS-001 步骤 3C — 建立委托、它的当前提交版本、申报包裹与接受决策任务，都不等于
// 接受。
func TestSubmitShipmentRequestRecordsSubmittedWithoutAcceptance(t *testing.T) {
	spec := submitSpec(t, "parcel-1", "parcel-2")

	request, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("submit shipment request: %v", err)
	}

	if request.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q, want SUBMITTED", request.State())
	}
	if request.ShipmentRequestID() != spec.Candidate.ShipmentRequestID() ||
		request.BatchID() != spec.Candidate.BatchID() {
		t.Fatalf("request lost its candidate identity: %#v", request)
	}
	if request.SubmittedAt() != submittedAt {
		t.Fatalf("submitted at = %v, want %v", request.SubmittedAt(), submittedAt)
	}

	version := request.CurrentSubmissionVersion()
	if version.VersionID() != spec.VersionID {
		t.Fatalf("current version = %q, want version-1", version.VersionID())
	}
	if got, want := stringValues(version.DeclaredParcelIDs()), []string{"parcel-1", "parcel-2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("declared parcels = %v, want %v", got, want)
	}
	if version.SourceSubmission() != spec.Candidate.SourceSubmission() {
		t.Fatal("current version lost the preserved source fingerprint")
	}

	task := request.AcceptanceDecisionTask()
	if task.TaskID() != spec.TaskID {
		t.Fatalf("task = %q, want task-1", task.TaskID())
	}
	if task.SubmissionVersionID() != spec.VersionID {
		t.Fatal("acceptance decision task is not bound to the current submission version")
	}
	if task.IsComplete() {
		t.Fatal("a freshly established acceptance decision task reported completion")
	}
}

// Covers: UC-PS-001 步骤 3C — 只有本产品持有生产归属才准入委托；被阻断的门禁不得产出委托。
func TestSubmitShipmentRequestRefusesABlockedGate(t *testing.T) {
	blocking := map[string]domain.FutureSubmissionGate{
		"other authority":  blockedGate(t, domain.ProductionAuthorityOther, domain.AdmissionControlOpen),
		"unresolved":       blockedGate(t, domain.ProductionAuthorityUnresolved, domain.AdmissionControlOpen),
		"admission paused": blockedGate(t, domain.ProductionAuthorityIDPParcel, domain.AdmissionControlPaused),
	}

	for name, gate := range blocking {
		t.Run(name, func(t *testing.T) {
			spec := submitSpec(t, "parcel-1")
			spec.Gate = gate

			request, err := domain.SubmitShipmentRequest(spec)
			if !errors.Is(err, domain.ErrFutureSubmissionNotAllowed) {
				t.Fatalf("error = %v, want ErrFutureSubmissionNotAllowed", err)
			}
			if request.State() != domain.ShipmentRequestStateInvalid {
				t.Fatalf("a blocked gate still produced a request in state %q", request.State())
			}
		})
	}
}

func blockedGate(
	t *testing.T,
	authority domain.ProductionAuthorityKind,
	control domain.AdmissionControl,
) domain.FutureSubmissionGate {
	t.Helper()
	decision := ownershipDecision(t, authority, control, "scope-1", "rev-1")
	gate, err := domain.EvaluateFutureSubmissionGate(
		decision,
		mustValue(t, domain.NewAdmissionScopeDigest, "scope-1"),
		mustValue(t, domain.NewProductionOwnershipRevision, "rev-1"),
		time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("evaluate gate: %v", err)
	}
	if gate.IsAllowed() {
		t.Fatal("fixture gate was expected to block")
	}
	return gate
}
