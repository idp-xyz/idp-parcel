package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodeopsdomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	nodeopsports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件证测量与作废共用一个来源身份空间：编排走生产装配（buildActualMeasurementOrchestration），登记册与 Outbox 是真实
// PostgreSQL。变化事件的标识与结算续办的依据都由来源身份定，撞号的作废会登记成功而重评被静默丢掉。测试输入是隔离合成，只记 `S`。

const sharedIdentityTenant = "SYN-TENANT-01"

type sharedIdentityFixture struct {
	pool         *pgxpool.Pool
	handler      nodeopshttp.ActualMeasurementHandler
	measurements *nopostgres.ActualMeasurements
	record       http.Handler
	invalidate   http.Handler
	tenant       nodeopsdomain.TenantID
}

func newSharedIdentityFixture(t *testing.T) *sharedIdentityFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	handler, err := buildActualMeasurementOrchestration(db)
	if err != nil {
		t.Fatalf("装配测量编排：%v", err)
	}
	measurements, err := nopostgres.NewActualMeasurements(db)
	if err != nil {
		t.Fatalf("构造测量登记册：%v", err)
	}
	intake, ok := workFactReceptionForTest(t).(nodeopshttp.MeasurementIntake)
	if !ok {
		t.Fatal("作业事实渠道的入口不是测量入口")
	}
	tenant, err := nodeopsdomain.NewTenantID(sharedIdentityTenant)
	if err != nil {
		t.Fatal(err)
	}
	return &sharedIdentityFixture{
		pool:         pool,
		handler:      handler,
		measurements: measurements,
		record:       nodeopshttp.NewRecordMeasurementEndpoint(intake, handler),
		invalidate:   nodeopshttp.NewInvalidateMeasurementEndpoint(intake, handler),
		tenant:       tenant,
	}
}

func (fixture *sharedIdentityFixture) post(t *testing.T, endpoint http.Handler, path, body string) (int, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	stampWorkFact(request, "NODE", "SYN-NODE-SHA-HUB", "SYN-DEVICE-NODE")
	endpoint.ServeHTTP(recorder, request)
	return recorder.Code, registrationOutcome(t, recorder)
}

func (fixture *sharedIdentityFixture) recordMeasurement(t *testing.T, sourceID, unit string) (int, string) {
	t.Helper()
	body := `{"sourceId":"` + sourceID + `","objectKind":"HANDLING_UNIT","objectId":"` + unit + `",` +
		`"location":"SYN-BENCH-01","basis":"SYN-PARCEL-09","performedBy":"SYN-SCALE-01","evidenceRef":"WEIGH/1",` +
		`"occurredAt":"2026-11-17T02:00:00Z","weightValue":"2.10","weightUnit":"KG",` +
		`"length":"20","width":"15","height":"10","dimensionUnit":"CM"}`
	return fixture.post(t, fixture.record, "/node-operations/measurements", body)
}

func (fixture *sharedIdentityFixture) recordInvalidation(t *testing.T, sourceID, target string) (int, string) {
	t.Helper()
	body := `{"sourceId":"` + sourceID + `","targetSourceId":"` + target + `","reason":"称错件",` +
		`"performedBy":"SYN-SCALE-01","evidenceRef":"WEIGH/2","occurredAt":"2026-11-17T03:00:00Z"}`
	return fixture.post(t, fixture.invalidate, "/node-operations/measurement-invalidations", body)
}

func (fixture *sharedIdentityFixture) mustRecordMeasurement(t *testing.T, sourceID, unit string) {
	t.Helper()
	if status, outcome := fixture.recordMeasurement(t, sourceID, unit); status != http.StatusCreated || outcome != "MEASUREMENT_RECORDED" {
		t.Fatalf("登记 %s = %d %s，want 201 MEASUREMENT_RECORDED", sourceID, status, outcome)
	}
}

func (fixture *sharedIdentityFixture) mustRecordInvalidation(t *testing.T, sourceID, target string) {
	t.Helper()
	if status, outcome := fixture.recordInvalidation(t, sourceID, target); status != http.StatusCreated || outcome != "INVALIDATION_RECORDED" {
		t.Fatalf("作废 %s = %d %s，want 201 INVALIDATION_RECORDED", sourceID, status, outcome)
	}
}

func (fixture *sharedIdentityFixture) stillValid(t *testing.T, unit string) map[string]bool {
	t.Helper()
	listed, err := fixture.measurements.ListStillValid(t.Context(), fixture.tenant, nodeopsdomain.HandlingUnitObject, unit)
	if err != nil {
		t.Fatalf("仍有效清单：%v", err)
	}
	ids := map[string]bool{}
	for _, record := range listed {
		ids[record.Key.SourceID] = true
	}
	return ids
}

