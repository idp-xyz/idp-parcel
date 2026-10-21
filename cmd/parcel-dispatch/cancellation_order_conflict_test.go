package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 经生产 wireDispatcher 证 ADR-0254 决定二第 3 格的两条消费链：取消已提交之后才到的边界
// 事实发生不晚于取消的业务时间，是事实冲突——冲突记进冲突册、消费门入账，不形成责任起点或非取消终局，已提交的取消
// 不被碰；重投推不动，所以不回滚、不重投到失败预算耗尽。已接受委托走 SYN-V0 那一套夹具（本包没有第二条能造出真已
// 接受行的路），取消决定直接经生产取消库落库——被测行为从「取消已提交」开始。

const carrierPickupJudgmentConsumerName = "parcel-shipment/judge-label-final-from-carrier-first-effective-pickup"

// Covers: `AT-PS-107`——节点收寄信封经生产路由投到 PS 采用消费者；取消的业务时间晚于收寄发生，收寄采用停在
// CANCELLATION_ORDER_CONFLICT：采用账无行、下游采用意图不入队，冲突册一行指名哪份取消对哪一代来源；消费门入账
// （inbox 有账、信封不带未决失败码），重拍不再投、冲突不翻倍。
func TestALateEarlierNodeIntakeIsConsumedAsARecordedConflict(t *testing.T) {
	fixture := acceptedSYNFixture(t)
	ctx := t.Context()

	seedStandingCancellation(t, fixture, nodeIntakeAssociation, time.Now().UTC())
	eventID := recordFormedNodeIntake(t, fixture)

	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if code := recordedFailureCode(t, fixture.db, eventID); code != "" {
		t.Fatalf("收寄信封失败码 = %q, want 空——冲突重投推不动，必须入账而不是回滚", code)
	}
	if n := fixture.countInbox(t, adoptNodeIntakeConsumerName, eventID); n != 1 {
		t.Fatalf("inbox 行数 = %d, want 1", n)
	}
	assertOneConflict(t, fixture, "NETWORK_INTAKE")
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.intake_adoption`); n != 0 {
		t.Fatalf("intake_adoption 行数 = %d, want 0——冲突不形成责任起点，也不按取消在先不采用", n)
	}
	if n := fixture.countOutboxOfType(t, networkIntakeRecordedType); n != 0 {
		t.Fatalf("发出了 %d 封 %s", n, networkIntakeRecordedType)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.parcel_cancellation WHERE kind = 'PARCEL_CANCELLED'`); n != 1 {
		t.Fatalf("已提交的取消变成 %d 行——冲突不得改写取消", n)
	}

	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("重拍：%v", err)
	}
	assertOneConflict(t, fixture, "NETWORK_INTAKE")
}

