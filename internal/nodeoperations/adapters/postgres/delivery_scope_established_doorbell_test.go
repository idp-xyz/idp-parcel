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
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证「交付范围成立」门铃（ADR-0212 决定六；票 ps-external-mark-relations/12）：两种触发同一类事件——最后
// 一个绑定确认时（租户，交接单），归入范围已成立的交接单时（租户，交接单，那一版判断版本）；信封身份由触发认领，重放不出第二封；
// 同一张交接单的门铃排一条队；与触发它的那一笔同生共死，无事务拒。

type scopeDoorbellFixture struct {
	doorbell   *adapter.OutboxDeliveryScopeEstablishedDoorbell
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newScopeDoorbellFixture(t *testing.T) *scopeDoorbellFixture {
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
	doorbell, err := adapter.NewOutboxDeliveryScopeEstablishedDoorbell(db, store, intakeHandoffClock{at: time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("构造门铃：%v", err)
	}
	return &scopeDoorbellFixture{doorbell: doorbell, transactor: db.Transactor(), pool: pool}
}

func scopeDoorbellRows(t *testing.T, pool *pgxpool.Pool) []doorbellRow {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT event_id, event_type, subject, scope, partition_key, payload FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_type = $1 ORDER BY event_id`, adapter.DeliveryScopeEstablishedEventType)
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

func scopeIntent(t *testing.T, tenant, note, version string) ports.DeliveryScopeEstablishedIntent {
	t.Helper()
	intent := ports.DeliveryScopeEstablishedIntent{
		Tenant:     ref(t, domain.NewTenantID, tenant),
		Note:       ref(t, domain.NewDeliveryHandoverNoteID, note),
		OccurredAt: time.Date(2026, 10, 19, 8, 30, 0, 0, time.UTC),
	}
	if version != "" {
		intent.Version = ref(t, domain.NewIntakeResultVersion, version)
	}
	return intent
}

// Covers: ADR-0212 决定六「交接单最后一个绑定得到确认时，同一笔事务给本上下文自投一封门铃（租户，交接单）；实物归入一张范围已成立的
// 交接单时，同一笔自投一封（租户，那一版判断版本）」——两种各认领自己的信封 ID，重放不出第二封；载荷只有租户、交接单与（归入那一种
// 的）判断版本，不带号码、实物、客户账户或候选；两种的分区键相同（同一张交接单的门铃排一条队），与信封 ID 不同源。
func TestTheScopeDoorbellNamesTheNoteOrTheAssignedVersion(t *testing.T) {
	fixture := newScopeDoorbellFixture(t)
	ctx := t.Context()
	whole := scopeIntent(t, "SYN-TENANT-PSXM12", "SYN-HANDOVER-PSXM12-DB", "")
	assigned := scopeIntent(t, "SYN-TENANT-PSXM12", "SYN-HANDOVER-PSXM12-DB", "SYN-INTAKEV-PSXM12-DB")
	ring := func(intent ports.DeliveryScopeEstablishedIntent) func(context.Context) error {
		return func(txCtx context.Context) error { return fixture.doorbell.RingDeliveryScopeEstablished(txCtx, intent) }
	}
	inTx(t, fixture.transactor, ctx, ring(whole))
	inTx(t, fixture.transactor, ctx, ring(assigned))
	inTx(t, fixture.transactor, ctx, ring(whole))
	inTx(t, fixture.transactor, ctx, ring(assigned))

	rows := scopeDoorbellRows(t, fixture.pool)
	if len(rows) != 2 {
		t.Fatalf("门铃 %d 封, want 两种各一封", len(rows))
	}
	byID := map[string]doorbellRow{}
	for _, row := range rows {
		byID[row.eventID] = row
	}
	noteID := string(outboxintent.FingerprintEventID("delivery-scope-established", "SYN-TENANT-PSXM12", "SYN-HANDOVER-PSXM12-DB"))
	unitID := string(outboxintent.FingerprintEventID("delivery-scope-established-unit", "SYN-TENANT-PSXM12", "SYN-HANDOVER-PSXM12-DB", "SYN-INTAKEV-PSXM12-DB"))
	noteRow, unitRow := byID[noteID], byID[unitID]
	switch {
	case noteRow.eventID == "" || unitRow.eventID == "":
		t.Fatalf("信封 ID = %v, want %q 与 %q", byID, noteID, unitID)
	case noteRow.scope != "SYN-TENANT-PSXM12" || noteRow.subject != "SYN-HANDOVER-PSXM12-DB" || unitRow.subject != "SYN-HANDOVER-PSXM12-DB":
		t.Fatalf("Scope %q Subject %q / %q, want 租户与交接单", noteRow.scope, noteRow.subject, unitRow.subject)
	case noteRow.partition != unitRow.partition || noteRow.partition == noteRow.eventID || noteRow.partition == "":
		t.Fatalf("分区键 %q / %q, want 同一张交接单同一个、与信封 ID 不同源", noteRow.partition, unitRow.partition)
	case len(noteRow.payload) != 2 || noteRow.payload["tenantId"] != "SYN-TENANT-PSXM12" || noteRow.payload["handoverNote"] != "SYN-HANDOVER-PSXM12-DB":
		t.Fatalf("交接单那一封载荷 = %v", noteRow.payload)
	case len(unitRow.payload) != 3 || unitRow.payload["judgmentVersion"] != "SYN-INTAKEV-PSXM12-DB" || unitRow.payload["handoverNote"] != "SYN-HANDOVER-PSXM12-DB":
		t.Fatalf("归入那一封载荷 = %v", unitRow.payload)
	}
}

// Covers: 门铃与触发它的那一笔同生共死——回滚一并消失；没有事务时拒；缺租户或交接单拒，不入一封指名不了任何东西的门铃。
func TestTheScopeDoorbellRollsBackAndRefusesToRunOutsideATransaction(t *testing.T) {
	fixture := newScopeDoorbellFixture(t)
	ctx := t.Context()
	intent := scopeIntent(t, "SYN-TENANT-PSXM12", "SYN-HANDOVER-PSXM12-RB", "")
	rollback := errors.New("回滚")
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := fixture.doorbell.RingDeliveryScopeEstablished(txCtx, intent); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚错误结束，实得：%v", err)
	}
	if rows := scopeDoorbellRows(t, fixture.pool); len(rows) != 0 {
		t.Fatalf("回滚后门铃 %d 封, want 0", len(rows))
	}
	if err := fixture.doorbell.RingDeliveryScopeEstablished(ctx, intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务 err = %v, want ErrTransactionRequired", err)
	}
	if err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.doorbell.RingDeliveryScopeEstablished(txCtx, ports.DeliveryScopeEstablishedIntent{Tenant: intent.Tenant})
	}); err == nil {
		t.Fatal("没指名交接单的门铃却没报错")
	}
}
