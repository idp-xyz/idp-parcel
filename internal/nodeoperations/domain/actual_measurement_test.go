package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

func measurementSource(t *testing.T, sourceID string) domain.WorkFactSource {
	t.Helper()
	source, err := domain.NewWorkFactSource(
		sourceID,
		mustValue(t, domain.NewPerformingPartyReference, "scale-7"),
		mustValue(t, domain.NewExecutionEvidenceReference, "WEIGH/7"),
		time.Date(2026, 11, 17, 2, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("来源：%v", err)
	}
	return source
}

func grossWeight(t *testing.T, value, unit string) domain.GrossWeight {
	t.Helper()
	weight, err := domain.NewGrossWeight(value, unit)
	if err != nil {
		t.Fatalf("毛重：%v", err)
	}
	return weight
}

func outerDimensions(t *testing.T, length, width, height, unit string) domain.OuterDimensions {
	t.Helper()
	dimensions, err := domain.NewOuterDimensions(length, width, height, unit)
	if err != nil {
		t.Fatalf("外廓尺寸：%v", err)
	}
	return dimensions
}

// measurementSpec 是一条完整的测量：毛重 2.10 KG，尺寸 20×15.5×10 CM。
func measurementSpec(t *testing.T) domain.ActualMeasurementSpec {
	t.Helper()
	return domain.ActualMeasurementSpec{
		Source:     measurementSource(t, "measure-1"),
		ObjectKind: domain.HandlingUnitObject,
		ObjectID:   "unit-1",
		Node:       mustValue(t, domain.NewNodeReference, "node-origin"),
		Location:   mustValue(t, domain.NewWorkLocationReference, "bench-2"),
		Basis:      mustValue(t, domain.NewWorkBasisReference, "intake-1"),
		Weight:     grossWeight(t, "2.10", "KG"),
		Dimensions: outerDimensions(t, "20", "15.5", "10", "CM"),
	}
}

func TestAnActualMeasurementKeepsTheReportedWeightAndDimensions(t *testing.T) {
	measurement, err := domain.NewActualMeasurement(measurementSpec(t))
	if err != nil {
		t.Fatalf("形成测量：%v", err)
	}
	if weight := measurement.Weight(); weight.Value() != "2.10" || weight.Unit() != "KG" {
		t.Fatalf("毛重 = %s %s", weight.Value(), weight.Unit())
	}
	if size := measurement.Dimensions(); size.Length() != "20" || size.Width() != "15.5" || size.Height() != "10" || size.Unit() != "CM" {
		t.Fatalf("尺寸 = %s %s %s %s", size.Length(), size.Width(), size.Height(), size.Unit())
	}
	if measurement.ContentDigest() == "" || measurement.ContentDigest() != measurement.ContentDigest() {
		t.Fatal("同一条测量的内容摘要不稳定")
	}

	for name, change := range map[string]func(*domain.ActualMeasurementSpec){
		"重量变了": func(spec *domain.ActualMeasurementSpec) { spec.Weight = grossWeight(t, "2.40", "KG") },
		"尺寸变了": func(spec *domain.ActualMeasurementSpec) {
			spec.Dimensions = outerDimensions(t, "20", "15.5", "12", "CM")
		},
	} {
		changed := measurementSpec(t)
		change(&changed)
		other, err := domain.NewActualMeasurement(changed)
		if err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		if other.ContentDigest() == measurement.ContentDigest() {
			t.Fatalf("%s，摘要却没变", name)
		}
	}
}

// Covers: 重量与尺寸都是必填。只称了重、没量方的那一条不收，只量了方、没称重的也不收。数字要是正数、单位要在场，这一层
// 由两个值对象的构造器管着，见 TestAGrossWeightRefusesAMalformedQuantityOrAMissingUnit 与
// TestOuterDimensionsRefuseAnySideThatIsMalformedOrAMissingUnit。
func TestAMeasurementNeedsBothItsWeightAndItsDimensions(t *testing.T) {
	cases := map[string]func(domain.ActualMeasurementSpec) domain.ActualMeasurementSpec{
		"只称了重、没量方": func(spec domain.ActualMeasurementSpec) domain.ActualMeasurementSpec {
			spec.Dimensions = domain.OuterDimensions{}
			return spec
		},
		"只量了方、没称重": func(spec domain.ActualMeasurementSpec) domain.ActualMeasurementSpec {
			spec.Weight = domain.GrossWeight{}
			return spec
		},
		"只有录入人没有原件": func(spec domain.ActualMeasurementSpec) domain.ActualMeasurementSpec {
			spec.EnteredBy = "operator-1"
			return spec
		},
		"对象种类空": func(spec domain.ActualMeasurementSpec) domain.ActualMeasurementSpec {
			spec.ObjectKind = domain.MeasurementObjectKindInvalid
			return spec
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := domain.NewActualMeasurement(change(measurementSpec(t)))
			if !errors.Is(err, domain.ErrInvalidActualMeasurement) {
				t.Fatalf("实得 %v", err)
			}
		})
	}
}

