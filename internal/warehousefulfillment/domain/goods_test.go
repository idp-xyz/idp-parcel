package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// 本文件证商品这一册的领域面（UC-WF-002 的商品修订）：编码与别名逐字保存、封闭集按字面认、一份资料的「内容相同」不看列表的先后、有效区间的交叠、
// 不可改的格、识别码的占用，以及修订与请求的构造。租户、货主与商品都是隔离合成值，只记 `S`。

func goodsLevel(t *testing.T, name string, factor int64) domain.PackagingLevel {
	t.Helper()
	return domain.NewPackagingLevel(valueOf(t, domain.NewGoodsUnitName, name), factor)
}

func goodsAlias(t *testing.T, code string, kind domain.GoodsAliasKind, platform string, validity domain.AliasValidity) domain.GoodsAlias {
	t.Helper()
	var named domain.PlatformName
	if platform != "" {
		named = valueOf(t, domain.NewPlatformName, platform)
	}
	return domain.NewGoodsAlias(valueOf(t, domain.NewGoodsAliasCode, code), kind, named, validity)
}

func goodsComponent(t *testing.T, code string, quantity int64) domain.GoodsComponent {
	t.Helper()
	return domain.NewGoodsComponent(valueOf(t, domain.NewGoodsCode, code), quantity)
}

func goodsDimensions(t *testing.T, length, width, height, unit string) *domain.DeclaredDimensions {
	t.Helper()
	dimensions, err := domain.NewDeclaredDimensions(
		measureOf(t, length), measureOf(t, width), measureOf(t, height), valueOf(t, domain.NewUnitCode, unit))
	if err != nil {
		t.Fatalf("NewDeclaredDimensions: %v", err)
	}
	return &dimensions
}

func goodsWeight(t *testing.T, value, unit string) *domain.DeclaredWeight {
	t.Helper()
	weight, err := domain.NewDeclaredWeight(measureOf(t, value), valueOf(t, domain.NewUnitCode, unit))
	if err != nil {
		t.Fatalf("NewDeclaredWeight: %v", err)
	}
	return &weight
}

func goodsMeasurement(t *testing.T, level string, dimensions *domain.DeclaredDimensions, weight *domain.DeclaredWeight) domain.DeclaredMeasurement {
	t.Helper()
	measurement, err := domain.NewDeclaredMeasurement(domain.DeclaredMeasurementSpec{
		Level: valueOf(t, domain.NewGoodsUnitName, level), Dimensions: dimensions, Weight: weight,
	})
	if err != nil {
		t.Fatalf("NewDeclaredMeasurement: %v", err)
	}
	return measurement
}

func goodsSpecOf(t *testing.T) domain.GoodsContentSpec {
	t.Helper()
	return domain.GoodsContentSpec{
		Name:     valueOf(t, domain.NewGoodsName, "合成商品"),
		BaseUnit: valueOf(t, domain.NewGoodsUnitName, "件"),
		PackagingLevels: []domain.PackagingLevel{
			goodsLevel(t, "件", 1), goodsLevel(t, "内盒", 6), goodsLevel(t, "箱", 24),
		},
		Aliases: []domain.GoodsAlias{
			goodsAlias(t, "SYN-BAR-01", domain.GoodsAliasBarcode, "", domain.AliasValidity{}),
			goodsAlias(t, "SYN-FNSKU-01", domain.GoodsAliasPlatformCode, "SYN-PLATFORM", domain.NewAliasValidity(
				time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))),
		},
		DeclaredMeasurements: []domain.DeclaredMeasurement{
			goodsMeasurement(t, "件", goodsDimensions(t, "10", "5", "2.5", "CM"), goodsWeight(t, "0.5", "KG")),
			goodsMeasurement(t, "箱", nil, goodsWeight(t, "12", "KG")),
		},
	}
}

func goodsContentOf(t *testing.T, edit func(*domain.GoodsContentSpec)) domain.GoodsContent {
	t.Helper()
	spec := goodsSpecOf(t)
	if edit != nil {
		edit(&spec)
	}
	content, err := domain.NewGoodsContent(spec)
	if err != nil {
		t.Fatalf("NewGoodsContent: %v", err)
	}
	return content
}

func ownerSubmission(t *testing.T, key string) domain.GoodsSubmission {
	t.Helper()
	submission, err := domain.NewOwnerSubmission(
		valueOf(t, domain.NewSubmissionSource, "SYN-CHANNEL-01"), valueOf(t, domain.NewRequestKey, key))
	if err != nil {
		t.Fatalf("NewOwnerSubmission: %v", err)
	}
	return submission
}

func onBehalfSubmission(t *testing.T, key, evidence string) domain.GoodsSubmission {
	t.Helper()
	submission, err := domain.NewOnBehalfSubmission(
		valueOf(t, domain.NewRequestKey, key), valueOf(t, domain.NewEvidenceReference, evidence))
	if err != nil {
		t.Fatalf("NewOnBehalfSubmission: %v", err)
	}
	return submission
}

func goodsRevisionOf(t *testing.T, code string, number int, content domain.GoodsContent, submission domain.GoodsSubmission) domain.GoodsRevision {
	t.Helper()
	revision, err := domain.NewGoodsRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"),
		valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		valueOf(t, domain.NewGoodsCode, code),
		number, content, submission,
		valueOf(t, domain.NewRegistrant, "operator-1"), registeredAt,
	)
	if err != nil {
		t.Fatalf("NewGoodsRevision: %v", err)
	}
	return revision
}

// Covers: AT-WF-014、AT-WF-017（商品的核心）「商品编码、别名、层名与组件编码逐字保存」——带首尾空白、内部连续空白与大小写混写的取值原样留下，
// 只差大小写或空白的是不同的值；空白串与存不下的串（NUL、非 UTF-8）在构造处拒，免得到库里才以技术失败露面。
func TestGoodsValuesKeepEveryByteAndRefuseBlankOrUnstorable(t *testing.T) {
	constructors := map[string]func(string) (string, error){
		"货主客户账户": func(raw string) (string, error) {
			value, err := domain.NewOwnerAccountID(raw)
			return value.String(), err
		},
		"商品编码": func(raw string) (string, error) { value, err := domain.NewGoodsCode(raw); return value.String(), err },
		"商品名称": func(raw string) (string, error) { value, err := domain.NewGoodsName(raw); return value.String(), err },
		"单位名": func(raw string) (string, error) {
			value, err := domain.NewGoodsUnitName(raw)
			return value.String(), err
		},
		"别名的码": func(raw string) (string, error) {
			value, err := domain.NewGoodsAliasCode(raw)
			return value.String(), err
		},
		"识别码": func(raw string) (string, error) {
			value, err := domain.NewIdentifierCode(raw)
			return value.String(), err
		},
		"平台或渠道名": func(raw string) (string, error) {
			value, err := domain.NewPlatformName(raw)
			return value.String(), err
		},
		"请求键": func(raw string) (string, error) { value, err := domain.NewRequestKey(raw); return value.String(), err },
		"来源": func(raw string) (string, error) {
			value, err := domain.NewSubmissionSource(raw)
			return value.String(), err
		},
		"依据引用": func(raw string) (string, error) {
			value, err := domain.NewEvidenceReference(raw)
			return value.String(), err
		},
		"申报品名": func(raw string) (string, error) {
			value, err := domain.NewDeclaredGoodsName(raw)
			return value.String(), err
		},
		"HS 编码": func(raw string) (string, error) { value, err := domain.NewHSCode(raw); return value.String(), err },
		"币种码": func(raw string) (string, error) {
			value, err := domain.NewCurrencyCode(raw)
			return value.String(), err
		},
	}
	for name, construct := range constructors {
		for _, raw := range []string{"  Sku-Mixed  01 ", "sku-1", "SKU-1", "SKU-1\t", "货主\n编码"} {
			got, err := construct(raw)
			if err != nil || got != raw {
				t.Errorf("%s %q：得 (%q, %v)，want 原样保存", name, raw, got, err)
			}
		}
		for _, raw := range []string{"", "   ", "\t\n"} {
			if _, err := construct(raw); !errors.Is(err, domain.ErrBlankValue) {
				t.Errorf("%s %q：err = %v，want ErrBlankValue", name, raw, err)
			}
		}
		for _, raw := range []string{"a\x00b", "\xff\xfe"} {
			if _, err := construct(raw); !errors.Is(err, domain.ErrUnstorableValue) {
				t.Errorf("%s %q：err = %v，want ErrUnstorableValue", name, raw, err)
			}
		}
	}
}

