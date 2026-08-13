package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/application"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
)

var incidentAt = time.Date(2026, 8, 13, 22, 0, 0, 0, time.UTC)

type suspensionStoreDouble struct {
	byID map[domain.SuspensionID]domain.SuspensionDecision
}

func (double *suspensionStoreDouble) FindByID(
	_ context.Context,
	id domain.SuspensionID,
) (domain.SuspensionDecision, bool, error) {
	decision, found := double.byID[id]
	return decision, found, nil
}

func (double *suspensionStoreDouble) Save(
	_ context.Context,
	decision domain.SuspensionDecision,
) (ports.GovernanceSaveOutcome, error) {
	if _, exists := double.byID[decision.ID()]; exists {
		return ports.GovernanceAlreadyRecorded, nil
	}
	double.byID[decision.ID()] = decision
	return ports.GovernanceSaved, nil
}

type resumptionStoreDouble struct {
	bySuspension map[domain.SuspensionID]domain.ResumptionDecision
	saved        int
}

func (double *resumptionStoreDouble) FindBySuspension(
	_ context.Context,
	id domain.SuspensionID,
) (domain.ResumptionDecision, bool, error) {
	decision, found := double.bySuspension[id]
	return decision, found, nil
}

func (double *resumptionStoreDouble) Save(
	_ context.Context,
	decision domain.ResumptionDecision,
) (ports.GovernanceSaveOutcome, error) {
	if _, exists := double.bySuspension[decision.Suspension()]; exists {
		return ports.GovernanceAlreadyRecorded, nil
	}
	double.bySuspension[decision.Suspension()] = decision
	double.saved++
	return ports.GovernanceSaved, nil
}

type takeoverStoreDouble struct {
	byInterval map[domain.AuthorityInterval]domain.TakeoverRecord
}

func (double *takeoverStoreDouble) FindByInterval(
	_ context.Context,
	interval domain.AuthorityInterval,
) (domain.TakeoverRecord, bool, error) {
	record, found := double.byInterval[interval]
	return record, found, nil
}

func (double *takeoverStoreDouble) Save(
	_ context.Context,
	record domain.TakeoverRecord,
) (ports.GovernanceSaveOutcome, error) {
	if _, exists := double.byInterval[record.Interval()]; exists {
		return ports.GovernanceAlreadyRecorded, nil
	}
	double.byInterval[record.Interval()] = record
	return ports.GovernanceSaved, nil
}

type governanceDownstreamDouble struct {
	intents []ports.GovernanceHandoffIntent
}

func (double *governanceDownstreamDouble) HandOffGovernance(
	_ context.Context,
	intent ports.GovernanceHandoffIntent,
) error {
	double.intents = append(double.intents, intent)
	return nil
}

type incidentFixture struct {
	handler     *application.GovernIncidentHandler
	suspensions *suspensionStoreDouble
	resumptions *resumptionStoreDouble
	takeovers   *takeoverStoreDouble
	intervals   *intervalStoreDouble
}

func newIncidentFixture(t *testing.T) *incidentFixture {
	t.Helper()
	fixture := &incidentFixture{
		suspensions: &suspensionStoreDouble{byID: map[domain.SuspensionID]domain.SuspensionDecision{}},
		resumptions: &resumptionStoreDouble{bySuspension: map[domain.SuspensionID]domain.ResumptionDecision{}},
		takeovers:   &takeoverStoreDouble{byInterval: map[domain.AuthorityInterval]domain.TakeoverRecord{}},
		intervals:   &intervalStoreDouble{},
	}
	fixture.handler = application.NewGovernIncidentHandler(application.GovernIncidentDeps{
		Suspensions: fixture.suspensions,
		Resumptions: fixture.resumptions,
		Takeovers:   fixture.takeovers,
		Intervals:   fixture.intervals,
		Downstream:  &governanceDownstreamDouble{},
		Clock:       fixedClock{at: incidentAt},
	})
	return fixture
}

func suspensionSpec(t *testing.T) domain.SuspensionDecisionSpec {
	t.Helper()
	return domain.SuspensionDecisionSpec{
		ID:            mustValue(t, domain.NewSuspensionID, "suspension-1"),
		TriggerSource: "STAGE-REVIEW/no-go-7",
		Basis:         "acceptance defect rate above threshold",
		Evidence:      "evidence-pack/incident-11",
		Scope:         mustValue(t, domain.NewScopeVersionReference, "pilot-scope/v1"),
		ExecutedBy:    "pilot-business-owner",
		OccurredAt:    incidentAt.Add(-2 * time.Hour),
		EffectiveAt:   incidentAt.Add(-time.Hour),
		InTransitNote: "in-transit objects continue to conclusion under current authority",
	}
}

func inventory(t *testing.T) domain.InTransitInventory {
	t.Helper()
	taken, err := domain.TakeInventory([]domain.InventoryEntry{{
		ObjectIdentity:   "parcel-1",
		CurrentFacts:     "accepted, in transit at node-origin",
		CurrentAuthority: "idp-parcel",
		ResponsibleParty: "pilot-operations",
		NextAction:       "continue to conclusion",
		ReviewBy:         incidentAt.Add(24 * time.Hour),
	}}, incidentAt.Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("take inventory: %v", err)
	}
	return taken
}

