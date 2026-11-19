package domain_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 本文件证库位这一册的领域面（UC-WF-001 的库位修订）：库位编码逐字保存、库位类型与优先级「没有」与「有」是不同的内容、一笔修订是
// 一份完整内容、「新指名」的判法，以及内容相同的比较只比内容、不比登记责任方与登记时刻。

func locationContentSpec(t *testing.T) domain.LocationContentSpec {
	t.Helper()
	return domain.LocationContentSpec{ZoneCode: valueOf(t, domain.NewZoneCode, "SYN-ZONE-01")}
}

func locationContentOf(t *testing.T, edit func(*domain.LocationContentSpec)) domain.LocationContent {
	t.Helper()
	spec := locationContentSpec(t)
	if edit != nil {
		edit(&spec)
	}
	content, err := domain.NewLocationContent(spec)
	if err != nil {
		t.Fatalf("NewLocationContent: %v", err)
	}
	return content
}

func locationRevisionOf(t *testing.T, number int, content domain.LocationContent) domain.LocationRevision {
	t.Helper()
	revision, err := domain.NewLocationRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		valueOf(t, domain.NewLocationCode, "SYN-LOC-01"),
		number, content,
		valueOf(t, domain.NewRegistrant, "operator-1"),
		registeredAt,
	)
	if err != nil {
		t.Fatalf("NewLocationRevision: %v", err)
	}
	return revision
}

// Covers: AT-WF-001（库位的部分）「库位编码逐字保存」——带首尾空白、内部连续空白与大小写混写的库位编码原样留下，只差大小写或空白的
// 编码是两个身份；空白串与存不下的串（NUL、非 UTF-8）在构造处拒，免得到库里才以技术失败露面。
func TestLocationCodeKeepsEveryByteAndRefusesBlankOrUnstorable(t *testing.T) {
	for _, raw := range []string{"  Loc-Mixed\t01  ", "a  b", "loc-1", "LOC-1", "\u00a0nbsp\u00a0"} {
		got, err := domain.NewLocationCode(raw)
		if err != nil || got.String() != raw {
			t.Errorf("构造 %q = (%q, %v)，want 原样留下", raw, got.String(), err)
		}
	}
	for _, raw := range []string{"", " ", "\t\n  "} {
		if _, err := domain.NewLocationCode(raw); !errors.Is(err, domain.ErrBlankValue) {
			t.Errorf("构造 %q 的错误 = %v，want ErrBlankValue", raw, err)
		}
	}
	for _, raw := range []string{"a\x00b", "\xff\xfe"} {
		if _, err := domain.NewLocationCode(raw); !errors.Is(err, domain.ErrUnstorableValue) {
			t.Errorf("构造 %q 的错误 = %v，want ErrUnstorableValue", raw, err)
		}
	}
	upper := valueOf(t, domain.NewLocationCode, "LOC-1")
	lower := valueOf(t, domain.NewLocationCode, "loc-1")
	padded := valueOf(t, domain.NewLocationCode, " LOC-1")
	if upper == lower || upper == padded || lower == padded {
		t.Fatalf("只差大小写或空白的库位编码被折成了同一个：%q %q %q", upper, lower, padded)
	}
}

// Covers: 票面「库位」优先级——优先级可缺：不给就是没有，不取任何默认值，也不当成 0。零值是「没给」，给 0 是「给了，是 0」，两者的
// 取值方法分得开，可负，32 位有符号整数的两端都装得下。
func TestLocationPriorityDistinguishesNotGivenFromZero(t *testing.T) {
	var absent domain.LocationPriority
	if absent.Given() || absent.Value() != 0 {
		t.Fatalf("零值 = (%v, %d)，want 没给", absent.Given(), absent.Value())
	}
	for _, value := range []int32{0, 1, -1, math.MaxInt32, math.MinInt32} {
		given := domain.NewLocationPriority(value)
		if !given.Given() || given.Value() != value {
			t.Errorf("NewLocationPriority(%d) = (%v, %d)，want 给了、取值原样", value, given.Given(), given.Value())
		}
	}
	if absent == domain.NewLocationPriority(0) {
		t.Fatalf("「没给」与「给了 0」被判成同一个值")
	}
}

