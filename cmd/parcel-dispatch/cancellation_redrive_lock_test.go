package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// probedTargets 数续办向库问「当前已接受的委托目标」问了几次：那是续办第一道读库的动作。
type probedTargets struct {
	psports.CurrentAcceptedParcelTargetView
	calls *atomic.Int32
}

func (probe probedTargets) FindCurrentAcceptedByParcel(
	ctx context.Context,
	tenant psdomain.TenantID,
	parcel psdomain.DeclaredParcelID,
) (psdomain.CurrentAcceptedParcelTarget, bool, error) {
	probe.calls.Add(1)
	return probe.CurrentAcceptedParcelTargetView.FindCurrentAcceptedByParcel(ctx, tenant, parcel)
}

// Covers: ADR-0295 决定四、PS CONTEXT「同一包裹的取消提交、收寄采用与面单渠道服务的终局判断依次越过提交边界」——续办在读任何东西之前先在真库上排进这件包裹的取消边界锁。
// 另一笔事务握着这件包裹的锁时，续办要等在咨询锁上、一道读都没做；锁放开之后才读目标。续办里各条链进来时会再取一遍锁（重入），所以锁若只取在链里，续办自己
// 的读（目标、回放原料）就在锁外——最终落库看不出差别，这条用例守的是取锁在读之前的次序，次序靠的是咨询锁真的挡住了续办，不是替身记下的调用顺序。
func TestTheRedriveQueuesOnItsParcelBeforeReadingAnything(t *testing.T) {
	fixture := acceptedSYNFixture(t)
	ctx := t.Context()
	var targetReads atomic.Int32
	rig := newRedriveRigWithTargets(t, fixture, func(targets psports.CurrentAcceptedParcelTargetView) psports.CurrentAcceptedParcelTargetView {
		return probedTargets{CurrentAcceptedParcelTargetView: targets, calls: &targetReads}
	})
	locks, err := pspostgres.NewCancellationBoundaryLocks(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := fixture.identity.TenantID()
	parcel := mustPS(t, psdomain.NewDeclaredParcelID, nodeIntakeAssociation)

	// 另一笔事务先握住这件包裹的锁（与同一包裹上的取消、决定、采用同形）。
	held, release := make(chan struct{}), make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			if err := locks.LockCancellationBoundary(txCtx, tenant, parcel); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	select {
	case <-held:
	case err := <-holderDone:
		t.Fatalf("握锁的事务没握住就结束了：%v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("握锁的事务十秒内没握住锁")
	}
	var once = func() func() {
		done := false
		return func() {
			if !done {
				done = true
				close(release)
			}
		}
	}()
	defer once()

	redriveDone := make(chan error, 1)
	go func() {
		redriveDone <- fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			return rig.redriver.HandleNotUpheldCancellation(txCtx, psinbox.NotUpheldCancellation{
				TenantID: tenant.String(), Parcel: parcel.String(), Cancellation: "PCXL-NONE",
				BoundaryKind: psdomain.NetworkIntakeBoundary.String(), BoundaryRef: "NODE_INTAKE/v1",
			})
		})
	}()

	// 看锁的连接单独开：池里的两条连接已被握锁与续办各占一条。
	watcher, err := pgx.ConnectConfig(ctx, fixture.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatalf("开看锁连接：%v", err)
	}
	t.Cleanup(func() { _ = watcher.Close(context.Background()) })
	waiting := func() bool {
		var n int
		if err := watcher.QueryRow(ctx,
			`SELECT count(*) FROM pg_locks
			  WHERE locktype = 'advisory' AND NOT granted
			    AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`).Scan(&n); err != nil {
			t.Fatalf("看锁：%v", err)
		}
		return n > 0
	}
	finished := false
	var redriveErr error
	deadline := time.Now().Add(10 * time.Second)
	for !finished && !waiting() {
		select {
		case redriveErr = <-redriveDone:
			finished = true
		case <-time.After(20 * time.Millisecond):
		}
		if !finished && time.Now().After(deadline) {
			t.Fatal("续办十秒内既没结束、也没等在咨询锁上")
		}
	}
	if n := targetReads.Load(); n != 0 {
		t.Fatalf("续办在别的事务握着这件包裹的锁时已读了 %d 次目标（结束 %t, err=%v）——取锁必须在它自己的第一道读之前", n, finished, redriveErr)
	}

	once()
	if err := <-holderDone; err != nil {
		t.Fatalf("握锁的事务：%v", err)
	}
	if !finished {
		select {
		case redriveErr = <-redriveDone:
		case <-time.After(10 * time.Second):
			t.Fatal("锁放开之后续办十秒内没走完")
		}
	}
	if redriveErr != nil {
		t.Fatalf("续办：%v", redriveErr)
	}
	if n := targetReads.Load(); n != 1 {
		t.Fatalf("锁放开之后续办读目标 %d 次, want 1", n)
	}
}
