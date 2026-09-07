package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证 ADR-0112 从来源更正到段上替代版本这一整条路真的走通：真揽收 / 交接登记册 →
// 真编排的 Correct → 真段登记册的 Supersede，再从真段登记册读回链。fulfillment_segment_supersession_test.go
// 证的是登记册那一口，rederive_participation_on_correction_test.go 证的是编排对着替身的行为；两份合起来仍
// 证不了「真编排接真登记册时，链尾按回指派生、原行一字不动」——那正是 lc/24 教训里缺的那一格。两种来源各一正
// 一反，外加段已关闭仍重派生。夹具全为合成登记（S 级）。

var (
	rederivationEnteredAt   = segmentEnteredAtFixture
	rederivationCorrectedAt = segmentEnteredAtFixture.Add(2 * time.Hour)
)

type rederivationRegistries struct {
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
	segments   *adapter.FulfillmentSegments
	ender      *application.EndFulfillmentParticipationHandler
	pickups    *application.RegisterOffsitePickupHandler
	handovers  *application.RegisterTransportHandoverHandler
	tenant     domain.TenantID
}

func newRederivationRegistries(t *testing.T) rederivationRegistries {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	segments, err := adapter.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatalf("构造段登记册：%v", err)
	}
	judgments, err := adapter.NewActualCarrierJudgments(db)
	if err != nil {
		t.Fatalf("构造判断登记册：%v", err)
	}
	pickupRegistry, err := adapter.NewOffsitePickupRegistrations(db)
	if err != nil {
		t.Fatalf("构造揽收登记册：%v", err)
	}
	handoverRegistry, err := adapter.NewTransportHandovers(db)
	if err != nil {
		t.Fatalf("构造交接登记册：%v", err)
	}
	deliveries, err := adapter.NewEffectiveDeliveries(db)
	if err != nil {
		t.Fatalf("构造交付登记册：%v", err)
	}
	versions, err := adapter.NewResultVersions(db)
	if err != nil {
		t.Fatalf("构造版本签发器：%v", err)
	}
	clock := credentialClock{at: rederivationEnteredAt}
	ender := application.NewEndFulfillmentParticipationHandler(application.EndFulfillmentParticipationDeps{
		Segments: segments, Judgments: judgments, Handovers: handoverRegistry, Deliveries: deliveries, Clock: clock,
	})
	return rederivationRegistries{
		transactor: db.Transactor(),
		pool:       pool,
		segments:   segments,
		ender:      ender,
		pickups: application.NewRegisterOffsitePickupHandler(application.RegisterOffsitePickupDeps{
			Pickups: pickupRegistry, Segments: segments, Judgments: judgments,
			Versions: versions, Downstream: noPickupHandoff{}, Clock: clock,
		}),
		handovers: application.NewRegisterTransportHandoverHandler(application.RegisterTransportHandoverDeps{
			Handovers: handoverRegistry, Segments: segments, Judgments: judgments,
			Downstream: noHandoverHandoff{}, Clock: clock, ParticipationEnds: ender,
		}),
		tenant: segmentRef(t, domain.NewTenantID, "tenant-1"),
	}
}

type noHandoverHandoff struct{}

func (noHandoverHandoff) HandOffTransportHandover(context.Context, ports.TransportHandoverRegistrationIntent) error {
	return nil
}

func (registries rederivationRegistries) registerPickup(
	t *testing.T, ctx context.Context, object, segment string,
) application.RegisterOffsitePickupResult {
	t.Helper()
	var result application.RegisterOffsitePickupResult
	mustWithinSegmentTransaction(t, registries.transactor, ctx, func(txCtx context.Context) error {
		var err error
		result, err = registries.pickups.Register(txCtx, application.RegisterOffsitePickupCommand{
			TenantID:   registries.tenant,
			Object:     object,
			Task:       "task-1",
			Attempt:    "attempt-1",
			Place:      "place-1",
			Control:    "TRANSPORT-CONTROL/" + object,
			ExecutedBy: "executor-1",
			OccurredAt: rederivationEnteredAt,
			Segment:    segment,
		})
		return err
	})
	if result.Outcome() != application.PickupRegistered || result.SegmentContinuationReference() != "" {
		t.Fatalf("揽收首登：outcome=%s debt=%q", result.Outcome(), result.SegmentContinuationReference())
	}
	return result
}

