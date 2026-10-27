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

// halfRelease 先在调用方事务里落一行（提供方已落库的那一半），再按 answer 与 err 答复；abort 为真时落完再执行 `SELECT 1/0`，
// 把事务打进中止态——像 settlement-accounting 的释放编排在第二本账上遇到 SQL 级错误那样。
type halfRelease struct {
	db     *bentopg.DB
	answer ports.ControlReleaseAnswer
	err    error
	abort  bool
}

func (release halfRelease) ReleasePreAcceptanceControl(
	ctx context.Context,
	_ ports.ControlReleaseRequest,
) (ports.ControlReleaseAnswer, error) {
	executor, err := release.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ControlReleaseAnswer{}, err
	}
	if _, err := executor.Exec(ctx, `INSERT INTO persisted_half VALUES (1)`); err != nil {
		return ports.ControlReleaseAnswer{}, err
	}
	if release.abort {
		_, _ = executor.Exec(ctx, `SELECT 1/0`)
	}
	return release.answer, release.err
}

func notFormedAnswer(t *testing.T) ports.ControlReleaseAnswer {
	t.Helper()
	reason, err := domain.NewCheckReason("SA-EXPOSURE_LEDGER_UNAVAILABLE")
	if err != nil {
		t.Fatalf("原因：%v", err)
	}
	return ports.ControlReleaseAnswer{Outcome: ports.ControlReleaseNotFormed, Reason: reason}
}

func notAcceptedAnswer(t *testing.T) ports.ControlReleaseAnswer {
	t.Helper()
	reason, err := domain.NewCheckReason("SA-RELEASE_REQUEST_NOT_ACCEPTED")
	if err != nil {
		t.Fatalf("原因：%v", err)
	}
	return ports.ControlReleaseAnswer{Outcome: ports.ControlReleaseNotAccepted, Reason: reason}
}

// persistedHalfRows 数「提供方已落库的那一半」在调用方事务里还剩几行；事务若已中止，这一句自己就答 25P02。
func persistedHalfRows(ctx context.Context, db *bentopg.DB) (int, error) {
	executor, err := db.RequireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	var rows int
	err = executor.QueryRow(ctx, `SELECT count(*) FROM persisted_half`).Scan(&rows)
	return rows, err
}

func createPersistedHalf(ctx context.Context, db *bentopg.DB) error {
	executor, err := db.RequireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = executor.Exec(ctx, `CREATE TEMP TABLE persisted_half (n int) ON COMMIT DROP`)
	return err
}

// Covers: AT-PS-160、ADR-0122 决定三「冻结那本已落库的释放在暴露账本读不回时不撤回」在共享事务里成立——内层落了一半就答非`已收口`，
// 只要事务没中止，那一半就留着：`可重试未形成`（账本读回不来但语句没出错）、`确定未受理`、形不成三格的错误三种答法同此；
// 答复原样交回，随后的写照样提交。已收口一格也留着，不因这条改动回归。
func TestASavepointKeepsWhatAReleaseAlreadyPersistedWhileTheTransactionIsHealthy(t *testing.T) {
	cases := []struct {
		name    string
		answer  func(*testing.T) ports.ControlReleaseAnswer
		err     error
		outcome ports.ControlReleaseOutcome
	}{
		{name: "可重试未形成", answer: notFormedAnswer, outcome: ports.ControlReleaseNotFormed},
		{name: "确定未受理", answer: notAcceptedAnswer, outcome: ports.ControlReleaseNotAccepted},
		{name: "形不成三格之一的错误", answer: func(*testing.T) ports.ControlReleaseAnswer { return ports.ControlReleaseAnswer{} },
			err: ports.ErrUntranslatableProviderAnswer},
		{name: "已收口", answer: func(*testing.T) ports.ControlReleaseAnswer {
			return ports.ControlReleaseAnswer{Outcome: ports.ControlReleaseSettled}
		}, outcome: ports.ControlReleaseSettled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, _, _ := controlReleaseSavepointFixture(t, `SELECT 1`)
			release := adapter.NewSavepointedControlRelease(db, halfRelease{db: db, answer: tc.answer(t), err: tc.err})

			var answer ports.ControlReleaseAnswer
			var releaseErr error
			var kept int
			err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
				if err := createPersistedHalf(txCtx, db); err != nil {
					return err
				}
				answer, releaseErr = release.ReleasePreAcceptanceControl(txCtx, ports.ControlReleaseRequest{})
				var rowsErr error
				if kept, rowsErr = persistedHalfRows(txCtx, db); rowsErr != nil {
					return rowsErr
				}
				return writeAfterRelease(txCtx, db)
			})
			if err != nil {
				t.Fatalf("事务没中止，随后的读写与提交却失败：%v", err)
			}
			if tc.err != nil {
				if !errors.Is(releaseErr, tc.err) {
					t.Fatalf("释放错误 = %v, want 原样交回 %v", releaseErr, tc.err)
				}
			} else if releaseErr != nil || answer.Outcome != tc.outcome {
				t.Fatalf("答复 = %+v err = %v, want %v 原样交回", answer, releaseErr, tc.outcome)
			}
			if kept != 1 {
				t.Fatalf("内层已落库的那一半剩 %d 行, want 1——事务没中止却被一并撤回", kept)
			}
		})
	}
}

