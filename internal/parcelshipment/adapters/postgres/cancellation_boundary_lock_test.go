package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证取消边界锁本身：无事务拒取（锁随自动提交当场放掉，挡不住任何一路）；锁按（租户，包裹）
// 排队——同一件包裹的第二路等着，别的包裹、别的租户的同号包裹照走。

func newCancellationBoundaryLocks(t *testing.T) (*adapter.CancellationBoundaryLocks, *bentopg.DB) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	locks, err := adapter.NewCancellationBoundaryLocks(db)
	if err != nil {
		t.Fatalf("构造边界锁：%v", err)
	}
	return locks, db
}

func TestTheCancellationBoundaryLockRefusesToRunOutsideATransaction(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	locks, err := adapter.NewCancellationBoundaryLocks(db)
	if err != nil {
		t.Fatalf("构造边界锁：%v", err)
	}
	tenant := mustBuild(t, domain.NewTenantID, "tenant-1")
	parcel := mustBuild(t, domain.NewDeclaredParcelID, "parcel-1")
	if err := locks.LockCancellationBoundary(t.Context(), tenant, parcel); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务取边界锁应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return locks.LockCancellationBoundary(txCtx, tenant, parcel)
	}); err != nil {
		t.Fatalf("事务内取边界锁：%v", err)
	}
}

// Covers: ports.CancellationBoundaryLock「同一（租户，包裹）上……一次只走一路」——一笔事务握着 tenant-1/parcel-1 不放，另起的
// 事务限 lock_timeout 去取：同一件包裹取不到（55P03，等在锁上）；同租户别的包裹、别的租户同号的包裹都立刻取到。锁键少了包裹
// 或少了租户，后两格会被挡住。
func TestTheCancellationBoundaryLockQueuesOnlyTheSameParcelOfTheSameTenant(t *testing.T) {
	locks, db := newCancellationBoundaryLocks(t)
	ctx := t.Context()
	transactor := db.Transactor()

	held := make(chan struct{})
	release := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			if err := locks.LockCancellationBoundary(txCtx,
				mustBuild(t, domain.NewTenantID, "tenant-1"), mustBuild(t, domain.NewDeclaredParcelID, "parcel-1")); err != nil {
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
		t.Fatalf("握锁那一路没取到锁：%v", err)
	case <-ctx.Done():
		t.Fatal("握锁那一路没取到锁就超时了")
	}
	defer func() {
		close(release)
		if err := <-holderDone; err != nil {
			t.Errorf("握锁那一路：%v", err)
		}
	}()

	probe := func(tenant, parcel string) error {
		return transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			executor, err := db.RequireExecutor(txCtx)
			if err != nil {
				return err
			}
			if _, err := executor.Exec(txCtx, `SET LOCAL lock_timeout = '300ms'`); err != nil {
				return err
			}
			return locks.LockCancellationBoundary(txCtx,
				mustBuild(t, domain.NewTenantID, tenant), mustBuild(t, domain.NewDeclaredParcelID, parcel))
		})
	}

	if err := probe("tenant-1", "parcel-2"); err != nil {
		t.Fatalf("同租户别的包裹被挡住：%v", err)
	}
	if err := probe("tenant-2", "parcel-1"); err != nil {
		t.Fatalf("别的租户同号的包裹被挡住：%v", err)
	}
	var pgErr *pgconn.PgError
	if err := probe("tenant-1", "parcel-1"); !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
		t.Fatalf("同一件包裹的第二路应等在锁上直到 lock_timeout（55P03），实得：%v", err)
	}
}
