package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证生产装配的「投影派生 → 客户视图」那一路在消费事务里守得住投影的新旧判断：旧投影那封信
// 判过「仍是当前版」之后，后继投影不能在它读客户视图之前整段提交，否则它按旧投影写回、把当前视图
// 带回已被替代的投影内容（VE CONTEXT「不再有效的内容不得继续显示为当前事实」）。

// customerViewReadBarrier 在它那只连接池上第一次碰客户视图当前标记时停住，等测试放行。这只池子
// 只装旧投影那一路，碰到那张表的第一条语句就是派生编排读当前视图——那时投影的新旧已经判完。
type customerViewReadBarrier struct {
	once    sync.Once
	reached chan struct{}
	release chan struct{}
}

func (barrier *customerViewReadBarrier) TraceQueryStart(
	ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData,
) context.Context {
	if strings.Contains(data.SQL, "customer_view_current") {
		barrier.once.Do(func() {
			close(barrier.reached)
			<-barrier.release
		})
	}
	return ctx
}

func (*customerViewReadBarrier) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// Covers: 策略准许展示里程碑，P1 里交接未归类、视图里程碑维待确认；登记演示映射后 P2 使里程碑转为
// 展示。P1 那封派生信封重投、停在判完新旧之后，P2 期间经生产派发两拍（成投影、成视图）——它要么
// 整段提交，要么被看见等在锁上，才放 P1 那一路。收工后当前视图依据 P2、里程碑展示，P1 不再是
// 当前事实，也不多出第三版。
func TestAStaleDerivedProjectionEnvelopeCannotCarryTheCustomerViewBack(t *testing.T) {
	fixture := newAcceptedCustomerViewFixture(t)
	registerMilestonesShownPolicy(t, fixture)
	first := deriveViewFromHandover(t, fixture, transportHandoverScope, transportHandoverVersion)
	registerDemoMilestoneMapping(t, fixture)
	recordRegisteredTransportHandoverIn(t, fixture, secondHandoverScope, "SYN-HANDOVER-V2")

	barrier := &customerViewReadBarrier{reached: make(chan struct{}), release: make(chan struct{})}
	stale := staleCustomerViewConsumer(t, fixture, barrier)
	watcher, err := pgx.ConnectConfig(t.Context(), fixture.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatalf("开观察连接：%v", err)
	}
	defer func() { _ = watcher.Close(context.Background()) }()

	tenant := fixture.identity.TenantID().String()
	envelope := redeliveryEnvelope(tenant, "stale-derived-"+first.projection.Version().String(),
		veinbox.TrackingProjectionDerivedEventType, transportHandoverObject,
		`{"tenantId":"`+tenant+`","parcel":"`+transportHandoverObject+`","versionId":"`+first.projection.Version().String()+`"}`)
	envelope.Source = "idp-parcel/visibility-exception"

	staleDone := make(chan error, 1)
	go func() { staleDone <- stale.Consume(t.Context(), envelope) }()
	select {
	case <-barrier.reached:
	case err := <-staleDone:
		t.Fatalf("P1 那封信没读到客户视图就收工：%v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("P1 那封信十秒内没读到客户视图")
	}

	freshDone := make(chan error, 1)
	go func() {
		for beat := 1; beat <= 2; beat++ {
			published, err := fixture.beat.DispatchOnce(t.Context())
			if err != nil {
				freshDone <- err
				return
			}
			if published != 1 {
				freshDone <- errors.New("P2 那一路有一拍没有发出恰好一封信")
				return
			}
		}
		freshDone <- nil
	}()

	var freshErr error
	freshFinished := false
	deadline := time.Now().Add(10 * time.Second)
	for !freshFinished && !lockWaiterSeenInThisDatabase(watcher) {
		if time.Now().After(deadline) {
			close(barrier.release)
			t.Fatal("P2 那一路十秒内既没提交、也没等在锁上")
		}
		select {
		case freshErr = <-freshDone:
			freshFinished = true
		case <-time.After(20 * time.Millisecond):
		}
	}
	close(barrier.release)

	if err := <-staleDone; err != nil {
		t.Fatalf("P1 那封信：%v", err)
	}
	if !freshFinished {
		freshErr = <-freshDone
	}
	if freshErr != nil {
		t.Fatalf("P2 那一路：%v", freshErr)
	}

	second := currentProjectionOf(t, fixture)
	if second.Version() == first.projection.Version() {
		t.Fatal("第二份交接没有让投影换版")
	}
	view := currentCustomerView(t, fixture)
	if view.BasedOn() != second.Version() || view.ConfirmedOn() != second.Version() {
		t.Fatalf("形成时依据 = %s 确认依据 = %s，want 都是 %s——旧投影那封信把视图带回了已被替代的投影",
			view.BasedOn(), view.ConfirmedOn(), second.Version())
	}
	if state := view.Dimensions().Milestones.State(); state != vedomain.DimensionShown {
		t.Fatalf("里程碑维 = %v，want 展示（P2 的内容）", state)
	}
	if prior, superseding := view.PriorVersion(); !superseding || prior != first.view.Version() {
		t.Fatalf("前版 = %s（%v），want 指回 %s", prior, superseding, first.view.Version())
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM visibility_exception.customer_view_version`); n != 2 {
		t.Fatalf("customer_view_version 行数 = %d, want 2", n)
	}
}

// staleCustomerViewConsumer 按生产装配另接一路客户视图派生，连向同一个库、只是走一只带屏障的
// 连接池。
func staleCustomerViewConsumer(
	t *testing.T, fixture *synVerticalFixture, barrier *customerViewReadBarrier,
) dispatch.Consumer {
	t.Helper()
	config := fixture.pool.Config().Copy()
	config.ConnConfig.Tracer = barrier
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatalf("开屏障连接池：%v", err)
	}
	t.Cleanup(pool.Close)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造屏障那一路的框架 DB：%v", err)
	}
	outboxStore, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	inboxStore, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	consumer, err := deriveCustomerViewConsumer(db, outboxStore, inboxStore, systemClock{})
	if err != nil {
		t.Fatalf("装配客户视图派生那一路：%v", err)
	}
	return consumer
}

// lockWaiterSeenInThisDatabase 答本测试库里有没有正等在别的事务上、还没拿到锁的会话。按会话所在库
// 筛：事务号锁不属任何库，实例上别的用例各有各的库，它们的等待不算。
func lockWaiterSeenInThisDatabase(watcher *pgx.Conn) bool {
	var waiting int
	err := watcher.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_locks AS l
		   JOIN pg_stat_activity AS a ON a.pid = l.pid
		  WHERE NOT l.granted AND a.datname = current_database()`,
	).Scan(&waiting)
	return err == nil && waiting > 0
}

func currentProjectionOf(t *testing.T, fixture *synVerticalFixture) vedomain.TrackingProjection {
	t.Helper()
	projections, err := vepostgres.NewProjections(fixture.db)
	if err != nil {
		t.Fatalf("构造投影读口：%v", err)
	}
	projection, found, err := projections.FindCurrent(t.Context(),
		mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String()),
		mustVE(t, vedomain.NewTrackedParcelReference, transportHandoverObject))
	if err != nil || !found {
		t.Fatalf("读当前投影：err=%v found=%v", err, found)
	}
	return projection
}
