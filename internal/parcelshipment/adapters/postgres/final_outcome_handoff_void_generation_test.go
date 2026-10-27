package postgres_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件证终局信封的交接身份折进该包裹的作废代次（ADR-0295 决定六）：包裹上没有成立过「取消依据不成立」的决定时，信封 ID 与只认采用键四维的算法逐字相同；
// 每多一份这样的决定，同一采用键的终局交出另一封自己的信封。期望的 ID 是独立算出的字面（printf 拼 NUL 分隔的各维后取 sha256），不用生产公式重算。

const (
	// tenant-1/parcel-1/EFFECTIVE_DELIVERY/outcome-v1 四维的指纹。
	voidGenerationZeroEventID = "final-outcome/b4d68864261b1ffdd43de7565bb5e0e13f58c2f9b80e83c6b0d883159a6a7dab"
	// 同上再加一维 "1"。
	voidGenerationOneEventID = "final-outcome/209b174227e38eef6fbe6777dba3b0944ee76b97f6f661058731fb02e9983b64"
	// tenant-a/parcel-1/EFFECTIVE_DELIVERY/outcome-v1 四维的指纹。
	untouchedParcelEventID = "final-outcome/ab31270c82641514002f5f67f416e87720fcdb6359111d2e44a7b7a9a7a4c7b2"
)

// payloadKeysOf 读回一封信封载荷的字段名与作废代次（缺席时 present 为假）。
func payloadKeysOf(t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, eventID string) (keys []string, generation float64, present bool) {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(t.Context(), `SELECT payload FROM bento.outbox WHERE event_id = $1`, eventID).Scan(&raw); err != nil {
		t.Fatalf("读回载荷：%v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("载荷不是 JSON：%v", err)
	}
	for key := range body {
		keys = append(keys, key)
	}
	value, present := body["voidGeneration"]
	if present {
		generation, _ = value.(float64)
	}
	return keys, generation, present
}

// Covers: AT-PS-173；ADR-0295 决定六——没有作废决定的包裹，终局信封 ID 与只认采用键四维的算法逐字相同，已入队的信封与它们的重放去重不受影响。
func TestAFinalOutcomeOfAParcelWithoutAVoidDecisionKeepsTheIdItAlwaysHad(t *testing.T) {
	handoff, db, pool := newFinalOutcomeHandoffFixture(t)
	ctx := t.Context()

	for attempt := range 2 {
		if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			return handoff.HandOffFinalOutcome(txCtx, finalOutcomeHandoffIntent(t, "parcel-1", "outcome-v1"))
		}); err != nil {
			t.Fatalf("第 %d 次交接：%v", attempt+1, err)
		}
	}
	if count := countOutboxEventsIn(t, pool, untouchedParcelEventID); count != 1 {
		t.Fatalf("字面 ID %s 的信封 %d 封, want 1——代次为零的 ID 必须与旧算法逐字相同", untouchedParcelEventID, count)
	}
	if count := countOutboxEventsIn(t, pool, finalOutcomeEventID("parcel-1", "outcome-v1")); count != 1 {
		t.Fatalf("旧公式 ID 的信封 %d 封, want 1", count)
	}
	// 载荷也一样：代次为零时不带 voidGeneration 字段，只有采用键四维。
	if keys, _, present := payloadKeysOf(t, pool, untouchedParcelEventID); present || len(keys) != 4 {
		t.Fatalf("代次为零的载荷字段 = %v（带代次 %t）, want 只有采用键四维", keys, present)
	}
}

