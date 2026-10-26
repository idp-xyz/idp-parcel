package postgres_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 16 证票 demo-chain-gaps/130（ADR-0307）：真交接登记册 → 真编排的 Correct → 真结束参与 → 真段登记册
// 的进段与真实际承运商判断册，前版待确认、本版`已交接`的更正带对象进首登声明的段、结束它在先前段里的参与；对象此后已有更晚
// 的控制事实时一笔不写。夹具全为合成登记（S 级）。

// registerHandoverInto 登 object 在 scope 下 version 那一版交接；verdict 不是`已交接`时带依据。segment 空即不声明段。
func (registries rederivationRegistries) registerHandoverInto(
	t *testing.T, ctx context.Context, object, scope, version string, verdict domain.HandoverVerdict, judgedAt time.Time, segment string,
) application.RegisterTransportHandoverResult {
	t.Helper()
	command := application.RegisterTransportHandoverCommand{
		TenantID:             registries.tenant,
		Object:               object,
		Scope:                scope,
		ReleasedBy:           "node-1",
		ReceivedBy:           "carrier-1",
		Verdict:              verdict,
		ReleasingEvidence:    "release/" + object + "/" + version,
		ReceivingEvidence:    "receive/" + object + "/" + version,
		Rule:                 "handover-rule/v1",
		Version:              version,
		JudgedAt:             judgedAt,
		Segment:              segment,
		PlannedSegment:       "PLAN-" + segment,
		SegmentServiceAction: domain.SegmentServesLinehaul.String(),
	}
	if verdict != domain.ObjectHandedOver {
		command.ReceivingEvidence, command.Rule, command.Basis = "", "", "basis/"+object+"/"+version
	}
	var result application.RegisterTransportHandoverResult
	mustWithinSegmentTransaction(t, registries.transactor, ctx, func(txCtx context.Context) error {
		var err error
		result, err = registries.handovers.Register(txCtx, command)
		return err
	})
	if result.Outcome() != application.HandoverRegistered || result.SegmentContinuationReference() != "" {
		t.Fatalf("交接首登 %s：outcome=%s debt=%q", version, result.Outcome(), result.SegmentContinuationReference())
	}
	return result
}

func (registries rederivationRegistries) activeSegmentsOf(t *testing.T, ctx context.Context, object string) []string {
	t.Helper()
	keys, err := registries.segments.FindActiveSegments(ctx, registries.tenant, segmentRef(t, domain.NewCarriedObjectReference, object))
	if err != nil {
		t.Fatalf("按对象找在场段：%v", err)
	}
	segments := make([]string, 0, len(keys))
	for _, key := range keys {
		segments = append(segments, key.Segment.String())
	}
	return segments
}

