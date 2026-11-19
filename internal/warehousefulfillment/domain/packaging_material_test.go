package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 本文件证包材这一册的领域面（UC-WF-001 的包材修订）：编码、名称与单位码逐字保存、一笔修订是一份完整内容、数值按数值比而单位按码逐字比，
// 以及内容相同的比较只比内容、不比登记责任方与登记时刻。

func packagingMaterialSpec(t *testing.T) domain.PackagingMaterialContentSpec {
	t.Helper()
	return domain.PackagingMaterialContentSpec{
		Name:           valueOf(t, domain.NewPackagingMaterialName, "合成一号纸箱"),
		Length:         measureOf(t, "30"),
		Width:          measureOf(t, "20"),
		Height:         measureOf(t, "15.5"),
		DimensionsUnit: valueOf(t, domain.NewUnitCode, "CM"),
		Weight:         measureOf(t, "0.25"),
		WeightUnit:     valueOf(t, domain.NewUnitCode, "KG"),
	}
}

func packagingMaterialContentOf(t *testing.T, edit func(*domain.PackagingMaterialContentSpec)) domain.PackagingMaterialContent {
	t.Helper()
	spec := packagingMaterialSpec(t)
	if edit != nil {
		edit(&spec)
	}
	content, err := domain.NewPackagingMaterialContent(spec)
	if err != nil {
		t.Fatalf("NewPackagingMaterialContent: %v", err)
	}
	return content
}

func packagingMaterialRevisionOf(t *testing.T, code string, number int, content domain.PackagingMaterialContent) domain.PackagingMaterialRevision {
	t.Helper()
	revision, err := domain.NewPackagingMaterialRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewPackagingMaterialCode, code),
		number, content,
		valueOf(t, domain.NewRegistrant, "operator-1"),
		registeredAt,
	)
	if err != nil {
		t.Fatalf("NewPackagingMaterialRevision: %v", err)
	}
	return revision
}

// Covers: AT-WF-001（包材的部分）「编码逐字保存」——包材编码、名称与单位码带首尾空白、内部连续空白与大小写混写原样留下（单位码 `cm` 不是 `CM`，
// 折大小写是代登记方改单位），只差大小写或空白的编码是两个身份；空白串与存不下的串（NUL、非 UTF-8）在构造处拒，免得到库里才以技术失败露面。
func TestPackagingMaterialCodeNameAndUnitKeepEveryByteAndRefuseBlankOrUnstorable(t *testing.T) {
	constructors := map[string]func(string) (string, error){
		"包材编码": func(raw string) (string, error) {
			value, err := domain.NewPackagingMaterialCode(raw)
			return value.String(), err
		},
		"包材名称": func(raw string) (string, error) {
			value, err := domain.NewPackagingMaterialName(raw)
			return value.String(), err
		},
		"单位码": func(raw string) (string, error) {
			value, err := domain.NewUnitCode(raw)
			return value.String(), err
		},
	}
	for name, construct := range constructors {
		for _, raw := range []string{"  Box\t01  ", "a  b", "box-1", "BOX-1", "cm", "\u00a0nbsp\u00a0"} {
			got, err := construct(raw)
			if err != nil || got != raw {
				t.Errorf("%s 构造 %q = (%q, %v)，want 原样留下", name, raw, got, err)
			}
		}
		for _, raw := range []string{"", " ", "\t\n  "} {
			if _, err := construct(raw); !errors.Is(err, domain.ErrBlankValue) {
				t.Errorf("%s 构造 %q 的错误 = %v，want ErrBlankValue", name, raw, err)
			}
		}
		for _, raw := range []string{"a\x00b", "\xff\xfe"} {
			if _, err := construct(raw); !errors.Is(err, domain.ErrUnstorableValue) {
				t.Errorf("%s 构造 %q 的错误 = %v，want ErrUnstorableValue", name, raw, err)
			}
		}
	}
	upper := valueOf(t, domain.NewPackagingMaterialCode, "BOX-1")
	lower := valueOf(t, domain.NewPackagingMaterialCode, "box-1")
	padded := valueOf(t, domain.NewPackagingMaterialCode, " BOX-1")
	if upper == lower || upper == padded || lower == padded {
		t.Fatalf("只差大小写或空白的包材编码被折成了同一个：%q %q %q", upper, lower, padded)
	}
	if valueOf(t, domain.NewUnitCode, "cm") == valueOf(t, domain.NewUnitCode, "CM") {
		t.Fatalf("单位码 cm 与 CM 被折成了同一个")
	}
}

