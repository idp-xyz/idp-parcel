package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件对真实 PostgreSQL 16 证 ETA / 缺口 / 客户通知三库：当前版 UPSERT、缺口
// 键含窗口规则版本、通知按披露身份三维幂等、过程节点 jsonb 往返、形状 CHECK、
// 租户隔离由 SQL 条件承担。断言一律在事务闭包外。

var etaGapBaseAt = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

type etaGapNotificationFixture struct {
	etas          *adapter.ETAs
	gaps          *adapter.VisibilityGaps
	notifications *adapter.CustomerNotifications
	transactor    bentoapp.Transactor
	pool          *pgxpool.Pool
}

func newETAGapNotificationFixture(t *testing.T) *etaGapNotificationFixture {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	etas, err := adapter.NewETAs(db)
	if err != nil {
		t.Fatalf("构造 ETA 库：%v", err)
	}
	gaps, err := adapter.NewVisibilityGaps(db)
	if err != nil {
		t.Fatalf("构造缺口库：%v", err)
	}
	notifications, err := adapter.NewCustomerNotifications(db)
	if err != nil {
		t.Fatalf("构造通知库：%v", err)
	}
	return &etaGapNotificationFixture{
		etas:          etas,
		gaps:          gaps,
		notifications: notifications,
		transactor:    db.Transactor(),
		pool:          pool,
	}
}

func (fixture *etaGapNotificationFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func etaGapValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

func formedETA(t *testing.T, version, inputs string) domain.ETAPrediction {
	t.Helper()
	eta, err := domain.FormETAPrediction(domain.ETAPredictionSpec{
		Version:     etaGapValue(t, domain.NewETAVersionID, version),
		Parcel:      etaGapValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		Milestone:   etaGapValue(t, domain.NewMilestoneReference, "DELIVERED"),
		Source:      domain.OperatorDerivedETA,
		Inputs:      etaGapValue(t, domain.NewPredictionInputsReference, inputs),
		Model:       etaGapValue(t, domain.NewPredictionModelReference, "eta-model/v1"),
		RangeFrom:   etaGapBaseAt.Add(24 * time.Hour),
		RangeTo:     etaGapBaseAt.Add(48 * time.Hour),
		Confidence:  etaGapValue(t, domain.NewConfidenceReference, "MEDIUM/history"),
		PredictedAt: etaGapBaseAt,
	})
	if err != nil {
		t.Fatalf("构造 ETA：%v", err)
	}
	return eta
}

func formedGap(t *testing.T, windowRule string) domain.VisibilityGap {
	t.Helper()
	windowEnd := etaGapBaseAt.Add(-time.Hour)
	gap, err := domain.FormVisibilityGap(
		etaGapValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		etaGapValue(t, domain.NewExpectedObservationReference, "LINEHAUL_ARRIVAL_SCAN"),
		etaGapValue(t, domain.NewObservationWindowReference, windowRule),
		windowEnd,
		etaGapBaseAt,
	)
	if err != nil {
		t.Fatalf("构造缺口：%v", err)
	}
	return gap
}

func discloseForNotify(t *testing.T) domain.DisclosureDecision {
	t.Helper()
	decision, err := domain.DecideDisclosure(
		etaGapValue(t, domain.NewEpisodeID, "episode-1"),
		etaGapValue(t, domain.NewCustomerAccountReference, "customer-1"),
		etaGapValue(t, domain.NewDisclosurePolicyReference, "disclosure-policy/v1"),
		domain.DiscloseToCustomer,
		etaGapValue(t, domain.NewDisclosureContentReference, "content/v1"),
		etaGapBaseAt,
	)
	if err != nil {
		t.Fatalf("构造披露决定：%v", err)
	}
	return decision
}

func generatedNotification(t *testing.T, id string) *domain.CustomerNotification {
	t.Helper()
	notification, err := domain.GenerateNotification(
		etaGapValue(t, domain.NewNotificationID, id),
		discloseForNotify(t),
		etaGapBaseAt.Add(24*time.Hour),
		etaGapValue(t, domain.NewNotificationChannelReference, "portal-message/v1"),
		etaGapValue(t, domain.NewDisclosurePolicyReference, "notify-policy/v1"),
		etaGapBaseAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("构造通知：%v", err)
	}
	return notification
}

func (fixture *etaGapNotificationFixture) saveETA(t *testing.T, ctx context.Context, tenant string, eta domain.ETAPrediction) {
	t.Helper()
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.etas.Save(txCtx, etaGapValue(t, domain.NewTenantID, tenant), eta)
	})
}