// Covers: AT-WF-001（库位的部分）所依的内容——所属库区必填；库位类型与优先级可缺；是否停用缺省就是不停用。缺所属库区的构造不成，并
// 指出缺的是哪一格；带与不带库位类型、带与不带优先级各是不同的内容。
func TestLocationContentRequiresTheZoneAndKeepsOptionalCellsDistinct(t *testing.T) {
	spec := locationContentSpec(t)
	spec.ZoneCode = domain.ZoneCode{}
	_, err := domain.NewLocationContent(spec)
	var invalid *domain.LocationContentFieldError
	if !errors.Is(err, domain.ErrInvalidLocationContent) || !errors.As(err, &invalid) || invalid.Field != domain.LocationFieldZoneCode {
		t.Fatalf("缺所属库区：err = %v，want ErrInvalidLocationContent 指向 zoneCode", err)
	}

	bare := locationContentOf(t, nil)
	if _, has := bare.LocationType(); has || bare.Priority().Given() || bare.Deactivated() {
		t.Fatalf("只给所属库区的内容 = 类型 %v、优先级 %v、停用 %v，want 都没有、不停用", has, bare.Priority().Given(), bare.Deactivated())
	}
	typed := locationContentOf(t, func(spec *domain.LocationContentSpec) {
		spec.LocationType = valueOf(t, domain.NewLocationTypeCode, "SYN-LT-01")
	})
	if code, has := typed.LocationType(); !has || code.String() != "SYN-LT-01" {
		t.Fatalf("带库位类型的内容 = (%q, %v)", code.String(), has)
	}
	prioritised := locationContentOf(t, func(spec *domain.LocationContentSpec) { spec.Priority = domain.NewLocationPriority(0) })
	negative := locationContentOf(t, func(spec *domain.LocationContentSpec) { spec.Priority = domain.NewLocationPriority(-5) })
	if bare == typed || bare == prioritised || prioritised == negative || typed == prioritised {
		t.Fatalf("带与不带库位类型、带与不带优先级（含 0）被判成了同一份内容")
	}
	if prioritised.Priority().Value() != 0 || !prioritised.Priority().Given() || negative.Priority().Value() != -5 {
		t.Fatalf("优先级读回 = %d / %d", prioritised.Priority().Value(), negative.Priority().Value())
	}
}

// Covers: 票面「库位」与 AT-WF-003（库位的部分）「新指名」——这是首笔修订、或在册最新那一笔没带库位类型、或带的是另一个类型编码，才算
// 新指名；沿用在册最新那一笔里的同一个类型编码不算；这一笔没带类型就没有指名。
func TestLocationContentNamesALocationTypeNewlyOnlyWhenItDiffersFromTheLatest(t *testing.T) {
	withType := func(code string) domain.LocationContent {
		return locationContentOf(t, func(spec *domain.LocationContentSpec) {
			spec.LocationType = valueOf(t, domain.NewLocationTypeCode, code)
		})
	}
	bare := locationContentOf(t, nil)

	for name, test := range map[string]struct {
		candidate domain.LocationContent
		latest    domain.LocationContent
		hasLatest bool
		want      bool
	}{
		"首笔修订带类型":      {candidate: withType("SYN-LT-01"), want: true},
		"上一笔没带类型这一笔带上": {candidate: withType("SYN-LT-01"), latest: bare, hasLatest: true, want: true},
		"换成另一个类型":      {candidate: withType("SYN-LT-02"), latest: withType("SYN-LT-01"), hasLatest: true, want: true},
		"类型编码只差大小写":    {candidate: withType("syn-lt-01"), latest: withType("SYN-LT-01"), hasLatest: true, want: true},
		"沿用同一个类型":      {candidate: withType("SYN-LT-01"), latest: withType("SYN-LT-01"), hasLatest: true, want: false},
		"这一笔没带类型（首笔）":  {candidate: bare, want: false},
		"这一笔去掉了类型":     {candidate: bare, latest: withType("SYN-LT-01"), hasLatest: true, want: false},
		"两笔都没带类型":      {candidate: bare, latest: bare, hasLatest: true, want: false},
	} {
		if got := test.candidate.NamesLocationTypeNewly(test.latest, test.hasLatest); got != test.want {
			t.Errorf("%s：NamesLocationTypeNewly = %v，want %v", name, got, test.want)
		}
	}
}

