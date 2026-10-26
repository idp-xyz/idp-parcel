package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证出发门禁补问登记册与它的信封（迁移 0052，ADR-0305）：一次补问一行、只插不改，首版读数在移动事实
// 那一行上不动；按出发列出时按落册时刻、对象、次序排；撞键答已登记、不覆盖；只补在册的出发；装不回的那一行报得出来；信封与那一行
// 同一笔落、载荷恰是键五维、与那条出发同一分区、重发不出第二封；写口无事务即拒。夹具全为合成登记（S 级）。

type departureGateReaskFixture struct {
	reasks  *adapter.DepartureGateReasks
	facts   *adapter.MovementFacts
	handoff *adapter.OutboxDepartureGateReaskHandoff
	db      *bentopg.DB
	pool    *pgxpool.Pool
}

func newDepartureGateReaskFixture(t *testing.T) departureGateReaskFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	reasks, err := adapter.NewDepartureGateReasks(db)
	if err != nil {
		t.Fatalf("构造补问册：%v", err)
	}
	facts, err := adapter.NewMovementFacts(db)
	if err != nil {
		t.Fatalf("构造移动事实册：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxDepartureGateReaskHandoff(db, store, tfHandoffClock{at: time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("构造补问交出：%v", err)
	}
	return departureGateReaskFixture{reasks: reasks, facts: facts, handoff: handoff, db: db, pool: pool}
}

// departure 登一条出发 fact（版本 v1，夹具读数），交回它的记录。
func (fixture departureGateReaskFixture) departure(t *testing.T, fact string) ports.MovementFactRecord {
	t.Helper()
	record := movementFactRecord(t, fact, "v1", domain.DepartureFact, "")
	mustSaveMovementFact(t, fixture.db.Transactor(), t.Context(), fixture.facts, record)
	return record
}

func reaskOf(
	t *testing.T,
	departure ports.MovementFactRecord,
	object string,
	sequence int,
	reading domain.DepartureGateReading,
	at time.Time,
) ports.DepartureGateReaskRecord {
	t.Helper()
	return ports.DepartureGateReaskRecord{
		Key: ports.DepartureGateReaskKey{
			TenantID: departure.Key.TenantID, Fact: departure.Key.Fact, Version: departure.Key.Version,
			Object: segmentRef(t, domain.NewCarriedObjectReference, object), Sequence: sequence,
		},
		Reading:    reading,
		Trigger:    "transport-handover/" + object + "/scope-1/v1",
		RecordedAt: at,
	}
}

func blockedReaskReading(t *testing.T, object string) domain.DepartureGateReading {
	t.Helper()
	finding, err := domain.NewDepartureGateFinding(segmentRef(t, domain.NewCarriedObjectReference, object), domain.DepartureGateBlocked,
		"AADJ-"+object, []domain.DepartureGateBasis{{Action: "LOADING_DEPARTURE", Cause: "RESTRICTION_IN_FORCE", Sources: []string{"RST-9"}}})
	if err != nil {
		t.Fatalf("阻断那一格：%v", err)
	}
	reading, err := domain.FormDepartureGateReading(domain.DepartureGateReadingSpec{
		From:     segmentRef(t, domain.NewNetworkNodeReference, "NODE-SHA"),
		To:       segmentRef(t, domain.NewNetworkNodeReference, "NODE-SZX"),
		Findings: []domain.DepartureGateFinding{finding},
	})
	if err != nil {
		t.Fatalf("补问读数：%v", err)
	}
	return reading
}

func stoppedReaskReading(t *testing.T, stop domain.DepartureGateStop) domain.DepartureGateReading {
	t.Helper()
	reading, err := domain.FormDepartureGateReading(domain.DepartureGateReadingSpec{Stops: []domain.DepartureGateStop{stop}})
	if err != nil {
		t.Fatalf("停在本侧的补问读数：%v", err)
	}
	return reading
}

func (fixture departureGateReaskFixture) save(t *testing.T, record ports.DepartureGateReaskRecord) ports.DepartureGateReaskSaveOutcome {
	t.Helper()
	var outcome ports.DepartureGateReaskSaveOutcome
	mustWithinDispatchTaskTransaction(t, fixture.db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = fixture.reasks.Save(txCtx, record)
		return err
	})
	return outcome
}

