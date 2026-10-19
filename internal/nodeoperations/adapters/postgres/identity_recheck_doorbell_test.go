package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证落未核对行时自投的重核门铃（ADR-0199 决定三第三个触发，票 no-reception-source/05）：与未核对行
// 同一提交、回滚一并消失；信封只带（租户，判断版本），身份由那一版认领，重放不出第二封；无事务拒。

type doorbellFixture struct {
	receptions *adapter.Receptions
	doorbell   *adapter.OutboxIdentityRecheckDoorbell
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newDoorbellFixture(t *testing.T) *doorbellFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	receptions, err := adapter.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	doorbell, err := adapter.NewOutboxIdentityRecheckDoorbell(db, store, intakeHandoffClock{at: time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("构造门铃：%v", err)
	}
	return &doorbellFixture{receptions: receptions, doorbell: doorbell, transactor: db.Transactor(), pool: pool}
}

type doorbellRow struct {
	eventID, eventType, subject, scope, partition string
	payload                                       map[string]string
}

func doorbellRows(t *testing.T, pool *pgxpool.Pool) []doorbellRow {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT event_id, event_type, subject, scope, partition_key, payload FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_type = $1 ORDER BY event_id`, adapter.UncheckedPendingIdentityRecordedEventType)
	if err != nil {
		t.Fatalf("读门铃：%v", err)
	}
	defer rows.Close()
	var found []doorbellRow
	for rows.Next() {
		var row doorbellRow
		var raw []byte
		if err := rows.Scan(&row.eventID, &row.eventType, &row.subject, &row.scope, &row.partition, &raw); err != nil {
			t.Fatalf("扫门铃：%v", err)
		}
		if err := json.Unmarshal(raw, &row.payload); err != nil {
			t.Fatalf("门铃载荷：%v", err)
		}
		found = append(found, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读门铃：%v", err)
	}
	return found
}

// Covers: ADR-0199 决定三「落下未核对行的同一笔事务给本上下文自投一封门铃，只带（租户，那一版的判断版本），信封身份由那一版
// 认领」——与未核对行同一提交，信封 ID 是（租户，判断版本）的定长指纹，载荷只有租户与判断版本，不带号码、实物或候选；分区键
// 与 ID 不同源（同一件实物的门铃排一条队）。同一版再响一次不出第二封。
func TestTheDoorbellCommitsWithTheUncheckedRowAndCarriesOnlyTheVersion(t *testing.T) {
	fixture := newDoorbellFixture(t)
	ctx := t.Context()
	record := uncheckedPending(t, "SYN-TENANT-NRS05", "SYN-SCAN-NRS05-DOORBELL", "SYN-INTAKEV-NRS05-DOORBELL")
	ring := func(txCtx context.Context) error {
		return fixture.doorbell.RingIdentityRecheck(txCtx, ports.IdentityRecheckDoorbellIntent{Record: record})
	}
	inTx(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		if _, err := fixture.receptions.Save(txCtx, record); err != nil {
			return err
		}
		return ring(txCtx)
	})
	inTx(t, fixture.transactor, ctx, ring)

	rows := doorbellRows(t, fixture.pool)
	if len(rows) != 1 {
		t.Fatalf("门铃 %d 封, want 恰一封", len(rows))
	}
	got := rows[0]
	wantID := string(outboxintent.FingerprintEventID("unchecked-pending-identity", "SYN-TENANT-NRS05", "SYN-INTAKEV-NRS05-DOORBELL"))
	switch {
	case got.eventID != wantID:
		t.Fatalf("信封 ID = %q, want %q", got.eventID, wantID)
	case got.scope != "SYN-TENANT-NRS05" || got.subject != "SYN-INTAKEV-NRS05-DOORBELL":
		t.Fatalf("Scope %q Subject %q, want 租户与判断版本", got.scope, got.subject)
	case got.partition == "" || got.partition == got.eventID:
		t.Fatalf("分区键 = %q, want 与信封 ID 不同源", got.partition)
	case len(got.payload) != 2 || got.payload["tenantId"] != "SYN-TENANT-NRS05" || got.payload["judgmentVersion"] != "SYN-INTAKEV-NRS05-DOORBELL":
		t.Fatalf("载荷 = %v, want 只有 tenantId 与 judgmentVersion", got.payload)
	}
}

// Covers: 门铃与未核对行同生共死——回滚一并消失；没有事务时拒，不在事务外悄悄入队一封指名不存在的版本的门铃。
func TestTheDoorbellRollsBackWithTheRowAndRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newDoorbellFixture(t)
	ctx := t.Context()
	record := uncheckedPending(t, "SYN-TENANT-NRS05", "SYN-SCAN-NRS05-ROLLBACK", "SYN-INTAKEV-NRS05-ROLLBACK")
	rollback := errors.New("回滚")
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.receptions.Save(txCtx, record); err != nil {
			return err
		}
		if err := fixture.doorbell.RingIdentityRecheck(txCtx, ports.IdentityRecheckDoorbellIntent{Record: record}); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if rows := doorbellRows(t, fixture.pool); len(rows) != 0 {
		t.Fatalf("回滚后门铃 %d 封, want 0", len(rows))
	}
	if err := fixture.doorbell.RingIdentityRecheck(ctx, ports.IdentityRecheckDoorbellIntent{Record: record}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务 err = %v, want ErrTransactionRequired", err)
	}
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.doorbell.RingIdentityRecheck(txCtx, ports.IdentityRecheckDoorbellIntent{Record: formedRecord(t, "SYN-TENANT-NRS05", "SYN-SCAN-NRS05-FORMED")})
	}); err == nil {
		t.Fatal("给没有判断版本的记录响门铃却没报错")
	}
}
