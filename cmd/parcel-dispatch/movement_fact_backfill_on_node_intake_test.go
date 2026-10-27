package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	vetf "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
)

// 本文件对真实 PostgreSQL 证一件作业实物在袋出发之后才识别成功时，它随袋走过的移动事实经节点收寄形成信封补进全程追踪（票
// docs-consistency/code/74，ADR-0299），走的是生产 wireDispatcher：节点作业的单元、成员事实与收寄身份，运输履约的段与移动事实，全程
// 追踪的投影都在同一个库里各自的表上。测试输入是隔离合成，只记 `S`。

const (
	backfillOnIntakeConsumerName = "visibility-exception/backfill-movement-facts-on-node-intake"
	intakeCage                   = "SYN-CAGE-123"
	intakeMoveNested             = "SYN-MOVE-C74-NESTED"
	intakeMoveRecheck            = "SYN-MOVE-C74-RECHECK"
)

// openUnitAt 在节点作业册上开一件集运单元。
func openUnitAt(t *testing.T, fixture *synVerticalFixture, unit string, openedAt time.Time) {
	t.Helper()
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO node_operations.consolidation_unit (tenant_id, unit_id, asset_ref, phase, members, snapshots, opened_source)
		 VALUES ($1, $2, 'SYN-ASSET-' || $2, 'OPEN', '[]'::jsonb, '[]'::jsonb,
		         jsonb_build_object('sourceId', 'SYN-OPEN-' || $2, 'performedBy', 'SYN-PACKER', 'evidence', 'SYN-WORK',
		                            'occurredAt', $3::timestamptz))`,
		fixture.identity.TenantID().String(), unit, openedAt); err != nil {
		t.Fatalf("播种单元 %s：%v", unit, err)
	}
}

// memberFactAt 落一条移入或移出事实。
func memberFactAt(t *testing.T, fixture *synVerticalFixture, action, unit, member string, at time.Time) {
	t.Helper()
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO node_operations.consolidation_fact
			(tenant_id, source_id, content_digest, action, unit_id, member_id, performed_by, evidence, occurred_at, recorded_at)
		 VALUES ($1, 'SYN-C74-' || $2 || '-' || $3 || '-' || $4, 'syn-digest', $2, $3, $4, 'SYN-PACKER', 'SYN-WORK', $5, $5)`,
		fixture.identity.TenantID().String(), action, unit, member, at); err != nil {
		t.Fatalf("播种成员事实 %s %s %s：%v", action, unit, member, err)
	}
}