// 封闭集的词按字面认：别名类型、出库方式与提交途径，大小写不对、带空白、集外的词都不是；零值没有字面。
func TestGoodsClosedSetsAreRecognisedByLiteralWord(t *testing.T) {
	for word, want := range map[string]domain.GoodsAliasKind{"BARCODE": domain.GoodsAliasBarcode, "PLATFORM_CODE": domain.GoodsAliasPlatformCode} {
		if kind, err := domain.NewGoodsAliasKind(word); err != nil || kind != want || kind.String() != word {
			t.Errorf("别名类型 %q = (%v, %v)", word, kind, err)
		}
	}
	for word, want := range map[string]domain.GoodsOutboundMode{"BY_COMPONENT": domain.GoodsOutboundByComponent, "AS_FINISHED_GOODS": domain.GoodsOutboundAsFinishedGoods} {
		if mode, err := domain.NewGoodsOutboundMode(word); err != nil || mode != want || mode.String() != word {
			t.Errorf("出库方式 %q = (%v, %v)", word, mode, err)
		}
	}
	for word, want := range map[string]domain.SubmissionPath{"OWNER": domain.SubmissionByOwner, "ON_BEHALF": domain.SubmissionOnBehalf} {
		if path, err := domain.NewSubmissionPath(word); err != nil || path != want || path.String() != word {
			t.Errorf("提交途径 %q = (%v, %v)", word, path, err)
		}
	}
	for _, word := range []string{"", "barcode", " BARCODE", "BARCODE ", "QR", "SKU", "by_component", "BY_COMPONENT\n", "owner", "OPERATOR"} {
		if _, err := domain.NewGoodsAliasKind(word); !errors.Is(err, domain.ErrUnknownGoodsWord) {
			t.Errorf("别名类型 %q 应不被认", word)
		}
		if _, err := domain.NewGoodsOutboundMode(word); !errors.Is(err, domain.ErrUnknownGoodsWord) {
			t.Errorf("出库方式 %q 应不被认", word)
		}
		if _, err := domain.NewSubmissionPath(word); !errors.Is(err, domain.ErrUnknownGoodsWord) {
			t.Errorf("提交途径 %q 应不被认", word)
		}
	}
	if domain.GoodsAliasKindInvalid.String() != "" || domain.GoodsOutboundModeInvalid.String() != "" || domain.SubmissionPathInvalid.String() != "" {
		t.Errorf("封闭集的零值不该有字面")
	}
}

// Covers: AT-WF-017（商品的核心）「没给起或没给止的那一端按无界算的各种交叠情形」——区间起含、止不含：首尾相接不交叠；两端无界与任何区间交叠；只给起、
// 只给止的各种位置；一端无界的两个区间只有在无界方向上同向时必交叠；不成立的区间（止不晚于起）不占任何时刻。
func TestAliasValidityOverlapsAsHalfOpenIntervalsWithUnboundedEnds(t *testing.T) {
	at := func(day int) time.Time { return time.Date(2026, 6, day, 0, 0, 0, 0, time.UTC) }
	between := func(from, to int) domain.AliasValidity { return domain.NewAliasValidity(at(from), at(to)) }
	from := func(day int) domain.AliasValidity { return domain.NewAliasValidity(at(day), time.Time{}) }
	until := func(day int) domain.AliasValidity { return domain.NewAliasValidity(time.Time{}, at(day)) }
	unbounded := domain.AliasValidity{}

	for name, test := range map[string]struct {
		a, b domain.AliasValidity
		want bool
	}{
		"两端无界与两端无界":      {unbounded, unbounded, true},
		"两端无界与有界区间":      {unbounded, between(5, 6), true},
		"有界区间与两端无界":      {between(5, 6), unbounded, true},
		"有界区间部分交叠":       {between(1, 10), between(5, 15), true},
		"一个区间包住另一个":      {between(1, 20), between(5, 6), true},
		"首尾相接不交叠（止不含）":   {between(1, 5), between(5, 10), false},
		"首尾相接反过来也不交叠":    {between(5, 10), between(1, 5), false},
		"完全分开":           {between(1, 3), between(5, 8), false},
		"只给起的与更晚开始的有界区间": {from(1), between(5, 8), true},
		"只给起的与更早结束的有界区间": {from(10), between(1, 5), false},
		"只给起的与恰在起处结束的区间": {from(5), between(1, 5), false},
		"只给起的与恰在起处开始的区间": {from(5), between(5, 6), true},
		"只给止的与更早的有界区间":   {until(10), between(1, 5), true},
		"只给止的与更晚开始的有界区间": {until(5), between(7, 9), false},
		"只给止的与恰在止处开始的区间": {until(5), between(5, 9), false},
		"只给止的与恰在止前开始的区间": {until(5), between(4, 9), true},
		"只给起的与只给起的":      {from(1), from(30), true},
		"只给止的与只给止的":      {until(1), until(30), true},
		"只给起的与只给止的，起早于止": {from(1), until(10), true},
		"只给起的与只给止的，起晚于止": {from(10), until(1), false},
		"只给起的与只给止的，起等于止": {from(5), until(5), false},
		"只给起的与两端无界":      {from(5), unbounded, true},
		"只给止的与两端无界":      {until(5), unbounded, true},
		"不成立的区间与两端无界":    {between(10, 5), unbounded, false},
		"止等于起的区间也是空的":    {between(5, 5), unbounded, false},
		"两个不成立的区间":       {between(10, 5), between(9, 4), false},
	} {
		if got := test.a.Overlaps(test.b); got != test.want {
			t.Errorf("%s：Overlaps = %v，want %v", name, got, test.want)
		}
		if got := test.b.Overlaps(test.a); got != test.want {
			t.Errorf("%s（调换）：Overlaps = %v，want %v", name, got, test.want)
		}
	}

	if !between(10, 5).Inverted() || !between(5, 5).Inverted() || between(5, 6).Inverted() || from(5).Inverted() || until(5).Inverted() || unbounded.Inverted() {
		t.Errorf("Inverted 只该在两端都给且止不晚于起时为真")
	}
	validity := between(10, 20)
	for day, want := range map[int]bool{9: false, 10: true, 15: true, 19: true, 20: false, 21: false} {
		if got := validity.Contains(at(day)); got != want {
			t.Errorf("[10, 20) 含第 %d 天 = %v，want %v", day, got, want)
		}
	}
	if from(10).Contains(at(9)) || !from(10).Contains(at(10)) || !from(10).Contains(at(28)) {
		t.Errorf("只给起：起含，往后无界")
	}
	if until(10).Contains(at(10)) || !until(10).Contains(at(9)) || !until(10).Contains(at(1)) {
		t.Errorf("只给止：止不含，往前无界")
	}
	if !unbounded.Contains(at(1)) || !unbounded.Contains(time.Time{}.Add(time.Hour)) {
		t.Errorf("两端无界含任何时刻")
	}
}

