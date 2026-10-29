package refdata

import (
	"errors"
	"math/big"
	"regexp"
	"strings"
	"testing"
)

var (
	countryCodeShape  = regexp.MustCompile(`^[A-Z]{2}$`)
	currencyCodeShape = regexp.MustCompile(`^[A-Z]{3}$`)
	unitCodeShape     = regexp.MustCompile(`^[A-Z][A-Z0-9]*$`)
)

// Covers: ADR-0335 决定三——国家 / 地区目录是 ISO 3166-1 alpha-2 的全量，码唯一、两位大写、名称与大洲俱全。
func TestCountryCatalogueIsCompleteAndWellFormed(t *testing.T) {
	t.Parallel()

	seen := map[string]bool{}
	for _, row := range countries {
		if !countryCodeShape.MatchString(row.Code) {
			t.Errorf("国家码 %q 不是两位大写字母", row.Code)
		}
		if seen[row.Code] {
			t.Errorf("国家码 %s 重复", row.Code)
		}
		seen[row.Code] = true
		if strings.TrimSpace(row.NameZh) == "" || strings.TrimSpace(row.NameEn) == "" {
			t.Errorf("国家 %s 缺中文名或英文名", row.Code)
		}
		if !row.Region.Valid() {
			t.Errorf("国家 %s 的大洲 %q 不在封闭七词里", row.Code, row.Region)
		}
	}
	// ISO 3166-1 官方已分配 249 个 alpha-2 码；少了就是漏登，多了就是混进了用户指派码。
	if len(countries) != 249 {
		t.Errorf("国家目录 %d 项，ISO 3166-1 alpha-2 官方已分配 249 个", len(countries))
	}
	for _, code := range []string{"CN", "US", "GB", "JP", "SG", "HK", "MO", "TW", "DE", "FR", "AU", "BR", "AE", "ZA"} {
		if _, found := Country(code); !found {
			t.Errorf("跨境小包的常见国家 / 地区 %s 不在目录里", code)
		}
	}
	if _, found := Country("XK"); found {
		t.Error("XK 是用户指派码，不是 ISO 3166-1 已分配码，不该在目录里")
	}
	if _, found := Country("cn"); found {
		t.Error("目录只认大写写法，小写不算命中")
	}
}

// Covers: ADR-0335 决定三——币种目录覆盖跨境小包会遇到的常用集；码唯一、三位大写、小数位封闭在 0..3。
func TestCurrencyCatalogueCoversTheCommonSetAndIsWellFormed(t *testing.T) {
	t.Parallel()

	seen := map[string]bool{}
	for _, row := range currencies {
		if !currencyCodeShape.MatchString(row.Code) {
			t.Errorf("币种码 %q 不是三位大写字母", row.Code)
		}
		if seen[row.Code] {
			t.Errorf("币种码 %s 重复", row.Code)
		}
		seen[row.Code] = true
		if strings.TrimSpace(row.NameZh) == "" || strings.TrimSpace(row.NameEn) == "" || strings.TrimSpace(row.Symbol) == "" {
			t.Errorf("币种 %s 缺中文名、英文名或符号", row.Code)
		}
		if row.MinorUnitScale > 3 {
			t.Errorf("币种 %s 的小数位 %d 超出 0..3", row.Code, row.MinorUnitScale)
		}
	}
	if len(currencies) < 120 {
		t.Errorf("币种目录只有 %d 项，常用集至少 120 项", len(currencies))
	}
	for _, code := range []string{
		"CNY", "USD", "EUR", "GBP", "JPY", "KRW", "SGD", "HKD", "AUD", "CAD", "THB", "MYR", "VND", "IDR", "PHP", "INR",
		"AED", "SAR", "BRL", "MXN", "TWD", "CHF", "RUB", "TRY", "ZAR", "NZD", "SEK", "NOK", "DKK", "PLN", "MOP",
	} {
		if _, found := Currency(code); !found {
			t.Errorf("跨境小包的常见币种 %s 不在目录里", code)
		}
	}
}

