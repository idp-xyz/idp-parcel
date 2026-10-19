package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证准备编排的受理门、各口未决与撞键读回（ADR-0201 决定七）。五格结论与重放、冲突在真库上证（postgres 包的准备用例），
// 这里只用替身补真库难造的几格。夹具全为合成值（S 级）。

var preparedAt = time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC)

type demandDouble struct{ err error }

func (double demandDouble) LoadDeclaredDemand(context.Context, domain.TenantID, domain.CarriedObjectReference) (domain.DemandSnapshot, error) {
	if double.err != nil {
		return domain.DemandSnapshot{}, double.err
	}
	return domain.DeclaredDemand(domain.DeclaredDemandSpec{WeightValue: "1", WeightUnit: "KG"})
}

type poolsOnSchedulesDouble struct{ err error }

func (double poolsOnSchedulesDouble) ListPoolsOnSchedules(context.Context, domain.TenantID, []domain.ScheduleReference) ([]ports.CapacityPoolRecord, error) {
	return nil, double.err
}

// preparationStoreDouble 按身份存一份；raceWinner 在场时 Save 答已落过、读回交它——模拟另一方先落了同一身份。
type preparationStoreDouble struct {
	records    map[ports.TransportPreparationKey]ports.TransportPreparationRecord
	raceWinner *ports.TransportPreparationRecord
	findErr    error
}

func (double *preparationStoreDouble) FindByKey(_ context.Context, key ports.TransportPreparationKey) (ports.TransportPreparationRecord, bool, error) {
	if double.findErr != nil {
		return ports.TransportPreparationRecord{}, false, double.findErr
	}
	record, found := double.records[key]
	return record, found, nil
}

func (double *preparationStoreDouble) Save(_ context.Context, record ports.TransportPreparationRecord) (ports.TransportPreparationSaveOutcome, error) {
	if double.raceWinner != nil {
		double.records[record.Key] = *double.raceWinner
		return ports.TransportPreparationAlreadyRecorded, nil
	}
	double.records[record.Key] = record
	return ports.TransportPreparationSaved, nil
}

type clockDouble struct{}

func (clockDouble) Now() time.Time { return preparedAt }

func preparationHandler(demand demandDouble, schedules candidateSourceDouble, pools poolsOnSchedulesDouble, store *preparationStoreDouble) *application.PrepareOnPlannedLegsHandler {
	return application.NewPrepareOnPlannedLegsHandler(application.PrepareOnPlannedLegsDeps{
		Demand: demand, Schedules: schedules, Pools: pools, Preparations: store, Clock: clockDouble{},
	})
}

func plannedLeg(t *testing.T, reference string) domain.PreparationLeg {
	t.Helper()
	direction, err := domain.NewScheduleDirection("SYN-CONN-1", "SYN-NODE-A", "SYN-NODE-B")
	if err != nil {
		t.Fatalf("方向：%v", err)
	}
	segment, err := domain.NewPlannedSegmentReference(reference)
	if err != nil {
		t.Fatalf("段引用：%v", err)
	}
	leg, err := domain.NewPreparationLeg(segment, direction, preparedAt, preparedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("段：%v", err)
	}
	return leg
}

func formedCommand(t *testing.T, legs ...domain.PreparationLeg) application.PrepareOnPlannedLegsCommand {
	t.Helper()
	return application.PrepareOnPlannedLegsCommand{
		TenantID: opportunityTenant(t), Object: "SYN-PARCEL-01", Route: application.PlannedRouteFormed, Legs: legs,
	}
}

