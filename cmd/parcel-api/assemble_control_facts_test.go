package main

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var controlFactJudgedAt = time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)

// Covers: 控制事实四口的第二参是真编排（票 tf-segment-lifecycle-closure/04）——交接登记册、揽收
// 登记册、揽收尝试库、**段登记册**、结果版本签发与三条 Outbox 意图交付在真实 PostgreSQL 上装得
// 起来，且事务边界成立（重放走已有版本，证首笔真的提交了）。
//
// 本测试最要紧的一句是**进段那道门第一次在生产装配上被证明走得到**：带 Segment 的交接首登之后，
// 段登记册里读得到那个段，对象的参与关系带着 Intake 给的计划段；随后的收寄经 Join 加入同段；
// 一次到访里成功对象进段、失败对象不进（CONTEXT「失败结果不制造实际履约段」）。此前三条编排
// 的段登记册缝在生产上从未接过真——Deps 允许缺席，装配点若留空这一切都不会红。
// 测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredControlFactsEnterTheSegmentAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	orchestrations, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatalf("装配控制事实编排：%v", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatalf("段登记册读面：%v", err)
	}
	tenant := mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-1")
	segmentKey := tfports.FulfillmentSegmentKey{
		TenantID: tenant,
		Segment:  mustValue(t, tfdomain.NewFulfillmentSegmentReference, "SYN-SEGMENT-1"),
	}

	// 一、交接首登带段引用：交接登上、段立起来、参与关系带计划段。
	handoverCommand := registerHandoverCommand(t, "SYN-PARCEL-1", "SYN-SEGMENT-1", "SYN-PLANNED-1")
	registered, err := orchestrations.handover.Register(t.Context(), handoverCommand)
	if err != nil {
		t.Fatalf("首登交接：%v", err)
	}
	if got := registered.Outcome(); got != tfapp.HandoverRegistered {
		t.Fatalf("outcome = %v, want HANDOVER_REGISTERED", got)
	}
	if ref := registered.HandoverHandoffReference(); ref != "" {
		t.Fatalf("意图交付走真 Outbox 应当成功，却留了续办引用 %q", ref)
	}
	if ref := registered.SegmentContinuationReference(); ref != "" {
		t.Fatalf("段登记册接真后进段应当成功，却留了欠账 %q", ref)
	}

	replay, err := orchestrations.handover.Register(t.Context(), handoverCommand)
	if err != nil {
		t.Fatalf("重放首登：%v", err)
	}
	if got := replay.Outcome(); got != tfapp.HandoverExistingVersion {
		t.Fatalf("outcome = %v, want EXISTING_VERSION——重放没走已有版本，首笔事务没有提交", got)
	}

	assertParticipation(t, segments, segmentKey, "SYN-PARCEL-1", "SYN-PLANNED-1", tfdomain.EnteredByTransportHandover)

	// 二、单对象收寄加入同一个段：走的是 Join 那条窄口，不是 Save。
	pickupRegistered, err := orchestrations.pickupRegistration.Register(t.Context(), tfapp.RegisterOffsitePickupCommand{
		TenantID:       tenant,
		Object:         "SYN-PARCEL-2",
		Task:           "SYN-PICKUP-TASK-1",
		Attempt:        "SYN-ATTEMPT-1",
		Place:          "SYN-DOOR-1",
		Control:        "SYN-TRANSPORT-CONTROL-2",
		ExecutedBy:     "SYN-COURIER-1",
		OccurredAt:     controlFactJudgedAt.Add(30 * time.Minute),
		Segment:        "SYN-SEGMENT-1",
		PlannedSegment: "SYN-PLANNED-2",
	})
	if err != nil {
		t.Fatalf("单对象收寄登记：%v", err)
	}
	if got := pickupRegistered.Outcome(); got != tfapp.PickupRegistered {
		t.Fatalf("outcome = %v, want PICKUP_REGISTERED", got)
	}
	if ref := pickupRegistered.SegmentContinuationReference(); ref != "" {
		t.Fatalf("收寄加入既有段应当成功，却留了欠账 %q", ref)
	}
	assertParticipation(t, segments, segmentKey, "SYN-PARCEL-2", "SYN-PLANNED-2", tfdomain.EnteredByOffsitePickup)

	// 三、一次到访两对象一成一败：成功对象进段，失败对象不进，且不算欠账。
	attempt, err := orchestrations.pickupAttempt.Handle(t.Context(), tfapp.PerformOffsitePickupCommand{
		TenantID:    tenant,
		SourceID:    "SYN-SOURCE-1",
		Task:        "SYN-PICKUP-TASK-2",
		Attempt:     "SYN-ATTEMPT-2",
		ExecutedBy:  "SYN-COURIER-1",
		Place:       "SYN-WAREHOUSE-1",
		PlannedFrom: controlFactJudgedAt,
		PlannedTo:   controlFactJudgedAt.Add(3 * time.Hour),
		ArrivedAt:   controlFactJudgedAt.Add(time.Hour),
		Evidence:    "SYN-ATTEMPT-EVIDENCE-2",
		Segment:     "SYN-SEGMENT-1",
		Objects: []tfapp.ObjectPickupSubmission{
			{
				Object:         mustValue(t, tfdomain.NewCarriedObjectReference, "SYN-PARCEL-3"),
				Outcome:        tfdomain.ObjectPickedUp,
				Control:        mustValue(t, tfdomain.NewTransportControlReference, "SYN-TRANSPORT-CONTROL-3"),
				OccurredAt:     controlFactJudgedAt.Add(65 * time.Minute),
				PlannedSegment: "SYN-PLANNED-3",
			},
			{
				Object:     mustValue(t, tfdomain.NewCarriedObjectReference, "SYN-PARCEL-4"),
				Outcome:    tfdomain.CustomerAbsent,
				Basis:      mustValue(t, tfdomain.NewAttemptResultBasisReference, "SYN-REASON-ABSENT"),
				OccurredAt: controlFactJudgedAt.Add(70 * time.Minute),
			},
		},
	})
	if err != nil {
		t.Fatalf("多对象到访：%v", err)
	}
	if got := attempt.Outcome(); got != tfapp.PickupAttemptRecorded {
		t.Fatalf("outcome = %v, want ATTEMPT_RECORDED", got)
	}
	if owed := attempt.SegmentEntries(); len(owed) != 0 {
		t.Fatalf("段登记册接真后不该有逐对象欠账：%+v", owed)
	}
	assertParticipation(t, segments, segmentKey, "SYN-PARCEL-3", "SYN-PLANNED-3", tfdomain.EnteredByOffsitePickup)
	record, found, err := segments.FindByKey(t.Context(), segmentKey)
	if err != nil || !found {
		t.Fatalf("读回段：found=%v err=%v", found, err)
	}
	for _, participation := range record.Segment.Participations() {
		if participation.Object().String() == "SYN-PARCEL-4" {
			t.Fatal("失败对象 SYN-PARCEL-4 进了段——失败结果不制造实际履约段")
		}
	}
	if got := len(record.Segment.Participations()); got != 3 {
		t.Fatalf("段内参与关系 = %d，want 3（交接一条、收寄一条、到访成功一条）", got)
	}

	// 四、更正走版本链：新版本回指前版，同一事务边界。
	corrected, err := orchestrations.handover.Correct(t.Context(), tfapp.CorrectTransportHandoverCommand{
		TenantID:           tenant,
		Object:             "SYN-PARCEL-1",
		Scope:              "SYN-HANDOVER-SCOPE-1",
		PredecessorVersion: "SYN-HANDOVER-RESULT/PARCEL-1/v1",
		Verdict:            tfdomain.HandoverRefused,
		ReleasingEvidence:  "SYN-EVIDENCE-RELEASE-1",
		ReceivingEvidence:  "SYN-EVIDENCE-RECEIVE-1",
		Rule:               "SYN-HANDOVER-RULE/v1",
		Basis:              "SYN-REFUSAL-BASIS-1",
		NewVersion:         "SYN-HANDOVER-RESULT/PARCEL-1/v2",
		CorrectedAt:        controlFactJudgedAt.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("更正交接：%v", err)
	}
	if got := corrected.Outcome(); got != tfapp.HandoverCorrected {
		t.Fatalf("outcome = %v, want HANDOVER_CORRECTED", got)
	}
	correctedRecord, has := corrected.Record()
	if !has {
		t.Fatal("更正成功却没带回登记")
	}
	if predecessor, ok := correctedRecord.Handover.Corrects(); !ok || predecessor.String() != "SYN-HANDOVER-RESULT/PARCEL-1/v1" {
		t.Fatalf("corrects = (%q, %v)，版本链没回指前版", predecessor.String(), ok)
	}
}

