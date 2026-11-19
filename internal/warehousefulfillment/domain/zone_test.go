package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 本文件证库区这一册的领域面（UC-WF-001 的库区修订）：库区编码与名称逐字保存、库区用途是封闭六种、一笔修订是一份完整内容，
// 以及内容相同的比较只比内容、不比登记责任方与登记时刻。

func zoneContentSpec(t *testing.T) domain.ZoneContentSpec {
	t.Helper()
	return domain.ZoneContentSpec{
		Name:  valueOf(t, domain.NewZoneName, "合成存储区"),
		Usage: domain.ZoneUsageStorage,
	}
}

func zoneContentOf(t *testing.T, edit func(*domain.ZoneContentSpec)) domain.ZoneContent {
	t.Helper()
	spec := zoneContentSpec(t)
	if edit != nil {
		edit(&spec)
	}
	content, err := domain.NewZoneContent(spec)
	if err != nil {
		t.Fatalf("NewZoneContent: %v", err)
	}
	return content
}

func zoneRevisionOf(t *testing.T, number int, content domain.ZoneContent) domain.ZoneRevision {
	t.Helper()
	revision, err := domain.NewZoneRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		valueOf(t, domain.NewZoneCode, "SYN-ZONE-01"),
		number, content,
		valueOf(t, domain.NewRegistrant, "operator-1"),
		registeredAt,
	)
	if err != nil {
		t.Fatalf("NewZoneRevision: %v", err)
	}
	return revision
}

