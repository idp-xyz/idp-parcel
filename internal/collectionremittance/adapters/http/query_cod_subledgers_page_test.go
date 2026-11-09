package collectionhttp_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	collectionhttp "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// Covers: ADR-0144 决定五、六——答复带 page（size 取 Intake 给的页大小、next 与 total 原样带出，末页 next 为 null）；查询参数
// 解成查询对象原样交给读口，读口按它取页。
func TestCodSubledgersCarryThePageAndPassTheDecodedQuery(t *testing.T) {
	register := &stubCodSubledgers{
		entries: []ports.CodSubledgerCatalogueRow{{
			Customer: "SYN-CUST-01", LegalEntity: "SYN-LE-01", Currency: "SYN-CUR-01", Channel: "SYN-CHAN-01",
			CustodyBasis: "SYN-CUSTODY-01", OpenedAt: catalogueBaseAt,
		}},
		next:  "SYN-cursor",
		total: 7,
	}
	endpoint := collectionhttp.NewQueryCodSubledgersEndpoint(
		grantedCatalogueIntake{tenant: "SYN-TENANT-01", limit: 50}, register,
	)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, subledgerRequest(t, "?q=%20SYN-CHAN%20&sort=-customer"))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d；body = %s", response.Code, response.Body)
	}
	page := pageOf(t, response)
	if string(page["size"]) != "50" || string(page["next"]) != `"SYN-cursor"` || string(page["total"]) != "7" {
		t.Fatalf("page 不对：%s", response.Body.Bytes())
	}
	if register.gotQuery.Sort != (cataloguepage.Sort{Field: "customer", Descending: true}) ||
		register.gotQuery.Keyword != "SYN-CHAN" || register.gotQuery.After != nil {
		t.Fatalf("读口收到的查询不对：%+v", register.gotQuery)
	}

	last := &stubCodSubledgers{total: 1, entries: register.entries}
	response = httptest.NewRecorder()
	collectionhttp.NewQueryCodSubledgersEndpoint(grantedCatalogueIntake{tenant: "SYN-TENANT-01", limit: 50}, last).
		ServeHTTP(response, subledgerRequest(t, ""))
	if page := pageOf(t, response); string(page["next"]) != "null" || string(page["total"]) != "1" {
		t.Fatalf("末页的 page 不对：%s", response.Body.Bytes())
	}
	if last.gotQuery.Sort != (cataloguepage.Sort{Field: "customer"}) {
		t.Fatalf("不带 sort 时应是缺省序 customer，得 %+v", last.gotQuery.Sort)
	}
}

// Covers: ADR-0144 决定一、三、四——集外的键、排序维，解不开的游标与换了条件还拿来的旧游标，都答 400 带理由散文，读口不被
// 调用。本册不放筛选维，拿答复上的字段当筛选维问就是集外键。解码在 Intake 之后：渠道未配置时参数写对写错都答 403，答复不随
// 请求内容变（ADR-0055 决定三）。
func TestCodSubledgersRefuseQueriesOutsideTheDeclaration(t *testing.T) {
	keyed, err := ports.CodSubledgerPages.Decode(url.Values{"q": {"SYN-A"}})
	if err != nil {
		t.Fatal(err)
	}
	stale, err := keyed.CursorAfter(cataloguepage.Position{
		Value:    "SYN-CUST-01",
		Identity: []string{"SYN-CUST-01", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01"},
	})
	if err != nil {
		t.Fatal(err)
	}

	refusals := map[string]struct {
		query  string
		points string
	}{
		"未知键":      {"?tenant=SYN-TENANT-02", "tenant"},
		"筛选维不在声明里": {"?currency=SYN-CUR-01", "currency"},
		"集外排序维":    {"?sort=payableToCustomer", "payableToCustomer"},
		"解不开的游标":   {"?after=SYN-not-a-cursor", "游标"},
		"坏转义":      {"?q=%zz", "解不开的一段"},
		"换了条件的旧游标": {"?q=SYN-B&after=" + stale, "游标"},
	}
	for name, refusal := range refusals {
		t.Run(name, func(t *testing.T) {
			endpoint := collectionhttp.NewQueryCodSubledgersEndpoint(
				grantedCatalogueIntake{tenant: "SYN-TENANT-01", limit: 50}, unreachableCodSubledgers{t: t},
			)
			response := httptest.NewRecorder()
			endpoint.ServeHTTP(response, subledgerRequest(t, refusal.query))
			if detail := assertPageQueryRefused(t, response); !strings.Contains(detail, refusal.points) {
				t.Fatalf("理由散文应点到 %s：%q", refusal.points, detail)
			}
			assertNoOutcome(t, response)
		})
	}

	// 同一个旧游标在原条件下照常解得开：上面那一格拒的是换了条件，不是游标本身。
	if _, err := ports.CodSubledgerPages.Decode(url.Values{"q": {"SYN-A"}, "after": {stale}}); err != nil {
		t.Fatalf("原条件下的游标解不开：%v", err)
	}

	baseline := httptest.NewRecorder()
	unconfigured := collectionhttp.NewQueryCodSubledgersEndpoint(collectionhttp.UnconfiguredIntake{}, unreachableCodSubledgers{t: t})
	unconfigured.ServeHTTP(baseline, subledgerRequest(t, ""))
	for _, query := range []string{"?sort=payableToCustomer", "?sort=-customer&q=SYN", "?after=SYN-not-a-cursor", "?tenant=SYN-TENANT-02"} {
		response := httptest.NewRecorder()
		unconfigured.ServeHTTP(response, subledgerRequest(t, query))
		if response.Code != http.StatusForbidden || response.Body.String() != baseline.Body.String() {
			t.Fatalf("%s 渠道未配置：%d %s，want 与不带参数同答 403 %s", query, response.Code, response.Body, baseline.Body)
		}
	}
	if baseline.Code != http.StatusForbidden || problemCode(t, baseline) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("渠道未配置：%d %s", baseline.Code, baseline.Body)
	}
}
