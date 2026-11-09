package pricinghttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 价卡草稿查阅读口（ADR-0101 决定三与 Consequences；票 price-card-import/03）：按租户列出草稿册，可按状态筛。
// 草稿不是价卡版本、不进价卡目录，所以不并进 /pricing-price-cards；路径另立 `-draft-views` 是因为
// `/pricing-price-card-drafts` 已是录入口，端点表一个路径只挂一个方法（同 /shipment-requests 与
// /shipment-request-views）。

// PriceCardDraftQuery 是一次已授权的草稿查阅：租户来自认证结果，页大小由接入面定——两样都不采信调用方自报。筛哪几格
// 状态、怎么排、翻到哪一页是调用方的查询意图，不属授权，由端点按 ports.PriceCardDraftPages 解（ADR-0144 决定六）。
type PriceCardDraftQuery struct {
	Tenant domain.TenantID
	Limit  int
}

// PriceCardDraftQueryIntake 把一次已认证的接入请求翻译成草稿查阅。
type PriceCardDraftQueryIntake interface {
	IntakePriceCardDraftQuery(ctx context.Context, request *http.Request) (PriceCardDraftQuery, error)
}

// PriceCardDraftReader 是本端点消费的草稿册读口。
type PriceCardDraftReader interface {
	ListPriceCardDrafts(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
		query cataloguepage.Query,
	) (cataloguepage.Result[domain.PriceCardDraft], error)
}

var _ PriceCardDraftReader = ports.PriceCardDraftRead(nil)

// outcomePriceCardDraftsListed 是本端点唯一的业务成格：空册也是这一格（ADR-0077 决定四）。
const outcomePriceCardDraftsListed = "PRICE_CARD_DRAFTS_LISTED"

// NewQueryPriceCardDraftsEndpoint 交回草稿查阅读口（GET /pricing-price-card-draft-views）。
//
// 查询参数（after、sort、status 与 q）按 ports.PriceCardDraftPages 解码（ADR-0144 决定六），在 Intake 之后
// （渠道未配置时答复不随查询串变，ADR-0055 决定三），因而也在认证之后：没带令牌的一问不论查询串写得对不对都答 401。
func NewQueryPriceCardDraftsEndpoint(intake PriceCardDraftQueryIntake, reader PriceCardDraftReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		// 与录入口同一族 Intake、同一组答复格（未配置、凭据被拒、未授予、身份依赖不可用、请求形状不对），所以用命令面
		// 那份分流，不用目录查阅那份——后者没有操作者渠道的三格。
		query, err := intake.IntakePriceCardDraftQuery(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}
		pageQuery, err := ports.PriceCardDraftPages.DecodeRaw(request.URL.RawQuery)
		if err != nil {
			writePageQueryProblem(response, err)
			return
		}
		page, err := reader.ListPriceCardDrafts(request.Context(), query.Tenant, query.Limit, pageQuery)
		if err != nil {
			// 读不回（含重建门拒了一行）是答案未形成，不是空册。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		bodies := make([]priceCardDraftBody, 0, len(page.Rows))
		for _, draft := range page.Rows {
			bodies = append(bodies, priceCardDraftBodyOf(draft))
		}
		writeJSON(response, http.StatusOK, priceCardDraftListResponse{
			Outcome: outcomePriceCardDraftsListed,
			Drafts:  bodies,
			Page:    page.Page(query.Limit),
		})
	})
}

type priceCardDraftListResponse struct {
	Outcome string               `json:"outcome"`
	Drafts  []priceCardDraftBody `json:"drafts"`
	Page    cataloguepage.Page   `json:"page"`
}

// priceCardDraftPageSize 是操作者渠道这一口的页大小。操作者渠道归产品（ADR-0100），页大小是渠道契约的一格、
// 不是租户取值；一页装不下的由 page.next 往下翻（ADR-0144 决定一、二）。
const priceCardDraftPageSize = 200

var _ PriceCardDraftQueryIntake = (*OperatorRegistryIntake)(nil)

// IntakePriceCardDraftQuery 是操作者渠道对草稿查阅的译法：只认证，租户取认证出的身份，页大小取渠道契约。授予照登记册
// 配置写那一格认，与录入口同一个——ADR-0101 Consequences 把草稿查阅读口与各命令口一并挂在操作者 Intake 上，
// 看草稿的就是导入与批准草稿的人；只持查阅授予的答未授予。查询串不在这里解（ADR-0144 决定六），端点在这之后按声明解。
func (intake *OperatorRegistryIntake) IntakePriceCardDraftQuery(ctx context.Context, request *http.Request) (PriceCardDraftQuery, error) {
	operator, err := intake.authenticator.AuthenticateRegistryWrite(ctx, httpapi.BearerToken(request))
	if err != nil {
		return PriceCardDraftQuery{}, err
	}
	return PriceCardDraftQuery{Tenant: operator.Tenant, Limit: priceCardDraftPageSize}, nil
}