// Covers: `AT-PS-108`——TF 实际承运商首次有效收寄登记信封经生产路由投到面单渠道终局判断；收寄的业务发生时间早于
// 已提交取消的业务时间，判断答`不形成`并指名冲突：终局库无行，冲突册一行指名那一代收寄，消费门入账。
func TestALateEarlierCarrierPickupIsConsumedAsARecordedConflict(t *testing.T) {
	fixture := acceptedSYNFixture(t)
	ctx := t.Context()

	cancelledAt := time.Now().UTC()
	seedStandingCancellation(t, fixture, nodeIntakeAssociation, cancelledAt)
	eventID := recordFormedCarrierPickup(t, fixture, nodeIntakeAssociation, cancelledAt.Add(-time.Hour))

	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if code := recordedFailureCode(t, fixture.db, eventID); code != "" {
		t.Fatalf("收寄信封失败码 = %q, want 空——冲突是`不形成`，入账", code)
	}
	if n := fixture.countInbox(t, carrierPickupJudgmentConsumerName, eventID); n != 1 {
		t.Fatalf("inbox 行数 = %d, want 1", n)
	}
	assertOneConflict(t, fixture, "CARRIER_FIRST_EFFECTIVE_PICKUP")
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.final_outcome`); n != 0 {
		t.Fatalf("final_outcome 行数 = %d, want 0——冲突不形成非取消终局", n)
	}
}

func acceptedSYNFixture(t *testing.T) *synVerticalFixture {
	t.Helper()
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()
	fixture.submit(t, ctx)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}
	return fixture
}

func assertOneConflict(t *testing.T, fixture *synVerticalFixture, boundaryKind string) {
	t.Helper()
	register, err := pspostgres.NewCancellationOrderConflicts(fixture.db)
	if err != nil {
		t.Fatalf("构造冲突册：%v", err)
	}
	entries, err := register.ListByParcel(t.Context(), fixture.identity.TenantID(),
		mustPS(t, psdomain.NewDeclaredParcelID, nodeIntakeAssociation))
	if err != nil {
		t.Fatalf("按包裹查冲突：%v", err)
	}
	if len(entries) != 1 || entries[0].Boundary.Kind.String() != boundaryKind ||
		entries[0].Cancellation.String() != "SYN-PCXL-01" {
		t.Fatalf("冲突 = %+v, want 一行 %s 指名 SYN-PCXL-01", entries, boundaryKind)
	}
	if entries[0].Boundary.OccurredAt.After(entries[0].CancelledAt) {
		t.Fatal("冲突行的边界事实晚于取消——那是取消在先，不是冲突")
	}
}

// seedStandingCancellation 经生产取消库落一份已成立的取消，请求键取委托的来源请求键（取消编排的键形）。
func seedStandingCancellation(t *testing.T, fixture *synVerticalFixture, parcel string, requestedAt time.Time) {
	t.Helper()
	cancellation, err := psdomain.DecideParcelCancellation(psdomain.ParcelCancellationSpec{
		ID:          mustPS(t, psdomain.NewParcelCancellationID, "SYN-PCXL-01"),
		Parcel:      mustPS(t, psdomain.NewDeclaredParcelID, parcel),
		Requester:   mustPS(t, psdomain.NewCancellationRequesterReference, "SYN-CUSTOMER-01"),
		Authority:   mustPS(t, psdomain.NewCancellationAuthorityReference, "SYN-CANCEL-RULE/1"),
		Reason:      mustPS(t, psdomain.NewCancellationReasonReference, "SYN-CHANGED-MIND"),
		RequestedAt: requestedAt,
	}, psdomain.CurrentIntakeFact{})
	if err != nil {
		t.Fatalf("形成取消：%v", err)
	}
	store, err := pspostgres.NewParcelCancellations(fixture.db)
	if err != nil {
		t.Fatalf("构造取消库：%v", err)
	}
	var outcome psports.CancellationSaveOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = store.Save(txCtx, psports.CancellationRecord{
			Key: psports.CancellationRequestKey{
				TenantID:   fixture.identity.TenantID(),
				RequestKey: fixture.identity.RequestKey(),
				Parcel:     cancellation.Parcel(),
			},
			ContentDigest: "SYN-CANCEL-DIGEST-01",
			Kind:          psports.RecordParcelCancelled,
			Cancellation:  cancellation,
			DecidedAt:     requestedAt.Add(time.Second),
		})
		return saveErr
	})
	if outcome != psports.CancellationSaved {
		t.Fatalf("取消 outcome = %v", outcome)
	}
}

// recordFormedCarrierPickup 经 TF 领域构造门落一版已形成的首次有效收寄，并在同一事务经生产 handoff 交出登记信封，
// 交回信封 ID（同 recordFormedNodeIntake 的纪律：登记与意图同生共死）。
func recordFormedCarrierPickup(t *testing.T, fixture *synVerticalFixture, object string, occurredAt time.Time) string {
	t.Helper()
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	registry, err := tfpostgres.NewCarrierFirstEffectivePickups(fixture.db)
	if err != nil {
		t.Fatalf("构造收寄登记册：%v", err)
	}
	handoff, err := tfpostgres.NewOutboxCarrierFirstEffectivePickupHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造收寄交接：%v", err)
	}
	carrier, err := tfdomain.NewCarrierSubject(tfdomain.ExternalCarrierParty, "SYN-CARRIER-PARTY-01")
	if err != nil {
		t.Fatalf("承运主体：%v", err)
	}
	basis, err := tfdomain.NewCarrierPickupBasis(tfdomain.CarrierDirectPickupScan, "SYN-SCAN-01", "v1")
	if err != nil {
		t.Fatalf("依据：%v", err)
	}
	pickup, err := tfdomain.FormCarrierFirstEffectivePickup(tfdomain.CarrierFirstEffectivePickupSpec{
		TenantID:   mustPS(t, tfdomain.NewTenantID, fixture.identity.TenantID().String()),
		Object:     mustPS(t, tfdomain.NewCarriedObjectReference, object),
		Fact:       mustPS(t, tfdomain.NewCarrierFirstEffectivePickupReference, "SYN-CFEP-01"),
		Version:    mustPS(t, tfdomain.NewCarrierFirstEffectivePickupVersion, "SYN-CFEV-01"),
		Carrier:    carrier,
		OccurredAt: occurredAt,
		JudgedAt:   occurredAt.Add(time.Minute),
		Bases:      []tfdomain.CarrierPickupBasis{basis},
	})
	if err != nil {
		t.Fatalf("形成收寄：%v", err)
	}
	record := tfports.CarrierFirstEffectivePickupRecord{
		Key:        tfports.CarrierFirstEffectivePickupKey{TenantID: pickup.TenantID(), Fact: pickup.Fact(), Version: pickup.Version()},
		Pickup:     pickup,
		RecordedAt: occurredAt.Add(2 * time.Minute),
	}
	var outcome tfports.CarrierPickupSaveOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var saveErr error
		if outcome, saveErr = registry.Save(txCtx, record); saveErr != nil {
			return saveErr
		}
		return handoff.HandOffCarrierFirstEffectivePickup(txCtx, tfports.CarrierFirstEffectivePickupHandoffIntent{Record: record})
	})
	if outcome != tfports.CarrierPickupSaved {
		t.Fatalf("收寄 outcome = %v", outcome)
	}
	return pickup.TenantID().String() + "/SYN-CFEP-01/SYN-CFEV-01/carrier-first-effective-pickup"
}
