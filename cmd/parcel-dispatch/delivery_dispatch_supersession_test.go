package main

import (
	"context"
	"testing"
	"time"

	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 证票 demo-chain-gaps/129：交接更正后仍为`已交接`，更正版信封经派送发起这一路不另立第二项任务；
// 前版信封在更正之后才到，不替链尾那一版形成任务。另证票 demo-chain-gaps/130：待确认更正成`已交接`那一版带对象进派送段，
// 它的信封形成任务。交接与更正都经生产登记编排落库（真段、真替代参与、真结束参与），夹具全部合成，只记 `S`。

// correctHandover 对那一版交接登一份仍为`已交接`的更正，新版本是 ordinal 那一版。
func (fixture *handoverDispatchFixture) correctHandover(t *testing.T, object string, predecessor, ordinal int) {
	t.Helper()
	command := tfapplication.CorrectTransportHandoverCommand{
		TenantID:           mustTF(t, tfdomain.NewTenantID, handoverDispatchTenant),
		Object:             object,
		Scope:              handoverDispatchScope,
		PredecessorVersion: handoverDispatchVersion(object, predecessor),
		Verdict:            tfdomain.ObjectHandedOver,
		ReleasingEvidence:  "SYN-EVIDENCE-RELEASE-129",
		ReceivingEvidence:  "SYN-EVIDENCE-RECEIVE-129",
		Rule:               "SYN-HANDOVER-RULE/v1",
		NewVersion:         handoverDispatchVersion(object, ordinal),
		CorrectedAt:        handoverDispatchAt.Add(-time.Hour),
	}
	var result tfapplication.RegisterTransportHandoverResult
	mustWithinTX(t, fixture.db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = fixture.register.Correct(txCtx, command)
		return err
	})
	if result.Outcome() != tfapplication.HandoverCorrected || result.SegmentContinuationReference() != "" {
		t.Fatalf("更正交接 → %s：outcome=%s debt=%q", command.NewVersion, result.Outcome(), result.SegmentContinuationReference())
	}
}

// correctHandoverTo 对那一版交接登一份 verdict 的更正，新版本是 ordinal 那一版；不是`已交接`时带依据。
func (fixture *handoverDispatchFixture) correctHandoverTo(t *testing.T, object string, predecessor, ordinal int, verdict tfdomain.HandoverVerdict) {
	t.Helper()
	command := tfapplication.CorrectTransportHandoverCommand{
		TenantID:           mustTF(t, tfdomain.NewTenantID, handoverDispatchTenant),
		Object:             object,
		Scope:              handoverDispatchScope,
		PredecessorVersion: handoverDispatchVersion(object, predecessor),
		Verdict:            verdict,
		Basis:              "SYN-BASIS-130",
		NewVersion:         handoverDispatchVersion(object, ordinal),
		CorrectedAt:        handoverDispatchAt.Add(time.Duration(ordinal-5) * time.Minute),
	}
	if verdict == tfdomain.ObjectHandedOver {
		command.ReleasingEvidence, command.ReceivingEvidence, command.Rule, command.Basis =
			"SYN-EVIDENCE-RELEASE-130", "SYN-EVIDENCE-RECEIVE-130", "SYN-HANDOVER-RULE/v1", ""
	}
	var result tfapplication.RegisterTransportHandoverResult
	mustWithinTX(t, fixture.db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = fixture.register.Correct(txCtx, command)
		return err
	})
	if result.Outcome() != tfapplication.HandoverCorrected || result.SegmentContinuationReference() != "" ||
		result.SegmentEntryRefusal() != tfapplication.SegmentEntryRefusalNone {
		t.Fatalf("更正交接 → %s：outcome=%s debt=%q refusal=%s",
			command.NewVersion, result.Outcome(), result.SegmentContinuationReference(), result.SegmentEntryRefusal())
	}
}

