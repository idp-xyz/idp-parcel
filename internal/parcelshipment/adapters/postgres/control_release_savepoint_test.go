package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证释放口的 savepoint 隔离：内层在调用方事务里报 SQL 级错误时，那一笔事务随后照样能写、能提交。
// 随后的写借一张临时表，断言一律在事务闭包外。

// sqlRelease 在调用方事务里依次执行 sequence 中的一条 SQL，像 settlement-accounting 的释放编排那样把 SQL 错误折成`可重试
// 未形成`正常交回——事务却已中止，正是装饰器要隔开的那一格。
type sqlRelease struct {
	db       *bentopg.DB
	sequence []string
	asked    int
}

func (release *sqlRelease) ReleasePreAcceptanceControl(
	ctx context.Context,
	_ ports.ControlReleaseRequest,
) (ports.ControlReleaseAnswer, error) {
	query := release.sequence[release.asked]
	release.asked++
	executor, err := release.db.RequireExecutor(ctx)
	if errors.Is(err, bentopg.ErrTransactionRequired) {
		return ports.ControlReleaseAnswer{Outcome: ports.ControlReleaseSettled}, nil
	}
	if err != nil {
		return ports.ControlReleaseAnswer{}, err
	}
	if _, err := executor.Exec(ctx, query); err != nil {
		reason, reasonErr := domain.NewCheckReason("SA-FREEZE_LEDGER_UNAVAILABLE")
		if reasonErr != nil {
			return ports.ControlReleaseAnswer{}, reasonErr
		}
		return ports.ControlReleaseAnswer{Outcome: ports.ControlReleaseNotFormed, Reason: reason}, nil
	}
	return ports.ControlReleaseAnswer{Outcome: ports.ControlReleaseSettled}, nil
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

// Covers: 释放没收口不连累调用方事务——内层执行 `SELECT 1/0` 后答`可重试未形成`，装饰器把答复原样交回，同一事务随后的写照样
// 提交。依次失败、失败、成功：逐版释放一版一开一收，savepoint 名重用；某次退回没退干净，第三次或随后的写就会答 25P02。
func TestASavepointKeepsAFailedReleaseFromAbortingTheCaller(t *testing.T) {
	db, inner, release := controlReleaseSavepointFixture(t, `SELECT 1/0`, `SELECT 1/0`, `SELECT 1`)

	answers := make([]ports.ControlReleaseOutcome, 0, len(inner.sequence))
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		for range inner.sequence {
			answer, err := release.ReleasePreAcceptanceControl(txCtx, ports.ControlReleaseRequest{})
			if err != nil {
				return err
			}
			answers = append(answers, answer.Outcome)
		}
		return writeAfterRelease(txCtx, db)
	})
	want := []ports.ControlReleaseOutcome{ports.ControlReleaseNotFormed, ports.ControlReleaseNotFormed, ports.ControlReleaseSettled}
	if !slices.Equal(answers, want) {
		t.Fatalf("三次释放的答复 = %v, want 未形成、未形成、已收口", answers)
	}
	if err != nil {
		t.Fatalf("释放没收口连带了随后的写：%v", err)
	}
}

// failingRelease 在调用方事务里写坏一句再答错误：端口形不成三格之一的那一格同样要退回 savepoint。
type failingRelease struct{ db *bentopg.DB }

func (release failingRelease) ReleasePreAcceptanceControl(
	ctx context.Context,
	_ ports.ControlReleaseRequest,
) (ports.ControlReleaseAnswer, error) {
	executor, err := release.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ControlReleaseAnswer{}, err
	}
	_, _ = executor.Exec(ctx, `SELECT 1/0`)
	return ports.ControlReleaseAnswer{}, ports.ErrUntranslatableProviderAnswer
}

// Covers: 端口形不成三格之一时装饰器原样交回错误，并退回 savepoint——终局各路据此记册、照样提交，事务不能随之中止。
func TestASavepointRollsBackAReleaseThatAnsweredOutsideItsClosedSet(t *testing.T) {
	db, _, _ := controlReleaseSavepointFixture(t, `SELECT 1`)
	release := adapter.NewSavepointedControlRelease(db, failingRelease{db: db})

	var releaseErr error
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, releaseErr = release.ReleasePreAcceptanceControl(txCtx, ports.ControlReleaseRequest{})
		return writeAfterRelease(txCtx, db)
	})
	if !errors.Is(releaseErr, ports.ErrUntranslatableProviderAnswer) {
		t.Fatalf("释放错误 = %v, want 原样交回的端口哨兵", releaseErr)
	}
	if err != nil {
		t.Fatalf("集合外的答复连带了随后的写：%v", err)
	}
}

// Covers: 装饰器的无事务一支与装配缺口——ctx 不在事务里时直通内层、不发 SAVEPOINT；少接一件时答错误，不 panic。
func TestASavepointPassesTheReleaseThroughOutsideATransaction(t *testing.T) {
	db, inner, release := controlReleaseSavepointFixture(t, `SELECT 1`)
	if _, err := db.RequireExecutor(t.Context()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("前提不成立：ctx 应在事务外，RequireExecutor 却答 %v", err)
	}
	answer, err := release.ReleasePreAcceptanceControl(t.Context(), ports.ControlReleaseRequest{})
	if err != nil || answer.Outcome != ports.ControlReleaseSettled || inner.asked != 1 {
		t.Fatalf("事务外 answer = %+v err = %v 问内层 %d 次, want 直通一次", answer, err, inner.asked)
	}
	if _, err := adapter.NewSavepointedControlRelease(db, nil).ReleasePreAcceptanceControl(
		t.Context(), ports.ControlReleaseRequest{}); err == nil {
		t.Fatal("少接内层却答成功")
	}
}
