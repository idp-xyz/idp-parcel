package main

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: TF 四个 admin 写面的第二参是真编排（票 tf-segment-lifecycle-closure/07）。最要紧的是把票 01 的三条
// 完工判据第一次在装配点上串起来走一遍：立段 → 进一个对象 → 关段答 SEGMENT_STILL_ACTIVE（仍有在场参与
// 关不上）→ 明确终止那条参与 → 关段答 SEGMENT_CLOSED → 重放答 SEGMENT_ALREADY_CLOSED → 新对象凭正当交接
// 来到同段答 SEGMENT_CLOSED（票 04 那一格在真库上同样成立）。派送任务与装载分配各证一次首登 + 重放。
// 测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredSegmentOperationsRunTheClosureSequenceAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	operations, err := buildSegmentOperations(db)
	if err != nil {
		t.Fatalf("装配写面编排：%v", err)
	}
	controlFacts, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatalf("装配控制事实编排：%v", err)
	}
	tenant := mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-1")

	// 立段：一个对象凭已交接的权威交接进 SYN-SEGMENT-1。
	entered, err := controlFacts.handover.Register(t.Context(), registerHandoverCommand(t, "SYN-PARCEL-1", "SYN-SEGMENT-1", ""))
	if err != nil || entered.Outcome() != tfapp.HandoverRegistered || entered.SegmentContinuationReference() != "" {
		t.Fatalf("立段：outcome=%v err=%v debt=%q", entered.Outcome(), err, entered.SegmentContinuationReference())
	}

	closeCommand := tfapp.CloseFulfillmentSegmentCommand{TenantID: tenant, Segment: "SYN-SEGMENT-1", ClosedAt: controlFactJudgedAt.Add(4 * time.Hour)}
	stillActive, err := operations.closer.Close(t.Context(), closeCommand)
	if err != nil || stillActive.Outcome() != tfapp.SegmentStillHasActiveParticipations {
		t.Fatalf("仍有在场参与时关段：outcome=%v err=%v，want SEGMENT_STILL_ACTIVE", stillActive.Outcome(), err)
	}

	terminated, err := operations.enderOf.End(t.Context(), tfapp.EndFulfillmentParticipationCommand{
		TenantID: tenant,
		Segment:  "SYN-SEGMENT-1",
		Object:   "SYN-PARCEL-1",
		Source:   tfapp.ParticipationEndedByTermination,
		Basis:    "SYN-CONTROL-TERMINATION/CASE-1",
		EndedAt:  controlFactJudgedAt.Add(2 * time.Hour),
	})
	if err != nil || terminated.Outcome() != tfapp.ParticipationEndedNow {
		t.Fatalf("明确终止：outcome=%v err=%v", terminated.Outcome(), err)
	}

	closed, err := operations.closer.Close(t.Context(), closeCommand)
	if err != nil || closed.Outcome() != tfapp.SegmentClosedNow {
		t.Fatalf("关段：outcome=%v err=%v，want SEGMENT_CLOSED", closed.Outcome(), err)
	}
	replay, err := operations.closer.Close(t.Context(), closeCommand)
	if err != nil || replay.Outcome() != tfapp.SegmentAlreadyClosed {
		t.Fatalf("重放关段：outcome=%v err=%v，want SEGMENT_ALREADY_CLOSED——首笔事务没有提交", replay.Outcome(), err)
	}

	late := registerHandoverCommand(t, "SYN-PARCEL-2", "SYN-SEGMENT-1", "")
	late.Version = "SYN-HANDOVER-RESULT/PARCEL-2/v1"
	late.JudgedAt = controlFactJudgedAt.Add(5 * time.Hour)
	arrived, err := controlFacts.handover.Register(t.Context(), late)
	if err != nil || arrived.Outcome() != tfapp.HandoverRegistered {
		t.Fatalf("关闭后到场：outcome=%v err=%v", arrived.Outcome(), err)
	}
	if arrived.SegmentEntryRefusal() != tfapp.SegmentEntryRefusedSegmentClosed || arrived.SegmentContinuationReference() != "" {
		t.Fatalf("关闭后到场：refusal=%v debt=%q，want SEGMENT_CLOSED 且无欠账", arrived.SegmentEntryRefusal(), arrived.SegmentContinuationReference())
	}

	// 派送任务与装载分配：首登 + 重放各一次，证两口接的是各自的登记册且事务提交。
	taskCommand := tfapp.OpenDispatchTaskCommand{
		TenantID:   tenant,
		Task:       "SYN-DISPATCH-TASK-1",
		Kind:       tfdomain.PickupDispatch,
		Objects:    []string{"SYN-PARCEL-3", "SYN-PARCEL-4"},
		Place:      "SYN-WAREHOUSE-1",
		WindowFrom: controlFactJudgedAt.Add(24 * time.Hour),
		WindowTo:   controlFactJudgedAt.Add(27 * time.Hour),
		Conditions: "SYN-SERVICE-CONDITION/v1",
		OpenedAt:   controlFactJudgedAt,
	}
	opened, err := operations.opener.Open(t.Context(), taskCommand)
	if err != nil || opened.Outcome() != tfapp.DispatchTaskOpened {
		t.Fatalf("建派送任务：outcome=%v err=%v", opened.Outcome(), err)
	}
	if again, err := operations.opener.Open(t.Context(), taskCommand); err != nil || again.Outcome() != tfapp.DispatchTaskAlreadyRegistered {
		t.Fatalf("重投派送任务：outcome=%v err=%v", again.Outcome(), err)
	}

	assignmentCommand := tfapp.FormLoadAssignmentCommand{
		TenantID:   tenant,
		Assignment: "SYN-LOAD-ASSIGNMENT-1",
		Schedule:   "SYN-SCHEDULE-1",
		Members:    []string{"SYN-PARCEL-3", "SYN-PARCEL-4"},
		Version:    "SYN-LAV-000000000001",
		AssignedAt: controlFactJudgedAt,
	}
	formed, err := operations.assign.Form(t.Context(), assignmentCommand)
	if err != nil || formed.Outcome() != tfapp.LoadAssignmentFormed {
		t.Fatalf("形成装载分配：outcome=%v err=%v", formed.Outcome(), err)
	}
	if again, err := operations.assign.Form(t.Context(), assignmentCommand); err != nil || again.Outcome() != tfapp.LoadAssignmentVersionExists {
		t.Fatalf("重投装载分配：outcome=%v err=%v", again.Outcome(), err)
	}
}
