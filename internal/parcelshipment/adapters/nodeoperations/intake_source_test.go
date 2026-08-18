package nodeoperations_test

import (
	"context"
	"errors"
	"testing"
	"time"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/nodeoperations"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var receivedAt = time.Date(2026, 8, 9, 7, 30, 0, 0, time.UTC)

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

// acceptedRequest 真经领域把委托推到已接受（假状态钉不住采用编排的状态检查）。
func acceptedRequest(t *testing.T) psdomain.ShipmentRequest {
	t.Helper()
	fingerprint, err := psdomain.NewSourceSubmissionFingerprint(
		identity(t),
		value(t, psdomain.NewPayloadDigest, "digest-1"),
		receivedAt.Add(-2*time.Hour),
		receivedAt.Add(-2*time.Hour+time.Second),
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
	validity, err := psdomain.NewOwnershipValidityInterval(receivedAt.Add(-48*time.Hour), receivedAt.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("validity: %v", err)
	}
	decidedAt := receivedAt.Add(-90 * time.Minute)
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
		DecidedAt:  receivedAt.Add(-time.Hour),
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

func adoptHandler(t *testing.T) *psapplication.AdoptNetworkIntakeHandler {
	t.Helper()
	return newAdoptHandler(t, adoptHandlerConfig{})
}

type adoptHandlerConfig struct {
	downstream  psports.NetworkIntakeHandoff
	eligibility psports.IntakeEligibilityView
	adoptions   *adoptionStoreDouble
	identity    psdomain.SourceIdentity
	request     psdomain.ShipmentRequest
}

func newAdoptHandler(t *testing.T, config adoptHandlerConfig) *psapplication.AdoptNetworkIntakeHandler {
	t.Helper()
	downstream := config.downstream
	if downstream == nil {
		downstream = downstreamDouble{}
	}
	eligibility := config.eligibility
	if eligibility == nil {
		eligibility = eligibilityDouble{}
	}
	adoptions := config.adoptions
	if adoptions == nil {
		adoptions = &adoptionStoreDouble{byKey: map[psports.IntakeAdoptionKey]psports.IntakeAdoptionRecord{}}
	}
	id := config.identity
	if id == (psdomain.SourceIdentity{}) {
		id = identity(t)
	}
	request := config.request
	if request.ShipmentRequestID().String() == "" {
		request = acceptedRequest(t)
	}
	return psapplication.NewAdoptNetworkIntakeHandler(psapplication.AdoptNetworkIntakeDeps{
		Requests: &requestStoreDouble{records: map[psdomain.SourceIdentity]psdomain.ShipmentRequest{
			id: request,
		}},
		Eligibility: eligibility,
		Adoptions:   adoptions,
		Identities:  &commitmentIdentityDouble{},
		Downstream:  downstream,
		Clock:       fixedClock{at: receivedAt.Add(time.Minute)},
	})
}

func identifiedIntake(t *testing.T) nodomain.NodeIntake {
	t.Helper()
	intake, err := nodomain.FormNodeIntake(nodomain.NodeIntakeSpec{
		TenantID:    value(t, nodomain.NewTenantID, "tenant-1"),
		Unit:        value(t, nodomain.NewHandlingUnitID, "unit-1"),
		Node:        value(t, nodomain.NewNodeReference, "node-origin"),
		DeliveredBy: value(t, nodomain.NewDeliveringPartyReference, "customer-1"),
		Evidence:    value(t, nodomain.NewReceptionEvidenceReference, "SIGN-7"),
		Version:     value(t, nodomain.NewIntakeResultVersion, "intake-result/v1"),
		Association: value(t, nodomain.NewParcelAssociationReference, "parcel-1"),
		ReceivedAt:  receivedAt,
	})
	if err != nil {
		t.Fatalf("form node intake: %v", err)
	}
	return intake
}

func target(t *testing.T) adapter.TargetShipment {
	t.Helper()
	return adapter.TargetShipment{
		Identity:          identity(t),
		ShipmentRequestID: value(t, psdomain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: value(t, psdomain.NewSubmissionVersionID, "version-1"),
	}
}

// Covers: `AT-PS-038` 经适配器端到端——已识别的节点收寄译成来源采用命令走完真实采用
// 编排：正式承诺形成、生效恒等于节点接收时间、来源类型与控制依据逐维译到位。
func TestANodeIntakeFlowsThroughToACommitment(t *testing.T) {
	subject := adapter.NewNodeIntakeAdapter(adoptHandler(t))

	result, err := subject.AdoptFromNodeIntake(context.Background(), identifiedIntake(t), target(t))
	if err != nil {
		t.Fatalf("adopt from node intake: %v", err)
	}

	if result.Outcome() != psapplication.IntakeCommitmentFormed {
		t.Fatalf("outcome = %q, want COMMITMENT_FORMED", result.Outcome())
	}
	record, _ := result.Record()
	if record.Key.Kind != psdomain.NodeIntakeSource {
		t.Fatalf("kind = %q", record.Key.Kind)
	}
	if !record.Commitment.EffectiveAt().Equal(receivedAt) {
		t.Fatalf("effective at = %s, want the node reception time", record.Commitment.EffectiveAt())
	}
	if record.Intake.Source().Control().String() != "NODE-INTAKE/SIGN-7" {
		t.Fatalf("control = %q; 控制依据必须带来源前缀译过来", record.Intake.Source().Control())
	}
}

// Covers: 待识别实物的诚实防线——没有版本化包裹关联的收寄真实存在，但采用判断没有
// 包裹身份可锚：独立哨兵拒绝，等识别是业务续办不是修适配器；不拿作业实物标识冒充
// 正式包裹身份。
func TestAnUnidentifiedUnitIsRefusedWithItsOwnSentinel(t *testing.T) {
	subject := adapter.NewNodeIntakeAdapter(adoptHandler(t))
	pending, err := nodomain.FormNodeIntake(nodomain.NodeIntakeSpec{
		TenantID:    value(t, nodomain.NewTenantID, "tenant-1"),
		Unit:        value(t, nodomain.NewHandlingUnitID, "unit-1"),
		Node:        value(t, nodomain.NewNodeReference, "node-origin"),
		DeliveredBy: value(t, nodomain.NewDeliveringPartyReference, "customer-1"),
		Evidence:    value(t, nodomain.NewReceptionEvidenceReference, "SIGN-7"),
		Version:     value(t, nodomain.NewIntakeResultVersion, "intake-result/v1"),
		ReceivedAt:  receivedAt,
	})
	if err != nil {
		t.Fatalf("form pending intake: %v", err)
	}

	if _, err := subject.AdoptFromNodeIntake(context.Background(), pending, target(t)); !errors.Is(err, adapter.ErrUnidentifiedHandlingUnit) {
		t.Fatalf("err = %v, want ErrUnidentifiedHandlingUnit", err)
	}
}
