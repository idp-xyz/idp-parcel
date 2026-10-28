package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

const postIntakeParcel = "SYN-PARCEL-PID-01"

// handOffPostIntakeDisposition 用生产交接口把一份处置决定入队（与决定追加同笔的那一半；这里只钉信封到消费者这一段）。
func handOffPostIntakeDisposition(
	t *testing.T,
	fixture *synVerticalFixture,
	parcel string,
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
		Parcel:              mustPS(t, psdomain.NewDeclaredParcelID, parcel),
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

// postIntakeProcessing 取生产装配的处理方，供直接调 Handle 的用例；事务由用例自己开（消费门里它在 inbox 事务内跑）。
func (fixture *synVerticalFixture) postIntakeProcessing(t *testing.T) interface {
	HandleDecidedPostIntakeDisposition(context.Context, tfinbox.DecidedPostIntakeDisposition) error
} {
	t.Helper()
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	processing, err := startJourneyOnPostIntakeDispositionProcessing(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	return processing
}

func tfOnlyDecision(tenant, parcel string, version int, action string, at time.Time) tfinbox.DecidedPostIntakeDisposition {
	return tfinbox.DecidedPostIntakeDisposition{
		TenantID: tenant, Parcel: parcel, DecisionVersion: version, Direction: "RETURN",
		BasisKind:      "SERVICE_DISPOSITION",
		BasisReference: "POST_INTAKE_DISPOSITION/" + parcel + "/" + string(rune('0'+version)),
		DecidedAt:      at,
		Requests:       []tfinbox.PostIntakeDispositionOwnerRequest{{Owner: "TRANSPORT_FULFILLMENT", Action: action}},
	}
}

// Covers: AT-PS-083 的运输履约一半（其余所有者的承接未接）；ADR-0332 决定四、请求表；UC-TF-007——处理方直接调两次是幂等的：同一份只开一条旅程、
// 只交一份异常链意图；下一版的原旅程取前一条旅程；原旅程不把自己当原旅程。
func TestTheJourneyStartOnAPostIntakeDecisionIsIdempotentAndChainsOriginals(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	processing := fixture.postIntakeProcessing(t)
	tenant := fixture.identity.TenantID().String()
	base := time.Now().UTC().Add(-time.Hour)
	handle := func(decided tfinbox.DecidedPostIntakeDisposition) {
		t.Helper()
		mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
			return processing.HandleDecidedPostIntakeDisposition(txCtx, decided)
		})
	}

	first := tfOnlyDecision(tenant, postIntakeParcel, 1, "START_RETURN_JOURNEY", base)
	handle(first)
	handle(first)
	if n := fixture.countSQL(t, `SELECT count(*) FROM transport_fulfillment.alternate_journey`); n != 1 {
		t.Fatalf("同一份决定调两次之后旅程 %d 条, want 1", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM bento.outbox WHERE event_type = 'transport-fulfillment.exception-journey.recorded'`); n != 1 {
		t.Fatalf("异常链意图 %d 份, want 1", n)
	}
	original, purpose, basisKind := fixture.journeyOf(t, "JRN/"+postIntakeParcel+"/PID-1")
	if original != "JRN/"+postIntakeParcel || purpose != "RETURN" || basisKind != "SERVICE_DISPOSITION" {
		t.Fatalf("退运旅程 = 原 %q 目的 %q 依据种类 %q", original, purpose, basisKind)
	}

	handle(tfOnlyDecision(tenant, postIntakeParcel, 2, "START_ALTERNATE_JOURNEY", base.Add(time.Minute)))
	original, purpose, _ = fixture.journeyOf(t, "JRN/"+postIntakeParcel+"/PID-2")
	if original != "JRN/"+postIntakeParcel+"/PID-1" || purpose != "ALTERNATE" {
		t.Fatalf("改送旅程 = 原 %q 目的 %q, want 原为退运旅程、目的 ALTERNATE", original, purpose)
	}
}

// Covers: ADR-0332 决定四；评审修订——信封里有今天没人承接的请求项（NO、NR 各项，TF 的扣住）时整封答未决、不开旅程，信封留在可见的失败状态，不被标成已发布；
// `继续` 不请求任何所有者，照常入账。
func TestAPostIntakeDecisionWithAnUnhandledRequestStaysVisiblyFailed(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	base := time.Now().UTC().Add(-time.Hour)

	directions := []struct {
		parcel    string
		direction psdomain.PostIntakeDispositionDirection
		dest      string
	}{
		{"SYN-PARCEL-PID-RET", psdomain.PostIntakeReturn, ""},
		{"SYN-PARCEL-PID-RED", psdomain.PostIntakeRedirect, "SYN-DEST-02"},
		{"SYN-PARCEL-PID-INT", psdomain.PostIntakeIntercept, ""},
		{"SYN-PARCEL-PID-TER", psdomain.PostIntakeTerminateService, ""},
	}
	for _, d := range directions {
		handOffPostIntakeDisposition(t, fixture, d.parcel, 1, d.direction, d.dest, base)
	}
	handOffPostIntakeDisposition(t, fixture, "SYN-PARCEL-PID-CON", 1, psdomain.PostIntakeContinue, "", base)
	fixture.dispatchUntilQuiet(t)

	const eventType = "parcel-shipment.post-intake-disposition.decided"
	if n := fixture.countSQL(t, `SELECT count(*) FROM bento.outbox WHERE event_type = '`+eventType+`' AND failure_code = 'dispatch.consumer_undecided'`); n != len(directions) {
		t.Fatalf("可见失败的处置信封 %d 封, want %d——四个有未承接请求的去向各一封", n, len(directions))
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM bento.outbox WHERE event_type = '`+eventType+`' AND failure_code IS NULL`); n != 1 {
		t.Fatalf("无失败码的处置信封 %d 封, want 1——只有继续", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM transport_fulfillment.alternate_journey`); n != 0 {
		t.Fatalf("旅程 %d 条, want 0——整封未决不开旅程", n)
	}
}
