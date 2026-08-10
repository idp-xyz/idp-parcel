package application_test

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: UC-PS-001 步骤 1、2、3A、3B、3C — 先保全来源再判定归属，且只有本产品对整个准入
// 范围持有生产权威之后才建立委托。
func TestSubmitEstablishesSubmittedRequestAfterPreservingSource(t *testing.T) {
	fixture := newFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.OutcomeSubmitted {
		t.Fatalf("outcome = %q, want SUBMITTED", result.Outcome())
	}
	if got, want := fixture.calls, []string{"preserve-source", "decide-ownership", "insert-request"}; !slices.Equal(got, want) {
		t.Fatalf("call order = %v, want %v", got, want)
	}

	stored, found := fixture.requests.stored(t, fixture.identity(t))
	if !found {
		t.Fatal("no shipment request was stored for the submitted source")
	}
	if stored.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("stored state = %q, want SUBMITTED", stored.State())
	}
	if stored.AcceptanceDecisionTask().IsComplete() {
		t.Fatal("submission completed the acceptance decision task")
	}
	requestID, present := result.ShipmentRequestID()
	if !present || requestID != stored.ShipmentRequestID() {
		t.Fatal("result does not reference the stored request")
	}
}

// Covers: UC-PS-001 结果语义 — 同一逻辑请求的重放返回原结果，而不是造出第二份委托。
func TestSubmitReplayReturnsExistingResultWithoutASecondRequest(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	fixture.calls = nil

	replay, err := fixture.handler.Handle(ctx, fixture.command(t))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}

	if replay.Outcome() != application.OutcomeExistingResult {
		t.Fatalf("replay outcome = %q, want EXISTING_RESULT", replay.Outcome())
	}
	replayed, present := replay.ShipmentRequestID()
	original, _ := first.ShipmentRequestID()
	if !present || replayed != original {
		t.Fatal("replay returned a different shipment request")
	}
	if fixture.requests.insertCount != 1 {
		t.Fatalf("insert count = %d, want 1", fixture.requests.insertCount)
	}
	if fixture.ownership.decideCount != 1 {
		t.Fatalf("replay decided ownership again: %d decisions", fixture.ownership.decideCount)
	}
}

// Covers: UC-PS-001 一致性幂等与并发 — occurredAt/receivedAt 不同仍算重放，只追加这次观察；
// 它不重新保全、不重新判定，也不覆盖原始来源事实。
func TestSubmitReplayAppendsTheObservationWithoutOverwriting(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()

	if _, err := fixture.handler.Handle(ctx, fixture.command(t)); err != nil {
		t.Fatalf("first handle: %v", err)
	}

	later := fixture.command(t)
	later.OccurredAt = later.OccurredAt.Add(time.Hour)
	later.ReceivedAt = later.ReceivedAt.Add(time.Hour)

	replay, err := fixture.handler.Handle(ctx, later)
	if err != nil {
		t.Fatalf("later handle: %v", err)
	}

	if replay.Outcome() != application.OutcomeExistingResult {
		t.Fatalf("outcome = %q, want EXISTING_RESULT; timestamps must not reclassify a replay", replay.Outcome())
	}
	if got := len(fixture.sources.observations); got != 1 {
		t.Fatalf("appended observations = %d, want 1", got)
	}
	observed := fixture.sources.observations[0]
	if !observed.OccurredAt().Equal(later.OccurredAt) || !observed.ReceivedAt().Equal(later.ReceivedAt) {
		t.Fatalf("appended observation carries %v/%v, want the later timestamps", observed.OccurredAt(), observed.ReceivedAt())
	}

	preserved, _ := fixture.sources.stored(t, fixture.identity(t))
	if !preserved.OccurredAt().Equal(time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("the observation overwrote the preserved source: %v", preserved.OccurredAt())
	}
	if fixture.sources.preserveCount != 1 {
		t.Fatalf("preserve count = %d, want 1", fixture.sources.preserveCount)
	}
}

