package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrInvalidServiceAreaCoverage = errors.New("network routing: invalid service area coverage")

// GeoResolutionSide 是地理解析投影的一侧（寄件或收件）：国家 / 地区码与邮编各至多一格。值原样携带——
// 规范化走参考配置（ADR-0148 决定二），本上下文不去空白、不改大小写；在场标志让「没报」与「报了空白」分得开。
type GeoResolutionSide struct {
	country       string
	hasCountry    bool
	postalCode    string
	hasPostalCode bool
}

func NewGeoResolutionSide(country string, hasCountry bool, postalCode string, hasPostalCode bool) GeoResolutionSide {
	if !hasCountry {
		country = ""
	}
	if !hasPostalCode {
		postalCode = ""
	}
	return GeoResolutionSide{country: country, hasCountry: hasCountry, postalCode: postalCode, hasPostalCode: hasPostalCode}
}

func (side GeoResolutionSide) Country() (string, bool) {
	return side.country, side.hasCountry
}

func (side GeoResolutionSide) PostalCode() (string, bool) {
	return side.postalCode, side.hasPostalCode
}

// HasUsableCountry 答这一侧的国家 / 地区码在场且成形。覆盖匹配答资料不足时，国家码可用就说明缺的是邮编。
func (side GeoResolutionSide) HasUsableCountry() bool {
	return side.hasCountry && isCountryCodeShape(side.country)
}

// GeoResolutionProjection 是随判断请求携带的地理解析投影（ADR-0075；ADR-0148 决定二）：PS 地址要素的寄件段
// 与收件段。它是判断对象那一版的内容，本上下文只用它解析服务区域，不保存地址本体。
//
// 零值是「发起方没带投影」，与「带了、某一侧缺国家码」分得开：两者都落`资料不足`，但前者缺的是请求没把地址
// 带来，向客户要资料补不上它，缺口要单独点名。
type GeoResolutionProjection struct {
	sender   GeoResolutionSide
	delivery GeoResolutionSide
	carried  bool
}

func NewGeoResolutionProjection(sender, delivery GeoResolutionSide) GeoResolutionProjection {
	return GeoResolutionProjection{sender: sender, delivery: delivery, carried: true}
}

// Carried 答发起方是否随请求带了投影。
func (projection GeoResolutionProjection) Carried() bool {
	return projection.carried
}

func (projection GeoResolutionProjection) Sender() GeoResolutionSide {
	return projection.sender
}

func (projection GeoResolutionProjection) Delivery() GeoResolutionSide {
	return projection.delivery
}

// CoverageMatch 是一侧地址对一版服务区域覆盖的封闭答案。`资料不足`单独一格：它不是「不覆盖」，
// 把缺国家码读成不覆盖，会把一个缺资料的包裹判成不可达。
type CoverageMatch uint8

const (
	CoverageMatchInvalid CoverageMatch = iota
	CoverageCovered
	CoverageNotCovered
	CoverageInsufficient
)

func (match CoverageMatch) String() string {
	switch match {
	case CoverageCovered:
		return "COVERED"
	case CoverageNotCovered:
		return "NOT_COVERED"
	case CoverageInsufficient:
		return "INSUFFICIENT"
	default:
		return ""
	}
}

// ServiceAreaCoverage 是服务区域一版的地理覆盖，首版文法两种形态（ADR-0148 决定二）：整个国家 / 地区，或国家 /
// 地区加一组邮编前缀。覆盖的取值是租户的目录内容；形态与匹配规则归产品。前缀按字典序规整，同一组前缀
// 只有一种写法。
type ServiceAreaCoverage struct {
	country  string
	prefixes []string
}

func NewServiceAreaCoverage(country string, prefixes []string) (ServiceAreaCoverage, error) {
	if !isCountryCodeShape(country) {
		return ServiceAreaCoverage{}, fmt.Errorf("%w: country %q is not two upper-case letters", ErrInvalidServiceAreaCoverage, country)
	}
	ordered := append([]string(nil), prefixes...)
	sort.Strings(ordered)
	for index, prefix := range ordered {
		if strings.TrimSpace(prefix) == "" {
			return ServiceAreaCoverage{}, fmt.Errorf("%w: blank postal prefix", ErrInvalidServiceAreaCoverage)
		}
		if index > 0 && ordered[index-1] == prefix {
			return ServiceAreaCoverage{}, fmt.Errorf("%w: postal prefix %q appears twice", ErrInvalidServiceAreaCoverage, prefix)
		}
	}
	return ServiceAreaCoverage{country: country, prefixes: ordered}, nil
}

func (coverage ServiceAreaCoverage) Country() string {
	return coverage.country
}

// PostalPrefixes 交回一份拷贝；空即整国家 / 地区覆盖。
func (coverage ServiceAreaCoverage) PostalPrefixes() []string {
	return append([]string(nil), coverage.prefixes...)
}

// Match 判一侧地址落不落在这版覆盖里。国家码缺席或不成形答资料不足；国家不同即不覆盖，不再看邮编；前缀形态下
// 缺邮编答资料不足，有邮编就逐字比前缀。
func (coverage ServiceAreaCoverage) Match(side GeoResolutionSide) CoverageMatch {
	country, hasCountry := side.Country()
	if !hasCountry || !isCountryCodeShape(country) {
		return CoverageInsufficient
	}
	if country != coverage.country {
		return CoverageNotCovered
	}
	if len(coverage.prefixes) == 0 {
		return CoverageCovered
	}
	postalCode, hasPostalCode := side.PostalCode()
	if !hasPostalCode {
		return CoverageInsufficient
	}
	for _, prefix := range coverage.prefixes {
		if strings.HasPrefix(postalCode, prefix) {
			return CoverageCovered
		}
	}
	return CoverageNotCovered
}

// isCountryCodeShape 判 ISO 3166 两位大写字母的形状；码表不内置，与目录其余国家格同一判法。
func isCountryCodeShape(value string) bool {
	if len(value) != 2 {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < 'A' || value[index] > 'Z' {
			return false
		}
	}
	return true
}
