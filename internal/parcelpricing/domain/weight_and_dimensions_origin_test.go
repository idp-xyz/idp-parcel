package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

func TestWeightAndDimensionsOriginFollowsTheMeasurementFact(t *testing.T) {
	bare := syntheticInputForSubject(t, packageSubject(t, "package-1"), "1.50", "Z1")
	if _, ok := domain.WeightAndDimensionsOriginOf(bare); ok {
		t.Fatal("没有测量事实也答了来源")
	}
	actual := inputWithFact(t, "2.10", domain.ArtifactActualMeasurement, "measure-1")
	origin, ok := domain.WeightAndDimensionsOriginOf(actual)
	if !ok || origin.Kind != domain.ArtifactActualMeasurement ||
		origin.ActualWeight.Value().String() != "2.1" || origin.ActualWeight.Unit() != domain.WeightUnitKilogram {
		t.Fatalf("实测 %+v %v", origin, ok)
	}
	declared := inputWithFact(t, "1.50", domain.ArtifactDeclaredMeasurement, "declaration")
	origin, ok = domain.WeightAndDimensionsOriginOf(declared)
	if !ok || origin.Kind != domain.ArtifactDeclaredMeasurement {
		t.Fatalf("申报 %+v %v", origin, ok)
	}
	// 实测整份取用，解析器不会造出两类事实同在的快照；真遇到了，实测在场就不让申报顶替。
	both := inputWithFacts(t, "2.10", nil,
		fact(t, domain.ArtifactDeclaredMeasurement, "declaration"),
		fact(t, domain.ArtifactActualMeasurement, "measure-1"),
	)
	origin, ok = domain.WeightAndDimensionsOriginOf(both)
	if !ok || origin.Kind != domain.ArtifactActualMeasurement || origin.ActualWeight.Value().String() != "2.1" {
		t.Fatalf("两类事实同在 %+v %v，want 实测", origin, ok)
	}
}

// Covers: 来源连尺寸一起答——取较大者的卡上，实重没变、尺寸变了，计价重量照样会变，只答实重说不清它从哪来。实测一定
// 带尺寸；申报没报尺寸时答没有，不拿别的顶上。
func TestWeightAndDimensionsOriginCarriesTheDimensionsTheEvaluationUsed(t *testing.T) {
	remeasured := dimensions(t, "25", "30", "20", domain.LengthUnitCentimeter)
	measured := inputWithFacts(t, "2.10", &remeasured, fact(t, domain.ArtifactActualMeasurement, "measure-2"))
	origin, ok := domain.WeightAndDimensionsOriginOf(measured)
	if !ok || !origin.HasDimensions {
		t.Fatalf("实测 %+v %v，want 带尺寸", origin, ok)
	}
	sides := origin.Dimensions
	if sides.LongestSide().Value().String() != "30" || sides.SecondLongestSide().Value().String() != "25" ||
		sides.ShortestSide().Value().String() != "20" || sides.Unit() != domain.LengthUnitCentimeter {
		t.Fatalf("尺寸 = %+v，want 30×25×20 CM", sides)
	}

	undeclared := inputWithFact(t, "1.50", domain.ArtifactDeclaredMeasurement, "declaration")
	origin, ok = domain.WeightAndDimensionsOriginOf(undeclared)
	if !ok || origin.HasDimensions {
		t.Fatalf("申报没报尺寸 %+v %v，want 来源在、尺寸不在", origin, ok)
	}
}

