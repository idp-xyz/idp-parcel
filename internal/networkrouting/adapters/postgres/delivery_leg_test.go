package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证计划带交付段（ADR-0217，票 demo-chain-gaps/64）：派送时长经日历版本落库读回；
// 交付节点那一版日历登了派送时长，初始路由证据的候选路径带交付段，终点是解析收件地址所用的服务区域版本，窗口是
// 离开交付节点的时刻加派送时长；没登就不成段。带交付段的计划落库、读回、按段引用取窗口、在查阅口逐段列出；
// 种类出现之前的旧计划行读回不变。夹具全部合成。

// deliveryCarried 是寄件 CN、收件 US 的地理投影：seedCalendarNetwork 的 area-us 覆盖收件侧。
func deliveryCarried() ports.RequestCarriedContent {
	return ports.RequestCarriedContent{Geo: domain.NewGeoResolutionProjection(
		domain.NewGeoResolutionSide("CN", true, "200000", true),
		domain.NewGeoResolutionSide("US", true, "90001", true),
	)}
}

// Covers: AT-NR-054、AT-NR-057——交付节点日历修订 2 登了派送时长 180 分钟：候选路径在节点间段之后带一段交付段，
// 起于 node-lax、止于覆盖收件地址的 area-us@1，窗口自离开 node-lax（12:30Z）起、长 180 分钟，依据指那一版
// 日历；候选的时间投影延到交付段终点。修订 2 登记时接续闭合修订 1，判断时点只有修订 2 适用。
func TestInitialRouteEvidenceEndsWithADeliveryLegWhenTheDeliveryNodeRegistersADuration(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-delivery")
	asOf := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	effective := asOf.Add(-24 * time.Hour)
	seedCalendarNetwork(t, transactor, ctx, catalog, tenant, effective, true)
	processing, delivery := 30, 180
	within(t, transactor, ctx, func(txCtx context.Context) error {
		return catalog.RegisterServiceCalendarVersion(txCtx, tenant, ports.ServiceCalendarDefinitionVersion{
			TargetKind: ports.TargetNode, TargetCode: "node-lax", Version: 2, EffectiveFrom: effective.Add(time.Hour),
			ProcessingMinutes: &processing, DeliveryMinutes: &delivery,
		})
	})

	snapshot, _, err := catalog.LoadDefinitionsAt(ctx, tenant, asOf)
	if err != nil {
		t.Fatalf("选版：%v", err)
	}
	var lax []ports.ServiceCalendarDefinitionVersion
	for _, calendar := range snapshot.Calendars {
		if calendar.TargetCode == "node-lax" {
			lax = append(lax, calendar)
		}
	}
	if len(lax) != 1 || lax[0].Version != 2 || lax[0].DeliveryMinutes == nil || *lax[0].DeliveryMinutes != 180 {
		t.Fatalf("node-lax 适用日历 = %+v，想要修订 2 带派送时长 180", lax)
	}

	view, err := application.NewCatalogInitialRouteEvidence(catalog, catalogReachCustoms{}, calendarClock{at: asOf})
	if err != nil {
		t.Fatal(err)
	}
	evidence, configured, err := view.LoadInitialRouteEvidence(ctx, calendarRouteKey(t, tenant), deliveryCarried())
	if err != nil || !configured {
		t.Fatalf("configured=%v err=%v", configured, err)
	}
	if len(evidence.Paths) != 1 || len(evidence.Paths[0].Legs) != 1 || !evidence.Paths[0].HasDelivery {
		t.Fatalf("候选路径 = %+v，想要一段节点间段加交付段", evidence.Paths)
	}
	leg := evidence.Paths[0].Delivery
	scope, isDelivery := leg.DeliveryScope()
	if !isDelivery || leg.Kind() != domain.DeliveryLeg || leg.From().String() != "node-lax" || scope.String() != "area-us@1" {
		t.Fatalf("交付段 = %s → %s（%v）", leg.From(), scope, leg.Kind())
	}
	departed := time.Date(2026, 8, 20, 12, 30, 0, 0, time.UTC)
	window := leg.Window()
	if !window.Earliest().Equal(departed) || !window.Latest().Equal(departed.Add(180*time.Minute)) ||
		window.Basis().String() != "NODE/node-lax@2" || leg.Responsible().String() != "area-us@1" {
		t.Fatalf("交付段窗口 = [%v, %v] 依据 %s 责任 %s", window.Earliest(), window.Latest(), window.Basis(), leg.Responsible())
	}
	if len(evidence.Projections) != 1 || !evidence.Projections[0].Window().Latest().Equal(departed.Add(180*time.Minute)) {
		t.Fatalf("时间投影 = %+v，想要延到交付段终点", evidence.Projections)
	}
}