// Covers: AT-TF-343 存储半边（ADR-0305，迁移 0052）——补问一次一行：按键往返（读数、唤醒依据、落册时刻原样）；按出发列出时各件混在一起，
// 按落册时刻、对象、次序排；撞键答已登记、在册那一份不变；首版读数留在移动事实那一行上不动；没在册的出发补不进去（外键）。
func TestADepartureGateReaskAppendsAndRoundTrips(t *testing.T) {
	fixture := newDepartureGateReaskFixture(t)
	departure := fixture.departure(t, "MF-REASK-01")
	at := movedAtDBFixture.Add(3 * time.Hour)

	later := reaskOf(t, departure, "parcel-1", 2, stoppedReaskReading(t, domain.DepartureScheduleNotRegistered), at.Add(2*time.Minute))
	first := reaskOf(t, departure, "parcel-1", 1, blockedReaskReading(t, "parcel-1"), at)
	other := reaskOf(t, departure, "parcel-0", 1, stoppedReaskReading(t, domain.DepartureSegmentRegistryUnavailable), at.Add(time.Minute))
	for _, record := range []ports.DepartureGateReaskRecord{later, first, other} {
		if outcome := fixture.save(t, record); outcome != ports.DepartureGateReaskSaved {
			t.Fatalf("追加 %+v = %d", record.Key, outcome)
		}
	}

	found, exists, err := fixture.reasks.FindByKey(t.Context(), first.Key)
	if err != nil || !exists {
		t.Fatalf("按键取回：%v exists=%v", err, exists)
	}
	if found.Key != first.Key || found.Trigger != first.Trigger || !found.RecordedAt.Equal(at) ||
		found.Reading.Answer() != domain.DepartureGateBlocked {
		t.Fatalf("往返变形：%+v", found)
	}
	judgement, judged := found.Reading.Findings()[0].Judgement()
	if !judged || judgement != "AADJ-parcel-1" || found.Reading.Findings()[0].Bases()[0].Sources[0] != "RST-9" {
		t.Fatalf("关务判断标识与依据原文没随补问读回：%q %+v", judgement, found.Reading.Findings()[0].Bases())
	}

	listed, err := fixture.reasks.FindByDeparture(t.Context(), departure.Key)
	if err != nil {
		t.Fatalf("按出发列出：%v", err)
	}
	var order []string
	for _, record := range listed {
		order = append(order, fmt.Sprintf("%s#%d", record.Key.Object, record.Key.Sequence))
	}
	if len(order) != 3 || order[0] != "parcel-1#1" || order[1] != "parcel-0#1" || order[2] != "parcel-1#2" {
		t.Fatalf("列出次序 = %v, want 按落册时刻", order)
	}
	if empty, err := fixture.reasks.FindByDeparture(t.Context(),
		movementFactKeyFixture(t, "tenant-1", "MF-REASK-NONE", "v1")); err != nil || len(empty) != 0 {
		t.Fatalf("没补问过的出发列出 %d 份 err=%v", len(empty), err)
	}

	again := first
	again.Reading = stoppedReaskReading(t, domain.DepartureSegmentRegistryUnavailable)
	if outcome := fixture.save(t, again); outcome != ports.DepartureGateReaskAlreadyRegistered {
		t.Fatalf("撞键 outcome = %d, want 已登记", outcome)
	}
	if kept, _, _ := fixture.reasks.FindByKey(t.Context(), first.Key); kept.Reading.Answer() != domain.DepartureGateBlocked {
		t.Fatalf("撞键覆盖了在册那一份：%s", kept.Reading.Answer())
	}

	stored, _, err := fixture.facts.FindByKey(t.Context(), departure.Key)
	if err != nil {
		t.Fatalf("读回出发：%v", err)
	}
	if reading, _ := stored.Fact.GateReading(); reading.Answer() != domain.DepartureGateBlocked || len(reading.Findings()) != 2 {
		t.Fatalf("首版读数被动了：%s %d 格", reading.Answer(), len(reading.Findings()))
	}

	orphan := reaskOf(t, departure, "parcel-1", 1, blockedReaskReading(t, "parcel-1"), at)
	orphan.Key.Fact = segmentRef(t, domain.NewMovementFactReference, "MF-NOT-RECORDED")
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, err := fixture.reasks.Save(txCtx, orphan)
		return err
	}); err == nil {
		t.Fatal("补问补进了一条不在册的出发")
	}
}

