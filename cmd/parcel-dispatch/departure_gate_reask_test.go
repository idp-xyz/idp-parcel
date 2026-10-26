package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	vetf "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

// 本文件证出发门禁补问走的是生产 wireDispatcher（票 demo-chain-gaps/121，ADR-0305）：出发先于段登记、首版读数停在本侧；对象随后凭
// `已交接`进段，交接那一封上的补问经生产装配问真关务（判断在关务自己的事务里记下，补问那一步在消费门事务之外），追加一版读数并交
// 一封补问信封；全程追踪消费那一封，为补问读数里阻断的那一件形成出发门禁阻断信号，因果条件与依据都是那条出发。另证补问与补问信号
// 两路的失败分区与路由。证据只记 S。

const (
	departureGateReaskOnHandoverConsumerName = "transport-fulfillment/reask-departure-gates-on-transport-handover"
	departureGateReaskSignalConsumerName     = "visibility-exception/raise-departure-gate-signal-from-reask"
	reaskFact                                = "SYN-MOVE-REASK-01"
	reaskSchedule                            = "SYN-SCHED-REASK-SHA-SZX"
)

// Covers: 生产方与消费方类型串相等——补问信封 TF 交出口自写一份、VE 消费门自写一份；TF 补问那两扇门认的交接与揽收两串与 VE 各路
// 认的同一串。两串不等时生产方的每封信都撞无订阅者，或补问那一路永远收不到信。
func TestTheDepartureGateReaskEnvelopeTypesAgreeOnBothSides(t *testing.T) {
	if tfpostgres.DepartureGateReaskRecordedEventType != string(veinbox.DepartureGateReaskRecordedEventType) {
		t.Fatalf("补问信封：生产方 %q / 消费方 %q", tfpostgres.DepartureGateReaskRecordedEventType, veinbox.DepartureGateReaskRecordedEventType)
	}
	if tfinbox.OffsitePickupRegisteredEventType != veinbox.OffsitePickupRegisteredEventType ||
		tfinbox.TransportHandoverRegisteredEventType != veinbox.TransportHandoverRegisteredEventType {
		t.Fatalf("控制事实信封：TF 补问门认 %q / %q，VE 认 %q / %q", tfinbox.OffsitePickupRegisteredEventType,
			tfinbox.TransportHandoverRegisteredEventType, veinbox.OffsitePickupRegisteredEventType, veinbox.TransportHandoverRegisteredEventType)
	}
}