func (registries rederivationRegistries) correctPickup(
	t *testing.T, ctx context.Context, object, predecessor string, occurredAt, correctedAt time.Time,
) application.RegisterOffsitePickupResult {
	t.Helper()
	var result application.RegisterOffsitePickupResult
	mustWithinSegmentTransaction(t, registries.transactor, ctx, func(txCtx context.Context) error {
		var err error
		result, err = registries.pickups.Correct(txCtx, application.CorrectOffsitePickupCommand{
			TenantID:           registries.tenant,
			Object:             object,
			Attempt:            "attempt-1",
			PredecessorVersion: predecessor,
			Place:              "place-2",
			Control:            "TRANSPORT-CONTROL/" + object + "/corrected",
			ExecutedBy:         "executor-2",
			OccurredAt:         occurredAt,
			CorrectedAt:        correctedAt,
		})
		return err
	})
	if result.Outcome() != application.PickupCorrected {
		t.Fatalf("揽收更正：outcome=%s reason=%s", result.Outcome(), result.UndecidedReason())
	}
	return result
}

func (registries rederivationRegistries) registerHandover(
	t *testing.T, ctx context.Context, object, segment string,
) application.RegisterTransportHandoverResult {
	t.Helper()
	var result application.RegisterTransportHandoverResult
	mustWithinSegmentTransaction(t, registries.transactor, ctx, func(txCtx context.Context) error {
		var err error
		result, err = registries.handovers.Register(txCtx, application.RegisterTransportHandoverCommand{
			TenantID:          registries.tenant,
			Object:            object,
			Scope:             "scope-" + object,
			ReleasedBy:        "node-1",
			ReceivedBy:        "carrier-1",
			Verdict:           domain.ObjectHandedOver,
			ReleasingEvidence: "release/" + object + "/1",
			ReceivingEvidence: "receive/" + object + "/1",
			Rule:              "handover-rule/v1",
			Version:           "handover-result/" + object + "/v1",
			JudgedAt:          rederivationEnteredAt,
			Segment:           segment,
		})
		return err
	})
	if result.Outcome() != application.HandoverRegistered || result.SegmentContinuationReference() != "" {
		t.Fatalf("交接首登：outcome=%s debt=%q", result.Outcome(), result.SegmentContinuationReference())
	}
	return result
}

func (registries rederivationRegistries) correctHandover(
	t *testing.T, ctx context.Context, object string, verdict domain.HandoverVerdict, basis string,
) application.RegisterTransportHandoverResult {
	t.Helper()
	var result application.RegisterTransportHandoverResult
	mustWithinSegmentTransaction(t, registries.transactor, ctx, func(txCtx context.Context) error {
		var err error
		result, err = registries.handovers.Correct(txCtx, application.CorrectTransportHandoverCommand{
			TenantID:           registries.tenant,
			Object:             object,
			Scope:              "scope-" + object,
			PredecessorVersion: "handover-result/" + object + "/v1",
			Verdict:            verdict,
			ReleasingEvidence:  "release/" + object + "/2",
			ReceivingEvidence:  "receive/" + object + "/2",
			Rule:               "handover-rule/v1",
			Basis:              basis,
			NewVersion:         "handover-result/" + object + "/v2",
			CorrectedAt:        rederivationCorrectedAt,
		})
		return err
	})
	if result.Outcome() != application.HandoverCorrected {
		t.Fatalf("交接更正：outcome=%s reason=%s", result.Outcome(), result.UndecidedReason())
	}
	return result
}

