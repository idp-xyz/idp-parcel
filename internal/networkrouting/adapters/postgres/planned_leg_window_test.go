package postgres_test

import (
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证按计划履约段引用取段窗口的窄读口（ADR-0131 决定一 / 二，票
// nr-route-evidence-views/03）：初始计划的段与改路新计划的段都按各判断口的装载纪律重建后取段；
// 版本不存在 / 序位越界 / 跨租户三种情形答 found=false 且 err=nil——「没找到」是业务答案不是错误；
// TF 搬运的拼写解析回来就能直接读。夹具全部合成，不写任何真实节点与时间窗取值。

func newLegWindowFixture(t *testing.T) (
	*adapter.PlannedLegWindows, *adapter.InitialRoutes, *adapter.RouteReassessments, bentoapp.Transactor,
) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	windows, err := adapter.NewPlannedLegWindows(db)
	if err != nil {
		t.Fatalf("构造段窗口读口：%v", err)
	}
	routes, err := adapter.NewInitialRoutes(db)
	if err != nil {
		t.Fatalf("构造初始路由库：%v", err)
	}
	reassessments, err := adapter.NewRouteReassessments(db)
	if err != nil {
		t.Fatalf("构造复核库：%v", err)
	}
	return windows, routes, reassessments, db.Transactor()
}

func legReference(t *testing.T, version string, ordinal int) domain.PlannedLegReference {
	t.Helper()
	reference, err := domain.NewPlannedLegReference(scalar(t, domain.NewRoutePlanVersionID, version), ordinal)
	if err != nil {
		t.Fatalf("构造段引用 %s#%d：%v", version, ordinal, err)
	}
	return reference
}

// Covers: 初始路由判断行 plan 列里的段——按 plan_version 对上后经 FormInitialRoutePlan 重建再取段；
// 交回的是那一段自己的窗口（含形成依据），不是计划头部或别的段的。引用走的是 TF 会搬运的那个串：
// 解析回来直接读，拼写往返在读面上也成立。
func TestAPlannedLegWindowIsReadFromTheInitialPlanByReference(t *testing.T) {
	windows, routes, _, transactor := newLegWindowFixture(t)
	ctx := t.Context()

	key := routeKey(t, "tenant-1", "parcel-1")
	saveRoute(t, transactor, ctx, routes, ports.InitialRouteRecord{
		Key: key, Plan: formedPlan(t, key, "RPV-0001"), HasPlan: true,
	})

	reference, err := domain.ParsePlannedLegReference("RPV-0001#2")
	if err != nil {
		t.Fatalf("解析段引用：%v", err)
	}
	if reference != legReference(t, "RPV-0001", 2) {
		t.Fatalf("解析出的引用 = %+v，与构造门造的不等", reference)
	}

	window, found, err := windows.LoadPlannedLegWindow(ctx, key.TenantID, reference)
	if err != nil || !found {
		t.Fatalf("按引用读第二段：found=%v err=%v", found, err)
	}
	// formedPlan 的第二段窗口自 judgedAt+8h 起、长 4h、依据 calendar/v1——读回必须是这一段而不是首段。
	if !window.Earliest().Equal(routeJudgedAt.Add(8*time.Hour)) ||
		!window.Latest().Equal(routeJudgedAt.Add(12*time.Hour)) ||
		window.Basis().String() != "calendar/v1" {
		t.Fatalf("第二段窗口 = [%v, %v] 依据 %s", window.Earliest(), window.Latest(), window.Basis())
	}

	first, found, err := windows.LoadPlannedLegWindow(ctx, key.TenantID, legReference(t, "RPV-0001", 1))
	if err != nil || !found || !first.Earliest().Equal(routeJudgedAt.Add(2*time.Hour)) {
		t.Fatalf("按引用读首段：found=%v err=%v earliest=%v", found, err, first.Earliest())
	}
}

