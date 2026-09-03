package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
)

// 本文件对真实 PostgreSQL 16 证结果版本签发器：号不重复、两条序列互不相干、
// 回滚不把号还回去（否则两份结果会共用一个版本号），以及无事务拒。

func TestPickupResultVersionsNeverRepeat(t *testing.T) {
	factory, transactor := newResultVersions(t)
	ctx := t.Context()

	seen := make(map[string]struct{}, 8)
	for range 8 {
		var raw string
		mustWithinVersionTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			version, err := factory.NextPickupResultVersion(txCtx)
			if err != nil {
				return err
			}
			raw = version.String()
			return nil
		})
		// 断言留在闭包外，理由同 NR 侧：闭包里 t.Fatalf 会从事务回调中 Goexit。
		if _, duplicated := seen[raw]; duplicated {
			t.Fatalf("揽收版本号重复：%q", raw)
		}
		seen[raw] = struct{}{}
	}
	if len(seen) != 8 {
		t.Fatalf("签出 %d 个不同的号，want 8", len(seen))
	}
}

// TestTheTwoSequencesDoNotShareACounter 证两个域各有自己的号：合用一个计数器会让其中
// 一个必须跳号，而号本身没有业务含义，两个域共用一个计数器读起来却像有。
func TestTheTwoSequencesDoNotShareACounter(t *testing.T) {
	factory, transactor := newResultVersions(t)
	ctx := t.Context()

	var pickup, delivery string
	mustWithinVersionTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		first, err := factory.NextPickupResultVersion(txCtx)
		if err != nil {
			return err
		}
		second, err := factory.NextDeliveryResultVersion(txCtx)
		if err != nil {
			return err
		}
		pickup, delivery = first.String(), second.String()
		return nil
	})

	if pickup != "PRV-000000000001" {
		t.Errorf("首个揽收版本 = %q, want PRV-000000000001", pickup)
	}
	if delivery != "DRV-000000000001" {
		t.Errorf("首个交付版本 = %q, want DRV-000000000001（两条序列各自从 1 起）", delivery)
	}
}

// TestARolledBackTransactionStillBurnsTheNumber 证 nextval 不随事务回滚：一次未提交
// 的登记烧掉一个号，但绝不把这个号让给下一次——两份结果共用一个版本号会被登记表的
// 主键压成一行。
func TestARolledBackTransactionStillBurnsTheNumber(t *testing.T) {
	factory, transactor := newResultVersions(t)
	ctx := t.Context()
	rollback := errors.New("回滚")

	var burned string
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		version, err := factory.NextDeliveryResultVersion(txCtx)
		if err != nil {
			return err
		}
		burned = version.String()
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}

	var next string
	mustWithinVersionTransaction(t, transactor, ctx, func(txCtx context.Context) error {
		version, err := factory.NextDeliveryResultVersion(txCtx)
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

func TestMintingRefusesToRunOutsideATransaction(t *testing.T) {
	factory, _ := newResultVersions(t)
	ctx := t.Context()

	if _, err := factory.NextPickupResultVersion(ctx); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务签发揽收版本应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := factory.NextDeliveryResultVersion(ctx); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务签发交付版本应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := factory.NextExternalTrackingFactReference(ctx); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务签发外部轨迹事实身份应返回 ErrTransactionRequired，实得：%v", err)
	}
	if _, err := factory.NextExternalTrackingFactVersion(ctx); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务签发外部轨迹事实版本应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// TestExternalTrackingIdentitiesAreMintedFromTheirOwnSequences 证事实身份与版本各走一条序列，
// 号带前缀且两次不重——源事件标识由源给、本仓自己的身份另铸，两者分开保存（ADR-0102 决定四）。
func TestExternalTrackingIdentitiesAreMintedFromTheirOwnSequences(t *testing.T) {
	factory, transactor := newResultVersions(t)
	ctx := t.Context()
	var first, second, version string
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		a, err := factory.NextExternalTrackingFactReference(txCtx)
		if err != nil {
			return err
		}
		b, err := factory.NextExternalTrackingFactReference(txCtx)
		if err != nil {
			return err
		}
		v, err := factory.NextExternalTrackingFactVersion(txCtx)
		if err != nil {
			return err
		}
		first, second, version = a.String(), b.String(), v.String()
		return nil
	}); err != nil {
		t.Fatalf("签发：%v", err)
	}
	if first == second || !strings.HasPrefix(first, "EXTF-") || !strings.HasPrefix(version, "EXTV-") {
		t.Fatalf("身份与版本应各带前缀且不重：%q %q %q", first, second, version)
	}
}

// ---- 夹具 ----

func newResultVersions(t *testing.T) (*adapter.ResultVersions, bentoapp.Transactor) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	factory, err := adapter.NewResultVersions(db)
	if err != nil {
		t.Fatalf("构造版本签发器：%v", err)
	}
	return factory, db.Transactor()
}

func mustWithinVersionTransaction(
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