// Covers: AT-TF-343 真库半边、AT-VE-205 真库半边、票面判据「先于段登记、本该阻断的出发补问之后形成出发门禁阻断信号」（证据 S）——出发
// 先于段登记，首版读数停在「段不在册」，移动事实那一路入账、不形成信号；在控那一件随后经生产登记编排交接入段，生产路由一拍：交接那一封
// 上的补问问真关务（那一件有一条约束装载出发的限制），追加第 1 次补问、读数阻断，首版不改；再一拍：补问信封上的信号那一路为它开一期，
// 因果条件与依据是那条出发、事实版本 v1、判断标识取补问读数里那一格，进人工复核。交接那一封以新 ID 重投到补问那一路：它已有定论，不再
// 补问。没交接入段的那一件没有补问。
func TestABlockedDepartureRecordedBeforeItsSegmentIsReaskedAndSignalledThroughTheProductionRoute(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	registerDemoDepartureGateSignalRule(t, fixture)
	saveReaskSchedule(t, fixture)
	restrictLoadingDepartureOf(t, fixture, movementParcelOnBoard)

	factEvent := recordStoppedDepartureThroughProduction(t, fixture)
	if published, err := fixture.beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("出发那一拍 published = %d err = %v；失败码 = %q", published, err, recordedFailureCode(t, fixture.db, factEvent))
	}
	if n := signalEpisodes(t, fixture, movementParcelOnBoard); n != 0 {
		t.Fatalf("段不在册时已形成 %d 期——前提不成立", n)
	}

	handoverEvent := registerHandoverIntoMovementSegment(t, fixture)
	if published, err := fixture.beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("交接那一拍 published = %d err = %v；失败码 = %q", published, err, recordedFailureCode(t, fixture.db, handoverEvent))
	}
	if n := fixture.countInbox(t, departureGateReaskOnHandoverConsumerName, handoverEvent); n != 1 {
		t.Fatalf("补问 inbox 行数 = %d, want 1——路由表没把交接信封投给补问那一路", n)
	}
	reasks := departureGateReasksOf(t, fixture)
	if len(reasks) != 1 {
		t.Fatalf("补问 %d 次, want 1", len(reasks))
	}
	reask := reasks[0]
	findings := reask.Reading.Findings()
	if reask.Key.Object.String() != movementParcelOnBoard || reask.Key.Sequence != 1 ||
		reask.Reading.Answer() != tfdomain.DepartureGateBlocked || len(findings) != 1 {
		t.Fatalf("补问 = %+v %s %d 格", reask.Key, reask.Reading.Answer(), len(findings))
	}
	judgement, judged := findings[0].Judgement()
	if !judged || findings[0].Bases()[0].Cause != "RESTRICTION_IN_FORCE" {
		t.Fatalf("补问那一格 = %+v——要是真关务按那条限制答的阻断", findings[0].Bases())
	}
	if first := departureFirstReading(t, fixture); first.Answer() != tfdomain.DepartureGateUndecided ||
		first.Stops()[0] != tfdomain.DepartureSegmentNotRegistered {
		t.Fatalf("首版读数被改成了 %s %v", first.Answer(), first.Stops())
	}

	if published, err := fixture.beat.DispatchOnce(t.Context()); err != nil || published < 1 {
		t.Fatalf("补问信封那一拍 published = %d err = %v", published, err)
	}
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("读执行器：%v", err)
	}
	var cause, fact, version, basisJudgement, outcome string
	var hits int
	if err := querier.QueryRow(t.Context(),
		`SELECT e.causal_condition, e.fact_basis->0->>'fact', e.fact_basis->0->>'version', e.fact_basis->0->>'judgement', e.hits,
		        c.outcome
		   FROM visibility_exception.signal_episode e
		   JOIN visibility_exception.triage_conclusion c ON c.episode_id = e.episode_id
		  WHERE e.tenant_id = $1 AND e.parcel_ref = $2`,
		fixture.identity.TenantID().String(), movementParcelOnBoard,
	).Scan(&cause, &fact, &version, &basisJudgement, &hits, &outcome); err != nil {
		t.Fatalf("读补问形成的发作期：%v", err)
	}
	want := "movement-fact/" + movementParcelOnBoard + "/" + reaskFact
	if cause != "source-fact/TRANSPORT_FULFILLMENT/"+want || fact != want || version != "v1" || basisJudgement != judgement ||
		hits != 1 || outcome != "MANUAL_REVIEW" {
		t.Fatalf("发作期 = %s / %s / %s / %s / hits %d / %s", cause, fact, version, basisJudgement, hits, outcome)
	}

	redeliverHandoverToReask(t, fixture)
	if n := len(departureGateReasksOf(t, fixture)); n != 1 {
		t.Fatalf("已有定论之后又补问了：%d 次", n)
	}
	if n := signalEpisodes(t, fixture, movementParcelBlocked); n != 0 {
		t.Fatalf("没交接入段的那一件形成了 %d 期", n)
	}
}

