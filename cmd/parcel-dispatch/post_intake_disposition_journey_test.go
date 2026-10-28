package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

const postIntakeParcel = "SYN-PARCEL-PID-01"

// handOffPostIntakeDisposition 用生产交接口把一份处置决定入队（与决定追加同笔的那一半；这里只钉信封到消费者这一段）。
func handOffPostIntakeDisposition(
	t *testing.T,
	fixture *synVerticalFixture,
	version int,
	direction psdomain.PostIntakeDispositionDirection,
	destination string,
	decidedAt time.Time,
) {
	t.Helper()
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := pspostgres.NewOutboxPostIntakeDispositionHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	spec := psdomain.PostIntakeDispositionSpec{
		Tenant:              fixture.identity.TenantID(),
		Parcel:              mustPS(t, psdomain.NewDeclaredParcelID, postIntakeParcel),
		Version:             version,
		Supersedes:          version - 1,
		Direction:           direction,
		Source:              psdomain.PostIntakeOperatorInitiated,
		ResponsibilityStart: mustPS(t, psdomain.NewSourceResultVersion, "SYN-INTAKE-RESULT/v1"),
		Decider:             mustPS(t, psdomain.NewPostIntakeDispositionDeciderReference, "https://id.syn.example#operator"),
		Basis:               mustPS(t, psdomain.NewPostIntakeDispositionBasis, "SYN-BASIS"),
		DecidedAt:           decidedAt,
	}
	if destination != "" {
		spec.Destination = mustPS(t, psdomain.NewPostIntakeDispositionDestination, destination)
	}
	disposition, err := psdomain.DecidePostIntakeDisposition(spec)
	if err != nil {
		t.Fatal(err)
	}
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		return handoff.HandOffPostIntakeDisposition(txCtx, psports.PostIntakeDispositionHandoffIntent{Disposition: disposition})
	})
}

func (fixture *synVerticalFixture) dispatchUntilQuiet(t *testing.T) {
	t.Helper()
	for i := 0; i < 4; i++ {
		if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
			t.Fatalf("派发一拍：%v", err)
		}
	}
}

func (fixture *synVerticalFixture) journeyOf(t *testing.T, journey string) (original, purpose, basisKind string) {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := querier.QueryRow(t.Context(),
		`SELECT original_journey, purpose, basis_kind FROM transport_fulfillment.alternate_journey WHERE journey_id = $1`,
		journey).Scan(&original, &purpose, &basisKind); err != nil {
		t.Fatalf("读旅程 %s：%v", journey, err)
	}
	return original, purpose, basisKind
}

// Covers: AT-PS-083 的运输履约一半（其余所有者的承接未接）；ADR-0332 决定四、请求表；UC-TF-007——PS 的退运决定经信封在派发进程里落成一条运输履约退运旅程，依据指名链上这一版；
// 重投不开第二条；链上下一版改送指向前一条旅程为原旅程；拦截对运输履约的请求（不再安排向前移动）没有对应模型，入账、不开旅程。
func TestAPostIntakeReturnDecisionStartsATransportFulfillmentReturnJourney(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	base := time.Now().UTC().Add(-time.Hour)

	handOffPostIntakeDisposition(t, fixture, 1, psdomain.PostIntakeReturn, "", base)
	fixture.dispatchUntilQuiet(t)

	original, purpose, basisKind := fixture.journeyOf(t, "JRN/"+postIntakeParcel+"/PID-1")
	if original != "JRN/"+postIntakeParcel || purpose != "RETURN" || basisKind != "SERVICE_DISPOSITION" {
		t.Fatalf("退运旅程 = 原 %q 目的 %q 依据种类 %q", original, purpose, basisKind)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM transport_fulfillment.alternate_journey WHERE basis = 'POST_INTAKE_DISPOSITION/`+postIntakeParcel+`/1'`); n != 1 {
		t.Fatalf("依据指名第 1 版的旅程 %d 条, want 1", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM bento.outbox WHERE event_type = 'transport-fulfillment.exception-journey.recorded'`); n != 1 {
		t.Fatalf("异常链意图 %d 份, want 1——非监管来路只交异常链", n)
	}

	// 重投：把同一封信的 inbox 账之外的路径再走一遍——同一个版本再入队一次是同一个信封 ID，不会再有第二条旅程。
	handOffPostIntakeDisposition(t, fixture, 1, psdomain.PostIntakeReturn, "", base)
	fixture.dispatchUntilQuiet(t)
	if n := fixture.countSQL(t, `SELECT count(*) FROM transport_fulfillment.alternate_journey`); n != 1 {
		t.Fatalf("重投之后旅程 %d 条, want 1", n)
	}

	// 下一版：改送，原旅程取退运旅程（决定那一刻对象走的那一趟）。
	handOffPostIntakeDisposition(t, fixture, 2, psdomain.PostIntakeRedirect, "SYN-DEST-02", base.Add(time.Minute))
	fixture.dispatchUntilQuiet(t)
	original, purpose, _ = fixture.journeyOf(t, "JRN/"+postIntakeParcel+"/PID-2")
	if original != "JRN/"+postIntakeParcel+"/PID-1" || purpose != "ALTERNATE" {
		t.Fatalf("改送旅程 = 原 %q 目的 %q, want 原为退运旅程、目的 ALTERNATE", original, purpose)
	}

	// 拦截：运输履约只被请求「不再安排向前移动」，不开旅程，信封照常入账。
	handOffPostIntakeDisposition(t, fixture, 3, psdomain.PostIntakeIntercept, "", base.Add(2*time.Minute))
	fixture.dispatchUntilQuiet(t)
	if n := fixture.countSQL(t, `SELECT count(*) FROM transport_fulfillment.alternate_journey`); n != 2 {
		t.Fatalf("拦截之后旅程 %d 条, want 2", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM bento.outbox WHERE event_type = 'parcel-shipment.post-intake-disposition.decided' AND failure_code IS NOT NULL`); n != 0 {
		t.Fatalf("处置信封失败 %d 封, want 0", n)
	}
}
