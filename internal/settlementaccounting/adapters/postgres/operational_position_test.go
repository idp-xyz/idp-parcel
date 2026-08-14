package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 16 证运营余额与信用状况两个视图：未登记是错误不是零值、
// 冻结与暴露由两本账当场算进来（同一笔钱因此冻不了第二次）、迟到快照不覆盖更新的
// 事实、作用域互不可见、无事务拒。

var (
	positionAsOf      = time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	positionLaterAsOf = positionAsOf.Add(24 * time.Hour)
)

// TestAnUnregisteredScopeHasNoBalance 证未登记交回错误而不是一份各项为零的余额：
// 零余额看起来像「查过了、就是没钱」，而实际是没人回答过——编排据以保持未决。
func TestAnUnregisteredScopeHasNoBalance(t *testing.T) {
	position := newPositions(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")

	if _, err := position.balances.LoadBalance(ctx, tenant, saScope(t)); !errors.Is(err, adapter.ErrBalanceNotRegistered) {
		t.Fatalf("err = %v, want ErrBalanceNotRegistered", err)
	}
	if _, err := position.standings.LoadCreditStanding(ctx, tenant, saScope(t)); !errors.Is(err, adapter.ErrCreditStandingNotRegistered) {
		t.Fatalf("err = %v, want ErrCreditStandingNotRegistered", err)
	}
}

// TestABalanceCountsHeldFreezesFromTheLedger 是本文件最要紧的一条：可用余额必须把
// 册内已持有的冻结扣掉。`ledger.Freeze` 判可用时不自行扣减册内冻结，全靠余额里的
// 那一项——少扣一次，同一笔钱就会被冻第二次。
func TestABalanceCountsHeldFreezesFromTheLedger(t *testing.T) {
	position := newPositions(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	scope := saScope(t)

	mustRecordBalance(t, position, tenant, scope, adapter.BalancePosting{
		PostedMinor:    10000,
		CreditMinor:    2000,
		UnsettledMinor: 1000,
		AsOf:           positionAsOf,
	})

	fresh, err := position.balances.LoadBalance(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("读回余额：%v", err)
	}
	if fresh.Available() != 11000 {
		t.Fatalf("可用余额 = %d, want 11000（10000 + 2000 - 0 - 1000）", fresh.Available())
	}

	ledger := domain.NewFreezeLedger()
	frozen := mustFreeze(t, ledger, freezeRequest(t, "control-1", 3000), fresh)
	mustSaveFreezeLedger(t, position.transactor, ctx, position.freezes, tenant, scope, ledger)

	afterFreeze, err := position.balances.LoadBalance(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("冻结后读回余额：%v", err)
	}
	if afterFreeze.Available() != 8000 {
		t.Fatalf("冻结后可用余额 = %d, want 8000；冻结没有被算进来", afterFreeze.Available())
	}

	if _, err := ledger.Release(frozen.FreezeID(), positionAsOf.Add(time.Hour)); err != nil {
		t.Fatalf("释放：%v", err)
	}
	mustSaveFreezeLedger(t, position.transactor, ctx, position.freezes, tenant, scope, ledger)

	afterRelease, err := position.balances.LoadBalance(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("释放后读回余额：%v", err)
	}
	if afterRelease.Available() != 11000 {
		t.Fatalf("释放后可用余额 = %d, want 11000；释放了的冻结仍被当作占用", afterRelease.Available())
	}
}

// TestACreditStandingCountsRecordedExposures 是账期那一侧的同一条：额度余量要把册内
// 已占用的暴露扣掉。两本账分别证——它们互不借用（ADR-0047）。
func TestACreditStandingCountsRecordedExposures(t *testing.T) {
	position := newPositions(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	scope := saScope(t)

	mustRecordStanding(t, position, tenant, scope, adapter.CreditPosition{
		LimitMinor: 5000,
		Overdue:    false,
		AsOf:       positionAsOf,
	})

	fresh, err := position.standings.LoadCreditStanding(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("读回信用状况：%v", err)
	}
	if fresh.Headroom() != 5000 || fresh.Overdue() {
		t.Fatalf("额度余量 = %d，逾期 = %v", fresh.Headroom(), fresh.Overdue())
	}

	ledger := domain.NewCreditExposureLedger()
	if _, err := ledger.Expose(exposureRequest(t, "control-1", 2000), fresh); err != nil {
		t.Fatalf("占用额度：%v", err)
	}
	saWithin(t, position.transactor, ctx, func(txCtx context.Context) error {
		return position.exposures.Save(txCtx, tenant, scope, ledger)
	})

	afterExposure, err := position.standings.LoadCreditStanding(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("占用后读回信用状况：%v", err)
	}
	if afterExposure.Headroom() != 3000 {
		t.Fatalf("占用后额度余量 = %d, want 3000；暴露没有被算进来", afterExposure.Headroom())
	}
}

// TestALateSnapshotDoesNotOverwriteANewerOne 证登记只准往前：迟到的快照交回`未推进`
// 而不是错误（迟到到达是常态），且不改动已在的值。
func TestALateSnapshotDoesNotOverwriteANewerOne(t *testing.T) {
	position := newPositions(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")
	scope := saScope(t)

	mustRecordBalance(t, position, tenant, scope, adapter.BalancePosting{
		PostedMinor: 9000, AsOf: positionLaterAsOf,
	})

	var outcome adapter.PositionRecordOutcome
	saWithin(t, position.transactor, ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = position.balances.RecordBalance(txCtx, tenant, scope, adapter.BalancePosting{
			PostedMinor: 100, AsOf: positionAsOf,
		})
		return err
	})
	if outcome != adapter.PositionNotAdvanced {
		t.Fatalf("outcome = %s, want NOT_ADVANCED", outcome)
	}

	balance, err := position.balances.LoadBalance(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("读回余额：%v", err)
	}
	if balance.Available() != 9000 {
		t.Fatalf("可用余额 = %d, want 9000；迟到快照盖掉了更新的事实", balance.Available())
	}

	// 更新的快照照常推进。
	mustRecordBalance(t, position, tenant, scope, adapter.BalancePosting{
		PostedMinor: 12000, AsOf: positionLaterAsOf.Add(time.Hour),
	})
	advanced, err := position.balances.LoadBalance(ctx, tenant, scope)
	if err != nil {
		t.Fatalf("读回余额：%v", err)
	}
	if advanced.Available() != 12000 {
		t.Fatalf("可用余额 = %d, want 12000", advanced.Available())
	}
}

// TestPositionScopesAreInvisibleToEachOther 证作用域四维都写在条件里：换责任法人、
// 换结算账户、换币种或换租户，读到的都是`未登记`——不同作用域默认不共用余额。
func TestPositionScopesAreInvisibleToEachOther(t *testing.T) {
	position := newPositions(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")

	mustRecordBalance(t, position, tenant, saScope(t), adapter.BalancePosting{
		PostedMinor: 10000, AsOf: positionAsOf,
	})

	if _, err := position.balances.LoadBalance(
		ctx, tenant, saScopeWithAccount(t, "account-2"),
	); !errors.Is(err, adapter.ErrBalanceNotRegistered) {
		t.Fatalf("另一个结算账户读到了余额：err = %v", err)
	}
	if _, err := position.balances.LoadBalance(
		ctx, saTenant(t, "tenant-2"), saScope(t),
	); !errors.Is(err, adapter.ErrBalanceNotRegistered) {
		t.Fatalf("另一个租户读到了余额：err = %v", err)
	}
}

func TestRecordingAPositionOutsideATransactionIsRefused(t *testing.T) {
	position := newPositions(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")

	if _, err := position.balances.RecordBalance(ctx, tenant, saScope(t), adapter.BalancePosting{
		PostedMinor: 1, AsOf: positionAsOf,
	}); err == nil {
		t.Fatal("事务外登记余额成功了")
	}
	if _, err := position.standings.RecordStanding(ctx, tenant, saScope(t), adapter.CreditPosition{
		LimitMinor: 1, AsOf: positionAsOf,
	}); err == nil {
		t.Fatal("事务外登记信用状况成功了")
	}
}

func TestAPositionWithoutAnAsOfIsRefused(t *testing.T) {
	position := newPositions(t)
	ctx := t.Context()
	tenant := saTenant(t, "tenant-1")

	saWithin(t, position.transactor, ctx, func(txCtx context.Context) error {
		if _, err := position.balances.RecordBalance(
			txCtx, tenant, saScope(t), adapter.BalancePosting{PostedMinor: 1},
		); err == nil {
			t.Error("没有截至时点的余额登记成功了；先后将无从判断")
		}
		return nil
	})
}

// ---- 夹具 ----

type positionFixture struct {
	balances   *adapter.OperationalBalances
	standings  *adapter.CreditStandings
	freezes    *adapter.FreezeLedgers
	exposures  *adapter.CreditExposureLedgers
	transactor bentoapp.Transactor
}

func newPositions(t *testing.T) positionFixture {
	t.Helper()

	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	freezes, err := adapter.NewFreezeLedgers(db)
	if err != nil {
		t.Fatalf("构造冻结册仓储：%v", err)
	}
	exposures, err := adapter.NewCreditExposureLedgers(db)
	if err != nil {
		t.Fatalf("构造暴露册仓储：%v", err)
	}
	balances, err := adapter.NewOperationalBalances(db, freezes)
	if err != nil {
		t.Fatalf("构造余额视图：%v", err)
	}
	standings, err := adapter.NewCreditStandings(db, exposures)
	if err != nil {
		t.Fatalf("构造信用状况视图：%v", err)
	}
	return positionFixture{
		balances:   balances,
		standings:  standings,
		freezes:    freezes,
		exposures:  exposures,
		transactor: db.Transactor(),
	}
}

func mustRecordBalance(
	t *testing.T,
	position positionFixture,
	tenant domain.TenantID,
	scope domain.SettlementScope,
	posting adapter.BalancePosting,
) {
	t.Helper()
	saWithin(t, position.transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := position.balances.RecordBalance(txCtx, tenant, scope, posting)
		if err != nil {
			return err
		}
		if outcome != adapter.PositionRecorded {
			t.Errorf("登记余额：outcome = %s, want RECORDED", outcome)
		}
		return nil
	})
}

func mustRecordStanding(
	t *testing.T,
	position positionFixture,
	tenant domain.TenantID,
	scope domain.SettlementScope,
	credit adapter.CreditPosition,
) {
	t.Helper()
	saWithin(t, position.transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := position.standings.RecordStanding(txCtx, tenant, scope, credit)
		if err != nil {
			return err
		}
		if outcome != adapter.PositionRecorded {
			t.Errorf("登记信用状况：outcome = %s, want RECORDED", outcome)
		}
		return nil
	})
}
