package commercialhttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// ServiceProductCatalogueReader 是服务产品目录端点消费的读口。
type ServiceProductCatalogueReader interface {
	ListServiceProducts(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.ServiceProductCatalogueRow, error)
}

// 编译期锁缝:读口形状与端口保持一致——本适配器不新造查询语义,只消费 ADR-0077
// 钉住的那一个读面。
var _ ServiceProductCatalogueReader = ports.ServiceProductCatalogueRead(nil)

// outcomeServiceProductsListed 是本端点唯一的业务成格:空目录也是这一格
// (ADR-0077 Decision 四,空表本身就是内容,不折成未配置)。
const outcomeServiceProductsListed = "SERVICE_PRODUCTS_LISTED"

// NewQueryServiceProductsEndpoint 交回服务产品目录查阅的 HTTP 入口
// (GET /commercial-service-products,ADR-0077、票 master-data-wiring/05)。
func NewQueryServiceProductsEndpoint(
	intake CommercialCatalogueIntake,
	reader ServiceProductCatalogueReader,
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

		rows, err := reader.ListServiceProducts(request.Context(), query.Scope.Tenant(), query.Limit)
		if err != nil {
			// 读不回是答案未形成,不是「空目录」——伪装成后者会让一次该重试的故障变成
			// 一份看起来如实的空册。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		// 空目录交回空数组而不是 null:调用方判「没有行」不该先判「有没有字段」。
		bodies := make([]serviceProductBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, serviceProductBodyOf(row))
		}
		writeJSON(response, http.StatusOK, serviceProductListResponse{
			Outcome:  outcomeServiceProductsListed,
			Products: bodies,
		})
	})
}

type serviceProductListResponse struct {
	Outcome  string               `json:"outcome"`
	Products []serviceProductBody `json:"products"`
}

// serviceProductBody 逐字段透出版本壳与形态。form 缺席即形态未登记——那是合法
// 缺席(ADR-0050:产品缺席不使解析退化),不是缺陷,不得为目录齐整补一个宽泛值;
// effectiveEndsAt 缺席即开放结束。
type serviceProductBody struct {
	ObjectID          string `json:"objectId"`
	Version           string `json:"version"`
	Scope             string `json:"scope"`
	Status            string `json:"status"`
	EffectiveStartsAt string `json:"effectiveStartsAt"`
	EffectiveEndsAt   string `json:"effectiveEndsAt,omitempty"`
	PublishedAt       string `json:"publishedAt"`
	Form              string `json:"form,omitempty"`
}

func serviceProductBodyOf(row ports.ServiceProductCatalogueRow) serviceProductBody {
	body := serviceProductBody{
		ObjectID:          row.ObjectID,
		Version:           row.VersionLabel,
		Scope:             row.Scope,
		Status:            row.Status,
		EffectiveStartsAt: rfc3339(row.EffectiveStartsAt),
		PublishedAt:       rfc3339(row.PublishedAt),
	}
	if row.HasEffectiveEnd {
		body.EffectiveEndsAt = rfc3339(row.EffectiveEndsAt)
	}
	if row.HasForm {
		body.Form = row.Form
	}
	return body
}
