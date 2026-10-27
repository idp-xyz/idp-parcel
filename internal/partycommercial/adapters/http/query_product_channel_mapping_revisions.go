package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// ProductChannelMappingRevisionHistoryReader 是产品—渠道映射修订历史端点消费的读口。它单独成接口而不并进
// ProductChannelCatalogueReader 的方法表，判据同 BusinessPartyRevisionHistoryReader：端点只声明自己用到的那一个方法，
// 装配点交进来的仍是同一只商业目录适配器。
type ProductChannelMappingRevisionHistoryReader interface {
	ListProductChannelMappingRevisions(
		ctx context.Context,
		tenant domain.TenantID,
		mapping domain.ProductChannelMappingID,
	) ([]ports.ProductChannelMappingRow, error)
}

// 编译期锁缝：读口形状与端口保持一致。
var _ ProductChannelMappingRevisionHistoryReader = ports.ProductChannelMappingRevisionHistoryRead(nil)

// 本端点唯一的业务成格；不在册的映射也是这一格（空数组），理由在构造函数注释。
const outcomeProductChannelMappingRevisionsListed = "PRODUCT_CHANNEL_MAPPING_REVISIONS_LISTED"

// mappingIDPathValue 是路由模式里的路径参数名；装配点的 Pattern 与这里必须同字，否则 PathValue 恒空、端点恒答 400。
const mappingIDPathValue = "mappingId"

// NewQueryProductChannelMappingRevisionsEndpoint 交回产品—渠道映射修订历史查阅的 HTTP 入口
// （GET /commercial-product-channel-mappings/{mappingId}/revisions）。
//
// 它是渠道产品目录页详情抽屉「历史」栏的供数面，把 NewQueryBusinessPartyRevisionsEndpoint 的形状搬到映射册：目录行是每个映射的
// 最新修订（ADR-0077），映射按修订推进、翻旧插新而不覆盖，这一口把一个映射的整条修订链交出来，让绑定从哪几个渠道换到哪几个、
// 区间改过几次、每次依据是什么在页面上看得见。不做 diff，两笔之间改了什么由前端并排显，这里只交事实。
//
// 映射标识从路径取而不从查询串取、**不在册答 200 + 空数组不答 404**、Intake 与其余目录查阅口同一个——三条裁决与理由与参与方那一口
// 一字不改（跨租户与不在册在列表形状上结构同形，不泄露存在性），此处不复述。
func NewQueryProductChannelMappingRevisionsEndpoint(
	intake CommercialCatalogueIntake,
	reader ProductChannelMappingRevisionHistoryReader,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		query, err := intake.IntakeCatalogueQuery(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}

		// 路径参数在 Intake 之后读：未配置要拒在一切请求内容之前（ADR-0055），路径也是请求内容。
		mapping, err := domain.NewProductChannelMappingID(request.PathValue(mappingIDPathValue))
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}

		rows, err := reader.ListProductChannelMappingRevisions(request.Context(), query.Scope.Tenant(), mapping)
		if err != nil {
			// 读不回是答案未形成，不是「没有历史」。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]productChannelMappingBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, productChannelMappingBodyOf(row))
		}
		writeJSON(response, http.StatusOK, productChannelMappingRevisionListResponse{
			Outcome:   outcomeProductChannelMappingRevisionsListed,
			MappingID: mapping.String(),
			Revisions: bodies,
		})
	})
}

// productChannelMappingRevisionListResponse 顶层回显映射标识：抽屉切换行时上一问的答案可能后到，页面据此核对答的是不是它此刻
// 问的那个映射，不拿数组首笔的 mappingId 去推（空数组没有首笔）。每一笔与目录行同一个 productChannelMappingBody。
type productChannelMappingRevisionListResponse struct {
	Outcome   string                      `json:"outcome"`
	MappingID string                      `json:"mappingId"`
	Revisions []productChannelMappingBody `json:"revisions"`
}
