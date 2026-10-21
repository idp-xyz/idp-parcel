package main

import (
	"context"
	"testing"
	"time"

	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 证票 demo-chain-gaps/129：交接更正后仍为`已交接`，更正版信封经派送发起这一路不另立第二项任务；
// 前版信封在更正之后才到，不替链尾那一版形成任务。交接与更正都经生产登记编排落库（真段、真替代参与），夹具全部合成，只记 `S`。

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