func (fixture *sharedIdentityFixture) measurementRow(t *testing.T, sourceID string) bool {
	t.Helper()
	_, found, err := fixture.measurements.FindMeasurement(t.Context(), nodeopsports.ActualMeasurementKey{TenantID: fixture.tenant, SourceID: sourceID})
	if err != nil {
		t.Fatalf("读测量 %s：%v", sourceID, err)
	}
	return found
}

func (fixture *sharedIdentityFixture) invalidationRow(t *testing.T, sourceID string) bool {
	t.Helper()
	_, found, err := fixture.measurements.FindInvalidation(t.Context(), nodeopsports.ActualMeasurementKey{TenantID: fixture.tenant, SourceID: sourceID})
	if err != nil {
		t.Fatalf("读作废 %s：%v", sourceID, err)
	}
	return found
}

func (fixture *sharedIdentityFixture) changeEvents(t *testing.T) int {
	t.Helper()
	var count int
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1`,
		nopostgres.ActualMeasurementChangedEventType,
	).Scan(&count); err != nil {
		t.Fatalf("数变化事件：%v", err)
	}
	return count
}

// Covers: 作废的来源身份已经是另一条测量的来源身份：口上答 200 SOURCE_CONFLICT，不记作废、不发变化事件，被它指着的那条测量仍在
// 仍有效清单里。另起一个没人用过的来源身份照常作废。
func TestAnInvalidationCannotTakeTheSourceIdentityOfAnotherMeasurementAgainstARealDatabase(t *testing.T) {
	fixture := newSharedIdentityFixture(t)
	const unit = "SYN-HU-09-0001"
	fixture.mustRecordMeasurement(t, "SYN-DEVICE-09/WEIGH-A", unit)
	fixture.mustRecordMeasurement(t, "SYN-DEVICE-09/WEIGH-B", unit)
	before := fixture.changeEvents(t)

	if status, outcome := fixture.recordInvalidation(t, "SYN-DEVICE-09/WEIGH-A", "SYN-DEVICE-09/WEIGH-B"); status != http.StatusOK || outcome != "SOURCE_CONFLICT" {
		t.Fatalf("用 A 的来源身份作废 B = %d %s，want 200 SOURCE_CONFLICT", status, outcome)
	}
	if fixture.invalidationRow(t, "SYN-DEVICE-09/WEIGH-A") {
		t.Fatal("冲突的作废落了册")
	}
	if ids := fixture.stillValid(t, unit); !ids["SYN-DEVICE-09/WEIGH-A"] || !ids["SYN-DEVICE-09/WEIGH-B"] {
		t.Fatalf("仍有效 = %v，want A、B 都在", ids)
	}
	if after := fixture.changeEvents(t); after != before {
		t.Fatalf("变化事件 %d 封，want %d（冲突的作废不发）", after, before)
	}

	fixture.mustRecordInvalidation(t, "SYN-DEVICE-09/VOID-B", "SYN-DEVICE-09/WEIGH-B")
	if ids := fixture.stillValid(t, unit); !ids["SYN-DEVICE-09/WEIGH-A"] || ids["SYN-DEVICE-09/WEIGH-B"] {
		t.Fatalf("另起来源身份作废之后仍有效 = %v，want 只剩 A", ids)
	}
}

// Covers: 测量的来源身份已经是某条作废的来源身份：口上答 200 SOURCE_CONFLICT，不记测量、不发变化事件；同类原样重报仍答
// EXISTING_RESULT。
func TestAMeasurementCannotTakeTheSourceIdentityOfAnInvalidationAgainstARealDatabase(t *testing.T) {
	fixture := newSharedIdentityFixture(t)
	const unit = "SYN-HU-09-0002"
	fixture.mustRecordMeasurement(t, "SYN-DEVICE-09/WEIGH-C", unit)
	fixture.mustRecordInvalidation(t, "SYN-DEVICE-09/VOID-C", "SYN-DEVICE-09/WEIGH-C")
	before := fixture.changeEvents(t)

	if status, outcome := fixture.recordMeasurement(t, "SYN-DEVICE-09/VOID-C", unit); status != http.StatusOK || outcome != "SOURCE_CONFLICT" {
		t.Fatalf("用作废的来源身份登记测量 = %d %s，want 200 SOURCE_CONFLICT", status, outcome)
	}
	if fixture.measurementRow(t, "SYN-DEVICE-09/VOID-C") {
		t.Fatal("冲突的测量落了册")
	}
	if ids := fixture.stillValid(t, unit); len(ids) != 0 {
		t.Fatalf("仍有效 = %v，want 空（C 已作废，冲突的那条没记）", ids)
	}
	if after := fixture.changeEvents(t); after != before {
		t.Fatalf("变化事件 %d 封，want %d（冲突的测量不发）", after, before)
	}

	if status, outcome := fixture.recordMeasurement(t, "SYN-DEVICE-09/WEIGH-C", unit); status != http.StatusOK || outcome != "EXISTING_RESULT" {
		t.Fatalf("测量原样重报 = %d %s，want 200 EXISTING_RESULT", status, outcome)
	}
	if status, outcome := fixture.recordInvalidation(t, "SYN-DEVICE-09/VOID-C", "SYN-DEVICE-09/WEIGH-C"); status != http.StatusOK || outcome != "EXISTING_RESULT" {
		t.Fatalf("作废原样重报 = %d %s，want 200 EXISTING_RESULT", status, outcome)
	}
}

// Covers: 一条测量与一条作废带同一个来源身份并发登记，只成一个：另一个答 SOURCE_CONFLICT，册上这个来源身份只在一张表里有行。
// 靠的是登记与作废在读另一类之前先取同一把（租户，来源身份）的锁；每轮都用新的来源身份重来，免得一次侥幸。
func TestAMeasurementAndAnInvalidationRacingForOneSourceIdentityLeaveExactlyOne(t *testing.T) {
	fixture := newSharedIdentityFixture(t)
	const rounds = 12
	for round := 0; round < rounds; round++ {
		unit := fmt.Sprintf("SYN-HU-09-RACE-%02d", round)
		target := fmt.Sprintf("SYN-DEVICE-09/RACE-T-%02d", round)
		shared := fmt.Sprintf("SYN-DEVICE-09/RACE-S-%02d", round)
		fixture.mustRecordMeasurement(t, target, unit)

		var (
			start       = make(chan struct{})
			group       sync.WaitGroup
			measured    nodeopsapp.RecordActualMeasurementResult
			measuredErr error
			voided      nodeopsapp.InvalidateActualMeasurementResult
			voidedErr   error
			ctx         = t.Context()
		)
		group.Add(2)
		go func() {
			defer group.Done()
			<-start
			measured, measuredErr = fixture.handler.Handle(ctx, nodeopsapp.RecordActualMeasurementCommand{
				TenantID: sharedIdentityTenant, SourceID: shared, PerformedBy: "SYN-SCALE-01", Evidence: "WEIGH/1",
				OccurredAt: time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC),
				ObjectKind: "HANDLING_UNIT", ObjectID: unit + "-OTHER", Node: "SYN-NODE-SHA-HUB",
				Location: "SYN-BENCH-01", Basis: "SYN-PARCEL-09",
				WeightValue: "2.10", WeightUnit: "KG", Length: "20", Width: "15", Height: "10", DimensionUnit: "CM",
			})
		}()
		go func() {
			defer group.Done()
			<-start
			voided, voidedErr = fixture.handler.Invalidate(ctx, nodeopsapp.InvalidateActualMeasurementCommand{
				TenantID: sharedIdentityTenant, SourceID: shared, TargetSourceID: target, Reason: "称错件",
				PerformedBy: "SYN-SCALE-01", Evidence: "WEIGH/2",
				OccurredAt: time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC),
				Scope:      nodeopsapp.InvalidationAcrossTenantNodes(),
			})
		}()
		close(start)
		group.Wait()

		if measuredErr != nil || voidedErr != nil {
			t.Fatalf("第 %d 轮：登记 err = %v，作废 err = %v", round, measuredErr, voidedErr)
		}
		measurementWon := measured.Outcome() == nodeopsapp.ActualMeasurementRecorded
		invalidationWon := voided.Outcome() == nodeopsapp.MeasurementInvalidationRecorded
		if measurementWon == invalidationWon {
			t.Fatalf("第 %d 轮：登记 %s，作废 %s，want 恰好一个记下", round, measured.Outcome(), voided.Outcome())
		}
		if !measurementWon && measured.Outcome() != nodeopsapp.ActualMeasurementSourceConflict {
			t.Fatalf("第 %d 轮：输掉的登记 = %s，want SOURCE_CONFLICT", round, measured.Outcome())
		}
		if !invalidationWon && voided.Outcome() != nodeopsapp.MeasurementInvalidationSourceConflict {
			t.Fatalf("第 %d 轮：输掉的作废 = %s，want SOURCE_CONFLICT", round, voided.Outcome())
		}
		if fixture.measurementRow(t, shared) != measurementWon || fixture.invalidationRow(t, shared) != invalidationWon {
			t.Fatalf("第 %d 轮：册上与答复不符，测量行 %v，作废行 %v", round, fixture.measurementRow(t, shared), fixture.invalidationRow(t, shared))
		}
	}
}
