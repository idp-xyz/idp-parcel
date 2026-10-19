package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	veregistration "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/registrationjson"
	vetf "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证 TF 移动事实已形成 → VE 投影与阻断信号这一路走的是生产 wireDispatcher（票 demo-chain-gaps/33，ADR-0205 决定五）：
// 生产方（TF 登记编排 + Outbox 交出）与消费方（路由表上的 VE 消费门）同一个变更上线，类型串两边相等；出发在段里那一刻在控的
// 每件对象上派生，按演示种子的里程碑映射归 DEPARTED；阻断的那一件按演示种子登的信号规则形成发作期，没登规则就如实不形成。
// TF 一侧走生产登记编排落库，读数由夹具按领域形成（问关务那一半是票 32 的事，cmd/parcel-api 的用例证）。证据只记 S。

const (
	deriveMovementFactConsumerName = "visibility-exception/derive-projection-from-movement-fact"
	movementSegment                = "SYN-SEG-SHA-SZX-DISPATCH"
	movementParcelOnBoard          = "SYN-PARCEL-DEP-01"
	movementParcelBlocked          = "SYN-PARCEL-DEP-02"
)

var (
	movementHandedOverAt = time.Date(2026, 10, 21, 13, 0, 0, 0, time.UTC)
	movementDepartedAt   = time.Date(2026, 10, 21, 14, 5, 0, 0, time.UTC)

	demoDepartureGateSignalRuleSeed = filepath.Join("..", "..", "scripts", "demo-seeds", "data", "visibility",
		"13-departure-gate-signal-rule-v1.json")
)

// Covers: 生产方与消费方类型串相等——TF 交出口自写一份，VE 消费门自写一份，路由表引的是后者；两串不等时生产方的每封信都撞
// 无订阅者。
func TestTheMovementFactProducerTypeIsTheRoutedConsumerType(t *testing.T) {
	if tfpostgres.MovementFactRecordedEventType != string(veinbox.MovementFactRecordedEventType) {
		t.Fatalf("生产方类型 %q 与消费方类型 %q 不等", tfpostgres.MovementFactRecordedEventType, veinbox.MovementFactRecordedEventType)
	}
}

// Covers: AT-TF-157 真库半边、票面演示判据「出发后 /tracking-projections 出现 DEPARTED」（证据 S）——经生产登记编排登一条出发，
// 生产路由表一拍定稿；段里出发那一刻在控的两件各得一份 movement-departure，按演示种子映射归 DEPARTED。读数阻断的那一件在
// 演示租户没登信号规则时不形成发作期，投影照派生、消费照入账。
func TestADepartureClassifiesAsDepartedUnderTheDemoMappingThroughTheProductionRoute(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	registerDemoMilestoneMapping(t, fixture)
	saveDepartureSegment(t, fixture)

	eventID := recordDepartureThroughProduction(t, fixture, "SYN-MOVE-DISPATCH-01")
	if published, err := fixture.beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("第一拍 published = %d err = %v；失败码 = %q", published, err, recordedFailureCode(t, fixture.db, eventID))
	}
	if status := outboxStatus(t, fixture.db, eventID); status != "PUBLISHED" {
		t.Fatalf("移动事实信封 status = %q, want PUBLISHED", status)
	}
	if n := fixture.countInbox(t, deriveMovementFactConsumerName, eventID); n != 1 {
		t.Fatalf("VE inbox 行数 = %d, want 1", n)
	}

	for _, parcel := range []string{movementParcelOnBoard, movementParcelBlocked} {
		entry := onlyProjectionEntry(t, fixture, parcel)
		milestone, classified := entry.Milestone()
		if !classified || milestone.String() != "DEPARTED" {
			t.Fatalf("%s 归类 = %q（%v），want DEPARTED——演示映射的出发那一行没命中 VE 交出的类型词", parcel, milestone, classified)
		}
		if entry.Fact().Kind().String() != "movement-departure" ||
			entry.Fact().Fact().String() != "movement-fact/"+parcel+"/SYN-MOVE-DISPATCH-01" {
			t.Fatalf("%s 条目 = %s %s", parcel, entry.Fact().Kind(), entry.Fact().Fact())
		}
	}
	if n := signalEpisodes(t, fixture, movementParcelBlocked); n != 0 {
		t.Fatalf("没登信号规则却形成了 %d 个发作期", n)
	}
}

