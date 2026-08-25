package pricinghttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// PriceCardCatalogueReader 是价卡目录端点消费的读口。
type PriceCardCatalogueReader interface {
	ListPriceCards(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ports.PriceCardCatalogueRow, error)
}

// 编译期锁缝:读口形状与端口保持一致——本适配器不新造查询语义,只消费 ADR-0077
// 钉住的那一个读面。
var _ PriceCardCatalogueReader = ports.PriceCardCatalogueRead(nil)

// outcomePriceCardsListed 是本端点唯一的业务成格:空目录也是这一格(ADR-0077
// Decision 四,空册本身就是内容,不折成未配置)。
const outcomePriceCardsListed = "PRICE_CARDS_LISTED"

// NewQueryPriceCardsEndpoint 交回价卡目录查阅的 HTTP 入口
// (GET /pricing-price-cards,ADR-0077、票 master-data-wiring/02;最终路径归装配票)。
func NewQueryPriceCardsEndpoint(
	intake PricingCatalogueIntake,
	reader PriceCardCatalogueReader,
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

		rows, err := reader.ListPriceCards(request.Context(), query.Scope.Tenant(), query.Limit)
		if err != nil {
			// 读不回是答案未形成,不是「空目录」——伪装成后者会让一次该重试的故障
			// 变成一份看起来如实的空册。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}

		// 空目录交回空数组而不是 null:调用方判「没有行」不该先判「有没有字段」。
		bodies := make([]priceCardBody, 0, len(rows))
		for _, row := range rows {
			bodies = append(bodies, priceCardBodyOf(row))
		}
		writeJSON(response, http.StatusOK, priceCardListResponse{
			Outcome: outcomePriceCardsListed,
			Cards:   bodies,
		})
	})
}

type priceCardListResponse struct {
	Outcome string          `json:"outcome"`
	Cards   []priceCardBody `json:"cards"`
}

// priceCardBody 逐字段透出检索列面。effectiveTo 缺席即无上界适用期;源文件名与
// SHA-256 是证据索引(真实价卡文件外置于受限证据库,ADR-0008),照登转写。
type priceCardBody struct {
	PlanID               string `json:"planId"`
	PlanVersion          string `json:"planVersion"`
	Direction            string `json:"direction"`
	Purpose              string `json:"purpose"`
	Scope                string `json:"scope"`
	RateTableID          string `json:"rateTableId"`
	RateTableVersion     string `json:"rateTableVersion"`
	EffectiveFrom        string `json:"effectiveFrom"`
	EffectiveTo          string `json:"effectiveTo,omitempty"`
	Canonicalization     string `json:"canonicalization"`
	ContentDigest        string `json:"contentDigest"`
	SourceFileName       string `json:"sourceFileName"`
	SourceFileSHA256     string `json:"sourceFileSha256"`
	AuthorizationID      string `json:"authorizationId"`
	AuthorizationVersion string `json:"authorizationVersion"`
	PublicationApprover  string `json:"publicationApprover"`
	RegisteredAt         string `json:"registeredAt"`
}

func priceCardBodyOf(row ports.PriceCardCatalogueRow) priceCardBody {
	body := priceCardBody{
		PlanID:               row.PlanID,
		PlanVersion:          row.PlanVersion,
		Direction:            row.Direction,
		Purpose:              row.Purpose,
		Scope:                row.Scope,
		RateTableID:          row.RateTableID,
		RateTableVersion:     row.RateTableVersion,
		EffectiveFrom:        rfc3339(row.EffectiveFrom),
		Canonicalization:     row.Canonicalization,
		ContentDigest:        row.ContentDigest,
		SourceFileName:       row.SourceFileName,
		SourceFileSHA256:     row.SourceFileSHA256,
		AuthorizationID:      row.AuthorizationID,
		AuthorizationVersion: row.AuthorizationVersion,
		PublicationApprover:  row.PublicationApprover,
		RegisteredAt:         rfc3339(row.RegisteredAt),
	}
	if row.HasEffectiveTo {
		body.EffectiveTo = rfc3339(row.EffectiveTo)
	}
	return body
}
