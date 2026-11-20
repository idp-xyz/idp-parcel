package nodeoperations_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/nodeoperations"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	noapplication "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

func TestNoAssociatedUnitAnswersNoMeasurement(t *testing.T) {
	view := measuredWeights(t, nil, nil, false)
	reading, err := view.LoadMemberMeasuredWeight(context.Background(), customsTenant(t), customsParcel(t, "parcel-1"))
	if err != nil || reading.Answer != ccports.MeasuredWeightNone || reading.GrossWeight != "" {
		t.Fatalf("没有实物 = %+v err=%v", reading, err)
	}
}

func TestOneMeasurementIsPassedThroughWithoutConversion(t *testing.T) {
	occurred := time.Date(2026, 11, 18, 2, 0, 0, 0, time.UTC)
	only := customsMeasured(t, "SYN-WEIGH-1", "G", "2.10", occurred)
	view := measuredWeights(t, []string{"unit-1"}, []nodomain.ActualMeasurement{only}, false)
	reading, err := view.LoadMemberMeasuredWeight(context.Background(), customsTenant(t), customsParcel(t, "parcel-1"))
	if err != nil || reading.Answer != ccports.MeasuredWeightOne ||
		reading.GrossWeight != only.Weight().Value() || reading.GrossWeightUnit != "G" ||
		reading.MeasurementSource != "SYN-WEIGH-1" || !reading.MeasuredAt.Equal(occurred) {
		t.Fatalf("原样 = %+v 领域数值 %s err=%v", reading, only.Weight().Value(), err)
	}
}

func TestSeveralMeasurementsWithoutAdoptionStayUndetermined(t *testing.T) {
	earlier := customsMeasured(t, "SYN-WEIGH-1", "KG", "2.1", time.Date(2026, 11, 18, 2, 0, 0, 0, time.UTC))
	later := customsMeasured(t, "SYN-WEIGH-2", "KG", "3", time.Date(2026, 11, 18, 3, 0, 0, 0, time.UTC))
	view := measuredWeights(t, []string{"unit-1", "unit-2"}, nil, false)
	severalUnits, err := view.LoadMemberMeasuredWeight(context.Background(), customsTenant(t), customsParcel(t, "parcel-1"))
	if err != nil || severalUnits.Answer != ccports.MeasuredWeightUndetermined ||
		severalUnits.Cause != noapplication.SeveralAssociatedHandlingUnits.String() {
		t.Fatalf("多件实物 = %+v err=%v", severalUnits, err)
	}
	view = measuredWeights(t, []string{"unit-1"}, []nodomain.ActualMeasurement{earlier, later}, false)
	several, err := view.LoadMemberMeasuredWeight(context.Background(), customsTenant(t), customsParcel(t, "parcel-1"))
	if err != nil || several.Answer != ccports.MeasuredWeightUndetermined ||
		several.Cause != noapplication.SeveralStillValidWithoutAdoption.String() || several.GrossWeight != "" {
		t.Fatalf("未采用 = %+v err=%v", several, err)
	}
}

func TestAMeasurementQueryErrorIsNotTurnedIntoNone(t *testing.T) {
	query, err := noapplication.NewCurrentParcelMeasurementQuery(failingUnits{}, measurementPorts{}, measurementPorts{})
	if err != nil {
		t.Fatal(err)
	}
	view, err := adapter.NewMemberMeasuredWeights(query)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := view.LoadMemberMeasuredWeight(context.Background(), customsTenant(t), customsParcel(t, "parcel-1")); err == nil {
		t.Fatal("读口报错被吞掉了")
	}
}

type failingUnits struct{}

func (failingUnits) ListHandlingUnits(context.Context, nodomain.TenantID, nodomain.ParcelAssociationReference) ([]nodomain.HandlingUnitID, error) {
	return nil, errors.New("associations down")
}

type measurementPorts struct {
	units        []string
	measurements []nodomain.ActualMeasurement
	adopted      bool
}

func (ports measurementPorts) ListHandlingUnits(context.Context, nodomain.TenantID, nodomain.ParcelAssociationReference) ([]nodomain.HandlingUnitID, error) {
	units := make([]nodomain.HandlingUnitID, 0, len(ports.units))
	for _, raw := range ports.units {
		unit, err := nodomain.NewHandlingUnitID(raw)
		if err != nil {
			return nil, err
		}
		units = append(units, unit)
	}
	return units, nil
}

func (ports measurementPorts) ListStillValid(context.Context, nodomain.TenantID, nodomain.MeasurementObjectKind, string) ([]noports.ActualMeasurementRecord, error) {
	records := make([]noports.ActualMeasurementRecord, 0, len(ports.measurements))
	for _, measurement := range ports.measurements {
		records = append(records, noports.ActualMeasurementRecord{Measurement: measurement})
	}
	return records, nil
}

func (ports measurementPorts) FindAdoption(context.Context, nodomain.TenantID) (noports.MeasurementSelectionAdoption, bool, error) {
	if !ports.adopted {
		return noports.MeasurementSelectionAdoption{}, false, nil
	}
	return noports.MeasurementSelectionAdoption{Form: nodomain.LatestStillValidByOccurredAt, Basis: "basis-1"}, true, nil
}

func measuredWeights(t *testing.T, units []string, measurements []nodomain.ActualMeasurement, adopted bool) *adapter.MemberMeasuredWeights {
	t.Helper()
	ports := measurementPorts{units: units, measurements: measurements, adopted: adopted}
	query, err := noapplication.NewCurrentParcelMeasurementQuery(ports, ports, ports)
	if err != nil {
		t.Fatal(err)
	}
	view, err := adapter.NewMemberMeasuredWeights(query)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func customsMeasured(t *testing.T, sourceID, unit, value string, occurred time.Time) nodomain.ActualMeasurement {
	t.Helper()
	source, err := nodomain.NewWorkFactSource(
		sourceID,
		mustNode(t, nodomain.NewPerformingPartyReference, "scale-7"),
		mustNode(t, nodomain.NewExecutionEvidenceReference, "WEIGH/"+sourceID),
		occurred,
	)
	if err != nil {
		t.Fatal(err)
	}
	gross, err := nodomain.NewGrossWeight(value, unit)
	if err != nil {
		t.Fatal(err)
	}
	outer, err := nodomain.NewOuterDimensions("30", "25", "20", "CM")
	if err != nil {
		t.Fatal(err)
	}
	measurement, err := nodomain.NewActualMeasurement(nodomain.ActualMeasurementSpec{
		Source: source, ObjectKind: nodomain.HandlingUnitObject, ObjectID: "unit-1",
		Node: mustNode(t, nodomain.NewNodeReference, "node-origin"), Location: mustNode(t, nodomain.NewWorkLocationReference, "bench-2"),
		Basis:  mustNode(t, nodomain.NewWorkBasisReference, "intake-1"),
		Weight: gross, Dimensions: outer,
	})
	if err != nil {
		t.Fatal(err)
	}
	return measurement
}

func customsTenant(t *testing.T) ccdomain.TenantID {
	t.Helper()
	tenant, err := ccdomain.NewTenantID("tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	return tenant
}

func customsParcel(t *testing.T, raw string) ccdomain.DeclaredParcelReference {
	t.Helper()
	parcel, err := ccdomain.NewDeclaredParcelReference(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parcel
}

func mustNode[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