// Covers: AT-TF-346 存储半边——补问那一行装不回（库面被绕过构造门写过：整格与明细对不上）时，按键取与按出发列都报错并包住坏行哨兵，
// 与读不动分得开；列的那一问不跳过那一行。
func TestAnUnreadableReaskRowIsNamed(t *testing.T) {
	fixture := newDepartureGateReaskFixture(t)
	departure := fixture.departure(t, "MF-REASK-BAD")
	record := reaskOf(t, departure, "parcel-1", 1, blockedReaskReading(t, "parcel-1"), movedAtDBFixture.Add(time.Hour))
	fixture.save(t, record)
	if _, err := fixture.pool.Exec(t.Context(),
		`UPDATE transport_fulfillment.departure_gate_reask SET gate_reading = 'NOT_GUARDED' WHERE fact_ref = 'MF-REASK-BAD'`); err != nil {
		t.Fatalf("改坏那一行：%v", err)
	}

	if _, _, err := fixture.reasks.FindByKey(t.Context(), record.Key); !errors.Is(err, ports.ErrDepartureGateReaskRecordUnreadable) {
		t.Fatalf("按键取坏行 err = %v, want ErrDepartureGateReaskRecordUnreadable", err)
	}
	if _, err := fixture.reasks.FindByDeparture(t.Context(), departure.Key); !errors.Is(err, ports.ErrDepartureGateReaskRecordUnreadable) {
		t.Fatalf("按出发列坏行 err = %v, want ErrDepartureGateReaskRecordUnreadable", err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := fixture.reasks.FindByKey(canceled, record.Key); err == nil ||
		errors.Is(err, ports.ErrDepartureGateReaskRecordUnreadable) {
		t.Fatalf("读不动 err = %v，不该带坏行哨兵", err)
	}
}

// Covers: AT-TF-343 存储半边（信封）——补问信封与那一行同一笔落、同一笔回滚；载荷恰是键五维，读数不随信封走；分区与它补的那条出发
// 的已形成信封同一区；ID 定长、带对象与次序，重发同一份不出第二封；缺键拒。
func TestAReaskEnvelopeTravelsWithItsRowInTheDeparturePartition(t *testing.T) {
	fixture := newDepartureGateReaskFixture(t)
	departure := fixture.departure(t, "MF-REASK-ENV")
	movementStore, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	movementHandoff, err := adapter.NewOutboxMovementFactHandoff(fixture.db, movementStore, tfHandoffClock{at: movedAtDBFixture})
	if err != nil {
		t.Fatalf("构造移动事实交出：%v", err)
	}
	mustWithinDispatchTaskTransaction(t, fixture.db.Transactor(), t.Context(), func(txCtx context.Context) error {
		return movementHandoff.HandOffMovementFact(txCtx, ports.MovementFactIntent{Record: departure})
	})

	rolledBack := errors.New("回滚")
	record := reaskOf(t, departure, "parcel-1", 1, blockedReaskReading(t, "parcel-1"), movedAtDBFixture.Add(time.Hour))
	saveAndHandOff := func(then error) error {
		return fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
			if outcome, err := fixture.reasks.Save(txCtx, record); err != nil || outcome != ports.DepartureGateReaskSaved {
				return errors.Join(err, errors.New("reask was not saved fresh"))
			}
			if err := fixture.handoff.HandOffDepartureGateReask(txCtx, record); err != nil {
				return err
			}
			return then
		})
	}
	if err := saveAndHandOff(rolledBack); !errors.Is(err, rolledBack) {
		t.Fatalf("回滚那一笔 err = %v", err)
	}
	if envelopes := reaskEnvelopesOf(t, fixture.pool, "MF-REASK-ENV"); len(envelopes) != 0 {
		t.Fatalf("回滚之后还留着 %d 封补问信封", len(envelopes))
	}
	if err := saveAndHandOff(nil); err != nil {
		t.Fatalf("落补问：%v", err)
	}
	mustWithinDispatchTaskTransaction(t, fixture.db.Transactor(), t.Context(), func(txCtx context.Context) error {
		return fixture.handoff.HandOffDepartureGateReask(txCtx, record)
	})

	envelopes := reaskEnvelopesOf(t, fixture.pool, "MF-REASK-ENV")
	if len(envelopes) != 1 {
		t.Fatalf("补问信封 %d 封, want 1（重发吞掉）", len(envelopes))
	}
	envelope := envelopes[0]
	want := map[string]string{"tenantId": "tenant-1", "fact": "MF-REASK-ENV", "version": "v1", "object": "parcel-1", "sequence": "1"}
	if len(envelope.payload) != len(want) {
		t.Fatalf("载荷 = %v, want 恰五键", envelope.payload)
	}
	for key, value := range want {
		if envelope.payload[key] != value {
			t.Fatalf("载荷 %s = %q, want %q", key, envelope.payload[key], value)
		}
	}
	movement := recordedEnvelopesOf(t, fixture.pool, "MF-REASK-ENV")
	if len(movement) != 1 || movement[0].partitionKey != envelope.partitionKey || movement[0].id == envelope.id {
		t.Fatalf("补问信封分区 %q / 出发信封 %+v——要与它补的那条出发同一区、ID 不同", envelope.partitionKey, movement)
	}

	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		return fixture.handoff.HandOffDepartureGateReask(txCtx, ports.DepartureGateReaskRecord{})
	}); err == nil {
		t.Fatal("缺键的补问入了队")
	}
}