func TestAManualMeasurementKeepsWhoEnteredIt(t *testing.T) {
	spec := measurementSpec(t)
	spec.EnteredBy = "operator-1"
	spec.OriginalDocument = mustValue(t, domain.NewMeasurementOriginalDocument, "slip-9")
	measurement, err := domain.NewActualMeasurement(spec)
	if err != nil {
		t.Fatalf("形成手工测量：%v", err)
	}
	enteredBy, document, present := measurement.ManualEntry()
	if !present || enteredBy != "operator-1" || document.String() != "slip-9" {
		t.Fatalf("手工录入 = %s %s present=%v", enteredBy, document, present)
	}
	device, err := domain.NewActualMeasurement(measurementSpec(t))
	if err != nil {
		t.Fatalf("形成设备测量：%v", err)
	}
	if measurement.ContentDigest() == device.ContentDigest() {
		t.Fatal("手工录入与设备上报被收成了同一份内容")
	}
}

func TestAnInvalidationPointsBackWithoutCopyingTheQuantity(t *testing.T) {
	invalidation, err := domain.NewMeasurementInvalidation(domain.MeasurementInvalidationSpec{
		Source:         measurementSource(t, "void-1"),
		TargetSourceID: "measure-1",
		Reason:         "称错件",
	})
	if err != nil {
		t.Fatalf("作废：%v", err)
	}
	if invalidation.TargetSourceID() != "measure-1" || invalidation.Reason() != "称错件" {
		t.Fatalf("作废指向 %s，原因 %s", invalidation.TargetSourceID(), invalidation.Reason())
	}
	if invalidation.ContentDigest() == "" {
		t.Fatal("作废没有内容摘要")
	}

	sameIdentity, err := domain.NewMeasurementInvalidation(domain.MeasurementInvalidationSpec{
		Source:         measurementSource(t, "measure-1"),
		TargetSourceID: "measure-1",
		Reason:         "称错件",
	})
	if !errors.Is(err, domain.ErrInvalidMeasurementInvalidation) || sameIdentity.Reason() != "" {
		t.Fatal("作废用了被作废那条自己的来源身份")
	}
}

// 带纳秒的发生时间与同一时刻截到微秒后的样子。接入口按 RFC 3339 能收到纳秒，而库列只存到微秒。
var (
	nanosecondOccurrence  = time.Date(2026, 11, 17, 2, 0, 0, 123456789, time.UTC)
	microsecondOccurrence = time.Date(2026, 11, 17, 2, 0, 0, 123456000, time.UTC)
)

func measurementSourceAt(t *testing.T, sourceID string, occurredAt time.Time) domain.WorkFactSource {
	t.Helper()
	source, err := domain.NewWorkFactSource(
		sourceID,
		mustValue(t, domain.NewPerformingPartyReference, "scale-7"),
		mustValue(t, domain.NewExecutionEvidenceReference, "WEIGH/7"),
		occurredAt,
	)
	if err != nil {
		t.Fatalf("来源：%v", err)
	}
	return source
}

