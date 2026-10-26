// Package visibilityhttp 是 visibility-exception 的 HTTP 入站适配器，按 ADR-0022 把
// 查询结果映射成响应：状态码只回答服务端有没有形成答案，业务判别一律进响应体的
// `outcome`。
//
// 包名与目录名不一致是刻意的，理由与 shipmenthttp 相同：目录按 ADR-0018 叫
// `adapters/http`，包名叫 `http` 会遮住标准库。
package visibilityhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	veps "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/parcelshipment"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ErrMalformedRequest 表示这次请求构造不出查询，且重发同样的内容不会改变结果。
// 外部运单号或其他标识只能定位候选对象，不能单独证明查询权限（UC-VE-008）——账户
// 只来自认证结果，请求自报账户、缺包裹定位、多带别的格或查询串解不开，查询都立不起来，落在这一格；
// 没有凭据或没有授予的请求答认证那几格，不落这里。
var ErrMalformedRequest = errors.New("visibility exception http: malformed request")

// TrackingViewQuery 是一次已授权的视图查询：租户与货主客户账户来自认证结果，包裹
// 引用来自请求定位。隔离在键上——查询只能问「我名下这个包裹」，问不出别人的；租户
// 是最高数据隔离边界（ADR-0003），同样只能来自认证结果。
type TrackingViewQuery struct {
	Tenant   domain.TenantID
	Customer domain.CustomerAccountReference
	Parcel   domain.TrackedParcelReference
}

// QueryIntake 把一次已认证的查询请求翻译成查询键。
//
// 它是接口而不是本包内的解析代码：请求方身份、货主客户账户与对象授权整组「必须同时
// 核对」（UC-VE-008），认证归首方客户渠道（ADR-0139 决定四），真实现是
// CustomerTrackingViewIntake；某租户客户既有渠道的适配才属 `PAR-INT-01`（ADR-0139
// 决定一）。采信客户自报的账户号会穿透账户隔离——那正是本上下文在视图编排里已经钉住
// 的边界，HTTP 面不得另开口子。本包不带任何采信实现，包括「开发用」的采信头部版本。
type QueryIntake interface {
	IntakeQuery(ctx context.Context, request *http.Request) (TrackingViewQuery, error)
}

// TrackingViewReader 是本端点消费的读口。查询不触发派生、披露或通知（CONTEXT：
// 「客户读取或门户展示→形成查询/展示结果，不自动形成异常披露决定、主动通知、送达
// 或客户确认」）——所以这里接存储读面，不接派生编排。
type TrackingViewReader interface {
	FindCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		customer domain.CustomerAccountReference,
		parcel domain.TrackedParcelReference,
	) (domain.CustomerTrackingView, bool, error)
}

// 传输层错误码。业务判别走 `outcome`，这里只说明为什么没有 `outcome`。
const (
	codeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	codeMalformedRequest = "MALFORMED_REQUEST"
	codeIntakeFailed     = "INTAKE_FAILED"
	codeNoAnswerFormed   = "NO_ANSWER_FORMED"
)

// 业务结果的封闭三格（ADR-0246 决定一），按客户系统的续办分（ADR-0029）：
//   - `CURRENT_VIEW`：读到请求方账户下这件包裹的当前视图。视图已形成而各维待确认或不展示也是这一格，逐维如实，
//     不改答「当前无可见结果」（ADR-0246 决定四）。
//   - `NO_VISIBLE_RESULT`：读不到视图，而 parcel-shipment 的当前已接受委托把这件包裹唯一归属于请求方账户。续办是
//     稍后再查。请求方账户只取自认证结果，拿得到这一格的只有归属方本人，他本就有权知道这件包裹存在。
//   - `VIEW_NOT_FOUND`：其余一切读不到视图的情形——包裹不存在、归属别的账户、归属不可确定（含委托接受之前）、多份
//     当前已接受委托同时声明它——同一格同一副字节，续办是核对查询标识。区分它们就是把对象存在性泄给跨账户探针
//     （UC-VE-008：「对未授权对象的外部响应不得泄露对象是否存在或属于其他客户」）；对外不另立「冲突」一格，理由见
//     ADR-0246 决定五。
const (
	outcomeCurrentView     = "CURRENT_VIEW"
	outcomeNoVisibleResult = "NO_VISIBLE_RESULT"
	outcomeViewNotFound    = "VIEW_NOT_FOUND"
)

