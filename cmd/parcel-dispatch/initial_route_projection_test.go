package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件证 INITIAL-ROUTE-ENVELOPE-KEY-B：NR 包裹级初始路由判断只投 VE 投影，不 FanOut。
// 映射目录未配置必须未归类入账（PROJECTION_DERIVED + MAPPING_NOT_CONFIGURED）；两支
// 结论各自成事实类型。不种 PAR-VIS-01，不登记 tracking-projection.derived。

// 键取 syn_pc_seed_test.go 的 synInitialRouteKey（SYN 判断键唯一权威）；下面的字面量
// 只是它各维的引用副本，供事实引用与信封 ID 拼串。
const (
	deriveInitialRouteConsumerName = "visibility-exception/derive-projection-from-initial-route"
	initialRouteParcel             = "SYN-PARCEL-01"
	initialRouteBaseline           = "SYN-VER-01"
	initialRoutePurpose            = "NETWORK_SERVICE"
	initialRouteFactRef            = "initial-route/" + initialRouteParcel + "/" + initialRoutePurpose
)

// synInitialRouteEventID 与 OutboxInitialRouteHandoff 的信封 ID 同构：判断键加类型段
// （ADR-0043 意图由判断键认领）。
func synInitialRouteEventID(fixture *synVerticalFixture) string {
	return fixture.identity.TenantID().String() + "/initial-route/" + fixture.requestID.String() + "/" +
		initialRouteParcel + "/" + initialRouteBaseline + "/" + initialRoutePurpose
}

// Covers: 生产 wireDispatcher 一拍——计划支派生未归类投影，初始路由信封定稿
// published==1。本路只投 VE；不要学 FanOut 各路去要 published==0。
func TestAFormedInitialRouteDerivesAnUnclassifiedProjectionAndPublishes(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	eventID := recordInitialRouteJudgment(t, fixture, true)

	published, err := fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("初始路由只投 VE 却没定稿：published = %d；失败码 = %q",
			published, recordedFailureCode(t, fixture.db, eventID))
	}

	assertUnclassifiedInitialRouteProjection(t, fixture, "initial-route-formed")
	if n := fixture.countInbox(t, deriveInitialRouteConsumerName, eventID); n != 1 {
		t.Fatalf("VE inbox 行数 = %d, want 1", n)
	}
	if n := fixture.countOutboxOfType(t, trackingProjectionDerivedType); n != 1 {
		t.Fatalf("投影交接信封 = %d, want 1——派生成功必须入队，且不得为测试去登记消费者", n)
	}
}

// Covers: 无当前路由支——同一路由表条目、同一消费门，事实类型必须是
// initial-route-no-current-route 而不是与计划支共用一格。
func TestANoCurrentRouteJudgmentDerivesItsOwnFactKind(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	eventID := recordInitialRouteJudgment(t, fixture, false)

	published, err := fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("无路由支没定稿：published = %d；失败码 = %q",
			published, recordedFailureCode(t, fixture.db, eventID))
	}

	assertUnclassifiedInitialRouteProjection(t, fixture, "initial-route-no-current-route")
	if n := fixture.countInbox(t, deriveInitialRouteConsumerName, eventID); n != 1 {
		t.Fatalf("VE inbox 行数 = %d, want 1", n)
	}
	if n := fixture.countOutboxOfType(t, trackingProjectionDerivedType); n != 1 {
		t.Fatalf("投影交接信封 = %d, want 1", n)
	}
}

// Covers: 判断本体还看不见时的未决分格——回滚不入账（dispatch.consumer_undecided），
// 判断落库后重投同一封信封即定稿。信封只带键、本体按键重取的续办半边。
func TestAnInvisibleInitialRouteRollsBackAndTheRedeliveryRetries(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	eventID := synInitialRouteEventID(fixture)
	enqueueForBeat(t, fixture.db, store, eventID, synV0InitialRouteType,
		`{"tenantId":"`+fixture.identity.TenantID().String()+
			`","customerAccountId":"`+fixture.identity.CustomerAccountID().String()+
			`","shipment":"`+fixture.requestID.String()+
			`","parcel":"`+initialRouteParcel+
			`","baseline":"`+initialRouteBaseline+
			`","purpose":"`+initialRoutePurpose+
			`","correlation":"SYN-ROUTE-CORR-01"}`)

	published, err := fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("判断还没落库却定稿了 %d 条", published)
	}
	if got := recordedFailureCode(t, fixture.db, eventID); got != "dispatch.consumer_undecided" {
		t.Fatalf("failure_code = %q, want dispatch.consumer_undecided", got)
	}
	if n := fixture.countInbox(t, deriveInitialRouteConsumerName, eventID); n != 0 {
		t.Fatalf("inbox 行数 = %d, want 0——未决必须回滚，不能冒充已处理", n)
	}

	saveInitialRouteRecord(t, fixture, synInitialRouteRecord(t, fixture, true))

	published, err = fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("重拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("判断已落库重投却没定稿：published = %d；失败码 = %q",
			published, recordedFailureCode(t, fixture.db, eventID))
	}
	assertUnclassifiedInitialRouteProjection(t, fixture, "initial-route-formed")
	if n := fixture.countInbox(t, deriveInitialRouteConsumerName, eventID); n != 1 {
		t.Fatalf("重投后 inbox 行数 = %d, want 1", n)
	}
}