// 各格的字面量是应用层结果里「指出哪一格」的取值，外半据它译装；改字面量等于改了对外的词。
func TestLocationFieldLiterals(t *testing.T) {
	for field, literal := range map[domain.LocationField]string{
		domain.LocationFieldWarehouseCode:    "warehouseCode",
		domain.LocationFieldZoneCode:         "zoneCode",
		domain.LocationFieldLocationTypeCode: "locationTypeCode",
	} {
		if field.String() != literal {
			t.Errorf("字面量 = %q，want %q", field.String(), literal)
		}
	}
	if domain.LocationFieldInvalid.String() != "" {
		t.Errorf("零值的字面量 = %q，want 空串", domain.LocationFieldInvalid.String())
	}
}

func TestLocationRevisionRefusesNonPositiveNumbersAndUnconstructedParts(t *testing.T) {
	content := locationContentOf(t, nil)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	code := valueOf(t, domain.NewLocationCode, "SYN-LOC-01")
	registrant := valueOf(t, domain.NewRegistrant, "operator-1")

	for _, number := range []int{0, -1} {
		if _, err := domain.NewLocationRevision(tenant, warehouse, code, number, content, registrant, registeredAt); !errors.Is(err, domain.ErrInvalidRevisionNumber) {
			t.Errorf("修订号 %d 的错误 = %v，want ErrInvalidRevisionNumber", number, err)
		}
	}
	for name, build := range map[string]func() (domain.LocationRevision, error){
		"缺租户": func() (domain.LocationRevision, error) {
			return domain.NewLocationRevision(domain.TenantID{}, warehouse, code, 1, content, registrant, registeredAt)
		},
		"缺仓编码": func() (domain.LocationRevision, error) {
			return domain.NewLocationRevision(tenant, domain.WarehouseCode{}, code, 1, content, registrant, registeredAt)
		},
		"缺库位编码": func() (domain.LocationRevision, error) {
			return domain.NewLocationRevision(tenant, warehouse, domain.LocationCode{}, 1, content, registrant, registeredAt)
		},
		"缺登记责任方": func() (domain.LocationRevision, error) {
			return domain.NewLocationRevision(tenant, warehouse, code, 1, content, domain.Registrant{}, registeredAt)
		},
		"缺登记时刻": func() (domain.LocationRevision, error) {
			return domain.NewLocationRevision(tenant, warehouse, code, 1, content, registrant, time.Time{})
		},
	} {
		if _, err := build(); !errors.Is(err, domain.ErrInvalidLocationRevision) {
			t.Errorf("%s：err = %v，want ErrInvalidLocationRevision", name, err)
		}
	}
	var invalid *domain.LocationContentFieldError
	if _, err := domain.NewLocationRevision(tenant, warehouse, code, 1, domain.LocationContent{}, registrant, registeredAt); !errors.As(err, &invalid) ||
		invalid.Field != domain.LocationFieldZoneCode {
		t.Errorf("未经构造器的零值内容的错误 = %v，want 指出缺所属库区的 LocationContentFieldError", err)
	}
}

