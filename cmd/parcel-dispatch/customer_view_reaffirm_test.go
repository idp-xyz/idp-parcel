package main

import (
	"context"
	"testing"
	"time"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证客户视图换不换代的生产链半边：同一包裹第二份已接受事实使投影换版之后，生产
// wireDispatcher 派生出的视图只在四维变了时换代（VE CONTEXT「客户全程追踪视图的版本由披露
// 更正与替代关系驱动，不与内部投影版本一一对应；投影重新派生本身不要求形成新的客户视图版本」）。

const secondHandoverScope = "SYN-SCOPE-02"

// Covers: 四维不变——披露策略空册，两版投影派生出的四维都是全部待确认：视图仍是同一版、
// 版本行一行，当前版的确认依据跟上第二版投影、形成时依据仍是第一版，对客视图发布意图仍是一份。
func TestASecondProjectionWithUnchangedDimensionsKeepsTheCustomerViewVersion(t *testing.T) {
	fixture := newAcceptedCustomerViewFixture(t)

	first := deriveViewFromHandover(t, fixture, transportHandoverScope, transportHandoverVersion)
	second := deriveViewFromHandover(t, fixture, secondHandoverScope, "SYN-HANDOVER-V2")
	if first.projection.Version() == second.projection.Version() {
		t.Fatal("第二份交接没有让投影换版")
	}

	view := currentCustomerView(t, fixture)
	if view.Version() != first.view.Version() {
		t.Fatalf("视图版本 = %s，want 仍是 %s——四维未变不得换代", view.Version(), first.view.Version())
	}
	if view.ConfirmedOn() != second.projection.Version() || view.BasedOn() != first.projection.Version() {
		t.Fatalf("确认依据 = %s 形成时依据 = %s，want %s / %s",
			view.ConfirmedOn(), view.BasedOn(), second.projection.Version(), first.projection.Version())
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM visibility_exception.customer_view_version`); n != 1 {
		t.Fatalf("customer_view_version 行数 = %d, want 1", n)
	}
	if n := fixture.countOutboxOfType(t, customerViewPublishedType); n != 1 {
		t.Fatalf("视图发布意图 = %d, want 仍为 1", n)
	}
}

// Covers: 四维变化——策略准许展示里程碑，第一版投影里交接未归类（映射未登），里程碑一维
// 答待确认；登记演示映射后第二份交接使两条都归类，里程碑一维转为展示：视图换代、指回前版，
// 版本行两行，对客视图发布意图两份。
func TestASecondProjectionThatChangesTheDimensionsSupersedesTheCustomerView(t *testing.T) {
	fixture := newAcceptedCustomerViewFixture(t)
	registerMilestonesShownPolicy(t, fixture)

	first := deriveViewFromHandover(t, fixture, transportHandoverScope, transportHandoverVersion)
	if state := first.view.Dimensions().Milestones.State(); state != vedomain.DimensionPendingConfirmation {
		t.Fatalf("第一版里程碑维 = %v, want 待确认（投影里没有已归类条目）", state)
	}

	registerDemoMilestoneMapping(t, fixture)
	second := deriveViewFromHandover(t, fixture, secondHandoverScope, "SYN-HANDOVER-V2")

	view := currentCustomerView(t, fixture)
	if view.Version() == first.view.Version() {
		t.Fatal("四维变了却没换代")
	}
	if prior, superseding := view.PriorVersion(); !superseding || prior != first.view.Version() {
		t.Fatalf("前版 = %s（%v），want 指回 %s", prior, superseding, first.view.Version())
	}
	if view.BasedOn() != second.projection.Version() {
		t.Fatalf("新版形成时依据 = %s, want %s", view.BasedOn(), second.projection.Version())
	}
	if state := view.Dimensions().Milestones.State(); state != vedomain.DimensionShown {
		t.Fatalf("第二版里程碑维 = %v, want 展示", state)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM visibility_exception.customer_view_version`); n != 2 {
		t.Fatalf("customer_view_version 行数 = %d, want 2", n)
	}
	if n := fixture.countOutboxOfType(t, customerViewPublishedType); n != 2 {
		t.Fatalf("视图发布意图 = %d, want 2", n)
	}
}

type derivedView struct {
	projection vedomain.TrackingProjection
	view       vedomain.CustomerTrackingView
}

// newAcceptedCustomerViewFixture 先让 PS 有一份已接受委托申报 SYN-PARCEL-01（账户
// SYN-CUSTOMER-01）——视图派生反查账户的「恰一行」格。接受信封停在 NR 证据未配置的未决上，
// 不占 published。
func newAcceptedCustomerViewFixture(t *testing.T) *synVerticalFixture {
	t.Helper()
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()
	fixture.submit(t, ctx)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.Outcome() != psapplication.AcceptanceDecided {
		t.Fatalf("接受未形成：outcome = %q", result.Outcome())
	}
	return fixture
}

// deriveViewFromHandover 落一份交接登记并走两拍：第一拍成投影并入队派生信封，第二拍派生信封
// 成客户视图。
func deriveViewFromHandover(t *testing.T, fixture *synVerticalFixture, scope, version string) derivedView {
	t.Helper()
	ctx := t.Context()
	recordRegisteredTransportHandoverIn(t, fixture, scope, version)
	for beat := 1; beat <= 2; beat++ {
		published, err := fixture.beat.DispatchOnce(ctx)
		if err != nil {
			t.Fatalf("%s 第 %d 拍：%v", scope, beat, err)
		}
		if published != 1 {
			t.Fatalf("%s 第 %d 拍 published = %d, want 1", scope, beat, published)
		}
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
	return derivedView{projection: projection, view: currentCustomerView(t, fixture)}
}

func currentCustomerView(t *testing.T, fixture *synVerticalFixture) vedomain.CustomerTrackingView {
	t.Helper()
	views, err := vepostgres.NewCustomerViews(fixture.db)
	if err != nil {
		t.Fatalf("构造视图读口：%v", err)
	}
	view, found, err := views.FindCurrent(t.Context(),
		mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String()),
		mustVE(t, vedomain.NewCustomerAccountReference, "SYN-CUSTOMER-01"),
		mustVE(t, vedomain.NewTrackedParcelReference, transportHandoverObject))
	if err != nil || !found {
		t.Fatalf("读客户视图：err=%v found=%v", err, found)
	}
	return view
}

// registerMilestonesShownPolicy 给夹具租户的 SYN-CUSTOMER-01 登一版披露策略：里程碑准许展示，
// 其余三维待确认。策略内容属租户取值，这里直接写行，证的是派生读得对。
func registerMilestonesShownPolicy(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	tenant := fixture.identity.TenantID().String()
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		executor, err := fixture.db.RequireExecutor(txCtx)
		if err != nil {
			return err
		}
		if _, err := executor.Exec(txCtx,
			`INSERT INTO visibility_exception.disclosure_policy_version
				(tenant_id, policy_version, effective_from, effective_to, approved_by)
			 VALUES ($1, 'SYN-VE-DISCLOSE-CV', $2, NULL, 'SYN-OPS-TRACKING-LEAD')`,
			tenant, time.Now().UTC().Add(-24*time.Hour)); err != nil {
			return err
		}
		_, err = executor.Exec(txCtx,
			`INSERT INTO visibility_exception.disclosure_policy_entry
				(tenant_id, policy_version, customer_account_ref,
				 milestones_state, eta_state, final_state, note_state,
				 milestones_content, eta_content, final_content, note_content)
			 VALUES ($1, 'SYN-VE-DISCLOSE-CV', 'SYN-CUSTOMER-01',
			         'SHOWN', 'PENDING_CONFIRMATION', 'PENDING_CONFIRMATION', 'PENDING_CONFIRMATION',
			         'SYN-VE-CONTENT-PUBLIC-MILESTONES-V1', NULL, NULL, NULL)`,
			tenant)
		return err
	})
}