// formedReceptionOf 落一份识别成功的收寄：handling 这件实物认出了 parcel。交回这份收寄的来源标识。
func formedReceptionOf(t *testing.T, fixture *synVerticalFixture, handling, parcel string) string {
	t.Helper()
	source := "SYN-SCAN-C74-" + handling
	receivedAt := movementDepartedAt.Add(-4 * time.Hour)
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO node_operations.reception
			(tenant_id, source_id, content_digest, kind, intake, control, candidates, identity_conflict, service_markers,
			 service_result_check, recorded_at)
		 VALUES ($1, $5, 'syn-digest', 'INTAKE_FORMED',
		         jsonb_build_object('unit', $2::text, 'node', 'SYN-NODE-SHA-HUB', 'deliveredBy', 'SYN-COURIER',
		                            'evidence', 'SYN-EVIDENCE', 'version', 'SYN-INTAKE-' || $2,
		                            'association', $3::text, 'receivedAt', $4::timestamptz),
		         jsonb_build_object('unit', $2::text, 'node', 'SYN-NODE-SHA-HUB', 'kind', 'NODE_INTAKE',
		                            'basis', 'SYN-INTAKE-' || $2, 'establishedAt', $4::timestamptz),
		         '[]'::jsonb, false, '[]'::jsonb, 'CHECKED', $4)`,
		fixture.identity.TenantID().String(), handling, parcel, receivedAt, source); err != nil {
		t.Fatalf("播种收寄 %s：%v", handling, err)
	}
	return source
}

// enqueueNodeIntakeFormed 入队一封节点收寄形成信封，载荷与生产交出口一字不差（租户、来源标识两维）。交回事件 ID。
func enqueueNodeIntakeFormed(t *testing.T, fixture *synVerticalFixture, source string) string {
	t.Helper()
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	tenant := fixture.identity.TenantID().String()
	id := "SYN-C74-INTAKE-FORMED-" + source
	at := beatInstant()
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		return store.Enqueue(txCtx, eventing.Envelope{
			SpecVersion: eventing.SpecVersion, ID: eventing.EventID(id), Source: "idp-parcel/node-operations",
			Type: veinbox.NodeIntakeFormedEventType, Version: 1, Scope: tenant, Subject: source, PartitionKey: id,
			OccurredAt: at, RecordedAt: at, ContentType: eventing.JSONContentType,
			Payload: []byte(`{"tenantId":"` + tenant + `","sourceId":"` + source + `"}`),
		})
	})
	return id
}

// saveSegmentOf 站在 TF 侧立出发声明的那一段：objects 在出发前一小时交接入段。
func saveSegmentOf(t *testing.T, fixture *synVerticalFixture, objects ...string) {
	t.Helper()
	segments, err := tfpostgres.NewFulfillmentSegments(fixture.db)
	if err != nil {
		t.Fatalf("构造段册：%v", err)
	}
	tenant := mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String())
	reference := mustTF(t, tfdomain.NewFulfillmentSegmentReference, movementSegment)
	var participations []tfdomain.RehydrateParticipationSpec
	for _, object := range objects {
		participations = append(participations, tfdomain.RehydrateParticipationSpec{
			Object:     mustTF(t, tfdomain.NewCarriedObjectReference, object),
			EntryKind:  tfdomain.EnteredByTransportHandover,
			EntryBasis: mustTF(t, tfdomain.NewParticipationBasisReference, "SYN-HANDOVER/"+object),
			EnteredAt:  movementHandedOverAt,
		})
	}
	segment, err := tfdomain.RehydrateActualFulfillmentSegment(tfdomain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: tenant, Segment: reference, Participations: participations,
	})
	if err != nil {
		t.Fatalf("段：%v", err)
	}
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := segments.Save(txCtx, tfports.FulfillmentSegmentRecord{
			Key:        tfports.FulfillmentSegmentKey{TenantID: tenant, Segment: reference},
			Segment:    segment,
			RecordedAt: movementHandedOverAt,
		})
		if err == nil && outcome != tfports.SegmentSaved {
			t.Fatalf("段 outcome = %v", outcome)
		}
		return err
	})
}

// assertDepartureOn 核这条出发在这件包裹上恰有一份事实、一个条目，归 DEPARTED，发生时间是事实的业务发生时间。
func assertDepartureOn(t *testing.T, fixture *synVerticalFixture, parcel, fact string) {
	t.Helper()
	var departed int
	for _, entry := range projectionEntries(t, fixture, parcel) {
		if entry.Fact().Fact().String() != "movement-fact/"+parcel+"/"+fact {
			continue
		}
		departed++
		if milestone, classified := entry.Milestone(); entry.Fact().Kind().String() != "movement-departure" ||
			!classified || milestone.String() != "DEPARTED" {
			t.Fatalf("%s 的出发 = %s 归类 %q（%v），want movement-departure 归 DEPARTED", parcel, entry.Fact().Kind(), milestone, classified)
		}
		if !entry.Fact().OccurredAt().Equal(movementDepartedAt) {
			t.Fatalf("%s 的出发发生时间 = %s, want 事实的业务发生时间", parcel, entry.Fact().OccurredAt())
		}
	}
	if departed != 1 {
		t.Fatalf("%s 的出发条目 %d 个, want 1", parcel, departed)
	}
	if n := acceptedFactVersions(t, fixture, "movement-fact/"+parcel+"/"+fact); n != 1 {
		t.Fatalf("%s 这一版事实 %d 份, want 1", parcel, n)
	}
}

func assertNoDepartureOn(t *testing.T, fixture *synVerticalFixture, parcel, fact string) {
	t.Helper()
	for _, entry := range projectionEntries(t, fixture, parcel) {
		if entry.Fact().Fact().String() == "movement-fact/"+parcel+"/"+fact {
			t.Fatalf("%s 不该得到这条出发，却有它的条目", parcel)
		}
	}
	if n := acceptedFactVersions(t, fixture, "movement-fact/"+parcel+"/"+fact); n != 0 {
		t.Fatalf("%s 被派生了 %d 份", parcel, n)
	}
}

// Covers: AT-VE-218 真库半边、票面判据「识别成功后追踪上补出随袋出发」（证据 S）——笼里套着袋、袋里装着已识别的 A、B 与四件还没有收寄
// 记录的实物（一件出发时在袋里、一件出发前已移出、一件出发之后才装进、一件从没进过任何单元），段里在控的是笼；出发先经生产路由一拍：
// A、B 得到出发，没有收寄记录的一件入账不派生。之后四件实物各识别成功（收寄形成信封经生产路由一拍）：出发时在袋里的那一件在新认出的
// 包裹上补出出发，归 DEPARTED、发生时间是出发的业务时间；出发前已移出的、出发之后才装进的、从没进过单元的都不补。
// Covers: AT-VE-219 真库半边——逐层向上：候选是装它的袋与装袋的笼，补派生在笼有参与的那一段上用与正常路径同一处的展开找到它。
func TestAMemberIdentifiedAfterItsNestedBagDepartedGetsTheDepartureThroughTheProductionRoute(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	registerDemoMilestoneMapping(t, fixture)
	seedBagAroundTheDeparture(t, fixture)
	openUnitAt(t, fixture, intakeCage, movementDepartedAt.Add(-4*time.Hour))
	memberFactAt(t, fixture, "ADD_MEMBER", intakeCage, consolidationBag, movementDepartedAt.Add(-90*time.Minute))
	memberFactAt(t, fixture, "ADD_MEMBER", consolidationBag, "SYN-HU-123-GONE", movementDepartedAt.Add(-3*time.Hour))
	memberFactAt(t, fixture, "REMOVE_MEMBER", consolidationBag, "SYN-HU-123-GONE", movementDepartedAt.Add(-150*time.Minute))
	memberFactAt(t, fixture, "ADD_MEMBER", consolidationBag, "SYN-HU-123-AFTER", movementDepartedAt.Add(2*time.Minute))
	saveSegmentOf(t, fixture, intakeCage, movementParcelOnBoard)

	eventID := recordDepartureThroughProduction(t, fixture, intakeMoveNested)
	if published, err := fixture.beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("出发那一拍 published = %d err = %v；失败码 = %q", published, err, recordedFailureCode(t, fixture.db, eventID))
	}
	assertDepartedOntoBagMembers(t, fixture, intakeMoveNested)

	identified := map[string]string{
		"SYN-HU-123-UNRECEIVED": "SYN-PARCEL-C74-INBAG",
		"SYN-HU-123-GONE":       "SYN-PARCEL-C74-GONE",
		"SYN-HU-123-AFTER":      "SYN-PARCEL-C74-AFTER",
		"SYN-HU-123-NEVER":      "SYN-PARCEL-C74-NEVER",
	}
	var intakeEvents []string
	for handling, parcel := range identified {
		intakeEvents = append(intakeEvents, enqueueNodeIntakeFormed(t, fixture, formedReceptionOf(t, fixture, handling, parcel)))
	}
	if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("识别那一拍：%v", err)
	}
	for _, id := range intakeEvents {
		if n := fixture.countInbox(t, backfillOnIntakeConsumerName, id); n != 1 {
			t.Fatalf("补派生 inbox 行数 = %d, want 1——路由表没把节点收寄信封投给补派生那一路；失败码 = %q",
				n, recordedFailureCode(t, fixture.db, id))
		}
	}

	assertDepartureOn(t, fixture, "SYN-PARCEL-C74-INBAG", intakeMoveNested)
	for _, parcel := range []string{"SYN-PARCEL-C74-GONE", "SYN-PARCEL-C74-AFTER", "SYN-PARCEL-C74-NEVER"} {
		assertNoDepartureOn(t, fixture, parcel, intakeMoveNested)
	}
	assertDepartedOntoBagMembers(t, fixture, intakeMoveNested)
	for _, absent := range []string{consolidationBag, intakeCage} {
		assertNoDepartureOn(t, fixture, absent, intakeMoveNested)
	}

	// 同一封信以新 ID 再投一次、移动事实那一路以新 ID 重投（此刻它展开得到这件新认出的包裹）：都只答已有结果。
	derive, inboxStore := redeliveryParts(t, fixture)
	backfill, err := backfillMovementFactsOnNodeIntakeConsumer(fixture.db, inboxStore, derive)
	if err != nil {
		t.Fatalf("装配补派生那一路：%v", err)
	}
	tenant := fixture.identity.TenantID().String()
	source := "SYN-SCAN-C74-SYN-HU-123-UNRECEIVED"
	if err := backfill.Consume(t.Context(), redeliveryEnvelope(tenant, "redeliver-node-intake", veinbox.NodeIntakeFormedEventType, source,
		`{"tenantId":"`+tenant+`","sourceId":"`+source+`"}`)); err != nil {
		t.Fatalf("收寄形成信封重投：%v", err)
	}
	redeliverBagMovementFact(t, fixture, intakeMoveNested)
	assertDepartureOn(t, fixture, "SYN-PARCEL-C74-INBAG", intakeMoveNested)
	assertDepartedOntoBagMembers(t, fixture, intakeMoveNested)
}

// redeliverBagMovementFact 以新 ID 把同一条移动事实交给生产的移动事实那一路。
func redeliverBagMovementFact(t *testing.T, fixture *synVerticalFixture, fact string) {
	t.Helper()
	derive, inboxStore := redeliveryParts(t, fixture)
	consumer, err := deriveMovementFactConsumer(fixture.db, inboxStore, derive)
	if err != nil {
		t.Fatalf("装配移动事实那一路：%v", err)
	}
	tenant := fixture.identity.TenantID().String()
	if err := consumer.Consume(t.Context(), redeliveryEnvelope(tenant, "redeliver-"+fact, veinbox.MovementFactRecordedEventType, fact,
		`{"tenantId":"`+tenant+`","fact":"`+fact+`","version":"v1"}`)); err != nil {
		t.Fatalf("移动事实重投：%v", err)
	}
}

// Covers: AT-VE-218 真库半边（两路先后）——识别成功已落册、收寄形成信封还没到时，移动事实那一路先重投：此刻成员身份成立，它在新认出的
// 包裹上派生；随后收寄形成信封到达，补派生答已有结果。事实与条目都只有一份。
func TestTheNormalPathBeforeTheIntakeEnvelopeLeavesTheBackfillOneEntry(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	registerDemoMilestoneMapping(t, fixture)
	seedBagAroundTheDeparture(t, fixture)
	saveBagSegment(t, fixture)

	eventID := recordDepartureThroughProduction(t, fixture, consolidationMoveDispatch)
	if published, err := fixture.beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("出发那一拍 published = %d err = %v；失败码 = %q", published, err, recordedFailureCode(t, fixture.db, eventID))
	}
	source := formedReceptionOf(t, fixture, "SYN-HU-123-UNRECEIVED", "SYN-PARCEL-C74-ORDER")
	redeliverBagMovementFact(t, fixture, consolidationMoveDispatch)
	assertDepartureOn(t, fixture, "SYN-PARCEL-C74-ORDER", consolidationMoveDispatch)

	intakeEvent := enqueueNodeIntakeFormed(t, fixture, source)
	if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("收寄形成那一拍：%v", err)
	}
	if n := fixture.countInbox(t, backfillOnIntakeConsumerName, intakeEvent); n != 1 {
		t.Fatalf("补派生 inbox 行数 = %d, want 1；失败码 = %q", n, recordedFailureCode(t, fixture.db, intakeEvent))
	}
	assertDepartureOn(t, fixture, "SYN-PARCEL-C74-ORDER", consolidationMoveDispatch)
}

// Covers: AT-VE-218 真库半边（生产识别链）、票面判据「识别成功后追踪上补出随袋出发」（证据 S）——待识别的实物是袋里的成员，出发时它没有
// 可投的追踪包裹；之后 parcel-shipment 接受委托、客户包裹件号形成关联，门铃经生产派发器投给 node-operations 的重核消费者，重核追加
// 一版`节点收寄已形成`并交收寄意图一封，意图再经生产路由一拍：新认出的包裹上补出这条出发，归 DEPARTED。
func TestAPendingMemberRecognizedByTheProductionRecheckGetsTheDepartureOfItsBag(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()
	registerDemoMilestoneMapping(t, fixture)
	tenant := mustNO(t, nodomain.NewTenantID, fixture.identity.TenantID().String())
	receivedAt := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)

	scopeBasis := preAdviseForDispatch(t, fixture.db, tenant, fixture.identity.CustomerAccountID().String(), recheckMark, receivedAt)
	pending := receivePendingForRecheck(t, fixture.db, tenant, receivedAt, scopeBasis)
	seedBagAroundTheDeparture(t, fixture)
	memberFactAt(t, fixture, "ADD_MEMBER", consolidationBag, recheckUnit, movementDepartedAt.Add(-2*time.Hour))
	saveBagSegment(t, fixture)

	eventID := recordDepartureThroughProduction(t, fixture, intakeMoveRecheck)
	if published, err := fixture.beat.DispatchOnce(ctx); err != nil || published != 1 {
		t.Fatalf("出发那一拍 published = %d err = %v；失败码 = %q", published, err, recordedFailureCode(t, fixture.db, eventID))
	}
	assertDepartedOntoBagMembers(t, fixture, intakeMoveRecheck)

	submitWithCustomerParcelNumber(t, fixture, recheckMark)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("门铃那一拍：%v", err)
	}
	intents := intakeIntentSources(t, fixture.db)
	if want := "IDENTITY-RECHECK/" + pending.JudgmentVersion.String(); len(intents) != 1 || intents[0] != want {
		t.Fatalf("收寄意图 = %v, want 恰一封指回重核追加版 %s", intents, want)
	}
	assertNoDepartureOn(t, fixture, "SYN-PARCEL-01", intakeMoveRecheck)

	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("收寄意图那一拍：%v", err)
	}
	intentEvents := fixture.outboxIDsOfType(t, nodeIntakeFormedType)
	if len(intentEvents) != 1 {
		t.Fatalf("收寄意图 %d 封, want 1", len(intentEvents))
	}
	if n := fixture.countInbox(t, backfillOnIntakeConsumerName, intentEvents[0]); n != 1 {
		t.Fatalf("补派生 inbox 行数 = %d, want 1；失败码 = %q", n, recordedFailureCode(t, fixture.db, intentEvents[0]))
	}
	assertDepartureOn(t, fixture, "SYN-PARCEL-01", intakeMoveRecheck)
	assertDepartedOntoBagMembers(t, fixture, intakeMoveRecheck)
}

// Covers: AT-VE-221 派发半边——节点收寄信封上补派生的失败分格：收寄读不回、曾装它的单元或单元成员读不动、对象的段读不动、按段列事实读不动、
// 派生编排未决落 dispatch.consumer_undecided，其余保持 dispatch.publish_failed。名单取本路的哨兵变量本身，理由同交接那一份。
func TestMovementFactBackfillOnNodeIntakeFailuresLandInTheRightPartition(t *testing.T) {
	for _, test := range []struct {
		name     string
		err      error
		wantCode string
	}{
		{"收寄读不回", vetf.ErrFormedIntakeNotVisible, "dispatch.consumer_undecided"},
		{"集运成员读不动", vetf.ErrConsolidationMembershipNotVisible, "dispatch.consumer_undecided"},
		{"对象的段读不动", vetf.ErrObjectSegmentsNotVisible, "dispatch.consumer_undecided"},
		{"按段列事实读不动", vetf.ErrMovementFactsOfSegmentNotVisible, "dispatch.consumer_undecided"},
		{"派生编排未决", vetf.ErrProjectionUndecided, "dispatch.consumer_undecided"},
		{"收寄记录不一致", vetf.ErrFormedIntakeInconsistent, "dispatch.publish_failed"},
		{"集运成员答复不一致", vetf.ErrConsolidationMembershipInconsistent, "dispatch.publish_failed"},
		{"段册或事实不变量已破", vetf.ErrMovementFactRecordInconsistent, "dispatch.publish_failed"},
		{"事实引用译不出", vetf.ErrMovementFactUntranslatableAnswer, "dispatch.publish_failed"},
		{"投影意图待发", vetf.ErrProjectionHandoffPending, "dispatch.publish_failed"},
		{"投影封闭集合外", veconsume.ErrUnexpectedProjectionOutcome, "dispatch.publish_failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			beat, db, store := beatWithSentinelledConsumer(t, &stallingConsumer{err: test.err},
				veinbox.NodeIntakeFormedEventType, veMovementFactBackfillOnNodeIntakeUndecidedSentinels)
			enqueueForBeat(t, db, store, "intake-formed-1", veinbox.NodeIntakeFormedEventType, `{}`)

			published, err := beat.DispatchOnce(t.Context())
			if err != nil {
				t.Fatalf("一拍：%v", err)
			}
			if published != 0 {
				t.Fatalf("失败的投递被定稿了 %d 条", published)
			}
			if got := recordedFailureCode(t, db, "intake-formed-1"); got != test.wantCode {
				t.Fatalf("failure_code = %q, want %q", got, test.wantCode)
			}
		})
	}
}
