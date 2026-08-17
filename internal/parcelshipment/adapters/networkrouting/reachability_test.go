package networkrouting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/networkrouting"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var (
	reachAsOfAt = time.Date(2026, 6, 2, 8, 0, 0, 0, time.UTC)
	judgedAt    = time.Date(2026, 6, 2, 9, 30, 0, 0, time.UTC)
)

func value[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

type eligibilityDouble struct {
	eligibility nrdomain.NetworkEligibility
	err         error
}

func (double *eligibilityDouble) AssessNetworkEligibility(
	_ context.Context,
	_ nrdomain.ReachabilityJudgmentKey,
) (nrdomain.NetworkEligibility, error) {
	if double.err != nil {
		return nrdomain.NetworkEligibility{}, double.err
	}
	return double.eligibility, nil
}

type evidenceDouble struct {
	areas         []nrdomain.ServiceAreaResolution
	revision      string
	err           error
	notConfigured bool
}

func (double *evidenceDouble) LoadNetworkEvidence(
	_ context.Context,
	_ nrdomain.ReachabilityJudgmentKey,
) (nrports.NetworkEvidence, bool, error) {
	if double.err != nil {
		return nrports.NetworkEvidence{}, false, double.err
	}
	if double.notConfigured {
		return nrports.NetworkEvidence{}, false, nil
	}
	revision := double.revision
	if revision == "" {
		revision = "net-view-rev-1"
	}
	return nrports.NetworkEvidence{
		ServiceAreas: double.areas,
		ViewRevision: mustRevision(revision),
	}, true, nil
}

func mustRevision(raw string) nrdomain.NetworkViewRevision {
	revision, err := nrdomain.NewNetworkViewRevision(raw)
	if err != nil {
		panic(err)
	}
	return revision
}

// memoryStore 是真持有状态的判断库替身：形成半边写进去、重校半边按关联读回来——两口
// 共用同一份中间状态正是 ADR-0027 的形状，各造各的假状态就测不出这条链。
type memoryStore struct {
	records map[string]nrports.ReachabilityJudgmentRecord
}

func newMemoryStore() *memoryStore {
	return &memoryStore{records: map[string]nrports.ReachabilityJudgmentRecord{}}
}

func (store *memoryStore) FindByCorrelation(
	_ context.Context,
	_ nrdomain.TenantID,
	correlation nrdomain.RequestCorrelationID,
) (nrports.ReachabilityJudgmentRecord, bool, error) {
	record, found := store.records[correlation.String()]
	return record, found, nil
}

func (store *memoryStore) Save(
	_ context.Context,
	correlation nrdomain.RequestCorrelationID,
	record nrports.ReachabilityJudgmentRecord,
) (nrports.ReachabilityJudgmentSaveOutcome, error) {
	if _, exists := store.records[correlation.String()]; exists {
		return nrports.ReachabilityJudgmentAlreadyRecorded, nil
	}
	store.records[correlation.String()] = record
	return nrports.ReachabilityJudgmentSaved, nil
}

type handoffDouble struct{ intents int }

func (double *handoffDouble) HandOffReachabilityJudgment(
	_ context.Context,
	_ nrports.ReachabilityJudgmentHandoffIntent,
) error {
	double.intents++
	return nil
}

type reachabilityFixture struct {
	adapter     *adapter.ReachabilityAdapter
	eligibility *eligibilityDouble
	evidence    *evidenceDouble
	store       *memoryStore
}

func coveringArea(t *testing.T, id string) nrdomain.ServiceAreaResolution {
	t.Helper()
	resolution, err := nrdomain.NewServiceAreaResolution(nrdomain.ServiceAreaResolutionSpec{
		Candidate:   value(t, nrdomain.NewCandidateID, id),
		Outcome:     nrdomain.AreaCoversDestination,
		AreaVersion: value(t, nrdomain.NewServiceAreaVersionReference, "AREA-V1"),
	})
	if err != nil {
		t.Fatalf("new service area resolution: %v", err)
	}
	return resolution
}

func newReachabilityFixture(t *testing.T) *reachabilityFixture {
	t.Helper()
	eligibility, err := nrdomain.NewNetworkEligibility(nrdomain.NetworkJudgmentRequired, nrdomain.EligibilityBasisReference{})
	if err != nil {
		t.Fatalf("new network eligibility: %v", err)
	}
	fixture := &reachabilityFixture{
		eligibility: &eligibilityDouble{eligibility: eligibility},
		evidence:    &evidenceDouble{areas: []nrdomain.ServiceAreaResolution{coveringArea(t, "candidate-1")}},
		store:       newMemoryStore(),
	}
	fixture.adapter = adapter.NewReachabilityAdapter(adapter.ReachabilityAdapterDeps{
		Assess: nrapplication.NewAssessParcelReachabilityHandler(
			fixture.eligibility, fixture.evidence, fixture.store, &handoffDouble{}, fixedClock{at: judgedAt}),
		Revalidate: nrapplication.NewValidateReachabilityJudgmentHandler(fixture.evidence, fixture.store),
		Purpose:    value(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"),
	})
	return fixture
}

func (fixture *reachabilityFixture) request(t *testing.T) psports.ReachabilityRequest {
	t.Helper()
	identity, err := psdomain.NewSourceIdentity(
		value(t, psdomain.NewTenantID, "tenant-1"),
		value(t, psdomain.NewCustomerAccountID, "customer-1"),
		value(t, psdomain.NewSource, "source-a"),
		value(t, psdomain.NewSourceRequestKey, "key-1"),
	)
	if err != nil {
		t.Fatalf("new source identity: %v", err)
	}
	echoed, err := psdomain.NewEchoedAsOfPolicy(
		psdomain.ReachabilityJudgmentKind,
		value(t, psdomain.NewAsOfSemanticsReference, "CURRENT_SUBMISSION_RECEIVED_AT"),
		value(t, psdomain.NewAsOfPolicyVersion, "asof-strategy-v1"),
	)
	if err != nil {
		t.Fatalf("new echoed policy: %v", err)
	}
	asOf, err := psdomain.NewJudgmentAsOf(reachAsOfAt, echoed)
	if err != nil {
		t.Fatalf("new judgment as-of: %v", err)
	}
	return psports.ReachabilityRequest{
		Identity:          identity,
		ShipmentRequestID: value(t, psdomain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: value(t, psdomain.NewSubmissionVersionID, "version-1"),
		DeclaredParcelID:  value(t, psdomain.NewDeclaredParcelID, "parcel-1"),
		AsOf:              asOf,
	}
}

func (fixture *reachabilityFixture) revalidationQuery(t *testing.T) psports.ReachabilityRevalidationQuery {
	t.Helper()
	request := fixture.request(t)
	return psports.ReachabilityRevalidationQuery{
		Identity:          request.Identity,
		ShipmentRequestID: request.ShipmentRequestID,
		SubmissionVersion: request.SubmissionVersion,
		DeclaredParcelID:  request.DeclaredParcelID,
		AsOf:              request.AsOf,
	}
}

// Covers: UC-NR-002 与 UC-PS-001 的可达性衔接经第一个真实翻译打通——三值结论逐名译回、
// 判断标识取请求关联（提供方不签发判断号，关联即回指入口，ADR-0027）、重放交回同一份。
func TestAFormedFindingBecomesAReachabilityJudgment(t *testing.T) {
	fixture := newReachabilityFixture(t)

	assessment, err := fixture.adapter.AssessParcelReachability(context.Background(), fixture.request(t))
	if err != nil {
		t.Fatalf("assess parcel reachability: %v", err)
	}

	if assessment.Outcome != psports.ReachabilityAssessed {
		t.Fatalf("outcome = %q reason = %q, want ASSESSED", assessment.Outcome, assessment.Reason)
	}
	if assessment.Judgment.Value() != psdomain.ReachabilityReachable {
		t.Fatalf("value = %q, want REACHABLE", assessment.Judgment.Value())
	}
	if assessment.Judgment.JudgmentID().String() != "request-1/version-1/parcel-1" {
		t.Fatalf("judgment ID = %q, want the request correlation", assessment.Judgment.JudgmentID())
	}
	if !assessment.Judgment.AsOf().At().Equal(reachAsOfAt) {
		t.Fatalf("as-of = %s, want the declared judgment time", assessment.Judgment.AsOf().At())
	}

	replay, err := fixture.adapter.AssessParcelReachability(context.Background(), fixture.request(t))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome != psports.ReachabilityAssessed ||
		replay.Judgment.JudgmentID() != assessment.Judgment.JudgmentID() {
		t.Fatal("重放没有交回同一份判断")
	}
}

// Covers: `AT-PS-037`「被有效新版本或限制推翻 → 原结果不再用于接受」的适配器一环——同一
// 关联重校：视图没换代确认，换代判`已换代`；查无此判断按未找回译回。
func TestARevalidationTracksTheEvidenceViewRevision(t *testing.T) {
	fixture := newReachabilityFixture(t)
	if _, err := fixture.adapter.AssessParcelReachability(context.Background(), fixture.request(t)); err != nil {
		t.Fatalf("assess: %v", err)
	}

	still, err := fixture.adapter.RevalidateReachabilityJudgment(context.Background(), fixture.revalidationQuery(t))
	if err != nil {
		t.Fatalf("revalidate: %v", err)
	}
	if still.Outcome != psports.ReachabilityJudgmentStillCurrent {
		t.Fatalf("outcome = %q, want STILL_CURRENT", still.Outcome)
	}

	fixture.evidence.revision = "net-view-rev-2"
	superseded, err := fixture.adapter.RevalidateReachabilityJudgment(context.Background(), fixture.revalidationQuery(t))
	if err != nil {
		t.Fatalf("revalidate after revision change: %v", err)
	}
	if superseded.Outcome != psports.ReachabilityJudgmentSuperseded {
		t.Fatalf("outcome = %q, want SUPERSEDED", superseded.Outcome)
	}

	t.Run("a judgment never formed is not found", func(t *testing.T) {
		fresh := newReachabilityFixture(t)
		revalidation, err := fresh.adapter.RevalidateReachabilityJudgment(context.Background(), fresh.revalidationQuery(t))
		if err != nil {
			t.Fatalf("revalidate: %v", err)
		}
		if revalidation.Outcome != psports.ReachabilityRevalidationJudgmentNotFound {
			t.Fatalf("outcome = %q, want JUDGMENT_NOT_FOUND", revalidation.Outcome)
		}
	})
}

// Covers: ADR-0025「翻译必须是全函数」在可达性一侧——不适用带依据、未形成带 NR- 前缀
// 原因、依赖失败经重校译成`无法判定`。
func TestEveryProviderAnswerLandsOnItsOwnConsumerValue(t *testing.T) {
	t.Run("not applicable carries the eligibility basis", func(t *testing.T) {
		fixture := newReachabilityFixture(t)
		eligibility, err := nrdomain.NewNetworkEligibility(
			nrdomain.NetworkJudgmentNotRequired,
			value(t, nrdomain.NewEligibilityBasisReference, "PC-NO-NETWORK-BASIS-1"),
		)
		if err != nil {
			t.Fatalf("new network eligibility: %v", err)
		}
		fixture.eligibility.eligibility = eligibility

		assessment, err := fixture.adapter.AssessParcelReachability(context.Background(), fixture.request(t))
		if err != nil {
			t.Fatalf("assess: %v", err)
		}
		if assessment.Outcome != psports.ReachabilityAssessed ||
			assessment.Judgment.Value() != psdomain.ReachabilityNotApplicable {
			t.Fatalf("outcome = %q value = %q, want ASSESSED/NOT_APPLICABLE", assessment.Outcome, assessment.Judgment.Value())
		}
		if assessment.Judgment.Basis().String() != "PC-NO-NETWORK-BASIS-1" {
			t.Fatalf("basis = %q, want the eligibility basis carried through", assessment.Judgment.Basis())
		}
	})

	t.Run("not formed carries the provider reason", func(t *testing.T) {
		fixture := newReachabilityFixture(t)
		fixture.evidence.err = errors.New("evidence down")

		assessment, err := fixture.adapter.AssessParcelReachability(context.Background(), fixture.request(t))
		if err != nil {
			t.Fatalf("assess: %v", err)
		}
		if assessment.Outcome != psports.ReachabilityNotFormed {
			t.Fatalf("outcome = %q, want NOT_FORMED", assessment.Outcome)
		}
		if assessment.Reason.String() != "NR-NETWORK_EVIDENCE_UNAVAILABLE" {
			t.Fatalf("reason = %q, want NR-NETWORK_EVIDENCE_UNAVAILABLE", assessment.Reason)
		}
	})

	t.Run("an unreadable authority keeps the revalidation undetermined", func(t *testing.T) {
		fixture := newReachabilityFixture(t)
		if _, err := fixture.adapter.AssessParcelReachability(context.Background(), fixture.request(t)); err != nil {
			t.Fatalf("assess: %v", err)
		}
		fixture.evidence.err = errors.New("evidence down")

		revalidation, err := fixture.adapter.RevalidateReachabilityJudgment(context.Background(), fixture.revalidationQuery(t))
		if err != nil {
			t.Fatalf("revalidate: %v", err)
		}
		if revalidation.Outcome != psports.ReachabilityRevalidationUndetermined {
			t.Fatalf("outcome = %q, want UNDETERMINED", revalidation.Outcome)
		}
		if revalidation.Reason.String() != "NR-NETWORK_EVIDENCE_UNAVAILABLE" {
			t.Fatalf("reason = %q", revalidation.Reason)
		}
	})
}

// Covers: 实例半边纪律——服务目的由产品定义，未配置时形成停`未形成`、重校停`无法判定`，
// 且都不问提供方（问一次就读了这个客户的网络资格）。
func TestAnUnconfiguredPurposeStopsWithoutAskingTheProvider(t *testing.T) {
	fixture := newReachabilityFixture(t)
	unconfigured := adapter.NewReachabilityAdapter(adapter.ReachabilityAdapterDeps{
		Assess: nrapplication.NewAssessParcelReachabilityHandler(
			fixture.eligibility, fixture.evidence, fixture.store, &handoffDouble{}, fixedClock{at: judgedAt}),
		Revalidate: nrapplication.NewValidateReachabilityJudgmentHandler(fixture.evidence, fixture.store),
	})

	assessment, err := unconfigured.AssessParcelReachability(context.Background(), fixture.request(t))
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if assessment.Outcome != psports.ReachabilityNotFormed ||
		assessment.Reason.String() != "REACHABILITY_PURPOSE_NOT_CONFIGURED" {
		t.Fatalf("assessment = %q/%q, want NOT_FORMED/REACHABILITY_PURPOSE_NOT_CONFIGURED",
			assessment.Outcome, assessment.Reason)
	}

	revalidation, err := unconfigured.RevalidateReachabilityJudgment(context.Background(), fixture.revalidationQuery(t))
	if err != nil {
		t.Fatalf("revalidate: %v", err)
	}
	if revalidation.Outcome != psports.ReachabilityRevalidationUndetermined ||
		revalidation.Reason.String() != "REACHABILITY_PURPOSE_NOT_CONFIGURED" {
		t.Fatalf("revalidation = %q/%q", revalidation.Outcome, revalidation.Reason)
	}
	if len(fixture.store.records) != 0 {
		t.Fatal("目的未配置却形成了判断")
	}
}
