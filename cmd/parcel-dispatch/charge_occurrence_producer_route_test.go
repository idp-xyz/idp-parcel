package main

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	tfpartycommercial "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/partycommercial"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件钉发生项已形成信封的生产方与消费方对得上（票 demo-chain-gaps/27 完成判据，出自票 26 评审）：发生项经 TF 真登记
// 用例与真交接口入队，不按消费者认的形状手工入队——类型串与载荷三键若在两侧各写一份而写岔，这一拍就到不了 BUY 请求。
// 一拍之后结算发起恰一份 BUY 请求，发生项、原因、版本与协议取自登记的那一份。证据只记 S。

// registerEndedTrunkSegment 登一个干线段：一件包裹凭 SZX 交接入场，以 SIN 的下一次交接离场。
func registerEndedTrunkSegment(t *testing.T, db *bentopg.DB, tenant string) {
	t.Helper()
	value := func(construct func(string) (tfdomain.FulfillmentSegmentReference, error), raw string) tfdomain.FulfillmentSegmentReference {
		built, err := construct(raw)
		if err != nil {
			t.Fatal(err)
		}
		return built
	}
	basis := func(raw string) tfdomain.ParticipationBasisReference {
		built, err := tfdomain.NewParticipationBasisReference(raw)
		if err != nil {
			t.Fatal(err)
		}
		return built
	}
	tenantID, err := tfdomain.NewTenantID(tenant)
	if err != nil {
		t.Fatal(err)
	}
	parcel, err := tfdomain.NewCarriedObjectReference("SYN-PARCEL-01")
	if err != nil {
		t.Fatal(err)
	}
	segment, err := tfdomain.RehydrateActualFulfillmentSegment(tfdomain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: tenantID,
		Segment:  value(tfdomain.NewFulfillmentSegmentReference, "SYN-SEG-SZX-SIN-01"),
		Participations: []tfdomain.RehydrateParticipationSpec{{
			Object:     parcel,
			EntryKind:  tfdomain.EnteredByTransportHandover,
			EntryBasis: basis("TRANSPORT-HANDOVER/SYN-HO-SZX-01"),
			EnteredAt:  time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC),
			EndKind:    tfdomain.EndedByNextHandover,
			EndBasis:   basis("TRANSPORT-HANDOVER/SYN-HO-SIN-01"),
			EndedAt:    time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC),
		}},
	})
	if err != nil {
		t.Fatalf("段夹具：%v", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatal(err)
	}
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		outcome, err := segments.Save(txCtx, tfports.FulfillmentSegmentRecord{
			Key:        tfports.FulfillmentSegmentKey{TenantID: segment.TenantID(), Segment: segment.Segment()},
			Segment:    segment,
			RecordedAt: beatInstant(),
		})
		if err == nil && outcome != tfports.SegmentSaved {
			err = errors.New("segment was not saved fresh")
		}
		return err
	})
}

