package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 本文件证库位类型这一册的领域面（UC-WF-001 的库位类型修订）：编码与名称逐字保存、一笔修订是一份完整内容，以及内容相同的比较只比
// 内容、不比登记责任方与登记时刻。库位类型没有登记后不改的格，也没有指名别的对象的格。

func locationTypeContentOf(t *testing.T, edit func(*domain.LocationTypeContentSpec)) domain.LocationTypeContent {
	t.Helper()
	spec := domain.LocationTypeContentSpec{Name: valueOf(t, domain.NewLocationTypeName, "合成货架位")}
	if edit != nil {
		edit(&spec)
	}
	content, err := domain.NewLocationTypeContent(spec)
	if err != nil {
		t.Fatalf("NewLocationTypeContent: %v", err)
	}
	return content
}

func locationTypeRevisionOf(t *testing.T, code string, number int, content domain.LocationTypeContent) domain.LocationTypeRevision {
	t.Helper()
	revision, err := domain.NewLocationTypeRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewLocationTypeCode, code),
		number, content,
		valueOf(t, domain.NewRegistrant, "operator-1"),
		registeredAt,
	)
	if err != nil {
		t.Fatalf("NewLocationTypeRevision: %v", err)
	}
	return revision
}

// Covers: AT-WF-001（库位类型的部分）「编码逐字保存」——带首尾空白、内部连续空白与大小写混写的编码与名称原样留下，只差大小写或空白的编码
// 是两个身份；空白串与存不下的串（NUL、非 UTF-8）在构造处拒，免得到库里才以技术失败露面。
func TestLocationTypeCodeAndNameKeepEveryByteAndRefuseBlankOrUnstorable(t *testing.T) {
	constructors := map[string]func(string) (string, error){
		"库位类型编码": func(raw string) (string, error) {
			value, err := domain.NewLocationTypeCode(raw)
			return value.String(), err
		},
		"库位类型名称": func(raw string) (string, error) {
			value, err := domain.NewLocationTypeName(raw)
			return value.String(), err
		},
	}
	for name, construct := range constructors {
		for _, raw := range []string{"  Rack\t01  ", "a  b", "rack-1", "RACK-1", "\u00a0nbsp\u00a0"} {
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
	upper := valueOf(t, domain.NewLocationTypeCode, "RACK-1")
	lower := valueOf(t, domain.NewLocationTypeCode, "rack-1")
	padded := valueOf(t, domain.NewLocationTypeCode, " RACK-1")
	if upper == lower || upper == padded || lower == padded {
		t.Fatalf("只差大小写或空白的库位类型编码被折成了同一个：%q %q %q", upper, lower, padded)
	}
}

// Covers: 内容是一份完整内容——缺名称的构造不成，并指出缺的是 name 这一格（未经构造器的零值名称也算缺）；是否停用缺省就是不停用。
func TestLocationTypeContentRefusesAMissingNameAndDefaultsToNotDeactivated(t *testing.T) {
	_, err := domain.NewLocationTypeContent(domain.LocationTypeContentSpec{})
	var invalid *domain.LocationTypeContentFieldError
	if !errors.Is(err, domain.ErrInvalidLocationTypeContent) || !errors.As(err, &invalid) || invalid.Field != domain.LocationTypeFieldName {
		t.Fatalf("缺名称：err = %v，want ErrInvalidLocationTypeContent 指向 name", err)
	}
	if locationTypeContentOf(t, nil).Deactivated() {
		t.Fatalf("没给是否停用却成了停用")
	}
}

// 字面量是应用层结果里「指出哪一格」的取值，外半据它译装；改字面量等于改了对外的词。
func TestLocationTypeFieldLiterals(t *testing.T) {
	if domain.LocationTypeFieldName.String() != "name" {
		t.Errorf("name 的字面量 = %q", domain.LocationTypeFieldName.String())
	}
	if domain.LocationTypeFieldInvalid.String() != "" {
		t.Errorf("零值的字面量 = %q，want 空串", domain.LocationTypeFieldInvalid.String())
	}
}

func TestLocationTypeRevisionRefusesNonPositiveNumbersAndUnconstructedParts(t *testing.T) {
	content := locationTypeContentOf(t, nil)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	code := valueOf(t, domain.NewLocationTypeCode, "SYN-LT-01")
	registrant := valueOf(t, domain.NewRegistrant, "operator-1")

	for _, number := range []int{0, -1} {
		if _, err := domain.NewLocationTypeRevision(tenant, code, number, content, registrant, registeredAt); !errors.Is(err, domain.ErrInvalidRevisionNumber) {
			t.Errorf("修订号 %d 的错误 = %v，want ErrInvalidRevisionNumber", number, err)
		}
	}
	for name, build := range map[string]func() (domain.LocationTypeRevision, error){
		"缺租户": func() (domain.LocationTypeRevision, error) {
			return domain.NewLocationTypeRevision(domain.TenantID{}, code, 1, content, registrant, registeredAt)
		},
		"缺库位类型编码": func() (domain.LocationTypeRevision, error) {
			return domain.NewLocationTypeRevision(tenant, domain.LocationTypeCode{}, 1, content, registrant, registeredAt)
		},
		"缺登记责任方": func() (domain.LocationTypeRevision, error) {
			return domain.NewLocationTypeRevision(tenant, code, 1, content, domain.Registrant{}, registeredAt)
		},
		"缺登记时刻": func() (domain.LocationTypeRevision, error) {
			return domain.NewLocationTypeRevision(tenant, code, 1, content, registrant, time.Time{})
		},
	} {
		if _, err := build(); !errors.Is(err, domain.ErrInvalidLocationTypeRevision) {
			t.Errorf("%s：err = %v，want ErrInvalidLocationTypeRevision", name, err)
		}
	}
	var invalid *domain.LocationTypeContentFieldError
	if _, err := domain.NewLocationTypeRevision(tenant, code, 1, domain.LocationTypeContent{}, registrant, registeredAt); !errors.As(err, &invalid) {
		t.Errorf("未经构造器的零值内容的错误 = %v，want 指出缺哪一格的 LocationTypeContentFieldError", err)
	}
}

// 登记时刻收成库里存得下的精度（微秒）并统一到 UTC：不收的话，回显的时刻与读回的差在纳秒上。
func TestLocationTypeRevisionKeepsTheRegistrationInstantAtStorablePrecision(t *testing.T) {
	shanghai := time.FixedZone("UTC+8", 8*3600)
	at := time.Date(2026, 11, 18, 17, 30, 0, 123456789, shanghai)
	revision, err := domain.NewLocationTypeRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewLocationTypeCode, "SYN-LT-01"),
		1, locationTypeContentOf(t, nil), valueOf(t, domain.NewRegistrant, "operator-1"), at)
	if err != nil {
		t.Fatalf("NewLocationTypeRevision: %v", err)
	}
	want := time.Date(2026, 11, 18, 9, 30, 0, 123456000, time.UTC)
	if got := revision.RegisteredAt(); !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("登记时刻 = %v，want %v（UTC，微秒）", got, want)
	}
}