// Covers: AT-NR-055——交付节点日历没登派送时长：候选路径止于交付节点，不补默认时长，投影止于离开交付节点。
func TestNoDeliveryDurationLeavesThePathEndingAtTheDeliveryNode(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	ctx := t.Context()
	tenant := scalar(t, domain.NewTenantID, "tenant-no-delivery")
	asOf := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	seedCalendarNetwork(t, transactor, ctx, catalog, tenant, asOf.Add(-24*time.Hour), true)

	view, err := application.NewCatalogInitialRouteEvidence(catalog, catalogReachCustoms{}, calendarClock{at: asOf})
	if err != nil {
		t.Fatal(err)
	}
	evidence, configured, err := view.LoadInitialRouteEvidence(ctx, calendarRouteKey(t, tenant), deliveryCarried())
	if err != nil || !configured {
		t.Fatalf("configured=%v err=%v", configured, err)
	}
	if len(evidence.Paths) != 1 || evidence.Paths[0].HasDelivery || len(evidence.Paths[0].PlanLegs()) != 1 {
		t.Fatalf("候选路径 = %+v，没登派送时长不该成交付段", evidence.Paths)
	}
	if latest := evidence.Projections[0].Window().Latest(); !latest.Equal(time.Date(2026, 8, 20, 12, 30, 0, 0, time.UTC)) {
		t.Fatalf("投影终点 = %v，想要离开交付节点的时刻", latest)
	}
}

// Covers: AT-NR-063 的库面半边——派送时长为负被迁移 0020 的 CHECK 挡住（受理门之外的最后一道）。
func TestANegativeDeliveryDurationIsRefusedByTheTable(t *testing.T) {
	catalog, transactor, _ := newNetworkCatalog(t)
	negative := -1
	err := transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return catalog.RegisterServiceCalendarVersion(txCtx, scalar(t, domain.NewTenantID, "tenant-negative"),
			ports.ServiceCalendarDefinitionVersion{
				TargetKind: ports.TargetNode, TargetCode: "node-x", Version: 1, EffectiveFrom: catalogAsOf,
				DeliveryMinutes: &negative,
			})
	})
	if err == nil {
		t.Fatal("负的派送时长落了库")
	}
}

// deliveryLegFixture 是同一个库上的初始路由库、段窗口读口与查阅口，外带直读 jsonb 用的库句柄。
type deliveryLegFixture struct {
	pool      *pgxpool.Pool
	db        *bentopg.DB
	routes    *adapter.InitialRoutes
	windows   *adapter.PlannedLegWindows
	catalogue *adapter.RoutePlanCatalogue
}

func newDeliveryLegFixture(t *testing.T) deliveryLegFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	fixture := deliveryLegFixture{pool: pool, db: db}
	if fixture.routes, err = adapter.NewInitialRoutes(db); err != nil {
		t.Fatal(err)
	}
	if fixture.windows, err = adapter.NewPlannedLegWindows(db); err != nil {
		t.Fatal(err)
	}
	if fixture.catalogue, err = adapter.NewRoutePlanCatalogue(db); err != nil {
		t.Fatal(err)
	}
	return fixture
}

// legKindsInJSON 直读计划 jsonb 里各段的 kind 键（缺席读成空串），证落库形状。
func (fixture deliveryLegFixture) legKindsInJSON(t *testing.T, key domain.InitialRouteJudgmentKey) []string {
	t.Helper()
	rows, err := fixture.pool.Query(t.Context(),
		`SELECT coalesce(leg->>'kind', '')
		   FROM network_routing.initial_route,
		        jsonb_array_elements(plan->'legs') WITH ORDINALITY AS legs(leg, ordinal)
		  WHERE tenant_id = $1 AND declared_parcel_id = $2
		  ORDER BY ordinal`,
		key.TenantID.String(), key.DeclaredParcelID.String())
	if err != nil {
		t.Fatalf("直读段种类：%v", err)
	}
	defer rows.Close()
	var kinds []string
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			t.Fatal(err)
		}
		kinds = append(kinds, kind)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return kinds
}