func (registries rederivationRegistries) readSegment(t *testing.T, ctx context.Context, segment string) domain.ActualFulfillmentSegment {
	t.Helper()
	record, found, err := registries.segments.FindByKey(ctx, segmentKeyFixture(t, "tenant-1", segment))
	if err != nil || !found {
		t.Fatalf("读回段 %s：found=%v err=%v", segment, found, err)
	}
	return record.Segment
}

// participationRows 直接数库里的行：链的形状读回时是派生的，「原行一字不动」只能在库面上数。
func (registries rederivationRegistries) participationRows(t *testing.T, ctx context.Context, segment, object string) int {
	t.Helper()
	var rows int
	if err := registries.pool.QueryRow(ctx,
		`SELECT count(*) FROM transport_fulfillment.fulfillment_participation
		  WHERE tenant_id = 'tenant-1' AND segment_ref = $1 AND object_ref = $2`, segment, object,
	).Scan(&rows); err != nil {
		t.Fatalf("数参与行：%v", err)
	}
	return rows
}

func pickupVersionOf(t *testing.T, result application.RegisterOffsitePickupResult) string {
	t.Helper()
	record, present := result.Record()
	if !present {
		t.Fatal("结果里没有揽收记录")
	}
	return record.Pickup.Version().String()
}

// Covers: ADR-0112 决定一、二在揽收来源上的正格——更正落新版本，同事务在同段替代该对象的参与：链尾凭
// `OFFSITE-PICKUP/<新版本>` 入场、回指 `OFFSITE-PICKUP/<前版>`、起点随更正后的发生时刻；首登那一行一字不动
// 且不再是当前；段那一半两格都空。
func TestAPickupCorrectionSupersedesTheParticipationInTheDatabaseWhileTheRootRowStays(t *testing.T) {
	registries := newRederivationRegistries(t)
	ctx := t.Context()
	object := segmentRef(t, domain.NewCarriedObjectReference, "parcel-1")

	first := registries.registerPickup(t, ctx, "parcel-1", "SEG-RD-1")
	rootVersion := pickupVersionOf(t, first)
	correctedOccurredAt := rederivationEnteredAt.Add(time.Hour)

	corrected := registries.correctPickup(t, ctx, "parcel-1", rootVersion, correctedOccurredAt, rederivationCorrectedAt)
	if corrected.SegmentEntryRefusal() != application.SegmentEntryRefusalNone || corrected.SegmentContinuationReference() != "" {
		t.Fatalf("段那一半答了拒绝或欠账：refusal=%s debt=%q", corrected.SegmentEntryRefusal(), corrected.SegmentContinuationReference())
	}
	newVersion := pickupVersionOf(t, corrected)
	if newVersion == rootVersion {
		t.Fatalf("更正没有签发新版本：%s", newVersion)
	}

	segment := registries.readSegment(t, ctx, "SEG-RD-1")
	current, present := segment.ParticipationFor(object)
	if !present || current.EntryBasis().String() != "OFFSITE-PICKUP/"+newVersion {
		t.Fatalf("当前参与不是替代版本：present=%v basis=%s", present, current.EntryBasis())
	}
	if supersedes, chained := current.Supersedes(); !chained || supersedes.String() != "OFFSITE-PICKUP/"+rootVersion {
		t.Fatalf("替代版本没有回指首登：chained=%v supersedes=%s", chained, supersedes)
	}
	if !current.EnteredAt().Equal(correctedOccurredAt) || !current.Active() || current.EntryKind() != domain.EnteredByOffsitePickup {
		t.Fatalf("替代版本的起点或在场不对：enteredAt=%v active=%v kind=%v", current.EnteredAt(), current.Active(), current.EntryKind())
	}
	if planned, has := current.PlannedSegment(); has {
		t.Fatalf("首登没有计划段，替代版本却长出了 %s", planned)
	}

	history := segment.ParticipationHistory(object)
	if len(history) != 2 {
		t.Fatalf("链长 = %d，want 2", len(history))
	}
	root := history[0]
	if root.EntryBasis().String() != "OFFSITE-PICKUP/"+rootVersion || !root.EnteredAt().Equal(rederivationEnteredAt) || !root.Superseded() || root.Active() {
		t.Fatalf("首登被改写或没有被标为已被替代：basis=%s enteredAt=%v superseded=%v active=%v",
			root.EntryBasis(), root.EnteredAt(), root.Superseded(), root.Active())
	}
	if segment.ActiveParticipations() != 1 {
		t.Fatalf("在场计数 = %d，want 1（只数链尾）", segment.ActiveParticipations())
	}
	if rows := registries.participationRows(t, ctx, "SEG-RD-1", "parcel-1"); rows != 2 {
		t.Fatalf("库里参与行 = %d，want 2（只插不改）", rows)
	}

	var rootSupersedes *string
	var rootEnteredAt time.Time
	if err := registries.pool.QueryRow(ctx,
		`SELECT supersedes_entry_basis, entered_at FROM transport_fulfillment.fulfillment_participation
		  WHERE tenant_id = 'tenant-1' AND segment_ref = 'SEG-RD-1' AND object_ref = 'parcel-1' AND entry_basis = $1`,
		"OFFSITE-PICKUP/"+rootVersion,
	).Scan(&rootSupersedes, &rootEnteredAt); err != nil {
		t.Fatalf("读首登行：%v", err)
	}
	if rootSupersedes != nil || !rootEnteredAt.Equal(rederivationEnteredAt) {
		t.Fatalf("首登行被动过：supersedes=%v enteredAt=%v", rootSupersedes, rootEnteredAt)
	}
}

