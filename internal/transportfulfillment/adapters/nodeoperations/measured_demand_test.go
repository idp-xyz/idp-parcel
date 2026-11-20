package nodeoperations

import (
	"testing"
	"time"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
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

func mustNode[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
