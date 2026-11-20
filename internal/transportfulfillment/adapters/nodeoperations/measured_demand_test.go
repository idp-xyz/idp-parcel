package nodeoperations

import (
	"context"
	"errors"
	"testing"
	"time"

	noapplication "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: 实测译成需求量时重量、三边单位与来源身份原样交，体积仍由 MeasuredDemand 用三边乘出。
func TestMeasuredReadingCopiesTheMeasurementWithoutConvertingIt(t *testing.T) {
	occurred := time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
	source, err := nodomain.NewWorkFactSource(
		"SYN-MEASURE-1",
		mustNode[nodomain.PerformingPartyReference](t, nodomain.NewPerformingPartyReference, "SYN-SCALE"),
		mustNode[nodomain.ExecutionEvidenceReference](t, nodomain.NewExecutionEvidenceReference, "WEIGH/1"),
		occurred,
	)
	if err != nil {
		t.Fatalf("来源：%v", err)
	}
	weight, err := nodomain.NewGrossWeight("2.50", "KG")
	if err != nil {
		t.Fatalf("毛重：%v", err)
	}
	dimensions, err := nodomain.NewOuterDimensions("30", "20", "10", "CM")
	if err != nil {
		t.Fatalf("外廓：%v", err)
	}
	measurement, err := nodomain.NewActualMeasurement(nodomain.ActualMeasurementSpec{
		Source: source, ObjectKind: nodomain.HandlingUnitObject, ObjectID: "SYN-HU-1",
		Node:     mustNode[nodomain.NodeReference](t, nodomain.NewNodeReference, "SYN-NODE-A"),
		Location: mustNode[nodomain.WorkLocationReference](t, nodomain.NewWorkLocationReference, "bench"),
		Basis:    mustNode[nodomain.WorkBasisReference](t, nodomain.NewWorkBasisReference, "intake"),
		Weight:   weight, Dimensions: dimensions,
	})
	if err != nil {
		t.Fatalf("实测：%v", err)
	}
	reading, err := measuredReadingOf(measurement)
	if err != nil {
		t.Fatalf("译：%v", err)
	}
	if reading.Answer != tfports.MeasuredDemandOne || !reading.OccurredAt.Equal(occurred) {
		t.Fatalf("答复 = %s %s", reading.Answer, reading.OccurredAt)
	}
	anchor, present := reading.Snapshot.Anchor()
	if reading.Snapshot.Source() != tfdomain.DemandFromActualMeasurement || !present || anchor != "SYN-MEASURE-1" {
		t.Fatalf("来源 %s 锚 %q", reading.Snapshot.Source(), anchor)
	}
	quantities := reading.Snapshot.Quantities()
	if len(quantities) != 3 || quantities[0].Amount != "2.50" || quantities[0].Unit.String() != "KG" ||
		quantities[1].Amount != "6000" || quantities[1].Unit.String() != "CM3" || quantities[2].Amount != "1" {
		t.Fatalf("数量 = %+v", quantities)
	}
}

// Covers: LoadMeasuredDemand 保住没有、恰一条、定不下三种答案；查询报错原样交回，不包成译不过去。
func TestLoadMeasuredDemandKeepsEachAnswerAndReturnsAQueryErrorAsIs(t *testing.T) {
	occurred := time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
	measurement := weighedParcel(t, "SYN-MEASURE-1", occurred)
	cases := []struct {
		name  string
		ports measurementQueryDouble
		want  tfports.MeasuredDemandAnswer
	}{
		{"没有", measurementQueryDouble{}, tfports.MeasuredDemandNone},
		{"恰一条", measurementQueryDouble{units: []string{"SYN-HU-1"}, measurements: []nodomain.ActualMeasurement{measurement}}, tfports.MeasuredDemandOne},
		{"定不下", measurementQueryDouble{units: []string{"SYN-HU-1", "SYN-HU-2"}}, tfports.MeasuredDemandUndetermined},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			reading, err := loadMeasuredDemand(t, test.ports)
			if err != nil || reading.Answer != test.want {
				t.Fatalf("%s = %s %v", test.name, reading.Answer, err)
			}
			if test.want != tfports.MeasuredDemandOne {
				if len(reading.Snapshot.Quantities()) != 0 || !reading.OccurredAt.IsZero() {
					t.Fatalf("%s 带了快照 %+v %s", test.name, reading.Snapshot.Quantities(), reading.OccurredAt)
				}
				return
			}
			anchor, present := reading.Snapshot.Anchor()
			quantities := reading.Snapshot.Quantities()
			if !reading.OccurredAt.Equal(occurred) || reading.Snapshot.Source() != tfdomain.DemandFromActualMeasurement ||
				!present || anchor != "SYN-MEASURE-1" || len(quantities) != 3 ||
				quantities[0].Amount != "2.50" || quantities[0].Unit.String() != "KG" {
				t.Fatalf("恰一条 时刻 %s 来源 %s 锚 %q 数量 %+v", reading.OccurredAt, reading.Snapshot.Source(), anchor, quantities)
			}
		})
	}

	queryErr := errors.New("measurements down")
	reading, err := loadMeasuredDemand(t, measurementQueryDouble{err: queryErr})
	if err != queryErr || errors.Is(err, tfports.ErrMeasuredDemandUntranslatable) {
		t.Fatalf("查询报错 = %v 答复 %s", err, reading.Answer)
	}
}

