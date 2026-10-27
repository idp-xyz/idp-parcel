package parcelshipment_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	veidentity "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/identity"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/parcelshipment"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件对真实 PostgreSQL 证投影派生信封的新旧判断在消费事务里站得住：处理旧投影 P1 那封信的
// 事务已经判过「P1 仍是当前版」、还没读客户视图时，后继投影 P2 连同它自己那封信整段提交，P1 那一路
// 随后读到的视图已经跟上 P2，再按 P1 写回就把当前视图带回已被替代的投影（VE CONTEXT「客户全程
// 追踪视图只基于当前有效的全程追踪投影……形成」「不再有效的内容不得继续显示为当前事实」）。视图
// 写口的乐观锁对这一格不起作用：P1 那一路读到的当前版正是 P2 那一路刚写的那一版。
//
// 屏障停在 P1 那一路读完投影之后、进派生编排之前。P2 那一路要么整段提交，要么被看见等在锁上，
// 才放 P1 那一路——证的是 P2 确实被挡在 P1 判完新旧之后，而不是碰巧先后跑。

// heldViewDerive 在第一次进派生编排之前停住，等测试放行。只装在 P1 那一路上。
type heldViewDerive struct {
	inner   adapter.CustomerViewDeriveHandler
	reached chan struct{}
	release chan struct{}
}

func newHeldViewDerive(inner adapter.CustomerViewDeriveHandler) *heldViewDerive {
	return &heldViewDerive{inner: inner, reached: make(chan struct{}), release: make(chan struct{})}
}

func (held *heldViewDerive) Handle(
	ctx context.Context, command veapplication.DeriveCustomerViewCommand,
) (veapplication.DeriveCustomerViewResult, error) {
	close(held.reached)
	<-held.release
	return held.inner.Handle(ctx, command)
}

// notePerProjectionPolicy 让追踪说明一维的内容随投影版本而变，其余三维待确认：两版投影派生出的
// 四维必然不同，换代那一格才走得到。这是测试夹具，不是任何租户的披露策略。
type notePerProjectionPolicy struct{}

func (notePerProjectionPolicy) AssessDisclosure(
	_ context.Context, _ vedomain.CustomerAccountReference, projection vedomain.TrackingProjection,
) (veports.DisclosureAnswer, bool, error) {
	content, err := vedomain.NewViewContentReference("SYN-NOTE-" + projection.Version().String())
	if err != nil {
		return veports.DisclosureAnswer{}, false, err
	}
	pending := veports.DimensionDisclosure{State: vedomain.DimensionPendingConfirmation}
	return veports.DisclosureAnswer{
		Milestones: pending,
		ETA:        pending,
		Final:      pending,
		Note:       veports.DimensionDisclosure{State: vedomain.DimensionShown, Content: content},
	}, true, nil
}

type staleEnvelopeRace struct {
	pool        *pgxpool.Pool
	tenant      vedomain.TenantID
	projections *vepostgres.Projections
	views       *vepostgres.CustomerViews
	transactor  bentoapp.Transactor
	derive      *veapplication.DeriveCustomerViewHandler
	first       vedomain.TrackingProjection
	second      vedomain.TrackingProjection
}

func newStaleEnvelopeRace(t *testing.T, policy veports.DisclosurePolicyView) *staleEnvelopeRace {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	projections, err := vepostgres.NewProjections(db)
	if err != nil {
		t.Fatalf("构造投影库：%v", err)
	}
	views, err := vepostgres.NewCustomerViews(db)
	if err != nil {
		t.Fatalf("构造视图库：%v", err)
	}
	identities, err := veidentity.NewCustomerViewVersions()
	if err != nil {
		t.Fatalf("构造视图版本签发：%v", err)
	}
	first := builtProjection(t, "projection-1", "parcel-1")
	second, err := first.Rederive(finalValue(t, vedomain.NewProjectionVersionID, "projection-2"),
		first.Entries(), viewDerivedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("重派生投影：%v", err)
	}
	return &staleEnvelopeRace{
		pool:        pool,
		tenant:      finalValue(t, vedomain.NewTenantID, "tenant-1"),
		projections: projections,
		views:       views,
		transactor:  db.Transactor(),
		derive: veapplication.NewDeriveCustomerViewHandler(veapplication.DeriveCustomerViewDeps{
			Policy:        policy,
			ETAJudgements: emptyJudgementRead{},
			Views:         views,
			Identities:    identities,
			Downstream:    &viewDownstreamDouble{},
			Clock:         viewClock{at: viewDerivedAt.Add(2 * time.Minute)},
		}),
		first:  first,
		second: second,
	}
}