// Covers: AT-WF-001「原样重投答已登记；同一修订号换内容答内容冲突」所依的比较（库位类型的部分）——只比完整内容（租户、编码、修订号、
// 名称、是否停用），不比登记责任方与登记时刻。库位类型各仓共用，身份里没有仓。
func TestLocationTypeSameContentComparesEveryContentCellAndNothingElse(t *testing.T) {
	base := locationTypeRevisionOf(t, "SYN-LT-01", 1, locationTypeContentOf(t, nil))

	otherRegistrantAndInstant, err := domain.NewLocationTypeRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewLocationTypeCode, "SYN-LT-01"),
		1, locationTypeContentOf(t, nil),
		valueOf(t, domain.NewRegistrant, "operator-2"),
		registeredAt.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("NewLocationTypeRevision: %v", err)
	}
	if !base.SameContentAs(otherRegistrantAndInstant) || !otherRegistrantAndInstant.SameContentAs(base) {
		t.Fatalf("换了登记责任方与登记时刻的同一份内容被判成了不同")
	}

	changed := map[string]domain.LocationTypeRevision{
		"修订号": locationTypeRevisionOf(t, "SYN-LT-01", 2, locationTypeContentOf(t, nil)),
		"名称": locationTypeRevisionOf(t, "SYN-LT-01", 1, locationTypeContentOf(t, func(spec *domain.LocationTypeContentSpec) {
			spec.Name = valueOf(t, domain.NewLocationTypeName, "合成货架位 ")
		})),
		"停用": locationTypeRevisionOf(t, "SYN-LT-01", 1, locationTypeContentOf(t, func(spec *domain.LocationTypeContentSpec) {
			spec.Deactivated = true
		})),
		"编码只差大小写": locationTypeRevisionOf(t, "syn-lt-01", 1, locationTypeContentOf(t, nil)),
	}
	for cell, other := range changed {
		if base.SameContentAs(other) || other.SameContentAs(base) {
			t.Errorf("只改了%s，却被判成内容相同", cell)
		}
	}

	otherTenant, err := domain.NewLocationTypeRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-02"),
		valueOf(t, domain.NewLocationTypeCode, "SYN-LT-01"),
		1, locationTypeContentOf(t, nil), valueOf(t, domain.NewRegistrant, "operator-1"), registeredAt)
	if err != nil {
		t.Fatalf("NewLocationTypeRevision: %v", err)
	}
	if base.SameContentAs(otherTenant) {
		t.Errorf("另一个租户的同编码库位类型被判成内容相同")
	}
}
