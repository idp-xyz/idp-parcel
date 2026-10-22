package postgres_test

import (
	"context"
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证释放口的 savepoint 隔离：内层在调用方事务里报 SQL 级错误时，那一笔事务随后照样能写、能提交。
// 随后的写借一张临时表，断言一律在事务闭包外。

// sqlRelease 在调用方事务里依次执行 sequence 中的一条 SQL，出错时把错误原样交回，像一本读写不动的账本那样。
type sqlRelease struct {
	db       *bentopg.DB
	sequence []string
	asked    int
}

func (release *sqlRelease) ReleasePreAcceptanceControl(ctx context.Context, _ ports.ControlReleaseRequest) error {
	query := release.sequence[release.asked]
	release.asked++
	executor, err := release.db.RequireExecutor(ctx)
	if errors.Is(err, bentopg.ErrTransactionRequired) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = executor.Exec(ctx, query)
	return err
}

func controlReleaseSavepointFixture(t *testing.T, sequence ...string) (*bentopg.DB, *sqlRelease, *adapter.SavepointedControlRelease) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	inner := &sqlRelease{db: db, sequence: sequence}
	return db, inner, adapter.NewSavepointedControlRelease(db, inner)
}

// writeAfterRelease 在同一笔事务里写一行：事务若已中止，这一句答 25P02。
func writeAfterRelease(ctx context.Context, db *bentopg.DB) error {
	executor, err := db.RequireExecutor(ctx)
	if err != nil {
		return err
	}
	if _, err := executor.Exec(ctx, `CREATE TEMP TABLE after_release (n int) ON COMMIT DROP`); err != nil {
		return err
	}
	_, err = executor.Exec(ctx, `INSERT INTO after_release VALUES (1)`)
	return err
}

// Covers: 释放失败不连累调用方事务——内层执行 `SELECT 1/0`，装饰器把错误原样交回，同一事务随后的写照样提交。依次失败、失败、
// 成功：逐版释放一版一开一收，savepoint 名重用；某次退回没退干净，第三次或随后的写就会答 25P02。
func TestASavepointKeepsAFailedReleaseFromAbortingTheCaller(t *testing.T) {
	db, inner, release := controlReleaseSavepointFixture(t, `SELECT 1/0`, `SELECT 1/0`, `SELECT 1`)

	errs := make([]error, 0, len(inner.sequence))
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		for range inner.sequence {
			errs = append(errs, release.ReleasePreAcceptanceControl(txCtx, ports.ControlReleaseRequest{}))
		}
		return writeAfterRelease(txCtx, db)
	})
	if len(errs) != 3 || errs[0] == nil || errs[1] == nil || errs[2] != nil {
		t.Fatalf("三次释放的错误 = %v, want 失败、失败、成功", errs)
	}
	if err != nil {
		t.Fatalf("释放失败连带了随后的写：%v", err)
	}
}

// Covers: 装饰器的无事务一支与装配缺口——ctx 不在事务里时直通内层、不发 SAVEPOINT；少接一件时答错误，不 panic。
func TestASavepointPassesTheReleaseThroughOutsideATransaction(t *testing.T) {
	db, inner, release := controlReleaseSavepointFixture(t, `SELECT 1`)
	if _, err := db.RequireExecutor(t.Context()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("前提不成立：ctx 应在事务外，RequireExecutor 却答 %v", err)
	}
	if err := release.ReleasePreAcceptanceControl(t.Context(), ports.ControlReleaseRequest{}); err != nil || inner.asked != 1 {
		t.Fatalf("事务外 err = %v 问内层 %d 次, want 直通一次", err, inner.asked)
	}
	if err := adapter.NewSavepointedControlRelease(db, nil).ReleasePreAcceptanceControl(
		t.Context(), ports.ControlReleaseRequest{}); err == nil {
		t.Fatal("少接内层却答成功")
	}
}
