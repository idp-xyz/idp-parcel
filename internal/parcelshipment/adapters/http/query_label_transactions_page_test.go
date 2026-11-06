package shipmenthttp_test

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// Covers: ADR-0144 决定五、六——答复带 page（size 取 Intake 给的页大小、next 与 total 原样带出）；查询参数解成查询对象原样交给
// 读口，读口按它取页。
func TestLabelTransactionsCarryThePageAndPassTheDecodedQuery(t *testing.T) {
	reader := &labelTransactionsReaderDouble{
		records: []ports.LabelTransactionRecord{labelTransactionRecordFixture(t)},
		next:    "SYN-cursor",
		total:   7,
	}
	response := requestLabelTransactions(t, labelTransactionIntake(t), reader, http.MethodGet,
		"/label-transactions?transactionResult=FAILED&transactionResult=SUCCEEDED&q=SYN&sort=transactionId")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d；body = %s", response.Code, response.Body)
	}
	page := pageOf(t, response)
	if string(page["size"]) != "50" || string(page["next"]) != `"SYN-cursor"` || string(page["total"]) != "7" {
		t.Fatalf("page 不对：%s", response.Body.Bytes())
	}
	if reader.query.Sort != (cataloguepage.Sort{Field: "transactionId"}) || reader.query.Keyword != "SYN" ||
		!reflect.DeepEqual(reader.query.Filters["transactionResult"], []string{"FAILED", "SUCCEEDED"}) {
		t.Fatalf("读口收到的查询不对：%+v", reader.query)
	}
}

// Covers: ADR-0144 决定一、三、四——集外的键、排序维与筛选值，解不开的游标，都答 400 带理由散文，读口不被调用；查询参数属传输
// 形状、先于 Intake：渠道未配置时写错参数也先答 400，写对了才答 403。
func TestLabelTransactionsRefuseQueriesOutsideTheDeclaration(t *testing.T) {
	for name, target := range map[string]string{
		"未知键":    "/label-transactions?tenant=SYN-TENANT-02",
		"集外排序维":  "/label-transactions?sort=parcelId",
		"词表外筛选值": "/label-transactions?transactionResult=ARCHIVED",
		"解不开的游标": "/label-transactions?after=SYN-not-a-cursor",
	} {
		t.Run(name, func(t *testing.T) {
			reader := &labelTransactionsReaderDouble{}
			assertPageQueryRefused(t, requestLabelTransactions(t, labelTransactionIntake(t), reader, http.MethodGet, target))
			if reader.called {
				t.Fatal("坏请求不该走到读口")
			}
		})
	}

	refused := requestLabelTransactions(t, shipmenthttp.UnconfiguredIntake{}, &labelTransactionsReaderDouble{},
		http.MethodGet, "/label-transactions?sort=parcelId")
	if detail := assertPageQueryRefused(t, refused); !strings.Contains(detail, "parcelId") {
		t.Fatalf("理由散文应点到那一格：%q", detail)
	}
	unconfigured := requestLabelTransactions(t, shipmenthttp.UnconfiguredIntake{}, &labelTransactionsReaderDouble{},
		http.MethodGet, "/label-transactions?sort=-establishedAt")
	if unconfigured.Code != http.StatusForbidden {
		t.Fatalf("参数写对、渠道未配置 status = %d, want 403", unconfigured.Code)
	}
}