// 有效区间的时刻统一到 UTC、收成微秒：换一个时区写的同一时刻与纳秒零头不造成两个不同的区间。
func TestAliasValidityNormalisesInstantsToUTCMicroseconds(t *testing.T) {
	plus8 := time.FixedZone("UTC+8", 8*3600)
	a := domain.NewAliasValidity(time.Date(2026, 6, 1, 8, 0, 0, 123456789, plus8), time.Time{})
	b := domain.NewAliasValidity(time.Date(2026, 6, 1, 0, 0, 0, 123456000, time.UTC), time.Time{})
	from, hasFrom := a.From()
	if !hasFrom || !from.Equal(time.Date(2026, 6, 1, 0, 0, 0, 123456000, time.UTC)) || from.Location() != time.UTC {
		t.Fatalf("起 = %v（%v），want UTC 微秒精度", from, hasFrom)
	}
	if _, hasTo := a.To(); hasTo {
		t.Fatalf("没给止，不该有止")
	}
	left := goodsAlias(t, "SYN-X", domain.GoodsAliasBarcode, "", a)
	right := goodsAlias(t, "SYN-X", domain.GoodsAliasBarcode, "", b)
	contentA := goodsContentOf(t, func(spec *domain.GoodsContentSpec) { spec.Aliases = []domain.GoodsAlias{left} })
	contentB := goodsContentOf(t, func(spec *domain.GoodsContentSpec) { spec.Aliases = []domain.GoodsAlias{right} })
	if !contentA.SameAs(contentB) {
		t.Errorf("换时区写的同一时刻该是同一内容")
	}
}

// Covers: AT-WF-014（商品的核心）「列表次序不同的同一份资料是同一内容」——别名、包装层级、组件与声明的尺寸重量换个先后仍是同一内容；读回保持交来的先后。
func TestGoodsContentSameAsIgnoresTheOrderOfLists(t *testing.T) {
	original := goodsContentOf(t, func(spec *domain.GoodsContentSpec) {
		spec.Composite = true
		spec.OutboundMode = domain.GoodsOutboundByComponent
		spec.Components = []domain.GoodsComponent{goodsComponent(t, "SYN-A", 2), goodsComponent(t, "SYN-B", 3)}
	})
	reordered := goodsContentOf(t, func(spec *domain.GoodsContentSpec) {
		spec.Composite = true
		spec.OutboundMode = domain.GoodsOutboundByComponent
		spec.Components = []domain.GoodsComponent{goodsComponent(t, "SYN-B", 3), goodsComponent(t, "SYN-A", 2)}
		spec.PackagingLevels = []domain.PackagingLevel{goodsLevel(t, "箱", 24), goodsLevel(t, "件", 1), goodsLevel(t, "内盒", 6)}
		spec.Aliases = []domain.GoodsAlias{spec.Aliases[1], spec.Aliases[0]}
		spec.DeclaredMeasurements = []domain.DeclaredMeasurement{spec.DeclaredMeasurements[1], spec.DeclaredMeasurements[0]}
	})
	if !original.SameAs(reordered) || !reordered.SameAs(original) {
		t.Fatalf("换了列表先后的同一份资料该是同一内容")
	}
	if got := reordered.PackagingLevels(); got[0].Name().String() != "箱" || got[1].Name().String() != "件" {
		t.Errorf("读回的包装层级先后 = %v，want 交来的先后（箱、件、内盒）", got)
	}
	if got := reordered.Components(); got[0].Code().String() != "SYN-B" || got[1].Code().String() != "SYN-A" {
		t.Errorf("读回的组件先后不是交来的先后")
	}
}

// 「内容相同」逐格看得出差别：每一格改动都不再是同一内容，改的格里没有一格被先后的宽容吞掉。
func TestGoodsContentSameAsNoticesEveryChangedCell(t *testing.T) {
	base := goodsContentOf(t, nil)
	for name, edit := range map[string]func(*domain.GoodsContentSpec){
		"名称": func(spec *domain.GoodsContentSpec) { spec.Name = valueOf(t, domain.NewGoodsName, "另一个名称") },
		"基本单位": func(spec *domain.GoodsContentSpec) {
			spec.BaseUnit = valueOf(t, domain.NewGoodsUnitName, "盒")
			spec.PackagingLevels = []domain.PackagingLevel{goodsLevel(t, "盒", 1), goodsLevel(t, "内盒", 6), goodsLevel(t, "箱", 24)}
		},
		"层的折合数": func(spec *domain.GoodsContentSpec) {
			spec.PackagingLevels = []domain.PackagingLevel{goodsLevel(t, "件", 1), goodsLevel(t, "内盒", 6), goodsLevel(t, "箱", 12)}
		},
		"少一层": func(spec *domain.GoodsContentSpec) { spec.PackagingLevels = spec.PackagingLevels[:2] },
		"多一层": func(spec *domain.GoodsContentSpec) {
			spec.PackagingLevels = append(spec.PackagingLevels, goodsLevel(t, "托", 480))
		},
		"跟踪批号":  func(spec *domain.GoodsContentSpec) { spec.TrackBatch = true },
		"跟踪效期":  func(spec *domain.GoodsContentSpec) { spec.TrackExpiry = true },
		"少一个别名": func(spec *domain.GoodsContentSpec) { spec.Aliases = spec.Aliases[:1] },
		"别名的码": func(spec *domain.GoodsContentSpec) {
			spec.Aliases[0] = goodsAlias(t, "SYN-BAR-02", domain.GoodsAliasBarcode, "", domain.AliasValidity{})
		},
		"别名的类型": func(spec *domain.GoodsContentSpec) {
			spec.Aliases[0] = goodsAlias(t, "SYN-BAR-01", domain.GoodsAliasPlatformCode, "", domain.AliasValidity{})
		},
		"别名的平台名": func(spec *domain.GoodsContentSpec) {
			spec.Aliases[1] = goodsAlias(t, "SYN-FNSKU-01", domain.GoodsAliasPlatformCode, "SYN-OTHER", spec.Aliases[1].Validity())
		},
		"别名的区间起": func(spec *domain.GoodsContentSpec) {
			spec.Aliases[1] = goodsAlias(t, "SYN-FNSKU-01", domain.GoodsAliasPlatformCode, "SYN-PLATFORM", domain.NewAliasValidity(
				time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)))
		},
		"别名的区间止": func(spec *domain.GoodsContentSpec) {
			spec.Aliases[1] = goodsAlias(t, "SYN-FNSKU-01", domain.GoodsAliasPlatformCode, "SYN-PLATFORM", domain.NewAliasValidity(
				time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{}))
		},
		"是否组合": func(spec *domain.GoodsContentSpec) {
			spec.Composite = true
			spec.OutboundMode = domain.GoodsOutboundByComponent
			spec.Components = []domain.GoodsComponent{goodsComponent(t, "SYN-A", 1)}
		},
		"申报品名": func(spec *domain.GoodsContentSpec) {
			spec.Declaration = declarationOf(t, domain.GoodsDeclarationSpec{Name: valueOf(t, domain.NewDeclaredGoodsName, "T-shirt")})
		},
		"声明的尺寸": func(spec *domain.GoodsContentSpec) {
			spec.DeclaredMeasurements[0] = goodsMeasurement(t, "件", goodsDimensions(t, "10", "5", "2.6", "CM"), goodsWeight(t, "0.5", "KG"))
		},
		"声明的重量单位": func(spec *domain.GoodsContentSpec) {
			spec.DeclaredMeasurements[0] = goodsMeasurement(t, "件", goodsDimensions(t, "10", "5", "2.5", "CM"), goodsWeight(t, "0.5", "G"))
		},
		"声明的层级": func(spec *domain.GoodsContentSpec) {
			spec.DeclaredMeasurements[1] = goodsMeasurement(t, "内盒", nil, goodsWeight(t, "12", "KG"))
		},
		"少声明一条": func(spec *domain.GoodsContentSpec) { spec.DeclaredMeasurements = spec.DeclaredMeasurements[:1] },
		"尺寸变成没声明": func(spec *domain.GoodsContentSpec) {
			spec.DeclaredMeasurements[0] = goodsMeasurement(t, "件", nil, goodsWeight(t, "0.5", "KG"))
		},
	} {
		edited := goodsContentOf(t, edit)
		if base.SameAs(edited) || edited.SameAs(base) {
			t.Errorf("改了%s，不该仍是同一内容", name)
		}
	}
}

