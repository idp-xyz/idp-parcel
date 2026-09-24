package shipmenthttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

// 本文件是`面单继续尝试决定`写面的两个 HTTP 入口（票 label-channel/30 做法 4）：受控关闭与重开各一个端点，
// 同一族受控编排壳。适配器不判断任何业务结果，只转交与映射；两条命令的响应形状同一个——它们落进同一条版本链，
// 调用方读的是同一种东西。

// ContinuedAttemptDecisionIntake 把一次已认证的接入请求翻译成关闭或重开命令。它是接口而非解析代码：委托来源
// 身份、请求方与货主账户都不采信自报——采信了就是让任何调用方替任何货主签字；登录操作人的身份只作操作证据、
// 不进任何一格（CONTEXT「登录操作人可以作为操作证据，但不能替代实际决定方和授权角色」），决定方由授权答复给出。
// 生产渠道是操作者渠道的「运营决定」能力面（ADR-0151），真渠道 Intake 就位前本包不带任何实现。
type ContinuedAttemptDecisionIntake interface {
	IntakeControlledClosure(ctx context.Context, request *http.Request) (application.FormControlledClosureCommand, error)
	IntakeReopening(ctx context.Context, request *http.Request) (application.FormReopeningCommand, error)
}

// ContinuedAttemptDecisionHandler 是本适配器转交的应用编排（两条命令一个 handler，与应用层同形）。
type ContinuedAttemptDecisionHandler interface {
	FormControlledClosure(ctx context.Context, command application.FormControlledClosureCommand) (application.ContinuedAttemptDecisionResult, error)
	FormReopening(ctx context.Context, command application.FormReopeningCommand) (application.ContinuedAttemptDecisionResult, error)
}

// NewFormControlledClosureEndpoint 交回受控关闭决定的 HTTP 入口。
func NewFormControlledClosureEndpoint(intake ContinuedAttemptDecisionIntake, handler ContinuedAttemptDecisionHandler) http.Handler {
	return continuedAttemptDecisionEndpoint(func(ctx context.Context, request *http.Request) (application.ContinuedAttemptDecisionResult, error, error) {
		command, err := intake.IntakeControlledClosure(ctx, request)
		if err != nil {
			return application.ContinuedAttemptDecisionResult{}, err, nil
		}
		result, err := handler.FormControlledClosure(ctx, command)
		return result, nil, err
	})
}

// NewFormReopeningEndpoint 交回重开决定的 HTTP 入口。
func NewFormReopeningEndpoint(intake ContinuedAttemptDecisionIntake, handler ContinuedAttemptDecisionHandler) http.Handler {
	return continuedAttemptDecisionEndpoint(func(ctx context.Context, request *http.Request) (application.ContinuedAttemptDecisionResult, error, error) {
		command, err := intake.IntakeReopening(ctx, request)
		if err != nil {
			return application.ContinuedAttemptDecisionResult{}, err, nil
		}
		result, err := handler.FormReopening(ctx, command)
		return result, nil, err
	})
}

// continuedAttemptDecisionEndpoint 是两个端点共用的传输层分流（ADR-0022 纪律，与撤回端点同款）：非 POST 405；
// 接入未配置 403、构造不出命令 400（重发不会变）、接入依赖不可用 5xx；编排没形成答案 5xx `NO_ANSWER_FORMED`——
// 不细分「包裹不存在」：登记册以租户为键，否定结果本就不带这个差别，细分出一个「未找到」码就把统一不可见结果拆开了。
// step 交回两个 error 位：第一个是接入那一段的，第二个是编排那一段的，两段的映射不同。
func continuedAttemptDecisionEndpoint(
	step func(context.Context, *http.Request) (application.ContinuedAttemptDecisionResult, error, error),
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		result, intakeErr, handleErr := step(request.Context(), request)
		if intakeErr != nil {
			writeIntakeProblem(response, intakeErr)
			return
		}
		if handleErr != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeContinuedAttemptDecisionOutcome(response, result)
	})
}

// continuedAttemptDecisionResponse 是两个端点共用的封闭响应形状。`outcome` 取应用结果枚举的原名；`已形成`时带决定的
// 身份几格（标识、种类、生效时间、关闭的截断边界或重开所解的关闭）；`未决`带等的是哪个依赖；`输入未受理`带哪一格
// 立不起来。决定方 / 授权角色 / 授权依据快照不在响应里：它们是审计面（票 label-channel/10 读面派生）要读的，
// 写面回执只需让调用方知道形成了哪一条。
type continuedAttemptDecisionResponse struct {
	Outcome             string `json:"outcome"`
	DecisionID          string `json:"decisionId,omitempty"`
	DecisionKind        string `json:"decisionKind,omitempty"`
	EffectiveAt         string `json:"effectiveAt,omitempty"`
	CutoffBoundary      string `json:"cutoffBoundary,omitempty"`
	RelatedPriorClosure string `json:"relatedPriorClosure,omitempty"`
	PendingReason       string `json:"pendingReason,omitempty"`
	Refusal             string `json:"refusal,omitempty"`
}

func writeContinuedAttemptDecisionOutcome(response http.ResponseWriter, result application.ContinuedAttemptDecisionResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := continuedAttemptDecisionResponse{Outcome: outcome}
	if decision, present := result.Decision(); present {
		body.DecisionID = decision.ID().String()
		body.DecisionKind = decision.Kind().String()
		body.EffectiveAt = decision.EffectiveAt().UTC().Format(time.RFC3339Nano)
		body.CutoffBoundary = decision.CutoffBoundary().String()
		body.RelatedPriorClosure = decision.RelatedPriorClosure().String()
	}
	if reason := result.PendingReason().String(); reason != "" {
		body.PendingReason = reason
	}
	if refusal := result.Refusal().String(); refusal != "" {
		body.Refusal = refusal
	}

	status := http.StatusOK
	if result.Outcome() == application.ContinuedAttemptDecisionFormed {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
