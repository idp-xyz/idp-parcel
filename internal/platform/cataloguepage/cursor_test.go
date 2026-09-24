package cataloguepage_test

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// staleCursor 是 ADR-0144 决定一点名的理由散文：游标解不开、或与本次的排序 / 筛选不符，都说这一句。
const staleCursor = "游标与本次的排序或筛选不符，请从第一页重取"

var registeredAt = time.Date(2026, 9, 24, 6, 18, 31, 983129000, time.UTC)

func lastRow() cataloguepage.Position {
	return cataloguepage.Position{
		Value:    cataloguepage.FormatInstant(registeredAt),
		Identity: []string{"SYN-ACC-07", cataloguepage.FormatInteger(3)},
	}
}

// with 在一组查询参数上接一个游标，返回新的一组，不改原来那组。
func with(values url.Values, cursor string) url.Values {
	next := url.Values{}
	for key, given := range values {
		next[key] = append([]string(nil), given...)
	}
	next.Set("after", cursor)
	return next
}

func cursorAfter(t *testing.T, values url.Values, last cataloguepage.Position) string {
	t.Helper()
	cursor, err := decode(t, values).CursorAfter(last)
	if err != nil {
		t.Fatalf("编游标：%v", err)
	}
	return cursor
}

func TestACursorRoundTripsThePositionOfTheLastRow(t *testing.T) {
	first := url.Values{
		"status":          {"EFFECTIVE", "REGISTERED"},
		"customerPartyId": {"SYN-PARTY-01"},
		"q":               {"SYN"},
		"family":          {"SYN-FAMILY"},
	}
	next := decode(t, with(first, cursorAfter(t, first, lastRow())))

	if next.After == nil || !reflect.DeepEqual(*next.After, lastRow()) {
		t.Fatalf("游标解回的位置：得 %+v，应为 %+v", next.After, lastRow())
	}
	at, err := cataloguepage.ParseInstant(next.After.Value)
	if err != nil || !at.Equal(registeredAt) {
		t.Fatalf("排序维值取回：得 %v（%v）", at, err)
	}
	revision, err := cataloguepage.ParseInteger(next.After.Identity[1])
	if err != nil || revision != 3 {
		t.Fatalf("标识整数段取回：得 %d（%v）", revision, err)
	}
	if next.Sort != decode(t, first).Sort || !reflect.DeepEqual(next.Filters, decode(t, first).Filters) || next.Keyword != "SYN" {
		t.Fatalf("接游标的查询应保留原条件，得 %+v", next)
	}
}

func TestACursorRoundTripsUnderEachValueKind(t *testing.T) {
	cases := map[string]cataloguepage.Position{
		"accountId": {Value: "SYN-ACC-07", Identity: []string{"SYN-ACC-07", "1"}},
		"-revision": {Value: cataloguepage.FormatInteger(12), Identity: []string{"SYN-ACC-07", "12"}},
	}
	for sort, last := range cases {
		t.Run(sort, func(t *testing.T) {
			first := url.Values{"sort": {sort}}
			next := decode(t, with(first, cursorAfter(t, first, last)))
			if next.After == nil || !reflect.DeepEqual(*next.After, last) {
				t.Fatalf("得 %+v，应为 %+v", next.After, last)
			}
		})
	}
}

func TestTheFirstPageHasNoPosition(t *testing.T) {
	if decode(t, url.Values{}).After != nil {
		t.Fatal("不带 after 应是第一页")
	}
}

// Covers: ADR-0144 决定一「摘要与本次请求的排序 / 筛选不符 → MALFORMED_REQUEST」——换了条件还拿旧游标，
// 翻出来的是另一份列表的中段，不能静默照翻。选择器一变就是另一册，同样拒。
func TestAnOldCursorIsRefusedOnceTheConditionsChange(t *testing.T) {
	first := url.Values{"status": {"EFFECTIVE"}, "q": {"SYN"}, "family": {"SYN-FAMILY"}}
	cursor := cursorAfter(t, first, lastRow())

	changes := map[string]func(url.Values){
		"换排序维":   func(values url.Values) { values.Set("sort", "accountId") },
		"换排序方向":  func(values url.Values) { values.Set("sort", "registeredAt") },
		"换筛选值":   func(values url.Values) { values.Set("status", "DEACTIVATED") },
		"加一个筛选值": func(values url.Values) { values.Add("status", "REGISTERED") },
		"去掉筛选维":  func(values url.Values) { values.Del("status") },
		"加一个筛选维": func(values url.Values) { values.Set("customerPartyId", "SYN-PARTY-01") },
		"换检索词":   func(values url.Values) { values.Set("q", "SYN-2") },
		"去掉检索词":  func(values url.Values) { values.Del("q") },
		"换选择器":   func(values url.Values) { values.Set("family", "SYN-OTHER") },
		"去掉选择器":  func(values url.Values) { values.Del("family") },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			values := with(first, cursor)
			change(values)
			if reason := refused(t, values); reason != staleCursor {
				t.Fatalf("理由：得 %q，应为 %q", reason, staleCursor)
			}
		})
	}
}

// Covers: 条件是同一组、只是写法不同（维内次序、重复给同值、q 的首尾空白）时，摘要必须相同——否则客户端
// 原样回送自己的条件也会被当成换了条件。
func TestTheSameConditionsWrittenDifferentlyKeepTheCursor(t *testing.T) {
	cursor := cursorAfter(t, url.Values{"status": {"EFFECTIVE", "REGISTERED"}, "q": {"SYN"}}, lastRow())
	rewritten := url.Values{"status": {"REGISTERED", "EFFECTIVE", "REGISTERED"}, "q": {"  SYN "}}
	if next := decode(t, with(rewritten, cursor)); next.After == nil {
		t.Fatal("同一组条件换了写法，游标应仍可用")
	}
}