// planWithDeliveryLeg 是 formedPlan 的两段接一段交付段：起于 zone-c，止于 area-sg@1，窗口长 3 小时。
func planWithDeliveryLeg(t *testing.T, key domain.InitialRouteJudgmentKey, version string) domain.InitialRoutePlan {
	t.Helper()
	base := formedPlan(t, key, version)
	window, err := domain.NewPlannedTimeWindow(routeJudgedAt.Add(12*time.Hour), routeJudgedAt.Add(15*time.Hour),
		scalar(t, domain.NewWindowBasisReference, "NODE/zone-c@2"))
	if err != nil {
		t.Fatalf("交付段窗口：%v", err)
	}
	delivery, err := domain.NewDeliveryLeg(domain.DeliveryLegSpec{
		From:        scalar(t, domain.NewPlanNodeReference, "zone-c"),
		Scope:       scalar(t, domain.NewServiceAreaVersionReference, "area-sg@1"),
		Responsible: scalar(t, domain.NewResponsiblePartyReference, "area-sg@1"),
		Window:      window,
	})
	if err != nil {
		t.Fatalf("交付段：%v", err)
	}
	plan, err := domain.FormInitialRoutePlan(domain.InitialRoutePlanSpec{
		Key:           key,
		Version:       base.Version(),
		Selected:      base.SelectedCandidate(),
		Candidates:    base.Candidates(),
		Legs:          append(base.Legs(), delivery),
		Strategy:      base.Strategy(),
		ViewRevision:  base.ViewRevision(),
		JudgedAt:      base.JudgedAt(),
		EffectiveFrom: base.EffectiveFrom(),
	})
	if err != nil {
		t.Fatalf("带交付段的计划：%v", err)
	}
	return plan
}