// Covers: PN-08 治理记录经编排入册——暂停重放返原不可覆盖；恢复的身份挂在它要解除
// 的暂停上（引用不存在的暂停未受理级答复）、一个暂停至多一次恢复（重复恢复按已恢复
// 作答，再暂停是新决定不是往返）。
func TestSuspensionsRecordOnceAndResumptionsBindTheirSuspension(t *testing.T) {
	fixture := newIncidentFixture(t)

	suspended, err := fixture.handler.Suspend(context.Background(), suspensionSpec(t))
	if err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if suspended.Outcome() != application.SuspensionRecorded {
		t.Fatalf("outcome = %q", suspended.Outcome())
	}

	replayed, err := fixture.handler.Suspend(context.Background(), suspensionSpec(t))
	if err != nil {
		t.Fatalf("suspend replay: %v", err)
	}
	if replayed.Outcome() != application.SuspensionExisting {
		t.Fatalf("replay = %q", replayed.Outcome())
	}

	orphan := domain.ResumptionDecisionSpec{
		Suspension:       mustValue(t, domain.NewSuspensionID, "suspension-9"),
		ReleaseEvidence:  "evidence-pack/fixed",
		ConsistencyCheck: "consistency-check/pass",
		Inventory:        inventory(t),
		DecidedBy:        "pilot-business-owner",
		DecidedAt:        incidentAt,
		EffectiveAt:      incidentAt.Add(time.Hour),
	}
	missing, err := fixture.handler.Resume(context.Background(), orphan)
	if err != nil {
		t.Fatalf("resume missing: %v", err)
	}
	if missing.Outcome() != application.SuspensionNotFound {
		t.Fatalf("missing = %q; 恢复的身份挂在暂停上", missing.Outcome())
	}

	resume := orphan
	resume.Suspension = mustValue(t, domain.NewSuspensionID, "suspension-1")
	resumed, err := fixture.handler.Resume(context.Background(), resume)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if resumed.Outcome() != application.ResumptionRecorded {
		t.Fatalf("outcome = %q", resumed.Outcome())
	}

	again, err := fixture.handler.Resume(context.Background(), resume)
	if err != nil {
		t.Fatalf("resume again: %v", err)
	}
	if again.Outcome() != application.ResumptionExisting || fixture.resumptions.saved != 1 {
		t.Fatalf("again = %q saved = %d", again.Outcome(), fixture.resumptions.saved)
	}
}

// Covers: PN-08 失败场景「原权威确实无法继续时的对象级接管」经编排——新权威区间撞上
// 仍开着的既有区间即阻断带全部冲突对（先关原区间再接管——权威重叠正是本模块点名要防
// 的双写）；原区间关闭后接管入册且新区间追加；同区间身份重放返原。
func TestTakeoversAreBlockedByOpenIntervalsAndRecordOnce(t *testing.T) {
	fixture := newIncidentFixture(t)
	fixture.intervals.intervals = []domain.AuthorityInterval{{
		ObjectScope: "pilot-scope/v1",
		Capability:  "SHIPMENT_ACCEPTANCE",
		FactKind:    "ACCEPTANCE_DECISION",
		Authority:   "idp-parcel",
		From:        incidentAt.Add(-48 * time.Hour),
	}}

	spec := domain.TakeoverRecordSpec{
		StopEvidence: "evidence-pack/authority-stopped",
		Interval: domain.AuthorityInterval{
			ObjectScope: "pilot-scope/v1",
			Capability:  "SHIPMENT_ACCEPTANCE",
			FactKind:    "ACCEPTANCE_DECISION",
			Authority:   "legacy-system",
			From:        incidentAt,
		},
		AcceptedFacts:    "facts accepted as-is from prior authority",
		PendingExternals: "two customs declarations awaiting external results",
		ActualControl:    "objects physically at node-origin",
		Responsibilities: "legacy operations team",
		NextAction:       "resume manual processing",
		Inventory:        inventory(t),
		EffectiveAt:      incidentAt,
	}
	blocked, err := fixture.handler.TakeOver(context.Background(), spec)
	if err != nil {
		t.Fatalf("blocked takeover: %v", err)
	}
	if blocked.Outcome() != application.TakeoverConflictBlocked || len(blocked.Conflicts()) != 1 {
		t.Fatalf("outcome = %q conflicts = %d", blocked.Outcome(), len(blocked.Conflicts()))
	}

	fixture.intervals.intervals[0].To = incidentAt.Add(-time.Minute)
	recorded, err := fixture.handler.TakeOver(context.Background(), spec)
	if err != nil {
		t.Fatalf("takeover: %v", err)
	}
	if recorded.Outcome() != application.TakeoverRecorded {
		t.Fatalf("outcome = %q", recorded.Outcome())
	}
	if len(fixture.intervals.intervals) != 2 {
		t.Fatalf("intervals = %d; 新权威区间没有追加", len(fixture.intervals.intervals))
	}

	replayed, err := fixture.handler.TakeOver(context.Background(), spec)
	if err != nil {
		t.Fatalf("takeover replay: %v", err)
	}
	if replayed.Outcome() != application.TakeoverExisting {
		t.Fatalf("replay = %q", replayed.Outcome())
	}
	if len(fixture.intervals.intervals) != 2 {
		t.Fatalf("intervals = %d; 重放又追加了区间", len(fixture.intervals.intervals))
	}
}
