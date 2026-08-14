package postgres_test

import (
	"context"
	"errors"
	"testing"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证计划版本签发器：号不重复、回滚不把号还回去、无事务拒。
// 「不重复」是 plan_applicability 以 plan_id 单列作主键的前提——两个计划共用一个版本号
// 会被那张表压成一行。

func TestRoutePlanVersionsNeverRepeat(t *testing.T) {
	factory, transactor := newRouteIdentities(t)
	ctx := t.Context()

	seen := make(map[string]struct{}, 8)
	for range 8 {
		var raw string
		mustWithinIdentityTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			version, err := factory.NextRoutePlanVersionID(txCtx)
			if err != nil {
				return err
			}
			raw = version.String()
			return nil
		})
		// 断言留在闭包外：闭包里 t.Fatalf 会从事务回调中 runtime.Goexit，而
		// WithinTransaction 并不预期它的回调不返回。
		if _, duplicated := seen[raw]; duplicated {
			t.Fatalf("计划版本号重复：%q", raw)
		}
		seen[raw] = struct{}{}
	}
	if len(seen) != 8 {
		t.Fatalf("签出 %d 个不同的号，want 8", len(seen))
	}
}

func TestTheFirstRoutePlanVersionIsShapedAndPrefixed(t *testing.T) {
	factory, transactor := newRouteIdentities(t)

	var first string
	mustWithinIdentityTransaction(t, transactor, t.Context(), func(txCtx context.Context) error {
		version, err := factory.NextRoutePlanVersionID(txCtx)
		if err != nil {
			return err
		}
		first = version.String()
		return nil
	})
	if first != "RPV-000000000001" {
		t.Errorf("首个计划版本 = %q, want RPV-000000000001", first)
	}
}

// TestARolledBackRouteTransactionStillBurnsTheNumber 证 nextval 不随事务回滚：一次
// 未提交的判断烧掉一个号，但绝不把它让给下一次。
func TestARolledBackRouteTransactionStillBurnsTheNumber(t *testing.T) {
	factory, transactor := newRouteIdentities(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	var burned string
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		version, err := factory.NextRoutePlanVersionID(txCtx)
		if err != nil {
			return err
		}
		burned = version.String()
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	var next string
	mustWithinIdentityTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		version, err := factory.NextRoutePlanVersionID(txCtx)
		if err != nil {
			return err
		}
		next = version.String()
		return nil
	})
	if next == burned {
		t.Fatalf("回滚把号还了回去：两次都签出 %q", burned)
	}
}

func TestRouteMintingRefusesToRunOutsideATransaction(t *testing.T) {
	factory, _ := newRouteIdentities(t)

	if _, err := factory.NextRoutePlanVersionID(t.Context()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务签发应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// ---- 夹具 ----

func newRouteIdentities(t *testing.T) (*adapter.RouteIdentities, bentoapp.Transactor) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	factory, err := adapter.NewRouteIdentities(db)
	if err != nil {
		t.Fatalf("构造计划版本签发器：%v", err)
	}
	return factory, db.Transactor()
}

func mustWithinIdentityTransaction(
	t *testing.T,
	transactor bentoapp.Transactor,
	ctx context.Context,
	fn func(context.Context) error,
) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内签发失败：%v", err)
	}
}