// Covers: ADR-0112 决定二的反格——对象从未进段，更正照样落新版本，段那一半如实答 NO_PARTICIPATION_TO_REDERIVE，
// 不是欠账（重试不会变），库里也不会凭空长出参与。
func TestAPickupCorrectionForAnObjectOutsideAnySegmentAnswersNoParticipationToRederive(t *testing.T) {
	registries := newRederivationRegistries(t)
	ctx := t.Context()

	first := registries.registerPickup(t, ctx, "parcel-2", "")
	corrected := registries.correctPickup(t, ctx, "parcel-2", pickupVersionOf(t, first), rederivationEnteredAt.Add(time.Hour), rederivationCorrectedAt)

	if corrected.SegmentEntryRefusal() != application.SegmentEntryRefusedNoParticipationToRederive {
		t.Fatalf("refusal = %s, want NO_PARTICIPATION_TO_REDERIVE", corrected.SegmentEntryRefusal())
	}
	if corrected.SegmentContinuationReference() != "" {
		t.Fatalf("无可替代不是欠账：debt=%q", corrected.SegmentContinuationReference())
	}
	var rows int
	if err := registries.pool.QueryRow(ctx,
		`SELECT count(*) FROM transport_fulfillment.fulfillment_participation WHERE tenant_id = 'tenant-1' AND object_ref = 'parcel-2'`,
	).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("从未进段的对象长出了参与：rows=%d err=%v", rows, err)
	}
}