func declarationOf(t *testing.T, spec domain.GoodsDeclarationSpec) domain.GoodsDeclaration {
	t.Helper()
	declaration, err := domain.NewGoodsDeclaration(spec)
	if err != nil {
		t.Fatalf("NewGoodsDeclaration: %v", err)
	}
	return declaration
}

// 数值按数值比、码与名逐字比：声明的尺寸 10.0 与 10 同一内容，单位码 CM 与 cm 不同；申报单价 5.50 与 5.5 同一内容。
func TestGoodsContentSameAsComparesNumbersByValueAndCodesVerbatim(t *testing.T) {
	declared := func(price string) domain.GoodsDeclaration {
		return declarationOf(t, domain.GoodsDeclarationSpec{
			UnitPrice: measureOf(t, price), Currency: valueOf(t, domain.NewCurrencyCode, "USD"),
		})
	}
	base := goodsContentOf(t, func(spec *domain.GoodsContentSpec) { spec.Declaration = declared("5.5") })
	rewritten := goodsContentOf(t, func(spec *domain.GoodsContentSpec) {
		spec.Declaration = declared("05.50")
		spec.DeclaredMeasurements[0] = goodsMeasurement(t, "件", goodsDimensions(t, "10.0", "5.00", "2.50", "CM"), goodsWeight(t, "0.50", "KG"))
	})
	if !base.SameAs(rewritten) {
		t.Errorf("数值换一种写法该是同一内容")
	}
	lower := goodsContentOf(t, func(spec *domain.GoodsContentSpec) {
		spec.Declaration = declared("5.5")
		spec.DeclaredMeasurements[0] = goodsMeasurement(t, "件", goodsDimensions(t, "10", "5", "2.5", "cm"), goodsWeight(t, "0.5", "KG"))
	})
	if base.SameAs(lower) {
		t.Errorf("单位码 CM 与 cm 是不同的内容")
	}
}

// 必填格没给：名称与基本单位、各列表项里的必填格；缺哪一格就指出哪一格，位置按交来的先后。可缺的格缺了不算缺。
func TestGoodsContentRequiresTheMandatoryCells(t *testing.T) {
	if _, err := domain.NewGoodsContent(domain.GoodsContentSpec{}); !errors.Is(err, domain.ErrInvalidGoodsContent) {
		t.Fatalf("空的资料：err = %v，want ErrInvalidGoodsContent", err)
	}
	var fieldError *domain.GoodsContentFieldError
	_, err := domain.NewGoodsContent(domain.GoodsContentSpec{})
	if !errors.As(err, &fieldError) || fieldError.Cell.String() != "name" {
		t.Fatalf("空的资料先指出 name：%v", err)
	}
	if got := (domain.GoodsContent{}).MissingCells(); len(got) != 2 || got[0].String() != "name" || got[1].String() != "baseUnit" {
		t.Fatalf("零值资料缺的格 = %v，want name、baseUnit", got)
	}

	// 各列表项里的必填格：零值的列表项没经构造器，缺哪一格就指出哪一格，位置按交来的先后。
	for name, test := range map[string]struct {
		edit func(*domain.GoodsContentSpec)
		want string
	}{
		"层名": {func(spec *domain.GoodsContentSpec) {
			spec.PackagingLevels = append(spec.PackagingLevels, domain.NewPackagingLevel(domain.GoodsUnitName{}, 2))
		}, "packagingLevels[3].name"},
		"别名的码": {func(spec *domain.GoodsContentSpec) {
			spec.Aliases = append(spec.Aliases, domain.NewGoodsAlias(domain.GoodsAliasCode{}, domain.GoodsAliasBarcode, domain.PlatformName{}, domain.AliasValidity{}))
		}, "aliases[2].code"},
		"别名的类型": {func(spec *domain.GoodsContentSpec) {
			spec.Aliases = append(spec.Aliases, goodsAlias(t, "SYN-NOKIND", domain.GoodsAliasKindInvalid, "", domain.AliasValidity{}))
		}, "aliases[2].type"},
		"组件的商品编码": {func(spec *domain.GoodsContentSpec) {
			spec.Composite = true
			spec.OutboundMode = domain.GoodsOutboundByComponent
			spec.Components = []domain.GoodsComponent{goodsComponent(t, "SYN-A", 1), domain.NewGoodsComponent(domain.GoodsCode{}, 1)}
		}, "composition.components[1].code"},
		"声明的尺寸重量的层级": {func(spec *domain.GoodsContentSpec) {
			noLevel, err := domain.NewDeclaredMeasurement(domain.DeclaredMeasurementSpec{Weight: goodsWeight(t, "1", "KG")})
			if err != nil {
				t.Fatal(err)
			}
			spec.DeclaredMeasurements = append(spec.DeclaredMeasurements, noLevel)
		}, "declaredMeasurements[2].level"},
	} {
		spec := goodsSpecOf(t)
		test.edit(&spec)
		_, err := domain.NewGoodsContent(spec)
		if !errors.As(err, &fieldError) || fieldError.Cell.String() != test.want {
			t.Errorf("%s缺了：err = %v，want 指出 %s", name, err, test.want)
		}
	}

	optional := goodsContentOf(t, func(spec *domain.GoodsContentSpec) {
		spec.PackagingLevels, spec.Aliases, spec.DeclaredMeasurements = nil, nil, nil
	})
	if len(optional.MissingCells()) != 0 || len(optional.Aliases()) != 0 || optional.Declaration() != (domain.GoodsDeclaration{}) {
		t.Errorf("别名、层级、声明与尺寸重量都可缺")
	}
}