// NewQueryCustomerTrackingViewEndpoint 交回 `UC-VE-008` 普通追踪查询的 HTTP 入口。它是首方客户族的口，4xx/5xx 写 RFC 9457
// 问题详情（ADR-0140 决定三）；本包操作者族各口仍写 {"error":{"code"}}，迁移另立后续票。读不到视图也是业务答案（200，
// ADR-0246 决定一），不是 404：归属方本人答 `NO_VISIBLE_RESULT`，其余答 `VIEW_NOT_FOUND`。
//
// attributions 是 parcel-shipment 的客户归属反查口，与视图派生同一口（ADR-0246 决定二）。它只是读：查询不因此补派生视图，
// 视图迟迟不形成的原因归派生一侧与运营追踪查阅。
func NewQueryCustomerTrackingViewEndpoint(
	intake QueryIntake,
	views TrackingViewReader,
	attributions ports.ParcelCustomerAccountView,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		query, err := intake.IntakeQuery(request.Context(), request)
		if err != nil {
			writeCustomerQueryIntakeProblem(response, request, err)
			return
		}

		view, found, err := views.FindCurrent(request.Context(), query.Tenant, query.Customer, query.Parcel)
		if err != nil {
			// 读不回是答案未形成，不是「无轨迹」也不是「无权」——伪装成后两者会让
			// 调用方把一次该重试的故障当成终局（UC-VE-008 AT-VE-168 的半边）。
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if !found {
			outcome, formed := outcomeWithoutView(request.Context(), attributions, query)
			if !formed {
				httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
				return
			}
			writeJSON(response, http.StatusOK, queryResponse{Outcome: outcome})
			return
		}
		writeJSON(response, http.StatusOK, queryResponse{
			Outcome: outcomeCurrentView,
			View:    newTrackingViewBody(view),
		})
	})
}

// outcomeWithoutView 判一次读不到视图的查询答哪一格（ADR-0246 决定二）。归属只在这里问，读到视图的热路径不多一次读；
// 每一次读不到视图的查询都问且只问一次，不论答出什么——问不问取决于答案的话，读的次数本身就成了探针看得见的区别
// （决定三）。比的只是「唯一归属账户等于请求方账户」这一位，答复不带归属于谁。
//
// 多于一份当前已接受委托同时声明这件包裹：派生一侧据同一具名错误落未决、不择一；查询一侧它是读不到视图的又一种，答
// `VIEW_NOT_FOUND`（决定五）。其余错误（反查读不动，或答复译不成本上下文的账户）都没形成归属结论，formed 为假，与视图
// 读不回同一格——降成任何业务格都会让客户系统把一次该重试的故障当成终局（UC-VE-008 AT-VE-168）。
func outcomeWithoutView(
	ctx context.Context,
	attributions ports.ParcelCustomerAccountView,
	query TrackingViewQuery,
) (outcome string, formed bool) {
	account, attributed, err := attributions.FindCustomerAccount(ctx, query.Tenant, query.Parcel)
	switch {
	case errors.Is(err, veps.ErrAmbiguousCustomerAccount):
		return outcomeViewNotFound, true
	case err != nil:
		return "", false
	case attributed && account == query.Customer:
		return outcomeNoVisibleResult, true
	default:
		return outcomeViewNotFound, true
	}
}

