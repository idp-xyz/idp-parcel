package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 本文件证仓这一册的领域面（UC-WF-001 的仓修订）：几格取值逐字保存、封闭的两种仓、一笔修订是一份完整内容，
// 以及内容相同的比较只比内容、不比登记责任方与登记时刻。

func valueOf[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

var registeredAt = time.Date(2026, 11, 18, 9, 30, 0, 0, time.UTC)

func contentSpec(t *testing.T) domain.WarehouseContentSpec {
	t.Helper()
	return domain.WarehouseContentSpec{
		Name: valueOf(t, domain.NewWarehouseName, "合成一号仓"),
		Kind: domain.WarehouseKindSelfOperated,
		Address: domain.NewWarehouseAddress(
			valueOf(t, domain.NewCountryCode, "CN"),
			valueOf(t, domain.NewAddressText, "合成路 1 号"),
		),
		BusinessTimeZone: valueOf(t, domain.NewBusinessTimeZone, "Asia/Shanghai"),
		ColocatedNode:    valueOf(t, domain.NewColocatedNode, "SYN-NODE-01"),
	}
}

func contentOf(t *testing.T, edit func(*domain.WarehouseContentSpec)) domain.WarehouseContent {
	t.Helper()
	spec := contentSpec(t)
	if edit != nil {
		edit(&spec)
	}
	content, err := domain.NewWarehouseContent(spec)
	if err != nil {
		t.Fatalf("NewWarehouseContent: %v", err)
	}
	return content
}

func revisionOf(t *testing.T, number int, content domain.WarehouseContent) domain.WarehouseRevision {
	t.Helper()
	revision, err := domain.NewWarehouseRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		number, content,
		valueOf(t, domain.NewRegistrant, "operator-1"),
		registeredAt,
	)
	if err != nil {
		t.Fatalf("NewWarehouseRevision: %v", err)
	}
	return revision
}