// 构造器复制各列表：调用方之后改自己的切片，不动已构造的资料；读回的切片也是副本。
func TestGoodsContentCopiesItsLists(t *testing.T) {
	spec := goodsSpecOf(t)
	content, err := domain.NewGoodsContent(spec)
	if err != nil {
		t.Fatal(err)
	}
	spec.PackagingLevels[0] = goodsLevel(t, "被改掉的", 99)
	spec.Aliases = spec.Aliases[:0]
	read := content.PackagingLevels()
	read[1] = goodsLevel(t, "读回之后改", 77)
	if got := content.PackagingLevels(); got[0].Name().String() != "件" || got[1].Name().String() != "内盒" || len(content.Aliases()) != 2 {
		t.Errorf("资料被调用方的切片改动：%v", got)
	}
}

// 申报资料：申报单价与币种同在同缺（只给一个是报文形状不合）；整份不带与各格都没给是同一回事；各格逐字。
func TestGoodsDeclarationPairsPriceWithCurrencyAndEmptyMeansNotDeclared(t *testing.T) {
	price := measureOf(t, "5.50")
	currency := valueOf(t, domain.NewCurrencyCode, "USD")
	if _, err := domain.NewGoodsDeclaration(domain.GoodsDeclarationSpec{UnitPrice: price}); !errors.Is(err, domain.ErrInvalidGoodsDeclaration) {
		t.Errorf("只给单价：err = %v", err)
	}
	if _, err := domain.NewGoodsDeclaration(domain.GoodsDeclarationSpec{Currency: currency}); !errors.Is(err, domain.ErrInvalidGoodsDeclaration) {
		t.Errorf("只给币种：err = %v", err)
	}
	full := declarationOf(t, domain.GoodsDeclarationSpec{
		Name: valueOf(t, domain.NewDeclaredGoodsName, " T-shirt "), HSCode: valueOf(t, domain.NewHSCode, "6109.10"),
		OriginCountry: valueOf(t, domain.NewCountryCode, "CN"), UnitPrice: price, Currency: currency,
	})
	if name, has := full.Name(); !has || name.String() != " T-shirt " {
		t.Errorf("申报品名 = %q（%v），want 逐字", name, has)
	}
	if hs, has := full.HSCode(); !has || hs.String() != "6109.10" {
		t.Errorf("HS 编码 = %q（%v），want 逐字，不判归类", hs, has)
	}
	if got, gotCurrency, has := full.UnitPrice(); !has || got.String() != "5.5" || gotCurrency.String() != "USD" {
		t.Errorf("申报单价 = %s %s（%v）", got, gotCurrency, has)
	}
	empty := declarationOf(t, domain.GoodsDeclarationSpec{})
	if _, has := empty.Name(); has {
		t.Errorf("没给的格不该有")
	}
	if _, _, has := empty.UnitPrice(); has {
		t.Errorf("没给单价与币种，不该有")
	}
	if empty != (domain.GoodsDeclaration{}) {
		t.Errorf("整份不带与各格都没给应是同一个值")
	}
}

// 声明的尺寸重量：尺寸四格一起给、重量两格一起给、一条至少给一样；各样可缺。
func TestDeclaredMeasurementRequiresWholeGroupsAndAtLeastOne(t *testing.T) {
	unit := valueOf(t, domain.NewUnitCode, "CM")
	if _, err := domain.NewDeclaredDimensions(measureOf(t, "1"), measureOf(t, "2"), domain.Measure{}, unit); !errors.Is(err, domain.ErrInvalidDeclaredMeasurement) {
		t.Errorf("尺寸缺高：err = %v", err)
	}
	if _, err := domain.NewDeclaredDimensions(measureOf(t, "1"), measureOf(t, "2"), measureOf(t, "3"), domain.UnitCode{}); !errors.Is(err, domain.ErrInvalidDeclaredMeasurement) {
		t.Errorf("尺寸缺单位：err = %v", err)
	}
	if _, err := domain.NewDeclaredWeight(domain.Measure{}, valueOf(t, domain.NewUnitCode, "KG")); !errors.Is(err, domain.ErrInvalidDeclaredMeasurement) {
		t.Errorf("重量缺数值：err = %v", err)
	}
	if _, err := domain.NewDeclaredWeight(measureOf(t, "1"), domain.UnitCode{}); !errors.Is(err, domain.ErrInvalidDeclaredMeasurement) {
		t.Errorf("重量缺单位：err = %v", err)
	}
	if _, err := domain.NewDeclaredMeasurement(domain.DeclaredMeasurementSpec{Level: valueOf(t, domain.NewGoodsUnitName, "件")}); !errors.Is(err, domain.ErrInvalidDeclaredMeasurement) {
		t.Errorf("两样都没给：err = %v", err)
	}
	onlyWeight := goodsMeasurement(t, "件", nil, goodsWeight(t, "1", "KG"))
	if _, has := onlyWeight.Dimensions(); has {
		t.Errorf("没声明尺寸，不该有")
	}
	if weight, has := onlyWeight.Weight(); !has || weight.Value().String() != "1" {
		t.Errorf("重量 = %v（%v）", weight, has)
	}
}

