package shipmenthttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

// SupplementIntake 把一次已认证的接入请求翻译成一条受控补充命令（ADR-0045 的编排半边，
// ADR-0106 Decision 四把它接进生产）。它是接口而非解析代码，理由与 SubmissionIntake 完全相同
// 且多一条：补充请求自己的来源身份与产生委托的那次提交必须分开成两个信封，且基准版本是客户
// 自报的「我在补充哪一版」——三样都只能来自渠道认证与接入契约（PAR-INT-01 待提供）。未决期间
// 本包不带任何实现，包括「开发用」的采信头部版本。
type SupplementIntake interface {
	IntakeSupplement(ctx context.Context, request *http.Request) (application.FormNewSubmissionVersionCommand, error)
}

// SupplementHandler 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。真实
// 装配接 cmd/parcel-api 的受控补充边界壳（新版本落库与「新提交版本已形成」信封同一事务，
// ADR-0106 Decision 三）。
type SupplementHandler interface {
	Handle(
		ctx context.Context,
		command application.FormNewSubmissionVersionCommand,
	) (application.FormNewSubmissionVersionResult, error)
}

// NewFormNewSubmissionVersionEndpoint 交回受控补充的 HTTP 入口：客户在`已提交`委托上形成同一
// 委托的新提交版本（`AT-PS-036` 第一支）。
//
// 委托查不到时应用层上抛技术错误，本端点把它与其余没形成答案的失败一并回 5xx
// `NO_ANSWER_FORMED`——统一不可见结果不拆开，理由同撤回端点。
func NewFormNewSubmissionVersionEndpoint(intake SupplementIntake, handler SupplementHandler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakeSupplement(request.Context(), request)
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
		writeSupplementOutcome(response, result)
	})
}

// supplementResponse 是本端点的封闭响应形状。`outcome` 取应用结果枚举的原名；新版本已形成
// （或重放读回原版本）时带版本号——客户下一次补充要以它为基准；决定已越过提交边界时带决定
// 视图（补充来晚了，客户要读的是那份决定再按方向走资料修订或关联新委托）；未决时带原因与
// 续办引用。没有任何一格是接受判决：新版本落地后委托仍为`已提交`。
type supplementResponse struct {
	Outcome               string `json:"outcome"`
	SubmissionVersionID   string `json:"submissionVersionId,omitempty"`
	DecisionKind          string `json:"decisionKind,omitempty"`
	PendingReason         string `json:"pendingReason,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
}

func writeSupplementOutcome(response http.ResponseWriter, result application.FormNewSubmissionVersionResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := supplementResponse{Outcome: outcome}
	if version, present := result.Version(); present {
		body.SubmissionVersionID = version.VersionID().String()
	}
	if decision, present := result.AcceptanceDecision(); present {
		if decision.Accepted() {
			body.DecisionKind = "ACCEPTED"
		} else {
			body.DecisionKind = "REJECTED"
		}
	}
	if reason := result.PendingReason().String(); reason != "" {
		body.PendingReason = reason
	}
	if continuation := result.ContinuationReference().String(); continuation != "" {
		body.ContinuationReference = continuation
	}

	status := http.StatusOK
	if result.Outcome() == application.SupplementRecorded {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