// Covers: AT-TF-329 真库半边（票 demo-chain-gaps/130；ADR-0307 决定一、二）——对象凭一次`已交接`在先前的段里，下一次交接首登
// 为待确认、声明进 SEG-NEXT；更正成`已交接`之后：先前段那一行补上终点（下一次交接、依据是更正那一版按交接键铸的串、时刻是交接
// 业务时间），SEG-NEXT 由这一版立起来（服务动作与计划段取首登声明、首版实际承运商判断随之铸出），对象此刻只在 SEG-NEXT 在场；
// 更正那一行带着首登的段声明。
func TestAPendingHandoverCorrectedToHandedOverEntersTheDeclaredSegmentInTheDatabase(t *testing.T) {
	registries := newRederivationRegistries(t)
	ctx := t.Context()
	pendingAt := rederivationEnteredAt.Add(time.Hour)
	registries.registerHandoverInto(t, ctx, "parcel-1", "scope-h0", "h0", domain.ObjectHandedOver, rederivationEnteredAt, "SEG-PREV")
	registries.registerHandoverInto(t, ctx, "parcel-1", "scope-parcel-1", "handover-result/parcel-1/v1",
		domain.HandoverPendingConfirmation, pendingAt, "SEG-NEXT")
	if active := registries.activeSegmentsOf(t, ctx, "parcel-1"); len(active) != 1 || active[0] != "SEG-PREV" {
		t.Fatalf("待确认之后在场的段 = %v, want 仍只在 SEG-PREV", active)
	}

	corrected := registries.correctHandoverVersion(t, ctx, "parcel-1", "v1", "v2", domain.ObjectHandedOver, "", rederivationCorrectedAt)
	if corrected.ParticipationEnd() != application.ParticipationEndedNow ||
		corrected.SegmentEntryRefusal() != application.SegmentEntryRefusalNone || corrected.SegmentContinuationReference() != "" {
		t.Fatalf("participationEnd=%s refusal=%s debt=%q, want PARTICIPATION_ENDED、两格皆空",
			corrected.ParticipationEnd(), corrected.SegmentEntryRefusal(), corrected.SegmentContinuationReference())
	}

	basis := "TRANSPORT-HANDOVER-KEY/8:parcel-1/14:scope-parcel-1/27:handover-result/parcel-1/v2"
	object := segmentRef(t, domain.NewCarriedObjectReference, "parcel-1")
	previous, _ := registries.readSegment(t, ctx, "SEG-PREV").ParticipationFor(object)
	if kind, endBasis, endedAt, ended := previous.End(); !ended || kind != domain.EndedByNextHandover ||
		endBasis.String() != basis || !endedAt.Equal(pendingAt) {
		t.Fatalf("先前段终点 = %v %s %s %v, want 下一次交接、%s、%s", kind, endBasis, endedAt, ended, basis, pendingAt)
	}
	next := registries.readSegment(t, ctx, "SEG-NEXT")
	current, present := next.ParticipationFor(object)
	if !present || !current.Active() || current.EntryBasis().String() != basis || !current.EnteredAt().Equal(pendingAt) {
		t.Fatalf("SEG-NEXT 里的参与 = %+v present=%v, want 在场、凭 %s、起于 %s", current, present, basis, pendingAt)
	}
	if planned, has := current.PlannedSegment(); !has || planned.String() != "PLAN-SEG-NEXT" {
		t.Fatalf("计划段 = %v %v, want 首登声明的 PLAN-SEG-NEXT", planned, has)
	}
	if action, _ := next.ServiceAction(); action != domain.SegmentServesLinehaul {
		t.Fatalf("SEG-NEXT 服务动作 = %s, want 首登声明的 LINEHAUL", action)
	}
	if active := registries.activeSegmentsOf(t, ctx, "parcel-1"); len(active) != 1 || active[0] != "SEG-NEXT" {
		t.Fatalf("更正之后在场的段 = %v, want 只在 SEG-NEXT", active)
	}
	var judgments int
	if err := registries.pool.QueryRow(ctx,
		`SELECT count(*) FROM transport_fulfillment.actual_carrier_judgment WHERE tenant_id = 'tenant-1' AND segment_ref = 'SEG-NEXT'`,
	).Scan(&judgments); err != nil || judgments != 1 {
		t.Fatalf("SEG-NEXT 的实际承运商判断 %d 份（%v）, want 立段时铸出 1 份", judgments, err)
	}
	var declared *string
	if err := registries.pool.QueryRow(ctx,
		`SELECT declared_segment FROM transport_fulfillment.transport_handover
		  WHERE tenant_id = 'tenant-1' AND object_ref = 'parcel-1' AND handover_version = 'handover-result/parcel-1/v2'`,
	).Scan(&declared); err != nil || declared == nil || *declared != "SEG-NEXT" {
		t.Fatalf("更正那一行的段声明 = %v（%v）, want 承继首登的 SEG-NEXT", declared, err)
	}
}

// Covers: AT-TF-347 真库半边（票 demo-chain-gaps/130；ADR-0307 决定三）——待确认那一版之后，对象已被下一次交接带进 SEG-ONWARD；
// 这时把待确认那一版更正成`已交接`：更正落下，段那一半答 LATER_CONTROL_FACTS、不留续办，SEG-NEXT 不立、先前段那一行的终点
// 仍是下一次交接留下的那一份，对象仍只在 SEG-ONWARD 在场。
func TestACorrectionIntoTheSegmentAfterAnOnwardHandoverWritesNoSegmentInTheDatabase(t *testing.T) {
	registries := newRederivationRegistries(t)
	ctx := t.Context()
	pendingAt := rederivationEnteredAt.Add(time.Hour)
	onwardAt := rederivationEnteredAt.Add(90 * time.Minute)
	registries.registerHandoverInto(t, ctx, "parcel-1", "scope-h0", "h0", domain.ObjectHandedOver, rederivationEnteredAt, "SEG-PREV")
	registries.registerHandoverInto(t, ctx, "parcel-1", "scope-parcel-1", "handover-result/parcel-1/v1",
		domain.HandoverPendingConfirmation, pendingAt, "SEG-NEXT")
	registries.registerHandoverInto(t, ctx, "parcel-1", "scope-onward", "onward", domain.ObjectHandedOver, onwardAt, "SEG-ONWARD")

	corrected := registries.correctHandoverVersion(t, ctx, "parcel-1", "v1", "v2", domain.ObjectHandedOver, "", rederivationCorrectedAt)
	if corrected.SegmentEntryRefusal() != application.SegmentEntryRefusedLaterControlFacts ||
		corrected.SegmentContinuationReference() != "" || corrected.ParticipationEnd() != application.ParticipationEndOutcomeInvalid {
		t.Fatalf("refusal=%s debt=%q end=%s, want LATER_CONTROL_FACTS、无欠账、没结束任何参与",
			corrected.SegmentEntryRefusal(), corrected.SegmentContinuationReference(), corrected.ParticipationEnd())
	}
	if _, found, err := registries.segments.FindByKey(ctx, segmentKeyFixture(t, "tenant-1", "SEG-NEXT")); err != nil || found {
		t.Fatalf("SEG-NEXT 被立起来了（%v）", err)
	}
	object := segmentRef(t, domain.NewCarriedObjectReference, "parcel-1")
	previous, _ := registries.readSegment(t, ctx, "SEG-PREV").ParticipationFor(object)
	if _, endBasis, endedAt, _ := previous.End(); endBasis.String() != "TRANSPORT-HANDOVER-KEY/8:parcel-1/12:scope-onward/6:onward" ||
		!endedAt.Equal(onwardAt) {
		t.Fatalf("先前段的终点被改写：%s %s", endBasis, endedAt)
	}
	if active := registries.activeSegmentsOf(t, ctx, "parcel-1"); len(active) != 1 || active[0] != "SEG-ONWARD" {
		t.Fatalf("在场的段 = %v, want 仍只在 SEG-ONWARD", active)
	}
}