func (fixture *etaGapNotificationFixture) loadETA(t *testing.T, ctx context.Context, tenant string) (domain.ETAPrediction, bool) {
	t.Helper()
	eta, found, err := fixture.etas.FindCurrent(ctx,
		etaGapValue(t, domain.NewTenantID, tenant),
		etaGapValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		etaGapValue(t, domain.NewMilestoneReference, "DELIVERED"))
	if err != nil {
		t.Fatalf("读 ETA：%v", err)
	}
	return eta, found
}

func (fixture *etaGapNotificationFixture) saveGap(t *testing.T, ctx context.Context, tenant string, gap domain.VisibilityGap) {
	t.Helper()
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.gaps.Save(txCtx, etaGapValue(t, domain.NewTenantID, tenant), gap)
	})
}

func (fixture *etaGapNotificationFixture) loadGap(t *testing.T, ctx context.Context, tenant, windowRule string) (domain.VisibilityGap, bool) {
	t.Helper()
	gap, found, err := fixture.gaps.FindCurrent(ctx,
		etaGapValue(t, domain.NewTenantID, tenant),
		etaGapValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		etaGapValue(t, domain.NewExpectedObservationReference, "LINEHAUL_ARRIVAL_SCAN"),
		etaGapValue(t, domain.NewObservationWindowReference, windowRule))
	if err != nil {
		t.Fatalf("读缺口：%v", err)
	}
	return gap, found
}

func (fixture *etaGapNotificationFixture) saveNotification(t *testing.T, ctx context.Context, tenant string, notification *domain.CustomerNotification) {
	t.Helper()
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.notifications.Save(txCtx, etaGapValue(t, domain.NewTenantID, tenant), notification)
	})
}

func (fixture *etaGapNotificationFixture) loadNotification(t *testing.T, ctx context.Context, tenant string) (*domain.CustomerNotification, bool) {
	t.Helper()
	notification, found, err := fixture.notifications.FindByDisclosure(ctx,
		etaGapValue(t, domain.NewTenantID, tenant),
		discloseForNotify(t))
	if err != nil {
		t.Fatalf("读通知：%v", err)
	}
	return notification, found
}

