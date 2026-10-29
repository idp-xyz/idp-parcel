package refdata

import (
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
)

// UnitCategory 是计量单位的类别，封闭四词。同一类别的单位可以互相换算，跨类别没有换算。
type UnitCategory string

const (
	UnitCategoryWeight UnitCategory = "WEIGHT"
	UnitCategoryLength UnitCategory = "LENGTH"
	UnitCategoryVolume UnitCategory = "VOLUME"
	UnitCategoryCount  UnitCategory = "COUNT"
)

// UnitCategories 按固定顺序列出封闭四词。
func UnitCategories() []UnitCategory {
	return []UnitCategory{UnitCategoryWeight, UnitCategoryLength, UnitCategoryVolume, UnitCategoryCount}
}

// Valid 判断是不是封闭四词之一。
func (category UnitCategory) Valid() bool {
	for _, known := range UnitCategories() {
		if category == known {
			return true
		}
	}
	return false
}

// UnitInfo 是计量单位目录的一行。FactorToBase 是“1 个本单位等于多少个基准单位”的精确十进制文本
// （非指数写法、无尾零，基准单位自己是 "1"），文本而非浮点：0.45359237 这类定义值不能经二进制浮点走一圈。
type UnitInfo struct {
	Code         string
	NameZh       string
	Category     UnitCategory
	BaseUnit     string
	FactorToBase string
}

// Factor 把 FactorToBase 解成精确有理数；每次交回新值，调用方可以随便改。
func (unit UnitInfo) Factor() *big.Rat {
	factor, ok := new(big.Rat).SetString(unit.FactorToBase)
	if !ok {
		// 目录是随码发布的常量，坏文本是编程错误，由目录完整性用例在构建期拦下，不留到运行时当业务错误答。
		panic(fmt.Sprintf("refdata: unit %s has a malformed factor %q", unit.Code, unit.FactorToBase))
	}
	return factor
}

// 各类别的基准单位：重量 KG、长度 CM、体积 M3、件数 PCS。基准是目录内部的换算枢纽，
// 不是哪个租户的计价口径——各类别里换算系数都是对它的定义值。
//
// 体积里的 CM3、IN3、FT3 是按长度单位立方得来的精确值（2.54³ cm³ = 16.387064 cm³，0.3048³ m³ = 0.028316846592 m³），
// 体积重换算里最常遇到。CBM 是 M3 的另一种写法，系数同为 1；PIECE 之于 PCS 同理。两对别名都留着，
// 因为 parcelpricing 里两种写法都在用，目录不替数据挑一个。
var units = []UnitInfo{
	{"KG", "千克", UnitCategoryWeight, "KG", "1"},
	{"G", "克", UnitCategoryWeight, "KG", "0.001"},
	{"MG", "毫克", UnitCategoryWeight, "KG", "0.000001"},
	{"T", "吨", UnitCategoryWeight, "KG", "1000"},
	{"LB", "磅", UnitCategoryWeight, "KG", "0.45359237"},
	{"OZ", "盎司", UnitCategoryWeight, "KG", "0.028349523125"},

	{"CM", "厘米", UnitCategoryLength, "CM", "1"},
	{"MM", "毫米", UnitCategoryLength, "CM", "0.1"},
	{"M", "米", UnitCategoryLength, "CM", "100"},
	{"IN", "英寸", UnitCategoryLength, "CM", "2.54"},
	{"FT", "英尺", UnitCategoryLength, "CM", "30.48"},
	{"YD", "码", UnitCategoryLength, "CM", "91.44"},

	{"M3", "立方米", UnitCategoryVolume, "M3", "1"},
	{"CBM", "立方米（CBM 写法）", UnitCategoryVolume, "M3", "1"},
	{"L", "升", UnitCategoryVolume, "M3", "0.001"},
	{"ML", "毫升", UnitCategoryVolume, "M3", "0.000001"},
	{"CM3", "立方厘米", UnitCategoryVolume, "M3", "0.000001"},
	{"IN3", "立方英寸", UnitCategoryVolume, "M3", "0.000016387064"},
	{"FT3", "立方英尺", UnitCategoryVolume, "M3", "0.028316846592"},

	{"PCS", "件", UnitCategoryCount, "PCS", "1"},
	{"PIECE", "件（PIECE 写法）", UnitCategoryCount, "PCS", "1"},
	{"PR", "双（对）", UnitCategoryCount, "PCS", "2"},
	{"DZ", "打", UnitCategoryCount, "PCS", "12"},
}

