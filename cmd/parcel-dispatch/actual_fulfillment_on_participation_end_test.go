package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	tfpartycommercial "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/partycommercial"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// 本文件对真实 PostgreSQL 证实际履约发生项随交接与有效交付信封自动形成（ADR-0218）。干线协议经 PC 真发布（实际履约按件计），
// 委托照提交口落下的形状落册（记着协议版本），段由生产交接登记编排立起、参与由生产编排结束，采用走真采用编排，信封经生产
// 交出口入队；派发进程照生产路由表一拍。只在「读不动」那一条用例里把协议核对口换成可开关的替身。夹具全部合成，只记 `S`。

const (
	actualOnHandoverConsumerName = "transport-fulfillment/form-actual-fulfillment-occurrence-from-transport-handover"
	actualOnDeliveryConsumerName = "transport-fulfillment/form-actual-fulfillment-occurrence-from-effective-delivery"
	formationTenant              = "SYN-TENANT-01"
	formationSegment             = "SYN-SEG-SZX-SIN-DCG67"
	formationCommission          = "SYN-COMMISSION-TRUNK-DCG67"
	formationScope               = "SYN-HO-DCG67"
)

// formationAt 把业务时刻钉在刚过去的真实时间上：派发一拍按真实时钟认领，入队时刻写成未来会一封都取不到。
func formationAt(offset time.Duration) time.Time {
	return beatInstant().Add(-4 * time.Hour).Add(offset).Truncate(time.Second)
}

// saveTrunkCommission 照提交口落下的形状落一份干线委托：指名 v1、记着那一版，成员是给定的包裹。不经提交口：那一口会另交一封
// 委托信封，本进程没有它的消费者（transport_commission_unrouted_test.go 钉着），与本文件要证的无关。
func saveTrunkCommission(t *testing.T, db *bentopg.DB, members ...string) {
	t.Helper()
	objects := make([]tfdomain.CarriedObjectReference, 0, len(members))
	for _, member := range members {
		objects = append(objects, mustTF(t, tfdomain.NewCarriedObjectReference, member))
	}
	tenant := mustTF(t, tfdomain.NewTenantID, formationTenant)
	agreement, err := tfdomain.NewSupplierAgreementVersion(trunkAgreementObject, "v1")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tfdomain.DraftTransportCommission(tfdomain.TransportCommissionDraftSpec{
		TenantID:       tenant,
		Commission:     mustTF(t, tfdomain.NewTransportCommissionReference, formationCommission),
		Provider:       mustTF(t, tfdomain.NewServiceProviderReference, "SYN-PARTY-CARRIER-TRUNK-01"),
		Agreement:      agreement,
		Conditions:     mustTF(t, tfdomain.NewConditionsSnapshotReference, "SYN-CONDITIONS-TRUNK-01"),
		Role:           mustTF(t, tfdomain.NewRoleSnapshotReference, "SYN-ROLE-TRUNK-01"),
		Responsibility: mustTF(t, tfdomain.NewResponsibilitySnapshotReference, "SYN-RESPONSIBILITY-TRUNK-01"),
		Members:        objects,
		SubmittedAt:    formationAt(0),
	})
	if err != nil {
		t.Fatalf("委托草稿：%v", err)
	}
	commission, err := draft.Submit(
		mustTF(t, tfdomain.NewAgreementSnapshotReference, trunkAgreementObject+"/v1"),
		mustTF(t, tfdomain.NewProcurementLegalEntityReference, "SYN-LE-01"))
	if err != nil {
		t.Fatalf("委托：%v", err)
	}
	commissions, err := tfpostgres.NewTransportCommissions(db)
	if err != nil {
		t.Fatal(err)
	}
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		saved, err := commissions.Save(txCtx, tfports.TransportCommissionRecord{
			Key:           tfports.TransportCommissionKey{TenantID: tenant, Commission: commission.Commission()},
			ContentDigest: "TFC-1:synthetic-dcg67",
			Commission:    commission,
			RecordedAt:    formationAt(0),
		})
		if err == nil && saved != tfports.CommissionSaved {
			err = errors.New("commission was not saved fresh")
		}
		return err
	})
}