// Covers: AT-NR-060——带交付段的计划落库读回不变；按段引用取窗口，交付段 `#3` 交回它自己的窗口，越过段链的 `#4`
// 答未找到。jsonb 里只有交付段带种类，节点间段与种类出现之前的行同形。
func TestADeliveryLegRoundTripsAndIsReadByItsReference(t *testing.T) {
	fixture := newDeliveryLegFixture(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")
	saveRoute(t, fixture.db.Transactor(), ctx, fixture.routes, ports.InitialRouteRecord{
		Key: key, Plan: planWithDeliveryLeg(t, key, "RPV-0064"), HasPlan: true,
	})

	record, found, err := fixture.routes.FindByKey(ctx, key)
	if err != nil || !found {
		t.Fatalf("读回：found=%v err=%v", found, err)
	}
	legs := record.Plan.Legs()
	if len(legs) != 3 || legs[2].Kind() != domain.DeliveryLeg || legs[0].Kind() != domain.BetweenNodesLeg ||
		!legs[1].Opaque() {
		t.Fatalf("读回段链 = %+v", legs)
	}
	if scope, _ := legs[2].DeliveryScope(); scope.String() != "area-sg@1" || legs[2].From().String() != "zone-c" {
		t.Fatalf("读回交付段 = %s → %s", legs[2].From(), scope)
	}

	delivery, found, err := fixture.windows.LoadPlannedLegWindow(ctx, key.TenantID, legReference(t, "RPV-0064", 3))
	if err != nil || !found {
		t.Fatalf("按引用读交付段：found=%v err=%v", found, err)
	}
	if !delivery.Earliest().Equal(routeJudgedAt.Add(12*time.Hour)) || !delivery.Latest().Equal(routeJudgedAt.Add(15*time.Hour)) ||
		delivery.Basis().String() != "NODE/zone-c@2" {
		t.Fatalf("交付段窗口 = [%v, %v] 依据 %s", delivery.Earliest(), delivery.Latest(), delivery.Basis())
	}
	if _, found, err := fixture.windows.LoadPlannedLegWindow(ctx, key.TenantID, legReference(t, "RPV-0064", 4)); err != nil || found {
		t.Fatalf("交付段之后的序位：found=%v err=%v，想要未找到", found, err)
	}

	if kinds := fixture.legKindsInJSON(t, key); len(kinds) != 3 || kinds[0] != "" || kinds[1] != "" || kinds[2] != "DELIVERY" {
		t.Fatalf("jsonb 段种类 = %q，想要只有交付段带种类", kinds)
	}
}

// Covers: AT-NR-061——种类出现之前落库的计划行（段没有 kind 键）读回不变：全部是节点间段，节点序列照旧。直插一行
// 旧形状的 jsonb，不经今天的写口，免得写口改了形状这里跟着变而看不出来。
func TestAnOldPlanRowWithoutLegKindsReadsBackUnchanged(t *testing.T) {
	fixture := newDeliveryLegFixture(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-old")
	oldPlan := `{"version":"RPV-0063","selected":"candidate-1",
		"candidates":[{"id":"candidate-1","outcome":"QUALIFIED"}],
		"legs":[
			{"from":"hub-a","to":"hub-b","responsible":"carrier-1","earliest":"2026-08-20T12:00:00Z","latest":"2026-08-20T16:00:00Z","window_basis":"calendar/v1"},
			{"from":"hub-b","to":"zone-c","responsible":"partner-2","earliest":"2026-08-20T18:00:00Z","latest":"2026-08-20T22:00:00Z","window_basis":"calendar/v1","opaque":true}],
		"strategy":"strategy/v1","view_revision":"netview-42",
		"judged_at":"2026-08-20T10:00:00Z","effective_from":"2026-08-20T11:00:00Z"}`
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO network_routing.initial_route
			(tenant_id, customer_account_id, shipment_request_id, acceptance_baseline,
			 declared_parcel_id, service_purpose, conclusion, plan_version, plan, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 'ROUTE_FORMED', 'RPV-0063', $7::jsonb, now())`,
		key.TenantID.String(), key.CustomerAccountID.String(), key.ShipmentRequestID.String(),
		key.AcceptanceBaseline.String(), key.DeclaredParcelID.String(), key.ServicePurpose.String(), oldPlan,
	); err != nil {
		t.Fatalf("插旧形状计划行：%v", err)
	}

	record, found, err := fixture.routes.FindByKey(ctx, key)
	if err != nil || !found {
		t.Fatalf("读回：found=%v err=%v", found, err)
	}
	legs := record.Plan.Legs()
	if len(legs) != 2 || !legs[1].Opaque() {
		t.Fatalf("旧行段链 = %+v", legs)
	}
	for index, leg := range legs {
		if leg.Kind() != domain.BetweenNodesLeg {
			t.Fatalf("旧行第 %d 段读成了 %s", index+1, leg.Kind())
		}
	}
	if nodes := record.Plan.Nodes(); len(nodes) != 3 || nodes[2].String() != "zone-c" {
		t.Fatalf("旧行节点序列 = %v", nodes)
	}

	// 今天的写口写只有节点间段的计划，与旧行同形：段上没有 kind 键。
	fresh := routeKey(t, "tenant-1", "parcel-fresh")
	saveRoute(t, fixture.db.Transactor(), ctx, fixture.routes, ports.InitialRouteRecord{
		Key: fresh, Plan: formedPlan(t, fresh, "RPV-0065"), HasPlan: true,
	})
	if kinds := fixture.legKindsInJSON(t, fresh); len(kinds) != 2 || kinds[0] != "" || kinds[1] != "" {
		t.Fatalf("只有节点间段的计划 jsonb 段种类 = %q，想要与旧行同形", kinds)
	}
}

// Covers: AT-NR-062——路由计划查阅口的成计划行逐段列出引用、种类、起讫与窗口；末段是交付段，终点是交付范围。
func TestTheRoutePlanCatalogueListsLegsEndingWithTheDeliveryLeg(t *testing.T) {
	fixture := newDeliveryLegFixture(t)
	ctx := t.Context()
	key := routeKey(t, "tenant-1", "parcel-1")
	saveRoute(t, fixture.db.Transactor(), ctx, fixture.routes, ports.InitialRouteRecord{
		Key: key, Plan: planWithDeliveryLeg(t, key, "RPV-0064"), HasPlan: true,
	})

	rows, err := fixture.catalogue.ListInitialRoutes(ctx, key.TenantID, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("上列：%d 行 err=%v", len(rows), err)
	}
	legs := rows[0].Legs
	if len(legs) != 3 {
		t.Fatalf("逐段 = %+v，想要三段", legs)
	}
	first, last := legs[0], legs[2]
	if first.Reference != "RPV-0064#1" || first.Kind != "BETWEEN_NODES" || first.From != "hub-a" || first.To != "hub-b" {
		t.Fatalf("首段 = %+v", first)
	}
	if last.Reference != "RPV-0064#3" || last.Kind != "DELIVERY" || last.From != "zone-c" || last.To != "area-sg@1" ||
		last.Responsible != "area-sg@1" || last.WindowBasis != "NODE/zone-c@2" ||
		!last.Earliest.Equal(routeJudgedAt.Add(12*time.Hour)) || !last.Latest.Equal(routeJudgedAt.Add(15*time.Hour)) {
		t.Fatalf("末段 = %+v", last)
	}
}