// Covers: ADR-0335 决定五——Currency.MinorUnitScale 改查目录而行为不变。parcelpricing 改查之前内置的是两张封闭小表
// （0 位十六个、3 位七个，其余按 2 位答），目录里每个币种的小数位必须与之一致；这里钉死那两张表，
// 改目录时若想动其中任何一个码，先来这里改并说明。
func TestCurrencyMinorUnitScaleMatchesTheTableItReplaced(t *testing.T) {
	t.Parallel()

	zeroDigit := []string{"JPY", "KRW", "VND", "CLP", "ISK", "XOF", "XAF", "XPF", "KMF", "BIF", "DJF", "GNF", "RWF", "UGX", "PYG", "VUV"}
	threeDigit := []string{"KWD", "BHD", "OMR", "JOD", "TND", "IQD", "LYD"}
	expected := map[string]uint8{}
	for _, code := range zeroDigit {
		expected[code] = 0
	}
	for _, code := range threeDigit {
		expected[code] = 3
	}

	for _, row := range currencies {
		want, special := expected[row.Code]
		if !special {
			want = 2
		}
		if row.MinorUnitScale != want {
			t.Errorf("币种 %s 的小数位 = %d，改查目录之前的表给 %d", row.Code, row.MinorUnitScale, want)
		}
	}
	for code := range expected {
		if _, found := Currency(code); !found {
			t.Errorf("旧表里的异常小数位币种 %s 不在目录里，查目录会把它退回 2 位", code)
		}
	}
}

// Covers: ADR-0335 决定三——计量单位目录：码唯一，基准单位存在、同类别且自己是自己的基准（系数 1），系数是可解析的正数。
func TestUnitCatalogueIsConsistent(t *testing.T) {
	t.Parallel()

	seen := map[string]bool{}
	baseOfCategory := map[UnitCategory]string{}
	for _, row := range units {
		if !unitCodeShape.MatchString(row.Code) {
			t.Errorf("单位码 %q 不是大写字母数字", row.Code)
		}
		if seen[row.Code] {
			t.Errorf("单位码 %s 重复", row.Code)
		}
		seen[row.Code] = true
		if strings.TrimSpace(row.NameZh) == "" {
			t.Errorf("单位 %s 缺中文名", row.Code)
		}
		if !row.Category.Valid() {
			t.Errorf("单位 %s 的类别 %q 不在封闭四词里", row.Code, row.Category)
		}
		base, found := Unit(row.BaseUnit)
		if !found {
			t.Errorf("单位 %s 的基准单位 %s 不在目录里", row.Code, row.BaseUnit)
			continue
		}
		if base.Category != row.Category {
			t.Errorf("单位 %s 的基准单位 %s 属 %s，与自己的 %s 不同类", row.Code, base.Code, base.Category, row.Category)
		}
		if base.BaseUnit != base.Code || base.FactorToBase != "1" {
			t.Errorf("基准单位 %s 必须以自己为基准、系数为 1", base.Code)
		}
		if previous, again := baseOfCategory[row.Category]; again && previous != row.BaseUnit {
			t.Errorf("类别 %s 有两个基准单位 %s 与 %s", row.Category, previous, row.BaseUnit)
		}
		baseOfCategory[row.Category] = row.BaseUnit

		factor, ok := new(big.Rat).SetString(row.FactorToBase)
		if !ok || factor.Sign() <= 0 {
			t.Errorf("单位 %s 的系数 %q 不是正的十进制数", row.Code, row.FactorToBase)
		}
		if strings.ContainsAny(row.FactorToBase, "eE") || (strings.Contains(row.FactorToBase, ".") && strings.HasSuffix(row.FactorToBase, "0")) {
			t.Errorf("单位 %s 的系数 %q 不是非指数、无尾零的规范写法", row.Code, row.FactorToBase)
		}
	}
	for _, category := range UnitCategories() {
		if baseOfCategory[category] == "" {
			t.Errorf("类别 %s 下没有任何单位", category)
		}
	}
	// 现有计价代码里出现过的单位写法，目录都得认。
	for _, code := range []string{"KG", "G", "LB", "OZ", "CM", "IN", "M3", "CBM", "PCS", "PIECE"} {
		if _, found := Unit(code); !found {
			t.Errorf("计价里在用的单位 %s 不在目录里", code)
		}
	}
}

func rat(t *testing.T, text string) *big.Rat {
	t.Helper()
	value, ok := new(big.Rat).SetString(text)
	if !ok {
		t.Fatalf("不是数：%q", text)
	}
	return value
}