// registerTrunkOccurrence 走 TF 的真登记用例：协议经真核对口点读 PC，发生项与信封经真登记册与真交接口同一笔落库。
func registerTrunkOccurrence(t *testing.T, db *bentopg.DB, store *outbox.Store, tenant string) tfports.ChargeOccurrenceRecord {
	t.Helper()
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatal(err)
	}
	judgments, err := tfpostgres.NewActualCarrierJudgments(db)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := pcpostgres.NewSupplierAgreementContents(db)
	if err != nil {
		t.Fatal(err)
	}
	agreements, err := tfpartycommercial.NewSupplierAgreementSource(versions)
	if err != nil {
		t.Fatal(err)
	}
	occurrences, err := tfpostgres.NewChargeOccurrences(db)
	if err != nil {
		t.Fatal(err)
	}
	clock := systemClock{}
	handoff, err := tfpostgres.NewOutboxChargeOccurrenceHandoff(db, store, clock)
	if err != nil {
		t.Fatal(err)
	}
	handler := tfapp.NewRegisterChargeOccurrenceHandler(tfapp.RegisterChargeOccurrenceDeps{
		Segments:         segments,
		CarrierJudgments: judgments,
		Agreements:       agreements,
		Occurrences:      occurrences,
		Handoff:          handoff,
		Clock:            clock,
	})
	tenantID, err := tfdomain.NewTenantID(tenant)
	if err != nil {
		t.Fatal(err)
	}
	var result tfapp.RegisterChargeOccurrenceResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		result, err = handler.Register(txCtx, tfapp.RegisterChargeOccurrenceCommand{
			TenantID:           tenantID,
			Registrant:         "https://dex.syn.example#SYN-OPERATOR-CONFIG-01",
			Reason:             tfdomain.ActualFulfillmentOccurrence,
			Segment:            "SYN-SEG-SZX-SIN-01",
			Objects:            []string{"SYN-PARCEL-01"},
			Journey:            "SYN-JRN-SYN-PARCEL-01",
			Provider:           "SYN-PARTY-CARRIER-TRUNK-01",
			AgreementObject:    trunkAgreementObject,
			AgreementVersion:   "v1",
			Quantity:           1,
			Unit:               "SEGMENT",
			ConditionAssertion: "SYN 断言：干线协议 v1 段到段运输计一段",
		})
		return err
	})
	record, present := result.Record()
	if result.Outcome() != tfapp.ChargeOccurrenceRegistered || !present {
		t.Fatalf("登记：%q %q", result.Outcome(), result.Detail())
	}
	return record
}

// Covers: 票 demo-chain-gaps/27 完成判据「cmd/parcel-dispatch 有一条用例经本票的真交接口入队」——生产方的类型串与
// sainbox.ChargeOccurrenceFormedEventType 相等，载荷三键被消费门认下，一拍之后发起恰一份 BUY 请求（触发编排 FIRED）。
func TestAChargeOccurrenceRegisteredByTransportFulfillmentFiresABuyEvaluationRequest(t *testing.T) {
	if tfpostgres.ChargeOccurrenceFormedEventType != string(sainbox.ChargeOccurrenceFormedEventType) {
		t.Fatalf("生产方类型 %q 与消费方类型 %q 不等", tfpostgres.ChargeOccurrenceFormedEventType, sainbox.ChargeOccurrenceFormedEventType)
	}
	const tenant = "tenant-a"
	beat, db, store := wiredBeat(t)
	registerTrunkAgreement(t, db, tenant)
	registerBuyTriggers(t, db, tenant)
	registerEndedTrunkSegment(t, db, tenant)
	record := registerTrunkOccurrence(t, db, store, tenant)

	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var eventID string
	if err := querier.QueryRow(t.Context(),
		`SELECT event_id FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`,
		tfpostgres.ChargeOccurrenceFormedEventType,
	).Scan(&eventID); err != nil {
		t.Fatalf("找生产方入队的那一封：%v", err)
	}

	if _, err := beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if status := outboxStatus(t, db, eventID); status != "PUBLISHED" {
		t.Fatalf("发生项信封 status = %q, want PUBLISHED；失败码 = %q", status, recordedFailureCode(t, db, eventID))
	}
	if n := inboxRowsOf(t, db, occurrenceFormedConsumer, eventID); n != 1 {
		t.Fatalf("消费门 inbox 行数 = %d, want 1", n)
	}

	var occurrence, reason, version, agreement, feeItem string
	if err := querier.QueryRow(t.Context(), `
		SELECT occurrence_id, occurrence_reason, occurrence_version, agreement_ref, fee_item
		  FROM settlement_accounting.evaluation_request
		 WHERE tenant_id = $1`, tenant,
	).Scan(&occurrence, &reason, &version, &agreement, &feeItem); err != nil {
		t.Fatalf("读 BUY 请求（应恰一份）：%v", err)
	}
	if occurrence != record.Key.Occurrence.String() || reason != "ACTUAL_FULFILLMENT" || version != "V1" ||
		agreement != trunkAgreementObject+"/v1" || feeItem != trunkFeeItem {
		t.Fatalf("BUY 请求 = %s / %s / %s / %s / %s，与登记的发生项对不上", occurrence, reason, version, agreement, feeItem)
	}
}