// Covers: 内容是一份完整内容——缺名称、任一数值或任一单位的构造不成，并指出缺的是哪一格（未经构造器的零值也算缺，没经构造器的数值不是零）；
// 各格都给齐、数值为零也构造得出来：「为正」是登记门，不在构造器里。是否停用缺省就是不停用。
func TestPackagingMaterialContentRefusesAMissingCellAndNamesIt(t *testing.T) {
	for name, test := range map[string]struct {
		edit func(*domain.PackagingMaterialContentSpec)
		want domain.PackagingMaterialField
	}{
		"缺名称":   {func(spec *domain.PackagingMaterialContentSpec) { spec.Name = domain.PackagingMaterialName{} }, domain.PackagingMaterialFieldName},
		"缺长":    {func(spec *domain.PackagingMaterialContentSpec) { spec.Length = domain.Measure{} }, domain.PackagingMaterialFieldDimensionsLength},
		"缺宽":    {func(spec *domain.PackagingMaterialContentSpec) { spec.Width = domain.Measure{} }, domain.PackagingMaterialFieldDimensionsWidth},
		"缺高":    {func(spec *domain.PackagingMaterialContentSpec) { spec.Height = domain.Measure{} }, domain.PackagingMaterialFieldDimensionsHeight},
		"缺尺寸单位": {func(spec *domain.PackagingMaterialContentSpec) { spec.DimensionsUnit = domain.UnitCode{} }, domain.PackagingMaterialFieldDimensionsUnit},
		"缺重量":   {func(spec *domain.PackagingMaterialContentSpec) { spec.Weight = domain.Measure{} }, domain.PackagingMaterialFieldWeightValue},
		"缺重量单位": {func(spec *domain.PackagingMaterialContentSpec) { spec.WeightUnit = domain.UnitCode{} }, domain.PackagingMaterialFieldWeightUnit},
	} {
		spec := packagingMaterialSpec(t)
		test.edit(&spec)
		_, err := domain.NewPackagingMaterialContent(spec)
		var invalid *domain.PackagingMaterialContentFieldError
		if !errors.Is(err, domain.ErrInvalidPackagingMaterialContent) || !errors.As(err, &invalid) || invalid.Field != test.want {
			t.Errorf("%s：err = %v，want ErrInvalidPackagingMaterialContent 指向 %s", name, err, test.want)
		}
	}

	zero := packagingMaterialContentOf(t, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Length, spec.Weight = measureOf(t, "0"), measureOf(t, "0.0")
	})
	if !zero.Length().IsZero() || !zero.Weight().IsZero() {
		t.Errorf("数值为零的内容该构造得出来，由登记门拦")
	}
	if packagingMaterialContentOf(t, nil).Deactivated() {
		t.Fatalf("没给是否停用却成了停用")
	}
}

// 各格的字面量是应用层结果里「指出哪一格」的取值，外半据它译装；改字面量等于改了对外的词。键路径与登记报文里的写法同一套。
func TestPackagingMaterialFieldLiterals(t *testing.T) {
	for field, literal := range map[domain.PackagingMaterialField]string{
		domain.PackagingMaterialFieldName:             "name",
		domain.PackagingMaterialFieldDimensionsLength: "dimensions.length",
		domain.PackagingMaterialFieldDimensionsWidth:  "dimensions.width",
		domain.PackagingMaterialFieldDimensionsHeight: "dimensions.height",
		domain.PackagingMaterialFieldDimensionsUnit:   "dimensions.unit",
		domain.PackagingMaterialFieldWeightValue:      "weight.value",
		domain.PackagingMaterialFieldWeightUnit:       "weight.unit",
	} {
		if field.String() != literal {
			t.Errorf("字面量 = %q，want %q", field.String(), literal)
		}
	}
	if domain.PackagingMaterialFieldInvalid.String() != "" {
		t.Errorf("零值的字面量 = %q，want 空串", domain.PackagingMaterialFieldInvalid.String())
	}
}

