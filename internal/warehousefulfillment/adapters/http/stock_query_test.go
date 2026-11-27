package warehousehttp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

func TestOperatorStockQueryTakesAGoodsWord(t *testing.T) {
	tenant := stockTenant(t)
	cells := stockQueryRequest(t, url.Values{
		"ownerAccount": {"SYN-OWNER-01"}, "view": {"cells"}, "q": {"  合成  "}, "page": {"2"}, "pageSize": {"20"},
	})
	query, reason := operatorStockQuery(cells, tenant, 200)
	if reason != "" || query.Keyword != "合成" || query.Number != 2 || query.Limit != 20 {
		t.Fatalf("%s %+v", reason, query)
	}
	movements := stockQueryRequest(t, url.Values{
		"ownerAccount": {"SYN-OWNER-01"}, "view": {"movements"}, "q": {"SYN-GOODS-01"}, "pageSize": {"20"},
	})
	listed, reason := operatorStockQuery(movements, tenant, 200)
	if reason != "" || listed.Keyword != "SYN-GOODS-01" || listed.Page.Keyword != "SYN-GOODS-01" {
		t.Fatalf("流水 %s %q %q", reason, listed.Keyword, listed.Page.Keyword)
	}
	narrowed := stockQueryRequest(t, url.Values{"ownerAccount": {"SYN-OWNER-01"}, "view": {"cells"}, "stockState": {"GOOD"}, "page": {"1"}, "pageSize": {"50"}})
	stateQuery, reason := operatorStockQuery(narrowed, tenant, 200)
	if reason != "" || stateQuery.State != "GOOD" {
		t.Fatalf("状态 %s %q", reason, stateQuery.State)
	}
	blank := stockQueryRequest(t, url.Values{"ownerAccount": {"SYN-OWNER-01"}, "view": {"cells"}, "q": {"  "}})
	quiet, reason := operatorStockQuery(blank, tenant, 200)
	if reason != "" || quiet.Keyword != "" {
		t.Fatalf("空白 %s %q", reason, quiet.Keyword)
	}
}

func TestOperatorStockQueryRejectsAWordTheLedgerCannotStore(t *testing.T) {
	tenant := stockTenant(t)
	long := stockQueryRequest(t, url.Values{"ownerAccount": {"SYN-OWNER-01"}, "view": {"cells"}, "q": {strings.Repeat("货", 101)}})
	if _, reason := operatorStockQuery(long, tenant, 200); reason == "" {
		t.Fatal("超长的检索词不该收下")
	}
	sorted := stockQueryRequest(t, url.Values{"ownerAccount": {"SYN-OWNER-01"}, "view": {"movements"}, "sort": {"postedAt"}})
	if _, reason := operatorStockQuery(sorted, tenant, 200); reason == "" {
		t.Fatal("sort 不该收下")
	}
	customer := httptest.NewRequest(http.MethodGet, "/v1/warehouse/reads/stock?view=cells&q=SYN", nil)
	if _, reason := customerStockQuery(customer, tenant, stockOwner(t)); reason == "" {
		t.Fatal("货主面不该收检索词")
	}
}

func stockQueryRequest(t *testing.T, values url.Values) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodGet, "/warehouse-fulfillment-stock?"+values.Encode(), nil)
}

func stockTenant(t *testing.T) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	return tenant
}

func stockOwner(t *testing.T) domain.OwnerAccountID {
	t.Helper()
	owner, err := domain.NewOwnerAccountID("SYN-OWNER-01")
	if err != nil {
		t.Fatal(err)
	}
	return owner
}