// Covers: 改路成的新计划住在复核判断行的 new_plan 列（route_reassessment.go 自注「新计划归 new_plan
// 列独家拥有」）——同一读口按新版本的引用也能取到段；而钉在旧版本上的引用仍交回旧版本那一段的内容
// （ADR-0131 决定二：取的是引用所钉的那一版，被替代不折进窗口答案）。
func TestAPlannedLegWindowIsReadFromTheReroutedNewPlanByReference(t *testing.T) {
	windows, routes, reassessments, transactor := newLegWindowFixture(t)
	ctx := t.Context()

	key := routeKey(t, "tenant-1", "parcel-1")
	saveRoute(t, transactor, ctx, routes, ports.InitialRouteRecord{
		Key: key, Plan: formedPlan(t, key, "RPV-0001"), HasPlan: true,
	})
	correlation := scalar(t, domain.NewRequestCorrelationID, "trigger-1")
	saveReassessment(t, transactor, ctx, reassessments, correlation, reroutedRecord(t, key, correlation))

	window, found, err := windows.LoadPlannedLegWindow(ctx, key.TenantID, legReference(t, "RPV-0002", 1))
	if err != nil || !found {
		t.Fatalf("按新版本引用读首段：found=%v err=%v", found, err)
	}
	if !window.Earliest().Equal(routeJudgedAt.Add(2*time.Hour)) || window.Basis().String() != "calendar/v1" {
		t.Fatalf("新计划首段窗口 = [%v, %v] 依据 %s", window.Earliest(), window.Latest(), window.Basis())
	}

	superseded, found, err := windows.LoadPlannedLegWindow(ctx, key.TenantID, legReference(t, "RPV-0001", 2))
	if err != nil || !found || !superseded.Earliest().Equal(routeJudgedAt.Add(8*time.Hour)) {
		t.Fatalf("被替代版本的引用应仍交回它自己那一段：found=%v err=%v earliest=%v", found, err, superseded.Earliest())
	}
}

// Covers: 版本不在本租户下——两处判断行都对不上——答 found=false 且 err=nil。一条指错的引用重跑
// 一万次也不会长出那一段来，它不是`读不到`（ADR-0131 决定三的读法）。
func TestAnUnknownPlanVersionAnswersNotFoundNotAnError(t *testing.T) {
	windows, routes, _, transactor := newLegWindowFixture(t)
	ctx := t.Context()

	key := routeKey(t, "tenant-1", "parcel-1")
	saveRoute(t, transactor, ctx, routes, ports.InitialRouteRecord{
		Key: key, Plan: formedPlan(t, key, "RPV-0001"), HasPlan: true,
	})

	if _, found, err := windows.LoadPlannedLegWindow(ctx, key.TenantID, legReference(t, "RPV-9999", 1)); err != nil || found {
		t.Fatalf("不存在的版本：found=%v err=%v，want false, nil", found, err)
	}
}

// Covers: 版本对上、序位越出该版本段链——InitialRoutePlan.LegAt 答没有，读口照实答 found=false 且
// err=nil；初始计划与改路新计划两处同判。
func TestAnOrdinalBeyondTheLegChainAnswersNotFoundNotAnError(t *testing.T) {
	windows, routes, reassessments, transactor := newLegWindowFixture(t)
	ctx := t.Context()

	key := routeKey(t, "tenant-1", "parcel-1")
	saveRoute(t, transactor, ctx, routes, ports.InitialRouteRecord{
		Key: key, Plan: formedPlan(t, key, "RPV-0001"), HasPlan: true,
	})
	correlation := scalar(t, domain.NewRequestCorrelationID, "trigger-1")
	saveReassessment(t, transactor, ctx, reassessments, correlation, reroutedRecord(t, key, correlation))

	for _, version := range []string{"RPV-0001", "RPV-0002"} {
		if _, found, err := windows.LoadPlannedLegWindow(ctx, key.TenantID, legReference(t, version, 3)); err != nil || found {
			t.Errorf("%s 第三段（段链只有两段）：found=%v err=%v，want false, nil", version, found, err)
		}
	}
}

// Covers: 租户条件进每条语句（ADR-0003）——他租户拿同一个版本标识按引用读，两处判断行都零行，
// 答 found=false 且 err=nil，不区分「不存在」与「属于另一个租户」。
func TestPlannedLegWindowsAreInvisibleAcrossTenants(t *testing.T) {
	windows, routes, reassessments, transactor := newLegWindowFixture(t)
	ctx := t.Context()

	key := routeKey(t, "tenant-1", "parcel-1")
	saveRoute(t, transactor, ctx, routes, ports.InitialRouteRecord{
		Key: key, Plan: formedPlan(t, key, "RPV-0001"), HasPlan: true,
	})
	correlation := scalar(t, domain.NewRequestCorrelationID, "trigger-1")
	saveReassessment(t, transactor, ctx, reassessments, correlation, reroutedRecord(t, key, correlation))

	other := scalar(t, domain.NewTenantID, "tenant-b")
	for _, version := range []string{"RPV-0001", "RPV-0002"} {
		if _, found, err := windows.LoadPlannedLegWindow(ctx, other, legReference(t, version, 1)); err != nil || found {
			t.Errorf("他租户按 %s 读到了本租户的段：found=%v err=%v", version, found, err)
		}
	}
}
