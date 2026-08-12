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

func measurementOf(t *testing.T, raw, unit string) domain.DeclaredMeasurement {
	t.Helper()
	measurement, err := domain.NewDeclaredMeasurement(declaredWeight(t, raw, unit), domain.DeclaredDimensions{})
	if err != nil {
		t.Fatalf("new measurement: %v", err)
	}
	return measurement
}

func profileOf(t *testing.T, parcel, raw string) domain.DeclaredParcelProfile {
	t.Helper()
	profile, err := domain.NewDeclaredParcelProfile(
		mustValue(t, domain.NewDeclaredParcelID, parcel), measurementOf(t, raw, "KG"))
	if err != nil {
		t.Fatalf("new profile: %v", err)
	}
	return profile
}

// Covers: ADR-0048 第二增量——画像随首个提交版本出生并按成员可取；部分成员无画像合法
// （测量必填由真实产品定）；指着集合外成员与一员两张在构造期拒收，估价装配才不会拿到
// 另一个包裹的测量。
func TestAVersionCarriesItsDeclaredProfilesByMember(t *testing.T) {
	spec := submitSpec(t, "parcel-1", "parcel-2")
	spec.Profiles = []domain.DeclaredParcelProfile{profileOf(t, "parcel-1", "1.50")}

	request, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	version := request.CurrentSubmissionVersion()
	profile, present := version.ProfileFor(mustValue(t, domain.NewDeclaredParcelID, "parcel-1"))
	if !present || profile.Measurement().Weight().Value().String() != "1.50" {
		t.Fatalf("profile = %#v present = %v; 画像没有随版本保全", profile, present)
	}
	if _, present := version.ProfileFor(mustValue(t, domain.NewDeclaredParcelID, "parcel-2")); present {
		t.Fatal("没申报的成员凭空长出了画像")
	}

	outside := submitSpec(t, "parcel-1")
	outside.Profiles = []domain.DeclaredParcelProfile{profileOf(t, "parcel-9", "1")}
	if _, err := domain.SubmitShipmentRequest(outside); !errors.Is(err, domain.ErrInvalidDeclaredMeasurement) {
		t.Fatalf("err = %v; 指着集合外成员的画像被收下了", err)
	}

	duplicated := submitSpec(t, "parcel-1")
	duplicated.Profiles = []domain.DeclaredParcelProfile{profileOf(t, "parcel-1", "1"), profileOf(t, "parcel-1", "2")}
	if _, err := domain.SubmitShipmentRequest(duplicated); !errors.Is(err, domain.ErrInvalidDeclaredMeasurement) {
		t.Fatalf("err = %v; 一员两张矛盾测量被收下了", err)
	}
}

// Covers: ADR-0048「画像随本版本重报，不从旧版本静默继承」——受控纠错改测量时新版本
// 是新申报，旧版本的画像留在历史里原样可读（旧版本及其判断历史继续保留）。
func TestASupersedingVersionRedeclaresItsProfiles(t *testing.T) {
	spec := submitSpec(t, "parcel-1", "parcel-2")
	spec.Profiles = []domain.DeclaredParcelProfile{profileOf(t, "parcel-1", "1.50")}
	request, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	superseded, err := request.FormNewSubmissionVersion(domain.NewSubmissionVersionSpec{
		VersionID:         mustValue(t, domain.NewSubmissionVersionID, "version-2"),
		TaskID:            mustValue(t, domain.NewAcceptanceDecisionTaskID, "task-2"),
		SourceSubmission:  sourceFingerprint(t, "tenant-1", "customer-1", "source-a", "key-2", "digest-2"),
		DeclaredParcelIDs: request.CurrentSubmissionVersion().DeclaredParcelIDs(),
		Profiles:          []domain.DeclaredParcelProfile{profileOf(t, "parcel-1", "2.00")},
		EstablishedAt:     request.SubmittedAt().Add(1),
	})
	if err != nil {
		t.Fatalf("form new submission version: %v", err)
	}

	current, _ := superseded.CurrentSubmissionVersion().ProfileFor(mustValue(t, domain.NewDeclaredParcelID, "parcel-1"))
	if current.Measurement().Weight().Value().String() != "2.00" {
		t.Fatalf("current = %s, want the redeclared 2.00", current.Measurement().Weight().Value())
	}
	prior := superseded.PriorSubmissionVersions()
	if len(prior) != 1 {
		t.Fatalf("prior versions = %d, want 1", len(prior))
	}
	kept, present := prior[0].ProfileFor(mustValue(t, domain.NewDeclaredParcelID, "parcel-1"))
	if !present || kept.Measurement().Weight().Value().String() != "1.50" {
		t.Fatalf("prior profile = %#v present = %v; 旧版本的申报被改写了", kept, present)
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
