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

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ErrMalformedRequest 表示这次请求构造不出查询，且重发同样的内容不会改变结果。
// 外部运单号或其他标识只能定位候选对象，不能单独证明查询权限（UC-VE-008）——缺
// 账户授权的请求在翻译处就立不起来，落在这一格。
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
// 它是接口而不是本包内的解析代码，理由与 shipmenthttp.SubmissionIntake 相同：请求方
// 身份、货主客户账户与对象授权整组「必须同时核对」（UC-VE-008），认证方式属
// `PAR-INT-01` 待提供；采信客户自报的账户号会穿透账户隔离——那正是本上下文在视图
// 编排里已经钉住的边界，HTTP 面不得另开口子。未决期间本包不带任何实现，包括「开发用」
// 的采信头部版本。
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

// 业务结果的封闭两格。`VIEW_NOT_FOUND` 承担 ADR-0029 的探针纪律：包裹不存在、不属于
// 请求账户、或视图尚未形成，一律这一格——区分它们就是把对象存在性泄给跨账户探针
// （UC-VE-008：「对未授权对象的外部响应不得泄露对象是否存在或属于其他客户」）。
const (
	outcomeCurrentView  = "CURRENT_VIEW"
	outcomeViewNotFound = "VIEW_NOT_FOUND"
)

// NewQueryCustomerTrackingViewEndpoint 交回 `UC-VE-008` 普通追踪查询的 HTTP 入口。
func NewQueryCustomerTrackingViewEndpoint(intake QueryIntake, views TrackingViewReader) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			response.Header().Set("Allow", http.MethodGet)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		query, err := intake.IntakeQuery(request.Context(), request)
		if err != nil {
			if errors.Is(err, ErrMalformedRequest) {
				writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
				return
			}
			writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
			return
		}

		view, found, err := views.FindCurrent(request.Context(), query.Tenant, query.Customer, query.Parcel)
		if err != nil {
			// 读不回是答案未形成，不是「无轨迹」也不是「无权」——伪装成后两者会让
			// 调用方把一次该重试的故障当成终局（UC-VE-008 AT-VE-168 的半边）。
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		if !found {
			writeJSON(response, http.StatusOK, queryResponse{Outcome: outcomeViewNotFound})
			return
		}
		writeJSON(response, http.StatusOK, queryResponse{
			Outcome: outcomeCurrentView,
			View:    newTrackingViewBody(view),
		})
	})
}

// queryResponse 是本端点的封闭响应形状。`outcome` 两格取本文件的字面常量，传输层
// 不合并、不改名，也没有「其他」一格。
type queryResponse struct {
	Outcome string            `json:"outcome"`
	View    *trackingViewBody `json:"view,omitempty"`
}

// trackingViewBody 逐字段透出视图版本：四维三态如实转写，不合成统一状态，也不替
// 待确认或不展示的维编内容——那两态在领域构造期就不带内容，这里无内容可编。
type trackingViewBody struct {
	Version           string        `json:"version"`
	Parcel            string        `json:"parcel"`
	BasedOnProjection string        `json:"basedOnProjection"`
	PublishedAt       string        `json:"publishedAt"`
	PriorVersion      string        `json:"priorVersion,omitempty"`
	Milestones        dimensionBody `json:"milestones"`
	ETA               dimensionBody `json:"eta"`
	Final             dimensionBody `json:"final"`
	Note              dimensionBody `json:"note"`
}

type dimensionBody struct {
	State      string `json:"state"`
	ContentRef string `json:"contentRef,omitempty"`
}

func newTrackingViewBody(view domain.CustomerTrackingView) *trackingViewBody {
	dimensions := view.Dimensions()
	body := &trackingViewBody{
		Version:           view.Version().String(),
		Parcel:            view.Parcel().String(),
		BasedOnProjection: view.BasedOn().String(),
		PublishedAt:       view.PublishedAt().UTC().Format(time.RFC3339Nano),
		Milestones:        newDimensionBody(dimensions.Milestones),
		ETA:               newDimensionBody(dimensions.ETA),
		Final:             newDimensionBody(dimensions.Final),
		Note:              newDimensionBody(dimensions.Note),
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