// Covers: ADR-0112 决定一、二在交接来源上的正格——仍`已交接`的更正形成替代参与版本，链尾凭
// `TRANSPORT-HANDOVER/<新版本>` 入场、回指 `TRANSPORT-HANDOVER/<前版>`；起点是交接的判断时刻（更正不改业务时间）。
func TestAHandoverCorrectionThatStillHandsOverSupersedesTheParticipationInTheDatabase(t *testing.T) {
	registries := newRederivationRegistries(t)
	ctx := t.Context()
	object := segmentRef(t, domain.NewCarriedObjectReference, "parcel-3")

	registries.registerHandover(t, ctx, "parcel-3", "SEG-RD-3")
	corrected := registries.correctHandover(t, ctx, "parcel-3", domain.ObjectHandedOver, "")
	if corrected.SegmentEntryRefusal() != application.SegmentEntryRefusalNone || corrected.SegmentContinuationReference() != "" {
		t.Fatalf("段那一半答了拒绝或欠账：refusal=%s debt=%q", corrected.SegmentEntryRefusal(), corrected.SegmentContinuationReference())
	}

	segment := registries.readSegment(t, ctx, "SEG-RD-3")
	current, present := segment.ParticipationFor(object)
	if !present || current.EntryBasis().String() != "TRANSPORT-HANDOVER/handover-result/parcel-3/v2" {
		t.Fatalf("当前参与不是替代版本：present=%v basis=%s", present, current.EntryBasis())
	}
	if supersedes, chained := current.Supersedes(); !chained || supersedes.String() != "TRANSPORT-HANDOVER/handover-result/parcel-3/v1" {
		t.Fatalf("替代版本没有回指首登：chained=%v supersedes=%s", chained, supersedes)
	}
	if !current.EnteredAt().Equal(rederivationEnteredAt) || current.EntryKind() != domain.EnteredByTransportHandover || !current.Active() {
		t.Fatalf("替代版本的起点、种类或在场不对：enteredAt=%v kind=%v active=%v", current.EnteredAt(), current.EntryKind(), current.Active())
	}
	if history := segment.ParticipationHistory(object); len(history) != 2 || !history[0].Superseded() {
		t.Fatalf("链形不对：len=%d", len(history))
	}
	if rows := registries.participationRows(t, ctx, "SEG-RD-3", "parcel-3"); rows != 2 {
		t.Fatalf("库里参与行 = %d，want 2", rows)
	}
}

// Covers: ADR-0112 决定四——`已交接`被更正为拒收：更正落新版本，段那一半答 CORRECTION_WITHDRAWS_CONTROL，
// 原参与照旧站着（失效格另票 tf/11 落地前不猜也不静默）。
func TestAHandoverCorrectionThatWithdrawsControlLeavesTheParticipationInTheDatabase(t *testing.T) {
	registries := newRederivationRegistries(t)
	ctx := t.Context()
	object := segmentRef(t, domain.NewCarriedObjectReference, "parcel-4")

	registries.registerHandover(t, ctx, "parcel-4", "SEG-RD-4")
	corrected := registries.correctHandover(t, ctx, "parcel-4", domain.HandoverRefused, "refusal/damaged-seal")
	if corrected.SegmentEntryRefusal() != application.SegmentEntryRefusedCorrectionWithdrawsControl {
		t.Fatalf("refusal = %s, want CORRECTION_WITHDRAWS_CONTROL", corrected.SegmentEntryRefusal())
	}
	if corrected.SegmentContinuationReference() != "" {
		t.Fatalf("撤回控制不是欠账：debt=%q", corrected.SegmentContinuationReference())
	}

	segment := registries.readSegment(t, ctx, "SEG-RD-4")
	current, present := segment.ParticipationFor(object)
	if !present || current.EntryBasis().String() != "TRANSPORT-HANDOVER/handover-result/parcel-4/v1" || !current.Active() {
		t.Fatalf("原参与被动了：present=%v basis=%s active=%v", present, current.EntryBasis(), current.Active())
	}
	if rows := registries.participationRows(t, ctx, "SEG-RD-4", "parcel-4"); rows != 1 {
		t.Fatalf("库里参与行 = %d，want 1", rows)
	}
}

