package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件在生产路由表上证实测变化让已有申报准备的包裹多出按实测判的那一份：重投不多落，作废使当前实测退回前一条时也不多落。
// 证据只记 S。

func TestAMeasurementChangeAddsAMeasuredPreparationBesideTheDeclaration(t *testing.T) {
	beat, db, store := wiredBeat(t)
	fixture := &synVerticalFixture{db: db, transactor: db.Transactor()}
	seedMeasuredReception(t, db)
	occurred := time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
	declarePreparation(t, db, occurred)
	if got := preparationIdentities(t, db); len(got) != 1 || got[0] != (preparationIdentity{source: "declaration"}) {
		t.Fatalf("申报之后 = %+v", got)
	}

	const firstSource = "SYN-MEASURE-1"
	recordDemoMeasurement(t, fixture, measurementRedeliveryUnit, firstSource, "2.50", occurred)
	noticeMeasurementChange(t, db, store, firstSource)
	dispatchMeasurementChange(t, beat, db, firstSource)
	want := []preparationIdentity{
		{source: "actual-measurement", measurement: firstSource},
		{source: "declaration"},
	}
	if got := preparationIdentities(t, db); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("实测变化之后 = %+v", got)
	}
	assertMeasuredDemandCopied(t, db, "2.50", "6000")

	changeEnvelope := outboxEventIDOfSubject(t, db, nopostgres.ActualMeasurementChangedEventType, firstSource)
	reissueUnderAnotherEnvelopeID(t, fixture, changeEnvelope, "measurement-preparation-redelivery")
	if published, err := beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("重投：published = %d err = %v", published, err)
	}
	if got := preparationIdentities(t, db); len(got) != 2 {
		t.Fatalf("重投多落了：%+v", got)
	}

	const laterSource = "SYN-MEASURE-2"
	const voidSource = "SYN-MEASURE-VOID"
	recordDemoMeasurement(t, fixture, measurementRedeliveryUnit, laterSource, "3.00", occurred.Add(time.Hour))
	voidMeasurement(t, fixture, voidSource, laterSource, occurred.Add(2*time.Hour))
	noticeMeasurementChange(t, db, store, voidSource)
	dispatchMeasurementChange(t, beat, db, voidSource)
	if got := preparationIdentities(t, db); len(got) != 2 || got[0].measurement != firstSource {
		t.Fatalf("作废退回前一条之后 = %+v，want 仍是申报与 %s", got, firstSource)
	}
}

type preparationIdentity struct {
	source      string
	measurement string
}

func declarePreparation(t *testing.T, db *bentopg.DB, occurred time.Time) {
	t.Helper()
	handler, err := prepareOnPlannedLegsHandler(db, systemClock{})
	if err != nil {
		t.Fatalf("准备编排：%v", err)
	}
	tenant, err := tfdomain.NewTenantID(demoTenant)
	if err != nil {
		t.Fatal(err)
	}
	direction, err := tfdomain.NewScheduleDirection("SYN-CONN-EMPTY", "SYN-NODE-A", "SYN-NODE-B")
	if err != nil {
		t.Fatal(err)
	}
	reference, err := tfdomain.NewPlannedSegmentReference("SYN-RPV-MC#1")
	if err != nil {
		t.Fatal(err)
	}
	leg, err := tfdomain.NewPreparationLeg(reference, direction, occurred.Add(-time.Hour), occurred.Add(6*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	mustWithinTX(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		result, err := handler.Prepare(ctx, tfapplication.PrepareOnPlannedLegsCommand{
			TenantID: tenant, Object: measurementRedeliveryParcel, Route: tfapplication.PlannedRouteFormed,
			Legs: []tfdomain.PreparationLeg{leg},
		})
		if err != nil {
			return err
		}
		if result.Outcome() != tfapplication.PreparationRunAnswered || len(result.Legs()) != 1 {
			return errors.New(result.Outcome().String() + "/" + result.UndecidedReason().String())
		}
		return nil
	})
}

func dispatchMeasurementChange(t *testing.T, beat Beat, db *bentopg.DB, source string) {
	t.Helper()
	if published, err := beat.DispatchOnce(t.Context()); err != nil || published != 1 {
		t.Fatalf("派发 %s：published = %d err = %v", source, published, err)
	}
	eventID := outboxEventIDOfSubject(t, db, nopostgres.ActualMeasurementChangedEventType, source)
	if status := outboxStatus(t, db, eventID); status != "PUBLISHED" {
		t.Fatalf("%s 信封 status = %q 失败码 = %q", source, status, recordedFailureCode(t, db, eventID))
	}
}

func preparationIdentities(t *testing.T, db *bentopg.DB) []preparationIdentity {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := querier.Query(t.Context(),
		`SELECT demand_source, measurement_ref FROM transport_fulfillment.transport_preparation
		  ORDER BY demand_source, measurement_ref`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []preparationIdentity
	for rows.Next() {
		var identity preparationIdentity
		if err := rows.Scan(&identity.source, &identity.measurement); err != nil {
			t.Fatal(err)
		}
		got = append(got, identity)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

func assertMeasuredDemandCopied(t *testing.T, db *bentopg.DB, weight, volume string) {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var demand string
	if err := querier.QueryRow(t.Context(),
		`SELECT demand::text FROM transport_fulfillment.transport_preparation WHERE demand_source = 'actual-measurement'`,
	).Scan(&demand); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(demand, weight) || !strings.Contains(demand, volume) ||
		!strings.Contains(demand, "KG") || !strings.Contains(demand, "CM3") {
		t.Fatalf("实测需求量 = %s，want 重量 %s KG、体积 %s CM3", demand, weight, volume)
	}
}

func voidMeasurement(t *testing.T, fixture *synVerticalFixture, sourceID, target string, occurred time.Time) {
	t.Helper()
	source, err := nodomain.NewWorkFactSource(
		sourceID,
		rpfRef(t, nodomain.NewPerformingPartyReference, "SYN-SCALE-RPF"),
		rpfRef(t, nodomain.NewExecutionEvidenceReference, "WEIGH/"+sourceID),
		occurred,
	)
	if err != nil {
		t.Fatalf("作废来源：%v", err)
	}
	invalidation, err := nodomain.NewMeasurementInvalidation(nodomain.MeasurementInvalidationSpec{
		Source: source, TargetSourceID: target, Reason: "称错件",
	})
	if err != nil {
		t.Fatalf("作废：%v", err)
	}
	measurements, err := nopostgres.NewActualMeasurements(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	record := noports.MeasurementInvalidationRecord{
		Key: noports.ActualMeasurementKey{
			TenantID: rpfRef(t, nodomain.NewTenantID, demoTenant), SourceID: sourceID,
		},
		Invalidation:  invalidation,
		ContentDigest: invalidation.ContentDigest(),
		RecordedAt:    occurred.Add(time.Minute),
	}
	mustWithinTX(t, fixture.transactor, t.Context(), func(ctx context.Context) error {
		outcome, err := measurements.SaveInvalidation(ctx, record)
		if err != nil {
			return err
		}
		if outcome != noports.MeasurementInvalidationSaved {
			return errors.New(outcome.String())
		}
		return nil
	})
}
