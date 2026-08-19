package main

import (
	"testing"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证 WIRE-CUSTOMER-VIEW：`visibility-exception.tracking-projection.derived` 已被
// 本进程接住——第一拍源事实成投影并入队派生信封，第二拍派生信封成客户视图。账户维
// 经 PS 按包裹反查（ADR-0060 三格）：有已接受委托的包裹取回账户成视图；零行入账不派
// 生、不发明账户。披露策略空册即四维全部待确认（实例半边如实说等），不在装配层兜底。

const (
	deriveCustomerViewConsumerName = "visibility-exception/derive-customer-view-from-projection"
	customerViewPublishedType      = "visibility-exception.customer-view.published"
)

// Covers: 生产 wireDispatcher 两拍贯通——交接投影的派生信封被消费，已接受委托的包裹
// 反查出账户，客户视图落库且采用当前投影版本，发布意图入队。
func TestADerivedProjectionBecomesACustomerViewForTheAcceptedParcel(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	// PS 侧先有一份已接受委托申报 SYN-PARCEL-01（账户 SYN-CUSTOMER-01）——反查的
	// 「恰一行」格。接受信封会一并入队，停在 NR 证据未配置的未决上，不占 published。
	fixture.submit(t, ctx)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.Outcome() != psapplication.AcceptanceDecided {
		t.Fatalf("接受未形成：outcome = %q", result.Outcome())
	}

	recordRegisteredTransportHandover(t, fixture)

	published, err := fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("第一拍应恰好定稿交接信封：published = %d", published)
	}

	tenant := mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String())
	parcel := mustVE(t, vedomain.NewTrackedParcelReference, transportHandoverObject)
	projections, err := vepostgres.NewProjections(fixture.db)
	if err != nil {
		t.Fatalf("构造投影读口：%v", err)
	}
	projection, found, err := projections.FindCurrent(ctx, tenant, parcel)
	if err != nil || !found {
		t.Fatalf("读当前投影：err=%v found=%v", err, found)
	}

	published, err = fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第二拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("第二拍应恰好定稿派生信封：published = %d；失败码 = %q",
			published, recordedFailureCode(t, fixture.db, projection.Version().String()))
	}

	views, err := vepostgres.NewCustomerViews(fixture.db)
	if err != nil {
		t.Fatalf("构造视图读口：%v", err)
	}
	customer := mustVE(t, vedomain.NewCustomerAccountReference, "SYN-CUSTOMER-01")
	view, found, err := views.FindCurrent(ctx, tenant, customer, parcel)
	if err != nil {
		t.Fatalf("读客户视图：%v", err)
	}
	if !found {
		t.Fatal("客户视图没落库——反查出的账户没接进派生")
	}
	if view.BasedOn() != projection.Version() {
		t.Fatalf("视图采用版本 = %s, want %s", view.BasedOn(), projection.Version())
	}
	// 披露策略空册：四维全部待确认，如实说等，不虚构可见性（实例半边）。
	dimensions := view.Dimensions()
	for name, dimension := range map[string]vedomain.ViewDimension{
		"里程碑": dimensions.Milestones,
		"ETA": dimensions.ETA,
		"终局":  dimensions.Final,
		"说明":  dimensions.Note,
	} {
		if dimension.State() != vedomain.DimensionPendingConfirmation {
			t.Fatalf("%s 维 = %v, want 待确认（披露策略未配置）", name, dimension.State())
		}
	}

	if n := fixture.countInbox(t, deriveCustomerViewConsumerName, projection.Version().String()); n != 1 {
		t.Fatalf("视图消费 inbox 行数 = %d, want 1", n)
	}
	if n := fixture.countOutboxOfType(t, customerViewPublishedType); n != 1 {
		t.Fatalf("视图发布意图 = %d, want 1", n)
	}
}

// Covers: 反查三格之「零行」——没有已接受委托的包裹，派生信封入账收工：不派生视图、
// 不发明账户，投影照旧存在；未派生的记录就是消费账上这封已入账而无视图行的信封。
func TestADerivedProjectionWithoutAnAcceptedParcelLeavesNoCustomerView(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	recordRegisteredTransportHandover(t, fixture)

	published, err := fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("第一拍：published = %d", published)
	}

	tenant := mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String())
	parcel := mustVE(t, vedomain.NewTrackedParcelReference, transportHandoverObject)
	projections, err := vepostgres.NewProjections(fixture.db)
	if err != nil {
		t.Fatalf("构造投影读口：%v", err)
	}
	projection, found, err := projections.FindCurrent(ctx, tenant, parcel)
	if err != nil || !found {
		t.Fatalf("读当前投影：err=%v found=%v", err, found)
	}

	published, err = fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第二拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("零行应入账收工：published = %d；失败码 = %q",
			published, recordedFailureCode(t, fixture.db, projection.Version().String()))
	}

	if n := fixture.countSQL(t, `SELECT count(*) FROM visibility_exception.customer_view`); n != 0 {
		t.Fatalf("customer_view 行数 = %d, want 0——账户不得发明", n)
	}
	if n := fixture.countOutboxOfType(t, customerViewPublishedType); n != 0 {
		t.Fatalf("视图发布意图 = %d, want 0", n)
	}
	if n := fixture.countInbox(t, deriveCustomerViewConsumerName, projection.Version().String()); n != 1 {
		t.Fatalf("视图消费 inbox 行数 = %d, want 1——入账就是未派生的记录", n)
	}
}