// Covers: UC-PS-001 结果语义 — 同一逻辑身份携带不同的规范化载荷是冲突，绝不是第二份委托。
func TestSubmitConflictPreservesTheOriginalWithoutBuilding(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()

	if _, err := fixture.handler.Handle(ctx, fixture.command(t)); err != nil {
		t.Fatalf("first handle: %v", err)
	}

	changed := fixture.command(t)
	changed.PayloadDigest = mustValue(t, domain.NewPayloadDigest, "digest-2")
	conflict, err := fixture.handler.Handle(ctx, changed)
	if err != nil {
		t.Fatalf("conflicting handle: %v", err)
	}

	if conflict.Outcome() != application.OutcomeIngressConflict {
		t.Fatalf("outcome = %q, want INGRESS_CONFLICT", conflict.Outcome())
	}
	if fixture.requests.insertCount != 1 {
		t.Fatalf("a conflict created a second request: %d inserts", fixture.requests.insertCount)
	}
	preserved, _ := fixture.sources.stored(t, fixture.identity(t))
	if preserved.Digest().String() != "digest-1" {
		t.Fatalf("conflict overwrote the preserved payload: %q", preserved.Digest())
	}
}

// Covers: UC-PS-001 结果语义与首发试点叠加条件 — 三种拒绝彼此分明。暂停回答的是本产品此刻
// 是否接新活，而不是范围归谁，所以它不得被报成归属未决。
func TestSubmitFormsNoRequestWhenThisProductLacksAuthority(t *testing.T) {
	cases := map[string]struct {
		authority domain.ProductionAuthorityKind
		control   domain.AdmissionControl
		want      application.SubmitOutcome
	}{
		"other authority":  {domain.ProductionAuthorityOther, domain.AdmissionControlOpen, application.OutcomeOtherProductionAuthority},
		"unresolved":       {domain.ProductionAuthorityUnresolved, domain.AdmissionControlOpen, application.OutcomeOwnershipUnresolved},
		"admission paused": {domain.ProductionAuthorityIDPParcel, domain.AdmissionControlPaused, application.OutcomeAdmissionPaused},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newFixture(t)
			fixture.ownership.authority = testCase.authority
			fixture.ownership.control = testCase.control

			result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}

			if result.Outcome() != testCase.want {
				t.Fatalf("outcome = %q, want %q", result.Outcome(), testCase.want)
			}
			if _, present := result.ShipmentRequestID(); present {
				t.Fatal("a refusal named a shipment request")
			}
			if decision, present := result.OwnershipDecision(); !present || decision.Authority() != testCase.authority {
				t.Fatal("a refusal did not report the ownership decision behind it")
			}
			if len(result.GateBlockReasons()) == 0 {
				t.Fatal("a refusal reported no gate block reason")
			}
			if fixture.requests.insertCount != 0 {
				t.Fatalf("a request was built without production authority: %d inserts", fixture.requests.insertCount)
			}
			if _, found := fixture.sources.stored(t, fixture.identity(t)); !found {
				t.Fatal("the source was not preserved even though ownership was evaluated")
			}
		})
	}
}

// Covers: UC-PS-001 步骤 3A 先于 3B — 立不起最小委托身份的输入不产生占位委托，也没有值得
// 拿去问权威的准入范围。
func TestSubmitWithoutDeclaredParcelsIsNotAccepted(t *testing.T) {
	fixture := newFixture(t)
	command := fixture.command(t)
	command.DeclaredParcelIDs = nil

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.OutcomeInputNotAccepted {
		t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", result.Outcome())
	}
	if fixture.ownership.decideCount != 0 {
		t.Fatal("ownership was decided for input that established no request identity")
	}
	if fixture.requests.insertCount != 0 {
		t.Fatalf("a placeholder request was created: %d inserts", fixture.requests.insertCount)
	}
	if _, found := fixture.sources.stored(t, fixture.identity(t)); !found {
		t.Fatal("unusable input discarded the preserved source")
	}
}

// Covers: UC-PS-001 — 一次查询不得泄露某个对象存在于另一个租户或客户范围里。
func TestSubmitIsolatesScopesSharingASourceRequestKey(t *testing.T) {
	fixture := newFixture(t)
	ctx := context.Background()

	if _, err := fixture.handler.Handle(ctx, fixture.command(t)); err != nil {
		t.Fatalf("first handle: %v", err)
	}

	neighbour := fixture.command(t)
	neighbour.Identity = sourceIdentity(t, "tenant-2", "customer-1", "source-a", "key-1")
	neighbour.ShipmentRequestID = mustValue(t, domain.NewShipmentRequestID, "request-2")

	result, err := fixture.handler.Handle(ctx, neighbour)
	if err != nil {
		t.Fatalf("neighbour handle: %v", err)
	}

	if result.Outcome() != application.OutcomeSubmitted {
		t.Fatalf("neighbour outcome = %q, want SUBMITTED", result.Outcome())
	}
	if fixture.requests.insertCount != 2 {
		t.Fatalf("insert count = %d, want 2 independent requests", fixture.requests.insertCount)
	}
}