// Covers: ADR-0335 决定四——Convert 精确：1 磅 = 0.45359237 千克，1 千克 = 1000 克，1 磅 = 16 盎司，都不经取整。
func TestConvertIsExact(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value, from, to, want string
	}{
		{"1", "LB", "KG", "0.45359237"},
		{"2", "LB", "KG", "0.90718474"},
		{"1", "KG", "G", "1000"},
		{"2.5", "KG", "G", "2500"},
		{"1", "LB", "OZ", "16"},
		{"1", "OZ", "G", "28.349523125"},
		{"1", "T", "KG", "1000"},
		{"1", "IN", "CM", "2.54"},
		{"1", "M", "MM", "1000"},
		{"1", "FT", "IN", "12"},
		{"1", "CBM", "M3", "1"},
		{"1", "M3", "L", "1000"},
		{"1", "IN3", "CM3", "16.387064"},
		{"2", "DZ", "PCS", "24"},
		{"3", "PIECE", "PCS", "3"},
		{"7", "KG", "KG", "7"},
		{"0", "LB", "KG", "0"},
	}
	for _, c := range cases {
		got, err := Convert(rat(t, c.value), c.from, c.to)
		if err != nil {
			t.Errorf("Convert(%s %s→%s) 报错：%v", c.value, c.from, c.to, err)
			continue
		}
		if got.Cmp(rat(t, c.want)) != 0 {
			t.Errorf("Convert(%s %s→%s) = %s，应为 %s", c.value, c.from, c.to, got.RatString(), c.want)
		}
	}
}

// Covers: ADR-0335 决定四——保留精度、不取整：往返换算回到原值，且输入不被修改；1 千克折多少磅是无限小数，照样精确。
func TestConvertKeepsPrecisionOnTheRoundTrip(t *testing.T) {
	t.Parallel()

	for _, pair := range [][2]string{{"LB", "KG"}, {"OZ", "G"}, {"IN", "CM"}, {"FT3", "L"}, {"DZ", "PCS"}, {"KG", "LB"}} {
		original := rat(t, "3.7")
		snapshot := new(big.Rat).Set(original)
		there, err := Convert(original, pair[0], pair[1])
		if err != nil {
			t.Fatal(err)
		}
		back, err := Convert(there, pair[1], pair[0])
		if err != nil {
			t.Fatal(err)
		}
		if back.Cmp(snapshot) != 0 {
			t.Errorf("%s→%s→%s 往返 = %s，应回到 3.7", pair[0], pair[1], pair[0], back.RatString())
		}
		if original.Cmp(snapshot) != 0 {
			t.Errorf("Convert 改了输入：%s", original.RatString())
		}
	}

	pounds, err := Convert(rat(t, "1"), "KG", "LB")
	if err != nil {
		t.Fatal(err)
	}
	if pounds.IsInt() {
		t.Error("1 千克折磅不是整数；有理数结果被取整了")
	}
	// 1 ÷ 0.45359237 = 100000000/45359237，分母不整除 10 的任何幂，所以不是有限小数；它在有理数里仍是精确值，说明没有任何一步取整。
	if pounds.Cmp(rat(t, "100000000/45359237")) != 0 {
		t.Errorf("1 千克 = %s 磅，应为 100000000/45359237", pounds.RatString())
	}
}

// Covers: ADR-0335 决定四——跨类别没有换算；单位码不在目录里是另一种错。
func TestConvertRefusesCrossCategoryAndUnknownUnits(t *testing.T) {
	t.Parallel()

	if _, err := Convert(rat(t, "1"), "KG", "CM"); !errors.Is(err, ErrUnitCategoryMismatch) {
		t.Errorf("千克换厘米应报 ErrUnitCategoryMismatch，得到 %v", err)
	}
	if _, err := Convert(rat(t, "1"), "PCS", "KG"); !errors.Is(err, ErrUnitCategoryMismatch) {
		t.Errorf("件换千克应报 ErrUnitCategoryMismatch，得到 %v", err)
	}
	if _, err := Convert(rat(t, "1"), "M3", "M"); !errors.Is(err, ErrUnitCategoryMismatch) {
		t.Errorf("体积换长度应报 ErrUnitCategoryMismatch，得到 %v", err)
	}
	if _, err := Convert(rat(t, "1"), "STONE", "KG"); !errors.Is(err, ErrUnknownUnit) {
		t.Errorf("未知源单位应报 ErrUnknownUnit，得到 %v", err)
	}
	if _, err := Convert(rat(t, "1"), "KG", "kg"); !errors.Is(err, ErrUnknownUnit) {
		t.Errorf("小写单位码不算命中，应报 ErrUnknownUnit，得到 %v", err)
	}
}