// Covers: AT-WF-016（商品的核心）——结构上不可改的格，次序固定：基本单位，是否组合，再按在册那一笔里各层的先后看（先看删没删、再看折合数）；追加层级不算改；
// 包装层级按单位名认；标量与别名等别的格改了不算。
func TestGoodsContentStructuralChangeFromFindsTheFirstImmutableCell(t *testing.T) {
	previous := goodsContentOf(t, nil)
	if _, changed := previous.StructuralChangeFrom(previous); changed {
		t.Fatalf("没改不该有不可改的格")
	}
	appended := goodsContentOf(t, func(spec *domain.GoodsContentSpec) {
		spec.PackagingLevels = append(spec.PackagingLevels, goodsLevel(t, "托", 480))
	})
	if cell, changed := appended.StructuralChangeFrom(previous); changed {
		t.Errorf("追加包装层级不算改：%s", cell)
	}
	renamed := goodsContentOf(t, func(spec *domain.GoodsContentSpec) {
		spec.Name = valueOf(t, domain.NewGoodsName, "改了名称")
		spec.Aliases = nil
		spec.TrackBatch = true
	})
	if cell, changed := renamed.StructuralChangeFrom(previous); changed {
		t.Errorf("名称、别名与跟踪属性不是结构格：%s", cell)
	}

	for name, test := range map[string]struct {
		edit      func(*domain.GoodsContentSpec)
		wantField domain.GoodsImmutableField
		wantLevel string
	}{
		"换基本单位": {func(spec *domain.GoodsContentSpec) {
			spec.BaseUnit = valueOf(t, domain.NewGoodsUnitName, "盒")
			spec.PackagingLevels = []domain.PackagingLevel{goodsLevel(t, "盒", 1), goodsLevel(t, "内盒", 6), goodsLevel(t, "箱", 24)}
		}, domain.GoodsImmutableBaseUnit, ""},
		"非组合改成组合": {func(spec *domain.GoodsContentSpec) {
			spec.Composite = true
			spec.OutboundMode = domain.GoodsOutboundAsFinishedGoods
			spec.Components = []domain.GoodsComponent{goodsComponent(t, "SYN-A", 1)}
		}, domain.GoodsImmutableComposite, ""},
		"删掉一个层级": {func(spec *domain.GoodsContentSpec) {
			spec.PackagingLevels = []domain.PackagingLevel{goodsLevel(t, "件", 1), goodsLevel(t, "箱", 24)}
		}, domain.GoodsImmutablePackagingLevelRemoved, "内盒"},
		"改层级名等于删掉旧层": {func(spec *domain.GoodsContentSpec) {
			spec.PackagingLevels = []domain.PackagingLevel{goodsLevel(t, "件", 1), goodsLevel(t, "内盒", 6), goodsLevel(t, "纸箱", 24)}
		}, domain.GoodsImmutablePackagingLevelRemoved, "箱"},
		"改一个层级的折合数": {func(spec *domain.GoodsContentSpec) {
			spec.PackagingLevels = []domain.PackagingLevel{goodsLevel(t, "件", 1), goodsLevel(t, "内盒", 6), goodsLevel(t, "箱", 12)}
		}, domain.GoodsImmutablePackagingLevelFactor, "箱"},
	} {
		edited := goodsContentOf(t, test.edit)
		cell, changed := edited.StructuralChangeFrom(previous)
		if !changed || cell.Field() != test.wantField {
			t.Errorf("%s = (%v, %v)，want %s", name, cell, changed, test.wantField)
			continue
		}
		level, named := cell.Level()
		if test.wantLevel == "" && named || test.wantLevel != "" && (!named || level.String() != test.wantLevel) {
			t.Errorf("%s 指出的层 = %q（%v），want %q", name, level, named, test.wantLevel)
		}
	}

	both := goodsContentOf(t, func(spec *domain.GoodsContentSpec) {
		spec.BaseUnit = valueOf(t, domain.NewGoodsUnitName, "盒")
		spec.PackagingLevels = []domain.PackagingLevel{goodsLevel(t, "盒", 1)}
		spec.Composite = true
		spec.OutboundMode = domain.GoodsOutboundByComponent
		spec.Components = []domain.GoodsComponent{goodsComponent(t, "SYN-A", 1)}
	})
	if cell, _ := both.StructuralChangeFrom(previous); cell.Field() != domain.GoodsImmutableBaseUnit {
		t.Errorf("几处同时改，先指出基本单位，实得 %s", cell)
	}
	reordered := goodsContentOf(t, func(spec *domain.GoodsContentSpec) {
		spec.PackagingLevels = []domain.PackagingLevel{goodsLevel(t, "箱", 24), goodsLevel(t, "内盒", 6), goodsLevel(t, "件", 1)}
	})
	if cell, changed := reordered.StructuralChangeFrom(previous); changed {
		t.Errorf("层级换了先后不算改：%s", cell)
	}
	for field, want := range map[domain.GoodsImmutableField]string{
		domain.GoodsImmutableBaseUnit: "baseUnit", domain.GoodsImmutableComposite: "composite",
		domain.GoodsImmutablePackagingLevelRemoved: "packagingLevels", domain.GoodsImmutablePackagingLevelFactor: "packagingLevels.factor",
		domain.GoodsImmutableTrackBatch: "trackBatch", domain.GoodsImmutableTrackExpiry: "trackExpiry",
	} {
		if field.String() != want {
			t.Errorf("%d 的字面 = %q，want %q", field, field.String(), want)
		}
	}
}

// Covers: AT-WF-016（商品的核心）——是否跟踪批号、效期单独判：先看批号再看效期，没改不指出；它们只在没有在库库存时才可改，所以与结构格分开。
func TestGoodsContentTrackingChangeFromFindsBatchBeforeExpiry(t *testing.T) {
	previous := goodsContentOf(t, nil)
	if _, changed := previous.TrackingChangeFrom(previous); changed {
		t.Fatalf("没改不该有")
	}
	batch := goodsContentOf(t, func(spec *domain.GoodsContentSpec) { spec.TrackBatch = true })
	if cell, changed := batch.TrackingChangeFrom(previous); !changed || cell.Field() != domain.GoodsImmutableTrackBatch {
		t.Errorf("改批号 = (%v, %v)", cell, changed)
	}
	expiry := goodsContentOf(t, func(spec *domain.GoodsContentSpec) { spec.TrackExpiry = true })
	if cell, changed := expiry.TrackingChangeFrom(previous); !changed || cell.Field() != domain.GoodsImmutableTrackExpiry {
		t.Errorf("改效期 = (%v, %v)", cell, changed)
	}
	both := goodsContentOf(t, func(spec *domain.GoodsContentSpec) { spec.TrackBatch, spec.TrackExpiry = true, true })
	if cell, _ := both.TrackingChangeFrom(previous); cell.Field() != domain.GoodsImmutableTrackBatch {
		t.Errorf("两格同改先指出批号，实得 %s", cell)
	}
	if cell, changed := batch.TrackingChangeFrom(batch); changed {
		t.Errorf("同样跟踪不算改：%s", cell)
	}
}

