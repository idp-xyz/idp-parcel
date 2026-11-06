package cataloguepage_test

import (
	"net/url"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

type accountRow struct {
	id         string
	revision   int64
	registered time.Time
}

func accountPosition(row accountRow, field string) cataloguepage.Position {
	value := cataloguepage.FormatInstant(row.registered)
	switch field {
	case "accountId":
		value = row.id
	case "revision":
		value = cataloguepage.FormatInteger(row.revision)
	}
	return cataloguepage.Position{Value: value, Identity: []string{row.id, cataloguepage.FormatInteger(row.revision)}}
}

func accountRows(count int) []accountRow {
	rows := make([]accountRow, count)
	for index := range rows {
		rows[index] = accountRow{
			id:         "SYN-ACC-0" + string(rune('9'-index)),
			revision:   int64(index + 1),
			registered: registeredAt.Add(-time.Duration(index) * time.Minute),
		}
	}
	return rows
}

// Covers: 多取的那一行只用来判有没有下一页、不交给调用方；下一页的游标编的是本页末行的位置，接着它解出来的查询
// 就从那一行之后取（ADR-0144 决定一、五）。
func TestFinishCutsTheExtraRowAndEncodesTheCursorAfterTheLastRow(t *testing.T) {
	values := url.Values{"status": {"EFFECTIVE"}}
	query := decode(t, values)
	fetched := accountRows(3)

	result, err := cataloguepage.Finish(query, fetched, 2, 17, accountPosition)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Rows, fetched[:2]) || result.Total != 17 || result.Next == "" {
		t.Fatalf("得 %+v", result)
	}
	next := decode(t, with(values, result.Next))
	if next.After == nil || !reflect.DeepEqual(*next.After, accountPosition(fetched[1], "registeredAt")) {
		t.Fatalf("游标解回的位置：得 %+v，应为本页末行", next.After)
	}
}

// Covers: 恰好一页、不满一页与空页都是末页：没有游标，客户端据此停在这里，不去请求一页注定为空的下一页。
func TestFinishOnTheLastPageHasNoCursor(t *testing.T) {
	for name, count := range map[string]int{"恰好一页": 2, "不满一页": 1, "空": 0} {
		t.Run(name, func(t *testing.T) {
			result, err := cataloguepage.Finish(decode(t, url.Values{}), accountRows(count), 2, int64(count), accountPosition)
			if err != nil {
				t.Fatal(err)
			}
			if result.Next != "" || len(result.Rows) != count {
				t.Fatalf("得 %+v", result)
			}
		})
	}
}

func TestFinishRefusesANonPositiveLimit(t *testing.T) {
	if _, err := cataloguepage.Finish(decode(t, url.Values{}), accountRows(1), 0, 1, accountPosition); err == nil {
		t.Fatal("页大小为零应被拒")
	}
}

// Covers: 结果直接拼出答复的 page：size 是本次的页大小、不是本页行数（ADR-0144 决定五）。
func TestAResultSpellsItsPage(t *testing.T) {
	result := cataloguepage.Result[accountRow]{Rows: accountRows(1), Next: "SYN-cursor", Total: 431}
	if got := marshal(t, result.Page(200)); got != `{"size":200,"next":"SYN-cursor","total":431}` {
		t.Fatalf("得 %s", got)
	}
	if got := marshal(t, cataloguepage.Result[accountRow]{}.Page(200)); got != `{"size":200,"next":null,"total":0}` {
		t.Fatalf("空页：得 %s", got)
	}
}
