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

// LocationCatalogueReader 是各库位当前修订列表端点消费的读口。
type LocationCatalogueReader interface {
	ListLocations(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.LocationRow], error)
}

// 编译期锁缝：读口形状与查阅用例保持一致。
var _ LocationCatalogueReader = (*application.QueryLocationsHandler)(nil)

// 本端点唯一的业务成格；空册也是这一格（ADR-0077 决定四），不答未配置。
const outcomeLocationsListed = "LOCATIONS_LISTED"

type locationListResponse struct {
	Outcome   string               `json:"outcome"`
	Locations []locationBody       `json:"locations"`
	Page      cataloguepage.Page   `json:"page"`
	Facets    cataloguepage.Facets `json:"facets,omitempty"`
}

// NewQueryLocationsEndpoint 交回各库位当前修订的查阅入口（GET /warehouse-fulfillment-locations），规矩同各库区当前修订的列表口：查询
// 参数按 ports.LocationPages 解码（ADR-0144），在 Intake 之后；读不回是答案未形成，不是空册：空册答 200 与空数组，读不动答 500。
func NewQueryLocationsEndpoint(intake CatalogueIntake, reader LocationCatalogueReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}

		pageQuery, err := ports.LocationPages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		if pageQuery.PageSize > 0 {
			query.Limit = pageQuery.PageSize
		}

		page, err := reader.ListLocations(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "location list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeJSON(response, http.StatusOK, locationListResponse{
			Outcome:   outcomeLocationsListed,
			Locations: locationBodiesOfRows(page.Rows),
			Page:      page.Page(query.Limit),
			Facets:    page.Facets,
		})
	})
}