// Covers: AT-WF-017（商品的核心）——识别码的占用：商品编码占它那个码、两端无界，别名占自己声明的区间；同一份资料里一个码出现两次（含别名等于自己的商品编码，
// 不论区间）按先出现两次的那个码指出；两处占用碰撞要同一个码且区间交叠。
func TestGoodsContentClaimsIdentifiersAndFindsRepeats(t *testing.T) {
	code := valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01")
	content := goodsContentOf(t, nil)
	claims := content.IdentifierClaims(code)
	if len(claims) != 3 || claims[0].Code().String() != "SYN-GOODS-01" || claims[1].Code().String() != "SYN-BAR-01" || claims[2].Code().String() != "SYN-FNSKU-01" {
		t.Fatalf("占用 = %v，want 商品编码在先、别名随后", claims)
	}
	if claims[0].Validity() != (domain.AliasValidity{}) {
		t.Errorf("商品编码该两端无界")
	}
	if repeated, repeats := content.RepeatedIdentifier(code); repeats {
		t.Errorf("没有重复的码，却指出了 %s", repeated)
	}

	own := goodsContentOf(t, func(spec *domain.GoodsContentSpec) {
		spec.Aliases = append(spec.Aliases, goodsAlias(t, "SYN-GOODS-01", domain.GoodsAliasBarcode, "", domain.AliasValidity{}))
	})
	if repeated, repeats := own.RepeatedIdentifier(code); !repeats || repeated.String() != "SYN-GOODS-01" {
		t.Errorf("别名等于自己的商品编码：(%q, %v)", repeated, repeats)
	}
	twice := goodsContentOf(t, func(spec *domain.GoodsContentSpec) {
		at := func(day int) time.Time { return time.Date(2026, 2, day, 0, 0, 0, 0, time.UTC) }
		spec.Aliases = append(spec.Aliases,
			goodsAlias(t, "SYN-DUP", domain.GoodsAliasBarcode, "", domain.NewAliasValidity(at(1), at(5))),
			goodsAlias(t, "SYN-DUP", domain.GoodsAliasBarcode, "", domain.NewAliasValidity(at(7), at(9))))
	})
	if repeated, repeats := twice.RepeatedIdentifier(code); !repeats || repeated.String() != "SYN-DUP" {
		t.Errorf("同一个码的两个别名（区间分开也算出现两次）：(%q, %v)", repeated, repeats)
	}
	caseOnly := goodsContentOf(t, func(spec *domain.GoodsContentSpec) {
		spec.Aliases = append(spec.Aliases, goodsAlias(t, "syn-goods-01", domain.GoodsAliasBarcode, "", domain.AliasValidity{}))
	})
	if _, repeats := caseOnly.RepeatedIdentifier(code); repeats {
		t.Errorf("只差大小写的码是另一个码，不算出现两次")
	}

	at := func(day int) time.Time { return time.Date(2026, 3, day, 0, 0, 0, 0, time.UTC) }
	held := goodsContentOf(t, func(spec *domain.GoodsContentSpec) {
		spec.Aliases = []domain.GoodsAlias{goodsAlias(t, "SYN-X", domain.GoodsAliasBarcode, "", domain.NewAliasValidity(at(1), at(10)))}
	}).IdentifierClaims(valueOf(t, domain.NewGoodsCode, "SYN-HELD"))
	asked := func(code string, validity domain.AliasValidity) domain.IdentifierClaim {
		return goodsContentOf(t, func(spec *domain.GoodsContentSpec) {
			spec.Aliases = []domain.GoodsAlias{goodsAlias(t, code, domain.GoodsAliasBarcode, "", validity)}
		}).IdentifierClaims(valueOf(t, domain.NewGoodsCode, "SYN-ASKER"))[1]
	}
	if !held[1].Collides(asked("SYN-X", domain.NewAliasValidity(at(5), at(15)))) {
		t.Errorf("同码、区间交叠该碰撞")
	}
	if held[1].Collides(asked("SYN-X", domain.NewAliasValidity(at(10), at(15)))) {
		t.Errorf("同码、区间首尾相接不碰撞")
	}
	if held[1].Collides(asked("SYN-Y", domain.AliasValidity{})) {
		t.Errorf("不同的码不碰撞")
	}
	if !held[0].Collides(asked("SYN-HELD", domain.NewAliasValidity(at(20), at(21)))) {
		t.Errorf("商品编码两端无界，与同码的任何区间都碰撞")
	}
}

// 格的键路径：列表里的格带位置，别的格不带；位置只对列表里的格有意义。
func TestGoodsCellWritesItsKeyPath(t *testing.T) {
	for _, test := range []struct {
		cell domain.GoodsCell
		want string
	}{
		{domain.NewGoodsCell(domain.GoodsFieldBaseUnit, 0), "baseUnit"},
		{domain.NewGoodsCell(domain.GoodsFieldBaseUnit, 7), "baseUnit"},
		{domain.NewGoodsCell(domain.GoodsFieldPackagingLevels, 0), "packagingLevels"},
		{domain.NewGoodsCell(domain.GoodsFieldPackagingLevelFactor, 2), "packagingLevels[2].factor"},
		{domain.NewGoodsCell(domain.GoodsFieldAliasValidity, 0), "aliases[0].validity"},
		{domain.NewGoodsCell(domain.GoodsFieldComponentQuantity, 1), "composition.components[1].quantity"},
		{domain.NewGoodsCell(domain.GoodsFieldMeasurementDimensionsUnit, 3), "declaredMeasurements[3].dimensions.unit"},
		{domain.NewGoodsCell(domain.GoodsFieldDeclarationCurrency, 0), "declaration.currency"},
	} {
		if got := test.cell.String(); got != test.want {
			t.Errorf("%v 的键路径 = %q，want %q", test.cell.Field(), got, test.want)
		}
	}
	if _, indexed := domain.NewGoodsCell(domain.GoodsFieldName, 4).Index(); indexed {
		t.Errorf("name 不在列表里，不该有位置")
	}
	if index, indexed := domain.NewGoodsCell(domain.GoodsFieldAliasCode, 4).Index(); !indexed || index != 4 {
		t.Errorf("别名的码该带位置 4，实得 (%d, %v)", index, indexed)
	}
	if domain.GoodsFieldInvalid.String() != "" {
		t.Errorf("零值没有键路径")
	}
}

// 提交：货主口要来源与键、不带依据引用；代录口要决定标识与依据引用、没有来源；拼凑出来的零值提交不是提交。同一个判重位置只比途径、来源与键。
func TestGoodsSubmissionIsSelfConsistentAndSlotIgnoresEvidence(t *testing.T) {
	source := valueOf(t, domain.NewSubmissionSource, "SYN-CHANNEL-01")
	key := valueOf(t, domain.NewRequestKey, "REQ-1")
	evidence := valueOf(t, domain.NewEvidenceReference, "AUTH-1")
	if _, err := domain.NewOwnerSubmission(domain.SubmissionSource{}, key); !errors.Is(err, domain.ErrInvalidGoodsSubmission) {
		t.Errorf("货主口缺来源：err = %v", err)
	}
	if _, err := domain.NewOwnerSubmission(source, domain.RequestKey{}); !errors.Is(err, domain.ErrInvalidGoodsSubmission) {
		t.Errorf("货主口缺键：err = %v", err)
	}
	if _, err := domain.NewOnBehalfSubmission(key, domain.EvidenceReference{}); !errors.Is(err, domain.ErrInvalidGoodsSubmission) {
		t.Errorf("代录口缺依据引用：err = %v", err)
	}
	if _, err := domain.NewOnBehalfSubmission(domain.RequestKey{}, evidence); !errors.Is(err, domain.ErrInvalidGoodsSubmission) {
		t.Errorf("代录口缺决定标识：err = %v", err)
	}

	owner := ownerSubmission(t, "REQ-1")
	if _, hasEvidence := owner.EvidenceReference(); hasEvidence || owner.Path() != domain.SubmissionByOwner {
		t.Errorf("货主口不带依据引用")
	}
	if got, hasSource := owner.Source(); !hasSource || got.String() != "SYN-CHANNEL-01" {
		t.Errorf("货主口的来源 = %q（%v）", got, hasSource)
	}
	onBehalf := onBehalfSubmission(t, "REQ-1", "AUTH-1")
	if _, hasSource := onBehalf.Source(); hasSource || onBehalf.Path() != domain.SubmissionOnBehalf {
		t.Errorf("代录口没有来源")
	}
	if got, hasEvidence := onBehalf.EvidenceReference(); !hasEvidence || got.String() != "AUTH-1" {
		t.Errorf("代录的依据引用 = %q（%v）", got, hasEvidence)
	}
	if owner.SameSlotAs(onBehalf) || onBehalf.SameSlotAs(owner) {
		t.Errorf("同一个键、不同途径是两个判重位置")
	}
	if !onBehalf.SameSlotAs(onBehalfSubmission(t, "REQ-1", "AUTH-OTHER")) {
		t.Errorf("依据引用不属于判重位置")
	}
	if onBehalf.SameSlotAs(onBehalfSubmission(t, "REQ-2", "AUTH-1")) || owner.SameSlotAs(ownerSubmission(t, "REQ-2")) {
		t.Errorf("键不同是不同的位置")
	}
	otherSource, err := domain.NewOwnerSubmission(valueOf(t, domain.NewSubmissionSource, "SYN-CHANNEL-02"), key)
	if err != nil || owner.SameSlotAs(otherSource) {
		t.Errorf("来源不同是不同的位置")
	}
}

