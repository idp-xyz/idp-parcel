package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

type calendarClock struct{ at time.Time }

func (clock calendarClock) Now() time.Time { return clock.at }

// Covers: 跨时区两节点按绝对时刻折出完成时刻；目录再写一笔，视图修订换代。
func TestInitialRouteEvidenceFoldsCalendarTimeAcrossZones(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-calendar")
	asOf := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	effective := asOf.Add(-24 * time.Hour)
	seedCalendarNetwork(t, transactor, ctx, catalog, tenant, effective, true)

	view, err := application.NewCatalogInitialRouteEvidence(catalog, catalogReachCustoms{}, calendarClock{at: asOf})
	if err != nil {
		t.Fatal(err)
	}
	evidence, configured, err := view.LoadInitialRouteEvidence(ctx, calendarRouteKey(t, tenant), ports.RequestCarriedContent{})
	if err != nil || !configured {
		t.Fatalf("configured=%v err=%v", configured, err)
	}
	if len(evidence.Projections) != 1 {
		t.Fatalf("projections = %d", len(evidence.Projections))
	}
	latest := evidence.Projections[0].Window().Latest()
	want := time.Date(2026, 8, 20, 12, 30, 0, 0, time.UTC)
	if !latest.Equal(want) {
		t.Fatalf("latest = %s, want %s", latest, want)
	}
	if len(evidence.Paths) != 1 || len(evidence.Paths[0].Legs) != 1 {
		t.Fatalf("paths = %+v", evidence.Paths)
	}
	firstRevision := evidence.ViewRevision.String()

	within(t, transactor, ctx, func(txCtx context.Context) error {
		return catalog.RegisterNodeVersion(txCtx, tenant, ports.NodeDefinitionVersion{
			Code: "node-extra", Version: 1, BusinessTimezone: "UTC", EffectiveFrom: effective,
		})
	})
	again, _, err := view.LoadInitialRouteEvidence(ctx, calendarRouteKey(t, tenant), ports.RequestCarriedContent{})
	if err != nil {
		t.Fatal(err)
	}
	if again.ViewRevision.String() == firstRevision {
		t.Fatal("目录又写了一笔，视图修订却没变")
	}
}

// Covers: 日历缺登的节点不补默认窗口，候选在路径可执行性上不可用并点名那个节点。
func TestANodeWithoutACalendarIsNotGivenADefaultWindow(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-no-calendar")
	asOf := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	seedCalendarNetwork(t, transactor, ctx, catalog, tenant, asOf.Add(-24*time.Hour), false)

	view, err := application.NewCatalogInitialRouteEvidence(catalog, catalogReachCustoms{}, calendarClock{at: asOf})
	if err != nil {
		t.Fatal(err)
	}
	evidence, configured, err := view.LoadInitialRouteEvidence(ctx, calendarRouteKey(t, tenant), ports.RequestCarriedContent{})
	if err != nil || !configured {
		t.Fatalf("configured=%v err=%v", configured, err)
	}
	if len(evidence.Projections) != 0 {
		t.Fatal("缺日历的节点被补了一份时间投影")
	}
	if len(evidence.PathExecutability) != 1 ||
		evidence.PathExecutability[0].Outcome() != domain.PathNotExecutable ||
		evidence.PathExecutability[0].Schedule().String() != "CALENDAR_NOT_REGISTERED/node-lax" {
		t.Fatalf("executability = %+v", evidence.PathExecutability)
	}
}