// recordInitialRouteJudgment 把一份已提交判断连同交接信封落进 fixture 的库：formed
// 走计划支，否则走无路由支。信封由生产的 OutboxInitialRouteHandoff 入队，不手搓。
func recordInitialRouteJudgment(t *testing.T, fixture *synVerticalFixture, formed bool) string {
	t.Helper()

	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	routes, err := nrpostgres.NewInitialRoutes(fixture.db)
	if err != nil {
		t.Fatalf("构造初始路由库：%v", err)
	}
	handoff, err := nrpostgres.NewOutboxInitialRouteHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造初始路由交接：%v", err)
	}

	record := synInitialRouteRecord(t, fixture, formed)
	var outcome nrports.InitialRouteSaveOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var saveErr error
		if outcome, saveErr = routes.Save(txCtx, record); saveErr != nil {
			return saveErr
		}
		return handoff.HandOffInitialRoute(txCtx, nrports.InitialRouteHandoffIntent{
			Correlation: mustNR(t, nrdomain.NewRequestCorrelationID, "SYN-ROUTE-CORR-01"),
			Record:      record,
		})
	})
	if outcome != nrports.InitialRouteSaved {
		t.Fatalf("save outcome = %v, want 已写入", outcome)
	}
	return synInitialRouteEventID(fixture)
}

func saveInitialRouteRecord(t *testing.T, fixture *synVerticalFixture, record nrports.InitialRouteRecord) {
	t.Helper()
	routes, err := nrpostgres.NewInitialRoutes(fixture.db)
	if err != nil {
		t.Fatalf("构造初始路由库：%v", err)
	}
	var outcome nrports.InitialRouteSaveOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = routes.Save(txCtx, record)
		return saveErr
	})
	if outcome != nrports.InitialRouteSaved {
		t.Fatalf("save outcome = %v, want 已写入", outcome)
	}
}

