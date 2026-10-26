package main

import (
	"testing"
	"time"
)

// 本文件对真实 PostgreSQL 16 经生产 wireDispatcher 证有效交付那条终局链读已成立的取消：包裹已有取消在册，迟到的有效交付不形成
// 非取消终局。已接受委托走 SYN-V0 那一套夹具，取消决定直接经生产取消库落库（同 cancellation_order_conflict_test.go），被测行为
// 从「取消已提交」开始。终局规则在这里一律不种：不采用先于规则判断，种了反而遮住「规则未配置也停不住它」。
//
// 链上的接线靠这两条拦：边界锁没装，编排停在 FINAL_STORE_UNAVAILABLE、交付信封回滚重投，第一条红；取消视图没读，交付落到规则
// 未配置的停点，同样红。

const finalNotAdoptedBasisOfSYNCancellation = "PARCEL_CANCELLATION/SYN-PCXL-01"

// Covers: `AT-PS-080` 在有效交付一路上的对偶——取消已成立，TF 有效交付登记信封经生产路由投到 PS 终局消费者：不采用，依据指名那份
// 取消；终局库只有一份不采用记录（来源保全），没有当前有效终局；消费门入账（不采用是业务终局的负向，重投推不动，不回滚）。
func TestAnEffectiveDeliveryOverAStandingCancellationIsConsumedAsARefusal(t *testing.T) {
	fixture := acceptedSYNFixture(t)
	ctx := t.Context()

	seedStandingCancellation(t, fixture, effectiveDeliveryObject, time.Now().UTC().Add(-time.Hour))
	eventID := recordRegisteredEffectiveDelivery(t, fixture)

	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if code := recordedFailureCode(t, fixture.db, eventID); code != "" {
		t.Fatalf("交付信封失败码 = %q, want 空——取消在册，交付不采用并入账；停在未决说明锁没装或取消没读", code)
	}
	if n := fixture.countInbox(t, formFinalFromDeliveryConsumerName, eventID); n != 1 {
		t.Fatalf("inbox 行数 = %d, want 1", n)
	}
	assertOnlyARefusalOnTheCancelledParcel(t, fixture)

	// 重拍：不采用已入账，不重投、不长出第二行。
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("重拍：%v", err)
	}
	assertOnlyARefusalOnTheCancelledParcel(t, fixture)
}

// Covers: `AT-PS-107`（ADR-0254 决定二第 3 格）——包裹先停在取消先后冲突上（节点收寄早于已提交取消的业务时间，冲突记进册，没有
// 责任起点），此后到的有效交付经生产链同样不采用：不绕过冲突形成终局，冲突册那一行与已提交的取消原样不动。
func TestAnEffectiveDeliveryDoesNotGetPastAStandingCancellationOrderConflict(t *testing.T) {
	fixture := acceptedSYNFixture(t)
	ctx := t.Context()

	seedStandingCancellation(t, fixture, nodeIntakeAssociation, time.Now().UTC())
	intakeID := recordFormedNodeIntake(t, fixture)
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("收寄那一拍：%v", err)
	}
	if code := recordedFailureCode(t, fixture.db, intakeID); code != "" {
		t.Fatalf("收寄信封失败码 = %q, want 空；包裹要先停在冲突上", code)
	}
	assertOneConflict(t, fixture, "NETWORK_INTAKE")

	deliveryID := recordRegisteredEffectiveDelivery(t, fixture)
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("交付那一拍：%v", err)
	}
	if code := recordedFailureCode(t, fixture.db, deliveryID); code != "" {
		t.Fatalf("交付信封失败码 = %q, want 空——停在冲突上的包裹，交付不采用并入账", code)
	}
	assertOnlyARefusalOnTheCancelledParcel(t, fixture)
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.intake_adoption`); n != 0 {
		t.Fatalf("intake_adoption 行数 = %d, want 0——冲突不形成责任起点", n)
	}
	assertOneConflict(t, fixture, "NETWORK_INTAKE")
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.parcel_cancellation WHERE kind = 'PARCEL_CANCELLED'`); n != 1 {
		t.Fatalf("已提交的取消变成 %d 行——交付不得改写取消", n)
	}
}

// assertOnlyARefusalOnTheCancelledParcel 钉住终局库此刻的样子：恰一行，是不采用，依据指名那份取消，没有任何已采用终局。
func assertOnlyARefusalOnTheCancelledParcel(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.final_outcome`); n != 1 {
		t.Fatalf("final_outcome 行数 = %d, want 1（一份不采用记录保全来源）", n)
	}
	if n := fixture.countSQL(t,
		`SELECT count(*) FROM parcel_shipment.final_outcome WHERE refusal_basis = '`+finalNotAdoptedBasisOfSYNCancellation+`'`); n != 1 {
		t.Fatalf("依据指名 %s 的不采用记录 = %d 行, want 1", finalNotAdoptedBasisOfSYNCancellation, n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.final_outcome WHERE refusal_basis IS NULL`); n != 0 {
		t.Fatalf("已采用终局 %d 行——取消在册的包裹带上了非取消终局", n)
	}
}
