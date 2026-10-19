package main

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// Covers: AT-TF-156 生产装配半边（票 demo-chain-gaps/33，ADR-0205 决定五）——`/transport-fulfillment/movement-facts` 背后的生产编排
// 在登记出发的那一笔事务里同时入队移动事实已形成信封；同内容重投重交同一份，不出第二封。演示种子登节点区域与周期安排、交接
// 立段之后走的是与演示判据同一条线，证据只记 S。信封被谁消费、投成什么，由 cmd/parcel-dispatch 的用例证。
func TestTheWiredMovementFactEnqueuesItsRecordedEnvelopeWithTheFact(t *testing.T) {
	line := newDepartureLine(t)
	line.handOver("SYN-SEG-SHA-SZX-HANDOFF", "SYN-PARCEL-HANDOFF-01")
	line.depart("SYN-MOVE-HANDOFF-01", "SYN-SEG-SHA-SZX-HANDOFF")

	if n := recordedMovementEnvelopes(t, line, "SYN-MOVE-HANDOFF-01"); n != 1 {
		t.Fatalf("出发登记后信封 %d 封, want 1——事实与信封不在同一笔", n)
	}

	replay, err := line.movement.Record(t.Context(), line.departureCommand("SYN-MOVE-HANDOFF-01", "SYN-SEG-SHA-SZX-HANDOFF"))
	if err != nil || replay.Outcome() != tfapp.MovementFactVersionExists {
		t.Fatalf("重投：outcome=%q err=%v", replay.Outcome(), err)
	}
	if n := recordedMovementEnvelopes(t, line, "SYN-MOVE-HANDOFF-01"); n != 1 {
		t.Fatalf("重投后信封 %d 封, want 1", n)
	}
}

func recordedMovementEnvelopes(t *testing.T, line *departureLine, fact string) int {
	t.Helper()
	var count int
	if err := line.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1 AND subject = $2`,
		tfpostgres.MovementFactRecordedEventType, fact).Scan(&count); err != nil {
		t.Fatalf("数信封：%v", err)
	}
	return count
}