// Covers: 实际测量在形成处把发生时间折成册上的样子（UTC、微秒）。库列只存到微秒，读回时用行上的值重算摘要；形成时不截，
// 同一条测量落册前后的摘要就差几个纳秒。来源上的发生时间、摘要都用折过的那一个时刻，微秒以内仍分得开。
func TestAMeasurementIsFormedAtTheRegisterInstantOfItsOccurrence(t *testing.T) {
	nanos := measurementSpec(t)
	nanos.Source = measurementSourceAt(t, "measure-1", nanosecondOccurrence)
	formed, err := domain.NewActualMeasurement(nanos)
	if err != nil {
		t.Fatalf("形成带纳秒的测量：%v", err)
	}
	occurred := formed.Source().OccurredAt()
	if !occurred.Equal(microsecondOccurrence) || occurred.Nanosecond()%1000 != 0 || occurred.Location() != time.UTC {
		t.Fatalf("发生时间 = %s（纳秒 %d），want %s", occurred.Format(time.RFC3339Nano), occurred.Nanosecond(), microsecondOccurrence.Format(time.RFC3339Nano))
	}
	if nanos.Source.OccurredAt().Nanosecond() != nanosecondOccurrence.Nanosecond() {
		t.Fatal("折叠改了调用方手里的那份来源")
	}

	micros := measurementSpec(t)
	micros.Source = measurementSourceAt(t, "measure-1", microsecondOccurrence)
	atMicros, err := domain.NewActualMeasurement(micros)
	if err != nil {
		t.Fatalf("形成截到微秒的测量：%v", err)
	}
	if formed.ContentDigest() != atMicros.ContentDigest() {
		t.Fatalf("带纳秒的摘要 %s ≠ 同一时刻截到微秒的摘要 %s", formed.ContentDigest(), atMicros.ContentDigest())
	}

	later := measurementSpec(t)
	later.Source = measurementSourceAt(t, "measure-1", microsecondOccurrence.Add(time.Microsecond))
	atLater, err := domain.NewActualMeasurement(later)
	if err != nil {
		t.Fatalf("形成晚一微秒的测量：%v", err)
	}
	if formed.ContentDigest() == atLater.ContentDigest() {
		t.Fatal("晚一微秒的测量摘要没变，折叠截得太粗")
	}
}

// Covers: 作废与测量同理，形成处折到微秒；作废的摘要里有发生时间，不折就在库里读不回来。
func TestAnInvalidationIsFormedAtTheRegisterInstantOfItsOccurrence(t *testing.T) {
	form := func(occurredAt time.Time) domain.MeasurementInvalidation {
		t.Helper()
		invalidation, err := domain.NewMeasurementInvalidation(domain.MeasurementInvalidationSpec{
			Source:         measurementSourceAt(t, "void-1", occurredAt),
			TargetSourceID: "measure-1",
			Reason:         "称错件",
		})
		if err != nil {
			t.Fatalf("作废：%v", err)
		}
		return invalidation
	}
	formed := form(nanosecondOccurrence)
	occurred := formed.Source().OccurredAt()
	if !occurred.Equal(microsecondOccurrence) || occurred.Nanosecond()%1000 != 0 || occurred.Location() != time.UTC {
		t.Fatalf("发生时间 = %s（纳秒 %d），want %s", occurred.Format(time.RFC3339Nano), occurred.Nanosecond(), microsecondOccurrence.Format(time.RFC3339Nano))
	}
	if formed.ContentDigest() != form(microsecondOccurrence).ContentDigest() {
		t.Fatal("带纳秒的作废摘要与同一时刻截到微秒的不同")
	}
	if formed.ContentDigest() == form(microsecondOccurrence.Add(time.Microsecond)).ContentDigest() {
		t.Fatal("晚一微秒的作废摘要没变，折叠截得太粗")
	}
}
