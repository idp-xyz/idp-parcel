package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

func newCommissionHandoffFixture(t *testing.T) (*adapter.OutboxTransportCommissionHandoff, *bentopg.DB, *pgxpool.Pool) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxTransportCommissionHandoff(db, store, tfHandoffClock{
		at: time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	return handoff, db, pool
}

func commissionHandoffIntent(t *testing.T, id string) ports.TransportCommissionIntent {
	t.Helper()
	return ports.TransportCommissionIntent{Record: submittedCommission(t, "tenant-a", id)}
}

// commissionEventID 照生产同一公式算本口的信封 ID（口名前缀加租户与委托两维的指纹）。
func commissionEventID(tenant, commission string) string {
	return string(outboxintent.FingerprintEventID("transport-commission", tenant, commission))
}

// TestTransportCommissionFollowsTheTransactionalTemplate 证委托意图复现样板四条：
// 首发一行、回滚无痕、重发同一份、无事务拒。信封 ID 由委托幂等键认领。
func TestTransportCommissionFollowsTheTransactionalTemplate(t *testing.T) {
	handoff, db, pool := newCommissionHandoffFixture(t)
	ctx := t.Context()
	transactor := db.Transactor()

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffTransportCommission(txCtx, commissionHandoffIntent(t, "commission-1"))
	}); err != nil {
		t.Fatalf("首发：%v", err)
	}
	if count := countTFIntents(t, pool, commissionEventID("tenant-a", "commission-1")); count != 1 {
		t.Fatalf("commission-1 行数 = %d, want 1", count)
	}

	rollback := errors.New("回滚")
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := handoff.HandOffTransportCommission(txCtx, commissionHandoffIntent(t, "commission-2")); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if count := countTFIntents(t, pool, commissionEventID("tenant-a", "commission-2")); count != 0 {
		t.Fatalf("回滚后 commission-2 行数 = %d, want 0", count)
	}

	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return handoff.HandOffTransportCommission(txCtx, commissionHandoffIntent(t, "commission-1"))
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if count := countTFIntents(t, pool, commissionEventID("tenant-a", "commission-1")); count != 1 {
		t.Fatalf("重发后 commission-1 行数 = %d, want 1——重发的必须是同一份", count)
	}

	if err := handoff.HandOffTransportCommission(ctx, commissionHandoffIntent(t, "commission-3")); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func TestTransportCommissionRefusesABlankKey(t *testing.T) {
	handoff, db, _ := newCommissionHandoffFixture(t)
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffTransportCommission(txCtx, ports.TransportCommissionIntent{})
	})
	if err == nil {
		t.Fatal("缺幂等键的委托意图入了队")
	}
}

// Covers: 委托信封 ID 定长、分区键保留可读形——委托标识取到 eventing.MaxEventIDLength 那么长，原样串接的 ID 必然越限，
// 指纹 ID 照样入队（不随两维变长）；分区键仍是「租户/委托/口名」，运维认得出是哪份委托。只证到信封 ID 上限这一档：
// 分区键与 Subject 仍随标识变长，越过 eventing.MaxPartitionKeyLength 照样被确定性拒收——envelope-id-bounds/01 登记的
// 残余，去处是票 demo-chain-gaps/41。
func TestATransportCommissionEnqueuesPastTheConcatenatedIDLimit(t *testing.T) {
	handoff, db, pool := newCommissionHandoffFixture(t)
	long := strings.Repeat("c", eventing.MaxEventIDLength)
	intent := ports.TransportCommissionIntent{Record: submittedCommission(t, "tenant-a", long)}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffTransportCommission(txCtx, intent)
	}); err != nil {
		t.Fatalf("长标识入队：%v", err)
	}
	if count := countTFIntents(t, pool, commissionEventID("tenant-a", long)); count != 1 {
		t.Fatalf("长标识的委托意图行数 = %d, want 1", count)
	}
	var partition string
	if err := pool.QueryRow(t.Context(),
		`SELECT partition_key FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`, commissionEventID("tenant-a", long),
	).Scan(&partition); err != nil {
		t.Fatalf("读分区键：%v", err)
	}
	if partition != "tenant-a/"+long+"/transport-commission" {
		t.Fatalf("分区键 = %q", partition)
	}
}
