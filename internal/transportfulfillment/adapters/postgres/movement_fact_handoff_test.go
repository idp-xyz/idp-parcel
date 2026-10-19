package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证移动事实已形成信封（票 demo-chain-gaps/33，ADR-0205 决定五）：与事实同一笔落、同一笔回滚；
// 载荷恰是键三维、键名与消费门认的一字不差；ID 带版本且定长，重发不出第二封；同一事实的两代同分区；无事务拒、缺键拒。
// 夹具全是合成登记，证据只记 S。

type movementFactHandoffFixture struct {
	handoff *adapter.OutboxMovementFactHandoff
	facts   *adapter.MovementFacts
	db      *bentopg.DB
	pool    *pgxpool.Pool
}

func newMovementFactHandoffFixture(t *testing.T) movementFactHandoffFixture {
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
	handoff, err := adapter.NewOutboxMovementFactHandoff(db, store, tfHandoffClock{at: time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	facts, err := adapter.NewMovementFacts(db)
	if err != nil {
		t.Fatalf("构造移动事实登记册：%v", err)
	}
	return movementFactHandoffFixture{handoff: handoff, facts: facts, db: db, pool: pool}
}

// saveAndHandOff 在一笔事务里落事实、交意图，再按 then 决定提交还是回滚——生产装配点就是这样包的。
func (fixture movementFactHandoffFixture) saveAndHandOff(t *testing.T, record ports.MovementFactRecord, then error) error {
	t.Helper()
	return fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if outcome, err := fixture.facts.Save(txCtx, record); err != nil || outcome != ports.MovementFactSaved {
			return errors.Join(err, errors.New("movement fact was not saved fresh"))
		}
		if err := fixture.handoff.HandOffMovementFact(txCtx, ports.MovementFactIntent{Record: record}); err != nil {
			return err
		}
		return then
	})
}

type recordedEnvelopeRow struct {
	id           string
	partitionKey string
	payload      map[string]string
}

func recordedEnvelopesOf(t *testing.T, pool *pgxpool.Pool, fact string) []recordedEnvelopeRow {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT event_id, partition_key, payload FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_type = $1 AND subject = $2 ORDER BY event_id`,
		adapter.MovementFactRecordedEventType, fact)
	if err != nil {
		t.Fatalf("读信封：%v", err)
	}
	defer rows.Close()
	var found []recordedEnvelopeRow
	for rows.Next() {
		var row recordedEnvelopeRow
		var raw []byte
		if err := rows.Scan(&row.id, &row.partitionKey, &raw); err != nil {
			t.Fatalf("读信封行：%v", err)
		}
		if err := json.Unmarshal(raw, &row.payload); err != nil {
			t.Fatalf("译载荷：%v", err)
		}
		found = append(found, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读信封行：%v", err)
	}
	return found
}

func movementFactRowCount(t *testing.T, pool *pgxpool.Pool, fact string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.transport_movement_fact WHERE fact_ref = $1`, fact).Scan(&count); err != nil {
		t.Fatalf("数事实行：%v", err)
	}
	return count
}

// Covers: AT-TF-156 的「同笔交出已形成信封」——出发与信封同一笔提交、同一笔回滚；载荷恰三键，读数不随信封走；ID 定长且不同
// 于分区键；重发同一份不出第二封。
func TestAMovementFactAndItsRecordedEnvelopeCommitAndRollBackTogether(t *testing.T) {
	fixture := newMovementFactHandoffFixture(t)

	if err := fixture.saveAndHandOff(t, movementFactRecord(t, "MF-HANDOFF-01", "v1", domain.DepartureFact, ""), nil); err != nil {
		t.Fatalf("登记并交意图：%v", err)
	}
	envelopes := recordedEnvelopesOf(t, fixture.pool, "MF-HANDOFF-01")
	if len(envelopes) != 1 {
		t.Fatalf("信封 %d 封, want 1", len(envelopes))
	}
	envelope := envelopes[0]
	if len(envelope.payload) != 3 || envelope.payload["tenantId"] != "tenant-1" ||
		envelope.payload["fact"] != "MF-HANDOFF-01" || envelope.payload["version"] != "v1" {
		t.Fatalf("载荷 = %v, want 恰 tenantId / fact / version 三键", envelope.payload)
	}
	if envelope.partitionKey != "tenant-1/movement-fact/MF-HANDOFF-01" || envelope.id == envelope.partitionKey {
		t.Fatalf("分区键 = %q，ID = %q", envelope.partitionKey, envelope.id)
	}
	if len(envelope.id) > eventing.MaxEventIDLength {
		t.Fatalf("ID 长 %d，超出框架上限", len(envelope.id))
	}

	rollback := errors.New("回滚")
	if err := fixture.saveAndHandOff(t, movementFactRecord(t, "MF-ROLLBACK-01", "v1", domain.ArrivalFact, ""), rollback); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚结束：%v", err)
	}
	if rows, envelopes := movementFactRowCount(t, fixture.pool, "MF-ROLLBACK-01"),
		recordedEnvelopesOf(t, fixture.pool, "MF-ROLLBACK-01"); rows != 0 || len(envelopes) != 0 {
		t.Fatalf("回滚后留下事实 %d 行、信封 %d 封", rows, len(envelopes))
	}

	replay := movementFactRecord(t, "MF-HANDOFF-01", "v1", domain.DepartureFact, "")
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return fixture.handoff.HandOffMovementFact(txCtx, ports.MovementFactIntent{Record: replay})
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if again := recordedEnvelopesOf(t, fixture.pool, "MF-HANDOFF-01"); len(again) != 1 {
		t.Fatalf("重发后信封 %d 封, want 1", len(again))
	}
}

// Covers: 同一事实的两个版本各一封、同一分区——版本只进 ID，更正与原版排一条队。
func TestTwoVersionsOfAMovementFactShareAPartition(t *testing.T) {
	fixture := newMovementFactHandoffFixture(t)
	for _, version := range []string{"v1", "v2"} {
		if err := fixture.saveAndHandOff(t, movementFactRecord(t, "MF-VERSIONS-01", version, domain.ArrivalFact, ""), nil); err != nil {
			t.Fatalf("登记 %s：%v", version, err)
		}
	}
	envelopes := recordedEnvelopesOf(t, fixture.pool, "MF-VERSIONS-01")
	if len(envelopes) != 2 || envelopes[0].id == envelopes[1].id {
		t.Fatalf("两代信封 = %+v, want 两封、ID 不同", envelopes)
	}
	if envelopes[0].partitionKey != envelopes[1].partitionKey {
		t.Fatalf("两代分区键 %q / %q 不同，就没有先后可言", envelopes[0].partitionKey, envelopes[1].partitionKey)
	}
}

func TestTheMovementFactHandoffRefusesNoTransactionAndABlankKey(t *testing.T) {
	fixture := newMovementFactHandoffFixture(t)
	intent := ports.MovementFactIntent{Record: movementFactRecord(t, "MF-NTX-01", "v1", domain.ArrivalFact, "")}
	if err := fixture.handoff.HandOffMovementFact(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return fixture.handoff.HandOffMovementFact(txCtx, ports.MovementFactIntent{})
	}); err == nil {
		t.Fatal("缺事实键的意图入了队")
	}
}