// Covers: AT-TF-346 派发半边、AT-VE-207 派发半边——补问两路与补问信号那一路的失败分格：补问停在读得到的依赖上、补问或出发读不回、
// 信号搁置落 dispatch.consumer_undecided；补问的缺陷、仓储不变量已破、引用译不出、分诊意图待发、封闭集合外保持 dispatch.publish_failed。
// 名单取各路的哨兵变量本身。
func TestDepartureGateReaskFailuresLandInTheRightPartition(t *testing.T) {
	type failure struct {
		name     string
		err      error
		wantCode string
	}
	tfFailures := []failure{
		{"补问停在读得到的依赖上", tfinbox.ErrDepartureGateReaskUndecided, "dispatch.consumer_undecided"},
		{"补问撞上缺陷", tfinbox.ErrDepartureGateReaskDefect, "dispatch.publish_failed"},
	}
	lanes := []struct {
		name      string
		event     eventing.EventType
		sentinels []error
		failures  []failure
	}{
		{"交接上的补问", veinbox.TransportHandoverRegisteredEventType, tfDepartureGateReaskUndecidedSentinels, tfFailures},
		{"揽收上的补问", veinbox.OffsitePickupRegisteredEventType, tfDepartureGateReaskUndecidedSentinels, tfFailures},
		{"补问信号", veinbox.DepartureGateReaskRecordedEventType, veDepartureGateReaskSignalUndecidedSentinels, []failure{
			{"补问读不回", vetf.ErrDepartureGateReaskNotVisible, "dispatch.consumer_undecided"},
			{"出发读不回", vetf.ErrMovementFactNotVisible, "dispatch.consumer_undecided"},
			{"信号搁置", vetf.ErrDepartureGateSignalUndecided, "dispatch.consumer_undecided"},
			{"仓储不变量已破", vetf.ErrMovementFactRecordInconsistent, "dispatch.publish_failed"},
			{"引用译不出", vetf.ErrMovementFactUntranslatableAnswer, "dispatch.publish_failed"},
			{"分诊意图待发", vetf.ErrDepartureGateSignalHandoffPending, "dispatch.publish_failed"},
			{"信号封闭集合外", veapplication.ErrUnexpectedDepartureGateSignalOutcome, "dispatch.publish_failed"},
		}},
	}
	for _, lane := range lanes {
		for _, test := range lane.failures {
			t.Run(lane.name+"/"+test.name, func(t *testing.T) {
				beat, db, store := beatWithSentinelledConsumer(t, &stallingConsumer{err: test.err}, lane.event, lane.sentinels)
				enqueueForBeat(t, db, store, "reask-1", lane.event, `{}`)

				published, err := beat.DispatchOnce(t.Context())
				if err != nil {
					t.Fatalf("一拍：%v", err)
				}
				if published != 0 {
					t.Fatalf("失败的投递被定稿了 %d 条", published)
				}
				if got := recordedFailureCode(t, db, "reask-1"); got != test.wantCode {
					t.Fatalf("failure_code = %q, want %q", got, test.wantCode)
				}
			})
		}
	}
}

// Covers: 路由表的补问信号一条——补问信封只投 VE。毒丸载荷让消费门显式拒收入账并交回 nil，因此这一条会被定稿；漏挂或挂错的话这里
// 撞的是无订阅者。
func TestADepartureGateReaskReachesTheSignalConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "reask-route-1", veinbox.DepartureGateReaskRecordedEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把补问信封投给 VE", published, recordedFailureCode(t, db, "reask-route-1"))
	}
}

// saveReaskSchedule 站在 TF 侧登出发引用的那一班：SHA 枢纽 → SZX 口岸。
func saveReaskSchedule(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	schedules, err := tfpostgres.NewTransportSchedules(fixture.db)
	if err != nil {
		t.Fatalf("构造班次册：%v", err)
	}
	tenant := mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String())
	direction, err := tfdomain.NewScheduleDirection("SYN-CONN-SHA-SZX", "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE")
	if err != nil {
		t.Fatalf("方向：%v", err)
	}
	schedule, err := tfdomain.FormTransportSchedule(tfdomain.TransportScheduleSpec{
		TenantID:     tenant,
		Schedule:     mustTF(t, tfdomain.NewScheduleReference, reaskSchedule),
		Direction:    direction,
		DepartsAt:    movementDepartedAt,
		ArrivesAt:    movementDepartedAt.Add(4 * time.Hour),
		ServiceScope: mustTF(t, tfdomain.NewScheduleServiceScopeReference, "SYN-SCOPE-SHA-SZX"),
	})
	if err != nil {
		t.Fatalf("班次：%v", err)
	}
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		_, err := schedules.Save(txCtx, tfports.ScheduleRecord{
			Key:           tfports.ScheduleKey{TenantID: tenant, Schedule: schedule.Schedule()},
			ContentDigest: "syn-digest-reask",
			Schedule:      schedule,
			RecordedAt:    movementHandedOverAt,
		})
		return err
	})
}