// Covers: AT-PS-173；ADR-0295 决定六——同一采用键的终局，在包裹上每成立一份「取消依据不成立」的决定之后各有各的信封：
// 代次为零的 ID 不变；交来源方更正的决定、别的包裹与别的租户上的决定都不算；同一代次内重放不翻倍。
func TestEachVoidDecisionOnTheParcelGivesTheSameAdoptionKeyAnotherEnvelope(t *testing.T) {
	rig := newConflictDecisionRig(t)
	store, err := outbox.NewStore(rig.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	handoff, err := adapter.NewOutboxFinalOutcomeHandoff(rig.db, store, handoffClock{at: conflictDecidedAt})
	if err != nil {
		t.Fatalf("构造意图适配器：%v", err)
	}
	handOff := func() {
		t.Helper()
		rig.inTx(t, func(ctx context.Context) error {
			return handoff.HandOffFinalOutcome(ctx, ports.FinalOutcomeHandoffIntent{
				Record: firstFinalRecord(t, "tenant-1", "parcel-1", "outcome-v1", "digest-1"),
			})
		})
	}
	formedEnvelopes := func() int {
		t.Helper()
		return rig.countRows(t, `SELECT count(*) FROM bento.outbox WHERE event_type = 'parcel-shipment.final-outcome.formed'`)
	}
	envelopesWith := func(eventID string) int {
		t.Helper()
		return rig.countRows(t, `SELECT count(*) FROM bento.outbox WHERE event_id = $1`, eventID)
	}
	conflictOn := func(tenant, parcel, cancellation string) ports.CancellationOrderConflictKey {
		t.Helper()
		return rig.recordConflict(t, tenant,
			conflictOf(t, parcel, cancellation, "NODE_INTAKE/v1", domain.NetworkIntakeBoundary, cancelRequestedAt, cancelRequestedAt.Add(-time.Hour)),
			conflictDetectedAt)
	}

	handOff()
	if formedEnvelopes() != 1 || envelopesWith(voidGenerationZeroEventID) != 1 {
		t.Fatalf("代次为零：formed %d 封、字面 ID 的 %d 封, want 各 1", formedEnvelopes(), envelopesWith(voidGenerationZeroEventID))
	}

	// 交来源方更正的决定不作废任何东西，不抬代次。
	rig.decide(t, decisionOf(t, conflictOn("tenant-1", "parcel-1", "PCXL-1"), domain.BoundaryFactReferred, "https://id.syn.example#alice"))
	// 别的包裹、别的租户上的作废决定不算这个包裹的代次。
	rig.decide(t, decisionOf(t, conflictOn("tenant-1", "parcel-2", "PCXL-2"), domain.CancellationNotUpheld, "https://id.syn.example#alice"))
	rig.decide(t, decisionOf(t, conflictOn("tenant-2", "parcel-1", "PCXL-3"), domain.CancellationNotUpheld, "https://id.syn.example#alice"))
	handOff()
	if formedEnvelopes() != 1 {
		t.Fatalf("无关决定之后 formed 信封 %d 封, want 1——它们不抬这个包裹的代次", formedEnvelopes())
	}

	rig.decide(t, decisionOf(t, conflictOn("tenant-1", "parcel-1", "PCXL-4"), domain.CancellationNotUpheld, "https://id.syn.example#alice"))
	handOff()
	handOff()
	if formedEnvelopes() != 2 || envelopesWith(voidGenerationOneEventID) != 1 || envelopesWith(voidGenerationZeroEventID) != 1 {
		t.Fatalf("一份作废决定之后：formed %d 封、代次一的字面 ID %d 封、代次零的 %d 封, want 2、1、1——同一采用键另有一封，同代次重放不翻倍",
			formedEnvelopes(), envelopesWith(voidGenerationOneEventID), envelopesWith(voidGenerationZeroEventID))
	}

	if _, generation, present := payloadKeysOf(t, rig.pool, voidGenerationOneEventID); !present || generation != 1 {
		t.Fatalf("代次一的载荷 voidGeneration = %v（在场 %t）, want 1", generation, present)
	}

	rig.decide(t, decisionOf(t, conflictOn("tenant-1", "parcel-1", "PCXL-5"), domain.CancellationNotUpheld, "https://id.syn.example#bob"))
	handOff()
	if formedEnvelopes() != 3 {
		t.Fatalf("两份作废决定之后 formed 信封 %d 封, want 3", formedEnvelopes())
	}
	if distinct := rig.countRows(t, `SELECT count(DISTINCT partition_key) FROM bento.outbox WHERE event_type = 'parcel-shipment.final-outcome.formed'`); distinct != 1 {
		t.Fatalf("三封信封分区键 %d 个, want 1——同一包裹的先后几封排在一条队里", distinct)
	}
}