// Covers: 提交前重校走真取数侧。第二次读取前目录再写一笔，修订变了就按新证据重判，不提交第一次的结果。
func TestACatalogWriteBeforeCommitRejudgesOnTheRealReadSide(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-rejudge")
	asOf := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	effective := asOf.Add(-24 * time.Hour)
	seedCalendarNetwork(t, transactor, ctx, catalog, tenant, effective, true)

	flip := &flipOnSecondRead{inner: catalog, transactor: transactor, tenant: tenant, effective: effective}
	view, err := application.NewCatalogInitialRouteEvidence(flip, catalogReachCustoms{}, calendarClock{at: asOf})
	if err != nil {
		t.Fatal(err)
	}
	eligibility, err := domain.NewNetworkEligibility(domain.NetworkJudgmentRequired, domain.EligibilityBasisReference{})
	if err != nil {
		t.Fatal(err)
	}
	store := &calendarRouteStore{}
	handler := application.NewCreateInitialRouteHandler(application.CreateInitialRouteDeps{
		Applicability: calendarApplicability{eligibility: eligibility},
		Evidence:      view,
		Store:         store,
		Log:           &calendarHandoffLog{digests: map[domain.RequestCorrelationID]string{}},
		Downstream:    calendarDownstream{},
		Identities:    &calendarIdentities{},
		Clock:         calendarClock{at: asOf},
	})
	result, err := handler.Handle(ctx, application.CreateInitialRouteCommand{
		Handoff: domain.RouteHandoffSpec{
			Correlation:        scalar(t, domain.NewRequestCorrelationID, "route-handoff-calendar"),
			TenantID:           tenant,
			CustomerAccountID:  scalar(t, domain.NewCustomerAccountID, "customer-1"),
			ShipmentRequestID:  scalar(t, domain.NewShipmentRequestID, "request-1"),
			AcceptanceDecision: scalar(t, domain.NewAcceptanceDecisionReference, "decision-1"),
			AcceptanceBaseline: scalar(t, domain.NewAcceptanceBaselineReference, "baseline-1"),
			Parcels:            []domain.DeclaredParcelID{scalar(t, domain.NewDeclaredParcelID, "parcel-1")},
			AcceptedAt:         asOf.Add(-time.Hour),
		},
		Purpose:    scalar(t, domain.NewServicePurpose, "NETWORK_SERVICE"),
		Resolution: scalar(t, domain.NewCommercialResolutionReference, "RES-1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if flip.calls < 3 || !flip.wrote {
		t.Fatalf("calls=%d wrote=%v，提交前没有按新修订重读", flip.calls, flip.wrote)
	}
	if result.Parcels()[0].Outcome() == application.ParcelRouteFormed {
		t.Fatal("成本缺席却形成了计划")
	}
	if store.saved != 0 && store.revision == flip.before {
		t.Fatal("提交了换代前那一版")
	}
}

type flipOnSecondRead struct {
	inner      *adapter.NetworkCatalog
	transactor bentoapp.Transactor
	tenant     domain.TenantID
	effective  time.Time
	calls      int
	wrote      bool
	before     string
}

func (flip *flipOnSecondRead) LoadDefinitionsAt(
	ctx context.Context, tenant domain.TenantID, asOf time.Time,
) (ports.NetworkCatalogSnapshot, bool, error) {
	flip.calls++
	if flip.calls == 2 {
		snapshot, _, err := flip.inner.LoadDefinitionsAt(ctx, tenant, asOf)
		if err != nil {
			return ports.NetworkCatalogSnapshot{}, false, err
		}
		flip.before = snapshot.Revision.String()
		if err := flip.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			return flip.inner.RegisterNodeVersion(txCtx, flip.tenant, ports.NodeDefinitionVersion{
				Code: "node-extra", Version: 1, BusinessTimezone: "UTC", EffectiveFrom: flip.effective,
			})
		}); err != nil {
			return ports.NetworkCatalogSnapshot{}, false, err
		}
		flip.wrote = true
	}
	return flip.inner.LoadDefinitionsAt(ctx, tenant, asOf)
}