// Covers: AT-WF-001（仓的部分）「仓编码逐字保存」——带首尾空白、内部连续空白与大小写混写的值原样留下，两个只差大小写
// 或空白的编码是两个身份；空白串与存不下的串（NUL、非 UTF-8）在构造处拒，免得到库里才以技术失败露面。
func TestVerbatimValuesKeepEveryByteAndRefuseBlankOrUnstorable(t *testing.T) {
	constructors := map[string]func(string) (string, error){
		"仓编码": func(raw string) (string, error) {
			value, err := domain.NewWarehouseCode(raw)
			return value.String(), err
		},
		"仓名称": func(raw string) (string, error) {
			value, err := domain.NewWarehouseName(raw)
			return value.String(), err
		},
		"地址正文": func(raw string) (string, error) {
			value, err := domain.NewAddressText(raw)
			return value.String(), err
		},
		"国家码": func(raw string) (string, error) {
			value, err := domain.NewCountryCode(raw)
			return value.String(), err
		},
		"同址物流节点": func(raw string) (string, error) {
			value, err := domain.NewColocatedNode(raw)
			return value.String(), err
		},
		"业务时区": func(raw string) (string, error) {
			value, err := domain.NewBusinessTimeZone(raw)
			return value.String(), err
		},
		"登记责任方": func(raw string) (string, error) {
			value, err := domain.NewRegistrant(raw)
			return value.String(), err
		},
		"租户": func(raw string) (string, error) {
			value, err := domain.NewTenantID(raw)
			return value.String(), err
		},
	}
	for name, construct := range constructors {
		for _, raw := range []string{"  Wh-Mixed\t01  ", "a  b", "wh-1", "WH-1", "\u00a0nbsp\u00a0"} {
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

	upper := valueOf(t, domain.NewWarehouseCode, "WH-1")
	lower := valueOf(t, domain.NewWarehouseCode, "wh-1")
	padded := valueOf(t, domain.NewWarehouseCode, " WH-1")
	if upper == lower || upper == padded || lower == padded {
		t.Fatalf("只差大小写或空白的仓编码被折成了同一个：%q %q %q", upper, lower, padded)
	}
}

// Covers: 票面「会话代决」的「仓的种类字面量」——仓的种类是封闭两格，字面量 SELF_OPERATED 与 AGENT；集外的词反查不出，交由接入侧按坏载荷答。
func TestWarehouseKindIsAClosedSetOfTwoLiterals(t *testing.T) {
	for literal, want := range map[string]domain.WarehouseKind{
		"SELF_OPERATED": domain.WarehouseKindSelfOperated,
		"AGENT":         domain.WarehouseKindAgent,
	} {
		got, known := domain.WarehouseKindNamed(literal)
		if !known || got != want || got.String() != literal {
			t.Errorf("WarehouseKindNamed(%q) = (%v, %v)，want (%v, true)", literal, got, known, want)
		}
	}
	for _, literal := range []string{"", "self_operated", "Agent", " AGENT", "AGENT ", "OWNED", "SELF-OPERATED"} {
		if kind, known := domain.WarehouseKindNamed(literal); known {
			t.Errorf("WarehouseKindNamed(%q) = %v，集外的词不该认得", literal, kind)
		}
	}
	if domain.WarehouseKindInvalid.String() != "" {
		t.Errorf("零值的字面量 = %q，want 空串", domain.WarehouseKindInvalid.String())
	}
}

// Covers: AT-WF-006（仓的部分）——业务时区与同址物流节点都可缺；缺就是没登记，不取任何默认。
func TestContentMayLeaveTheBusinessTimeZoneAndColocatedNodeOut(t *testing.T) {
	bare := contentOf(t, func(spec *domain.WarehouseContentSpec) {
		spec.BusinessTimeZone = domain.BusinessTimeZone{}
		spec.ColocatedNode = domain.ColocatedNode{}
	})
	if bare.BusinessTimeZone().Registered() || bare.ColocatedNode().Named() {
		t.Fatalf("没给的业务时区或同址物流节点被当成了有：%q %q", bare.BusinessTimeZone(), bare.ColocatedNode())
	}
	full := contentOf(t, nil)
	if !full.BusinessTimeZone().Registered() || full.BusinessTimeZone().String() != "Asia/Shanghai" ||
		!full.ColocatedNode().Named() || full.ColocatedNode().String() != "SYN-NODE-01" {
		t.Fatalf("给了的业务时区或同址物流节点读不回：%q %q", full.BusinessTimeZone(), full.ColocatedNode())
	}
}

// Covers: 内容是一份完整内容——缺名称、种类、国家或地址正文的构造不成，并指出缺的是哪一格（未经构造器的零值取值也算缺）。
func TestContentRefusesAMissingCellAndNamesIt(t *testing.T) {
	for name, test := range map[string]struct {
		edit func(*domain.WarehouseContentSpec)
		want domain.WarehouseField
	}{
		"缺名称": {func(spec *domain.WarehouseContentSpec) { spec.Name = domain.WarehouseName{} }, domain.WarehouseFieldName},
		"缺种类": {func(spec *domain.WarehouseContentSpec) { spec.Kind = domain.WarehouseKindInvalid }, domain.WarehouseFieldKind},
		"缺国家": {func(spec *domain.WarehouseContentSpec) {
			spec.Address = domain.NewWarehouseAddress(domain.CountryCode{}, valueOf(t, domain.NewAddressText, "x"))
		}, domain.WarehouseFieldAddressCountry},
		"缺地址正文": {func(spec *domain.WarehouseContentSpec) {
			spec.Address = domain.NewWarehouseAddress(valueOf(t, domain.NewCountryCode, "CN"), domain.AddressText{})
		}, domain.WarehouseFieldAddressText},
	} {
		spec := contentSpec(t)
		test.edit(&spec)
		_, err := domain.NewWarehouseContent(spec)
		var invalid *domain.ContentFieldError
		if !errors.Is(err, domain.ErrInvalidWarehouseContent) || !errors.As(err, &invalid) || invalid.Field != test.want {
			t.Errorf("%s：err = %v，want ErrInvalidWarehouseContent 指向 %s", name, err, test.want)
		}
	}
}

// 各格的字面量是应用层结果里「指出哪一格」的取值，外半据它译装；改字面量等于改了对外的词。
func TestWarehouseFieldLiterals(t *testing.T) {
	want := map[domain.WarehouseField]string{
		domain.WarehouseFieldName:             "name",
		domain.WarehouseFieldKind:             "kind",
		domain.WarehouseFieldAddressCountry:   "address.country",
		domain.WarehouseFieldAddressText:      "address.text",
		domain.WarehouseFieldBusinessTimeZone: "businessTimeZone",
	}
	for field, literal := range want {
		if field.String() != literal {
			t.Errorf("字面量 = %q，want %q", field.String(), literal)
		}
	}
	if domain.WarehouseFieldInvalid.String() != "" {
		t.Errorf("零值的字面量 = %q，want 空串", domain.WarehouseFieldInvalid.String())
	}
}

func TestRevisionRefusesNonPositiveNumbersAndUnconstructedParts(t *testing.T) {
	content := contentOf(t, nil)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	code := valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	registrant := valueOf(t, domain.NewRegistrant, "operator-1")

	for _, number := range []int{0, -1} {
		if _, err := domain.NewWarehouseRevision(tenant, code, number, content, registrant, registeredAt); !errors.Is(err, domain.ErrInvalidRevisionNumber) {
			t.Errorf("修订号 %d 的错误 = %v，want ErrInvalidRevisionNumber", number, err)
		}
	}
	for name, build := range map[string]func() (domain.WarehouseRevision, error){
		"缺租户": func() (domain.WarehouseRevision, error) {
			return domain.NewWarehouseRevision(domain.TenantID{}, code, 1, content, registrant, registeredAt)
		},
		"缺仓编码": func() (domain.WarehouseRevision, error) {
			return domain.NewWarehouseRevision(tenant, domain.WarehouseCode{}, 1, content, registrant, registeredAt)
		},
		"缺登记责任方": func() (domain.WarehouseRevision, error) {
			return domain.NewWarehouseRevision(tenant, code, 1, content, domain.Registrant{}, registeredAt)
		},
		"缺登记时刻": func() (domain.WarehouseRevision, error) {
			return domain.NewWarehouseRevision(tenant, code, 1, content, registrant, time.Time{})
		},
	} {
		if _, err := build(); !errors.Is(err, domain.ErrInvalidWarehouseRevision) {
			t.Errorf("%s：err = %v，want ErrInvalidWarehouseRevision", name, err)
		}
	}
	var invalid *domain.ContentFieldError
	if _, err := domain.NewWarehouseRevision(tenant, code, 1, domain.WarehouseContent{}, registrant, registeredAt); !errors.As(err, &invalid) {
		t.Errorf("未经构造器的零值内容的错误 = %v，want 指出缺哪一格的 ContentFieldError", err)
	}
}

// 登记时刻收成库里存得下的精度（微秒）并统一到 UTC：不收的话，回显的时刻与读回的差在纳秒上。
func TestRevisionKeepsTheRegistrationInstantAtStorablePrecision(t *testing.T) {
	shanghai := time.FixedZone("UTC+8", 8*3600)
	at := time.Date(2026, 11, 18, 17, 30, 0, 123456789, shanghai)
	revision, err := domain.NewWarehouseRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		1, contentOf(t, nil), valueOf(t, domain.NewRegistrant, "operator-1"), at)
	if err != nil {
		t.Fatalf("NewWarehouseRevision: %v", err)
	}
	want := time.Date(2026, 11, 18, 9, 30, 0, 123456000, time.UTC)
	if got := revision.RegisteredAt(); !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("登记时刻 = %v，want %v（UTC，微秒）", got, want)
	}
}

