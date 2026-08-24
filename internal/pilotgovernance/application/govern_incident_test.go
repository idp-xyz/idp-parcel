package application_test

import (
	"context"
	"errors"
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
	err     error
	intents []ports.GovernanceHandoffIntent
}

func (double *governanceDownstreamDouble) HandOffGovernance(
	_ context.Context,
	intent ports.GovernanceHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

func (double *governanceDownstreamDouble) count(
	pick func(ports.GovernanceHandoffIntent) bool,
) int {
	total := 0
	for _, intent := range double.intents {
		if pick(intent) {
			total++
		}
	}
	return total
}

type incidentFixture struct {
	handler     *application.GovernIncidentHandler
	suspensions *suspensionStoreDouble
	resumptions *resumptionStoreDouble
	takeovers   *takeoverStoreDouble
	intervals   *intervalStoreDouble
	downstream  *governanceDownstreamDouble
}

func newIncidentFixture(t *testing.T) *incidentFixture {
	t.Helper()
	fixture := &incidentFixture{
		suspensions: &suspensionStoreDouble{byID: map[domain.SuspensionID]domain.SuspensionDecision{}},
		resumptions: &resumptionStoreDouble{bySuspension: map[domain.SuspensionID]domain.ResumptionDecision{}},
		takeovers:   &takeoverStoreDouble{byInterval: map[domain.AuthorityInterval]domain.TakeoverRecord{}},
		intervals:   &intervalStoreDouble{},
		downstream:  &governanceDownstreamDouble{},
	}
	fixture.handler = application.NewGovernIncidentHandler(application.GovernIncidentDeps{
		Suspensions: fixture.suspensions,
		Resumptions: fixture.resumptions,
		Takeovers:   fixture.takeovers,
		Intervals:   fixture.intervals,
		Downstream:  fixture.downstream,
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
// 作答，再暂停是新决定不是往返）。点名验收矩阵 GOV-06「紧急暂停与恢复……恢复由试点
// 业务责任角色明确决定」的机制半边（恢复必须显式挂在暂停上，不自动恢复）。
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

// Covers: PN-08 失败场景「原权威确实无法继续时的对象级接管」经编排，即验收矩阵
// GOV-07「回退与对象级接管……原权威写入先停止，不双写」的机制半边——新权威区间撞上
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

// Covers: 评审发现①的修法（镜像 06084b6）——接管入册而区间追加失败时接管不翻但交回
// 续办引用；重放路（TakeoverExisting）凭同一命令补追加同一份区间，补上后引用清空；
// 已在册的区间不重复追加。
func TestAFailedTakeoverIntervalAppendLeavesAContinuationAndReplayHeals(t *testing.T) {
	fixture := newIncidentFixture(t)
	fixture.intervals.appendErr = errors.New("interval store unreachable")

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

	first, err := fixture.handler.TakeOver(context.Background(), spec)
	if err != nil {
		t.Fatalf("first takeover: %v", err)
	}
	if first.Outcome() != application.TakeoverRecorded {
		t.Fatalf("outcome = %q; 追加失败不得翻接管", first.Outcome())
	}
	if first.HandoffReference() == "" {
		t.Fatal("追加失败没有留续办引用——接管与区间分岔无处可知")
	}

	fixture.intervals.appendErr = nil
	replay, err := fixture.handler.TakeOver(context.Background(), spec)
	if err != nil {
		t.Fatalf("replay takeover: %v", err)
	}
	if replay.Outcome() != application.TakeoverExisting {
		t.Fatalf("replay = %q", replay.Outcome())
	}
	if replay.HandoffReference() != "" {
		t.Fatal("补追加成功后续办引用还挂着")
	}
	if len(fixture.intervals.intervals) != 1 {
		t.Fatalf("intervals = %d; 重放没有补上区间", len(fixture.intervals.intervals))
	}

	again, err := fixture.handler.TakeOver(context.Background(), spec)
	if err != nil {
		t.Fatalf("second replay: %v", err)
	}
	if again.HandoffReference() != "" || len(fixture.intervals.intervals) != 1 {
		t.Fatalf("intervals = %d; 已在册的区间被重复追加", len(fixture.intervals.intervals))
	}
}

func takeoverSpec(t *testing.T) domain.TakeoverRecordSpec {
	t.Helper()
	return domain.TakeoverRecordSpec{
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
}

// Covers: 票 pg-takeover-replay-handoff/01——首次接管在区间追加处中断时 handOff 从未
// 被尝试；重放走 TakeoverExisting 分支必须在补追加成功后补尝试 handOff，否则这次接管
// 的信封永不入队。断点纪律不变：追加未修好前不越过断点发信封。「入队恰一次」的幂等
// 半边由端口的 EnqueueOnce 承担（ports.GovernanceHandoff 注），本测试在端口缝上断言
// 发出的份数与身份。
func TestTakeoverReplayAfterAppendFailureHandsOffTheEnvelope(t *testing.T) {
	fixture := newIncidentFixture(t)
	fixture.intervals.appendErr = errors.New("interval store unreachable")

	first, err := fixture.handler.TakeOver(context.Background(), takeoverSpec(t))
	if err != nil {
		t.Fatalf("first takeover: %v", err)
	}
	if first.Outcome() != application.TakeoverRecorded || first.HandoffReference() == "" {
		t.Fatalf("outcome = %q ref = %q", first.Outcome(), first.HandoffReference())
	}
	if got := len(fixture.downstream.intents); got != 0 {
		t.Fatalf("handoffs = %d; 区间追加失败即断点，不得越过断点发信封", got)
	}

	fixture.intervals.appendErr = nil
	replay, err := fixture.handler.TakeOver(context.Background(), takeoverSpec(t))
	if err != nil {
		t.Fatalf("replay takeover: %v", err)
	}
	if replay.Outcome() != application.TakeoverExisting || replay.HandoffReference() != "" {
		t.Fatalf("replay = %q ref = %q", replay.Outcome(), replay.HandoffReference())
	}
	if got := len(fixture.downstream.intents); got != 1 {
		t.Fatalf("handoffs = %d; 重放补追加成功后必须补尝试 handOff——否则接管信封永不入队", got)
	}
	intent := fixture.downstream.intents[0]
	if intent.Takeover == nil || intent.Takeover.Interval() != takeoverSpec(t).Interval {
		t.Fatal("补发的不是这次接管的信封")
	}
}

// Covers: 首次全程成功后的重放重发同一份信封——同区间身份认领同一个信封 ID，
// EnqueueOnce 答已入队、不重复入队（ADR-0043「重放重发同一份」）。重发与首发身份
// 相同是收敛成立的前提，身份漂移会让幂等失认、重复入队。
func TestTakeoverReplayAfterFullSuccessResendsTheSameEnvelope(t *testing.T) {
	fixture := newIncidentFixture(t)

	first, err := fixture.handler.TakeOver(context.Background(), takeoverSpec(t))
	if err != nil {
		t.Fatalf("first takeover: %v", err)
	}
	if first.Outcome() != application.TakeoverRecorded || first.HandoffReference() != "" {
		t.Fatalf("outcome = %q ref = %q", first.Outcome(), first.HandoffReference())
	}

	replay, err := fixture.handler.TakeOver(context.Background(), takeoverSpec(t))
	if err != nil {
		t.Fatalf("replay takeover: %v", err)
	}
	if replay.Outcome() != application.TakeoverExisting || replay.HandoffReference() != "" {
		t.Fatalf("replay = %q ref = %q", replay.Outcome(), replay.HandoffReference())
	}
	if got := len(fixture.downstream.intents); got != 2 {
		t.Fatalf("handoffs = %d; 重放要重发同一份，幂等由 EnqueueOnce 答已入队", got)
	}
	sent, resent := fixture.downstream.intents[0], fixture.downstream.intents[1]
	if sent.Takeover == nil || resent.Takeover == nil ||
		sent.Takeover.Interval() != resent.Takeover.Interval() {
		t.Fatal("重发的信封身份与首发不同——EnqueueOnce 认不出同一份")
	}
	if len(fixture.intervals.intervals) != 1 {
		t.Fatalf("intervals = %d; 重放又追加了区间", len(fixture.intervals.intervals))
	}
}

// Covers: 票 01 顺带核一格的结论——Suspension/Resumption 的重放分支与接管同型漏
// handOff，同型同修：首次 handOff 失败留续办引用后，重放答已在册时必须补尝试
// handOff（EnqueueOnce 幂等，已入队者答已入队），否则信封无人再发。
func TestSuspensionAndResumptionReplaysRetryTheHandoff(t *testing.T) {
	fixture := newIncidentFixture(t)
	fixture.downstream.err = errors.New("outbox unreachable")

	first, err := fixture.handler.Suspend(context.Background(), suspensionSpec(t))
	if err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if first.Outcome() != application.SuspensionRecorded || first.HandoffReference() != "CONT-GOV-SUSPENSION" {
		t.Fatalf("outcome = %q ref = %q", first.Outcome(), first.HandoffReference())
	}

	fixture.downstream.err = nil
	replay, err := fixture.handler.Suspend(context.Background(), suspensionSpec(t))
	if err != nil {
		t.Fatalf("suspend replay: %v", err)
	}
	if replay.Outcome() != application.SuspensionExisting || replay.HandoffReference() != "" {
		t.Fatalf("replay = %q ref = %q", replay.Outcome(), replay.HandoffReference())
	}
	suspensions := fixture.downstream.count(func(intent ports.GovernanceHandoffIntent) bool {
		return intent.Suspension != nil
	})
	if suspensions != 1 {
		t.Fatalf("suspension handoffs = %d; 重放没有补发暂停信封", suspensions)
	}

	resume := domain.ResumptionDecisionSpec{
		Suspension:       mustValue(t, domain.NewSuspensionID, "suspension-1"),
		ReleaseEvidence:  "evidence-pack/fixed",
		ConsistencyCheck: "consistency-check/pass",
		Inventory:        inventory(t),
		DecidedBy:        "pilot-business-owner",
		DecidedAt:        incidentAt,
		EffectiveAt:      incidentAt.Add(time.Hour),
	}
	fixture.downstream.err = errors.New("outbox unreachable")
	firstResume, err := fixture.handler.Resume(context.Background(), resume)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if firstResume.Outcome() != application.ResumptionRecorded || firstResume.HandoffReference() != "CONT-GOV-RESUMPTION" {
		t.Fatalf("outcome = %q ref = %q", firstResume.Outcome(), firstResume.HandoffReference())
	}

	fixture.downstream.err = nil
	resumeReplay, err := fixture.handler.Resume(context.Background(), resume)
	if err != nil {
		t.Fatalf("resume replay: %v", err)
	}
	if resumeReplay.Outcome() != application.ResumptionExisting || resumeReplay.HandoffReference() != "" {
		t.Fatalf("replay = %q ref = %q", resumeReplay.Outcome(), resumeReplay.HandoffReference())
	}
	resumptions := fixture.downstream.count(func(intent ports.GovernanceHandoffIntent) bool {
		return intent.Resumption != nil
	})
	if resumptions != 1 {
		t.Fatalf("resumption handoffs = %d; 重放没有补发恢复信封", resumptions)
	}
}
