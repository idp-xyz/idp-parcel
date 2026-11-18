package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// measuredAt 造一条完整的测量：毛重由用例给，尺寸 20×15×10 CM。
func measuredAt(t *testing.T, sourceID, weight string, occurred time.Time) domain.ActualMeasurement {
	t.Helper()
	source, err := domain.NewWorkFactSource(
		sourceID,
		mustValue(t, domain.NewPerformingPartyReference, "scale-7"),
		mustValue(t, domain.NewExecutionEvidenceReference, "WEIGH/"+sourceID),
		occurred,
	)
	if err != nil {
		t.Fatalf("来源：%v", err)
	}
	spec := measurementSpec(t)
	spec.Source = source
	spec.WeightValue = weight
	measurement, err := domain.NewActualMeasurement(spec)
	if err != nil {
		t.Fatalf("测量：%v", err)
	}
	return measurement
}

func TestOneStillValidMeasurementIsCurrentWithoutAdoption(t *testing.T) {
	only := measuredAt(t, "measure-1", "2.10", time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC))
	got, answer, cause := domain.SelectCurrentMeasurement([]domain.ActualMeasurement{only}, false)
	if answer != domain.CurrentMeasurementOne || cause != domain.CurrentMeasurementCauseInvalid {
		t.Fatalf("answer=%s cause=%s", answer, cause)
	}
	if value, _ := got.Weight(); value != "2.10" {
		t.Fatalf("取出 %s", value)
	}
	if _, answer, _ := domain.SelectCurrentMeasurement(nil, true); answer != domain.CurrentMeasurementNone {
		t.Fatalf("没有测量 answer=%s", answer)
	}
}

// Covers: 复测是再登一条完整的测量。租户采用形态之前多于一条定不下；采用之后取发生时间最近的那一条，重量与尺寸都用它的。
func TestSeveralMeasurementsStayUndeterminedUntilTheTenantAdoptsTheForm(t *testing.T) {
	earlier := measuredAt(t, "measure-1", "2.10", time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC))
	later := measuredAt(t, "measure-2", "2.40", time.Date(2026, 11, 17, 3, 0, 0, 0, time.UTC))
	_, answer, cause := domain.SelectCurrentMeasurement([]domain.ActualMeasurement{earlier, later}, false)
	if answer != domain.CurrentMeasurementUndetermined || cause != domain.SeveralMeasurementsWithoutAdoption {
		t.Fatalf("没采用：answer=%s cause=%s", answer, cause)
	}

	got, answer, cause := domain.SelectCurrentMeasurement([]domain.ActualMeasurement{earlier, later}, true)
	if answer != domain.CurrentMeasurementOne || cause != domain.CurrentMeasurementCauseInvalid || got.Source().SourceID() != "measure-2" {
		t.Fatalf("采用后取出 %s answer=%s cause=%s", got.Source().SourceID(), answer, cause)
	}
}

func TestTheSameOccurredTimeDoesNotPickEitherMeasurement(t *testing.T) {
	at := time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC)
	first := measuredAt(t, "measure-1", "2.10", at)
	second := measuredAt(t, "measure-2", "2.40", at)
	_, answer, cause := domain.SelectCurrentMeasurement([]domain.ActualMeasurement{first, second}, true)
	if answer != domain.CurrentMeasurementUndetermined || cause != domain.SameOccurredAt {
		t.Fatalf("answer=%s cause=%s", answer, cause)
	}
}
