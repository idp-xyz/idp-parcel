package cataloguepage

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	"go.idp.xyz/idp-parcel/internal/platform/storable"
)

// Query 是解码后的一次目录查询，由端点处理器与作用域、页大小一起交给读端口（ADR-0144 决定六）。
type Query struct {
	Sort Sort
	// Filters 按维给值：维内为或、维间为与（ADR-0144 决定四）。值去重后升序；缺席的维不在其中。
	Filters map[string][]string
	// Keyword 是去掉首尾空白的 q；空串即缺席。
	Keyword string
	// After 是上一页末行的位置，读面取严格排在它之后的行；nil 即第一页。
	After *Position

	catalogue *Catalogue
	digest    string
}

// MalformedQuery 是查询参数不成立的拒绝。各上下文把它映射到 MALFORMED_REQUEST，Reason 原样放进
// problemDetail.Detail——它是给操作者看的散文，调用方不据此分支。
type MalformedQuery struct {
	Reason string
}

func (refusal *MalformedQuery) Error() string {
	return "cataloguepage: 查询不成立：" + refusal.Reason
}

func malformed(format string, arguments ...any) *MalformedQuery {
	return &MalformedQuery{Reason: fmt.Sprintf(format, arguments...)}
}

// ParseQuery 照 url.ParseQuery 解请求的原始查询串（ADR-0144 决定四、七）。百分号转义不完整或用了分号分隔的那一段解不开，
// 整条答 *MalformedQuery：URL.Query() 会把那一段静默丢下、余下的照常作答，一个写坏的筛选值就成了没给，答出整册。理由不回显
// 那一段——解不开的字节原样回给调用方，日志与界面都得再防一次。
//
// 册名、选择器这类由端点自己读的键，从这里交回的同一份取值里读，再把整份交给 Decode，不另解一遍。
//
// 唯一的例外是 Intake 之前的分派：按某个键在不在选 Intake 的口，只能照 URL.Query() 宽松地看。在那里调本函数，解不开的段就会
// 在 Intake 之前答 400，渠道未配置时答复随请求内容变（ADR-0055 决定三）。分派键本身写坏时，宽松读把它丢下，请求落进不带
// 该键的列表那一支；那一支在 Intake 之后照本函数再解，坏段仍答 400。
func ParseQuery(raw string) (url.Values, error) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return nil, malformed("查询串里有解不开的一段：百分号转义不完整，或用了分号分隔")
	}
	return values, nil
}

// DecodeRaw 从请求的原始查询串解出本册的一次查询：先 ParseQuery，再 Decode。端点不自己读任何键时用它。
func (catalogue *Catalogue) DecodeRaw(raw string) (Query, error) {
	values, err := ParseQuery(raw)
	if err != nil {
		return Query{}, err
	}
	return catalogue.Decode(values)
}

// Decode 从解好的查询参数解出本册的一次查询。集外键、集外排序维、词表外或为空的筛选值、超长的 q、库存不下的 q、筛选值与
// 选择器取值、只能给一次却给了多次的键、解不开或与本次条件不符的游标，一律答 *MalformedQuery。
//
// 端点不拿 URL.Query() 喂它：那一份已把解不开的段静默丢下，到这里再也看不见。从请求来的取值走 DecodeRaw，或先经 ParseQuery。
func (catalogue *Catalogue) Decode(values url.Values) (Query, error) {
	query := Query{Sort: catalogue.defaultSort, Filters: map[string][]string{}, catalogue: catalogue}
	selectors := map[string][]string{}
	cursor, hasCursor := "", false

	// 按键名顺序走，同一个坏请求每次报同一条理由。
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		given := values[key]
		switch {
		case key == keySort:
			value, err := single(key, given)
			if err != nil {
				return Query{}, err
			}
			field, descending := strings.CutPrefix(value, "-")
			if _, declared := catalogue.sorts[field]; !declared {
				return Query{}, malformed("排序维 %q 不在本目录的可排维度内", value)
			}
			query.Sort = Sort{Field: field, Descending: descending}
		case key == keyKeyword:
			value, err := single(key, given)
			if err != nil {
				return Query{}, err
			}
			keyword := strings.TrimSpace(value)
			if !storable.Text(keyword) {
				return Query{}, malformed("检索词 q 含库存不下的字符（NUL 或不合法的 UTF-8）")
			}
			if utf8.RuneCountInString(keyword) > MaxKeywordRunes {
				return Query{}, malformed("检索词 q 超过 %d 个字符", MaxKeywordRunes)
			}
			query.Keyword = keyword
		case key == keyAfter:
			value, err := single(key, given)
			if err != nil {
				return Query{}, err
			}
			cursor, hasCursor = value, true
		case catalogue.selectors[key]:
			// 选择器的取值归端点自己读、自己判含义；这里只判它存不存得进库，再记进摘要。端点拿它去查库之前都要过这一步，
			// 带 NUL 的选择器就和筛选值一样进不了 SQL。
			for _, value := range given {
				if !storable.Text(value) {
					return Query{}, malformed("选择器 %s 的取值含库存不下的字符（NUL 或不合法的 UTF-8）", key)
				}
			}
			selectors[key] = canonical(given)
		default:
			vocabulary, isFilter := catalogue.filters[key]
			if !isFilter {
				return Query{}, malformed("查询参数 %s 不在本目录接受的集合内", key)
			}
			for _, value := range given {
				if value == "" {
					return Query{}, malformed("筛选维 %s 的取值为空", key)
				}
				if !storable.Text(value) {
					return Query{}, malformed("筛选维 %s 的取值含库存不下的字符（NUL 或不合法的 UTF-8）", key)
				}
				if vocabulary != nil && !vocabulary[value] {
					return Query{}, malformed("筛选维 %s 的取值 %q 不在词表内", key, value)
				}
			}
			query.Filters[key] = canonical(given)
		}
	}

	// 摘要要等条件全部解完才算得出，游标因此最后解。
	query.digest = catalogue.digest(query, selectors)
	if hasCursor {
		position, err := catalogue.decodeCursor(cursor, query)
		if err != nil {
			return Query{}, err
		}
		query.After = &position
	}
	return query, nil
}

func single(key string, given []string) (string, error) {
	if len(given) != 1 {
		return "", malformed("查询参数 %s 只能给一次", key)
	}
	return given[0], nil
}

// canonical 去重并升序：维内为或，次序与重复都不改变含义，同一组条件只有一种写法。
func canonical(values []string) []string {
	seen := make(map[string]bool, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			unique = append(unique, value)
		}
	}
	sort.Strings(unique)
	return unique
}
