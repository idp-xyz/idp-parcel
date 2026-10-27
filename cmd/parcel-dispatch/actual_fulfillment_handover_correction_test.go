package main

import (
	"context"
	"testing"
	"time"

	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 证票 demo-chain-gaps/130（ADR-0307）与实际履约发生项自动形成（ADR-0218）的交互：干线段采用了委托，
// 对象在 SIN 的交接首登为待确认、之后更正成`已交接`。更正那一版结束对象在干线段里的参与，它那一封信封由同一路认出「这件事实
// 结束了干线段那一条参与」，形成发生项。夹具沿用同目录 ADR-0218 那份（PC 真发布协议、真采用、生产登记编排、生产路由），只记 `S`。

// Covers: AT-TF-329 与 AT-TF-246 交互的真库半边（S）——SIN 交接首登为待确认：那一封答交接不是`已交接`、入账，干线段的参与仍在场，
// 不形成发生项。经生产登记编排更正成`已交接`之后，干线段那一条参与由更正那一版结束（业务时间取交接时刻），更正版那一封经生产
// 路由一拍，形成恰一项发生项，成员是这件包裹。票 130 之前更正不结束干线段的参与，这一封答没有参与被结束、干线这一段不计费。
func TestAPendingHandoverCorrectedToHandedOverAtTheEndOfAnAdoptedSegmentFormsTheOccurrence(t *testing.T) {
	const parcel = "SYN-PARCEL-DCG130-01"
	beat, db, store := wiredBeat(t)
	registerTrunkAgreement(t, db, formationTenant)
	registerBuyTriggers(t, db, formationTenant)
	saveTrunkCommission(t, db, parcel)
	register := newHandoverRegistration(t, db, store)
	handOverAtSZX(t, register, db, parcel)
	adoptTrunkCommission(t, db)

	tenant := mustTF(t, tfdomain.NewTenantID, formationTenant)
	pendingVersion, confirmedVersion := "SIN-"+parcel, "SIN-"+parcel+"-confirmed"
	var pending tfapplication.RegisterTransportHandoverResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		pending, err = register.Register(txCtx, tfapplication.RegisterTransportHandoverCommand{
			TenantID:          tenant,
			Object:            parcel,
			Scope:             formationScope,
			ReleasedBy:        "SYN-PARTY-CARRIER-TRUNK-01",
			ReceivedBy:        "SYN-NODE-SIN-HUB",
			Verdict:           tfdomain.HandoverPendingConfirmation,
			ReleasingEvidence: "SYN-EVIDENCE-RELEASE-DCG130",
			Basis:             "SYN-BASIS-COUNT-UNCLEAR-DCG130",
			Version:           pendingVersion,
			JudgedAt:          formationAt(3 * time.Hour),
		})
		return err
	})
	if pending.Outcome() != tfapplication.HandoverRegistered {
		t.Fatalf("登记待确认：%s", pending.Outcome())
	}
	pendingEvent := handoverEventID(formationTenant, parcel, formationScope, pendingVersion)
	dispatchUntilPublished(t, beat, db, pendingEvent)
	if occurrences := formationOccurrences(t, db); len(occurrences) != 0 {
		t.Fatalf("待确认那一封形成了 %d 项发生项, want 0", len(occurrences))
	}

	var corrected tfapplication.RegisterTransportHandoverResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		corrected, err = register.Correct(txCtx, tfapplication.CorrectTransportHandoverCommand{
			TenantID:           tenant,
			Object:             parcel,
			Scope:              formationScope,
			PredecessorVersion: pendingVersion,
			Verdict:            tfdomain.ObjectHandedOver,
			ReleasingEvidence:  "SYN-EVIDENCE-RELEASE-DCG130",
			ReceivingEvidence:  "SYN-EVIDENCE-RECEIVE-DCG130",
			Rule:               "SYN-HANDOVER-RULE/v1",
			NewVersion:         confirmedVersion,
			CorrectedAt:        formationAt(3*time.Hour + 10*time.Minute),
		})
		return err
	})
	if corrected.Outcome() != tfapplication.HandoverCorrected || corrected.ParticipationEnd() != tfapplication.ParticipationEndedNow {
		t.Fatalf("更正成已交接：outcome=%s end=%s", corrected.Outcome(), corrected.ParticipationEnd())
	}
	confirmedEvent := handoverEventID(formationTenant, parcel, formationScope, confirmedVersion)
	dispatchUntilPublished(t, beat, db, confirmedEvent)
	if n := inboxRowsOf(t, db, actualOnHandoverConsumerName, confirmedEvent); n != 1 {
		t.Fatalf("本路 inbox 行 = %d, want 1", n)
	}
	occurrences := formationOccurrences(t, db)
	if len(occurrences) != 1 || len(occurrences[0].members) != 1 || occurrences[0].members[0] != parcel ||
		occurrences[0].journey != "JRN/"+parcel || occurrences[0].registeredBy != tfapplication.ActualFulfillmentFormationRegistrant {
		t.Fatalf("发生项 = %+v, want 恰一项、成员是 %s", occurrences, parcel)
	}
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var occurredAt time.Time
	if err := querier.QueryRow(t.Context(),
		`SELECT occurred_at FROM transport_fulfillment.transport_charge_occurrence WHERE tenant_id = $1 AND occurrence_ref = $2`,
		formationTenant, occurrences[0].occurrence).Scan(&occurredAt); err != nil || !occurredAt.Equal(formationAt(3*time.Hour)) {
		t.Fatalf("发生项业务时间 = %s（%v）, want 交接时刻 %s", occurredAt, err, formationAt(3*time.Hour))
	}
}