func TestACursorFromAnotherCatalogueIsRefused(t *testing.T) {
	spec := accountsSpec()
	spec.Name = "SYN-other-accounts"
	other := cataloguepage.MustCatalogue(spec)
	query, err := other.Decode(url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := query.CursorAfter(lastRow())
	if err != nil {
		t.Fatal(err)
	}
	if reason := refused(t, url.Values{"after": {cursor}}); reason != staleCursor {
		t.Fatalf("理由：得 %q", reason)
	}
}

// tamper 按游标此刻的线上形状（base64url 包着的 JSON）拆开改一格再包回去。游标不防篡改，
// 解码时的校验是唯一的一道：伪造的值要在这里答 400，不能带进 SQL。
func tamper(t *testing.T, cursor string, change func(body map[string]any)) string {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	change(body)
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded)
}

// trailing 在游标解开后的字节末尾接上 suffix 再包回去：前面那段 JSON 本身完好。
func trailing(t *testing.T, cursor, suffix string) string {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(append(raw, suffix...))
}

// Covers: ADR-0144 决定一「游标解不开 → MALFORMED_REQUEST」，理由散文与摘要不符时同一句。
func TestAnUndecodableOrForgedCursorIsRefused(t *testing.T) {
	good := cursorAfter(t, url.Values{}, lastRow())
	cursors := map[string]string{
		"空串":        "",
		"不是 base64": "!!!",
		"不是 JSON":   base64.RawURLEncoding.EncodeToString([]byte("SYN-not-json")),
		"多一格":       tamper(t, good, func(body map[string]any) { body["x"] = "SYN" }),
		"排序维被改":     tamper(t, good, func(body map[string]any) { body["s"] = "accountId" }),
		"时刻带时区偏移":   tamper(t, good, func(body map[string]any) { body["v"] = "2026-09-24T14:18:31+08:00" }),
		"时刻不成形":     tamper(t, good, func(body map[string]any) { body["v"] = "SYN-yesterday" }),
		"标识少一段":     tamper(t, good, func(body map[string]any) { body["i"] = []string{"SYN-ACC-07"} }),
		"标识多一段":     tamper(t, good, func(body map[string]any) { body["i"] = []string{"SYN-ACC-07", "3", "SYN"} }),
		"标识整数段带前导零": tamper(t, good, func(body map[string]any) { body["i"] = []string{"SYN-ACC-07", "03"} }),
		"标识整数段不是整数": tamper(t, good, func(body map[string]any) { body["i"] = []string{"SYN-ACC-07", "three"} }),
		"摘要被改":      tamper(t, good, func(body map[string]any) { body["d"] = "SYN" }),
		"末尾拖着多余的字节": trailing(t, good, ` {"s":"SYN"}`),
	}
	for name, cursor := range cursors {
		t.Run(name, func(t *testing.T) {
			if reason := refused(t, url.Values{"after": {cursor}}); reason != staleCursor {
				t.Fatalf("理由：得 %q，应为 %q", reason, staleCursor)
			}
		})
	}
}

func TestACursorGivenTwiceIsRefused(t *testing.T) {
	cursor := cursorAfter(t, url.Values{}, lastRow())
	refused(t, url.Values{"after": {cursor, cursor}})
}

// Covers: 读面交来的末行位置不合本册声明是它的编程错误——照实报出来，不编出一个下次解不开的游标。
func TestCursorAfterRefusesAPositionThatDoesNotFitTheDeclaration(t *testing.T) {
	query := decode(t, url.Values{})
	positions := map[string]cataloguepage.Position{
		"标识段数不符":     {Value: cataloguepage.FormatInstant(registeredAt), Identity: []string{"SYN-ACC-07"}},
		"排序维值不是规整时刻": {Value: "2026-09-24 06:18:31", Identity: []string{"SYN-ACC-07", "3"}},
		"标识整数段不规整":   {Value: cataloguepage.FormatInstant(registeredAt), Identity: []string{"SYN-ACC-07", "+3"}},
	}
	for name, position := range positions {
		t.Run(name, func(t *testing.T) {
			if _, err := query.CursorAfter(position); err == nil {
				t.Fatal("不合声明的位置编出了游标")
			}
		})
	}
	if _, err := (cataloguepage.Query{}).CursorAfter(lastRow()); err == nil {
		t.Fatal("不是 Decode 解出的查询对象编出了游标")
	}
}

// Covers: 一个值在游标里只有一种写法：规整形状之外的写法一律拒，往返才稳，伪造的变体也进不来。
func TestTheValueFormatsAcceptOnlyTheirCanonicalSpelling(t *testing.T) {
	if got := cataloguepage.FormatInstant(time.Date(2026, 9, 24, 14, 18, 31, 0, time.FixedZone("CST", 8*3600))); got != "2026-09-24T06:18:31Z" {
		t.Fatalf("时刻应规整为 UTC，得 %q", got)
	}
	for _, spelling := range []string{"2026-09-24T14:18:31+08:00", "2026-09-24T06:18:31.000Z", "2026-09-24", ""} {
		if _, err := cataloguepage.ParseInstant(spelling); err == nil {
			t.Fatalf("非规整时刻 %q 被接受", spelling)
		}
	}
	for _, spelling := range []string{"03", "+3", "3.0", "-0", "", "SYN"} {
		if _, err := cataloguepage.ParseInteger(spelling); err == nil {
			t.Fatalf("非规整整数 %q 被接受", spelling)
		}
	}
	if value, err := cataloguepage.ParseInteger(cataloguepage.FormatInteger(-42)); err != nil || value != -42 {
		t.Fatalf("负整数往返：得 %d（%v）", value, err)
	}
}
