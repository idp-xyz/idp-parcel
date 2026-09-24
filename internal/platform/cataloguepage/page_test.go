package cataloguepage_test

import (
	"encoding/json"
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

// Covers: ADR-0144 决定五「契约允许将来的大表读口显式给 null」——total 缺的时候编出 null，不是 0：
// 0 会让客户端说「共 0 条」。
func TestAMissingTotalIsNullNotZero(t *testing.T) {
	page := cataloguepage.NewPage(200, "", 0)
	page.Total = nil
	if got := marshal(t, page); got != `{"size":200,"next":null,"total":null}` {
		t.Fatalf("得 %s", got)
	}
}