func TestPackagingMaterialRevisionRefusesNonPositiveNumbersAndUnconstructedParts(t *testing.T) {
	content := packagingMaterialContentOf(t, nil)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	code := valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-01")
	registrant := valueOf(t, domain.NewRegistrant, "operator-1")

	for _, number := range []int{0, -1} {
		if _, err := domain.NewPackagingMaterialRevision(tenant, code, number, content, registrant, registeredAt); !errors.Is(err, domain.ErrInvalidRevisionNumber) {
			t.Errorf("修订号 %d 的错误 = %v，want ErrInvalidRevisionNumber", number, err)
		}
	}
	for name, build := range map[string]func() (domain.PackagingMaterialRevision, error){
		"缺租户": func() (domain.PackagingMaterialRevision, error) {
			return domain.NewPackagingMaterialRevision(domain.TenantID{}, code, 1, content, registrant, registeredAt)
		},
		"缺包材编码": func() (domain.PackagingMaterialRevision, error) {
			return domain.NewPackagingMaterialRevision(tenant, domain.PackagingMaterialCode{}, 1, content, registrant, registeredAt)
		},
		"缺登记责任方": func() (domain.PackagingMaterialRevision, error) {
			return domain.NewPackagingMaterialRevision(tenant, code, 1, content, domain.Registrant{}, registeredAt)
		},
		"缺登记时刻": func() (domain.PackagingMaterialRevision, error) {
			return domain.NewPackagingMaterialRevision(tenant, code, 1, content, registrant, time.Time{})
		},
	} {
		if _, err := build(); !errors.Is(err, domain.ErrInvalidPackagingMaterialRevision) {
			t.Errorf("%s：err = %v，want ErrInvalidPackagingMaterialRevision", name, err)
		}
	}
	var invalid *domain.PackagingMaterialContentFieldError
	if _, err := domain.NewPackagingMaterialRevision(tenant, code, 1, domain.PackagingMaterialContent{}, registrant, registeredAt); !errors.As(err, &invalid) {
		t.Errorf("未经构造器的零值内容的错误 = %v，want 指出缺哪一格的 PackagingMaterialContentFieldError", err)
	}
}

// 登记时刻收成库里存得下的精度（微秒）并统一到 UTC：不收的话，回显的时刻与读回的差在纳秒上。
func TestPackagingMaterialRevisionKeepsTheRegistrationInstantAtStorablePrecision(t *testing.T) {
	shanghai := time.FixedZone("UTC+8", 8*3600)
	at := time.Date(2026, 11, 18, 17, 30, 0, 123456789, shanghai)
	revision, err := domain.NewPackagingMaterialRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-01"),
		1, packagingMaterialContentOf(t, nil), valueOf(t, domain.NewRegistrant, "operator-1"), at)
	if err != nil {
		t.Fatalf("NewPackagingMaterialRevision: %v", err)
	}
	want := time.Date(2026, 11, 18, 9, 30, 0, 123456000, time.UTC)
	if got := revision.RegisteredAt(); !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("登记时刻 = %v，want %v（UTC，微秒）", got, want)
	}
}

