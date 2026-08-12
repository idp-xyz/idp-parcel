package transportfulfillment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/transportfulfillment"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

var pickedUpAt = time.Date(2026, 8, 9, 8, 15, 0, 0, time.UTC)

func value[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

func identity(t *testing.T) psdomain.SourceIdentity {
	t.Helper()
	built, err := psdomain.NewSourceIdentity(
		value(t, psdomain.NewTenantID, "tenant-1"),
		value(t, psdomain.NewCustomerAccountID, "customer-1"),
		value(t, psdomain.NewSource, "source-a"),
		value(t, psdomain.NewSourceRequestKey, "key-1"),
	)
	if err != nil {
		t.Fatalf("new source identity: %v", err)
	}
	return built
}

// acceptedRequest 真经领域把委托推到已接受，形状与节点收寄适配器的夹具一致。
func acceptedRequest(t *testing.T) psdomain.ShipmentRequest {
	t.Helper()
	fingerprint, err := psdomain.NewSourceSubmissionFingerprint(
		identity(t),
		value(t, psdomain.NewPayloadDigest, "digest-1"),
		pickedUpAt.Add(-2*time.Hour),
		pickedUpAt.Add(-2*time.Hour+time.Second),
	)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	candidate, err := psdomain.NewSubmissionCandidate(
		fingerprint,
		value(t, psdomain.NewSubmissionBatchID, "batch-1"),
		value(t, psdomain.NewShipmentRequestID, "request-1"),
		[]psdomain.DeclaredParcelID{value(t, psdomain.NewDeclaredParcelID, "parcel-1")},
	)
	if err != nil {
		t.Fatalf("candidate: %v", err)
	}
	scope, err := psdomain.NewAdmissionScope(
		value(t, psdomain.NewAdmissionScopeReference, "scope-ref-1"),
		value(t, psdomain.NewAdmissionScopeDigest, "scope-1"),
	)
	if err != nil {
		t.Fatalf("admission scope: %v", err)
	}
	validity, err := psdomain.NewOwnershipValidityInterval(pickedUpAt.Add(-48*time.Hour), pickedUpAt.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("validity: %v", err)
	}
	decidedAt := pickedUpAt.Add(-90 * time.Minute)
	ownership, err := psdomain.NewProductionOwnershipDecision(psdomain.ProductionOwnershipDecisionSpec{
		DecisionID:       value(t, psdomain.NewProductionOwnershipDecisionID, "decision-1"),
		Scope:            scope,
		Authority:        psdomain.ProductionAuthorityIDPParcel,
		AdmissionControl: psdomain.AdmissionControlOpen,
		RuleVersion:      value(t, psdomain.NewProductionOwnershipRuleVersion, "rule-1"),
		AsOf:             decidedAt,
		Validity:         validity,
		Revision:         value(t, psdomain.NewProductionOwnershipRevision, "rev-1"),
		DecisionAt:       decidedAt,
	})
	if err != nil {
		t.Fatalf("ownership decision: %v", err)
	}
	gate, err := psdomain.EvaluateFutureSubmissionGate(ownership, scope.Digest(),
		value(t, psdomain.NewProductionOwnershipRevision, "rev-1"), decidedAt)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	request, err := psdomain.SubmitShipmentRequest(psdomain.SubmitShipmentRequestSpec{
		Candidate:   candidate,
		Gate:        gate,
		VersionID:   value(t, psdomain.NewSubmissionVersionID, "version-1"),
		TaskID:      value(t, psdomain.NewAcceptanceDecisionTaskID, "task-1"),
		SubmittedAt: decidedAt,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	applicable, err := psdomain.NewApplicableCheckGroups(psdomain.NetworkReachabilityCheck)
	if err != nil {
		t.Fatalf("applicable groups: %v", err)
	}
	snapshot, err := psdomain.NewCommercialBasisSnapshot(psdomain.CommercialBasisSnapshotSpec{
		ResolutionID: value(t, psdomain.NewCommercialResolutionID, "RES-1"),
		RulePackage:  value(t, psdomain.NewRulePackageReference, "rules-1/v1"),
		ViewRevision: value(t, psdomain.NewCommercialViewRevision, "VIEW-1"),
		Applicable:   applicable,
		ManualReview: psdomain.ManualReviewNotRequiredByRules,
	})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	check, err := psdomain.NewAcceptanceCheck(
		psdomain.NetworkReachabilityCheck,
		value(t, psdomain.NewDeclaredParcelID, "parcel-1"),
		psdomain.CheckPassed,
		psdomain.CheckReason{},
	)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	accepted, err := request.Decide(psdomain.AcceptanceDecisionSpec{
		DecisionID: value(t, psdomain.NewAcceptanceDecisionID, "decision-1"),
		Checks:     []psdomain.AcceptanceCheck{check},
		Basis:      snapshot,
		DecidedAt:  pickedUpAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	return accepted
}

type requestStoreDouble struct {
	records map[psdomain.SourceIdentity]psdomain.ShipmentRequest
}

func (double *requestStoreDouble) FindBySourceIdentity(
	_ context.Context,
	identity psdomain.SourceIdentity,
) (psdomain.ShipmentRequest, bool, error) {
	record, found := double.records[identity]
	return record, found, nil
}

func (double *requestStoreDouble) Insert(
	_ context.Context, _ psdomain.SourceIdentity, _ psdomain.ShipmentRequest,
) (psports.ShipmentRequestInsertOutcome, error) {
	return 0, errors.New("not part of this seam")
}

func (double *requestStoreDouble) Save(
	_ context.Context, _ psdomain.SourceIdentity, _ psdomain.ShipmentRequest,
) (psports.ShipmentRequestSaveOutcome, error) {
	return 0, errors.New("not part of this seam")
}

type eligibilityDouble struct{}

func (eligibilityDouble) JudgeIntakeEligibility(
	_ context.Context, _ psdomain.SourceIdentity, _ psdomain.ShipmentRequestID, _ psdomain.IntakeSource,
) (psports.IntakeEligibility, bool, error) {
	return psports.IntakeEligibility{Outcome: psports.IntakeEligibilityEstablished}, true, nil
}

type adoptionStoreDouble struct {
	byKey map[psports.IntakeAdoptionKey]psports.IntakeAdoptionRecord
}

func (double *adoptionStoreDouble) FindByKey(
	_ context.Context, key psports.IntakeAdoptionKey,
) (psports.IntakeAdoptionRecord, bool, error) {
	record, found := double.byKey[key]
	return record, found, nil
}

func (double *adoptionStoreDouble) FindResponsibilityStart(
	_ context.Context, tenant psdomain.TenantID, parcel psdomain.DeclaredParcelID,
) (psports.IntakeAdoptionRecord, bool, error) {
	for _, record := range double.byKey {
		if record.Key.TenantID == tenant && record.Key.Parcel == parcel && record.Adopted {
			return record, true, nil
		}
	}
	return psports.IntakeAdoptionRecord{}, false, nil
}

func (double *adoptionStoreDouble) Save(
	_ context.Context, record psports.IntakeAdoptionRecord,
) (psports.IntakeAdoptionSaveOutcome, error) {
	double.byKey[record.Key] = record
	return psports.IntakeAdoptionSaved, nil
}

type downstreamDouble struct{}

func (downstreamDouble) HandOffNetworkIntake(_ context.Context, _ psports.NetworkIntakeHandoffIntent) error {
	return nil
}

type commitmentIdentityDouble struct{ next int }

func (double *commitmentIdentityDouble) NextCommitmentVersionID(_ context.Context) (psdomain.CommitmentVersionID, error) {
	double.next++
	return psdomain.NewCommitmentVersionID("commitment-" + string(rune('0'+double.next)))
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

// Covers: `AT-PS-039` 经适配器端到端——合格场外揽收译成来源采用命令走完真实采用编排：
// 同语义形成正式承诺，生效恒等于实际接货时间，不虚构节点到站（地点是实际接货位置）。
func TestAnOffsitePickupFlowsThroughToACommitment(t *testing.T) {
	requests := &requestStoreDouble{records: map[psdomain.SourceIdentity]psdomain.ShipmentRequest{
		identity(t): acceptedRequest(t),
	}}
	handler := psapplication.NewAdoptNetworkIntakeHandler(psapplication.AdoptNetworkIntakeDeps{
		Requests:    requests,
		Eligibility: eligibilityDouble{},
		Adoptions:   &adoptionStoreDouble{byKey: map[psports.IntakeAdoptionKey]psports.IntakeAdoptionRecord{}},
		Identities:  &commitmentIdentityDouble{},
		Downstream:  downstreamDouble{},
		Clock:       fixedClock{at: pickedUpAt.Add(time.Minute)},
	})
	subject := adapter.NewOffsitePickupAdapter(handler)

	pickup, err := tfdomain.FormOffsitePickup(tfdomain.OffsitePickupSpec{
		TenantID:   value(t, tfdomain.NewTenantID, "tenant-1"),
		Object:     value(t, tfdomain.NewCarriedObjectReference, "parcel-1"),
		Task:       value(t, tfdomain.NewPickupTaskReference, "pickup-task-1"),
		Attempt:    value(t, tfdomain.NewAttemptReference, "attempt-1"),
		Place:      value(t, tfdomain.NewPickupPlaceReference, "customer-warehouse-1"),
		Control:    value(t, tfdomain.NewTransportControlReference, "TF-3"),
		ExecutedBy: value(t, tfdomain.NewExecutingPartyReference, "courier-1"),
		Version:    value(t, tfdomain.NewPickupResultVersion, "pickup-result/v1"),
		OccurredAt: pickedUpAt,
	})
	if err != nil {
		t.Fatalf("form offsite pickup: %v", err)
	}

	result, err := subject.AdoptFromOffsitePickup(context.Background(), pickup, adapter.TargetShipment{
		Identity:          identity(t),
		ShipmentRequestID: value(t, psdomain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: value(t, psdomain.NewSubmissionVersionID, "version-1"),
	})
	if err != nil {
		t.Fatalf("adopt from offsite pickup: %v", err)
	}

	if result.Outcome() != psapplication.IntakeCommitmentFormed {
		t.Fatalf("outcome = %q, want COMMITMENT_FORMED", result.Outcome())
	}
	record, _ := result.Record()
	if record.Key.Kind != psdomain.OffsitePickupSource {
		t.Fatalf("kind = %q", record.Key.Kind)
	}
	if !record.Commitment.EffectiveAt().Equal(pickedUpAt) {
		t.Fatalf("effective at = %s, want the pickup time", record.Commitment.EffectiveAt())
	}
	if record.Intake.Source().Place().String() != "customer-warehouse-1" {
		t.Fatal("收寄地点必须是实际接货位置，不虚构节点到站")
	}
	if record.Intake.Source().Control().String() != "OFFSITE-PICKUP/TF-3" {
		t.Fatalf("control = %q", record.Intake.Source().Control())
	}
}
