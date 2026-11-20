package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

func TestCurrentParcelMeasurementWithoutAHandlingUnit(t *testing.T) {
	result, err := askCurrent(t, &currentParcelPorts{}, time.Time{})
	assertCurrent(t, result, err, application.NoCurrentParcelMeasurement, application.CurrentParcelMeasurementCauseInvalid, "")
}

func TestCurrentParcelMeasurementWithNoStillValidMeasurement(t *testing.T) {
	result, err := askCurrent(t, &currentParcelPorts{units: []string{"unit-1"}}, time.Time{})
	assertCurrent(t, result, err, application.NoCurrentParcelMeasurement, application.CurrentParcelMeasurementCauseInvalid, "")
}

func TestCurrentParcelMeasurementReturnsTheOnlyMeasurement(t *testing.T) {
	occurred := time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC)
	ports := &currentParcelPorts{
		units:        []string{"unit-1"},
		measurements: []domain.ActualMeasurement{weighedMeasurement(t, "measure-1", "2.10", occurred)},
	}
	result, err := askCurrent(t, ports, time.Time{})
	assertCurrent(t, result, err, application.OneCurrentParcelMeasurement, application.CurrentParcelMeasurementCauseInvalid, "measure-1")
	got := result.Measurement()
	if got.Weight().Value() != "2.10" || got.Weight().Unit() != "KG" || got.Dimensions().Unit() != "CM" || !got.Source().OccurredAt().Equal(occurred) {
		t.Fatalf("实测本体 = %s %s %s %s", got.Weight().Value(), got.Weight().Unit(), got.Dimensions().Unit(), got.Source().OccurredAt())
	}
	if ports.askedKind != domain.HandlingUnitObject || ports.askedObject != "unit-1" {
		t.Fatalf("问的是 %s %q，want 那一件作业实物", ports.askedKind, ports.askedObject)
	}
}

func TestAParcelWithSeveralHandlingUnitsIsUndetermined(t *testing.T) {
	// 多件实物在问测量与采用之前就定不下。这两只口即便报错，也不把这一格换成那份错误或其中一件的实测。
	ports := &currentParcelPorts{
		units:           []string{"unit-1", "unit-2"},
		measurements:    []domain.ActualMeasurement{weighedMeasurement(t, "measure-1", "2.10", time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC))},
		adopted:         true,
		measurementsErr: errors.New("measurements down"),
		adoptionErr:     errors.New("adoption down"),
	}
	result, err := askCurrent(t, ports, time.Time{})
	assertCurrent(t, result, err, application.UndeterminedCurrentParcelMeasurement, application.SeveralAssociatedHandlingUnits, "")
	asOf, err := askCurrent(t, ports, time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC))
	assertCurrent(t, asOf, err, application.UndeterminedCurrentParcelMeasurement, application.SeveralAssociatedHandlingUnits, "")
}

func TestSeveralMeasurementsWithoutAdoptionAreUndetermined(t *testing.T) {
	ports := &currentParcelPorts{
		units: []string{"unit-1"},
		measurements: []domain.ActualMeasurement{
			weighedMeasurement(t, "measure-1", "2.10", time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC)),
			weighedMeasurement(t, "measure-2", "2.40", time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC)),
		},
	}
	result, err := askCurrent(t, ports, time.Time{})
	assertCurrent(t, result, err, application.UndeterminedCurrentParcelMeasurement, application.SeveralStillValidWithoutAdoption, "")
}

func TestAnAdoptedFormTakesTheLatestMeasurement(t *testing.T) {
	ports := &currentParcelPorts{
		units: []string{"unit-1"},
		measurements: []domain.ActualMeasurement{
			weighedMeasurement(t, "measure-1", "2.10", time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC)),
			weighedMeasurement(t, "measure-2", "2.40", time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC)),
		},
		adopted: true,
	}
	result, err := askCurrent(t, ports, time.Time{})
	assertCurrent(t, result, err, application.OneCurrentParcelMeasurement, application.CurrentParcelMeasurementCauseInvalid, "measure-2")
	if result.Measurement().Weight().Value() != "2.40" {
		t.Fatalf("取到的毛重 = %s", result.Measurement().Weight().Value())
	}
}

func TestTiedOccurredAtStaysUndetermined(t *testing.T) {
	occurred := time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC)
	ports := &currentParcelPorts{
		units: []string{"unit-1"},
		measurements: []domain.ActualMeasurement{
			weighedMeasurement(t, "measure-1", "2.10", occurred),
			weighedMeasurement(t, "measure-2", "2.40", occurred),
		},
		adopted: true,
	}
	result, err := askCurrent(t, ports, time.Time{})
	assertCurrent(t, result, err, application.UndeterminedCurrentParcelMeasurement, application.OccurredAtTied, "")
}

