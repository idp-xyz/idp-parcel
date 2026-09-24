// Package cataloguepage 是各上下文目录读口共用的翻页、排序与筛选件（ADR-0144 决定七）：游标编解码、
// 查询参数校验与答复里 page 一格的拼装。它不含任何上下文的词——各册只声明自己的可排维、筛选维、行标识
// 与缺省序；q 覆盖哪几列、游标位置怎样落成 SQL 的比较，都在各册自己的读面里。
package cataloguepage

import (
	"errors"
	"fmt"
)

// MaxKeywordRunes 是检索词 q 去掉首尾空白之后的长度上限，按字符计。ADR-0144 决定四要求它在共用件里
// 定一次，各册不另设。
const MaxKeywordRunes = 100

// 翻页、排序与检索的参数名在各册之间同名同义，筛选维与册子选择器都不得占用。
const (
	keyAfter   = "after"
	keySort    = "sort"
	keyKeyword = "q"
)

// ValueKind 是游标里一格值的形状。游标对客户端不透明，却不防篡改：解码时按它校验，伪造的值才会在
// 接入面答 400，而不是带进 SQL 变成 500。
type ValueKind string

const (
	Text    ValueKind = "text"
	Integer ValueKind = "integer"
	// Instant 在游标里只认 FormatInstant 的写法：UTC 的 RFC 3339，秒以下按需。
	Instant ValueKind = "instant"
)

func (kind ValueKind) valid() bool {
	return kind == Text || kind == Integer || kind == Instant
}

// SortDimension 是一个可排维。Name 取答复体里的 JSON 字段名（ADR-0144 决定三）。
type SortDimension struct {
	Name string
	Kind ValueKind
}

// FilterDimension 是一个筛选维。Name 取答复体里的 JSON 字段名（ADR-0144 决定四）；Vocabulary 非空即
// 封闭词表，值不在其中即拒，为空表示这一维不设词表（如按某个标识筛）。
type FilterDimension struct {
	Name       string
	Vocabulary []string
}

// Sort 是一次的排序。一次只按一维排；决胜键总是行标识，与排序同向（ADR-0144 决定三）。
type Sort struct {
	Field      string
	Descending bool
}

// String 给出 sort 参数上的写法。
func (sort Sort) String() string {
	if sort.Descending {
		return "-" + sort.Field
	}
	return sort.Field
}

// Spec 是一册交给共用件的声明。
type Spec struct {
	// Name 区分不同的列表并进游标摘要：一册的游标拿到另一册去用，翻出来的同样是另一份列表的中段。
	Name        string
	Sorts       []SortDimension
	DefaultSort Sort
	// Identity 是行标识各段的形状，按读面 ORDER BY 里决胜键的次序；组合键（如代码 + 版本号）逐段列出。
	Identity []ValueKind
	Filters  []FilterDimension
	// Selectors 是端点自己消费的册子选择器键（如 /network-catalog 的 family），保持原义、不兼作筛选维
	// （ADR-0144 决定四）。解码既不拒也不解它们，但取值进游标摘要——选择器一变，就是另一册。
	Selectors []string
}

// Catalogue 是校验过的一册声明，由 NewCatalogue 构造。
type Catalogue struct {
	name        string
	sorts       map[string]ValueKind
	defaultSort Sort
	identity    []ValueKind
	// filters 按维给封闭词表；值为 nil 的维不设词表。
	filters   map[string]map[string]bool
	selectors map[string]bool
}

// NewCatalogue 校验一册声明。声明是各册写死的常量，这里报错即编程错误，应在装配时就暴露。
func NewCatalogue(spec Spec) (*Catalogue, error) {
	if spec.Name == "" {
		return nil, errors.New("cataloguepage: 册名为空")
	}
	catalogue := &Catalogue{
		name:      spec.Name,
		sorts:     make(map[string]ValueKind, len(spec.Sorts)),
		filters:   make(map[string]map[string]bool, len(spec.Filters)),
		selectors: make(map[string]bool, len(spec.Selectors)),
	}

	if len(spec.Sorts) == 0 {
		return nil, fmt.Errorf("cataloguepage: 册 %s 没有可排维", spec.Name)
	}
	for _, dimension := range spec.Sorts {
		if dimension.Name == "" {
			return nil, fmt.Errorf("cataloguepage: 册 %s 有可排维名为空", spec.Name)
		}
		if _, seen := catalogue.sorts[dimension.Name]; seen {
			return nil, fmt.Errorf("cataloguepage: 册 %s 的可排维 %s 重名", spec.Name, dimension.Name)
		}
		if !dimension.Kind.valid() {
			return nil, fmt.Errorf("cataloguepage: 册 %s 的可排维 %s 值形状 %q 未知", spec.Name, dimension.Name, dimension.Kind)
		}
		catalogue.sorts[dimension.Name] = dimension.Kind
	}
	if _, declared := catalogue.sorts[spec.DefaultSort.Field]; !declared {
		return nil, fmt.Errorf("cataloguepage: 册 %s 的缺省序 %s 不在可排维里", spec.Name, spec.DefaultSort)
	}
	catalogue.defaultSort = spec.DefaultSort

	if len(spec.Identity) == 0 {
		return nil, fmt.Errorf("cataloguepage: 册 %s 没有行标识", spec.Name)
	}
	for index, kind := range spec.Identity {
		if !kind.valid() {
			return nil, fmt.Errorf("cataloguepage: 册 %s 的行标识第 %d 段值形状 %q 未知", spec.Name, index+1, kind)
		}
	}
	catalogue.identity = append([]ValueKind(nil), spec.Identity...)

	// 筛选维与选择器都是查询串上的键，与保留键共用一个名字空间；可排维只出现在 sort 的值里，不在其中。
	keys := map[string]bool{keyAfter: true, keySort: true, keyKeyword: true}
	claim := func(role, name string) error {
		if name == "" {
			return fmt.Errorf("cataloguepage: 册 %s 有%s名为空", spec.Name, role)
		}
		if keys[name] {
			return fmt.Errorf("cataloguepage: 册 %s 的%s %s 与保留键或其他键重名", spec.Name, role, name)
		}
		keys[name] = true
		return nil
	}
	for _, dimension := range spec.Filters {
		if err := claim("筛选维", dimension.Name); err != nil {
			return nil, err
		}
		var vocabulary map[string]bool
		if len(dimension.Vocabulary) > 0 {
			vocabulary = make(map[string]bool, len(dimension.Vocabulary))
			for _, value := range dimension.Vocabulary {
				if value == "" {
					return nil, fmt.Errorf("cataloguepage: 册 %s 的筛选维 %s 词表里有空值", spec.Name, dimension.Name)
				}
				vocabulary[value] = true
			}
		}
		catalogue.filters[dimension.Name] = vocabulary
	}
	for _, selector := range spec.Selectors {
		if err := claim("选择器", selector); err != nil {
			return nil, err
		}
		catalogue.selectors[selector] = true
	}
	return catalogue, nil
}

// MustCatalogue 是 NewCatalogue 的包级声明写法：声明不合规即 panic。
func MustCatalogue(spec Spec) *Catalogue {
	catalogue, err := NewCatalogue(spec)
	if err != nil {
		panic(err)
	}
	return catalogue
}