// TestCurrentETARoundTripsAndRefreshReplacesTheRow 证当前版往返与刷新换版：同一
// （包裹+里程碑）只留一行，prior_version 指回前版。
func TestCurrentETARoundTripsAndRefreshReplacesTheRow(t *testing.T) {
	fixture := newETAGapNotificationFixture(t)
	ctx := t.Context()

	first := formedETA(t, "eta-1/v1", "inputs/v1")
	fixture.saveETA(t, ctx, "tenant-a", first)

	loaded, found := fixture.loadETA(t, ctx, "tenant-a")
	if !found || loaded.Version().String() != "eta-1/v1" || loaded.Inputs().String() != "inputs/v1" {
		t.Fatalf("首版往返失败：found=%v version=%s", found, loaded.Version())
	}
	if _, prior := loaded.PriorVersion(); prior {
		t.Fatal("首版带了指回")
	}

	refreshed, err := first.Refresh(domain.ETAPredictionSpec{
		Version:     etaGapValue(t, domain.NewETAVersionID, "eta-1/v2"),
		Parcel:      first.Parcel(),
		Milestone:   first.Milestone(),
		Source:      domain.CarrierProvidedETA,
		Inputs:      etaGapValue(t, domain.NewPredictionInputsReference, "inputs/v2"),
		Model:       first.Model(),
		RangeFrom:   etaGapBaseAt.Add(30 * time.Hour),
		RangeTo:     etaGapBaseAt.Add(50 * time.Hour),
		Confidence:  first.Confidence(),
		PredictedAt: etaGapBaseAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("刷新：%v", err)
	}
	fixture.saveETA(t, ctx, "tenant-a", refreshed)

	current, found := fixture.loadETA(t, ctx, "tenant-a")
	if !found || current.Version().String() != "eta-1/v2" {
		t.Fatalf("刷新后当前版 = %s found=%v", current.Version(), found)
	}
	prior, present := current.PriorVersion()
	if !present || prior.String() != "eta-1/v1" {
		t.Fatalf("指回 = %s present=%v", prior, present)
	}
	if current.Source() != domain.CarrierProvidedETA {
		t.Fatal("来源口径丢了")
	}

	var rows int
	if err := fixture.pool.QueryRow(ctx,
		`SELECT count(*) FROM visibility_exception.eta_prediction
		  WHERE tenant_id = 'tenant-a' AND parcel_ref = 'parcel-1'`).Scan(&rows); err != nil {
		t.Fatalf("数行：%v", err)
	}
	if rows != 1 {
		t.Fatalf("刷新后行数 = %d，库应只管当前版", rows)
	}
}

// TestNewWindowRuleKeepsTheOriginalGap 证缺口键含窗口规则版本：新窗口另成一行，
// 原判断仍在。
func TestNewWindowRuleKeepsTheOriginalGap(t *testing.T) {
	fixture := newETAGapNotificationFixture(t)
	ctx := t.Context()

	original := formedGap(t, "window-rule/v1")
	fixture.saveGap(t, ctx, "tenant-a", original)

	next := formedGap(t, "window-rule/v2")
	fixture.saveGap(t, ctx, "tenant-a", next)

	v1, found := fixture.loadGap(t, ctx, "tenant-a", "window-rule/v1")
	if !found || v1.WindowRule().String() != "window-rule/v1" {
		t.Fatal("原窗口判断被覆盖了")
	}
	v2, found := fixture.loadGap(t, ctx, "tenant-a", "window-rule/v2")
	if !found || v2.WindowRule().String() != "window-rule/v2" {
		t.Fatal("新窗口判断没立住")
	}

	// 同键重放不造第二行。
	fixture.saveGap(t, ctx, "tenant-a", original)
	var rows int
	if err := fixture.pool.QueryRow(ctx,
		`SELECT count(*) FROM visibility_exception.visibility_gap
		  WHERE tenant_id = 'tenant-a'`).Scan(&rows); err != nil {
		t.Fatalf("数行：%v", err)
	}
	if rows != 2 {
		t.Fatalf("缺口行数 = %d，想要新旧窗口各一行", rows)
	}
}

// TestNotificationRoundTripsMilestonesByDisclosureIdentity 证通知按披露三维定位，
// 过程节点分别记录往返。
func TestNotificationRoundTripsMilestonesByDisclosureIdentity(t *testing.T) {
	fixture := newETAGapNotificationFixture(t)
	ctx := t.Context()

	generated := generatedNotification(t, "notify-1")
	fixture.saveNotification(t, ctx, "tenant-a", generated)

	loaded, found := fixture.loadNotification(t, ctx, "tenant-a")
	if !found || loaded.ID().String() != "notify-1" {
		t.Fatalf("已生成往返失败：found=%v", found)
	}
	if len(loaded.Milestones()) != 1 || loaded.Milestones()[0] != domain.NotificationGenerated {
		t.Fatalf("节点 = %v", loaded.Milestones())
	}

	if err := generated.RecordMilestone(domain.NotificationSubmittedToChannel, etaGapBaseAt.Add(2*time.Minute)); err != nil {
		t.Fatalf("记提交：%v", err)
	}
	if err := generated.RecordMilestone(domain.NotificationFailed, etaGapBaseAt.Add(3*time.Minute)); err != nil {
		t.Fatalf("记失败：%v", err)
	}
	fixture.saveNotification(t, ctx, "tenant-a", generated)

	retried, found := fixture.loadNotification(t, ctx, "tenant-a")
	if !found {
		t.Fatal("追加节点后读不到通知")
	}
	milestones := retried.Milestones()
	if len(milestones) != 3 ||
		milestones[0] != domain.NotificationGenerated ||
		milestones[1] != domain.NotificationSubmittedToChannel ||
		milestones[2] != domain.NotificationFailed {
		t.Fatalf("过程节点被覆盖或乱序：%v", milestones)
	}
	if retried.Channel().String() != "portal-message/v1" {
		t.Fatal("渠道丢了")
	}
}

// TestETAGapNotificationChecksRejectBadShapes 证迁移 CHECK：倒置区间、未届满缺口、
// 非披露结论、缺 GENERATED 节点都拦下。
func TestETAGapNotificationChecksRejectBadShapes(t *testing.T) {
	fixture := newETAGapNotificationFixture(t)
	ctx := t.Context()

	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO visibility_exception.eta_prediction
			(tenant_id, parcel_ref, milestone_ref, version_id, source, inputs_ref,
			 model_ref, range_from, range_to, confidence_ref, predicted_at)
		 VALUES ('tenant-a', 'p', 'm', 'v', 'OPERATOR_DERIVED', 'i', 'm',
		         now(), now() - interval '1 hour', 'c', now())`); err == nil {
		t.Fatal("倒置区间的预测被库接受了")
	}

	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO visibility_exception.visibility_gap
			(tenant_id, parcel_ref, expectation_ref, window_rule_ref, window_end, formed_at)
		 VALUES ('tenant-a', 'p', 'e', 'w', now(), now() - interval '1 hour')`); err == nil {
		t.Fatal("未届满的缺口被库接受了")
	}

	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO visibility_exception.customer_notification
			(tenant_id, notification_id, customer_ref, episode_id, decided_at,
			 disclosure_policy_ref, disclosure_conclusion, content_ref, deadline,
			 channel_ref, obligation_ref, milestones)
		 VALUES ('tenant-a', 'n1', 'c', 'e', now(), 'p', 'NOT_YET_DISCLOSABLE', 'ct',
		         now(), 'ch', 'o',
		         '[{"milestone":"GENERATED","recordedAt":"2026-08-14T12:01:00Z"}]')`); err == nil {
		t.Fatal("非披露结论的通知被库接受了")
	}

	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO visibility_exception.customer_notification
			(tenant_id, notification_id, customer_ref, episode_id, decided_at,
			 disclosure_policy_ref, disclosure_conclusion, content_ref, deadline,
			 channel_ref, obligation_ref, milestones)
		 VALUES ('tenant-a', 'n2', 'c', 'e', now(), 'p', 'DISCLOSE', 'ct',
		         now(), 'ch', 'o',
		         '[{"milestone":"FAILED","recordedAt":"2026-08-14T12:01:00Z"}]')`); err == nil {
		t.Fatal("不以 GENERATED 起头的通知被库接受了")
	}
}

