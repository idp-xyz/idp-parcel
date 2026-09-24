package cataloguepage_test

import (
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

func decode(t *testing.T, values url.Values) cataloguepage.Query {
	t.Helper()
	query, err := accounts.Decode(values)
	if err != nil {
		t.Fatalf("解码 %v：%v", values, err)
	}
	return query
}

// refused 断言这次查询被拒成 MalformedQuery，并交回给操作者看的理由散文。
func refused(t *testing.T, values url.Values) string {
	t.Helper()
	_, err := accounts.Decode(values)
	var malformed *cataloguepage.MalformedQuery
	if !errors.As(err, &malformed) {
		t.Fatalf("%v 应拒成 MalformedQuery，实得 %v", values, err)
	}
	if malformed.Reason == "" {
		t.Fatalf("%v 的拒绝没有理由散文", values)
	}
	return malformed.Reason
}

func TestAnEmptyQueryIsTheFirstPageInTheDefaultOrder(t *testing.T) {
	query := decode(t, url.Values{})
	if query.Sort != (cataloguepage.Sort{Field: "registeredAt", Descending: true}) {
		t.Fatalf("缺省序：得 %v", query.Sort)
	}
	if len(query.Filters) != 0 || query.Keyword != "" {
		t.Fatalf("空查询应不带筛选与检索，得 %+v", query)
	}
}

// Covers: ADR-0144 决定四「未知参数键 → MALFORMED_REQUEST」——多给的键若被静默忽略，前端以为筛了、服务端
// 其实没筛。页大小不开调用方参数（决定二），所以 limit / pageSize 同样是集外键；键名区分大小写。
func TestAKeyOutsideTheDeclaredSetIsRefused(t *testing.T) {
	for _, key := range []string{"limit", "pageSize", "page", "Status", "kind", "foo"} {
		t.Run(key, func(t *testing.T) {
			reason := refused(t, url.Values{key: {"1"}})
			if !strings.Contains(reason, key) {
				t.Fatalf("理由应点名 %s，得 %q", key, reason)
			}
		})
	}
}

func TestTheSortParameterReadsTheDescendingPrefix(t *testing.T) {
	cases := map[string]cataloguepage.Sort{
		"accountId":     {Field: "accountId"},
		"-revision":     {Field: "revision", Descending: true},
		"registeredAt":  {Field: "registeredAt"},
		"-registeredAt": {Field: "registeredAt", Descending: true},
	}
	for given, want := range cases {
		if got := decode(t, url.Values{"sort": {given}}).Sort; got != want {
			t.Fatalf("sort=%s：得 %v，应为 %v", given, got, want)
		}
	}
}

// Covers: ADR-0144 决定三「可排的维由各册在自己的读口里列出，集外 → MALFORMED_REQUEST」。
func TestASortOutsideTheDeclaredDimensionsIsRefused(t *testing.T) {
	for _, given := range []string{"name", "-name", "", "-", "--registeredAt", "RegisteredAt"} {
		t.Run(given, func(t *testing.T) {
			refused(t, url.Values{"sort": {given}})
		})
	}
}

// Covers: ADR-0144 决定四「值属封闭词表的维，值也按词表校验」；词表按原样比，不替调用方改大小写或去空白。
// 空值无论维是否设词表都拒——它筛不出任何东西，却会让人以为筛过了。
func TestAFilterValueOutsideTheVocabularyIsRefused(t *testing.T) {
	cases := map[string]url.Values{
		"词表外":       {"status": {"ACTIVE"}},
		"大小写不同":     {"status": {"effective"}},
		"带空白":       {"status": {" EFFECTIVE"}},
		"多值里混一个词表外": {"status": {"EFFECTIVE", "ACTIVE"}},
		"封闭维空值":     {"status": {""}},
		"不设词表的维空值":  {"customerPartyId": {""}},
	}
	for name, values := range cases {
		t.Run(name, func(t *testing.T) {
			refused(t, values)
		})
	}
}

func TestADimensionWithoutAVocabularyTakesAnyNonEmptyValue(t *testing.T) {
	query := decode(t, url.Values{"customerPartyId": {"SYN-PARTY-01"}})
	if got := query.Filters["customerPartyId"]; !reflect.DeepEqual(got, []string{"SYN-PARTY-01"}) {
		t.Fatalf("得 %v", got)
	}
}

// Covers: ADR-0144 决定四「同一维可重复给，维内为『或』、维间为『与』」。重复给同一个值不改变含义，去重；
// 值按升序排，给的次序不同解出同一个查询对象。
func TestARepeatedDimensionDecodesToSeveralValues(t *testing.T) {
	query := decode(t, url.Values{
		"status":          {"REGISTERED", "EFFECTIVE", "REGISTERED"},
		"customerPartyId": {"SYN-PARTY-02"},
	})
	if got := query.Filters["status"]; !reflect.DeepEqual(got, []string{"EFFECTIVE", "REGISTERED"}) {
		t.Fatalf("status 应解成去重后升序的两个值，得 %v", got)
	}
	if got := query.Filters["customerPartyId"]; !reflect.DeepEqual(got, []string{"SYN-PARTY-02"}) {
		t.Fatalf("另一维不受影响，得 %v", got)
	}
}

// Covers: ADR-0144 决定四「q：去掉首尾空白，空即缺席」。
func TestTheKeywordIsTrimmedAndEmptyMeansAbsent(t *testing.T) {
	cases := map[string]string{
		"  SYN-ACC-01 ": "SYN-ACC-01",
		"\tSYN 客户\n":    "SYN 客户",
		"   ":           "",
		"":              "",
	}
	for given, want := range cases {
		if got := decode(t, url.Values{"q": {given}}).Keyword; got != want {
			t.Fatalf("q=%q：得 %q，应为 %q", given, got, want)
		}
	}
}

// Covers: ADR-0144 决定四「长度上限在共用件里定一次」。按字符计而不是按字节：中文名称一字三字节，
// 按字节计会让中文检索词只剩三分之一的余量。首尾空白不计入。
func TestTheKeywordLengthLimitCountsCharacters(t *testing.T) {
	atLimit := strings.Repeat("客", cataloguepage.MaxKeywordRunes)
	if got := decode(t, url.Values{"q": {"  " + atLimit + "  "}}).Keyword; got != atLimit {
		t.Fatalf("恰好到上限的检索词应被接受并去掉首尾空白")
	}
	reason := refused(t, url.Values{"q": {atLimit + "户"}})
	if !strings.Contains(reason, "q") {
		t.Fatalf("理由应点名 q，得 %q", reason)
	}
}

// Covers: ADR-0144 决定三「一次只按一维排」；q 与游标同样只有一个。给两次时取哪一个都是替调用方做决定。
func TestASingularKeyGivenTwiceIsRefused(t *testing.T) {
	for _, values := range []url.Values{
		{"sort": {"accountId", "revision"}},
		{"q": {"SYN-A", "SYN-B"}},
	} {
		refused(t, values)
	}
}

// Covers: ADR-0144 决定四「已被用作册子选择器的参数保持原义，不兼作筛选维」——选择器由端点自己读，
// 共用件不拒它，也不把它解成筛选维。
func TestASelectorIsAcceptedButNotDecodedAsAFilter(t *testing.T) {
	query := decode(t, url.Values{"family": {"SYN-FAMILY"}})
	if _, present := query.Filters["family"]; present {
		t.Fatalf("选择器被解成了筛选维：%v", query.Filters)
	}
}

func TestTheRefusalErrorCarriesItsReason(t *testing.T) {
	_, err := accounts.Decode(url.Values{"foo": {"1"}})
	var malformed *cataloguepage.MalformedQuery
	if !errors.As(err, &malformed) || !strings.Contains(err.Error(), malformed.Reason) {
		t.Fatalf("错误串应含理由散文，得 %v", err)
	}
}