// Covers: ADR-0112 决定三——段已关闭仍重派生：替代版本照插进已关闭的段，段不重开、关闭时刻不动；替代版本继承
// 原参与的离场三件（终止那一格），所以它不在场、在场计数仍为零。
func TestAPickupCorrectionRederivesIntoAClosedSegmentInTheDatabaseWithoutReopeningIt(t *testing.T) {
	registries := newRederivationRegistries(t)
	ctx := t.Context()
	object := segmentRef(t, domain.NewCarriedObjectReference, "parcel-5")
	key := segmentKeyFixture(t, "tenant-1", "SEG-RD-5")
	endedAt := rederivationEnteredAt.Add(3 * time.Hour)
	closedAt := rederivationEnteredAt.Add(4 * time.Hour)

	first := registries.registerPickup(t, ctx, "parcel-5", "SEG-RD-5")
	rootVersion := pickupVersionOf(t, first)

	var ended application.EndFulfillmentParticipationResult
	mustWithinSegmentTransaction(t, registries.transactor, ctx, func(txCtx context.Context) error {
		var err error
		ended, err = registries.ender.End(txCtx, application.EndFulfillmentParticipationCommand{
			TenantID: registries.tenant,
			Segment:  "SEG-RD-5",
			Object:   "parcel-5",
			Source:   application.ParticipationEndedByTermination,
			Basis:    "CONTROL-TERMINATION/parcel-5",
			EndedAt:  endedAt,
		})
		return err
	})
	if ended.Outcome() != application.ParticipationEndedNow {
		t.Fatalf("终止参与：outcome=%s", ended.Outcome())
	}
	closing, err := registries.readSegment(t, ctx, "SEG-RD-5").CloseSegment(closedAt)
	if err != nil {
		t.Fatalf("领域关段：%v", err)
	}
	mustWithinSegmentTransaction(t, registries.transactor, ctx, func(txCtx context.Context) error {
		outcome, err := registries.segments.CloseSegment(txCtx, key, closedAt)
		if err == nil && outcome != ports.SegmentClosed {
			t.Fatalf("关段 outcome = %d", outcome)
		}
		return err
	})
	if !closing.Closed() {
		t.Fatal("夹具没有把段关上")
	}

	correctedOccurredAt := rederivationEnteredAt.Add(30 * time.Minute)
	corrected := registries.correctPickup(t, ctx, "parcel-5", rootVersion, correctedOccurredAt, rederivationEnteredAt.Add(5*time.Hour))
	if corrected.SegmentEntryRefusal() != application.SegmentEntryRefusalNone || corrected.SegmentContinuationReference() != "" {
		t.Fatalf("已关闭的段拒了重派生：refusal=%s debt=%q", corrected.SegmentEntryRefusal(), corrected.SegmentContinuationReference())
	}

	segment := registries.readSegment(t, ctx, "SEG-RD-5")
	if at, isClosed := segment.ClosedAt(); !segment.Closed() || !isClosed || !at.Equal(closedAt) {
		t.Fatalf("段被重开或关闭时刻被动：closed=%v at=%v", segment.Closed(), at)
	}
	current, present := segment.ParticipationFor(object)
	if !present || current.EntryBasis().String() != "OFFSITE-PICKUP/"+pickupVersionOf(t, corrected) || !current.EnteredAt().Equal(correctedOccurredAt) {
		t.Fatalf("当前参与不是替代版本：present=%v basis=%s enteredAt=%v", present, current.EntryBasis(), current.EnteredAt())
	}
	kind, basis, at, done := current.End()
	if !done || kind != domain.EndedByControlTermination || basis.String() != "CONTROL-TERMINATION/parcel-5" || !at.Equal(endedAt) {
		t.Fatalf("替代版本没有继承离场三件：done=%v kind=%v basis=%s at=%v", done, kind, basis, at)
	}
	if current.Active() || segment.ActiveParticipations() != 0 {
		t.Fatalf("已关闭段上长出了在场参与：active=%v count=%d", current.Active(), segment.ActiveParticipations())
	}
	if history := segment.ParticipationHistory(object); len(history) != 2 || !history[0].Superseded() {
		t.Fatalf("链形不对：len=%d", len(history))
	}
}
