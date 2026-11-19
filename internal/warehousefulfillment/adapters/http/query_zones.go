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

// ZoneCatalogueReader 是各库区当前修订列表端点消费的读口。
type ZoneCatalogueReader interface {
	ListZones(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.ZoneRow], error)
}

// 编译期锁缝：读口形状与查阅用例保持一致。
var _ ZoneCatalogueReader = (*application.QueryZonesHandler)(nil)

// 本端点唯一的业务成格；空册也是这一格（ADR-0077 决定四），不答未配置。
const outcomeZonesListed = "ZONES_LISTED"

type zoneListResponse struct {
	Outcome string             `json:"outcome"`
	Zones   []zoneBody         `json:"zones"`
	Page    cataloguepage.Page `json:"page"`
}

// NewQueryZonesEndpoint 交回各库区当前修订的查阅入口（GET /warehouse-fulfillment-zones），规矩同各仓当前修订的列表口：查询参数按
// ports.ZonePages 解码（ADR-0144），在 Intake 之后；读不回是答案未形成，不是空册：空册答 200 与空数组，读不动答 500。
func NewQueryZonesEndpoint(intake CatalogueIntake, reader ZoneCatalogueReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}

		pageQuery, err := ports.ZonePages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}

		page, err := reader.ListZones(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "zone list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeJSON(response, http.StatusOK, zoneListResponse{
			Outcome: outcomeZonesListed,
			Zones:   zoneBodiesOfRows(page.Rows),
			Page:    page.Page(query.Limit),
		})
	})
}