// Covers: AT-WF-001（库区的部分）「库区编码逐字保存」——带首尾空白、内部连续空白与大小写混写的库区编码与名称原样留下，只差大小写
// 或空白的编码是两个身份；空白串与存不下的串（NUL、非 UTF-8）在构造处拒，免得到库里才以技术失败露面。
func TestZoneCodeAndNameKeepEveryByteAndRefuseBlankOrUnstorable(t *testing.T) {
	constructors := map[string]func(string) (string, error){
		"库区编码": func(raw string) (string, error) {
			value, err := domain.NewZoneCode(raw)
			return value.String(), err
		},
		"库区名称": func(raw string) (string, error) {
			value, err := domain.NewZoneName(raw)
			return value.String(), err
		},
	}
	for name, construct := range constructors {
		for _, raw := range []string{"  Zone-Mixed\t01  ", "a  b", "zone-1", "ZONE-1", "\u00a0nbsp\u00a0"} {
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
	upper := valueOf(t, domain.NewZoneCode, "ZONE-1")
	lower := valueOf(t, domain.NewZoneCode, "zone-1")
	padded := valueOf(t, domain.NewZoneCode, " ZONE-1")
	if upper == lower || upper == padded || lower == padded {
		t.Fatalf("只差大小写或空白的库区编码被折成了同一个：%q %q %q", upper, lower, padded)
	}
}

// Covers: AT-WF-004（库区的部分）、票面「库区」——库区用途是产品内置的封闭六种，字面量逐字如下；集外的词（含小写、带空白、
// 空串、仓的种类）反查不出，交由接入侧按坏载荷答；零值没有字面量。
func TestZoneUsageIsAClosedSetOfSixLiterals(t *testing.T) {
	want := map[string]domain.ZoneUsage{
		"RECEIVING_STAGING": domain.ZoneUsageReceivingStaging,
		"STORAGE":           domain.ZoneUsageStorage,
		"PICKING":           domain.ZoneUsagePicking,
		"DEFECTIVE":         domain.ZoneUsageDefective,
		"RETURN_STAGING":    domain.ZoneUsageReturnStaging,
		"OUTBOUND_STAGING":  domain.ZoneUsageOutboundStaging,
	}
	for literal, usage := range want {
		got, known := domain.ZoneUsageNamed(literal)
		if !known || got != usage || got.String() != literal {
			t.Errorf("ZoneUsageNamed(%q) = (%v, %v)，want (%v, true)", literal, got, known, usage)
		}
	}
	for _, literal := range []string{"", "storage", "Picking", " STORAGE", "STORAGE ", "STAGING", "RECEIVING", "SELF_OPERATED"} {
		if usage, known := domain.ZoneUsageNamed(literal); known {
			t.Errorf("ZoneUsageNamed(%q) = %v，集外的词不该认得", literal, usage)
		}
	}
	if domain.ZoneUsageInvalid.String() != "" {
		t.Errorf("零值的字面量 = %q，want 空串", domain.ZoneUsageInvalid.String())
	}

	all := domain.ZoneUsages()
	if len(all) != len(want) {
		t.Fatalf("ZoneUsages() = %v，want 恰是封闭的 %d 种", all, len(want))
	}
	for _, usage := range all {
		if _, inSet := want[usage.String()]; !inSet {
			t.Errorf("ZoneUsages() 里有集外的取值 %v", usage)
		}
	}
	// 交回的是副本：调用方改它不碰封闭集本身。
	all[0] = domain.ZoneUsageInvalid
	if again := domain.ZoneUsages(); again[0] == domain.ZoneUsageInvalid {
		t.Errorf("ZoneUsages() 交回了封闭集本身，调用方改动会漏进去")
	}
}

// Covers: 内容是一份完整内容——缺名称或用途的构造不成，并指出缺的是哪一格（未经构造器的零值名称也算缺）。
func TestZoneContentRefusesAMissingCellAndNamesIt(t *testing.T) {
	for name, test := range map[string]struct {
		edit func(*domain.ZoneContentSpec)
		want domain.ZoneField
	}{
		"缺名称": {func(spec *domain.ZoneContentSpec) { spec.Name = domain.ZoneName{} }, domain.ZoneFieldName},
		"缺用途": {func(spec *domain.ZoneContentSpec) { spec.Usage = domain.ZoneUsageInvalid }, domain.ZoneFieldUsage},
	} {
		spec := zoneContentSpec(t)
		test.edit(&spec)
		_, err := domain.NewZoneContent(spec)
		var invalid *domain.ZoneContentFieldError
		if !errors.Is(err, domain.ErrInvalidZoneContent) || !errors.As(err, &invalid) || invalid.Field != test.want {
			t.Errorf("%s：err = %v，want ErrInvalidZoneContent 指向 %s", name, err, test.want)
		}
	}
}

// 各格的字面量是应用层结果里「指出哪一格」的取值，外半据它译装；改字面量等于改了对外的词。仓编码那一格是库区答「指名的仓查不到」时
// 指出的格，与仓这一册答复里的同一个键名。
func TestZoneFieldLiterals(t *testing.T) {
	for field, literal := range map[domain.ZoneField]string{
		domain.ZoneFieldWarehouseCode: "warehouseCode",
		domain.ZoneFieldName:          "name",
		domain.ZoneFieldUsage:         "usage",
	} {
		if field.String() != literal {
			t.Errorf("字面量 = %q，want %q", field.String(), literal)
		}
	}
	if domain.ZoneFieldInvalid.String() != "" {
		t.Errorf("零值的字面量 = %q，want 空串", domain.ZoneFieldInvalid.String())
	}
}

func TestZoneRevisionRefusesNonPositiveNumbersAndUnconstructedParts(t *testing.T) {
	content := zoneContentOf(t, nil)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	warehouse := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	code := valueOf(t, domain.NewZoneCode, "SYN-ZONE-01")
	registrant := valueOf(t, domain.NewRegistrant, "operator-1")

	for _, number := range []int{0, -1} {
		if _, err := domain.NewZoneRevision(tenant, warehouse, code, number, content, registrant, registeredAt); !errors.Is(err, domain.ErrInvalidRevisionNumber) {
			t.Errorf("修订号 %d 的错误 = %v，want ErrInvalidRevisionNumber", number, err)
		}
	}
	for name, build := range map[string]func() (domain.ZoneRevision, error){
		"缺租户": func() (domain.ZoneRevision, error) {
			return domain.NewZoneRevision(domain.TenantID{}, warehouse, code, 1, content, registrant, registeredAt)
		},
		"缺仓编码": func() (domain.ZoneRevision, error) {
			return domain.NewZoneRevision(tenant, domain.WarehouseCode{}, code, 1, content, registrant, registeredAt)
		},
		"缺库区编码": func() (domain.ZoneRevision, error) {
			return domain.NewZoneRevision(tenant, warehouse, domain.ZoneCode{}, 1, content, registrant, registeredAt)
		},
		"缺登记责任方": func() (domain.ZoneRevision, error) {
			return domain.NewZoneRevision(tenant, warehouse, code, 1, content, domain.Registrant{}, registeredAt)
		},
		"缺登记时刻": func() (domain.ZoneRevision, error) {
			return domain.NewZoneRevision(tenant, warehouse, code, 1, content, registrant, time.Time{})
		},
	} {
		if _, err := build(); !errors.Is(err, domain.ErrInvalidZoneRevision) {
			t.Errorf("%s：err = %v，want ErrInvalidZoneRevision", name, err)
		}
	}
	var invalid *domain.ZoneContentFieldError
	if _, err := domain.NewZoneRevision(tenant, warehouse, code, 1, domain.ZoneContent{}, registrant, registeredAt); !errors.As(err, &invalid) {
		t.Errorf("未经构造器的零值内容的错误 = %v，want 指出缺哪一格的 ZoneContentFieldError", err)
	}
}

// 登记时刻收成库里存得下的精度（微秒）并统一到 UTC：不收的话，回显的时刻与读回的差在纳秒上。
func TestZoneRevisionKeepsTheRegistrationInstantAtStorablePrecision(t *testing.T) {
	shanghai := time.FixedZone("UTC+8", 8*3600)
	at := time.Date(2026, 11, 18, 17, 30, 0, 123456789, shanghai)
	revision, err := domain.NewZoneRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		valueOf(t, domain.NewZoneCode, "SYN-ZONE-01"),
		1, zoneContentOf(t, nil), valueOf(t, domain.NewRegistrant, "operator-1"), at)
	if err != nil {
		t.Fatalf("NewZoneRevision: %v", err)
	}
	want := time.Date(2026, 11, 18, 9, 30, 0, 123456000, time.UTC)
	if got := revision.RegisteredAt(); !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("登记时刻 = %v，want %v（UTC，微秒）", got, want)
	}
}

