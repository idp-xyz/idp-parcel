package pricinghttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// SellPriceScopeReader 按范围名数此刻在用的售价卡。合同页用它标来源，不拿价卡目录去翻。
type SellPriceScopeReader interface {
	CountCardsInForceRegardlessOfProduct(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.PricingScopeID,
		direction domain.PricingDirection,
		purpose domain.PricingPurpose,
		at time.Time,
	) (int, error)
}

const (
	outcomeSellPriceScopeInForce       = "SELL_PRICE_SCOPE_IN_FORCE"
	outcomeSellPriceScopeNotConfigured = "SELL_PRICE_SCOPE_NOT_CONFIGURED"
)

// NewQuerySellPriceScopeEndpoint 交回「这个范围名此刻有几张在用的 SELL·客户费用卡」
// （GET /pricing-sell-price-scope?scope=）。方向与目的钉死：这一口只回答合同页那一问。
// 租户只来自操作者信封。查询串只认 scope。
func NewQuerySellPriceScopeEndpoint(
	intake PricingCatalogueIntake,
	reader SellPriceScopeReader,
	clock CoverageClock,
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
		values := request.URL.Query()
		if len(values) != 1 || len(values["scope"]) != 1 || values["scope"][0] == "" {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		scope, err := domain.NewPricingScopeID(values["scope"][0])
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}
		count, err := reader.CountCardsInForceRegardlessOfProduct(
			request.Context(),
			query.Scope.Tenant(),
			scope,
			domain.PricingDirectionSell,
			domain.PricingPurposeCustomerCharge,
			clock.Now(),
		)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		outcome := outcomeSellPriceScopeNotConfigured
		if count > 0 {
			outcome = outcomeSellPriceScopeInForce
		}
		writeJSON(response, http.StatusOK, sellPriceScopeResponse{Outcome: outcome, Scope: scope.String(), Cards: count})
	})
}

type sellPriceScopeResponse struct {
	Outcome string `json:"outcome"`
	Scope   string `json:"scope"`
	Cards   int    `json:"cards"`
}