// Covers: AT-PS-160、SQL 级错误那一格如实记录（ADR-0291）——内层落了一半、随后一条语句出错把事务打进中止态：不论答复落在哪一格（可重试未形成、
// 确定未受理、形不成三格的错误），落法只看事务状态，一律退回 savepoint，落下的那一半随之撤掉（事务本就保不住它），答复原样交回，
// 退回之后同一事务照样读写与提交。
func TestASavepointUndoesTheHalfWhenTheTransactionIsAborted(t *testing.T) {
	cases := []struct {
		name   string
		answer func(*testing.T) ports.ControlReleaseAnswer
		err    error
	}{
		{name: "可重试未形成", answer: notFormedAnswer},
		{name: "确定未受理", answer: notAcceptedAnswer},
		{name: "形不成三格之一的错误", answer: func(*testing.T) ports.ControlReleaseAnswer { return ports.ControlReleaseAnswer{} },
			err: ports.ErrUntranslatableProviderAnswer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, _, _ := controlReleaseSavepointFixture(t, `SELECT 1`)
			want := tc.answer(t)
			release := adapter.NewSavepointedControlRelease(db, halfRelease{db: db, answer: want, err: tc.err, abort: true})

			var answer ports.ControlReleaseAnswer
			var releaseErr error
			var left int
			err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
				if err := createPersistedHalf(txCtx, db); err != nil {
					return err
				}
				answer, releaseErr = release.ReleasePreAcceptanceControl(txCtx, ports.ControlReleaseRequest{})
				var rowsErr error
				if left, rowsErr = persistedHalfRows(txCtx, db); rowsErr != nil {
					return rowsErr
				}
				return writeAfterRelease(txCtx, db)
			})
			if err != nil {
				t.Fatalf("事务已中止，退回 savepoint 之后随后的读写与提交应当照样成功：%v", err)
			}
			if tc.err != nil {
				if !errors.Is(releaseErr, tc.err) {
					t.Fatalf("释放错误 = %v, want 原样交回 %v", releaseErr, tc.err)
				}
			} else if releaseErr != nil || answer != want {
				t.Fatalf("答复 = %+v err = %v, want %+v 原样交回，不是 savepoint 不可用", answer, releaseErr, want)
			}
			if left != 0 {
				t.Fatalf("内层落下的那一半剩 %d 行, want 0——事务已中止，退回 savepoint 应当把它撤掉", left)
			}
		})
	}
}

// Covers: AT-PS-160、savepoint 名逐版重用——先中止（退回）、再健康未收口（收起）、再已收口（收起），三次之间互不牵连：每次的落法只看
// 那一次事务此刻的状态。
func TestASavepointDecidesEachReleaseByThatMomentsTransactionState(t *testing.T) {
	db, _, _ := controlReleaseSavepointFixture(t, `SELECT 1`)
	steps := []halfRelease{
		{db: db, answer: notFormedAnswer(t), abort: true},
		{db: db, answer: notFormedAnswer(t)},
		{db: db, answer: ports.ControlReleaseAnswer{Outcome: ports.ControlReleaseSettled}},
	}
	var rows []int
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if err := createPersistedHalf(txCtx, db); err != nil {
			return err
		}
		for _, step := range steps {
			if _, err := adapter.NewSavepointedControlRelease(db, step).ReleasePreAcceptanceControl(
				txCtx, ports.ControlReleaseRequest{}); err != nil {
				return err
			}
			count, err := persistedHalfRows(txCtx, db)
			if err != nil {
				return err
			}
			rows = append(rows, count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("三次释放之间互相牵连：%v", err)
	}
	if !slices.Equal(rows, []int{0, 1, 2}) {
		t.Fatalf("每次释放后剩的行数 = %v, want [0 1 2]：中止那次撤掉，健康未收口与已收口两次各留一行", rows)
	}
}
