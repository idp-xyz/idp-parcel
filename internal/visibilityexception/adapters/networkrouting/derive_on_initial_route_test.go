package networkrouting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/networkrouting"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var (
	routeJudgedAt   = time.Date(2026, 8, 19, 11, 0, 0, 0, time.UTC)
	routeRecordedAt = routeJudgedAt.Add(time.Second)
)

func routeValue[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type routeFinderDouble struct {
	record nrports.InitialRouteRecord
	found  bool
	err    error
	last   nrdomain.InitialRouteJudgmentKey
}

func (double *routeFinderDouble) FindByKey(
	_ context.Context, key nrdomain.InitialRouteJudgmentKey,
) (nrports.InitialRouteRecord, bool, error) {
	double.last = key
	if double.err != nil {
		return nrports.InitialRouteRecord{}, false, double.err
	}
	return double.record, double.found, nil
}

type routeCountingHandler struct {
	calls int
}

func (double *routeCountingHandler) Handle(
	context.Context, veapplication.DeriveProjectionCommand,
) (veapplication.DeriveProjectionResult, error) {
	double.calls++
	return veapplication.DeriveProjectionResult{}, errors.New("derive should not be called")
}

type routeFactStoreDouble struct {
	byKey map[veports.FactKey]veports.FactRecord
}

func (double *routeFactStoreDouble) FindByKey(_ context.Context, key veports.FactKey) (veports.FactRecord, bool, error) {
	record, found := double.byKey[key]
	return record, found, nil
}

func (double *routeFactStoreDouble) FindByParcel(
	_ context.Context,
	tenant vedomain.TenantID,
	parcel vedomain.TrackedParcelReference,
) ([]veports.FactRecord, error) {
	records := make([]veports.FactRecord, 0)
	for _, record := range double.byKey {
		if record.Key.Tenant == tenant && record.Fact.Parcel() == parcel {
			records = append(records, record)
		}
	}
	return records, nil
}

func (double *routeFactStoreDouble) Save(_ context.Context, record veports.FactRecord) (veports.FactSaveOutcome, error) {
	if _, exists := double.byKey[record.Key]; exists {
		return veports.FactAlreadyRecorded, nil
	}
	double.byKey[record.Key] = record
	return veports.FactSaved, nil
}

type routeMappingViewDouble struct {
	configured bool
	err        error
}

func (double *routeMappingViewDouble) ClassifyFact(
	_ context.Context,
	_ vedomain.AcceptedSourceFact,
) (veports.MilestoneAnswer, bool, error) {
	if double.err != nil {
		return veports.MilestoneAnswer{}, false, double.err
	}
	return veports.MilestoneAnswer{}, double.configured, nil
}

type routeProjectionStoreDouble struct {
	byKey map[string]vedomain.TrackingProjection
}

func (double *routeProjectionStoreDouble) FindCurrent(
	_ context.Context,
	tenant vedomain.TenantID,
	parcel vedomain.TrackedParcelReference,
) (vedomain.TrackingProjection, bool, error) {
	projection, found := double.byKey[tenant.String()+"/"+parcel.String()]
	return projection, found, nil
}

func (double *routeProjectionStoreDouble) Save(
	_ context.Context, tenant vedomain.TenantID, projection vedomain.TrackingProjection,
) error {
	double.byKey[tenant.String()+"/"+projection.Parcel().String()] = projection
	return nil
}

// 派生编排不消费按版本读回的口（那是审计与读侧的入口）；替身只存当前版，如实答未找到。
func (double *routeProjectionStoreDouble) FindByVersion(
	_ context.Context,
	_ vedomain.TenantID,
	_ vedomain.ProjectionVersionID,
) (vedomain.TrackingProjection, bool, error) {
	return vedomain.TrackingProjection{}, false, nil
}

type routeProjectionIdentityDouble struct{}

func (routeProjectionIdentityDouble) NextProjectionVersionID(_ context.Context) (vedomain.ProjectionVersionID, error) {
	return vedomain.NewProjectionVersionID("projection-initial-route-1")
}

type routeProjectionDownstreamDouble struct {
	err error
}

func (double *routeProjectionDownstreamDouble) HandOffProjection(
	context.Context, veports.ProjectionHandoffIntent,
) error {
	return double.err
}

type routeClock struct{ at time.Time }

func (clock routeClock) Now() time.Time { return clock.at }

func judgmentKey(t *testing.T) nrdomain.InitialRouteJudgmentKey {
	t.Helper()
	return nrdomain.InitialRouteJudgmentKey{
		TenantID:           routeValue(t, nrdomain.NewTenantID, "tenant-1"),
		CustomerAccountID:  routeValue(t, nrdomain.NewCustomerAccountID, "customer-1"),
		ShipmentRequestID:  routeValue(t, nrdomain.NewShipmentRequestID, "request-1"),
		AcceptanceBaseline: routeValue(t, nrdomain.NewAcceptanceBaselineReference, "baseline-v1"),
		DeclaredParcelID:   routeValue(t, nrdomain.NewDeclaredParcelID, "parcel-1"),
		ServicePurpose:     routeValue(t, nrdomain.NewServicePurpose, "LAST_MILE_DELIVERY"),
	}
}

func routeCandidates(t *testing.T) []nrdomain.RouteCandidate {
	t.Helper()
	qualified, err := nrdomain.NewRouteCandidate(
		routeValue(t, nrdomain.NewCandidateID, "candidate-1"),
		nrdomain.CandidateQualified,
		nrdomain.CandidateReason{},
	)
	if err != nil {
		t.Fatalf("构造合格候选：%v", err)
	}
	eliminated, err := nrdomain.NewRouteCandidate(
		routeValue(t, nrdomain.NewCandidateID, "candidate-2"),
		nrdomain.CandidateEliminated,
		routeValue(t, nrdomain.NewCandidateReason, "path-closed"),
	)
	if err != nil {
		t.Fatalf("构造淘汰候选：%v", err)
	}
	return []nrdomain.RouteCandidate{qualified, eliminated}
}

// routeRecord 造一份已提交记录：formed 走计划支（EffectiveFrom = JudgedAt+1h，供
// 有效时间断言与发生时间区分），否则走无路由支。
func routeRecord(t *testing.T, formed bool) nrports.InitialRouteRecord {
	t.Helper()
	key := judgmentKey(t)
	if formed {
		window, err := nrdomain.NewPlannedTimeWindow(
			routeJudgedAt.Add(2*time.Hour), routeJudgedAt.Add(6*time.Hour),
			routeValue(t, nrdomain.NewWindowBasisReference, "calendar/v1"))
		if err != nil {
			t.Fatalf("构造窗口：%v", err)
		}
		leg, err := nrdomain.NewPlannedLeg(nrdomain.PlannedLegSpec{
			From:        routeValue(t, nrdomain.NewPlanNodeReference, "hub-a"),
			To:          routeValue(t, nrdomain.NewPlanNodeReference, "zone-b"),
			Responsible: routeValue(t, nrdomain.NewResponsiblePartyReference, "carrier-1"),
			Window:      window,
		})
		if err != nil {
			t.Fatalf("构造段：%v", err)
		}
		plan, err := nrdomain.FormInitialRoutePlan(nrdomain.InitialRoutePlanSpec{
			Key:           key,
			Version:       routeValue(t, nrdomain.NewRoutePlanVersionID, "RPV-0001"),
			Selected:      routeValue(t, nrdomain.NewCandidateID, "candidate-1"),
			Candidates:    routeCandidates(t),
			Legs:          []nrdomain.PlannedLeg{leg},
			Strategy:      routeValue(t, nrdomain.NewRouteStrategyReference, "strategy/v1"),
			ViewRevision:  routeValue(t, nrdomain.NewNetworkViewRevision, "netview-42"),
			JudgedAt:      routeJudgedAt,
			EffectiveFrom: routeJudgedAt.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("形成计划：%v", err)
		}
		return nrports.InitialRouteRecord{
			Key: key, Plan: plan, HasPlan: true, RecordedAt: routeRecordedAt,
		}
	}
	eliminated := func(id, reason string) nrdomain.RouteCandidate {
		candidate, err := nrdomain.NewRouteCandidate(
			routeValue(t, nrdomain.NewCandidateID, id),
			nrdomain.CandidateEliminated,
			routeValue(t, nrdomain.NewCandidateReason, reason),
		)
		if err != nil {
			t.Fatalf("构造淘汰候选：%v", err)
		}
		return candidate
	}
	judgment, err := nrdomain.FormNoCurrentRouteJudgment(nrdomain.NoCurrentRouteJudgmentSpec{
		Key:          key,
		Candidates:   []nrdomain.RouteCandidate{eliminated("candidate-1", "area-excluded"), eliminated("candidate-2", "path-closed")},
		Strategy:     routeValue(t, nrdomain.NewRouteStrategyReference, "strategy/v1"),
		ViewRevision: routeValue(t, nrdomain.NewNetworkViewRevision, "netview-42"),
		JudgedAt:     routeJudgedAt,
	})
	if err != nil {
		t.Fatalf("形成无路可走判断：%v", err)
	}
	return nrports.InitialRouteRecord{
		Key: key, NoRoute: judgment, HasNoRoute: true, RecordedAt: routeRecordedAt,
	}
}

func formedInitialRouteRef() veinbox.FormedInitialRoute {
	return veinbox.FormedInitialRoute{
		TenantID:          "tenant-1",
		CustomerAccountID: "customer-1",
		Shipment:          "request-1",
		Parcel:            "parcel-1",
		Baseline:          "baseline-v1",
		Purpose:           "LAST_MILE_DELIVERY",
	}
}

func routeDeriveHandler(t *testing.T, mapping routeMappingViewDouble, downstream routeProjectionDownstreamDouble) (
	*veapplication.DeriveProjectionHandler,
	*routeFactStoreDouble,
	*routeProjectionStoreDouble,
) {
	t.Helper()
	facts := &routeFactStoreDouble{byKey: map[veports.FactKey]veports.FactRecord{}}
	projections := &routeProjectionStoreDouble{byKey: map[string]vedomain.TrackingProjection{}}
	handler := veapplication.NewDeriveProjectionHandler(veapplication.DeriveProjectionDeps{
		Facts:       facts,
		Mapping:     &mapping,
		Projections: projections,
		Identities:  routeProjectionIdentityDouble{},
		Downstream:  &downstream,
		Clock:       routeClock{at: routeJudgedAt.Add(2 * time.Hour)},
	})
	return handler, facts, projections
}

func TestAMissingInitialRouteIsContinuableUndecided(t *testing.T) {
	derive := &routeCountingHandler{}
	subject, err := adapter.NewDeriveOnInitialRouteAdapter(&routeFinderDouble{}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedInitialRoute(t.Context(), formedInitialRouteRef()); !errors.Is(err, adapter.ErrInitialRouteNotVisible) {
		t.Fatalf("err = %v, want ErrInitialRouteNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("缺判断记录不该走到派生")
	}
}

func TestAnUnreadableInitialRouteIsContinuableUndecided(t *testing.T) {
	derive := &routeCountingHandler{}
	subject, err := adapter.NewDeriveOnInitialRouteAdapter(
		&routeFinderDouble{err: errors.New("store unavailable")}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedInitialRoute(t.Context(), formedInitialRouteRef()); !errors.Is(err, adapter.ErrInitialRouteNotVisible) {
		t.Fatalf("err = %v, want ErrInitialRouteNotVisible", err)
	}
	if derive.calls != 0 {
		t.Fatal("读失败不该走到派生")
	}
}

func TestAMismatchedInitialRouteKeyIsInconsistentNotUndecided(t *testing.T) {
	record := routeRecord(t, true)
	record.Key.DeclaredParcelID = routeValue(t, nrdomain.NewDeclaredParcelID, "other-parcel")
	derive := &routeCountingHandler{}
	subject, err := adapter.NewDeriveOnInitialRouteAdapter(
		&routeFinderDouble{record: record, found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedInitialRoute(t.Context(), formedInitialRouteRef()); !errors.Is(err, adapter.ErrInitialRouteRecordInconsistent) {
		t.Fatalf("err = %v, want ErrInitialRouteRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("键本体不符不该走到派生")
	}
}

func TestAnInitialRouteWithZeroRecordedAtIsInconsistentNotUndecided(t *testing.T) {
	record := routeRecord(t, true)
	record.RecordedAt = time.Time{}
	derive := &routeCountingHandler{}
	subject, err := adapter.NewDeriveOnInitialRouteAdapter(
		&routeFinderDouble{record: record, found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedInitialRoute(t.Context(), formedInitialRouteRef()); !errors.Is(err, adapter.ErrInitialRouteRecordInconsistent) {
		t.Fatalf("err = %v, want ErrInitialRouteRecordInconsistent", err)
	}
	if derive.calls != 0 {
		t.Fatal("时间为零不该走到派生")
	}
}

// TestAnInitialRouteWearingBothOrNeitherConclusionsIsInconsistent 证「计划或无路由
// 二居其一」的消费面：两支都带或都缺是坏写入，不是可等待的滞后。
func TestAnInitialRouteWearingBothOrNeitherConclusionsIsInconsistent(t *testing.T) {
	both := routeRecord(t, true)
	both.HasNoRoute = true
	neither := routeRecord(t, true)
	neither.HasPlan = false
	for name, record := range map[string]nrports.InitialRouteRecord{
		"两支都带": both,
		"两支都缺": neither,
	} {
		t.Run(name, func(t *testing.T) {
			derive := &routeCountingHandler{}
			subject, err := adapter.NewDeriveOnInitialRouteAdapter(
				&routeFinderDouble{record: record, found: true}, derive)
			if err != nil {
				t.Fatalf("构造：%v", err)
			}
			if err := subject.HandleFormedInitialRoute(t.Context(), formedInitialRouteRef()); !errors.Is(err, adapter.ErrInitialRouteRecordInconsistent) {
				t.Fatalf("err = %v, want ErrInitialRouteRecordInconsistent", err)
			}
			if derive.calls != 0 {
				t.Fatal("结论形状坏了不该走到派生")
			}
		})
	}
}

func TestAnUntranslatableInitialRouteReferenceKeepsItsSentinel(t *testing.T) {
	derive := &routeCountingHandler{}
	subject, err := adapter.NewDeriveOnInitialRouteAdapter(
		&routeFinderDouble{record: routeRecord(t, true), found: true}, derive)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	blank := func(mutate func(*veinbox.FormedInitialRoute)) veinbox.FormedInitialRoute {
		reference := formedInitialRouteRef()
		mutate(&reference)
		return reference
	}
	for name, reference := range map[string]veinbox.FormedInitialRoute{
		"空租户": blank(func(r *veinbox.FormedInitialRoute) { r.TenantID = "" }),
		"空客户": blank(func(r *veinbox.FormedInitialRoute) { r.CustomerAccountID = "" }),
		"空委托": blank(func(r *veinbox.FormedInitialRoute) { r.Shipment = "" }),
		"空包裹": blank(func(r *veinbox.FormedInitialRoute) { r.Parcel = "" }),
		"空基线": blank(func(r *veinbox.FormedInitialRoute) { r.Baseline = "" }),
		"空目的": blank(func(r *veinbox.FormedInitialRoute) { r.Purpose = "" }),
	} {
		t.Run(name, func(t *testing.T) {
			if err := subject.HandleFormedInitialRoute(
				t.Context(), reference); !errors.Is(err, adapter.ErrInitialRouteUntranslatableAnswer) {
				t.Fatalf("err = %v, want ErrInitialRouteUntranslatableAnswer", err)
			}
		})
	}
	if derive.calls != 0 {
		t.Fatal("引用译不出来不该走到派生")
	}
}

func TestFindByKeyLooksUpAllSixInitialRouteDimensions(t *testing.T) {
	finder := &routeFinderDouble{record: routeRecord(t, true), found: true}
	handler, _, _ := routeDeriveHandler(t, routeMappingViewDouble{configured: false}, routeProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnInitialRouteAdapter(finder, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedInitialRoute(t.Context(), formedInitialRouteRef()); err != nil {
		t.Fatalf("处理判断：%v", err)
	}
	if finder.last != judgmentKey(t) {
		t.Fatalf("FindByKey 键 = %+v；必须带全部六维（含 customerAccountId）", finder.last)
	}
}

func TestUnconfiguredMappingDerivesAnUnclassifiedInitialRouteProjection(t *testing.T) {
	handler, facts, projections := routeDeriveHandler(t, routeMappingViewDouble{configured: false}, routeProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnInitialRouteAdapter(
		&routeFinderDouble{record: routeRecord(t, true), found: true}, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}

	if err := subject.HandleFormedInitialRoute(t.Context(), formedInitialRouteRef()); err != nil {
		t.Fatalf("处理判断：%v", err)
	}

	if len(facts.byKey) != 1 {
		t.Fatalf("事实条数 = %d", len(facts.byKey))
	}
	var fact vedomain.AcceptedSourceFact
	for _, record := range facts.byKey {
		fact = record.Fact
	}
	if fact.Source() != vedomain.SourceNetworkRouting ||
		fact.Parcel().String() != "parcel-1" ||
		fact.Fact().String() != "initial-route/parcel-1/LAST_MILE_DELIVERY" ||
		fact.Kind().String() != "initial-route-formed" ||
		fact.Version().String() != "baseline-v1" ||
		!fact.OccurredAt().Equal(routeJudgedAt) ||
		!fact.EffectiveAt().Equal(routeJudgedAt.Add(time.Hour)) ||
		!fact.ReceivedAt().Equal(routeRecordedAt) {
		t.Fatalf("已接受事实维 = %+v kind=%q", fact, fact.Kind())
	}

	projection, found := projections.byKey["tenant-1/parcel-1"]
	if !found {
		t.Fatal("未配置映射必须照常派生投影——初始路由投影不因目录空缺而缺席")
	}
	if _, classified := projection.Entries()[0].Milestone(); classified {
		t.Fatal("映射未配置却归了类——禁止发明初始路由里程碑")
	}
	if projection.Entries()[0].MappingVersion().String() != "MAPPING_NOT_CONFIGURED" {
		t.Fatalf("mapping = %q", projection.Entries()[0].MappingVersion())
	}
}

// TestTwoInitialRouteConclusionsWriteDistinctFactKinds 证两支结论各自成事实类型且
// 有效时间各归各：计划支带自己的生效边界，无路由支有效时间同发生时间。
func TestTwoInitialRouteConclusionsWriteDistinctFactKinds(t *testing.T) {
	cases := map[string]struct {
		formed      bool
		kind        string
		effectiveAt time.Time
	}{
		"计划已形成": {formed: true, kind: "initial-route-formed", effectiveAt: routeJudgedAt.Add(time.Hour)},
		"无当前路由": {formed: false, kind: "initial-route-no-current-route", effectiveAt: routeJudgedAt},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			handler, facts, _ := routeDeriveHandler(t, routeMappingViewDouble{configured: false}, routeProjectionDownstreamDouble{})
			subject, err := adapter.NewDeriveOnInitialRouteAdapter(
				&routeFinderDouble{record: routeRecord(t, tc.formed), found: true}, handler)
			if err != nil {
				t.Fatalf("构造：%v", err)
			}
			if err := subject.HandleFormedInitialRoute(t.Context(), formedInitialRouteRef()); err != nil {
				t.Fatalf("处理判断：%v", err)
			}
			if len(facts.byKey) != 1 {
				t.Fatalf("事实条数 = %d", len(facts.byKey))
			}
			for _, record := range facts.byKey {
				if record.Fact.Kind().String() != tc.kind {
					t.Fatalf("kind = %q, want %q", record.Fact.Kind(), tc.kind)
				}
				if !record.Fact.EffectiveAt().Equal(tc.effectiveAt) {
					t.Fatalf("effectiveAt = %v, want %v", record.Fact.EffectiveAt(), tc.effectiveAt)
				}
			}
		})
	}
}

func TestAFailedInitialRouteProjectionHandoffKeepsThePendingSentinel(t *testing.T) {
	handler, _, _ := routeDeriveHandler(t, routeMappingViewDouble{configured: false}, routeProjectionDownstreamDouble{
		err: errors.New("outbox down"),
	})
	subject, err := adapter.NewDeriveOnInitialRouteAdapter(
		&routeFinderDouble{record: routeRecord(t, true), found: true}, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedInitialRoute(t.Context(), formedInitialRouteRef()); !errors.Is(err, adapter.ErrInitialRouteProjectionHandoffPending) {
		t.Fatalf("err = %v, want ErrInitialRouteProjectionHandoffPending", err)
	}
}

func TestInitialRouteMappingViewFailureDoesNotLookLikeUnconfigured(t *testing.T) {
	handler, _, _ := routeDeriveHandler(t, routeMappingViewDouble{err: errors.New("mapping unreachable")}, routeProjectionDownstreamDouble{})
	subject, err := adapter.NewDeriveOnInitialRouteAdapter(
		&routeFinderDouble{record: routeRecord(t, true), found: true}, handler)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleFormedInitialRoute(t.Context(), formedInitialRouteRef()); !errors.Is(err, adapter.ErrInitialRouteProjectionUndecided) {
		t.Fatalf("err = %v, want ErrInitialRouteProjectionUndecided", err)
	}
}
