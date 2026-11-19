package warehousehttp

import (
	"context"
	"log/slog"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// ZoneRevisionsReader 是一个库区修订史端点消费的读口。
type ZoneRevisionsReader interface {
	ZoneRevisions(
		ctx context.Context,
		tenant domain.TenantID,
		warehouse domain.WarehouseCode,
		code domain.ZoneCode,
	) ([]ports.ZoneRow, error)
}

// 编译期锁缝：读口形状与查阅用例保持一致。
var _ ZoneRevisionsReader = (*application.QueryZonesHandler)(nil)

const (
	outcomeZoneRevisionsListed = "ZONE_REVISIONS_LISTED"
	// zoneCodeQueryKey 与 warehouseCodeQueryKey 是本端点唯二的查询键：库区的身份是（仓编码，库区编码）两段，缺一段就指不到一个库区。
	// 编码是业务内容，不放路径里，理由同仓的修订史口。
	zoneCodeQueryKey = "zoneCode"
)

// zoneRevisionsResponse 顶层回显仓编码与库区编码：页面切换行时上一问的答案可能后到，据此核对答的是不是它此刻问的那个库区，
// 不拿数组首笔去推（空数组没有首笔）。
type zoneRevisionsResponse struct {
	Outcome       string     `json:"outcome"`
	WarehouseCode string     `json:"warehouseCode"`
	ZoneCode      string     `json:"zoneCode"`
	Revisions     []zoneBody `json:"revisions"`
}

// NewQueryZoneRevisionsEndpoint 交回一个库区全部修订的查阅入口（GET /warehouse-fulfillment-zone-revisions?warehouseCode=…&zoneCode=…），
// 按修订号升序。这个库区在本租户查不到时答 200 与空数组，与别的租户的同编码库区同答：读口不泄露别的租户有没有这个库区
// （UC-WF-001「权限、隔离与审计」），也就不答 404。两个编码在 Intake 之后读：未配置要拒在一切请求内容之前（ADR-0055 决定三），
// 查询串也是请求内容。
func NewQueryZoneRevisionsEndpoint(intake CatalogueIntake, reader ZoneRevisionsReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodGet) {
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}

		warehouse, code, reason := zoneIdentityOf(request)
		if reason != "" {
			writeMalformed(response, reason)
			return
		}

		rows, err := reader.ZoneRevisions(request.Context(), query.Tenant, warehouse, code)
		if err != nil {
			// 读不回是答案未形成，不是「没有历史」：伪装成后者会让一次该重试的故障变成一段看起来如实的空历史。
			slog.ErrorContext(request.Context(), "zone revisions did not form an answer",
				"warehouseCode", warehouse.String(), "zoneCode", code.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeJSON(response, http.StatusOK, zoneRevisionsResponse{
			Outcome:       outcomeZoneRevisionsListed,
			WarehouseCode: warehouse.String(),
			ZoneCode:      code.String(),
			Revisions:     zoneBodiesOfRows(rows),
		})
	})
}

// zoneIdentityOf 从查询串取库区的身份：恰各一个 `warehouseCode` 与 `zoneCode`，没有别的键。规矩同 warehouseCodeOf。不成立时交回
// 一句理由（只谈查询串的形状，不回显取值），成立时理由为空。
func zoneIdentityOf(request *http.Request) (domain.WarehouseCode, domain.ZoneCode, string) {
	cells, reason := queryCells(request, warehouseCodeQueryKey, zoneCodeQueryKey)
	if reason != "" {
		return domain.WarehouseCode{}, domain.ZoneCode{}, reason
	}
	warehouse, err := domain.NewWarehouseCode(cells[0])
	if err != nil {
		return domain.WarehouseCode{}, domain.ZoneCode{}, blankCellReason(warehouseCodeQueryKey)
	}
	code, err := domain.NewZoneCode(cells[1])
	if err != nil {
		return domain.WarehouseCode{}, domain.ZoneCode{}, blankCellReason(zoneCodeQueryKey)
	}
	return warehouse, code, ""
}
