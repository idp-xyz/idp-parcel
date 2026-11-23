package domain

import (
	"testing"
	"time"
)

// 本文件从包内钉逐件求和的换算边界：换算器为空仍拒，进位单位与查表单位不合仍拒，
// 尺寸换算走已有的体积换算，缺尺寸不退回合计。

func pieceWeightPolicy(t testing.TB, method PricingWeightMethod, unit WeightUnit, factor *VolumetricFactor) PricingWeightPolicy {
	t.Helper()
	increment, err := NewWeight(mustDecimal(t, "0.5"), unit)
	if err != nil {
		t.Fatal(err)
	}
	rounding, err := NewWeightRoundingPolicy(RoundingCeiling, increment)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := NewVersionReferenceIdentity(ArtifactWeightPolicy, "weight-piece-unit", "v1")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := NewPricingWeightPolicy(reference, method, rounding, factor)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func pieceInput(t testing.TB, total string, unit WeightUnit, pieces ...MemberPiece) PricingInputSnapshot {
	t.Helper()
	ids := make([]PackageID, 0, len(pieces))
	for _, piece := range pieces {
		ids = append(ids, piece.ID())
	}
	manifest, err := NewMemberManifest(ids, mustWeight(t, total, unit), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pieces) > 0 {
		manifest, err = manifest.WithPieces(pieces)
		if err != nil {
			t.Fatal(err)
		}
	}
	subject, err := NewShipmentSubject("SR-PIECE")
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := NewTenantID("tenant-1")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := NewPricingScopeID("scope-1")
	if err != nil {
		t.Fatal(err)
	}
	input, err := NewAggregatePricingInputSnapshot(tenant, scope, subject, "Z1", manifest, time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func memberPiece(t testing.TB, id, actual string, unit WeightUnit, sides *Dimensions) MemberPiece {
	t.Helper()
	packageID, err := NewPackageID(id)
	if err != nil {
		t.Fatal(err)
	}
	piece, err := NewMemberPiece(packageID, mustWeight(t, actual, unit), sides)
	if err != nil {
		t.Fatal(err)
	}
	return piece
}

func mustWeight(t testing.TB, text string, unit WeightUnit) Weight {
	t.Helper()
	weight, err := NewWeight(mustDecimal(t, text), unit)
	if err != nil {
		t.Fatal(err)
	}
	return weight
}

func TestNilConverterStillRejectsPerPieceUnitMismatch(t *testing.T) {
	input := pieceInput(t, "9", WeightUnitKilogram,
		memberPiece(t, "pkg-a", "1", WeightUnitKilogram, nil),
		memberPiece(t, "pkg-b", "1.1", WeightUnitKilogram, nil))
	policy := pieceWeightPolicy(t, PricingWeightActualOnly, WeightUnitPound, nil)
	if _, err := calculatePerPiecePricingWeight(input, policy, ShipmentWeightSumThenRoundOnce, WeightUnitPound, nil); err != ErrWeightUnitMismatch {
		t.Fatalf("实重 err = %v，want ErrWeightUnitMismatch", err)
	}

	sides, err := NewDimensions(mustDecimal(t, "30"), mustDecimal(t, "20"), mustDecimal(t, "10"), LengthUnitCentimeter)
	if err != nil {
		t.Fatal(err)
	}
	factor := volumetricFactorForTest(t, LengthUnitInch)
	volumetric := pieceWeightPolicy(t, PricingWeightMax, WeightUnitKilogram, &factor)
	if _, err := pieceVolumetric(sides, volumetric, nil); err != ErrLengthUnitMismatch {
		t.Fatalf("尺寸 err = %v，want ErrLengthUnitMismatch", err)
	}
}

func TestPerPieceDoesNotConvertTheRoundingUnit(t *testing.T) {
	input := pieceInput(t, "2", WeightUnitKilogram,
		memberPiece(t, "pkg-a", "1", WeightUnitKilogram, nil),
		memberPiece(t, "pkg-b", "1", WeightUnitKilogram, nil))
	policy := pieceWeightPolicy(t, PricingWeightActualOnly, WeightUnitKilogram, nil)
	converter := newUnitConverter(false, nil)
	if _, err := calculatePerPiecePricingWeight(input, policy, ShipmentWeightSumThenRoundOnce, WeightUnitPound, converter); err != ErrWeightUnitMismatch || len(converter.recordedSteps()) != 0 {
		t.Fatalf("进位单位与查表单位不合 err = %v，换算 %d 条", err, len(converter.recordedSteps()))
	}
}

func TestPieceVolumetricUsesTheExistingConverter(t *testing.T) {
	sides, err := NewDimensions(mustDecimal(t, "30"), mustDecimal(t, "20"), mustDecimal(t, "10"), LengthUnitCentimeter)
	if err != nil {
		t.Fatal(err)
	}
	factor := volumetricFactorForTest(t, LengthUnitInch)
	policy := pieceWeightPolicy(t, PricingWeightMax, WeightUnitKilogram, &factor)
	converter := newUnitConverter(false, nil)
	got, err := pieceVolumetric(sides, policy, converter)
	if err != nil {
		t.Fatal(err)
	}
	direct := newUnitConverter(false, nil)
	want, err := direct.volumetricWeightFromDimensions(sides, factor)
	if err != nil || got.value.String() != want.value.String() || got.unit != want.unit {
		t.Fatalf("体积重 %s %s，直接换算 %s %s，err %v", got.value, got.unit, want.value, want.unit, err)
	}
	if len(converter.recordedSteps()) != 1 || converter.recordedSteps()[0].Quantity() != ConvertedDimensions {
		t.Fatalf("尺寸换算步骤：%v", converter.recordedSteps())
	}

	matching, err := NewDimensions(mustDecimal(t, "12"), mustDecimal(t, "10"), mustDecimal(t, "8"), LengthUnitInch)
	if err != nil {
		t.Fatal(err)
	}
	same := newUnitConverter(false, nil)
	if _, err := pieceVolumetric(matching, policy, same); err != nil || len(same.recordedSteps()) != 0 {
		t.Fatalf("单位一致 err = %v，步骤 %d", err, len(same.recordedSteps()))
	}
}

func TestPerPieceMissingDimensionsStaysMissingWhenUnitsDiffer(t *testing.T) {
	box, err := NewDimensions(mustDecimal(t, "10"), mustDecimal(t, "10"), mustDecimal(t, "10"), LengthUnitInch)
	if err != nil {
		t.Fatal(err)
	}
	factor := volumetricFactorForTest(t, LengthUnitCentimeter)
	policy := pieceWeightPolicy(t, PricingWeightMax, WeightUnitKilogram, &factor)
	input := pieceInput(t, "20", WeightUnitPound,
		memberPiece(t, "pkg-a", "10", WeightUnitPound, &box),
		memberPiece(t, "pkg-b", "10", WeightUnitPound, nil))
	converter := newUnitConverter(false, nil)
	if _, err := calculatePerPiecePricingWeight(input, policy, ShipmentWeightRoundEachThenSum, WeightUnitKilogram, converter); err != ErrMissingDimensions {
		t.Fatalf("err = %v，want ErrMissingDimensions", err)
	}
}
