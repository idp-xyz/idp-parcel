package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证 LockCurrent 的两半：它读回的当前版在本事务结束前不被下一版投影换掉；它撞上一次
// 尚未提交的移标记时等那一笔提交，读回移过去的那一版，不读成「没有当前投影」。

func lockedProjectionPair(t *testing.T) (domain.TrackingProjection, domain.TrackingProjection) {
	t.Helper()
	first := derivedProjection(t, "projection-1", []domain.MilestoneClassification{
		classifiedEntry(t, "scan/origin", "PICKED_UP"),
	})
	second, err := first.Rederive(
		projectionValue(t, domain.NewProjectionVersionID, "projection-2"),
		[]domain.MilestoneClassification{
			classifiedEntry(t, "scan/origin", "PICKED_UP"),
			classifiedEntry(t, "scan/hub", "ARRIVED_AT_HUB"),
		},
		projectionBaseAt.Add(4*time.Hour),
	)
	if err != nil {
		t.Fatalf("重派生：%v", err)
	}
	return first, second
}

func (fixture *projectionFixture) watcher(t *testing.T) *pgx.Conn {
	t.Helper()
	conn, err := pgx.ConnectConfig(t.Context(), fixture.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatalf("开观察连接：%v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

// waitForRowLockWaiter 轮询本测试库里有没有正等在别的事务上、还没拿到锁的会话，到时限答 false。
// 按会话所在库筛：事务号锁不属任何库，实例上别的用例各有各的库，它们的等待不算。
func waitForRowLockWaiter(conn *pgx.Conn, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		var waiting int
		if err := conn.QueryRow(context.Background(),
			`SELECT count(*) FROM pg_locks AS l
			   JOIN pg_stat_activity AS a ON a.pid = l.pid
			  WHERE NOT l.granted AND a.datname = current_database()`,
		).Scan(&waiting); err == nil && waiting > 0 {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestLockCurrentKeepsTheCurrentVersionUntilTheTransactionEnds(t *testing.T) {
	fixture := newProjectionFixture(t)
	ctx := t.Context()
	tenant := projectionValue(t, domain.NewTenantID, "tenant-a")
	first, second := lockedProjectionPair(t)
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.projections.Save(txCtx, tenant, first)
	})
	watcher := fixture.watcher(t)

	locked := make(chan struct{})
	release := make(chan struct{})
	holderDone := make(chan error, 1)
	var heldAgain domain.TrackingProjection
	go func() {
		holderDone <- fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			held, found, err := fixture.projections.LockCurrent(txCtx, tenant, first.Parcel())
			if err != nil || !found || held.Version() != first.Version() {
				close(locked)
				return errors.Join(err, errors.New("锁读没有读回 projection-1"))
			}
			close(locked)
			<-release
			// 同一事务里再读一次：别人的移标记还等在锁上，当前版不该变。
			heldAgain, _, err = fixture.projections.LockCurrent(txCtx, tenant, first.Parcel())
			return err
		})
	}()
	<-locked

	moverDone := make(chan error, 1)
	go func() {
		moverDone <- fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			return fixture.projections.Save(txCtx, tenant, second)
		})
	}()
	if !waitForRowLockWaiter(watcher, 10*time.Second) {
		close(release)
		t.Fatal("下一版投影没有等在当前标记上")
	}
	select {
	case err := <-moverDone:
		close(release)
		t.Fatalf("锁读的事务未结束，下一版投影就移了标记：%v", err)
	default:
	}
	close(release)

	if err := <-holderDone; err != nil {
		t.Fatalf("持锁事务：%v", err)
	}
	if heldAgain.Version() != first.Version() {
		t.Fatalf("同一事务第二次锁读 = %s，want 仍是 projection-1", heldAgain.Version())
	}
	if err := <-moverDone; err != nil {
		t.Fatalf("下一版投影在锁放开后应落库：%v", err)
	}
	current, found, err := fixture.projections.FindCurrent(ctx, tenant, first.Parcel())
	if err != nil || !found || current.Version() != second.Version() {
		t.Fatalf("锁放开后当前版：err=%v found=%v version=%s，want projection-2", err, found, current.Version())
	}
}

func TestLockCurrentWaitsForAnUncommittedMoveAndReadsTheMovedVersion(t *testing.T) {
	fixture := newProjectionFixture(t)
	ctx := t.Context()
	tenant := projectionValue(t, domain.NewTenantID, "tenant-a")
	first, second := lockedProjectionPair(t)
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.projections.Save(txCtx, tenant, first)
	})
	watcher := fixture.watcher(t)

	moved := make(chan struct{})
	release := make(chan struct{})
	moverDone := make(chan error, 1)
	go func() {
		moverDone <- fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			err := fixture.projections.Save(txCtx, tenant, second)
			close(moved)
			if err != nil {
				return err
			}
			<-release
			return nil
		})
	}()
	<-moved

	type lockRead struct {
		projection domain.TrackingProjection
		found      bool
		err        error
	}
	readerDone := make(chan lockRead, 1)
	go func() {
		var read lockRead
		read.err = fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			var err error
			read.projection, read.found, err = fixture.projections.LockCurrent(txCtx, tenant, first.Parcel())
			return err
		})
		readerDone <- read
	}()
	if !waitForRowLockWaiter(watcher, 10*time.Second) {
		close(release)
		t.Fatal("锁读没有等在未提交的移标记上")
	}
	close(release)
	if err := <-moverDone; err != nil {
		t.Fatalf("移标记事务：%v", err)
	}

	read := <-readerDone
	if read.err != nil || !read.found {
		t.Fatalf("锁读：err=%v found=%v，want 读回移过去的那一版", read.err, read.found)
	}
	if read.projection.Version() != second.Version() || len(read.projection.Entries()) != 2 {
		t.Fatalf("锁读 = %s（%d 条），want projection-2（2 条）", read.projection.Version(), len(read.projection.Entries()))
	}
	if prior, ok := read.projection.PriorVersion(); !ok || prior != first.Version() {
		t.Fatalf("锁读的前版 = %s（%v），want projection-1", prior, ok)
	}
}

func TestLockCurrentAnswersNotFoundWithoutACurrentProjection(t *testing.T) {
	fixture := newProjectionFixture(t)
	ctx := t.Context()
	tenant := projectionValue(t, domain.NewTenantID, "tenant-a")
	first, _ := lockedProjectionPair(t)
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.projections.Save(txCtx, tenant, first)
	})

	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		other := projectionValue(t, domain.NewTenantID, "tenant-b")
		if _, found, err := fixture.projections.LockCurrent(txCtx, other, first.Parcel()); err != nil || found {
			t.Errorf("另一租户锁读：err=%v found=%v，want 查无", err, found)
		}
		unknown := projectionValue(t, domain.NewTrackedParcelReference, "parcel-unknown")
		if _, found, err := fixture.projections.LockCurrent(txCtx, tenant, unknown); err != nil || found {
			t.Errorf("无投影的包裹锁读：err=%v found=%v，want 查无", err, found)
		}
		return nil
	})
}

// PBC-08：锁读离了事务就没有可持的锁，必须被 RequireExecutor 拒绝，不退成一次不加锁的读。
func TestLockCurrentRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newProjectionFixture(t)
	_, _, err := fixture.projections.LockCurrent(t.Context(),
		projectionValue(t, domain.NewTenantID, "tenant-a"),
		projectionValue(t, domain.NewTrackedParcelReference, "parcel-1"))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务锁读应返回 ErrTransactionRequired，实得：%v", err)
	}
}