// Covers: AT-WF-014、AT-WF-015（商品的核心）——一笔修订：修订号从 1 起，修订 1 没有基准、其后每笔基于前一笔；登记时刻收成 UTC 微秒；缺身份、提交、登记责任方、
// 登记时刻或资料缺必填格都构造不出来；判重要比的请求不比登记责任方与登记时刻。
func TestGoodsRevisionDerivesItsBasisAndComparesRequests(t *testing.T) {
	content := goodsContentOf(t, nil)
	first := goodsRevisionOf(t, "SYN-GOODS-01", 1, content, ownerSubmission(t, "REQ-1"))
	if first.Basis().Given() {
		t.Errorf("修订 1 没有基准")
	}
	second := goodsRevisionOf(t, "SYN-GOODS-01", 2, content, ownerSubmission(t, "REQ-2"))
	if basis := second.Basis(); !basis.Given() || basis.Number() != 1 {
		t.Errorf("修订 2 的基准 = (%d, %v)，want 1", basis.Number(), basis.Given())
	}
	if first.Request().SameAs(second.Request()) {
		t.Errorf("基准不同的请求不是同一份")
	}

	replayed, err := domain.NewGoodsRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), 1, content, ownerSubmission(t, "REQ-1"),
		valueOf(t, domain.NewRegistrant, "operator-9"), registeredAt.Add(72*time.Hour))
	if err != nil || !first.Request().SameAs(replayed.Request()) {
		t.Errorf("换登记责任方与登记时刻的同一请求该是同一份：%v", err)
	}
	other := goodsRevisionOf(t, "SYN-GOODS-01", 1, goodsContentOf(t, func(spec *domain.GoodsContentSpec) {
		spec.Name = valueOf(t, domain.NewGoodsName, "改了名称")
	}), ownerSubmission(t, "REQ-1"))
	if first.Request().SameAs(other.Request()) {
		t.Errorf("资料不同的请求不是同一份")
	}
	otherCode := goodsRevisionOf(t, "SYN-GOODS-02", 1, content, ownerSubmission(t, "REQ-1"))
	if first.Request().SameAs(otherCode.Request()) {
		t.Errorf("商品编码不同的请求不是同一份")
	}
	withEvidence := goodsRevisionOf(t, "SYN-GOODS-01", 1, content, onBehalfSubmission(t, "REQ-1", "AUTH-1"))
	otherEvidence := goodsRevisionOf(t, "SYN-GOODS-01", 1, content, onBehalfSubmission(t, "REQ-1", "AUTH-2"))
	if withEvidence.Request().SameAs(otherEvidence.Request()) || withEvidence.Request().SameAs(first.Request()) {
		t.Errorf("依据引用不同的请求不是同一份")
	}
	otherOwner, err := domain.NewGoodsRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-02"),
		valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), 1, content, ownerSubmission(t, "REQ-1"),
		valueOf(t, domain.NewRegistrant, "operator-1"), registeredAt)
	if err != nil || first.Request().SameAs(otherOwner.Request()) {
		t.Errorf("货主不同的请求不是同一份：%v", err)
	}

	stamped, err := domain.NewGoodsRevision(
		valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), 1, content, ownerSubmission(t, "REQ-1"),
		valueOf(t, domain.NewRegistrant, "operator-1"), time.Date(2026, 11, 18, 17, 30, 0, 123456789, time.FixedZone("UTC+8", 8*3600)))
	if err != nil || !stamped.RegisteredAt().Equal(time.Date(2026, 11, 18, 9, 30, 0, 123456000, time.UTC)) || stamped.RegisteredAt().Location() != time.UTC {
		t.Errorf("登记时刻 = %v（%v），want UTC 微秒", stamped.RegisteredAt(), err)
	}
}

func TestGoodsRevisionRefusesIncompleteParts(t *testing.T) {
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	owner := valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01")
	code := valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01")
	registrant := valueOf(t, domain.NewRegistrant, "operator-1")
	content := goodsContentOf(t, nil)
	submission := ownerSubmission(t, "REQ-1")

	for _, number := range []int{0, -1} {
		if _, err := domain.NewGoodsRevision(tenant, owner, code, number, content, submission, registrant, registeredAt); !errors.Is(err, domain.ErrInvalidRevisionNumber) {
			t.Errorf("修订号 %d：err = %v，want ErrInvalidRevisionNumber", number, err)
		}
	}
	for name, build := range map[string]func() error{
		"缺租户": func() error {
			_, err := domain.NewGoodsRevision(domain.TenantID{}, owner, code, 1, content, submission, registrant, registeredAt)
			return err
		},
		"缺货主": func() error {
			_, err := domain.NewGoodsRevision(tenant, domain.OwnerAccountID{}, code, 1, content, submission, registrant, registeredAt)
			return err
		},
		"缺商品编码": func() error {
			_, err := domain.NewGoodsRevision(tenant, owner, domain.GoodsCode{}, 1, content, submission, registrant, registeredAt)
			return err
		},
		"缺提交": func() error {
			_, err := domain.NewGoodsRevision(tenant, owner, code, 1, content, domain.GoodsSubmission{}, registrant, registeredAt)
			return err
		},
		"缺登记责任方": func() error {
			_, err := domain.NewGoodsRevision(tenant, owner, code, 1, content, submission, domain.Registrant{}, registeredAt)
			return err
		},
		"缺登记时刻": func() error {
			_, err := domain.NewGoodsRevision(tenant, owner, code, 1, content, submission, registrant, time.Time{})
			return err
		},
	} {
		if err := build(); !errors.Is(err, domain.ErrInvalidGoodsRevision) {
			t.Errorf("%s：err = %v，want ErrInvalidGoodsRevision", name, err)
		}
	}
	var fieldError *domain.GoodsContentFieldError
	if _, err := domain.NewGoodsRevision(tenant, owner, code, 1, domain.GoodsContent{}, submission, registrant, registeredAt); !errors.As(err, &fieldError) || fieldError.Cell.String() != "name" {
		t.Errorf("零值资料：err = %v，want 指出 name", err)
	}
}
