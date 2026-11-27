package warehousehttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

type stockReadIntakeStub struct{}

func (stockReadIntakeStub) IntakeCatalogueQuery(context.Context, *http.Request) (warehousehttp.CatalogueQuery, error) {
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	return warehousehttp.CatalogueQuery{Tenant: tenant, Limit: 2}, nil
}

type customerStockIntakeStub struct{ owner string }

func (customerStockIntakeStub) IntakeInboundPreAdviceSubmission(context.Context, *http.Request) (application.AcceptPreAdviceCommand, error) {
	return application.AcceptPreAdviceCommand{}, warehousehttp.ErrAccessChannelNotConfigured
}

func (customerStockIntakeStub) IntakeInboundPreAdviceArrivalNotice(context.Context, *http.Request) (application.UpdatePreAdviceNoticeCommand, error) {
	return application.UpdatePreAdviceNoticeCommand{}, warehousehttp.ErrAccessChannelNotConfigured
}

func (customerStockIntakeStub) IntakeInboundPreAdviceWithdrawal(context.Context, *http.Request) (application.WithdrawPreAdviceCommand, error) {
	return application.WithdrawPreAdviceCommand{}, warehousehttp.ErrAccessChannelNotConfigured
}

func (stub customerStockIntakeStub) IntakeWarehouseRead(context.Context, *http.Request) (warehousehttp.CustomerReadIdentity, error) {
	tenant, _ := domain.NewTenantID("SYN-TENANT-01")
	owner, _ := domain.NewOwnerAccountID(stub.owner)
	return warehousehttp.CustomerReadIdentity{Tenant: tenant, Owner: owner}, nil
}

// stockReaderStub 记下交给编排的查询；看流水时照查询编一个下一页的游标，像读面在还有下一页时那样。
type stockReaderStub struct {
	queries []application.StockQuery
}

func (stub *stockReaderStub) Query(_ context.Context, query application.StockQuery) (application.StockSnapshot, error) {
	stub.queries = append(stub.queries, query)
	if query.View != application.StockViewMovements {
		return application.StockSnapshot{}, nil
	}
	next, err := query.Page.CursorAfter(cataloguepage.Position{Value: "2026-11-20T06:00:00Z", Identity: []string{"SYN-MOVEMENT-02"}})
	if err != nil {
		return application.StockSnapshot{}, err
	}
	page := cataloguepage.NewPage(query.Limit, next, 5)
	return application.StockSnapshot{
		Movements:    []application.StockMovementView{{Warehouse: "SYN-WH-01", Reason: "RECEIPT"}},
		MovementPage: &page,
	}, nil
}

type stockReadAnswer struct {
	Outcome string `json:"outcome"`
	Page    *struct {
		Size  int     `json:"size"`
		Next  *string `json:"next"`
		Total *int64  `json:"total"`
	} `json:"page"`
	Error *struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	} `json:"error"`
	// 货主面的拒绝是首方问题详情，代码与理由在顶层。
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

func stockRead(t *testing.T, handler http.Handler, path string) (*httptest.ResponseRecorder, stockReadAnswer) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", "Bearer presented.token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var answer stockReadAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("%s 答复解不开：%v，正文 %s", path, err, response.Body.String())
	}
	return response, answer
}