// TestDepartureGateReaskWritesRefuseToRunOutsideATransaction 证补问的两个写口无环境事务即拒。断言指名 ErrTransactionRequired。
func TestDepartureGateReaskWritesRefuseToRunOutsideATransaction(t *testing.T) {
	fixture := newDepartureGateReaskFixture(t)
	departure := fixture.departure(t, "MF-REASK-NTX")
	record := reaskOf(t, departure, "parcel-1", 1, blockedReaskReading(t, "parcel-1"), movedAtDBFixture.Add(time.Hour))

	if _, err := fixture.reasks.Save(t.Context(), record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务追加应返回 ErrTransactionRequired，实得：%v", err)
	}
	if err := fixture.handoff.HandOffDepartureGateReask(t.Context(), record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务入队应返回 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: 迁移 0052 的 CHECK——领域造不出的补问行进不了库：次序不是正数、唤醒依据空白、整格不在封闭集、停点与逐对象答复都空。
func TestDepartureGateReaskCheckConstraintsRejectRowsTheDomainCannotProduce(t *testing.T) {
	fixture := newDepartureGateReaskFixture(t)
	fixture.departure(t, "MF-REASK-CHK")
	for name, values := range map[string][]any{
		"次序为零":    {0, "UNDECIDED", `{"stops":["SEGMENT_NOT_REGISTERED"],"findings":[]}`, "trigger"},
		"唤醒依据空白":  {1, "UNDECIDED", `{"stops":["SEGMENT_NOT_REGISTERED"],"findings":[]}`, " "},
		"整格不在集里":  {1, "MAYBE", `{"stops":["SEGMENT_NOT_REGISTERED"],"findings":[]}`, "trigger"},
		"停点与对象都空": {1, "UNDECIDED", `{"stops":[],"findings":[]}`, "trigger"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := fixture.pool.Exec(t.Context(),
				`INSERT INTO transport_fulfillment.departure_gate_reask
				     (tenant_id, fact_ref, version, object_ref, sequence, gate_reading, gate_reading_detail, trigger_ref, recorded_at)
				 VALUES ('tenant-1', 'MF-REASK-CHK', 'v1', 'parcel-1', $1, $2, $3::jsonb, $4, now())`, values...)
			if err == nil {
				t.Fatal("领域造不出的行落进去了")
			}
		})
	}
}

type reaskEnvelopeRow struct {
	id           string
	partitionKey string
	payload      map[string]string
}

func reaskEnvelopesOf(t *testing.T, pool *pgxpool.Pool, fact string) []reaskEnvelopeRow {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT event_id, partition_key, payload FROM `+migrate.SchemaBento+`.outbox
		  WHERE event_type = $1 AND subject = $2 ORDER BY event_id`,
		adapter.DepartureGateReaskRecordedEventType, fact)
	if err != nil {
		t.Fatalf("读补问信封：%v", err)
	}
	defer rows.Close()
	var found []reaskEnvelopeRow
	for rows.Next() {
		var row reaskEnvelopeRow
		var raw []byte
		if err := rows.Scan(&row.id, &row.partitionKey, &raw); err != nil {
			t.Fatalf("读补问信封行：%v", err)
		}
		if err := json.Unmarshal(raw, &row.payload); err != nil {
			t.Fatalf("译载荷：%v", err)
		}
		found = append(found, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读补问信封行：%v", err)
	}
	return found
}