// 登记时刻收成库里存得下的精度（微秒）并统一到 UTC：不收的话，回显的时刻与读回的差在纳秒上。
func TestLocationRevisionKeepsTheRegistrationInstantAtStorablePrecision(t *testing.T) {
	shanghai := time.FixedZone("UTC+8", 8*3600)
	at := time.Date(2026, 11, 18, 17, 30, 0, 123456789, shanghai)
	revision, err := domain.NewLocationRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		valueOf(t, domain.NewLocationCode, "SYN-LOC-01"),
		1, locationContentOf(t, nil), valueOf(t, domain.NewRegistrant, "operator-1"), at)
	if err != nil {
		t.Fatalf("NewLocationRevision: %v", err)
	}
	want := time.Date(2026, 11, 18, 9, 30, 0, 123456000, time.UTC)
	if got := revision.RegisteredAt(); !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("登记时刻 = %v，want %v（UTC，微秒）", got, want)
	}
}

// Covers: AT-WF-001「原样重投答已登记；同一修订号换内容答内容冲突」所依的比较（库位的部分）——只比完整内容（租户、仓编码、库位编码、
// 修订号、所属库区、库位类型、优先级、是否停用），不比登记责任方与登记时刻。带与不带库位类型、带与不带优先级（含 0）各是不同的内容。
// 同一库位编码在另一个仓下是另一个库位。
func TestLocationSameContentComparesEveryContentCellAndNothingElse(t *testing.T) {
	base := locationRevisionOf(t, 1, locationContentOf(t, nil))

	otherRegistrantAndInstant, err := domain.NewLocationRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		valueOf(t, domain.NewLocationCode, "SYN-LOC-01"),
		1, locationContentOf(t, nil),
		valueOf(t, domain.NewRegistrant, "operator-2"),
		registeredAt.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("NewLocationRevision: %v", err)
	}
	if !base.SameContentAs(otherRegistrantAndInstant) || !otherRegistrantAndInstant.SameContentAs(base) {
		t.Fatalf("换了登记责任方与登记时刻的同一份内容被判成了不同")
	}

	changed := map[string]domain.LocationRevision{
		"修订号": locationRevisionOf(t, 2, locationContentOf(t, nil)),
		"所属库区": locationRevisionOf(t, 1, locationContentOf(t, func(spec *domain.LocationContentSpec) {
			spec.ZoneCode = valueOf(t, domain.NewZoneCode, "SYN-ZONE-02")
		})),
		"库位类型": locationRevisionOf(t, 1, locationContentOf(t, func(spec *domain.LocationContentSpec) {
			spec.LocationType = valueOf(t, domain.NewLocationTypeCode, "SYN-LT-01")
		})),
		"优先级为 0": locationRevisionOf(t, 1, locationContentOf(t, func(spec *domain.LocationContentSpec) {
			spec.Priority = domain.NewLocationPriority(0)
		})),
		"停用": locationRevisionOf(t, 1, locationContentOf(t, func(spec *domain.LocationContentSpec) {
			spec.Deactivated = true
		})),
	}
	for cell, other := range changed {
		if base.SameContentAs(other) || other.SameContentAs(base) {
			t.Errorf("只改了%s，却被判成内容相同", cell)
		}
	}

	build := func(tenant, warehouse, location string) domain.LocationRevision {
		built, err := domain.NewLocationRevision(
			valueOf(t, domain.NewTenantID, tenant), valueOf(t, domain.NewWarehouseCode, warehouse),
			valueOf(t, domain.NewLocationCode, location), 1, locationContentOf(t, nil),
			valueOf(t, domain.NewRegistrant, "operator-1"), registeredAt)
		if err != nil {
			t.Fatalf("NewLocationRevision: %v", err)
		}
		return built
	}
	for name, other := range map[string]domain.LocationRevision{
		"库位编码只差大小写": build("SYN-TENANT-01", "SYN-WH-01", "syn-loc-01"),
		"另一个仓":      build("SYN-TENANT-01", "SYN-WH-02", "SYN-LOC-01"),
		"另一个租户":     build("SYN-TENANT-02", "SYN-WH-01", "SYN-LOC-01"),
	} {
		if base.SameContentAs(other) {
			t.Errorf("%s 被判成内容相同", name)
		}
	}
}
