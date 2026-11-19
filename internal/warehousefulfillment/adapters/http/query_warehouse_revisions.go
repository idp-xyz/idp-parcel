package warehousehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// WarehouseRevisionsReader 是一个仓修订史端点消费的读口。
type WarehouseRevisionsReader interface {
	WarehouseRevisions(ctx context.Context, tenant domain.TenantID, code domain.WarehouseCode) ([]ports.WarehouseRow, error)
}

// 编译期锁缝：读口形状与查阅用例保持一致。
var _ WarehouseRevisionsReader = (*application.QueryWarehousesHandler)(nil)

const (
	outcomeWarehouseRevisionsListed = "WAREHOUSE_REVISIONS_LISTED"
	// warehouseCodeQueryKey 是本端点唯一的查询键：仓编码是业务内容，不放路径里——逐字保存的编码可以带斜杠与空白，路径段
	// 的转义与路由匹配都不替它担保。
	warehouseCodeQueryKey = "warehouseCode"
)

// warehouseRevisionsResponse 顶层回显仓编码：页面切换行时上一问的答案可能后到，据此核对答的是不是它此刻问的那个仓，不拿
// 数组首笔去推（空数组没有首笔）。
type warehouseRevisionsResponse struct {
	Outcome       string          `json:"outcome"`
	WarehouseCode string          `json:"warehouseCode"`
	Revisions     []warehouseBody `json:"revisions"`
}

// NewQueryWarehouseRevisionsEndpoint 交回一个仓全部修订的查阅入口（GET /warehouse-fulfillment-warehouse-revisions?warehouseCode=…），
// 按修订号升序。这个仓在本租户查不到时答 200 与空数组，与别的租户的同编码仓同答：读口不泄露别的租户有没有这个仓（UC-WF-001
// 「权限、隔离与审计」），也就不答 404。
//
// 仓编码在 Intake 之后读：未配置要拒在一切请求内容之前（ADR-0055 决定三），查询串也是请求内容。
func NewQueryWarehouseRevisionsEndpoint(intake CatalogueIntake, reader WarehouseRevisionsReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}

		code, reason := warehouseCodeOf(request)
		if reason != "" {
			writeMalformed(response, reason)
			return
		}

		rows, err := reader.WarehouseRevisions(request.Context(), query.Tenant, code)
		if err != nil {
			// 读不回是答案未形成，不是「没有历史」：伪装成后者会让一次该重试的故障变成一段看起来如实的空历史。
			slog.ErrorContext(request.Context(), "warehouse revisions did not form an answer", "warehouseCode", code.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeJSON(response, http.StatusOK, warehouseRevisionsResponse{
			Outcome:       outcomeWarehouseRevisionsListed,
			WarehouseCode: code.String(),
			Revisions:     warehouseBodiesOfRows(rows),
		})
	})
}

// warehouseCodeOf 从查询串取仓编码：恰一个 `warehouseCode`，没有别的键。按 ParseQuery 解，解不开的一段整条答 400，不静默丢掉
// 后照常作答——一个写坏的编码被丢下就成了没给。编码逐字交给领域，不去空白。不成立时交回一句理由（只谈查询串的形状，不回显取值），
// 成立时理由为空。
func warehouseCodeOf(request *http.Request) (domain.WarehouseCode, string) {
	values, err := cataloguepage.ParseQuery(request.URL.RawQuery)
	if err != nil {
		var malformed *cataloguepage.MalformedQuery
		if errors.As(err, &malformed) {
			return domain.WarehouseCode{}, malformed.Reason
		}
		return domain.WarehouseCode{}, "the query string could not be read"
	}
	for key := range values {
		if key != warehouseCodeQueryKey {
			return domain.WarehouseCode{}, "only " + warehouseCodeQueryKey + " is accepted as a query key"
		}
	}
	given := values[warehouseCodeQueryKey]
	if len(given) != 1 {
		return domain.WarehouseCode{}, warehouseCodeQueryKey + " must be given exactly once"
	}
	code, err := domain.NewWarehouseCode(given[0])
	if err != nil {
		return domain.WarehouseCode{}, warehouseCodeQueryKey + " must be a non-blank value that can be stored verbatim"
	}
	return code, ""
}
