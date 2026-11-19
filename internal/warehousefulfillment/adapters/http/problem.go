// Package warehousehttp 承载仓配履约主数据登记册的 HTTP 适配器（ADR-0375 决定四、五）：仓的登记口、各仓当前修订的查阅口、
// 一个仓的修订史口，以及部署开关关着时仓配各口那一份固定的答复。
//
// 本包的 4xx 与 5xx 一律是 RFC 9457 问题详情（ADR-0140 决定三），码归本包、媒体类型与成员归 httpapi。业务判别只在 `outcome`
// （ADR-0022）：问题详情只说明为什么没有 `outcome`。
package warehousehttp

import (
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 信封一层的各格答复，各对一种恢复动作（ADR-0029）。登记册配置写与主数据与运营查阅读这两个面都不判准入范围，所以没有
// 「不在准入范围」那一格（ADR-0100 决定四）。
var (
	// ErrAccessChannelNotConfigured：没有任何已启用的接入渠道（403）——接入方去提供并配置渠道参数，改请求或重试都不会好。
	ErrAccessChannelNotConfigured = errors.New("warehouse fulfillment http: access channel is not configured")
	// ErrOperatorCredentialRejected：令牌缺失、过期或校验不过（401）——持令牌的人去登录或换令牌。
	ErrOperatorCredentialRejected = errors.New("warehouse fulfillment http: operator credential rejected")
	// ErrOperatorNotGranted：令牌有效，但不在册、不绑这个租户或没有这一面的授予（403）——管授予的人去登记。
	ErrOperatorNotGranted = errors.New("warehouse fulfillment http: operator holds no grant for this face")
	// ErrIdentityDependencyUnavailable：发行方公钥集或操作者册读不动（503）——运维去救依赖。
	ErrIdentityDependencyUnavailable = errors.New("warehouse fulfillment http: identity dependency unavailable")
	// ErrMalformedRequest：这次请求构造不出命令或查询，重发同样的内容不会好（400）。
	ErrMalformedRequest = errors.New("warehouse fulfillment http: malformed request")
)

// 传输层错误码。码只命名状态，不命名参数（ADR-0055）。
const (
	codeAccessChannelNotConfigured    = "ACCESS_CHANNEL_NOT_CONFIGURED"
	codeOperatorCredentialRejected    = "OPERATOR_CREDENTIAL_REJECTED"
	codeOperatorNotGranted            = "OPERATOR_NOT_GRANTED"
	codeIdentityDependencyUnavailable = "IDENTITY_DEPENDENCY_UNAVAILABLE"
	codeMethodNotAllowed              = "METHOD_NOT_ALLOWED"
	codeMalformedRequest              = "MALFORMED_REQUEST"
	codeIntakeFailed                  = "INTAKE_FAILED"
	codeNoAnswerFormed                = "NO_ANSWER_FORMED"
	// codeUnnamedOutcome：应用层交回没有名字的结果是编程错误，不是业务答案，不能带空 `outcome` 上线——空串会被调用侧当成
	// 一种新的业务结果。
	codeUnnamedOutcome = "UNNAMED_OUTCOME"
)

// malformedRequest 是带理由的 ErrMalformedRequest：理由只谈调用方自己这份载荷或查询错在哪，随 400 的 detail 过线。
type malformedRequest struct {
	reason string
}

func (err *malformedRequest) Error() string {
	return ErrMalformedRequest.Error() + ": " + err.reason
}

func (err *malformedRequest) Is(target error) bool {
	return target == ErrMalformedRequest
}

// writeIntakeProblem 把 Intake 交回的错答成它的那一格。先分信封各格，再分请求畸形，其余是 Intake 自己出了故障（5xx，成因
// 不外泄）。
func writeIntakeProblem(response http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, ErrAccessChannelNotConfigured):
		httpapi.WriteProblem(response, request, http.StatusForbidden, codeAccessChannelNotConfigured)
	case errors.Is(err, ErrOperatorCredentialRejected):
		httpapi.WriteProblem(response, request, http.StatusUnauthorized, codeOperatorCredentialRejected)
	case errors.Is(err, ErrOperatorNotGranted):
		httpapi.WriteProblem(response, request, http.StatusForbidden, codeOperatorNotGranted)
	case errors.Is(err, ErrIdentityDependencyUnavailable):
		httpapi.WriteProblem(response, request, http.StatusServiceUnavailable, codeIdentityDependencyUnavailable)
	case errors.Is(err, ErrMalformedRequest):
		var malformed *malformedRequest
		if errors.As(err, &malformed) {
			httpapi.WriteProblemWithDetail(response, request, http.StatusBadRequest, codeMalformedRequest, malformed.reason)
			return
		}
		httpapi.WriteProblem(response, request, http.StatusBadRequest, codeMalformedRequest)
	default:
		httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeIntakeFailed)
	}
}

// writePageQueryProblem 答列表查询参数不成立的那一格（ADR-0144 决定一、四）：筛选维、排序与游标按册声明解不开即答 400，理由
// 散文原样进 detail。在 Intake 之后解码：渠道未配置时对一切请求同答 403，没有凭据的调用方也就探不出本册开放了哪些查询维度。
func writePageQueryProblem(response http.ResponseWriter, request *http.Request, err error) {
	var malformed *cataloguepage.MalformedQuery
	if errors.As(err, &malformed) {
		httpapi.WriteProblemWithDetail(response, request, http.StatusBadRequest, codeMalformedRequest, malformed.Reason)
		return
	}
	httpapi.WriteProblem(response, request, http.StatusBadRequest, codeMalformedRequest)
}

// refuseMethod 是各口自守的方法门：方法不对答 405 并带 Allow。路由层刻意不按方法设卡（httpapi.BusinessEndpoint），
// 否则 chi 会用自己的纯文本 405 先答。
func refuseMethod(response http.ResponseWriter, request *http.Request, allowed string) bool {
	if request.Method == allowed {
		return false
	}
	response.Header().Set("Allow", allowed)
	httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
	return true
}