func seedCalendarNetwork(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	catalog *adapter.NetworkCatalog,
	tenant domain.TenantID,
	effective time.Time,
	withDestinationCalendar bool,
) {
	t.Helper()
	cutoff := 18 * 60
	processingSHA := 60
	processingLAX := 30
	buffer := 120
	within(t, transactor, ctx, func(txCtx context.Context) error {
		steps := []func() error{
			func() error {
				return catalog.RegisterNodeVersion(txCtx, tenant, ports.NodeDefinitionVersion{
					Code: "node-sha", Version: 1, BusinessTimezone: "Asia/Shanghai", EffectiveFrom: effective,
				})
			},
			func() error {
				return catalog.RegisterNodeVersion(txCtx, tenant, ports.NodeDefinitionVersion{
					Code: "node-lax", Version: 1, BusinessTimezone: "America/Los_Angeles", EffectiveFrom: effective,
				})
			},
			func() error {
				return catalog.RegisterConnectionVersion(txCtx, tenant, ports.ConnectionDefinitionVersion{
					Code: "conn-sha-lax", Version: 1, FromNode: "node-sha", ToNode: "node-lax",
					BusinessTimezone: "Asia/Shanghai", EffectiveFrom: effective,
				})
			},
			func() error {
				return catalog.RegisterLineVersion(txCtx, tenant, ports.LineDefinitionVersion{
					Code: "line-sha-lax", Version: 1, Segments: []string{"conn-sha-lax"},
					BusinessTimezone: "Asia/Shanghai", ApplicableScope: "NETWORK_SERVICE", EffectiveFrom: effective,
				})
			},
			func() error {
				return catalog.RegisterServiceAreaVersion(txCtx, tenant, ports.ServiceAreaDefinitionVersion{
					Code: "area-cn", Version: 1, EffectiveFrom: effective,
					HasCoverage: true, CoverageCountry: "CN", OriginNodes: []string{"node-sha"},
				})
			},
			func() error {
				return catalog.RegisterServiceAreaVersion(txCtx, tenant, ports.ServiceAreaDefinitionVersion{
					Code: "area-us", Version: 1, EffectiveFrom: effective,
					HasCoverage: true, CoverageCountry: "US", DestinationNodes: []string{"node-lax"},
				})
			},
			func() error {
				return catalog.RegisterServiceCalendarVersion(txCtx, tenant, ports.ServiceCalendarDefinitionVersion{
					TargetKind: ports.TargetNode, TargetCode: "node-sha", Version: 1, EffectiveFrom: effective,
					CutoffLocalMinute: &cutoff, ProcessingMinutes: &processingSHA,
				})
			},
			func() error {
				return catalog.RegisterServiceCalendarVersion(txCtx, tenant, ports.ServiceCalendarDefinitionVersion{
					TargetKind: ports.TargetConnection, TargetCode: "conn-sha-lax", Version: 1, EffectiveFrom: effective,
					BufferMinutes: &buffer,
				})
			},
			func() error {
				return catalog.RegisterRouteStrategyVersion(txCtx, tenant, ports.RouteStrategyDefinitionVersion{
					Code: "strategy-1", Version: 1, ApplicableScope: "NETWORK_SERVICE",
					RankingForm: domain.CostSingleDimensionRanking, EffectiveFrom: effective,
				})
			},
		}
		for _, step := range steps {
			if err := step(); err != nil {
				return err
			}
		}
		if !withDestinationCalendar {
			return nil
		}
		return catalog.RegisterServiceCalendarVersion(txCtx, tenant, ports.ServiceCalendarDefinitionVersion{
			TargetKind: ports.TargetNode, TargetCode: "node-lax", Version: 1, EffectiveFrom: effective,
			ProcessingMinutes: &processingLAX,
		})
	})
}

func calendarRouteKey(t *testing.T, tenant domain.TenantID) domain.InitialRouteJudgmentKey {
	t.Helper()
	return domain.InitialRouteJudgmentKey{
		TenantID:           tenant,
		CustomerAccountID:  scalar(t, domain.NewCustomerAccountID, "customer-1"),
		ShipmentRequestID:  scalar(t, domain.NewShipmentRequestID, "request-1"),
		AcceptanceBaseline: scalar(t, domain.NewAcceptanceBaselineReference, "baseline-1"),
		DeclaredParcelID:   scalar(t, domain.NewDeclaredParcelID, "parcel-1"),
		ServicePurpose:     scalar(t, domain.NewServicePurpose, "NETWORK_SERVICE"),
	}
}

type calendarApplicability struct {
	eligibility domain.NetworkEligibility
}

func (double calendarApplicability) AssessRoutingApplicability(
	context.Context, domain.InitialRouteJudgmentKey, domain.CommercialResolutionReference,
) (domain.NetworkEligibility, error) {
	return double.eligibility, nil
}

type calendarRouteStore struct {
	saved    int
	revision string
}

func (store *calendarRouteStore) FindByKey(context.Context, domain.InitialRouteJudgmentKey) (ports.InitialRouteRecord, bool, error) {
	return ports.InitialRouteRecord{}, false, nil
}

func (store *calendarRouteStore) Save(_ context.Context, record ports.InitialRouteRecord) (ports.InitialRouteSaveOutcome, error) {
	store.saved++
	if record.HasNoRoute {
		store.revision = record.NoRoute.ViewRevision().String()
	}
	if record.HasPlan {
		store.revision = record.Plan.ViewRevision().String()
	}
	return ports.InitialRouteSaved, nil
}

type calendarHandoffLog struct {
	digests map[domain.RequestCorrelationID]string
}

func (log *calendarHandoffLog) FindDigest(context.Context, domain.TenantID, domain.RequestCorrelationID) (string, bool, error) {
	return "", false, nil
}

func (log *calendarHandoffLog) Append(_ context.Context, _ domain.TenantID, correlation domain.RequestCorrelationID, digest string) error {
	log.digests[correlation] = digest
	return nil
}

type calendarDownstream struct{}

func (calendarDownstream) HandOffInitialRoute(context.Context, ports.InitialRouteHandoffIntent) error {
	return nil
}

type calendarIdentities struct{ next int }

func (ids *calendarIdentities) NextRoutePlanVersionID(context.Context) (domain.RoutePlanVersionID, error) {
	ids.next++
	return domain.NewRoutePlanVersionID("plan-calendar")
}