// handOverAtSZX 用生产交接登记编排把包裹交给干线承运方、立干线段；handOverAtSIN 在 SIN 交出、结束那一条参与。两次的版本
// 各自写短：交接信封的 ID 仍是四维直拼。
func handOverAtSZX(t *testing.T, register *tfapplication.RegisterTransportHandoverHandler, db *bentopg.DB, parcel string) {
	t.Helper()
	registerFormationHandover(t, register, db, tfapplication.RegisterTransportHandoverCommand{
		Object: parcel, ReleasedBy: "SYN-NODE-SZX-GATE", ReceivedBy: "SYN-PARTY-CARRIER-TRUNK-01",
		Version: "SZX-" + parcel, JudgedAt: formationAt(time.Hour),
		Segment: formationSegment, SegmentServiceAction: tfdomain.SegmentServesLinehaul.String(),
	})
}

func handOverAtSIN(t *testing.T, register *tfapplication.RegisterTransportHandoverHandler, db *bentopg.DB, parcel string) string {
	t.Helper()
	registerFormationHandover(t, register, db, tfapplication.RegisterTransportHandoverCommand{
		Object: parcel, ReleasedBy: "SYN-PARTY-CARRIER-TRUNK-01", ReceivedBy: "SYN-NODE-SIN-HUB",
		Version: "SIN-" + parcel, JudgedAt: formationAt(3 * time.Hour),
	})
	return formationTenant + "/" + parcel + "/" + formationScope + "/SIN-" + parcel
}

func registerFormationHandover(
	t *testing.T,
	register *tfapplication.RegisterTransportHandoverHandler,
	db *bentopg.DB,
	command tfapplication.RegisterTransportHandoverCommand,
) {
	t.Helper()
	command.TenantID = mustTF(t, tfdomain.NewTenantID, formationTenant)
	command.Scope = formationScope
	command.Verdict = tfdomain.ObjectHandedOver
	command.ReleasingEvidence = "SYN-EVIDENCE-RELEASE-DCG67"
	command.ReceivingEvidence = "SYN-EVIDENCE-RECEIVE-DCG67"
	command.Rule = "SYN-HANDOVER-RULE/v1"
	var result tfapplication.RegisterTransportHandoverResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = register.Register(txCtx, command)
		return err
	})
	if result.Outcome() != tfapplication.HandoverRegistered || result.SegmentContinuationReference() != "" {
		t.Fatalf("登记交接 %s：outcome=%s debt=%q", command.Version, result.Outcome(), result.SegmentContinuationReference())
	}
}

// adoptTrunkCommission 走真采用编排：段在册、委托在册、服务方等于委托的服务方。
func adoptTrunkCommission(t *testing.T, db *bentopg.DB) {
	t.Helper()
	adoptions, err := tfpostgres.NewSegmentCommissionAdoptions(db)
	if err != nil {
		t.Fatal(err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatal(err)
	}
	commissions, err := tfpostgres.NewTransportCommissions(db)
	if err != nil {
		t.Fatal(err)
	}
	adopter := tfapplication.NewAdoptSegmentCommissionHandler(tfapplication.AdoptSegmentCommissionDeps{
		Adoptions: adoptions, Segments: segments, Commissions: commissions, StartAdvancement: commissions, Clock: systemClock{},
	})
	var adopted tfapplication.CommissionAdoptionResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		adopted, err = adopter.Adopt(txCtx, tfapplication.AdoptSegmentCommissionCommand{
			TenantID:   mustTF(t, tfdomain.NewTenantID, formationTenant),
			Segment:    formationSegment,
			Commission: formationCommission,
			Provider:   "SYN-PARTY-CARRIER-TRUNK-01",
		})
		return err
	})
	if adopted.Outcome() != tfapplication.CommissionAdopted {
		t.Fatalf("采用：%s", adopted.Outcome())
	}
}