// Covers: 本切片不得越过的边界 — 仓储失败不得被悄悄吞成一个业务结果。
func TestSubmitSurfacesPreservationFailureRatherThanDeciding(t *testing.T) {
	fixture := newFixture(t)
	failure := errors.New("preservation unavailable")
	fixture.sources.preserveErr = failure

	_, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if !errors.Is(err, failure) {
		t.Fatalf("error = %v, want the preservation failure", err)
	}
	if fixture.ownership.decideCount != 0 {
		t.Fatal("ownership was decided after the source failed to be preserved")
	}
}

type fixture struct {
	handler   *application.SubmitShipmentRequestHandler
	sources   *sourceRepositoryDouble
	requests  *shipmentRequestRepositoryDouble
	ownership *ownershipAuthorityDouble
	calls     []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	value := &fixture{}
	record := func(name string) { value.calls = append(value.calls, name) }

	value.sources = &sourceRepositoryDouble{records: map[domain.SourceIdentity]domain.SourceSubmissionFingerprint{}, record: record}
	value.requests = &shipmentRequestRepositoryDouble{records: map[domain.SourceIdentity]domain.ShipmentRequest{}, record: record}
	value.ownership = &ownershipAuthorityDouble{
		t:         t,
		authority: domain.ProductionAuthorityIDPParcel,
		control:   domain.AdmissionControlOpen,
		record:    record,
	}
	value.handler = application.NewSubmitShipmentRequestHandler(
		value.sources,
		value.requests,
		value.ownership,
		&identityFactoryDouble{},
		fixedClock{at: time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)},
	)
	return value
}

func (value *fixture) identity(t *testing.T) domain.SourceIdentity {
	t.Helper()
	return sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1")
}