func (race *staleEnvelopeRace) subject(t *testing.T, derive adapter.CustomerViewDeriveHandler) *adapter.DeriveCustomerViewOnProjectionAdapter {
	t.Helper()
	subject, err := adapter.NewDeriveCustomerViewOnProjectionAdapter(race.projections, resolvedAccount(t), derive)
	if err != nil {
		t.Fatalf("构造消费处理方：%v", err)
	}
	return subject
}

func (race *staleEnvelopeRace) saveProjection(ctx context.Context, projection vedomain.TrackingProjection) error {
	return race.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return race.projections.Save(txCtx, race.tenant, projection)
	})
}

func (race *staleEnvelopeRace) consume(
	ctx context.Context, subject *adapter.DeriveCustomerViewOnProjectionAdapter, projection vedomain.TrackingProjection,
) error {
	return race.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return subject.HandleDerivedTrackingProjection(txCtx, derivedRef(projection.Version().String()))
	})
}

// interleave 让 P1 那一路停在读完投影之后，P2 的投影派生与它那封信的消费在另一条连接上跑；
// P2 那一路整段提交、或被看见等在锁上，才放 P1 那一路。两路都必须无错收工。
func (race *staleEnvelopeRace) interleave(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	watcher, err := pgx.ConnectConfig(ctx, race.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatalf("开观察连接：%v", err)
	}
	defer func() { _ = watcher.Close(context.Background()) }()

	held := newHeldViewDerive(race.derive)
	stale := race.subject(t, held)
	fresh := race.subject(t, race.derive)

	staleDone := make(chan error, 1)
	go func() { staleDone <- race.consume(ctx, stale, race.first) }()
	select {
	case <-held.reached:
	case err := <-staleDone:
		t.Fatalf("P1 那一路没走到派生编排就收工：%v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("P1 那一路十秒内没走到派生编排")
	}

	freshDone := make(chan error, 1)
	go func() {
		if err := race.saveProjection(ctx, race.second); err != nil {
			freshDone <- err
			return
		}
		freshDone <- race.consume(ctx, fresh, race.second)
	}()

	var freshErr error
	freshFinished := false
	deadline := time.Now().Add(10 * time.Second)
	for !freshFinished && !rowLockWaiterSeen(watcher) {
		if time.Now().After(deadline) {
			close(held.release)
			t.Fatal("P2 那一路十秒内既没提交、也没等在锁上")
		}
		select {
		case freshErr = <-freshDone:
			freshFinished = true
		case <-time.After(20 * time.Millisecond):
		}
	}
	close(held.release)

	if err := <-staleDone; err != nil {
		t.Fatalf("P1 那一路：%v", err)
	}
	if !freshFinished {
		freshErr = <-freshDone
	}
	if freshErr != nil {
		t.Fatalf("P2 那一路：%v", freshErr)
	}
}

// rowLockWaiterSeen 答本测试库里有没有正等在别的事务上、还没拿到锁的会话。按会话所在库筛：
// 事务号锁不属任何库，实例上别的用例各有各的库，它们的等待不算。
func rowLockWaiterSeen(watcher *pgx.Conn) bool {
	var waiting int
	err := watcher.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_locks AS l
		   JOIN pg_stat_activity AS a ON a.pid = l.pid
		  WHERE NOT l.granted AND a.datname = current_database()`,
	).Scan(&waiting)
	return err == nil && waiting > 0
}

func (race *staleEnvelopeRace) currentView(t *testing.T) vedomain.CustomerTrackingView {
	t.Helper()
	view, found, err := race.views.FindCurrent(t.Context(),
		finalValue(t, vedomain.NewTenantID, "tenant-1"),
		finalValue(t, vedomain.NewCustomerAccountReference, "customer-1"),
		finalValue(t, vedomain.NewTrackedParcelReference, "parcel-1"))
	if err != nil || !found {
		t.Fatalf("读当前视图：err=%v found=%v", err, found)
	}
	return view
}

func (race *staleEnvelopeRace) versionRows(t *testing.T) int {
	t.Helper()
	var rows int
	if err := race.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM visibility_exception.customer_view_version`).Scan(&rows); err != nil {
		t.Fatalf("数视图版本行：%v", err)
	}
	return rows
}

