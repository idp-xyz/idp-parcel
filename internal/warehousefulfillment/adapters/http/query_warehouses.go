package warehousehttp

import (
	"context"
	"log/slog"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// WarehouseCatalogueReader 是各仓当前修订列表端点消费的读口。
type WarehouseCatalogueReader interface {
	ListWarehouses(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.WarehouseRow], error)
}

// 编译期锁缝：读口形状与查阅用例保持一致。
var _ WarehouseCatalogueReader = (*application.QueryWarehousesHandler)(nil)

// 本端点唯一的业务成格；空册也是这一格（ADR-0077 决定四），不答未配置。
const outcomeWarehousesListed = "WAREHOUSES_LISTED"

type warehouseListResponse struct {
	Outcome    string               `json:"outcome"`
	Warehouses []warehouseBody      `json:"warehouses"`
	Page       cataloguepage.Page   `json:"page"`
	Facets     cataloguepage.Facets `json:"facets,omitempty"`
}

// NewQueryWarehousesEndpoint 交回各仓当前修订的查阅入口（GET /warehouse-fulfillment-warehouses）。查询参数按
// ports.WarehousePages 解码（ADR-0144），在 Intake 之后：渠道未配置时对一切请求同答 403，没有凭据的调用方看不到查询校验的
// 结果。读不回是答案未形成，不是空册：空册答 200 与空数组，读不动答 500。
func NewQueryWarehousesEndpoint(intake CatalogueIntake, reader WarehouseCatalogueReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}

		pageQuery, err := ports.WarehousePages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		if pageQuery.PageSize > 0 {
			query.Limit = pageQuery.PageSize
		}

		page, err := reader.ListWarehouses(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "warehouse list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeJSON(response, http.StatusOK, warehouseListResponse{
			Outcome:    outcomeWarehousesListed,
			Warehouses: warehouseBodiesOfRows(page.Rows),
			Page:       page.Page(query.Limit),
			Facets:     page.Facets,
		})
	})
}

type customerWarehouseListResponse struct {
	Outcome    string                  `json:"outcome"`
	Warehouses []customerWarehouseBody `json:"warehouses"`
	Page       cataloguepage.Page      `json:"page"`
}

// NewListCustomerWarehousesEndpoint 交回货主列出本租户各仓当前修订的入口（GET /v1/warehouse/reads/warehouses）。
// 读的是运营列表同一只仓册，不另做一套。租户取自信封。查询串自带租户、货主或登记责任方是坏查询。空册答空数组。
func NewListCustomerWarehousesEndpoint(intake CustomerInboundPreAdvicePorts, reader WarehouseCatalogueReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseCustomerMethod(response, request, http.MethodGet) {
			return
		}
		identity, err := intake.IntakeWarehouseRead(request.Context(), request)
		if err != nil {
			answerCustomerIntake(response, request, err)
			return
		}
		if reason := refuseCarriedQueryIdentity(request); reason != "" {
			writeCustomerProblem(response, request, http.StatusBadRequest, codeMalformedRequest, reason)
			return
		}
		values, err := cataloguepage.ParseQuery(request.URL.RawQuery)
		if err != nil {
			writeCustomerQueryProblem(response, request, err)
			return
		}
		pageQuery, err := ports.WarehousePages.Decode(values)
		if err != nil {
			writeCustomerQueryProblem(response, request, err)
			return
		}
		limit := operatorCatalogueReadLimit
		if pageQuery.PageSize > 0 {
			limit = pageQuery.PageSize
		}
		page, err := reader.ListWarehouses(request.Context(), identity.Tenant, limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "customer warehouse list did not form an answer")
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		writeJSON(response, http.StatusOK, customerWarehouseListResponse{
			Outcome:    outcomeWarehousesListed,
			Warehouses: customerWarehouseBodiesOfRows(page.Rows),
			Page:       page.Page(limit),
		})
	})
}