// Covers: AT-TF-158 真库半边——演示租户按种子采用出发门禁阻断信号规则之后，阻断的那一件形成一个发作期，类型、规则版本与可信度
// 取种子那一份；不受门禁的那一件没有。分诊规则里没有这一类，信号如实进人工复核格。
func TestABlockedDepartureRaisesTheAdoptedSignalThroughTheProductionRoute(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	registerDemoDepartureGateSignalRule(t, fixture)
	saveDepartureSegment(t, fixture)

	eventID := recordDepartureThroughProduction(t, fixture, "SYN-MOVE-DISPATCH-02")
	if published, err := fixture.beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("第一拍 published = %d err = %v；失败码 = %q", published, err, recordedFailureCode(t, fixture.db, eventID))
	}

	if n := signalEpisodes(t, fixture, movementParcelOnBoard); n != 0 {
		t.Fatalf("不受门禁的那一件形成了 %d 个发作期", n)
	}
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("读执行器：%v", err)
	}
	var kind, rule, confidence, outcome string
	if err := querier.QueryRow(t.Context(),
		`SELECT e.kind_ref, e.rule_ref, e.confidence_ref, c.outcome
		   FROM visibility_exception.signal_episode e
		   JOIN visibility_exception.triage_conclusion c ON c.episode_id = e.episode_id
		  WHERE e.tenant_id = $1 AND e.parcel_ref = $2`,
		fixture.identity.TenantID().String(), movementParcelBlocked,
	).Scan(&kind, &rule, &confidence, &outcome); err != nil {
		t.Fatalf("读阻断那一件的发作期：%v", err)
	}
	if kind != "DEPARTURE_GATE_BLOCKED" || rule != "SYN-VE-DEPARTURE-GATE-V1" || confidence != "SOURCE_CONFIRMED" {
		t.Fatalf("发作期 = %s / %s / %s，want 种子那一份", kind, rule, confidence)
	}
	if outcome != "MANUAL_REVIEW" {
		t.Fatalf("分诊走向 = %q, want MANUAL_REVIEW（演示分诊规则没有这一类）", outcome)
	}
}

// Covers: AT-TF-160 派发半边——移动事实这一路的失败分格：读不回事实、段册读不动、信号搁置、派生编排未决落 dispatch.consumer_undecided，其余保持
// dispatch.publish_failed。名单取 veMovementFactUndecidedSentinels 本身：测试里重列一份会让漏登记的哨兵在测试里绿、在生产里塌成
// publish_failed。
func TestMovementFactFailuresLandInTheRightPartition(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"事实读不回", vetf.ErrMovementFactNotVisible, "dispatch.consumer_undecided"},
		{"段册读不动", vetf.ErrMovementSegmentNotVisible, "dispatch.consumer_undecided"},
		{"信号搁置", vetf.ErrDepartureGateSignalUndecided, "dispatch.consumer_undecided"},
		{"派生编排未决", vetf.ErrProjectionUndecided, "dispatch.consumer_undecided"},
		{"记录不变量已破", vetf.ErrMovementFactRecordInconsistent, "dispatch.publish_failed"},
		{"引用译不出", vetf.ErrMovementFactUntranslatableAnswer, "dispatch.publish_failed"},
		{"分诊意图待发", vetf.ErrDepartureGateSignalHandoffPending, "dispatch.publish_failed"},
		{"投影意图待发", vetf.ErrProjectionHandoffPending, "dispatch.publish_failed"},
		{"投影封闭集合外", veconsume.ErrUnexpectedProjectionOutcome, "dispatch.publish_failed"},
		{"信号封闭集合外", veapplication.ErrUnexpectedDepartureGateSignalOutcome, "dispatch.publish_failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			beat, db, store := beatWithSentinelledConsumer(t, &stallingConsumer{err: test.err},
				veinbox.MovementFactRecordedEventType, veMovementFactUndecidedSentinels)
			enqueueForBeat(t, db, store, "movement-1", veinbox.MovementFactRecordedEventType, `{}`)

			published, err := beat.DispatchOnce(t.Context())
			if err != nil {
				t.Fatalf("一拍：%v", err)
			}
			if published != 0 {
				t.Fatalf("失败的投递被定稿了 %d 条", published)
			}
			if got := recordedFailureCode(t, db, "movement-1"); got != test.wantCode {
				t.Fatalf("failure_code = %q, want %q", got, test.wantCode)
			}
		})
	}
}

