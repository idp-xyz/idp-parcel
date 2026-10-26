package settlementaccounting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件证保存委托之前取控制作用域锁的消费侧适配器（ports.PreAcceptanceControlScopeLock）：它把存下的占用作用域译成提供方的
// 结算作用域，取的就是提供方施加与释放取的同一把锁（票 docs-consistency/code/99）。译不成提供方的值的不取，交给提供方的
// 释放以`未受理`短路；锁口没接上是装配缺口，响亮报错。

type recordingScopeLock struct {
	tenants []sadomain.TenantID
	scopes  []sadomain.SettlementScope
	err     error
}

func (double *recordingScopeLock) LockControlScope(
	_ context.Context,
	tenant sadomain.TenantID,
	scope sadomain.SettlementScope,
) error {
	double.tenants = append(double.tenants, tenant)
	double.scopes = append(double.scopes, scope)
	return double.err
}

func occupationScopeOf(t *testing.T) psdomain.OccupationScope {
	t.Helper()
	scope, err := psdomain.NewOccupationScope("LE-1", "ACCOUNT-1", "CNY")
	if err != nil {
		t.Fatalf("occupation scope: %v", err)
	}
	return scope
}

// Covers: 存下的占用作用域逐段译成提供方的结算作用域，租户随之译过去；同一次取锁只落一次。
func TestTheScopeLockTranslatesTheStoredOccupationScopeAndTenant(t *testing.T) {
	inner := &recordingScopeLock{}
	locks := adapter.NewControlScopeLocks(inner)

	if err := locks.LockControlScope(t.Context(), value(t, psdomain.NewTenantID, "TENANT-1"), occupationScopeOf(t)); err != nil {
		t.Fatalf("lock: %v", err)
	}
	if len(inner.scopes) != 1 {
		t.Fatalf("提供方的锁被取 %d 次, want 1", len(inner.scopes))
	}
	if got := inner.tenants[0].String(); got != "TENANT-1" {
		t.Fatalf("租户 = %q, want TENANT-1", got)
	}
	scope := inner.scopes[0]
	if scope.LegalEntity().String() != "LE-1" || scope.Account().String() != "ACCOUNT-1" || scope.Currency().String() != "CNY" {
		t.Fatalf("作用域 = %v/%v/%v, want LE-1/ACCOUNT-1/CNY", scope.LegalEntity(), scope.Account(), scope.Currency())
	}
}

// Covers: 没存下作用域的存量结果不取：提供方的释放在读任何账本之前就以未受理短路，没有要排队的东西。
func TestTheScopeLockSkipsAScopeThatWasNeverStored(t *testing.T) {
	inner := &recordingScopeLock{}
	locks := adapter.NewControlScopeLocks(inner)

	if err := locks.LockControlScope(t.Context(), value(t, psdomain.NewTenantID, "TENANT-1"), psdomain.OccupationScope{}); err != nil {
		t.Fatalf("lock: %v", err)
	}
	if len(inner.scopes) != 0 {
		t.Fatalf("没存作用域却取了锁 %v", inner.scopes)
	}
}

// Covers: 提供方的锁取不到，原错误上抛供各路按「业务事实尚未形成」处置；锁口没接上是装配缺口，同样报错而不是静默放过。
func TestTheScopeLockReportsAnUnavailableOrUnassembledLock(t *testing.T) {
	cause := errors.New("lock unavailable")
	locks := adapter.NewControlScopeLocks(&recordingScopeLock{err: cause})
	if err := locks.LockControlScope(t.Context(), value(t, psdomain.NewTenantID, "TENANT-1"), occupationScopeOf(t)); !errors.Is(err, cause) {
		t.Fatalf("err = %v, want 包着提供方的错误", err)
	}
	unassembled := adapter.NewControlScopeLocks(nil)
	if err := unassembled.LockControlScope(t.Context(), value(t, psdomain.NewTenantID, "TENANT-1"), occupationScopeOf(t)); err == nil {
		t.Fatal("没接锁口却答成功")
	}
}

// Covers: 适配器取的与提供方施加、释放取的是同一把锁，且按（租户，作用域）分键：一笔事务经本适配器持着某个作用域的锁时，
// 经提供方自己的口取同一（租户，作用域）等在锁上（lock_timeout 量出 55P03），别的作用域、别的租户立刻取到。键译错一段，
// 两边各锁各的，撤回类路径先取的这把就挡不住接受链。
func TestTheScopeLockIsTheSameLockTheProviderTakesAndIsKeyedByTenantAndScope(t *testing.T) {
	db := openSettlementDB(t)
	providerLocks, err := sapostgres.NewPreAcceptanceControlScopeLocks(db)
	if err != nil {
		t.Fatalf("provider locks: %v", err)
	}
	locks := adapter.NewControlScopeLocks(providerLocks)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	held := make(chan struct{})
	release := make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			if err := locks.LockControlScope(txCtx, value(t, psdomain.NewTenantID, "TENANT-1"), occupationScopeOf(t)); err != nil {
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

	probe := func(tenant, entity, account, currency string) error {
		return db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			executor, err := db.RequireExecutor(txCtx)
			if err != nil {
				return err
			}
			if _, err := executor.Exec(txCtx, `SET LOCAL lock_timeout = '300ms'`); err != nil {
				return err
			}
			settlement, err := sadomain.NewSettlementScope(
				value(t, sadomain.NewLegalEntityReference, entity),
				value(t, sadomain.NewSettlementAccountID, account),
				value(t, sadomain.NewCurrencyCode, currency))
			if err != nil {
				return err
			}
			return providerLocks.LockControlScope(txCtx, value(t, sadomain.NewTenantID, tenant), settlement)
		})
	}

	var pgErr *pgconn.PgError
	if err := probe("TENANT-1", "LE-1", "ACCOUNT-1", "CNY"); !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
		t.Fatalf("提供方自己的口取同一（租户，作用域）应等在锁上直到 lock_timeout（55P03），实得：%v", err)
	}
	for name, args := range map[string][4]string{
		"别的租户":   {"TENANT-2", "LE-1", "ACCOUNT-1", "CNY"},
		"别的法人":   {"TENANT-1", "LE-2", "ACCOUNT-1", "CNY"},
		"别的结算账户": {"TENANT-1", "LE-1", "ACCOUNT-2", "CNY"},
		"别的币种":   {"TENANT-1", "LE-1", "ACCOUNT-1", "USD"},
	} {
		if err := probe(args[0], args[1], args[2], args[3]); err != nil {
			t.Fatalf("%s被挡住：%v", name, err)
		}
	}
}