func (value *fixture) command(t *testing.T) application.SubmitShipmentRequestCommand {
	t.Helper()
	scope, err := domain.NewAdmissionScope(
		mustValue(t, domain.NewAdmissionScopeReference, "scope-ref-1"),
		mustValue(t, domain.NewAdmissionScopeDigest, "scope-1"),
	)
	if err != nil {
		t.Fatalf("new admission scope: %v", err)
	}
	return application.SubmitShipmentRequestCommand{
		Identity:          value.identity(t),
		PayloadDigest:     mustValue(t, domain.NewPayloadDigest, "digest-1"),
		OccurredAt:        time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
		ReceivedAt:        time.Date(2026, 8, 7, 10, 0, 1, 0, time.UTC),
		BatchID:           mustValue(t, domain.NewSubmissionBatchID, "batch-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		DeclaredParcelIDs: []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "parcel-1")},
		AdmissionScope:    scope,
		ExpectedRevision:  mustValue(t, domain.NewProductionOwnershipRevision, "rev-1"),
	}
}

type sourceRepositoryDouble struct {
	records       map[domain.SourceIdentity]domain.SourceSubmissionFingerprint
	observations  []domain.SourceSubmissionFingerprint
	record        func(string)
	preserveErr   error
	preserveCount int
}

func (double *sourceRepositoryDouble) FindPreserved(
	_ context.Context,
	identity domain.SourceIdentity,
) (domain.SourceSubmissionFingerprint, bool, error) {
	existing, found := double.records[identity]
	return existing, found, nil
}

func (double *sourceRepositoryDouble) Preserve(
	_ context.Context,
	submission domain.SourceSubmissionFingerprint,
) error {
	double.record("preserve-source")
	if double.preserveErr != nil {
		return double.preserveErr
	}
	double.preserveCount++
	double.records[submission.Identity()] = submission
	return nil
}

func (double *sourceRepositoryDouble) AppendObservation(
	_ context.Context,
	observed domain.SourceSubmissionFingerprint,
) error {
	double.record("append-observation")
	double.observations = append(double.observations, observed)
	return nil
}

func (double *sourceRepositoryDouble) stored(
	t *testing.T,
	identity domain.SourceIdentity,
) (domain.SourceSubmissionFingerprint, bool) {
	t.Helper()
	existing, found := double.records[identity]
	return existing, found
}

type shipmentRequestRepositoryDouble struct {
	records     map[domain.SourceIdentity]domain.ShipmentRequest
	record      func(string)
	insertCount int
}

func (double *shipmentRequestRepositoryDouble) FindBySourceIdentity(
	_ context.Context,
	identity domain.SourceIdentity,
) (domain.ShipmentRequest, bool, error) {
	existing, found := double.records[identity]
	return existing, found, nil
}

func (double *shipmentRequestRepositoryDouble) Insert(
	_ context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequest,
) error {
	double.record("insert-request")
	double.insertCount++
	double.records[identity] = request
	return nil
}

func (double *shipmentRequestRepositoryDouble) Save(
	_ context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequest,
) error {
	double.records[identity] = request
	return nil
}

func (double *shipmentRequestRepositoryDouble) stored(
	t *testing.T,
	identity domain.SourceIdentity,
) (domain.ShipmentRequest, bool) {
	t.Helper()
	existing, found := double.records[identity]
	return existing, found
}

type ownershipAuthorityDouble struct {
	t           *testing.T
	authority   domain.ProductionAuthorityKind
	control     domain.AdmissionControl
	record      func(string)
	decideCount int
}

func (double *ownershipAuthorityDouble) DecideProductionOwnership(
	_ context.Context,
	scope domain.AdmissionScope,
) (domain.ProductionOwnershipDecision, error) {
	double.t.Helper()
	double.record("decide-ownership")
	double.decideCount++

	spec := domain.ProductionOwnershipDecisionSpec{
		DecisionID:       mustValue(double.t, domain.NewProductionOwnershipDecisionID, "decision-1"),
		Scope:            scope,
		Authority:        double.authority,
		AdmissionControl: double.control,
		RuleVersion:      mustValue(double.t, domain.NewProductionOwnershipRuleVersion, "rule-1"),
		AsOf:             time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC),
		Validity:         validity(double.t),
		Revision:         mustValue(double.t, domain.NewProductionOwnershipRevision, "rev-1"),
		DecisionAt:       time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC),
	}
	switch double.authority {
	case domain.ProductionAuthorityOther:
		spec.OtherAuthorityRef = mustValue(double.t, domain.NewProductionAuthorityReference, "other-authority-1")
		spec.HandoffRef = mustValue(double.t, domain.NewHandoffConfirmationReference, "handoff-1")
	case domain.ProductionAuthorityUnresolved:
		spec.UnresolvedReason = domain.OwnershipUnresolvedAuthorityNotUnique
		spec.ContinuationRef = mustValue(double.t, domain.NewOwnershipContinuationReference, "continue-1")
	}
	if double.control == domain.AdmissionControlPaused {
		spec.SuspensionRef = mustValue(double.t, domain.NewOwnershipSuspensionReference, "suspend-1")
	}

	decision, err := domain.NewProductionOwnershipDecision(spec)
	if err != nil {
		double.t.Fatalf("new ownership decision: %v", err)
	}
	return decision, nil
}

type identityFactoryDouble struct {
	versions int
	tasks    int
}

func (double *identityFactoryDouble) NextSubmissionVersionID(_ context.Context) (domain.SubmissionVersionID, error) {
	double.versions++
	return domain.NewSubmissionVersionID("version-" + strconv.Itoa(double.versions))
}

func (double *identityFactoryDouble) NextAcceptanceDecisionTaskID(_ context.Context) (domain.AcceptanceDecisionTaskID, error) {
	double.tasks++
	return domain.NewAcceptanceDecisionTaskID("task-" + strconv.Itoa(double.tasks))
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

var (
	_ ports.SourceSubmissionRepository   = (*sourceRepositoryDouble)(nil)
	_ ports.ShipmentRequestRepository    = (*shipmentRequestRepositoryDouble)(nil)
	_ ports.ProductionOwnershipAuthority = (*ownershipAuthorityDouble)(nil)
	_ ports.SubmissionIdentityFactory    = (*identityFactoryDouble)(nil)
	_ ports.Clock                        = fixedClock{}
)