// Covers: AT-TF-329 派送那一半真库半边（票 demo-chain-gaps/130；ADR-0307 决定一，ADR-0230 决定六）——交接首登为待确认、声明进
// 派送段：那一封入账、不形成任务。经生产登记编排更正成`已交接`，对象凭这一版进了派送段，更正版那一封形成一项任务；往回更正成待确认
// 那一封入账；再更正成`已交接`，对象从失效版本回到同一段，那一封认出先前那一项、库里仍一项。
func TestAPendingHandoverCorrectedIntoTheDeliverySegmentFormsTheTaskOnTheCorrection(t *testing.T) {
	fixture := newHandoverDispatchFixture(t)
	pending := handoverInto(t, "SYN-PARCEL-130-A", 1, handoverDispatchSegment, tfdomain.SegmentServesFinalDelivery.String())
	pending.Verdict, pending.ReceivingEvidence, pending.Rule, pending.Basis = tfdomain.HandoverPendingConfirmation, "", "", "SYN-BASIS-130"
	fixture.registerHandover(t, pending)
	fixture.assertSettledWithTasks(t, "SYN-PARCEL-130-A", 1, 0)

	fixture.correctHandoverTo(t, "SYN-PARCEL-130-A", 1, 2, tfdomain.ObjectHandedOver)
	fixture.assertSettledWithTasks(t, "SYN-PARCEL-130-A", 2, 1)

	fixture.correctHandoverTo(t, "SYN-PARCEL-130-A", 2, 3, tfdomain.HandoverPendingConfirmation)
	fixture.assertSettledWithTasks(t, "SYN-PARCEL-130-A", 3, 1)
	fixture.correctHandoverTo(t, "SYN-PARCEL-130-A", 3, 4, tfdomain.ObjectHandedOver)
	fixture.assertSettledWithTasks(t, "SYN-PARCEL-130-A", 4, 1)
}

// Covers: AT-TF-290（票 demo-chain-gaps/129）——前版一拍形成了任务，更正版信封到了：执行器沿替代链认出前版那一项，库里仍
// 一项任务，信封入账。
func TestACorrectedHandoverEnvelopeFindsTheTaskThePredecessorFormed(t *testing.T) {
	fixture := newHandoverDispatchFixture(t)
	fixture.registerHandover(t, handoverInto(t, "SYN-PARCEL-129-A", 1, handoverDispatchSegment,
		tfdomain.SegmentServesFinalDelivery.String()))
	if err := fixture.deliver(t, "SYN-PARCEL-129-A", 1); err != nil {
		t.Fatalf("投递前版信封：%v", err)
	}
	fixture.correctHandover(t, "SYN-PARCEL-129-A", 1, 2)

	if err := fixture.deliver(t, "SYN-PARCEL-129-A", 2); err != nil {
		t.Fatalf("投递更正版信封 = %v, want 入账", err)
	}
	if n := inboxRowsOf(t, fixture.db, handoverDispatchConsumerName, handoverDispatchEventID("SYN-PARCEL-129-A", 2)); n != 1 {
		t.Fatalf("更正版 inbox 行 = %d, want 1", n)
	}
	if n := dispatchTaskRows(t, fixture.db); n != 1 {
		t.Fatalf("任务 = %d 项, want 1——更正不是进段，不另立第二项", n)
	}
}

// Covers: AT-TF-289（票 demo-chain-gaps/129）——前版信封在更正之后才到（前版那一拍一直没跑）：此刻在场的段里链尾是更正版，
// 前版那封不是它的触发，入账不形成任务；更正版那封随后形成一项。
func TestALateEnvelopeOfASupersededHandoverDoesNotTriggerForTheCorrection(t *testing.T) {
	fixture := newHandoverDispatchFixture(t)
	fixture.registerHandover(t, handoverInto(t, "SYN-PARCEL-129-B", 1, handoverDispatchSegment,
		tfdomain.SegmentServesFinalDelivery.String()))
	fixture.correctHandover(t, "SYN-PARCEL-129-B", 1, 2)

	fixture.assertSettledWithTasks(t, "SYN-PARCEL-129-B", 1, 0)
	before := fixture.owners.calls
	if before != 0 {
		t.Fatalf("前版晚到的那封问了所有者 %d 次, want 0", before)
	}
	fixture.assertSettledWithTasks(t, "SYN-PARCEL-129-B", 2, 1)
}