// Covers: AT-WF-001「原样重投答已登记；同一修订号换内容答内容冲突」所依的比较（包材的部分）——只比完整内容（租户、编码、修订号、名称、四个数值、
// 两个单位、是否停用），不比登记责任方与登记时刻。数值按数值比：12.5 与 12.50、015.5 与 15.5 是同一内容；单位按码逐字比，登记时不折算：
// 30 CM 与 0.3 M 是两份不同的内容。
func TestPackagingMaterialSameContentComparesNumbersAsNumbersAndUnitsByCode(t *testing.T) {
	base := packagingMaterialRevisionOf(t, "SYN-BOX-01", 1, packagingMaterialContentOf(t, nil))

	otherRegistrantAndInstant, err := domain.NewPackagingMaterialRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-01"),
		1, packagingMaterialContentOf(t, nil),
		valueOf(t, domain.NewRegistrant, "operator-2"),
		registeredAt.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("NewPackagingMaterialRevision: %v", err)
	}
	if !base.SameContentAs(otherRegistrantAndInstant) || !otherRegistrantAndInstant.SameContentAs(base) {
		t.Fatalf("换了登记责任方与登记时刻的同一份内容被判成了不同")
	}

	rewritten := packagingMaterialRevisionOf(t, "SYN-BOX-01", 1, packagingMaterialContentOf(t, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Length, spec.Width, spec.Height = measureOf(t, "30.00"), measureOf(t, "020"), measureOf(t, "015.50")
		spec.Weight = measureOf(t, "0.250000")
	}))
	if !base.SameContentAs(rewritten) || !rewritten.SameContentAs(base) {
		t.Fatalf("只换了数值的写法，却被判成了不同的内容")
	}

	changed := map[string]domain.PackagingMaterialRevision{
		"修订号": packagingMaterialRevisionOf(t, "SYN-BOX-01", 2, packagingMaterialContentOf(t, nil)),
		"名称": packagingMaterialRevisionOf(t, "SYN-BOX-01", 1, packagingMaterialContentOf(t, func(spec *domain.PackagingMaterialContentSpec) {
			spec.Name = valueOf(t, domain.NewPackagingMaterialName, "合成一号纸箱 ")
		})),
		"长": packagingMaterialRevisionOf(t, "SYN-BOX-01", 1, packagingMaterialContentOf(t, func(spec *domain.PackagingMaterialContentSpec) {
			spec.Length = measureOf(t, "30.000001")
		})),
		"宽": packagingMaterialRevisionOf(t, "SYN-BOX-01", 1, packagingMaterialContentOf(t, func(spec *domain.PackagingMaterialContentSpec) {
			spec.Width = measureOf(t, "2")
		})),
		"高": packagingMaterialRevisionOf(t, "SYN-BOX-01", 1, packagingMaterialContentOf(t, func(spec *domain.PackagingMaterialContentSpec) {
			spec.Height = measureOf(t, "155")
		})),
		"重量": packagingMaterialRevisionOf(t, "SYN-BOX-01", 1, packagingMaterialContentOf(t, func(spec *domain.PackagingMaterialContentSpec) {
			spec.Weight = measureOf(t, "0.26")
		})),
		"尺寸单位（30 CM 与 30 M）": packagingMaterialRevisionOf(t, "SYN-BOX-01", 1, packagingMaterialContentOf(t, func(spec *domain.PackagingMaterialContentSpec) {
			spec.DimensionsUnit = valueOf(t, domain.NewUnitCode, "M")
		})),
		"尺寸单位的大小写": packagingMaterialRevisionOf(t, "SYN-BOX-01", 1, packagingMaterialContentOf(t, func(spec *domain.PackagingMaterialContentSpec) {
			spec.DimensionsUnit = valueOf(t, domain.NewUnitCode, "cm")
		})),
		"重量单位": packagingMaterialRevisionOf(t, "SYN-BOX-01", 1, packagingMaterialContentOf(t, func(spec *domain.PackagingMaterialContentSpec) {
			spec.WeightUnit = valueOf(t, domain.NewUnitCode, "G")
		})),
		"停用": packagingMaterialRevisionOf(t, "SYN-BOX-01", 1, packagingMaterialContentOf(t, func(spec *domain.PackagingMaterialContentSpec) {
			spec.Deactivated = true
		})),
		"编码只差大小写": packagingMaterialRevisionOf(t, "syn-box-01", 1, packagingMaterialContentOf(t, nil)),
	}
	for cell, other := range changed {
		if base.SameContentAs(other) || other.SameContentAs(base) {
			t.Errorf("只改了%s，却被判成内容相同", cell)
		}
	}

	// 登记时不折算单位：30 CM 与 0.3 M 量的是同一个长度，登记里仍是两份内容。
	converted := packagingMaterialRevisionOf(t, "SYN-BOX-01", 1, packagingMaterialContentOf(t, func(spec *domain.PackagingMaterialContentSpec) {
		spec.Length, spec.Width, spec.Height = measureOf(t, "0.3"), measureOf(t, "0.2"), measureOf(t, "0.155")
		spec.DimensionsUnit = valueOf(t, domain.NewUnitCode, "M")
	}))
	if base.SameContentAs(converted) {
		t.Errorf("30 CM 与 0.3 M 被折算成了同一份内容，登记时不折算单位")
	}

	otherTenant, err := domain.NewPackagingMaterialRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-02"),
		valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-01"),
		1, packagingMaterialContentOf(t, nil), valueOf(t, domain.NewRegistrant, "operator-1"), registeredAt)
	if err != nil {
		t.Fatalf("NewPackagingMaterialRevision: %v", err)
	}
	if base.SameContentAs(otherTenant) {
		t.Errorf("另一个租户的同编码包材被判成内容相同")
	}
}
