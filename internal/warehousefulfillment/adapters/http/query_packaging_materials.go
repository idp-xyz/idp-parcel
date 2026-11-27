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

// PackagingMaterialCatalogueReader 是各包材当前修订列表端点消费的读口。
type PackagingMaterialCatalogueReader interface {
	ListPackagingMaterials(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[ports.PackagingMaterialRow], error)
}

// 编译期锁缝：读口形状与查阅用例保持一致。
var _ PackagingMaterialCatalogueReader = (*application.QueryPackagingMaterialsHandler)(nil)

// 本端点唯一的业务成格；空册也是这一格（ADR-0077 决定四），不答未配置。
const outcomePackagingMaterialsListed = "PACKAGING_MATERIALS_LISTED"

type packagingMaterialListResponse struct {
	Outcome            string                  `json:"outcome"`
	PackagingMaterials []packagingMaterialBody `json:"packagingMaterials"`
	Page               cataloguepage.Page      `json:"page"`
}

// NewQueryPackagingMaterialsEndpoint 交回各包材当前修订的查阅入口（GET /warehouse-fulfillment-packaging-materials），规矩同各仓当前修订的列表口：
// 查询参数按 ports.PackagingMaterialPages 解码（ADR-0144），在 Intake 之后；读不回是答案未形成，不是空册：空册答 200 与空数组，读不动答 500。
func NewQueryPackagingMaterialsEndpoint(intake CatalogueIntake, reader PackagingMaterialCatalogueReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}

		pageQuery, err := ports.PackagingMaterialPages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		if pageQuery.PageSize > 0 {
			query.Limit = pageQuery.PageSize
		}

		page, err := reader.ListPackagingMaterials(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			slog.ErrorContext(request.Context(), "packaging material list did not form an answer")
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeJSON(response, http.StatusOK, packagingMaterialListResponse{
			Outcome:            outcomePackagingMaterialsListed,
			PackagingMaterials: packagingMaterialBodiesOfRows(page.Rows),
			Page:               page.Page(query.Limit),
		})
	})
}
