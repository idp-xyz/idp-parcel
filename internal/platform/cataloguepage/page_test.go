package cataloguepage_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

func marshal(t *testing.T, page cataloguepage.Page) string {
	t.Helper()
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// Covers: ADR-0144 决定五——空目录仍如实答空：next 为 null、total 为 0（ADR-0077 Decision 四不动）；
// 三格都在场，不因为没有下一页就省掉 next。
func TestAnEmptyCatalogueAnswersTheLastPageWithZeroTotal(t *testing.T) {
	if got := marshal(t, cataloguepage.NewPage(200, "", 0)); got != `{"size":200,"next":null,"total":0}` {
		t.Fatalf("得 %s", got)
	}
}

// Covers: ADR-0144 决定五「size 是本次使用的页大小（不是本页实际行数）」——page 只回显调用方给的数，
// 不去数行；next 原样带出游标。
func TestAPageWithMoreRowsCarriesTheNextCursor(t *testing.T) {
	if got := marshal(t, cataloguepage.NewPage(200, "SYN-cursor", 431)); got != `{"size":200,"next":"SYN-cursor","total":431}` {
		t.Fatalf("得 %s", got)
	}
}

// Covers: 读面照页大小多取一行，只拿那一行判断有没有下一页（ADR-0144 决定一、五）；多出来的行不交给
// 调用方，否则本页会比 page.size 多一行，末行位置也跟着错一格。
func TestTrimCutsTheExtraRowAndReportsWhetherMoreFollow(t *testing.T) {
	cases := map[string]struct {
		rows []string
		want []string
		more bool
	}{
		"多取的那一行在": {rows: []string{"SYN-1", "SYN-2", "SYN-3"}, want: []string{"SYN-1", "SYN-2"}, more: true},
		"恰好一页":    {rows: []string{"SYN-1", "SYN-2"}, want: []string{"SYN-1", "SYN-2"}, more: false},
		"不满一页":    {rows: []string{"SYN-1"}, want: []string{"SYN-1"}, more: false},
		"空":       {rows: nil, want: nil, more: false},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			page, more := cataloguepage.Trim(test.rows, 2)
			if !reflect.DeepEqual(page, test.want) || more != test.more {
				t.Fatalf("得 %v / %v，应为 %v / %v", page, more, test.want, test.more)
			}
		})
	}
}

// Covers: ADR-0144 决定五「契约允许将来的大表读口显式给 null」——total 缺的时候编出 null，不是 0：
// 0 会让客户端说「共 0 条」。
func TestAMissingTotalIsNullNotZero(t *testing.T) {
	page := cataloguepage.NewPage(200, "", 0)
	page.Total = nil
	if got := marshal(t, page); got != `{"size":200,"next":null,"total":null}` {
		t.Fatalf("得 %s", got)
	}
}