// 流水那一种看法翻页：第一页带 page 一格，照它的 next 再问得到下一页；换了仓或货主，上一份列表的游标不成立。
func TestStockReadPagesMovementsOnly(t *testing.T) {
	reader := &stockReaderStub{}
	endpoint := warehousehttp.NewQueryStockEndpoint(stockReadIntakeStub{}, reader)
	base := "/warehouse-fulfillment-stock?ownerAccount=SYN-OWNER-01&warehouseCode=SYN-WH-01&view=movements"

	response, first := stockRead(t, endpoint, base)
	if response.Code != http.StatusOK || first.Page == nil || first.Page.Size != 2 || first.Page.Next == nil || first.Page.Total == nil || *first.Page.Total != 5 {
		t.Fatalf("第一页 = %d %s", response.Code, response.Body.String())
	}
	if len(reader.queries) != 1 || reader.queries[0].Limit != 2 || reader.queries[0].Page.After != nil || !reader.queries[0].Page.Sort.Descending {
		t.Fatalf("第一页交给编排的查询 %+v", reader.queries)
	}
	after := "&after=" + url.QueryEscape(*first.Page.Next)

	response, _ = stockRead(t, endpoint, base+after)
	if response.Code != http.StatusOK || len(reader.queries) != 2 || reader.queries[1].Page.After == nil ||
		reader.queries[1].Page.After.Identity[0] != "SYN-MOVEMENT-02" {
		t.Fatalf("第二页 = %d %s，查询 %+v", response.Code, response.Body.String(), reader.queries)
	}

	for name, path := range map[string]string{
		"换了仓":       "/warehouse-fulfillment-stock?ownerAccount=SYN-OWNER-01&warehouseCode=SYN-WH-02&view=movements" + after,
		"不点仓":       "/warehouse-fulfillment-stock?ownerAccount=SYN-OWNER-01&view=movements" + after,
		"换了货主":      "/warehouse-fulfillment-stock?ownerAccount=SYN-OWNER-02&warehouseCode=SYN-WH-01&view=movements" + after,
		"游标解不开":     base + "&after=not-a-cursor",
		"游标是空的":     base + "&after=",
		"库存格不翻页":    "/warehouse-fulfillment-stock?ownerAccount=SYN-OWNER-01&warehouseCode=SYN-WH-01&view=cells" + after,
		"缺省看法不翻页":   "/warehouse-fulfillment-stock?ownerAccount=SYN-OWNER-01&warehouseCode=SYN-WH-01" + after,
		"不收 sort":   base + "&sort=postedAt",
		"after 给两次": base + after + after,
	} {
		response, answer := stockRead(t, endpoint, path)
		if response.Code != http.StatusBadRequest || answer.Error == nil || answer.Error.Code != "MALFORMED_REQUEST" || answer.Error.Detail == "" {
			t.Fatalf("%s = %d %s", name, response.Code, response.Body.String())
		}
	}
	if len(reader.queries) != 2 {
		t.Fatalf("被拒的请求不该到编排，查询次数 = %d", len(reader.queries))
	}
	response, _ = stockRead(t, endpoint, base+"&q=RECEIPT")
	if response.Code != http.StatusOK || len(reader.queries) != 3 || reader.queries[2].Keyword != "RECEIPT" {
		t.Fatalf("带检索词 = %d %s，查询 %+v", response.Code, response.Body.String(), reader.queries)
	}

	// 不翻页的看法照旧答整份，没有 page 一格。
	response, cells := stockRead(t, endpoint, "/warehouse-fulfillment-stock?ownerAccount=SYN-OWNER-01&warehouseCode=SYN-WH-01&view=cells")
	if response.Code != http.StatusOK || cells.Page != nil || strings.Contains(response.Body.String(), `"page"`) {
		t.Fatalf("库存格 = %d %s", response.Code, response.Body.String())
	}
}

// 货主面的流水不翻页：照旧只答最早入账的一页，答复里没有 page，游标不交出去——游标解得开，里面是货主面不给的
// 流水标识与入账时刻。带 after 照旧是这一口不收的键。
func TestCustomerStockReadDoesNotHandOutACursor(t *testing.T) {
	reader := &stockReaderStub{}
	path := "/v1/warehouse/reads/stock?warehouseCode=SYN-WH-01&view=movements"
	endpoint := warehousehttp.NewQueryCustomerStockEndpoint(customerStockIntakeStub{owner: "SYN-OWNER-01"}, reader)

	response, answer := stockRead(t, endpoint, path)
	if response.Code != http.StatusOK || answer.Page != nil || strings.Contains(response.Body.String(), `"page"`) {
		t.Fatalf("货主流水 = %d %s", response.Code, response.Body.String())
	}
	if len(reader.queries) != 1 {
		t.Fatalf("查询次数 = %d", len(reader.queries))
	}
	asked := reader.queries[0]
	if !asked.Customer || asked.Limit < 1 || asked.Page.After != nil || asked.Page.Sort.Field != "postedAt" || asked.Page.Sort.Descending {
		t.Fatalf("交给编排的查询 %+v，排序 %+v", asked, asked.Page.Sort)
	}

	for name, extra := range map[string]string{
		"带 after": "&after=anything",
		"带 sort":  "&sort=-postedAt",
		"带货主":     "&ownerAccount=SYN-OWNER-02",
	} {
		response, answer := stockRead(t, endpoint, path+extra)
		if response.Code != http.StatusBadRequest || answer.Code != "MALFORMED_REQUEST" || answer.Detail == "" {
			t.Fatalf("%s = %d %s", name, response.Code, response.Body.String())
		}
	}
	if len(reader.queries) != 1 {
		t.Fatalf("被拒的请求不该到编排，查询次数 = %d", len(reader.queries))
	}
}
