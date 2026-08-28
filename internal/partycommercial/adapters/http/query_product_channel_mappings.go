package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// ProductChannelCatalogueReader 是渠道产品目录端点消费的读口。
type ProductChannelCatalogueReader interface {
	ListProductChannelMappings(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.ProductChannelMappingRow, error)
}

// 编译期锁缝：读口形状与端口保持一致。
var _ ProductChannelCatalogueReader = ports.ProductChannelMappingCatalogueRead(nil)

// 本端点唯一的业务成格；空册也是这一格（ADR-0077 Decision 四）。
const outcomeProductChannelMappingsListed = "PRODUCT_CHANNEL_MAPPINGS_LISTED"

// NewQueryProductChannelMappingsEndpoint 交回渠道产品目录查阅的 HTTP 入口
// （GET /commercial-product-channel-mappings，ADR-0077，票
// admin-remainder-mechanism-batch/02）。
//
// 它不并进 /commercial-service-products：那边上列版本壳，这边上列映射登记信封——
// 行形状与修订轴不同，分立判据在 ports.ProductChannelMappingCatalogueRead 注释。
func NewQueryProductChannelMappingsEndpoint(
	intake CommercialCatalogueIntake,
	reader ProductChannelCatalogueReader,
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

		rows, err := reader.ListProductChannelMappings(request.Context(), query.Scope.Tenant(), query.Limit)
		if err != nil {
			// 读不回是答案未形成，不是「空目录」——伪装成后者会让一次该重试的故障
			// 变成一份看起来如实的空册。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		bodies := make([]productChannelMappingBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, productChannelMappingBodyOf(row))
		}
		writeJSON(response, http.StatusOK, productChannelMappingListResponse{
			Outcome:  outcomeProductChannelMappingsListed,
			Mappings: bodies,
		})
	})
}

type productChannelMappingListResponse struct {
	Outcome  string                      `json:"outcome"`
	Mappings []productChannelMappingBody `json:"mappings"`
}

// productChannelMappingBody 逐字段透出映射最新修订。channels 是渠道绑定格的引用
// 转写：空数组即显式登记的“未配置”绑定（页面据此如实显示），永不为 null——空与
// 缺席在这格必须不可混，绑定格是本册存在的理由。行上没有状态字段：映射没有独立
// 状态代数，是否参与新的渠道决策由消费方对区间判断（读口注释同一条裁决）。
type productChannelMappingBody struct {
	TenantID            string   `json:"tenantId"`
	MappingID           string   `json:"mappingId"`
	Revision            int      `json:"revision"`
	ProductObjectID     string   `json:"productObjectId"`
	ProductVersionLabel string   `json:"productVersionLabel"`
	Channels            []string `json:"channels"`
	Basis               string   `json:"basis"`
	EffectiveStartsAt   string   `json:"effectiveStartsAt"`
	EffectiveEndsAt     string   `json:"effectiveEndsAt,omitempty"`
	RegisteredAt        string   `json:"registeredAt"`
}

func productChannelMappingBodyOf(row ports.ProductChannelMappingRow) productChannelMappingBody {
	channels := row.Channels
	if channels == nil {
		channels = []string{}
	}
	body := productChannelMappingBody{
		TenantID:            row.TenantID,
		MappingID:           row.MappingID,
		Revision:            row.Revision,
		ProductObjectID:     row.ProductObjectID,
		ProductVersionLabel: row.ProductVersionLabel,
		Channels:            channels,
		Basis:               row.Basis,
		EffectiveStartsAt:   rfc3339(row.EffectiveStartsAt),
		RegisteredAt:        rfc3339(row.RegisteredAt),
	}
	if row.HasEffectiveEnd {
		body.EffectiveEndsAt = rfc3339(row.EffectiveEndsAt)
	}
	return body
}