// Covers: 受理门——缺租户、缺对象、路由结论不明、计划已形成却一段都没有、同一段引用出现两次，都答未受理、零写入。
func TestPreparationRefusesCommandsItCannotPlaceOnALeg(t *testing.T) {
	leg := plannedLeg(t, "RPV-1#1")
	blankTenant := formedCommand(t, leg)
	blankTenant.TenantID = domain.TenantID{}
	blankObject := formedCommand(t, leg)
	blankObject.Object = " "
	unknownRoute := formedCommand(t, leg)
	unknownRoute.Route = application.PlannedRouteStateInvalid
	for name, command := range map[string]application.PrepareOnPlannedLegsCommand{
		"缺租户": blankTenant, "缺对象": blankObject, "路由结论不明": unknownRoute,
		"一段都没有": formedCommand(t), "同一段两次": formedCommand(t, leg, leg),
	} {
		store := &preparationStoreDouble{records: map[ports.TransportPreparationKey]ports.TransportPreparationRecord{}}
		result, err := preparationHandler(demandDouble{}, candidateSourceDouble{}, poolsOnSchedulesDouble{}, store).
			Prepare(context.Background(), command)
		if err != nil || result.Outcome() != application.PreparationRunNotAccepted || len(store.records) != 0 {
			t.Fatalf("%s = %s %v，写了 %d 份", name, result.Outcome(), err, len(store.records))
		}
	}
}

// Covers: X6——每一口读不动都答未决、点名那一口、带续办引用、零写入；空册不是读不动。
func TestPreparationNamesTheDependencyItIsWaitingFor(t *testing.T) {
	down := errors.New("down")
	for name, test := range map[string]struct {
		demand    demandDouble
		schedules candidateSourceDouble
		pools     poolsOnSchedulesDouble
		findErr   error
		want      application.PreparationUndecidedReason
	}{
		"申报读口":  {demand: demandDouble{err: down}, want: application.DeclaredDemandUnavailable},
		"班次册":   {schedules: candidateSourceDouble{err: down}, want: application.ScheduleCandidatesUnavailable},
		"准备结果库": {findErr: down, want: application.TransportPreparationStoreUnavailable},
	} {
		store := &preparationStoreDouble{records: map[ports.TransportPreparationKey]ports.TransportPreparationRecord{}, findErr: test.findErr}
		result, err := preparationHandler(test.demand, test.schedules, test.pools, store).
			Prepare(context.Background(), formedCommand(t, plannedLeg(t, "RPV-1#1")))
		if err != nil || result.Outcome() != application.PreparationRunUndecided || result.UndecidedReason() != test.want ||
			result.ContinuationReference() == "" || len(store.records) != 0 {
			t.Fatalf("%s = %s %s %q %v", name, result.Outcome(), result.UndecidedReason(), result.ContinuationReference(), err)
		}
	}
}

// Covers: AT-TF-133（并发）——另一方先落了同一身份：读回赢家按依据分，同依据答已有结果，不宣称自己形成了它。
func TestARacedPreparationReadsBackTheWinner(t *testing.T) {
	store := &preparationStoreDouble{records: map[ports.TransportPreparationKey]ports.TransportPreparationRecord{}}
	first, err := preparationHandler(demandDouble{}, candidateSourceDouble{}, poolsOnSchedulesDouble{}, store).
		Prepare(context.Background(), formedCommand(t, plannedLeg(t, "RPV-1#1")))
	if err != nil || first.Outcome() != application.PreparationRunAnswered {
		t.Fatalf("首判 = %s %v", first.Outcome(), err)
	}
	winner := first.Legs()[0].Record
	if winner.Preparation.Outcome() != domain.PreparationNotConfigured {
		t.Fatalf("空册 = %s", winner.Preparation.Outcome())
	}

	racing := &preparationStoreDouble{records: map[ports.TransportPreparationKey]ports.TransportPreparationRecord{}, raceWinner: &winner}
	raced, err := preparationHandler(demandDouble{}, candidateSourceDouble{}, poolsOnSchedulesDouble{}, racing).
		Prepare(context.Background(), formedCommand(t, plannedLeg(t, "RPV-1#1")))
	if err != nil || raced.Outcome() != application.PreparationRunAnswered || raced.Legs()[0].Answer != application.LegPreparationExisting {
		t.Fatalf("撞键 = %s %v", raced.Outcome(), err)
	}
}