// Covers: 四维不变那一格——视图已按 P1 形成，P1 那封信重投时停在判完新旧之后，P2 期间形成并
// 使当前版跟上 P2；P1 那一路收工后当前版的确认依据仍是 P2，不被拉回 P1，也不多一版。
func TestAStaleProjectionEnvelopeCannotCarryTheConfirmedBasisBack(t *testing.T) {
	race := newStaleEnvelopeRace(t, &viewPolicyDouble{})
	ctx := t.Context()
	if err := race.saveProjection(ctx, race.first); err != nil {
		t.Fatalf("存 P1：%v", err)
	}
	if err := race.consume(ctx, race.subject(t, race.derive), race.first); err != nil {
		t.Fatalf("按 P1 形成视图：%v", err)
	}
	formed := race.currentView(t)

	race.interleave(t)

	view := race.currentView(t)
	if view.Version() != formed.Version() {
		t.Fatalf("视图版本 = %s，want 仍是 %s——四维未变不得换代", view.Version(), formed.Version())
	}
	if view.ConfirmedOn() != race.second.Version() {
		t.Fatalf("确认依据 = %s，want %s——旧投影那封信把确认依据拉回了已被替代的投影",
			view.ConfirmedOn(), race.second.Version())
	}
	if rows := race.versionRows(t); rows != 1 {
		t.Fatalf("视图版本行 = %d，want 1", rows)
	}
}

// Covers: 四维变化那一格——视图尚未形成，P1 那封信停在判完新旧之后，P2 期间形成；P1 那一路
// 收工后当前视图的内容与形成依据都是 P2 的，并指回依据 P1 形成的前一版。
func TestAStaleProjectionEnvelopeCannotSupersedeTheViewWithStaleContent(t *testing.T) {
	race := newStaleEnvelopeRace(t, notePerProjectionPolicy{})
	ctx := t.Context()
	if err := race.saveProjection(ctx, race.first); err != nil {
		t.Fatalf("存 P1：%v", err)
	}

	race.interleave(t)

	view := race.currentView(t)
	if view.BasedOn() != race.second.Version() || view.ConfirmedOn() != race.second.Version() {
		t.Fatalf("形成时依据 = %s 确认依据 = %s，want 都是 %s——旧投影那封信把视图带回了已被替代的投影",
			view.BasedOn(), view.ConfirmedOn(), race.second.Version())
	}
	content, shown := view.Dimensions().Note.Content()
	if !shown || content.String() != "SYN-NOTE-"+race.second.Version().String() {
		t.Fatalf("追踪说明 = %q（%v），want P2 的内容", content.String(), shown)
	}
	prior, superseding := view.PriorVersion()
	if !superseding {
		t.Fatal("当前视图没有指回前一版，want 替代 P1 形成的那一版")
	}
	earlier, found, err := race.views.FindVersion(ctx,
		finalValue(t, vedomain.NewTenantID, "tenant-1"),
		finalValue(t, vedomain.NewCustomerAccountReference, "customer-1"),
		finalValue(t, vedomain.NewTrackedParcelReference, "parcel-1"), prior)
	if err != nil || !found {
		t.Fatalf("读前一版：err=%v found=%v", err, found)
	}
	if earlier.BasedOn() != race.first.Version() {
		t.Fatalf("前一版形成时依据 = %s，want %s", earlier.BasedOn(), race.first.Version())
	}
}
