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
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证发生项已形成信封（票 demo-chain-gaps/27，ADR-0207 决定五）：与发生项同一笔落、同一笔
// 回滚；载荷恰是键三维、键名与消费门认的一字不差；ID 带版本且定长，重发不出第二封；同一发生项的两代同分区；无事务
// 拒、缺键拒。夹具全是合成登记，证据只记 S。

type chargeOccurrenceHandoffFixture struct {
	handoff     *adapter.OutboxChargeOccurrenceHandoff
	occurrences *adapter.ChargeOccurrences
	db          *bentopg.DB
	pool        *pgxpool.Pool
}

func newChargeOccurrenceHandoffFixture(t *testing.T) chargeOccurrenceHandoffFixture {
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
	handoff, err := adapter.NewOutboxChargeOccurrenceHandoff(db, store, tfHandoffClock{at: time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	occurrences, err := adapter.NewChargeOccurrences(db)
	if err != nil {
		t.Fatalf("构造发生项登记册：%v", err)
	}
	return chargeOccurrenceHandoffFixture{handoff: handoff, occurrences: occurrences, db: db, pool: pool}
}

// saveAndHandOff 在一笔事务里落发生项、交意图，再按 then 决定提交还是回滚——生产装配点就是这样包的。
func (fixture chargeOccurrenceHandoffFixture) saveAndHandOff(t *testing.T, record ports.ChargeOccurrenceRecord, then error) error {
	t.Helper()
	return fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if outcome, err := fixture.occurrences.Save(txCtx, record); err != nil || outcome != ports.ChargeOccurrenceSaved {
			return errors.Join(err, errors.New("charge occurrence was not saved fresh"))
		}
		if err := fixture.handoff.HandOffChargeOccurrence(txCtx, ports.ChargeOccurrenceIntent{Record: record}); err != nil {
			return err
		}
		return then
	})
}

type formedEnvelopeRow struct {
	id           string
	partitionKey string
	subject      string
	payload      map[string]string
}

func formedEnvelopesOf(t *testing.T, pool *pgxpool.Pool, occurrence string) []formedEnvelopeRow {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT event_id, partition_key, subject, payload FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_type = $1 AND subject = $2 ORDER BY event_id`,
		adapter.ChargeOccurrenceFormedEventType, occurrence)
	if err != nil {
		t.Fatalf("读信封：%v", err)
	}
	defer rows.Close()
	var found []formedEnvelopeRow
	for rows.Next() {
		var row formedEnvelopeRow
		var raw []byte
		if err := rows.Scan(&row.id, &row.partitionKey, &row.subject, &raw); err != nil {
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

func chargeOccurrenceRowCount(t *testing.T, pool *pgxpool.Pool, occurrence string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.transport_charge_occurrence WHERE occurrence_ref = $1`,
		occurrence).Scan(&count); err != nil {
		t.Fatalf("数发生项行：%v", err)
	}
	return count
}

// Covers: AT-TF-120 的「同笔交出已形成信封」与 ADR-0208 决定一的载荷形状——发生项与信封同一笔提交、同一笔回滚；
// 载荷恰三键；ID 定长且不同于分区键；重发同一份不出第二封。
func TestAChargeOccurrenceAndItsFormedEnvelopeCommitAndRollBackTogether(t *testing.T) {
	fixture := newChargeOccurrenceHandoffFixture(t)

	if err := fixture.saveAndHandOff(t, occurrenceRecord(t, "OCC-FORMED-01", "V1", "parcel-1"), nil); err != nil {
		t.Fatalf("登记并交意图：%v", err)
	}
	envelopes := formedEnvelopesOf(t, fixture.pool, "OCC-FORMED-01")
	if len(envelopes) != 1 {
		t.Fatalf("信封 %d 封, want 1", len(envelopes))
	}
	envelope := envelopes[0]
	if len(envelope.payload) != 3 || envelope.payload["tenantId"] != "tenant-1" ||
		envelope.payload["occurrence"] != "OCC-FORMED-01" || envelope.payload["version"] != "V1" {
		t.Fatalf("载荷 = %v, want 恰 tenantId / occurrence / version 三键", envelope.payload)
	}
	if envelope.partitionKey != "tenant-1/charge-occurrence/OCC-FORMED-01" || envelope.id == envelope.partitionKey {
		t.Fatalf("分区键 = %q，ID = %q", envelope.partitionKey, envelope.id)
	}
	if len(envelope.id) > eventing.MaxEventIDLength {
		t.Fatalf("ID 长 %d，超出框架上限", len(envelope.id))
	}

	rollback := errors.New("回滚")
	if err := fixture.saveAndHandOff(t, occurrenceRecord(t, "OCC-ROLLBACK-01", "V1", "parcel-2"), rollback); !errors.Is(err, rollback) {
		t.Fatalf("事务应以回滚结束：%v", err)
	}
	if rows, envelopes := chargeOccurrenceRowCount(t, fixture.pool, "OCC-ROLLBACK-01"),
		formedEnvelopesOf(t, fixture.pool, "OCC-ROLLBACK-01"); rows != 0 || len(envelopes) != 0 {
		t.Fatalf("回滚后留下发生项 %d 行、信封 %d 封", rows, len(envelopes))
	}

	replay := occurrenceRecord(t, "OCC-FORMED-01", "V1", "parcel-1")
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return fixture.handoff.HandOffChargeOccurrence(txCtx, ports.ChargeOccurrenceIntent{Record: replay})
	}); err != nil {
		t.Fatalf("重发：%v", err)
	}
	if again := formedEnvelopesOf(t, fixture.pool, "OCC-FORMED-01"); len(again) != 1 {
		t.Fatalf("重发后信封 %d 封, want 1", len(again))
	}
}

// Covers: 同一发生项的两个有效性版本各一封、同一分区——版本只进 ID，更正与原项排一条队。
func TestTwoValidityVersionsOfAnOccurrenceShareAPartition(t *testing.T) {
	fixture := newChargeOccurrenceHandoffFixture(t)
	for _, version := range []string{"V1", "V2"} {
		if err := fixture.saveAndHandOff(t, occurrenceRecord(t, "OCC-VERSIONS-01", version, "parcel-1"), nil); err != nil {
			t.Fatalf("登记 %s：%v", version, err)
		}
	}
	envelopes := formedEnvelopesOf(t, fixture.pool, "OCC-VERSIONS-01")
	if len(envelopes) != 2 || envelopes[0].id == envelopes[1].id {
		t.Fatalf("两代信封 = %+v, want 两封、ID 不同", envelopes)
	}
	if envelopes[0].partitionKey != envelopes[1].partitionKey {
		t.Fatalf("两代分区键 %q / %q 不同，就没有先后可言", envelopes[0].partitionKey, envelopes[1].partitionKey)
	}
}

func TestTheChargeOccurrenceHandoffRefusesNoTransactionAndABlankKey(t *testing.T) {
	fixture := newChargeOccurrenceHandoffFixture(t)
	intent := ports.ChargeOccurrenceIntent{Record: occurrenceRecord(t, "OCC-NTX-01", "V1", "parcel-1")}
	if err := fixture.handoff.HandOffChargeOccurrence(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return fixture.handoff.HandOffChargeOccurrence(txCtx, ports.ChargeOccurrenceIntent{})
	}); err == nil {
		t.Fatal("缺发生项键的意图入了队")
	}
}