func TestCurrentAsOfDropsMeasurementsThatOccurredLater(t *testing.T) {
	earlier := time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC)
	later := time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC)
	ports := twoWeighings(t, earlier, later, false)
	unbounded, err := askCurrent(t, ports, time.Time{})
	assertCurrent(t, unbounded, err, application.UndeterminedCurrentParcelMeasurement, application.SeveralStillValidWithoutAdoption, "")

	result, err := askCurrent(t, ports, earlier)
	assertCurrent(t, result, err, application.OneCurrentParcelMeasurement, application.CurrentParcelMeasurementCauseInvalid, "measure-1")
	if result.Measurement().Weight().Value() != "2.10" {
		t.Fatalf("截至较早那条的毛重 = %s", result.Measurement().Weight().Value())
	}
}

func TestCurrentAsOfKeepsAMeasurementThatOccurredAtTheBound(t *testing.T) {
	atBound := time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC)
	ports := &currentParcelPorts{
		units:        []string{"unit-1"},
		measurements: []domain.ActualMeasurement{weighedMeasurement(t, "measure-1", "2.10", atBound)},
	}
	result, err := askCurrent(t, ports, atBound)
	assertCurrent(t, result, err, application.OneCurrentParcelMeasurement, application.CurrentParcelMeasurementCauseInvalid, "measure-1")

	later := time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC)
	both := twoWeighings(t, atBound, later, false)
	kept, err := askCurrent(t, both, later)
	assertCurrent(t, kept, err, application.UndeterminedCurrentParcelMeasurement, application.SeveralStillValidWithoutAdoption, "")
}

func TestCurrentAsOfAnswersNoneWhenEveryMeasurementOccurredLater(t *testing.T) {
	earlier := time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC)
	later := time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC)
	ports := twoWeighings(t, earlier, later, false)
	result, err := askCurrent(t, ports, earlier.Add(-time.Second))
	assertCurrent(t, result, err, application.NoCurrentParcelMeasurement, application.CurrentParcelMeasurementCauseInvalid, "")
}

func TestCurrentParcelMeasurementReturnsEachPortError(t *testing.T) {
	sentinel := errors.New("port down")
	cases := []struct {
		name  string
		ports *currentParcelPorts
	}{
		{name: "作业实物", ports: &currentParcelPorts{unitsErr: sentinel}},
		{name: "仍有效实测", ports: &currentParcelPorts{units: []string{"unit-1"}, measurementsErr: sentinel}},
		{name: "采用形态", ports: &currentParcelPorts{units: []string{"unit-1"}, adoptionErr: sentinel}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := askCurrent(t, tc.ports, time.Time{})
			if err != sentinel {
				t.Fatalf("err = %v，want 原样 %v", err, sentinel)
			}
			_, err = askCurrent(t, tc.ports, time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC))
			if err != sentinel {
				t.Fatalf("带时点 err = %v，want 原样 %v", err, sentinel)
			}
		})
	}
}

func TestABlankParcelAnswersNoMeasurement(t *testing.T) {
	ports := refusingParcelPorts{t: t}
	query, err := application.NewCurrentParcelMeasurementQuery(ports, ports, ports)
	if err != nil {
		t.Fatal(err)
	}
	result, err := query.Current(context.Background(), mustTenant(t, "tenant-a"), domain.ParcelAssociationReference{})
	assertCurrent(t, result, err, application.NoCurrentParcelMeasurement, application.CurrentParcelMeasurementCauseInvalid, "")
	result, err = query.CurrentAsOf(context.Background(), mustTenant(t, "tenant-a"), domain.ParcelAssociationReference{}, time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC))
	assertCurrent(t, result, err, application.NoCurrentParcelMeasurement, application.CurrentParcelMeasurementCauseInvalid, "")
}

type currentParcelPorts struct {
	t               *testing.T
	units           []string
	measurements    []domain.ActualMeasurement
	adopted         bool
	unitsErr        error
	measurementsErr error
	adoptionErr     error
	askedKind       domain.MeasurementObjectKind
	askedObject     string
}

func (fixture *currentParcelPorts) ListHandlingUnits(context.Context, domain.TenantID, domain.ParcelAssociationReference) ([]domain.HandlingUnitID, error) {
	if fixture.unitsErr != nil {
		return nil, fixture.unitsErr
	}
	units := make([]domain.HandlingUnitID, 0, len(fixture.units))
	for _, raw := range fixture.units {
		units = append(units, handlingUnit(fixture.t, raw))
	}
	return units, nil
}