// Covers: ADR-0335 决定六——检索按码或名称（不分大小写、子串），空检索答全表，大洲 / 类别过滤；列表交回副本，改它不影响目录。
func TestSearchAndListing(t *testing.T) {
	t.Parallel()

	if got := SearchCountries("", ""); len(got) != len(countries) {
		t.Errorf("空检索答 %d 项，应为全表 %d 项", len(got), len(countries))
	}
	if got := SearchCountries("  ", ""); len(got) != len(countries) {
		t.Errorf("纯空白检索应同空检索，得到 %d 项", len(got))
	}
	if got := SearchCountries("cn", ""); !hasCountry(got, "CN") {
		t.Errorf("按码小写检索 cn 应命中 CN：%v", got)
	}
	if got := SearchCountries("中国", ""); !hasCountry(got, "CN") || !hasCountry(got, "HK") || !hasCountry(got, "TW") {
		t.Errorf("按中文名检索“中国”应命中 CN、HK、TW：%v", got)
	}
	if got := SearchCountries("united", ""); !hasCountry(got, "US") || !hasCountry(got, "GB") || !hasCountry(got, "AE") {
		t.Errorf("按英文名检索 united 应命中 US、GB、AE：%v", got)
	}
	europe := SearchCountries("", RegionEurope)
	if len(europe) == 0 || !hasCountry(europe, "DE") || hasCountry(europe, "CN") {
		t.Errorf("按大洲 EUROPE 过滤应含 DE、不含 CN：%d 项", len(europe))
	}
	if got := SearchCountries("zzzz", ""); len(got) != 0 {
		t.Errorf("无匹配应答空，得到 %v", got)
	}

	if got := SearchCurrencies("人民币"); len(got) != 1 || got[0].Code != "CNY" {
		t.Errorf("按中文名检索人民币应只命中 CNY：%v", got)
	}
	if got := SearchCurrencies("dollar"); !hasCurrency(got, "USD") || !hasCurrency(got, "HKD") {
		t.Errorf("按英文名检索 dollar 应命中 USD、HKD：%v", got)
	}
	if got := SearchCurrencies(""); len(got) != len(currencies) {
		t.Errorf("币种空检索应答全表，得到 %d 项", len(got))
	}

	weights := SearchUnits("", UnitCategoryWeight)
	if len(weights) == 0 || weights[0].Code != "KG" {
		t.Errorf("重量类应以基准单位 KG 打头：%v", weights)
	}
	for _, row := range weights {
		if row.Category != UnitCategoryWeight {
			t.Errorf("类别过滤混进了 %s", row.Code)
		}
	}
	if got := SearchUnits("磅", ""); len(got) != 1 || got[0].Code != "LB" {
		t.Errorf("按中文名检索磅应只命中 LB：%v", got)
	}
	if got := SearchUnits("", ""); len(got) != len(units) {
		t.Errorf("单位空检索应答全表，得到 %d 项", len(got))
	}

	listed := Countries()
	listed[0].NameZh = "被改了"
	if first, _ := Country(listed[0].Code); first.NameZh == "被改了" {
		t.Error("Countries 交回的不是副本，改它改到了目录")
	}
	for index := 1; index < len(listed); index++ {
		if listed[index-1].Code >= listed[index].Code {
			t.Errorf("国家列表没有按码升序：%s 在 %s 前", listed[index-1].Code, listed[index].Code)
		}
	}
}

func hasCountry(rows []CountryInfo, code string) bool {
	for _, row := range rows {
		if row.Code == code {
			return true
		}
	}
	return false
}

func hasCurrency(rows []CurrencyInfo, code string) bool {
	for _, row := range rows {
		if row.Code == code {
			return true
		}
	}
	return false
}