var unitIndex = indexOf(units, func(row UnitInfo) string { return row.Code })

// Unit 按码精确查一个计量单位，命中规则同 Country。
func Unit(code string) (UnitInfo, bool) {
	row, found := unitIndex[code]
	return row, found
}

// Units 按“类别在 UnitCategories 里的次序，类别内按基准单位在先、其余按码升序”交回全表的副本。
func Units() []UnitInfo {
	rank := make(map[UnitCategory]int, len(UnitCategories()))
	for position, category := range UnitCategories() {
		rank[category] = position
	}
	copied := append([]UnitInfo(nil), units...)
	sort.SliceStable(copied, func(left, right int) bool {
		a, b := copied[left], copied[right]
		if rank[a.Category] != rank[b.Category] {
			return rank[a.Category] < rank[b.Category]
		}
		aBase, bBase := a.Code == a.BaseUnit, b.Code == b.BaseUnit
		if aBase != bBase {
			return aBase
		}
		return a.Code < b.Code
	})
	return copied
}

// SearchUnits 按码或中文名称检索（不分大小写、子串匹配），category 非空时再按类别过滤。规则同 SearchCountries。
func SearchUnits(query string, category UnitCategory) []UnitInfo {
	needle := normalizedQuery(query)
	matched := make([]UnitInfo, 0, len(units))
	for _, row := range Units() {
		if category != "" && row.Category != category {
			continue
		}
		if matchesAny(needle, row.Code, row.NameZh) {
			matched = append(matched, row)
		}
	}
	return matched
}

var (
	// ErrUnknownUnit：单位码不在目录里。
	ErrUnknownUnit = errors.New("refdata: unknown unit")
	// ErrUnitCategoryMismatch：两个单位不同类别，没有换算（千克换不成厘米）。
	ErrUnitCategoryMismatch = errors.New("refdata: units belong to different categories")
)

// Convert 把 value 个 from 换成以 to 计的量，结果是精确有理数：不取整、不截断、不经浮点。
// 商可能是无限小数（1 千克折多少磅），所以交回的是有理数而不是定标十进制——要落成某个小数位的十进制文本，
// 由调用方在它自己的业务位置上取整一次（那是它的业务决定，目录不替它做）。输入的 value 不被修改。
//
// 目前没有任何生产调用方：计价里重量单位不一致仍报 ErrWeightUnitMismatch，不自动换算（ADR-0335 决定四）——
// 换算会改金额，接不接、在哪一步接、哪些口径允许，是单独的裁决。
func Convert(value *big.Rat, from, to string) (*big.Rat, error) {
	source, found := Unit(from)
	if !found {
		return nil, fmt.Errorf("%w: %q", ErrUnknownUnit, from)
	}
	target, found := Unit(to)
	if !found {
		return nil, fmt.Errorf("%w: %q", ErrUnknownUnit, to)
	}
	if source.Category != target.Category {
		return nil, fmt.Errorf("%w: %s is %s, %s is %s", ErrUnitCategoryMismatch,
			source.Code, strings.ToLower(string(source.Category)), target.Code, strings.ToLower(string(target.Category)))
	}
	inBase := new(big.Rat).Mul(value, source.Factor())
	return inBase.Quo(inBase, target.Factor()), nil
}