// Covers: 路由表的移动事实一条——TF 移动事实已形成只投 VE。手法同交接那一条：毒丸载荷（缺 tenantId/fact/version 三维之一）让消费门
// 显式拒收入账并交回 nil，因此这一条会被定稿。漏挂或挂错的话这里撞的是无订阅者。
func TestARecordedMovementFactReachesTheConsumerThroughTheRouteTable(t *testing.T) {
	beat, db, store := wiredBeat(t)
	enqueueForBeat(t, db, store, "movement-1", veinbox.MovementFactRecordedEventType, `{}`)

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把移动事实投给 VE",
			published, recordedFailureCode(t, db, "movement-1"))
	}
}

// saveDepartureSegment 站在 TF 侧立一个自营段：两件在出发前一小时交接入段。
func saveDepartureSegment(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	segments, err := tfpostgres.NewFulfillmentSegments(fixture.db)
	if err != nil {
		t.Fatalf("构造段册：%v", err)
	}
	tenant := mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String())
	reference := mustTF(t, tfdomain.NewFulfillmentSegmentReference, movementSegment)
	var participations []tfdomain.RehydrateParticipationSpec
	for _, parcel := range []string{movementParcelOnBoard, movementParcelBlocked} {
		participations = append(participations, tfdomain.RehydrateParticipationSpec{
			Object:     mustTF(t, tfdomain.NewCarriedObjectReference, parcel),
			EntryKind:  tfdomain.EnteredByTransportHandover,
			EntryBasis: mustTF(t, tfdomain.NewParticipationBasisReference, "SYN-HANDOVER/"+parcel),
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

// recordDepartureThroughProduction 经生产登记编排（真登记册 + 真 Outbox 交出）在一笔事务里登一条出发：读数问到两件，第二件阻断。
// 交回移动事实信封的事件 ID。
func recordDepartureThroughProduction(t *testing.T, fixture *synVerticalFixture, fact string) string {
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

	finding := func(parcel string, answer tfdomain.DepartureGateAnswer) tfdomain.DepartureGateFinding {
		built, err := tfdomain.NewDepartureGateFinding(mustTF(t, tfdomain.NewCarriedObjectReference, parcel), answer, "AADJ-"+parcel,
			[]tfdomain.DepartureGateBasis{{Action: "LOADING_DEPARTURE", Cause: "SYN-CAUSE"}})
		if err != nil {
			t.Fatalf("逐对象那一格：%v", err)
		}
		return built
	}
	reading, err := tfdomain.FormDepartureGateReading(tfdomain.DepartureGateReadingSpec{
		From:     mustTF(t, tfdomain.NewNetworkNodeReference, "SYN-NODE-SHA-HUB"),
		To:       mustTF(t, tfdomain.NewNetworkNodeReference, "SYN-NODE-SZX-GATE"),
		Findings: []tfdomain.DepartureGateFinding{finding(movementParcelOnBoard, tfdomain.DepartureGateNotGuarded), finding(movementParcelBlocked, tfdomain.DepartureGateBlocked)},
	})
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	command := tfapp.RecordMovementFactCommand{
		TenantID:   mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String()),
		Fact:       fact,
		Schedule:   "SYN-SCHED-SHA-SZX-DAILY/2026-10-21",
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
	return movementFactEventID(t, fixture.db, fact)
}

func movementFactEventID(t *testing.T, db *bentopg.DB, fact string) string {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("读执行器：%v", err)
	}
	var eventID string
	if err := querier.QueryRow(t.Context(),
		`SELECT event_id FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1 AND subject = $2`,
		tfpostgres.MovementFactRecordedEventType, fact,
	).Scan(&eventID); err != nil {
		t.Fatalf("读移动事实信封——登记那一笔没有同时入队：%v", err)
	}
	return eventID
}

func onlyProjectionEntry(t *testing.T, fixture *synVerticalFixture, parcel string) vedomain.MilestoneClassification {
	t.Helper()
	projections, err := vepostgres.NewProjections(fixture.db)
	if err != nil {
		t.Fatalf("构造投影读口：%v", err)
	}
	projection, found, err := projections.FindCurrent(t.Context(),
		mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String()), mustVE(t, vedomain.NewTrackedParcelReference, parcel))
	if err != nil || !found || len(projection.Entries()) != 1 {
		t.Fatalf("%s 当前投影 found = %v err = %v", parcel, found, err)
	}
	return projection.Entries()[0]
}

func signalEpisodes(t *testing.T, fixture *synVerticalFixture, parcel string) int {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("读执行器：%v", err)
	}
	var count int
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*) FROM visibility_exception.signal_episode WHERE tenant_id = $1 AND parcel_ref = $2`,
		fixture.identity.TenantID().String(), parcel,
	).Scan(&count); err != nil {
		t.Fatalf("数发作期：%v", err)
	}
	return count
}

// registerDemoDepartureGateSignalRule 把演示种子的出发门禁阻断信号规则经生产翻译与登记用例登进库——与 seed.sh 调
// parcel-ve-register 走的是同一份翻译与同一个写入方。
func registerDemoDepartureGateSignalRule(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	raw, err := os.ReadFile(demoDepartureGateSignalRuleSeed)
	if err != nil {
		t.Fatalf("读演示信号规则种子：%v", err)
	}
	command, err := veregistration.DepartureGateSignalRuleFromJSON(raw)
	if err != nil {
		t.Fatalf("翻译演示信号规则种子：%v", err)
	}
	registrar, err := vepostgres.NewCatalogRegistrar(fixture.db)
	if err != nil {
		t.Fatalf("构造目录写入方：%v", err)
	}
	catalogs, err := veapplication.NewCatalogRegistration(registrar)
	if err != nil {
		t.Fatalf("构造登记用例：%v", err)
	}
	var result veapplication.RegisterCatalogResult
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var registerErr error
		result, registerErr = catalogs.RegisterDepartureGateSignalRule(txCtx, command)
		return registerErr
	})
	if result.Outcome() != veapplication.CatalogRegistered {
		t.Fatalf("登记演示信号规则 outcome = %q，拒绝理由 = %q", result.Outcome(), result.RefusalReason())
	}
}

// beatWithSentinelledConsumer 只挂一条路由：inner 按本路的未决名单包好，挂在 eventType 上。
func beatWithSentinelledConsumer(
	t *testing.T,
	inner dispatch.Consumer,
	eventType eventing.EventType,
	sentinels []error,
) (Beat, *bentopg.DB, *outbox.Store) {
	t.Helper()

	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	routed, err := dispatch.WithUndecidedSentinels(inner, sentinels...)
	if err != nil {
		t.Fatalf("包装未决哨兵：%v", err)
	}
	config := dispatch.Config{Limit: 10, LeaseFor: time.Minute, MaxAttempts: 5, RetryAfter: 30 * time.Second}
	publisher, err := dispatch.NewDirectPublisher(map[eventing.EventType]dispatch.Consumer{eventType: routed}, 5*time.Second, config)
	if err != nil {
		t.Fatalf("直投发布器：%v", err)
	}
	beat, err := dispatch.NewDispatcher(store, store, publisher, systemClock{}, config)
	if err != nil {
		t.Fatalf("派发器：%v", err)
	}
	return beat, db, store
}