type formationOccurrenceRow struct {
	occurrence, journey, registeredBy, provider, agreement, unit string
	members                                                      []string
}

func formationOccurrences(t *testing.T, db *bentopg.DB) []formationOccurrenceRow {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	rows, err := querier.Query(t.Context(), `
		SELECT o.occurrence_ref, o.journey_ref, o.registered_by, o.provider_ref, o.agreement_ref, o.unit_ref,
		       ARRAY(SELECT m.object_ref FROM transport_fulfillment.transport_charge_occurrence_member m
		              WHERE m.tenant_id = o.tenant_id AND m.occurrence_ref = o.occurrence_ref
		                AND m.validity_version = o.validity_version ORDER BY m.object_ref)
		  FROM transport_fulfillment.transport_charge_occurrence o
		 WHERE o.tenant_id = $1 AND o.reason = 'ACTUAL_FULFILLMENT'
		 ORDER BY o.occurrence_ref`, formationTenant)
	if err != nil {
		t.Fatalf("读发生项：%v", err)
	}
	defer rows.Close()
	var found []formationOccurrenceRow
	for rows.Next() {
		var row formationOccurrenceRow
		if err := rows.Scan(&row.occurrence, &row.journey, &row.registeredBy, &row.provider, &row.agreement, &row.unit, &row.members); err != nil {
			t.Fatalf("读发生项行：%v", err)
		}
		found = append(found, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return found
}

// Covers: AT-TF-246、AT-TF-250（S，票 demo-chain-gaps/67 演示判据的真库半边，ADR-0218）——干线段采用了委托之后在 SIN 交接：派发第一拍
// 交接信封扇出到本路，实际履约发生项经既有登记用例形成（成员是这件包裹、旅程 `JRN/<包裹>`、登记者是本上下文的系统身份、
// 采购上下文取委托、按件计），已形成信封同笔入队；第二拍结算发起恰一份 BUY 请求——无需人工登记。此后人工按同一身份登记答
// `已存在`。两次交接认的事件类型与 VE 那一路相同，信封定稿 PUBLISHED。
func TestAHandoverAtTheEndOfAnAdoptedTrunkSegmentFormsTheOccurrenceAndFiresBuy(t *testing.T) {
	if tfinbox.TransportHandoverRegisteredEventType != veinbox.TransportHandoverRegisteredEventType {
		t.Fatalf("本路认 %q、VE 认 %q", tfinbox.TransportHandoverRegisteredEventType, veinbox.TransportHandoverRegisteredEventType)
	}
	const parcel = "SYN-PARCEL-DCG67-01"
	beat, db, store := wiredBeat(t)
	registerTrunkAgreement(t, db, formationTenant)
	registerBuyTriggers(t, db, formationTenant)
	saveTrunkCommission(t, db, parcel)
	register := newHandoverRegistration(t, db, store)
	handOverAtSZX(t, register, db, parcel)
	adoptTrunkCommission(t, db)
	eventID := handOverAtSIN(t, register, db, parcel)

	// 两次交接同一分区（租户/对象），SZX 那一封在前：一拍认领一个分区的队头，所以 SIN 那一封在第二拍。
	dispatchUntilPublished(t, beat, db, eventID)
	if n := inboxRowsOf(t, db, actualOnHandoverConsumerName, eventID); n != 1 {
		t.Fatalf("本路 inbox 行 = %d, want 1——FanOut 少了这一路", n)
	}
	occurrences := formationOccurrences(t, db)
	if len(occurrences) != 1 {
		t.Fatalf("发生项 %d 项, want 1：%+v", len(occurrences), occurrences)
	}
	formed := occurrences[0]
	if formed.journey != "JRN/"+parcel || formed.registeredBy != tfapplication.ActualFulfillmentFormationRegistrant ||
		formed.provider != "SYN-PARTY-CARRIER-TRUNK-01" || formed.agreement != trunkAgreementObject+"/v1" ||
		formed.unit != "PIECE" || len(formed.members) != 1 || formed.members[0] != parcel {
		t.Fatalf("自动形成的发生项 = %+v", formed)
	}

	if _, err := beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("发生项信封那一拍：%v", err)
	}
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var requests int
	var requested string
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*), max(occurrence_id) FROM settlement_accounting.evaluation_request WHERE tenant_id = $1`,
		formationTenant).Scan(&requests, &requested); err != nil || requests != 1 || requested != formed.occurrence {
		t.Fatalf("BUY 请求 %d 份、指向 %q（%v）, want 恰一份、指向自动形成的那一项 %s", requests, requested, err, formed.occurrence)
	}

	manual := manualRegistration(t, db, store, nil)
	var again tfapplication.RegisterChargeOccurrenceResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		again, err = manual.Register(txCtx, tfapplication.RegisterChargeOccurrenceCommand{
			TenantID:   mustTF(t, tfdomain.NewTenantID, formationTenant),
			Registrant: "https://dex.syn.example#SYN-OPERATOR-CONFIG-01",
			Reason:     tfdomain.ActualFulfillmentOccurrence,
			Segment:    formationSegment,
			Objects:    []string{parcel},
			Journey:    "JRN/" + parcel,
		})
		return err
	})
	if again.Outcome() != tfapplication.ChargeOccurrenceExisting {
		t.Fatalf("人工同身份重登 = %s/%s, want EXISTING", again.Outcome(), again.Detail())
	}
}

// dispatchUntilPublished 一拍一拍地跑，直到那一封定稿；跑满几拍还没定稿就红，带上失败码。
func dispatchUntilPublished(t *testing.T, beat Beat, db *bentopg.DB, eventID string) {
	t.Helper()
	for range 3 {
		if _, err := beat.DispatchOnce(t.Context()); err != nil {
			t.Fatalf("一拍：%v", err)
		}
		if outboxStatus(t, db, eventID) == "PUBLISHED" {
			return
		}
	}
	t.Fatalf("%s 跑满三拍仍未定稿：status = %s，失败码 = %q", eventID, outboxStatus(t, db, eventID), recordedFailureCode(t, db, eventID))
}

// manualRegistration 是人工登记那一口背后的同一个用例，接同一组真登记册；agreements 为空时接 PC 真点读口。
func manualRegistration(
	t *testing.T, db *bentopg.DB, store *outbox.Store, agreements tfports.SupplierAgreementSource,
) *tfapplication.RegisterChargeOccurrenceHandler {
	t.Helper()
	if agreements == nil {
		versions, err := pcpostgres.NewSupplierAgreementContents(db)
		if err != nil {
			t.Fatal(err)
		}
		if agreements, err = tfpartycommercial.NewSupplierAgreementSource(versions); err != nil {
			t.Fatal(err)
		}
	}
	segments, _ := tfpostgres.NewFulfillmentSegments(db)
	judgments, _ := tfpostgres.NewActualCarrierJudgments(db)
	adoptions, _ := tfpostgres.NewSegmentCommissionAdoptions(db)
	commissions, _ := tfpostgres.NewTransportCommissions(db)
	occurrences, _ := tfpostgres.NewChargeOccurrences(db)
	handoff, err := tfpostgres.NewOutboxChargeOccurrenceHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	return tfapplication.NewRegisterChargeOccurrenceHandler(tfapplication.RegisterChargeOccurrenceDeps{
		Segments: segments, CarrierJudgments: judgments, Adoptions: adoptions, Commissions: commissions,
		Agreements: agreements, Occurrences: occurrences, Handoff: handoff, Clock: systemClock{},
	})
}

// Covers: AT-TF-247（S，ADR-0218）——同一干线段的参与由有效交付结束：交付信封经生产交出口入队，派发一拍后本路记一行 inbox，
// 发生项形成，业务时间是交付时刻。认的事件类型与 PS、VE 两路相同。PS 终局那一路在这份夹具上停在自己的墙前，与本路无关：
// FanOut 每一路都调，本路的入账不被它卡住。
func TestAnEffectiveDeliveryEndingAnAdoptedSegmentsParticipationFormsTheOccurrence(t *testing.T) {
	for _, other := range []eventing.EventType{psinbox.EffectiveDeliveryRegisteredEventType, veinbox.EffectiveDeliveryRegisteredEventType} {
		if other != tfinbox.EffectiveDeliveryRegisteredEventType {
			t.Fatalf("本路认 %q、另一路认 %q", tfinbox.EffectiveDeliveryRegisteredEventType, other)
		}
	}
	const parcel = "SYN-PARCEL-DCG67-02"
	beat, db, store := wiredBeat(t)
	registerTrunkAgreement(t, db, formationTenant)
	saveTrunkCommission(t, db, parcel)
	handOverAtSZX(t, newHandoverRegistration(t, db, store), db, parcel)
	adoptTrunkCommission(t, db)
	deliveredAt := formationAt(3 * time.Hour)
	eventID := deliverAndEndParticipation(t, db, store, parcel, deliveredAt)

	for range 2 {
		if _, err := beat.DispatchOnce(t.Context()); err != nil {
			t.Fatalf("一拍：%v", err)
		}
	}
	if n := inboxRowsOf(t, db, actualOnDeliveryConsumerName, eventID); n != 1 {
		t.Fatalf("本路 inbox 行 = %d, want 1；失败码 = %q", n, recordedFailureCode(t, db, eventID))
	}
	occurrences := formationOccurrences(t, db)
	if len(occurrences) != 1 || occurrences[0].journey != "JRN/"+parcel ||
		occurrences[0].registeredBy != tfapplication.ActualFulfillmentFormationRegistrant {
		t.Fatalf("交付结束参与之后的发生项 = %+v", occurrences)
	}
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var occurredAt time.Time
	if err := querier.QueryRow(t.Context(),
		`SELECT occurred_at FROM transport_fulfillment.transport_charge_occurrence WHERE tenant_id = $1`, formationTenant,
	).Scan(&occurredAt); err != nil || !occurredAt.Equal(deliveredAt) {
		t.Fatalf("业务时间 = %s %v, want 交付时刻 %s", occurredAt, err, deliveredAt)
	}
}

// deliverAndEndParticipation 照交付登记编排的那一笔落下：交付行、交付信封（生产交出口），并由生产结束参与编排按交付结束那一条
// 参与。不经交付登记编排：那一口要先有派送尝试，本文件证的不是它。
func deliverAndEndParticipation(t *testing.T, db *bentopg.DB, store *outbox.Store, parcel string, at time.Time) string {
	t.Helper()
	deliveries, err := tfpostgres.NewEffectiveDeliveries(db)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := tfpostgres.NewOutboxEffectiveDeliveryHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	segments, _ := tfpostgres.NewFulfillmentSegments(db)
	handovers, _ := tfpostgres.NewTransportHandovers(db)
	ender := tfapplication.NewEndFulfillmentParticipationHandler(tfapplication.EndFulfillmentParticipationDeps{
		Segments: segments, Handovers: handovers, Deliveries: deliveries, Clock: systemClock{},
	})
	tenant := mustTF(t, tfdomain.NewTenantID, formationTenant)
	object := mustTF(t, tfdomain.NewCarriedObjectReference, parcel)
	attempt := mustTF(t, tfdomain.NewAttemptReference, "SYN-ATTEMPT-DCG67")
	delivery, err := tfdomain.RehydrateEffectiveDelivery(tfdomain.RehydrateEffectiveDeliverySpec{
		TenantID:   tenant,
		Object:     object,
		Attempt:    attempt,
		Place:      mustTF(t, tfdomain.NewAttemptPlaceReference, "SYN-DOOR-DCG67"),
		Method:     mustTF(t, tfdomain.NewDeliveryMethodReference, "SYN-METHOD-01"),
		Recipient:  mustTF(t, tfdomain.NewReceivingPartyReference, "SYN-RECIPIENT-01"),
		Proof:      mustTF(t, tfdomain.NewDeliveryProofReference, "SYN-POD-DCG67"),
		Version:    mustTF(t, tfdomain.NewDeliveryResultVersion, "SYN-DRV-DCG67"),
		OccurredAt: at,
	})
	if err != nil {
		t.Fatalf("交付：%v", err)
	}
	record := tfports.EffectiveDeliveryRecord{
		Key:           tfports.EffectiveDeliveryKey{TenantID: tenant, Object: object, Attempt: attempt},
		ContentDigest: "SYN-DELIVERY-DIGEST-DCG67",
		Delivery:      delivery,
		RecordedAt:    at.Add(time.Second),
	}
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		if _, err := deliveries.Save(txCtx, record); err != nil {
			return err
		}
		if err := handoff.HandOffEffectiveDelivery(txCtx, tfports.EffectiveDeliveryHandoffIntent{Record: record}); err != nil {
			return err
		}
		ended, err := ender.End(txCtx, tfapplication.EndFulfillmentParticipationCommand{
			TenantID: tenant, Object: parcel, Source: tfapplication.ParticipationEndedByDelivery, Attempt: "SYN-ATTEMPT-DCG67",
		})
		if err == nil && ended.Outcome() != tfapplication.ParticipationEndedNow {
			err = errors.New("participation was not ended by the delivery: " + ended.Outcome().String())
		}
		return err
	})
	return formationTenant + "/" + parcel + "/SYN-ATTEMPT-DCG67/SYN-DRV-DCG67/effective-delivery"
}

// switchableAgreements 是可开关的协议核对口：关着时读不动，开着时转给 PC 真点读口。
type switchableAgreements struct {
	inner  tfports.SupplierAgreementSource
	broken bool
}

func (source *switchableAgreements) CheckSupplierAgreement(
	ctx context.Context, tenant tfdomain.TenantID, agreement tfdomain.SupplierAgreementVersion,
	provider tfdomain.ServiceProviderReference, at time.Time,
) (tfports.SupplierAgreementCheck, error) {
	if source.broken {
		return tfports.SupplierAgreementCheck{}, errors.New("party-commercial unavailable")
	}
	return source.inner.CheckSupplierAgreement(ctx, tenant, agreement, provider, at)
}

// formationLane 接一条与生产同形的交接那一路（同一个 Over、同一份哨兵名单），只把协议核对口换成可开关的替身。
func formationLane(t *testing.T, db *bentopg.DB, store *outbox.Store) (dispatch.Consumer, *switchableAgreements) {
	t.Helper()
	versions, err := pcpostgres.NewSupplierAgreementContents(db)
	if err != nil {
		t.Fatal(err)
	}
	real, err := tfpartycommercial.NewSupplierAgreementSource(versions)
	if err != nil {
		t.Fatal(err)
	}
	agreements := &switchableAgreements{inner: real}
	former, err := actualFulfillmentFormerOver(db, store, systemClock{}, agreements)
	if err != nil {
		t.Fatalf("装配自动形成：%v", err)
	}
	inboxStore, err := inbox.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	consumer, err := tfinbox.NewActualFulfillmentOnHandoverConsumer(db.Transactor(), inboxStore, former)
	if err != nil {
		t.Fatal(err)
	}
	lane, err := dispatch.WithUndecidedSentinels(consumer, actualFulfillmentOnHandoverUndecidedSentinels...)
	if err != nil {
		t.Fatal(err)
	}
	return lane, agreements
}

func formationHandoverEnvelope(parcel string) eventing.Envelope {
	at := beatInstant()
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(formationTenant + "/" + parcel + "/" + formationScope + "/SIN-" + parcel),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         tfinbox.TransportHandoverRegisteredEventType,
		Version:      1,
		Scope:        formationTenant,
		Subject:      parcel,
		PartitionKey: formationTenant + "/" + parcel + "/transport-handover-registration",
		OccurredAt:   at,
		RecordedAt:   at,
		ContentType:  eventing.JSONContentType,
		Payload: []byte(`{"tenantId":"` + formationTenant + `","object":"` + parcel + `","scope":"` + formationScope +
			`","version":"SIN-` + parcel + `"}`),
	}
}