// Covers: 实测来源身份只在评价输入里恰好一条实际测量事实引用时带出，取的是那条引用的版本（那次实际测量的来源身份）。
// 只有申报时不带；多于一条实际测量事实引用时不带——聚合主体上每条实测各是一个成员的，挑其中一条会把计价重量说成出自没被采用的那次测量。
// 种类、实重、尺寸各格保持原样。一条实测旁边另有申报事实时，仍只认那一条实测。
func TestWeightAndDimensionsOriginCarriesTheMeasurementSourceOnlyForTheSingleActualFact(t *testing.T) {
	sides := dimensions(t, "25", "30", "20", domain.LengthUnitCentimeter)

	one := inputWithFacts(t, "2.10", &sides, fact(t, domain.ArtifactActualMeasurement, "SYN-WEIGH-M2"))
	origin, ok := domain.WeightAndDimensionsOriginOf(one)
	if !ok || origin.Kind != domain.ArtifactActualMeasurement || origin.MeasurementSource != "SYN-WEIGH-M2" ||
		origin.ActualWeight.Value().String() != "2.1" || !origin.HasDimensions ||
		origin.Dimensions.LongestSide().Value().String() != "30" ||
		origin.Dimensions.SecondLongestSide().Value().String() != "25" ||
		origin.Dimensions.ShortestSide().Value().String() != "20" {
		t.Fatalf("恰好一条实测 %+v %v，want 来源身份 SYN-WEIGH-M2、实重 2.1、尺寸 30×25×20", origin, ok)
	}

	withDeclared := inputWithFacts(t, "2.10", &sides,
		fact(t, domain.ArtifactDeclaredMeasurement, "declaration"),
		fact(t, domain.ArtifactActualMeasurement, "SYN-WEIGH-M1"),
	)
	origin, ok = domain.WeightAndDimensionsOriginOf(withDeclared)
	if !ok || origin.Kind != domain.ArtifactActualMeasurement || origin.MeasurementSource != "SYN-WEIGH-M1" ||
		origin.ActualWeight.Value().String() != "2.1" {
		t.Fatalf("一条实测加申报 %+v %v，want 实测来源身份 SYN-WEIGH-M1", origin, ok)
	}

	declared := inputWithFacts(t, "1.50", &sides, fact(t, domain.ArtifactDeclaredMeasurement, "declaration"))
	origin, ok = domain.WeightAndDimensionsOriginOf(declared)
	if !ok || origin.Kind != domain.ArtifactDeclaredMeasurement || origin.MeasurementSource != "" ||
		origin.ActualWeight.Value().String() != "1.5" || !origin.HasDimensions ||
		origin.Dimensions.LongestSide().Value().String() != "30" {
		t.Fatalf("只有申报 %+v %v，want 种类申报、来源身份缺席、实重与尺寸仍在", origin, ok)
	}

	several := inputWithFacts(t, "2.10", &sides,
		fact(t, domain.ArtifactActualMeasurement, "SYN-WEIGH-M1"),
		fact(t, domain.ArtifactActualMeasurement, "SYN-WEIGH-M2"),
	)
	origin, ok = domain.WeightAndDimensionsOriginOf(several)
	if !ok || origin.Kind != domain.ArtifactActualMeasurement || origin.MeasurementSource != "" ||
		origin.ActualWeight.Value().String() != "2.1" || !origin.HasDimensions ||
		origin.Dimensions.LongestSide().Value().String() != "30" ||
		origin.Dimensions.ShortestSide().Value().String() != "20" {
		t.Fatalf("多于一条实测 %+v %v，want 种类与实重尺寸仍在、来源身份缺席", origin, ok)
	}
}

func inputWithFact(t *testing.T, weight string, kind domain.ArtifactKind, version string) domain.PricingInputSnapshot {
	t.Helper()
	return inputWithFacts(t, weight, nil, fact(t, kind, version))
}

func inputWithFacts(
	t *testing.T,
	weight string,
	sides *domain.Dimensions,
	facts ...domain.VersionedFactReference,
) domain.PricingInputSnapshot {
	t.Helper()
	input, err := domain.NewPricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		packageSubject(t, "package-1"),
		"Z1",
		weightOf(t, weight),
		sides,
		time.Date(2026, 11, 17, 1, 0, 0, 0, time.UTC),
		facts...,
	)
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func fact(t *testing.T, kind domain.ArtifactKind, version string) domain.VersionedFactReference {
	t.Helper()
	reference, err := domain.NewVersionReferenceIdentity(kind, "package-1", version)
	if err != nil {
		t.Fatal(err)
	}
	built, err := domain.NewVersionedFactReference(reference)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func weightOf(t *testing.T, value string) domain.Weight {
	t.Helper()
	return weight(t, value, domain.WeightUnitKilogram)
}