// Covers: AT-WF-001「原样重投答已登记；同一修订号换内容答内容冲突」所依的比较（库区的部分）——只比完整内容（租户、仓编码、库区编码、
// 修订号、名称、用途、是否停用），不比登记责任方与登记时刻。同一库区编码在另一个仓下是另一个库区。
func TestZoneSameContentComparesEveryContentCellAndNothingElse(t *testing.T) {
	base := zoneRevisionOf(t, 1, zoneContentOf(t, nil))

	otherRegistrantAndInstant, err := domain.NewZoneRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		valueOf(t, domain.NewZoneCode, "SYN-ZONE-01"),
		1, zoneContentOf(t, nil),
		valueOf(t, domain.NewRegistrant, "operator-2"),
		registeredAt.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("NewZoneRevision: %v", err)
	}
	if !base.SameContentAs(otherRegistrantAndInstant) || !otherRegistrantAndInstant.SameContentAs(base) {
		t.Fatalf("换了登记责任方与登记时刻的同一份内容被判成了不同")
	}

	changed := map[string]domain.ZoneRevision{
		"修订号": zoneRevisionOf(t, 2, zoneContentOf(t, nil)),
		"名称": zoneRevisionOf(t, 1, zoneContentOf(t, func(spec *domain.ZoneContentSpec) {
			spec.Name = valueOf(t, domain.NewZoneName, "合成存储区 ")
		})),
		"用途": zoneRevisionOf(t, 1, zoneContentOf(t, func(spec *domain.ZoneContentSpec) {
			spec.Usage = domain.ZoneUsagePicking
		})),
		"停用": zoneRevisionOf(t, 1, zoneContentOf(t, func(spec *domain.ZoneContentSpec) {
			spec.Deactivated = true
		})),
	}
	for cell, other := range changed {
		if base.SameContentAs(other) || other.SameContentAs(base) {
			t.Errorf("只改了%s，却被判成内容相同", cell)
		}
	}

	build := func(tenant, warehouse, zone string) domain.ZoneRevision {
		built, err := domain.NewZoneRevision(
			valueOf(t, domain.NewTenantID, tenant), valueOf(t, domain.NewWarehouseCode, warehouse),
			valueOf(t, domain.NewZoneCode, zone), 1, zoneContentOf(t, nil),
			valueOf(t, domain.NewRegistrant, "operator-1"), registeredAt)
		if err != nil {
			t.Fatalf("NewZoneRevision: %v", err)
		}
		return built
	}
	for name, other := range map[string]domain.ZoneRevision{
		"库区编码只差大小写": build("SYN-TENANT-01", "SYN-WH-01", "syn-zone-01"),
		"另一个仓":      build("SYN-TENANT-01", "SYN-WH-02", "SYN-ZONE-01"),
		"另一个租户":     build("SYN-TENANT-02", "SYN-WH-01", "SYN-ZONE-01"),
	} {
		if base.SameContentAs(other) {
			t.Errorf("%s 被判成内容相同", name)
		}
	}
}
