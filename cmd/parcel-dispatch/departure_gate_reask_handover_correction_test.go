package main

import (
	"testing"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 证票 demo-chain-gaps/130（ADR-0307）与出发门禁补问（票 121，ADR-0305）的交互，走生产 wireDispatcher，
// 只记 `S`。

// Covers: AT-TF-329 与 AT-TF-343 交互的真库半边（S）——出发先于段登记、首版读数停在本侧「段不在册」；在控那一件的交接首登为待确认、
// 声明进出发那一段：那一封交到补问那一路照常入账，不补问（交接不是`已交接`，对象没进段）。随后经生产登记编排更正成`已交接`，更正带
// 对象进那一段；更正版那一封经生产路由一拍，补问与全程追踪补派生两路各入账一次、都不报错：补问按原出发时点替这一件问真关务（它有
// 一条约束装载出发的限制），追加第 1 次补问、读数阻断，首版不改。票 130 之前更正不带对象进段，这次出发永远停在本侧、不再补问。
func TestAPendingHandoverCorrectedIntoTheSegmentReasksTheStoppedDeparture(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	saveReaskSchedule(t, fixture)
	restrictLoadingDepartureOf(t, fixture, movementParcelOnBoard)

	factEvent := recordStoppedDepartureThroughProduction(t, fixture)
	if published, err := fixture.beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("出发那一拍 published = %d err = %v；失败码 = %q", published, err, recordedFailureCode(t, fixture.db, factEvent))
	}

	register := registerPendingHandoverIntoMovementSegment(t, fixture)
	pendingEvent := fixture.identity.TenantID().String() + "/" + movementParcelOnBoard + "/" + backfillScope + "/" + backfillVersion
	if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("待确认那一拍：%v", err)
	}
	if n := fixture.countInbox(t, departureGateReaskOnHandoverConsumerName, pendingEvent); n != 1 {
		t.Fatalf("待确认那一封在补问那一路 inbox 行数 = %d, want 1；失败码 = %q", n, recordedFailureCode(t, fixture.db, pendingEvent))
	}
	if n := len(departureGateReasksOf(t, fixture)); n != 0 {
		t.Fatalf("待确认那一封补问了 %d 次, want 0", n)
	}

	confirmedEvent := confirmPendingHandoverIntoMovementSegment(t, fixture, register)
	if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("更正版那一拍：%v", err)
	}
	for _, consumer := range []string{departureGateReaskOnHandoverConsumerName, backfillConsumerName} {
		if n := fixture.countInbox(t, consumer, confirmedEvent); n != 1 {
			t.Fatalf("%s inbox 行数 = %d, want 1；失败码 = %q", consumer, n, recordedFailureCode(t, fixture.db, confirmedEvent))
		}
	}
	if status := outboxStatus(t, fixture.db, confirmedEvent); status != "PUBLISHED" {
		t.Fatalf("更正版信封 status = %s，失败码 = %q", status, recordedFailureCode(t, fixture.db, confirmedEvent))
	}
	reasks := departureGateReasksOf(t, fixture)
	if len(reasks) != 1 || reasks[0].Key.Object.String() != movementParcelOnBoard || reasks[0].Key.Sequence != 1 ||
		reasks[0].Reading.Answer() != tfdomain.DepartureGateBlocked {
		t.Fatalf("补问 = %+v, want 这一件第 1 次补问、读数阻断", reasks)
	}
	if first := departureFirstReading(t, fixture); first.Answer() != tfdomain.DepartureGateUndecided ||
		first.Stops()[0] != tfdomain.DepartureSegmentNotRegistered {
		t.Fatalf("首版读数被改成了 %s %v", first.Answer(), first.Stops())
	}
}
