package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func declaredWeight(t *testing.T, raw, unit string) domain.DeclaredWeight {
	t.Helper()
	weight, err := domain.NewDeclaredWeight(
		mustValue(t, domain.NewMeasurementValue, raw),
		mustValue(t, domain.NewMeasurementUnitReference, unit),
	)
	if err != nil {
		t.Fatalf("new declared weight: %v", err)
	}
	return weight
}

// Covers: UC-PS-001 声明包裹输入组「客户声明成员及各自……声明测量」与 ADR-0048 的保真
// 纪律——数字与单位原样保全（"1.50" 不被规范化成 "1.5"），换算与计价重量归
// parcel-pricing，本上下文只回答客户报了什么。
func TestADeclaredMeasurementKeepsTheCustomersWordsVerbatim(t *testing.T) {
	weight := declaredWeight(t, "1.50", "KG")
	dimensions, err := domain.NewDeclaredDimensions(
		mustValue(t, domain.NewMeasurementValue, "30"),
		mustValue(t, domain.NewMeasurementValue, "20"),
		mustValue(t, domain.NewMeasurementValue, "10.5"),
		mustValue(t, domain.NewMeasurementUnitReference, "CM"),
	)
	if err != nil {
		t.Fatalf("new declared dimensions: %v", err)
	}
	measurement, err := domain.NewDeclaredMeasurement(weight, dimensions)
	if err != nil {
		t.Fatalf("new declared measurement: %v", err)
	}

	if measurement.Weight().Value().String() != "1.50" || measurement.Weight().Unit().String() != "KG" {
		t.Fatalf("weight = %s %s; 客户的数字被改写了", measurement.Weight().Value(), measurement.Weight().Unit())
	}
	declared, present := measurement.Dimensions()
	if !present || declared.Height().String() != "10.5" {
		t.Fatalf("dimensions = %#v present = %v; 外廓没有原样保全", declared, present)
	}

	profile, err := domain.NewDeclaredParcelProfile(
		mustValue(t, domain.NewDeclaredParcelID, "parcel-1"), measurement)
	if err != nil {
		t.Fatalf("new declared parcel profile: %v", err)
	}
	if profile.Parcel().String() != "parcel-1" {
		t.Fatalf("profile parcel = %s", profile.Parcel())
	}
}

// Covers: ADR-0048「外廓可缺席，毛重必备」——小包申报常只报重量，缺席是真话由读取方
// 处置；没有重量的测量装配不出任何估价输入，构造即死。
func TestDimensionsMayBeAbsentButWeightMayNot(t *testing.T) {
	measurement, err := domain.NewDeclaredMeasurement(declaredWeight(t, "2", "KG"), domain.DeclaredDimensions{})
	if err != nil {
		t.Fatalf("new weight-only measurement: %v", err)
	}
	if _, present := measurement.Dimensions(); present {
		t.Fatal("没申报外廓却报告在场")
	}

	if _, err := domain.NewDeclaredMeasurement(domain.DeclaredWeight{}, domain.DeclaredDimensions{}); !errors.Is(err, domain.ErrInvalidDeclaredMeasurement) {
		t.Fatalf("err = %v, want ErrInvalidDeclaredMeasurement", err)
	}
}

// Covers: 保真不等于什么都收——收下一个读不出数的字符串，译计价输入时才炸。形状（十进制、
// 正数）在构造期判定，花样格式与零值一律拒；半截外廓不是一个外廓。
func TestAMeasurementValueRefusesShapesThatCannotBeRead(t *testing.T) {
	for _, raw := range []string{"", "  ", "abc", "1,5", "1.2.3", "-1", "+1", "1e3", "0", "0.00", ".5", "5."} {
		t.Run("value "+raw, func(t *testing.T) {
			if _, err := domain.NewMeasurementValue(raw); !errors.Is(err, domain.ErrInvalidDeclaredMeasurement) {
				t.Fatalf("raw = %q err = %v, want ErrInvalidDeclaredMeasurement", raw, err)
			}
		})
	}

	if _, err := domain.NewDeclaredWeight(
		mustValue(t, domain.NewMeasurementValue, "1.5"), domain.MeasurementUnitReference{},
	); !errors.Is(err, domain.ErrInvalidDeclaredMeasurement) {
		t.Fatalf("err = %v; 没有单位的数字读不出重量", err)
	}
	if _, err := domain.NewDeclaredDimensions(
		mustValue(t, domain.NewMeasurementValue, "30"),
		domain.MeasurementValue{},
		mustValue(t, domain.NewMeasurementValue, "10"),
		mustValue(t, domain.NewMeasurementUnitReference, "CM"),
	); !errors.Is(err, domain.ErrInvalidDeclaredMeasurement) {
		t.Fatalf("err = %v; 两边加一个洞不是一个外廓", err)
	}
	if _, err := domain.NewDeclaredParcelProfile(domain.DeclaredParcelID{}, domain.DeclaredMeasurement{}); !errors.Is(err, domain.ErrInvalidDeclaredMeasurement) {
		t.Fatalf("err = %v; 半截画像立不起来", err)
	}
}