func migratedFormationDB(t *testing.T) (*bentopg.DB, *outbox.Store) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	return db, store
}

// Covers: AT-TF-253（S，ADR-0218）——协议核对口读不动：错误译成派发的未决、留着本路的哨兵，inbox 不落行、发生项与已形成信封
// 一行都不留（整笔回滚）；口恢复后重投，形成一项。
func TestAnUnreadableAgreementCheckIsRetriedAndLeavesNothingBehind(t *testing.T) {
	const parcel = "SYN-PARCEL-DCG67-03"
	db, store := migratedFormationDB(t)
	registerTrunkAgreement(t, db, formationTenant)
	saveTrunkCommission(t, db, parcel)
	register := newHandoverRegistration(t, db, store)
	handOverAtSZX(t, register, db, parcel)
	adoptTrunkCommission(t, db)
	handOverAtSIN(t, register, db, parcel)
	lane, agreements := formationLane(t, db, store)

	agreements.broken = true
	envelope := formationHandoverEnvelope(parcel)
	err := lane.Consume(t.Context(), envelope)
	if !errors.Is(err, dispatch.ErrConsumerUndecided) || !errors.Is(err, tfinbox.ErrActualFulfillmentFormationUndecided) {
		t.Fatalf("协议核对口读不动 = %v, want 派发的未决并留着本路的哨兵", err)
	}
	if n := inboxRowsOf(t, db, actualOnHandoverConsumerName, string(envelope.ID)); n != 0 {
		t.Fatalf("未决却入了账：inbox 行 = %d", n)
	}
	if n := len(formationOccurrences(t, db)); n != 0 {
		t.Fatalf("未决却留下了 %d 项发生项", n)
	}

	agreements.broken = false
	if err := lane.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("口恢复后重投：%v", err)
	}
	if n := len(formationOccurrences(t, db)); n != 1 {
		t.Fatalf("恢复后发生项 = %d 项, want 1", n)
	}
}

// Covers: AT-TF-248（S，ADR-0218）——段没有采用委托：本路入账，零写入；那一段的发生项照旧要人工登记。
func TestAnUnadoptedSegmentSettlesWithoutAnOccurrence(t *testing.T) {
	const parcel = "SYN-PARCEL-DCG67-04"
	db, store := migratedFormationDB(t)
	registerTrunkAgreement(t, db, formationTenant)
	register := newHandoverRegistration(t, db, store)
	handOverAtSZX(t, register, db, parcel)
	handOverAtSIN(t, register, db, parcel)
	lane, _ := formationLane(t, db, store)

	envelope := formationHandoverEnvelope(parcel)
	if err := lane.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("没有采用连线应入账：%v", err)
	}
	if n := inboxRowsOf(t, db, actualOnHandoverConsumerName, string(envelope.ID)); n != 1 {
		t.Fatalf("inbox 行 = %d, want 1", n)
	}
	if n := len(formationOccurrences(t, db)); n != 0 {
		t.Fatalf("没有采用连线却形成了 %d 项", n)
	}
}