// restrictLoadingDepartureOf 在关务限制册上给 parcel 登一条出发前三小时起有效、约束装载出发的监管限制。
func restrictLoadingDepartureOf(t *testing.T, fixture *synVerticalFixture, parcel string) {
	t.Helper()
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO customs_compliance.regulatory_restriction
			(tenant_id, restriction_id, decision_id, scope_ref, constrains, effective_at)
		 VALUES ($1, 'SYN-RESTRICTION-REASK', 'SYN-DECISION-HOLD', $2, jsonb_build_array('LOADING_DEPARTURE'), $3)`,
		fixture.identity.TenantID().String(), parcel, movementDepartedAt.Add(-3*time.Hour)); err != nil {
		t.Fatalf("登限制：%v", err)
	}
}

// recordStoppedDepartureThroughProduction 经生产登记编排登一条出发：段此刻不在册，首版读数停在本侧「段不在册」（登记那一刻读数
// 路径就答这一格，cmd/parcel-api 的用例证）。交回移动事实信封的事件 ID。
func recordStoppedDepartureThroughProduction(t *testing.T, fixture *synVerticalFixture) string {
	t.Helper()
	facts, err := tfpostgres.NewMovementFacts(fixture.db)
	if err != nil {
		t.Fatalf("构造移动事实册：%v", err)
	}
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := tfpostgres.NewOutboxMovementFactHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造移动事实交出：%v", err)
	}
	handler := tfapp.NewRecordMovementFactHandler(tfapp.RecordMovementFactDeps{Facts: facts, Handoff: handoff, Clock: systemClock{}})
	reading, err := tfdomain.FormDepartureGateReading(tfdomain.DepartureGateReadingSpec{
		From:  mustTF(t, tfdomain.NewNetworkNodeReference, "SYN-NODE-SHA-HUB"),
		To:    mustTF(t, tfdomain.NewNetworkNodeReference, "SYN-NODE-SZX-GATE"),
		Stops: []tfdomain.DepartureGateStop{tfdomain.DepartureSegmentNotRegistered},
	})
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	command := tfapp.RecordMovementFactCommand{
		TenantID:   mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String()),
		Fact:       reaskFact,
		Schedule:   reaskSchedule,
		Kind:       tfdomain.DepartureFact,
		Location:   "SYN-NODE-SHA-HUB",
		Source:     "SYN-OWN-FLEET/v1",
		Version:    "v1",
		OccurredAt: movementDepartedAt,
		Segment:    movementSegment,
	}
	var result tfapp.RecordMovementFactResult
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var recordErr error
		result, recordErr = handler.Record(txCtx, command, reading)
		return recordErr
	})
	if result.Outcome() != tfapp.MovementFactRecorded {
		t.Fatalf("登记出发 outcome = %s", result.Outcome())
	}
	return movementFactEventID(t, fixture.db, reaskFact)
}

func departureGateReasksOf(t *testing.T, fixture *synVerticalFixture) []tfports.DepartureGateReaskRecord {
	t.Helper()
	reasks, err := tfpostgres.NewDepartureGateReasks(fixture.db)
	if err != nil {
		t.Fatalf("构造补问册：%v", err)
	}
	listed, err := reasks.FindByDeparture(t.Context(), tfports.MovementFactKey{
		TenantID: mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String()),
		Fact:     mustTF(t, tfdomain.NewMovementFactReference, reaskFact),
		Version:  mustTF(t, tfdomain.NewMovementFactVersion, "v1"),
	})
	if err != nil {
		t.Fatalf("列补问：%v", err)
	}
	return listed
}

func departureFirstReading(t *testing.T, fixture *synVerticalFixture) tfdomain.DepartureGateReading {
	t.Helper()
	facts, err := tfpostgres.NewMovementFacts(fixture.db)
	if err != nil {
		t.Fatalf("构造移动事实册：%v", err)
	}
	record, found, err := facts.FindByKey(t.Context(), tfports.MovementFactKey{
		TenantID: mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String()),
		Fact:     mustTF(t, tfdomain.NewMovementFactReference, reaskFact),
		Version:  mustTF(t, tfdomain.NewMovementFactVersion, "v1"),
	})
	if err != nil || !found {
		t.Fatalf("读回出发：found=%v err=%v", found, err)
	}
	reading, _ := record.Fact.GateReading()
	return reading
}

// redeliverHandoverToReask 以新 ID 把同一版交接交给生产的补问那一路。
func redeliverHandoverToReask(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	_, inboxStore := redeliveryParts(t, fixture)
	onHandover, _, err := departureGateReaskConsumers(fixture.db, inboxStore, store, systemClock{})
	if err != nil {
		t.Fatalf("装配补问那一路：%v", err)
	}
	tenant := fixture.identity.TenantID().String()
	if err := onHandover.Consume(t.Context(), redeliveryEnvelope(tenant, "redeliver-handover-reask",
		veinbox.TransportHandoverRegisteredEventType, movementParcelOnBoard,
		`{"tenantId":"`+tenant+`","object":"`+movementParcelOnBoard+`","scope":"`+backfillScope+
			`","version":"`+backfillVersion+`"}`)); err != nil {
		t.Fatalf("交接重投到补问：%v", err)
	}
}
