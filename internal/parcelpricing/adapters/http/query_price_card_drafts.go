package pricinghttp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 价卡草稿查阅读口（ADR-0101 决定三与 Consequences；票 price-card-import/03）：按租户列出草稿册，可按状态筛。
// 草稿不是价卡版本、不进价卡目录，所以不并进 /pricing-price-cards；路径另立 `-draft-views` 是因为
// `/pricing-price-card-drafts` 已是录入口，端点表一个路径只挂一个方法（同 /shipment-requests 与
// /shipment-request-views）。

// PriceCardDraftQuery 是一次已授权的草稿查阅：租户来自认证结果，状态取自查询串（零值即四格都列），页大小由
// 接入面定——三样都不采信调用方自报。
type PriceCardDraftQuery struct {
	Tenant domain.TenantID
	Status domain.PriceCardDraftStatus
	Limit  int
}

// PriceCardDraftQueryIntake 把一次已认证的接入请求翻译成草稿查阅。
type PriceCardDraftQueryIntake interface {
	IntakePriceCardDraftQuery(ctx context.Context, request *http.Request) (PriceCardDraftQuery, error)
}

// PriceCardDraftReader 是本端点消费的草稿册读口。
type PriceCardDraftReader interface {
	ListPriceCardDrafts(ctx context.Context, tenant domain.TenantID, status domain.PriceCardDraftStatus, limit int) ([]domain.PriceCardDraft, error)
}

var _ PriceCardDraftReader = ports.PriceCardDraftRead(nil)

// outcomePriceCardDraftsListed 是本端点唯一的业务成格：空册也是这一格（ADR-0077 决定四）。
const outcomePriceCardDraftsListed = "PRICE_CARD_DRAFTS_LISTED"

// NewQueryPriceCardDraftsEndpoint 交回草稿查阅读口（GET /pricing-price-card-draft-views）。
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
		drafts, err := reader.ListPriceCardDrafts(request.Context(), query.Tenant, query.Status, query.Limit)
		if err != nil {
			// 读不回（含重建门拒了一行）是答案未形成，不是空册。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		bodies := make([]priceCardDraftBody, 0, len(drafts))
		for _, draft := range drafts {
			bodies = append(bodies, priceCardDraftBodyOf(draft))
		}
		writeJSON(response, http.StatusOK, priceCardDraftListResponse{Outcome: outcomePriceCardDraftsListed, Drafts: bodies})
	})
}

type priceCardDraftListResponse struct {
	Outcome string               `json:"outcome"`
	Drafts  []priceCardDraftBody `json:"drafts"`
}

// priceCardDraftPageSize 是操作者渠道这一口的页大小。操作者渠道归产品（ADR-0100），页大小是渠道契约的一格、
// 不是租户取值；一个租户在途的价卡草稿以「版」计，量级远在这个数之下，所以端口只要上限、不要翻页。
const priceCardDraftPageSize = 200

var _ PriceCardDraftQueryIntake = (*OperatorRegistryIntake)(nil)

// IntakePriceCardDraftQuery 是操作者渠道对草稿查阅的译法：先认证、后解查询串，租户取认证出的身份。授予照登记册
// 配置写那一格认，与录入口同一个——ADR-0101 Consequences 把草稿查阅读口与各命令口一并挂在操作者 Intake 上，
// 看草稿的就是导入与批准草稿的人；只持查阅授予的答未授予。
func (intake *OperatorRegistryIntake) IntakePriceCardDraftQuery(ctx context.Context, request *http.Request) (PriceCardDraftQuery, error) {
	operator, err := intake.authenticator.AuthenticateRegistryWrite(ctx, httpapi.BearerToken(request))
	if err != nil {
		return PriceCardDraftQuery{}, err
	}
	status, err := DecodePriceCardDraftQuery(request)
	if err != nil {
		return PriceCardDraftQuery{}, err
	}
	return PriceCardDraftQuery{Tenant: operator.Tenant, Status: status, Limit: priceCardDraftPageSize}, nil
}

// priceCardDraftStatusParameter 是查询串里唯一认的一格。
const priceCardDraftStatusParameter = "status"

// DecodePriceCardDraftQuery 封闭解查询串：只认 status 一格、至多一个值、取四格的名字原样；缺即零值，四格都列
// （端口约定）。别的键一律答坏请求而不是装没看见：租户只从信封来，按未知键拒同上传与 JSON 载荷；页大小由接入面
// 定，带一个 limit 来就是以为自己能定。错误一律包 ErrMalformedRequest。
func DecodePriceCardDraftQuery(request *http.Request) (domain.PriceCardDraftStatus, error) {
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return domain.PriceCardDraftStatusInvalid, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	for key := range values {
		if key != priceCardDraftStatusParameter {
			return domain.PriceCardDraftStatusInvalid, fmt.Errorf("%w: unknown query parameter %q", ErrMalformedRequest, key)
		}
	}
	names, present := values[priceCardDraftStatusParameter]
	if !present {
		return domain.PriceCardDraftStatusInvalid, nil
	}
	if len(names) != 1 {
		return domain.PriceCardDraftStatusInvalid, fmt.Errorf("%w: %q takes exactly one value", ErrMalformedRequest, priceCardDraftStatusParameter)
	}
	status, known := domain.ParsePriceCardDraftStatus(names[0])
	if !known {
		return domain.PriceCardDraftStatusInvalid, fmt.Errorf("%w: %q is not a draft status", ErrMalformedRequest, names[0])
	}
	return status, nil
}