type measurementQueryDouble struct {
	units        []string
	measurements []nodomain.ActualMeasurement
	err          error
}

func (double measurementQueryDouble) ListHandlingUnits(context.Context, nodomain.TenantID, nodomain.ParcelAssociationReference) ([]nodomain.HandlingUnitID, error) {
	if double.err != nil {
		return nil, double.err
	}
	units := make([]nodomain.HandlingUnitID, 0, len(double.units))
	for _, raw := range double.units {
		unit, err := nodomain.NewHandlingUnitID(raw)
		if err != nil {
			return nil, err
		}
		units = append(units, unit)
	}
	return units, nil
}

func (double measurementQueryDouble) ListStillValid(context.Context, nodomain.TenantID, nodomain.MeasurementObjectKind, string) ([]noports.ActualMeasurementRecord, error) {
	records := make([]noports.ActualMeasurementRecord, 0, len(double.measurements))
	for _, measurement := range double.measurements {
		records = append(records, noports.ActualMeasurementRecord{Measurement: measurement})
	}
	return records, nil
}

func (measurementQueryDouble) FindAdoption(context.Context, nodomain.TenantID) (noports.MeasurementSelectionAdoption, bool, error) {
	return noports.MeasurementSelectionAdoption{}, false, nil
}

func loadMeasuredDemand(t *testing.T, ports measurementQueryDouble) (tfports.MeasuredDemandReading, error) {
	t.Helper()
	query, err := noapplication.NewCurrentParcelMeasurementQuery(ports, ports, ports)
	if err != nil {
		t.Fatal(err)
	}
	source, err := NewMeasuredDemands(query)
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := tfdomain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	object, err := tfdomain.NewCarriedObjectReference("SYN-PARCEL-01")
	if err != nil {
		t.Fatal(err)
	}
	return source.LoadMeasuredDemand(context.Background(), tenant, object)
}

func weighedParcel(t *testing.T, sourceID string, occurred time.Time) nodomain.ActualMeasurement {
	t.Helper()
	source, err := nodomain.NewWorkFactSource(
		sourceID,
		mustNode[nodomain.PerformingPartyReference](t, nodomain.NewPerformingPartyReference, "SYN-SCALE"),
		mustNode[nodomain.ExecutionEvidenceReference](t, nodomain.NewExecutionEvidenceReference, "WEIGH/1"),
		occurred,
	)
	if err != nil {
		t.Fatalf("来源：%v", err)
	}
	weight, err := nodomain.NewGrossWeight("2.50", "KG")
	if err != nil {
		t.Fatalf("毛重：%v", err)
	}
	dimensions, err := nodomain.NewOuterDimensions("30", "20", "10", "CM")
	if err != nil {
		t.Fatalf("外廓：%v", err)
	}
	measurement, err := nodomain.NewActualMeasurement(nodomain.ActualMeasurementSpec{
		Source: source, ObjectKind: nodomain.HandlingUnitObject, ObjectID: "SYN-HU-1",
		Node:     mustNode[nodomain.NodeReference](t, nodomain.NewNodeReference, "SYN-NODE-A"),
		Location: mustNode[nodomain.WorkLocationReference](t, nodomain.NewWorkLocationReference, "bench"),
		Basis:    mustNode[nodomain.WorkBasisReference](t, nodomain.NewWorkBasisReference, "intake"),
		Weight:   weight, Dimensions: dimensions,
	})
	if err != nil {
		t.Fatalf("实测：%v", err)
	}
	return measurement
}

func mustNode[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