func registerHandoverCommand(t *testing.T, object, segment, planned string) tfapp.RegisterTransportHandoverCommand {
	t.Helper()
	return tfapp.RegisterTransportHandoverCommand{
		TenantID:          mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-1"),
		Object:            object,
		Scope:             "SYN-HANDOVER-SCOPE-1",
		ReleasedBy:        "SYN-NODE-1",
		ReceivedBy:        "SYN-CARRIER-1",
		Verdict:           tfdomain.ObjectHandedOver,
		ReleasingEvidence: "SYN-EVIDENCE-RELEASE-1",
		ReceivingEvidence: "SYN-EVIDENCE-RECEIVE-1",
		Rule:              "SYN-HANDOVER-RULE/v1",
		Version:           "SYN-HANDOVER-RESULT/PARCEL-1/v1",
		JudgedAt:          controlFactJudgedAt,
		Segment:           segment,
		PlannedSegment:    planned,
	}
}

// assertParticipation 从真库读回段，核对某对象在场、计划段与入场种类都与命令一致。
func assertParticipation(
	t *testing.T,
	segments tfports.ActualFulfillmentSegmentRegistry,
	key tfports.FulfillmentSegmentKey,
	object, planned string,
	entryKind tfdomain.ParticipationEntryKind,
) {
	t.Helper()
	record, found, err := segments.FindByKey(t.Context(), key)
	if err != nil {
		t.Fatalf("读回段：%v", err)
	}
	if !found {
		t.Fatalf("段登记册里没有 %s——进段那道门在生产装配上没走到", key.Segment)
	}
	for _, participation := range record.Segment.Participations() {
		if participation.Object().String() != object {
			continue
		}
		if !participation.Active() {
			t.Fatalf("%s 的参与关系已结束，刚进段不该如此", object)
		}
		if participation.EntryKind() != entryKind {
			t.Fatalf("%s entryKind = %v, want %v", object, participation.EntryKind(), entryKind)
		}
		got, present := participation.PlannedSegment()
		if !present || got.String() != planned {
			t.Fatalf("%s plannedSegment = (%q, %v), want %q——Intake 给的计划段没到库", object, got.String(), present, planned)
		}
		return
	}
	t.Fatalf("段 %s 里没有对象 %s", key.Segment, object)
}