// Covers: AT-TF-356 真库半边（票 demo-chain-gaps/183；ADR-0307 越权风险点 2）——对象在 SEG-PREV 的参与起于 rederivationEnteredAt，
// 首登`已交接`的业务时间早于它（离线补登）：交接照登，答 LATER_CONTROL_FACTS，SEG-NEXT 不立、SEG-PREV 的参与不动，对象只在
// SEG-PREV；此后下一次交接照常结束 SEG-PREV 的参与、进 SEG-ONWARD，不再撞 ErrObjectActiveInSeveralSegments。
func TestALateFirstHandoverLeavesTheObjectInOneSegmentInTheDatabase(t *testing.T) {
	registries := newRederivationRegistries(t)
	ctx := t.Context()
	lateAt := rederivationEnteredAt.Add(-time.Hour)
	onwardAt := rederivationEnteredAt.Add(2 * time.Hour)
	registries.registerHandoverInto(t, ctx, "parcel-1", "scope-h0", "h0", domain.ObjectHandedOver, rederivationEnteredAt, "SEG-PREV")

	late := registries.registerHandoverInto(t, ctx, "parcel-1", "scope-late", "late", domain.ObjectHandedOver, lateAt, "SEG-NEXT")
	if late.SegmentEntryRefusal() != application.SegmentEntryRefusedLaterControlFacts ||
		late.ParticipationEnd() != application.ParticipationEndOutcomeInvalid {
		t.Fatalf("refusal=%s end=%s, want LATER_CONTROL_FACTS、没结束任何参与", late.SegmentEntryRefusal(), late.ParticipationEnd())
	}
	if _, found, err := registries.segments.FindByKey(ctx, segmentKeyFixture(t, "tenant-1", "SEG-NEXT")); err != nil || found {
		t.Fatalf("SEG-NEXT 被立起来了（%v）", err)
	}
	object := segmentRef(t, domain.NewCarriedObjectReference, "parcel-1")
	previous, _ := registries.readSegment(t, ctx, "SEG-PREV").ParticipationFor(object)
	if !previous.Active() {
		t.Fatal("SEG-PREV 的参与被结束了")
	}
	if active := registries.activeSegmentsOf(t, ctx, "parcel-1"); len(active) != 1 || active[0] != "SEG-PREV" {
		t.Fatalf("在场的段 = %v, want 只在 SEG-PREV", active)
	}

	onward := registries.registerHandoverInto(t, ctx, "parcel-1", "scope-onward", "onward", domain.ObjectHandedOver, onwardAt, "SEG-ONWARD")
	if onward.ParticipationEnd() != application.ParticipationEndedNow || onward.SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
		t.Fatalf("此后的交接 end=%s refusal=%s, want PARTICIPATION_ENDED、进段", onward.ParticipationEnd(), onward.SegmentEntryRefusal())
	}
	if active := registries.activeSegmentsOf(t, ctx, "parcel-1"); len(active) != 1 || active[0] != "SEG-ONWARD" {
		t.Fatalf("此后在场的段 = %v, want 只在 SEG-ONWARD", active)
	}
}
