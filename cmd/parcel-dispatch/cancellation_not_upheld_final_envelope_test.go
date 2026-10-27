package main

import (
	"testing"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

const finalOutcomeFormedEventType = "parcel-shipment.final-outcome.formed"

// Covers: AT-PS-173；ADR-0295 决定三、四——认定取消依据不成立并续办之后，重新形成的终局自己交出一封 `parcel-shipment.final-outcome.formed` 信封。
// 作废之前，依据这份取消的不采用行已经交过一封同名信封并已投递；信封 ID 若只由采用键认领，终局与不采用行同键，入队按（来源，事件标识）去重，
// 重新形成的终局就没有信封，结算侧的有效交付终局计价时点与异常侧的终局投影都停在「不采用」。作废代次折进该包裹终局信封的交接身份，两封各有各的 ID。
func TestARedrivenFinalHandsOffItsOwnFormedEnvelopeAfterTheRefusalEnvelopeWasDelivered(t *testing.T) {
	fixture := acceptedSYNFixture(t)
	ctx := t.Context()
	seedStandingCancellation(t, fixture, nodeIntakeAssociation, time.Now().UTC())
	intakeID := recordFormedNodeIntake(t, fixture)
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("收寄那一拍：%v", err)
	}
	if code := recordedFailureCode(t, fixture.db, intakeID); code != "" {
		t.Fatalf("收寄信封失败码 = %q", code)
	}
	deliveryID := recordRegisteredEffectiveDelivery(t, fixture)
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("交付那一拍：%v", err)
	}
	if code := recordedFailureCode(t, fixture.db, deliveryID); code != "" {
		t.Fatalf("交付信封失败码 = %q, want 空——取消在册，交付不采用并入账", code)
	}
	formedBefore := fixture.countSQL(t, `SELECT count(*) FROM bento.outbox WHERE event_type = '`+finalOutcomeFormedEventType+`'`)
	if formedBefore != 1 {
		t.Fatalf("不采用之后 formed 信封 %d 封, want 1——不采用行也交一封", formedBefore)
	}
	// 让这封不采用信封被投递出去（下游已消费），之后的决定与续办面对的是已投递的旧信封。
	for i := 0; i < 3; i++ {
		if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
			t.Fatalf("再拍：%v", err)
		}
	}

	rig := newRedriveRig(t, fixture)
	rig.decideTheOnlyConflict(t, psdomain.CancellationNotUpheld)
	if err := rig.redrive(t); err != nil {
		t.Fatalf("续办：%v", err)
	}
	if n := fixture.countFinalizedOfParcel(t); n != 1 {
		t.Fatalf("续办之后当前有效终局 %d 份, want 1", n)
	}
	formedAfter := fixture.countSQL(t, `SELECT count(*) FROM bento.outbox WHERE event_type = '`+finalOutcomeFormedEventType+`'`)
	if formedAfter != formedBefore+1 {
		t.Fatalf("续办之后 formed 信封 %d 封, want %d——重新形成的终局要有自己的信封，不能被不采用行那一封的去重吞掉", formedAfter, formedBefore+1)
	}
	if distinct := fixture.countSQL(t, `SELECT count(DISTINCT event_id) FROM bento.outbox WHERE event_type = '`+finalOutcomeFormedEventType+`'`); distinct != formedAfter {
		t.Fatalf("formed 信封 ID 去重后 %d 个, want %d", distinct, formedAfter)
	}
	if partitions := fixture.countSQL(t, `SELECT count(DISTINCT partition_key) FROM bento.outbox WHERE event_type = '`+finalOutcomeFormedEventType+`'`); partitions != 1 {
		t.Fatalf("formed 信封分区键 %d 个, want 1——同一包裹的先后两封排在一条队里", partitions)
	}
}