// writeCustomerQueryIntakeProblem 按恢复动作分格答 Intake 的失败（ADR-0139 决定四；ADR-0029），各格都不带 outcome（ADR-0022）。
func writeCustomerQueryIntakeProblem(response http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, ErrAccessChannelNotConfigured):
		httpapi.WriteProblem(response, request, http.StatusForbidden, codeAccessChannelNotConfigured)
	case errors.Is(err, ErrCustomerClientCredentialRejected):
		httpapi.WriteProblem(response, request, http.StatusUnauthorized, codeCustomerClientCredentialRejected)
	case errors.Is(err, ErrCustomerClientNotGranted):
		httpapi.WriteProblem(response, request, http.StatusForbidden, codeCustomerClientNotGranted)
	case errors.Is(err, ErrIdentityDependencyUnavailable):
		httpapi.WriteProblem(response, request, http.StatusServiceUnavailable, codeIdentityDependencyUnavailable)
	case errors.Is(err, ErrMalformedRequest):
		httpapi.WriteProblem(response, request, http.StatusBadRequest, codeMalformedRequest)
	default:
		httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeIntakeFailed)
	}
}

// queryResponse 是本端点的封闭响应形状。`outcome` 三格取本文件的字面常量，传输层不合并、不改名，也没有「其他」一格；
// `view` 只随 `CURRENT_VIEW` 在场，`NO_VISIBLE_RESULT` 与 `VIEW_NOT_FOUND` 都只有 `outcome`。
type queryResponse struct {
	Outcome string            `json:"outcome"`
	View    *trackingViewBody `json:"view,omitempty"`
}

// trackingViewBody 逐字段透出视图版本：四维三态如实转写，不合成统一状态，也不替
// 待确认或不展示的维编内容——那两态在领域构造期就不带内容，这里无内容可编。
//
// 不带所依据的投影版本：投影是内部对象，客户的采用版本由 `version` 承担（CONTEXT「留存
// 投影版本的按版本读回属运营追踪查阅，不属客户发布物」）。视图四维不变时不换代、只跟上
// 新投影，带着它，同一个 `version` 就会对客户答出不同内容。
type trackingViewBody struct {
	Version      string        `json:"version"`
	Parcel       string        `json:"parcel"`
	PublishedAt  string        `json:"publishedAt"`
	PriorVersion string        `json:"priorVersion,omitempty"`
	Milestones   dimensionBody `json:"milestones"`
	ETA          dimensionBody `json:"eta"`
	Final        dimensionBody `json:"final"`
	Note         dimensionBody `json:"note"`
}

type dimensionBody struct {
	State      string `json:"state"`
	ContentRef string `json:"contentRef,omitempty"`
}

func newTrackingViewBody(view domain.CustomerTrackingView) *trackingViewBody {
	dimensions := view.Dimensions()
	body := &trackingViewBody{
		Version:     view.Version().String(),
		Parcel:      view.Parcel().String(),
		PublishedAt: view.PublishedAt().UTC().Format(time.RFC3339Nano),
		Milestones:  newDimensionBody(dimensions.Milestones),
		ETA:         newDimensionBody(dimensions.ETA),
		Final:       newDimensionBody(dimensions.Final),
		Note:        newDimensionBody(dimensions.Note),
	}
	if prior, superseding := view.PriorVersion(); superseding {
		body.PriorVersion = prior.String()
	}
	return body
}

func newDimensionBody(dimension domain.ViewDimension) dimensionBody {
	body := dimensionBody{State: dimension.State().String()}
	if content, shown := dimension.Content(); shown {
		body.ContentRef = content.String()
	}
	return body
}

// problemResponse 刻意不带自由文本消息，理由与 shipmenthttp 相同：底层失败的措辞会
// 捎带账户或对象的存在性，而本用例要求未授权响应不泄露对象是否存在。
type problemResponse struct {
	Error problemDetail `json:"error"`
}

type problemDetail struct {
	Code string `json:"code"`
}

func writeProblem(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, problemResponse{Error: problemDetail{Code: code}})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

// 编译期锁缝：读口的形状必须与视图库端口保持一致——本适配器不新造查询语义，只消费
// 编排批已经钉住的那一个读面。
var _ TrackingViewReader = ports.CustomerViewStore(nil)
