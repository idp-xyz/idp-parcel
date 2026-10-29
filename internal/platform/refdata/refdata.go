// Package refdata 是产品内置的基础数据目录：国家 / 地区、币种、计量单位（ADR-0335）。
//
// 三份目录都是公开标准（ISO 3166-1 alpha-2、ISO 4217）与物理常数（单位换算系数），属 ADR-0146 所说的“产品内置数据”，
// 不是租户取值：只读、随代码发布、不入库，租户改不了它，也就不会因为某个租户改了一个小数位或一个换算系数，
// 让同一份计价在别处算出不同的钱。目录的版本就是代码的版本——ISO 修订靠发版跟进。
//
// 本包只放纯 Go 数据与查询，不依赖 HTTP、数据库或任何限界上下文，所以各上下文的领域包可以直接引用
// （Currency.MinorUnitScale 就查这里）。它不替任何上下文做判断：某个码在目录里，只说明它是标准里的一个码；
// 某租户启用哪些国家 / 币种、是否允许某个码出现在某份数据里，是后续轮次的租户取值，不在此处。
package refdata

import (
	"sort"
	"strings"
)

// Region 是国家 / 地区的大洲分组，封闭七词。归属照联合国 M49 地理分区的大洲层（美洲在巴拿马与哥伦比亚之间分北南，
// 中美洲与加勒比归北美洲；塞浦路斯、土耳其、外高加索三国归亚洲，俄罗斯归欧洲）。它只供检索分组，
// 不是任何租户的区域维：服务区域、覆盖范围属 network-routing 的登记册。
type Region string

const (
	RegionAfrica       Region = "AFRICA"
	RegionAntarctica   Region = "ANTARCTICA"
	RegionAsia         Region = "ASIA"
	RegionEurope       Region = "EUROPE"
	RegionNorthAmerica Region = "NORTH_AMERICA"
	RegionOceania      Region = "OCEANIA"
	RegionSouthAmerica Region = "SOUTH_AMERICA"
)

// Regions 按固定顺序列出封闭七词。
func Regions() []Region {
	return []Region{
		RegionAsia, RegionEurope, RegionAfrica, RegionNorthAmerica, RegionSouthAmerica, RegionOceania, RegionAntarctica,
	}
}

// Valid 判断是不是封闭七词之一。
func (region Region) Valid() bool {
	for _, known := range Regions() {
		if region == known {
			return true
		}
	}
	return false
}

// CountryInfo 是 ISO 3166-1 alpha-2 目录的一行。Code 是两位大写字母。
type CountryInfo struct {
	Code   string
	NameZh string
	NameEn string
	Region Region
}

// CurrencyInfo 是 ISO 4217 常用集的一行。Code 是三位大写字母。
// MinorUnitScale 是最小币单位的小数位，封闭在 0..3。Symbol 是常见的显示符号，没有通行符号的币种取通行缩写。
type CurrencyInfo struct {
	Code           string
	NameZh         string
	NameEn         string
	Symbol         string
	MinorUnitScale uint8
}

var (
	countryIndex  = indexOf(countries, func(row CountryInfo) string { return row.Code })
	currencyIndex = indexOf(currencies, func(row CurrencyInfo) string { return row.Code })
)

func indexOf[Row any](rows []Row, code func(Row) string) map[string]Row {
	index := make(map[string]Row, len(rows))
	for _, row := range rows {
		index[code(row)] = row
	}
	return index
}

// Country 按码精确查一个国家 / 地区。码必须是目录里的写法（两位大写）；小写或带空白的不算命中，
// 规范化是调用方在边界上的事，目录不替它宽容。
func Country(code string) (CountryInfo, bool) {
	row, found := countryIndex[code]
	return row, found
}

// Currency 按码精确查一个币种，命中规则同 Country。
func Currency(code string) (CurrencyInfo, bool) {
	row, found := currencyIndex[code]
	return row, found
}

// Countries 按码升序交回全表的副本，调用方改它不影响目录。
func Countries() []CountryInfo {
	return sortedCopy(countries, func(row CountryInfo) string { return row.Code })
}

// Currencies 按码升序交回全表的副本。
func Currencies() []CurrencyInfo {
	return sortedCopy(currencies, func(row CurrencyInfo) string { return row.Code })
}

func sortedCopy[Row any](rows []Row, code func(Row) string) []Row {
	copied := append([]Row(nil), rows...)
	sort.Slice(copied, func(left, right int) bool { return code(copied[left]) < code(copied[right]) })
	return copied
}

// SearchCountries 按码或中英文名称检索（不分大小写、子串匹配），region 非空时再按大洲过滤。
// 检索词与 region 都空即全表。region 写错不是“没有匹配”而是调用方的问题，由 Region.Valid 在入口先拦。
func SearchCountries(query string, region Region) []CountryInfo {
	needle := normalizedQuery(query)
	matched := make([]CountryInfo, 0, len(countries))
	for _, row := range Countries() {
		if region != "" && row.Region != region {
			continue
		}
		if matchesAny(needle, row.Code, row.NameZh, row.NameEn) {
			matched = append(matched, row)
		}
	}
	return matched
}

// SearchCurrencies 按码或中英文名称检索，规则同 SearchCountries。
func SearchCurrencies(query string) []CurrencyInfo {
	needle := normalizedQuery(query)
	matched := make([]CurrencyInfo, 0, len(currencies))
	for _, row := range Currencies() {
		if matchesAny(needle, row.Code, row.NameZh, row.NameEn) {
			matched = append(matched, row)
		}
	}
	return matched
}

func normalizedQuery(query string) string {
	return strings.ToLower(strings.TrimSpace(query))
}

// matchesAny 在 needle 为空时恒真（空检索答全表），否则任一字段含它即真。
func matchesAny(needle string, fields ...string) bool {
	if needle == "" {
		return true
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), needle) {
			return true
		}
	}
	return false
}