func (fixture *currentParcelPorts) ListStillValid(_ context.Context, _ domain.TenantID, kind domain.MeasurementObjectKind, objectID string) ([]ports.ActualMeasurementRecord, error) {
	fixture.askedKind = kind
	fixture.askedObject = objectID
	if fixture.measurementsErr != nil {
		return nil, fixture.measurementsErr
	}
	records := make([]ports.ActualMeasurementRecord, 0, len(fixture.measurements))
	for _, measurement := range fixture.measurements {
		records = append(records, ports.ActualMeasurementRecord{Measurement: measurement})
	}
	return records, nil
}

func (fixture *currentParcelPorts) FindAdoption(context.Context, domain.TenantID) (ports.MeasurementSelectionAdoption, bool, error) {
	if fixture.adoptionErr != nil {
		return ports.MeasurementSelectionAdoption{}, false, fixture.adoptionErr
	}
	if !fixture.adopted {
		return ports.MeasurementSelectionAdoption{}, false, nil
	}
	return ports.MeasurementSelectionAdoption{Form: domain.LatestStillValidByOccurredAt, Basis: "basis-1"}, true, nil
}

type refusingParcelPorts struct{ t *testing.T }

func (fixture refusingParcelPorts) ListHandlingUnits(context.Context, domain.TenantID, domain.ParcelAssociationReference) ([]domain.HandlingUnitID, error) {
	fixture.t.Fatal("空白包裹去问了作业实物")
	return nil, nil
}

func (fixture refusingParcelPorts) ListStillValid(context.Context, domain.TenantID, domain.MeasurementObjectKind, string) ([]ports.ActualMeasurementRecord, error) {
	fixture.t.Fatal("空白包裹去问了实测")
	return nil, nil
}

func (fixture refusingParcelPorts) FindAdoption(context.Context, domain.TenantID) (ports.MeasurementSelectionAdoption, bool, error) {
	fixture.t.Fatal("空白包裹去问了采用形态")
	return ports.MeasurementSelectionAdoption{}, false, nil
}

func askCurrent(t *testing.T, ports *currentParcelPorts, asOf time.Time) (application.CurrentParcelMeasurement, error) {
	t.Helper()
	ports.t = t
	query, err := application.NewCurrentParcelMeasurementQuery(ports, ports, ports)
	if err != nil {
		t.Fatal(err)
	}
	parcel := mustValue(t, domain.NewParcelAssociationReference, "parcel-1")
	if asOf.IsZero() {
		return query.Current(context.Background(), mustTenant(t, "tenant-a"), parcel)
	}
	return query.CurrentAsOf(context.Background(), mustTenant(t, "tenant-a"), parcel, asOf)
}

func twoWeighings(t *testing.T, earlier, later time.Time, adopted bool) *currentParcelPorts {
	t.Helper()
	return &currentParcelPorts{
		units: []string{"unit-1"},
		measurements: []domain.ActualMeasurement{
			weighedMeasurement(t, "measure-1", "2.10", earlier),
			weighedMeasurement(t, "measure-2", "2.40", later),
		},
		adopted: adopted,
	}
}

func assertCurrent(
	t *testing.T,
	result application.CurrentParcelMeasurement,
	err error,
	answer application.CurrentParcelMeasurementAnswer,
	cause application.CurrentParcelMeasurementCause,
	sourceID string,
) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer() != answer || result.Cause() != cause || result.Measurement().Source().SourceID() != sourceID {
		t.Fatalf("answer=%s cause=%s source=%q，want answer=%s cause=%s source=%q",
			result.Answer(), result.Cause(), result.Measurement().Source().SourceID(), answer, cause, sourceID)
	}
}

func weighedMeasurement(t *testing.T, sourceID, value string, occurred time.Time) domain.ActualMeasurement {
	t.Helper()
	gross, err := domain.NewGrossWeight(value, "KG")
	if err != nil {
		t.Fatal(err)
	}
	outer, err := domain.NewOuterDimensions("30", "25", "20", "CM")
	if err != nil {
		t.Fatal(err)
	}
	measurement, err := domain.NewActualMeasurement(domain.ActualMeasurementSpec{
		Source:     workSource(t, sourceID, occurred),
		ObjectKind: domain.HandlingUnitObject,
		ObjectID:   "unit-1",
		Node:       mustValue(t, domain.NewNodeReference, "node-origin"),
		Location:   mustValue(t, domain.NewWorkLocationReference, "bench-2"),
		Basis:      mustValue(t, domain.NewWorkBasisReference, "intake-1"),
		Weight:     gross,
		Dimensions: outer,
	})
	if err != nil {
		t.Fatal(err)
	}
	return measurement
}