// Covers: AT-WF-001「原样重投答已登记；同一修订号换内容答内容冲突」所依的比较——只比完整内容（仓编码、修订号、名称、种类、
// 地址、业务时区、同址物流节点、是否停用），不比登记责任方与登记时刻。
func TestSameContentComparesEveryContentCellAndNothingElse(t *testing.T) {
	base := revisionOf(t, 1, contentOf(t, nil))

	otherRegistrantAndInstant, err := domain.NewWarehouseRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		1, contentOf(t, nil),
		valueOf(t, domain.NewRegistrant, "operator-2"),
		registeredAt.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("NewWarehouseRevision: %v", err)
	}
	if !base.SameContentAs(otherRegistrantAndInstant) || !otherRegistrantAndInstant.SameContentAs(base) {
		t.Fatalf("换了登记责任方与登记时刻的同一份内容被判成了不同")
	}

	changed := map[string]domain.WarehouseRevision{
		"修订号": revisionOf(t, 2, contentOf(t, nil)),
		"名称": revisionOf(t, 1, contentOf(t, func(spec *domain.WarehouseContentSpec) {
			spec.Name = valueOf(t, domain.NewWarehouseName, "合成一号仓 ")
		})),
		"种类": revisionOf(t, 1, contentOf(t, func(spec *domain.WarehouseContentSpec) {
			spec.Kind = domain.WarehouseKindAgent
		})),
		"国家": revisionOf(t, 1, contentOf(t, func(spec *domain.WarehouseContentSpec) {
			spec.Address = domain.NewWarehouseAddress(valueOf(t, domain.NewCountryCode, "US"), spec.Address.Text())
		})),
		"地址正文": revisionOf(t, 1, contentOf(t, func(spec *domain.WarehouseContentSpec) {
			spec.Address = domain.NewWarehouseAddress(spec.Address.Country(), valueOf(t, domain.NewAddressText, "合成路 1 号 "))
		})),
		"业务时区改值": revisionOf(t, 1, contentOf(t, func(spec *domain.WarehouseContentSpec) {
			spec.BusinessTimeZone = valueOf(t, domain.NewBusinessTimeZone, "Asia/Tokyo")
		})),
		"业务时区去掉": revisionOf(t, 1, contentOf(t, func(spec *domain.WarehouseContentSpec) {
			spec.BusinessTimeZone = domain.BusinessTimeZone{}
		})),
		"同址物流节点只差大小写": revisionOf(t, 1, contentOf(t, func(spec *domain.WarehouseContentSpec) {
			spec.ColocatedNode = valueOf(t, domain.NewColocatedNode, "syn-node-01")
		})),
		"同址物流节点去掉": revisionOf(t, 1, contentOf(t, func(spec *domain.WarehouseContentSpec) {
			spec.ColocatedNode = domain.ColocatedNode{}
		})),
		"停用": revisionOf(t, 1, contentOf(t, func(spec *domain.WarehouseContentSpec) {
			spec.Deactivated = true
		})),
	}
	for cell, other := range changed {
		if base.SameContentAs(other) || other.SameContentAs(base) {
			t.Errorf("只改了%s，却被判成内容相同", cell)
		}
	}

	otherCode, err := domain.NewWarehouseRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewWarehouseCode, "syn-wh-01"),
		1, contentOf(t, nil), valueOf(t, domain.NewRegistrant, "operator-1"), registeredAt)
	if err != nil {
		t.Fatalf("NewWarehouseRevision: %v", err)
	}
	otherTenant, err := domain.NewWarehouseRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-02"),
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		1, contentOf(t, nil), valueOf(t, domain.NewRegistrant, "operator-1"), registeredAt)
	if err != nil {
		t.Fatalf("NewWarehouseRevision: %v", err)
	}
	if base.SameContentAs(otherCode) || base.SameContentAs(otherTenant) {
		t.Errorf("只差大小写的仓编码或不同租户被判成内容相同")
	}
}
