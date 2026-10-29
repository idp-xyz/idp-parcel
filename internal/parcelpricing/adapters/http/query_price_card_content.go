package pricinghttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// PriceCardContentReader 是价卡内容查阅端点消费的读口。
type PriceCardContentReader interface {
	ReadPriceCardContent(
		ctx context.Context,
		tenant domain.TenantID,
		reference domain.VersionReference,
	) (ports.PriceCardContent, bool, error)
}

// 编译期锁缝：本端点不新造读取语义，只消费存储读面那一个读口。
var _ PriceCardContentReader = ports.PriceCardContentRead(nil)

// 本端点的业务成格（ADR-0022：状态码只说服务端有没有形成答案，业务判别在 outcome）。
const (
	// outcomePriceCardContentFound：册上有这一版，document 是它登记的规范化文档。
	outcomePriceCardContentFound = "PRICE_CARD_CONTENT_FOUND"
	// outcomePriceCardContentNotFound：册上没有这一版——含别的租户的卡，越权探针与真不存在同答（ADR-0029）。
	outcomePriceCardContentNotFound = "NOT_FOUND"
	// outcomePriceCardContentUnsupported：册上有这一版，但它按另一套规范化版本折装、本构建重建不了。与「没有」续办不同
	// （后者去登记，前者只能等支持该形状的构建，ADR-0014），所以单列一格而不折成查无。
	outcomePriceCardContentUnsupported = "CANONICALIZATION_UNSUPPORTED"
)

// 查询串里认的两格。租户从不来自请求：认证交回的信封是租户的唯一出处。
const (
	priceCardContentIDParameter      = "id"
	priceCardContentVersionParameter = "version"
)

// NewQueryPriceCardContentEndpoint 交回价卡内容查阅的 HTTP 入口
// （GET /pricing-price-cards/content?id=<定价方案标识>&version=<版本>）。
//
// 它答「这一版价卡登记了什么规则」，与目录那一口（/pricing-price-cards，答「登了哪些版本」）互补：目录上列检索列面、
// 不读快照，运营要看一张卡有哪些费用项、怎么收，只能靠试算一个个试；这一口把登记的规范化文档原样交出，翻成人话在
// 呈现面做，服务端不替某一个页面定形状。
//
// 先认证、后解查询串：令牌不过的请求不泄露本端点认哪些键。查询串封闭——只认 id 与 version，各恰一个值；别的键
// 一律答坏请求而不是装没看见，带一个 tenant 来就是以为自己能指定租户（同草稿查阅口的做法）。
func NewQueryPriceCardContentEndpoint(
	intake PricingCatalogueIntake,
	reader PriceCardContentReader,
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
		reference, err := decodePriceCardContentQuery(request)
		if err != nil {
			writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
			return
		}

		content, found, err := reader.ReadPriceCardContent(request.Context(), query.Scope.Tenant(), reference)
		switch {
		case errors.Is(err, domain.ErrCanonicalizationVersionUnsupported):
			writeJSON(response, http.StatusOK, priceCardContentMissingResponse{
				Outcome:     outcomePriceCardContentUnsupported,
				PlanID:      reference.ID(),
				PlanVersion: reference.Version(),
			})
		case err != nil:
			// 读不回（含重建门拒了这一行）是答案未形成，不是「查无」：后者会让一次该重试的故障长得像「没登过这张卡」。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		case !found:
			writeJSON(response, http.StatusOK, priceCardContentMissingResponse{
				Outcome:     outcomePriceCardContentNotFound,
				PlanID:      reference.ID(),
				PlanVersion: reference.Version(),
			})
		default:
			writeJSON(response, http.StatusOK, priceCardContentResponse{
				Outcome:             outcomePriceCardContentFound,
				PlanID:              content.PlanID,
				PlanVersion:         content.PlanVersion,
				Canonicalization:    content.Canonicalization,
				ContentDigest:       content.ContentDigest,
				PublicationApprover: content.PublicationApprover,
				RegisteredAt:        rfc3339(content.RegisteredAt),
				Document:            content.Document,
			})
		}
	})
}

// priceCardContentResponse 是查到时的答复。document 是登记的规范化文档原样（json.RawMessage 不重排、不改写），
// 其余是登记元数据。
type priceCardContentResponse struct {
	Outcome             string          `json:"outcome"`
	PlanID              string          `json:"planId"`
	PlanVersion         string          `json:"planVersion"`
	Canonicalization    string          `json:"canonicalization"`
	ContentDigest       string          `json:"contentDigest"`
	PublicationApprover string          `json:"publicationApprover"`
	RegisteredAt        string          `json:"registeredAt"`
	Document            json.RawMessage `json:"document"`
}

// priceCardContentMissingResponse 是没有内容可交时的答复：只回显所问，让调用方核对答的是不是它问的那一版。
type priceCardContentMissingResponse struct {
	Outcome     string `json:"outcome"`
	PlanID      string `json:"planId"`
	PlanVersion string `json:"planVersion"`
}

// decodePriceCardContentQuery 封闭解查询串，交回所问的方案版本引用。错误一律是坏请求。
func decodePriceCardContentQuery(request *http.Request) (domain.VersionReference, error) {
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return domain.VersionReference{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	for key := range values {
		if key != priceCardContentIDParameter && key != priceCardContentVersionParameter {
			return domain.VersionReference{}, fmt.Errorf("%w: unknown query parameter %q", ErrMalformedRequest, key)
		}
	}
	id, err := singleQueryValue(values, priceCardContentIDParameter)
	if err != nil {
		return domain.VersionReference{}, err
	}
	version, err := singleQueryValue(values, priceCardContentVersionParameter)
	if err != nil {
		return domain.VersionReference{}, err
	}
	reference, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, id, version)
	if err != nil {
		return domain.VersionReference{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return reference, nil
}

func singleQueryValue(values url.Values, key string) (string, error) {
	got := values[key]
	if len(got) != 1 {
		return "", fmt.Errorf("%w: %q takes exactly one value", ErrMalformedRequest, key)
	}
	return got[0], nil
}