// TestETAGapNotificationsOfAnotherTenantAreInvisible 证租户隔离：同名包裹/披露
// 在另一租户一律不可见，同名键可各自成行。
func TestETAGapNotificationsOfAnotherTenantAreInvisible(t *testing.T) {
	fixture := newETAGapNotificationFixture(t)
	ctx := t.Context()

	fixture.saveETA(t, ctx, "tenant-a", formedETA(t, "eta-1/v1", "inputs/v1"))
	fixture.saveGap(t, ctx, "tenant-a", formedGap(t, "window-rule/v1"))
	fixture.saveNotification(t, ctx, "tenant-a", generatedNotification(t, "notify-1"))

	if _, found := fixture.loadETA(t, ctx, "tenant-b"); found {
		t.Fatal("跨租户 ETA 可见")
	}
	if _, found := fixture.loadGap(t, ctx, "tenant-b", "window-rule/v1"); found {
		t.Fatal("跨租户缺口可见")
	}
	if _, found := fixture.loadNotification(t, ctx, "tenant-b"); found {
		t.Fatal("跨租户通知可见")
	}

	fixture.saveETA(t, ctx, "tenant-b", formedETA(t, "eta-1/v1", "inputs/v1"))
	fixture.saveGap(t, ctx, "tenant-b", formedGap(t, "window-rule/v1"))
	fixture.saveNotification(t, ctx, "tenant-b", generatedNotification(t, "notify-1"))
}

// TestETAGapNotificationWritesRequireTransaction 证事务纪律：无事务写一律拒。
func TestETAGapNotificationWritesRequireTransaction(t *testing.T) {
	fixture := newETAGapNotificationFixture(t)
	ctx := t.Context()
	tenant := etaGapValue(t, domain.NewTenantID, "tenant-a")

	if err := fixture.etas.Save(ctx, tenant, formedETA(t, "eta-1/v1", "inputs/v1")); err == nil {
		t.Fatal("无事务 Save ETA 被接受了")
	}
	if err := fixture.gaps.Save(ctx, tenant, formedGap(t, "window-rule/v1")); err == nil {
		t.Fatal("无事务 Save 缺口被接受了")
	}
	if err := fixture.notifications.Save(ctx, tenant, generatedNotification(t, "notify-1")); err == nil {
		t.Fatal("无事务 Save 通知被接受了")
	}
}
