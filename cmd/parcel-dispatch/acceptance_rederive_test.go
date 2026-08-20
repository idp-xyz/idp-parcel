package main

import (
	"testing"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证 VE-008 票 03 的倒序主场景（UC-VE-008 AT-VE-169）：包裹全部源事实已入账后
// 货主客户账户关系才建立——客户归属确立（接受决定）时按当前有效投影形成客户视图，
// 不等待新的源事实。正序（先接受、后流转）下本消费者空转，由 SYN-V0 钉住。

// Covers: 倒序补派生贯通——源事实先入账成投影（其派生信封因反查零行入账收工、无
// 视图），委托随后接受，接受信封经 FanOut 的 VE 腿按（租户+委托）取清单、按当前投影
// 补派生出客户视图；NR 腿仍停在证据实例墙，VE 腿的提交不受它牵连，整封失败码仍是
// dispatch.consumer_undecided。重投由 VE 账本跳过，视图不翻倍。
func TestALateAcceptanceRederivesTheCustomerViewFromTheCurrentProjection(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	// 倒序的前半：包裹先流转。交接登记成源事实（第一拍），其投影派生信封在第二拍
	// 因「当前没有已接受委托声明这件对象」入账收工——没有账户，不得发明视图。
	recordRegisteredTransportHandover(t, fixture)

	published, err := fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("第一拍应恰好定稿交接信封：published = %d", published)
	}
	published, err = fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第二拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("第二拍应恰好定稿派生信封（零行入账收工）：published = %d", published)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM visibility_exception.customer_view`); n != 0 {
		t.Fatalf("归属未确立就有了 %d 行客户视图——账户不得发明", n)
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

	// 倒序的后半：委托这时才建立并接受（客户归属确立）。SYN-PARCEL-01 在其声明清单
	// 里，账户 SYN-CUSTOMER-01。
	fixture.submit(t, ctx)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.Outcome() != psapplication.AcceptanceDecided {
		t.Fatalf("接受未形成：outcome = %q", result.Outcome())
	}

	// 第三拍：接受信封进 FanOut。VE 腿补派生成功并在自己的事务里入账；NR 腿停在
	// 证据实例墙让整封发布失败——published 记 0，但视图已经在库里。
	published, err = fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第三拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("NR 腿证据未配置，接受信封不该定稿：published = %d", published)
	}
	if got := recordedFailureCode(t, fixture.db, synV0DecisionID); got != "dispatch.consumer_undecided" {
		t.Fatalf("failure_code = %q, want dispatch.consumer_undecided（仅剩 NR 一路未决）", got)
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
		t.Fatal("客户视图没落库——归属确立没有触发补派生（AT-VE-169）")
	}
	if view.BasedOn() != projection.Version() {
		t.Fatalf("视图采用版本 = %s, want %s（按当前投影形成，不等新源事实）",
			view.BasedOn(), projection.Version())
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
	if n := fixture.countInbox(t, synV0VERederiveConsumer, synV0DecisionID); n != 1 {
		t.Fatalf("VE 补派生 inbox 行数 = %d, want 1", n)
	}
	if n := fixture.countOutboxOfType(t, customerViewPublishedType); n != 1 {
		t.Fatalf("视图发布意图 = %d, want 1", n)
	}

	// 第四拍：同一封接受信封重投。VE 腿由账本跳过、不重跑派生；NR 腿照旧未决。
	// 视图不翻倍，发布意图也不多一份。
	published, err = fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第四拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("重拍定稿了 %d 条", published)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM visibility_exception.customer_view`); n != 1 {
		t.Fatalf("customer_view 行数 = %d, want 1——重投不得再派生一版", n)
	}
	if n := fixture.countOutboxOfType(t, customerViewPublishedType); n != 1 {
		t.Fatalf("重拍后视图发布意图 = %d, want 仍为 1", n)
	}
}