func synInitialRouteRecord(t *testing.T, fixture *synVerticalFixture, formed bool) nrports.InitialRouteRecord {
	t.Helper()
	key := synInitialRouteKey(t, fixture)
	judgedAt := time.Now().UTC().Add(-2 * time.Minute)
	if formed {
		window, err := nrdomain.NewPlannedTimeWindow(
			judgedAt.Add(2*time.Hour), judgedAt.Add(6*time.Hour),
			mustNR(t, nrdomain.NewWindowBasisReference, "SYN-CALENDAR/v1"))
		if err != nil {
			t.Fatalf("构造窗口：%v", err)
		}
		leg, err := nrdomain.NewPlannedLeg(nrdomain.PlannedLegSpec{
			From:        mustNR(t, nrdomain.NewPlanNodeReference, "SYN-HUB-01"),
			To:          mustNR(t, nrdomain.NewPlanNodeReference, "SYN-ZONE-01"),
			Responsible: mustNR(t, nrdomain.NewResponsiblePartyReference, "SYN-CARRIER-01"),
			Window:      window,
		})
		if err != nil {
			t.Fatalf("构造段：%v", err)
		}
		plan, err := nrdomain.FormInitialRoutePlan(nrdomain.InitialRoutePlanSpec{
			Key:           key,
			Version:       mustNR(t, nrdomain.NewRoutePlanVersionID, "SYN-RPV-0001"),
			Selected:      mustNR(t, nrdomain.NewCandidateID, "SYN-CAND-01"),
			Candidates:    synInitialRouteCandidates(t),
			Legs:          []nrdomain.PlannedLeg{leg},
			Strategy:      mustNR(t, nrdomain.NewRouteStrategyReference, "SYN-STRATEGY/v1"),
			ViewRevision:  mustNR(t, nrdomain.NewNetworkViewRevision, "SYN-NETVIEW-01"),
			JudgedAt:      judgedAt,
			EffectiveFrom: judgedAt.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("形成计划：%v", err)
		}
		return nrports.InitialRouteRecord{Key: key, Plan: plan, HasPlan: true}
	}
	eliminated := func(id, reason string) nrdomain.RouteCandidate {
		candidate, err := nrdomain.NewRouteCandidate(
			mustNR(t, nrdomain.NewCandidateID, id),
			nrdomain.CandidateEliminated,
			mustNR(t, nrdomain.NewCandidateReason, reason),
		)
		if err != nil {
			t.Fatalf("构造淘汰候选：%v", err)
		}
		return candidate
	}
	judgment, err := nrdomain.FormNoCurrentRouteJudgment(nrdomain.NoCurrentRouteJudgmentSpec{
		Key: key,
		Candidates: []nrdomain.RouteCandidate{
			eliminated("SYN-CAND-01", "SYN-AREA-EXCLUDED"),
			eliminated("SYN-CAND-02", "SYN-PATH-CLOSED"),
		},
		Strategy:     mustNR(t, nrdomain.NewRouteStrategyReference, "SYN-STRATEGY/v1"),
		ViewRevision: mustNR(t, nrdomain.NewNetworkViewRevision, "SYN-NETVIEW-01"),
		JudgedAt:     judgedAt,
	})
	if err != nil {
		t.Fatalf("形成无路可走判断：%v", err)
	}
	return nrports.InitialRouteRecord{Key: key, NoRoute: judgment, HasNoRoute: true}
}

func synInitialRouteCandidates(t *testing.T) []nrdomain.RouteCandidate {
	t.Helper()
	qualified, err := nrdomain.NewRouteCandidate(
		mustNR(t, nrdomain.NewCandidateID, "SYN-CAND-01"),
		nrdomain.CandidateQualified,
		nrdomain.CandidateReason{},
	)
	if err != nil {
		t.Fatalf("构造合格候选：%v", err)
	}
	eliminated, err := nrdomain.NewRouteCandidate(
		mustNR(t, nrdomain.NewCandidateID, "SYN-CAND-02"),
		nrdomain.CandidateEliminated,
		mustNR(t, nrdomain.NewCandidateReason, "SYN-PATH-CLOSED"),
	)
	if err != nil {
		t.Fatalf("构造淘汰候选：%v", err)
	}
	return []nrdomain.RouteCandidate{qualified, eliminated}
}

func assertUnclassifiedInitialRouteProjection(t *testing.T, fixture *synVerticalFixture, wantKind string) {
	t.Helper()

	tenant := mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String())
	parcel := mustVE(t, vedomain.NewTrackedParcelReference, initialRouteParcel)
	factRef := mustVE(t, vedomain.NewSourceFactReference, initialRouteFactRef)
	version := mustVE(t, vedomain.NewSourceFactVersion, initialRouteBaseline)

	facts, err := vepostgres.NewAcceptedFacts(fixture.db)
	if err != nil {
		t.Fatalf("构造已接受事实读口：%v", err)
	}
	record, found, err := facts.FindByKey(t.Context(), veports.FactKey{
		Tenant:  tenant,
		Source:  vedomain.SourceNetworkRouting,
		Fact:    factRef,
		Version: version,
	})
	if err != nil {
		t.Fatalf("读已接受事实：%v", err)
	}
	if !found {
		t.Fatal("映射未配置却没有事实行——未归类也必须入账；版本维必须是接受基线")
	}
	if record.Fact.Source() != vedomain.SourceNetworkRouting ||
		record.Fact.Parcel().String() != initialRouteParcel ||
		record.Fact.Kind().String() != wantKind {
		t.Fatalf("事实维 source=%s parcel=%s kind=%s, want NETWORK_ROUTING/%s/%s",
			record.Fact.Source(), record.Fact.Parcel(), record.Fact.Kind(),
			initialRouteParcel, wantKind)
	}

	projections, err := vepostgres.NewProjections(fixture.db)
	if err != nil {
		t.Fatalf("构造投影读口：%v", err)
	}
	projection, found, err := projections.FindCurrent(t.Context(), tenant, parcel)
	if err != nil {
		t.Fatalf("读当前投影：%v", err)
	}
	if !found {
		t.Fatal("没有当前投影")
	}
	if len(projection.Entries()) != 1 {
		t.Fatalf("entries = %d, want 1", len(projection.Entries()))
	}
	entry := projection.Entries()[0]
	if _, classified := entry.Milestone(); classified {
		t.Fatal("映射未配置却归了类——禁止发明初始路由里程碑")
	}
	if entry.MappingVersion().String() != "MAPPING_NOT_CONFIGURED" {
		t.Fatalf("mapping = %q, want MAPPING_NOT_CONFIGURED", entry.MappingVersion())
	}
	if entry.Fact().Fact().String() != initialRouteFactRef {
		t.Fatalf("条目 fact = %s, want %s", entry.Fact().Fact(), initialRouteFactRef)
	}
}
