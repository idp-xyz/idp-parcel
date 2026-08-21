// Package shipmenthttp 是 parcel-shipment 的 HTTP 入站适配器，按 ADR-0022 把应用结果
// 映射成响应：状态码只回答服务端有没有形成答案，业务判别一律进响应体的 `outcome`。
//
// 包名与目录名不一致是刻意的。目录按 ADR-0018 的落位结论叫 `adapters/http`，而把包名也叫
// `http` 会遮住标准库，使本包每一处 `http.ResponseWriter` 都得写成别名。
package shipmenthttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// ErrMalformedRequest 表示这次请求构造不出命令，且重发同样的内容不会改变结果。
//
// SubmissionIntake 的实现用它包裹调用方的错，本包据以回 4xx；不带它的失败一律是 5xx。这条
// 分法直接决定离线客户端的动作——4xx 出队交给人，5xx 留在队里重发——所以把依赖不可用误包成
// 它，会让一条本该重试的请求被丢掉。
var ErrMalformedRequest = errors.New("parcel shipment http: malformed request")

// SubmissionIntake 把一次已认证的接入请求翻译成一条提交命令。
//
// 它是接口而不是本包内的解析代码，因为翻译要用到的三样东西没有一样是本包能自己定的：
//
//   - 来源信封（租户、货主客户账户、来源、请求标识）按 `UC-PS-001` 的输入语义契约整组
//     「客户不可声明」，只能来自认证结果。真实接入渠道的认证方式属 `PAR-INT-01` 待提供，
//     而采信客户自报的租户号会穿透 ADR-0003 的最高数据隔离边界。
//   - 载荷摘要由领域侧 `CanonicalizeSubmissionPayload` 按版本化规范化形状产出（ADR-0014，
//     形状已实现）；但把渠道原始载荷翻译成 `SubmissionPayloadSpec` 词表的规则属该渠道的
//     接入契约，与认证方式同在 `PAR-INT-01` 待提供。
//   - 准入范围与期望规则修订由试点准入控制按 `PAR-GOV-03..07` 装配；适配器自己造一个，
//     等于替治理规则决定「完整拟受理范围」是什么，而 `UC-PS-001` 步骤 3B 把那件事判给了
//     试点准入控制。
//
// 三项未决期间本包不带任何实现，包括「开发用」的采信头部版本——那正是红线所禁的生产默认值。
type SubmissionIntake interface {
	IntakeSubmission(ctx context.Context, request *http.Request) (application.SubmitShipmentRequestCommand, error)
}

// SubmissionHandler 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。
type SubmissionHandler interface {
	Handle(
		ctx context.Context,
		command application.SubmitShipmentRequestCommand,
	) (application.SubmitShipmentRequestResult, error)
}

// 传输层错误码。它们不是业务原因目录：业务原因走 `outcome`，这里只说明为什么没有 `outcome`。
const (
	codeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	codeMalformedRequest = "MALFORMED_REQUEST"
	codeIntakeFailed     = "INTAKE_FAILED"
	codeNoAnswerFormed   = "NO_ANSWER_FORMED"
	codeUnnamedOutcome   = "UNNAMED_OUTCOME"
)

// NewSubmitShipmentRequestEndpoint 交回 `UC-PS-001` 提交编排的 HTTP 入口。
func NewSubmitShipmentRequestEndpoint(intake SubmissionIntake, handler SubmissionHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakeSubmission(request.Context(), request)
		if err != nil {
			if errors.Is(err, ErrAccessChannelNotConfigured) {
				writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
				return
			}
			if errors.Is(err, ErrMalformedRequest) {
				writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
				return
			}
			writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
			return
		}

		result, err := handler.Handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeOutcome(response, result)
	})
}

// submitResponse 是本端点的封闭响应形状。`outcome` 取应用结果枚举的原名，传输层不合并、
// 不改名，也没有「其他」这一格——留那一格就是给下一次合成一格开的门。
type submitResponse struct {
	Outcome             string         `json:"outcome"`
	ShipmentRequestID   string         `json:"shipmentRequestId,omitempty"`
	ProductionOwnership *ownershipView `json:"productionOwnership,omitempty"`
	GateBlockReasons    []string       `json:"gateBlockReasons,omitempty"`
}

// ownershipView 交回归属决定本身而不只是一个结论字符串。`UC-PS-001` 的结果语义要求非本产品
// 归属报出当前权威方与安全交接结果、归属未决报出当前缺口与安全续办引用；少了它们，调用方
// 只知道被挡了，不知道该找治理还是找客户。
type ownershipView struct {
	DecisionID            string `json:"decisionId"`
	Authority             string `json:"authority"`
	AdmissionControl      string `json:"admissionControl"`
	RuleVersion           string `json:"ruleVersion"`
	Revision              string `json:"revision"`
	OtherAuthority        string `json:"otherAuthority,omitempty"`
	HandoffReference      string `json:"handoffReference,omitempty"`
	UnresolvedReason      string `json:"unresolvedReason,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
	SuspensionReference   string `json:"suspensionReference,omitempty"`
}

// problemResponse 刻意不带自由文本消息。底层失败的措辞会捎带租户、客户账户或线路的存在性，
// 而 `UC-PS-001` 要求错误信息不得泄露其他客户的存在、业务量或内容；调用方要分流靠稳定的
// `code`，要细节去查带关联标识的服务端记录。
type problemResponse struct {
	Error problemDetail `json:"error"`
}

type problemDetail struct {
	Code string `json:"code"`
}

func writeOutcome(response http.ResponseWriter, result application.SubmitShipmentRequestResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		// 应用层交回了一个没有名字的结果。这是编程错误，不是业务答案，因此不能带 `outcome`
		// 上线——空字符串会被客户端当成一种新的业务结果。
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := submitResponse{Outcome: outcome}
	if requestID, present := result.ShipmentRequestID(); present {
		body.ShipmentRequestID = requestID.String()
	}
	if decision, present := result.OwnershipDecision(); present {
		body.ProductionOwnership = newOwnershipView(decision)
	}
	for _, reason := range result.GateBlockReasons() {
		body.GateBlockReasons = append(body.GateBlockReasons, reason.String())
	}

	status := http.StatusOK
	if result.Outcome() == application.OutcomeSubmitted {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

func newOwnershipView(decision domain.ProductionOwnershipDecision) *ownershipView {
	view := &ownershipView{
		DecisionID:       decision.DecisionID().String(),
		Authority:        decision.Authority().String(),
		AdmissionControl: decision.AdmissionControl().String(),
		RuleVersion:      decision.RuleVersion().String(),
		Revision:         decision.Revision().String(),
	}
	if other, present := decision.OtherAuthorityReference(); present {
		view.OtherAuthority = other.String()
	}
	if handoff, present := decision.HandoffReference(); present {
		view.HandoffReference = handoff.String()
	}
	if reason, continuation, present := decision.UnresolvedDetails(); present {
		view.UnresolvedReason = reason.String()
		view.ContinuationReference = continuation.String()
	}
	if suspension, present := decision.SuspensionReference(); present {
		view.SuspensionReference = suspension.String()
	}
	return view
}

func writeProblem(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, problemResponse{Error: problemDetail{Code: code}})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
