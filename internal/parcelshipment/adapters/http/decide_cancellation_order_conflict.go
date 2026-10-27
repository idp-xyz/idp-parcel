package shipmenthttp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

// codeCancellationOrderConflictNotVisible 是`统一不可见结果`的问题码：册上没有这份冲突、属于另一个租户、几段对不上，同一个码。
const codeCancellationOrderConflictNotVisible = "CANCELLATION_ORDER_CONFLICT_NOT_VISIBLE"

// CancellationOrderConflictDecisionIntake 把一次已认证的接入请求翻译成冲突解开决定命令。它是接口而非解析代码，理由同本包其余运营决定口：决定人与租户只能来自
// 认证结果，采信自报就是让任何调用方替任何人裁断一件包裹上取消与边界事实谁在先。
type CancellationOrderConflictDecisionIntake interface {
	IntakeCancellationOrderConflictDecision(
		ctx context.Context,
		request *http.Request,
	) (application.DecideCancellationOrderConflictCommand, error)
}

// CancellationOrderConflictDecisionHandler 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。真实装配接
// DecideCancellationOrderConflictHandler，外面包一笔事务：决定留痕、作废与续办信封同生共死。
type CancellationOrderConflictDecisionHandler interface {
	Handle(
		ctx context.Context,
		command application.DecideCancellationOrderConflictCommand,
	) (application.DecideCancellationOrderConflictResult, error)
}

var _ CancellationOrderConflictDecisionIntake = UnconfiguredIntake{}

// IntakeCancellationOrderConflictDecision 不读请求：渠道未配置就无从铸造决定人。
func (UnconfiguredIntake) IntakeCancellationOrderConflictDecision(context.Context, *http.Request) (application.DecideCancellationOrderConflictCommand, error) {
	return application.DecideCancellationOrderConflictCommand{}, ErrAccessChannelNotConfigured
}

// NewDecideCancellationOrderConflictEndpoint 交回取消先后冲突解开的 HTTP 入口（POST /cancellation-order-conflict-decisions；ADR-0295 决定一至三）。
func NewDecideCancellationOrderConflictEndpoint(
	intake CancellationOrderConflictDecisionIntake,
	handler CancellationOrderConflictDecisionHandler,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeCancellationOrderConflictDecision(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := handler.Handle(request.Context(), command)
		if errors.Is(err, application.ErrCancellationOrderConflictNotVisible) {
			writeProblem(response, http.StatusNotFound, codeCancellationOrderConflictNotVisible)
			return
		}
		if err != nil {
			writeCommandProblem(response, err)
			return
		}
		writeCancellationOrderConflictDecisionOutcome(response, result)
	})
}

// cancellationOrderConflictDecisionResponse 是本端点的封闭响应形状。`已决定`与`已被决定`带去向与决定时刻（后者是先到那一份的）；`已决定`且认定取消不成立时带
// 本次作废的终局不采用行数。回执不回显决定人：调用方就是决定人，谁决定的在留痕里（`cancellation_order_conflict_decision` 表，格式 `发行方#sub`）。
type cancellationOrderConflictDecisionResponse struct {
	Outcome        string `json:"outcome"`
	DecisionKind   string `json:"decisionKind"`
	DecidedAt      string `json:"decidedAt"`
	VoidedRefusals int    `json:"voidedRefusals,omitempty"`
}

func writeCancellationOrderConflictDecisionOutcome(
	response http.ResponseWriter,
	result application.DecideCancellationOrderConflictResult,
) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	decision := result.Decision()
	status := http.StatusOK
	if result.Outcome() == application.CancellationOrderConflictDecided {
		status = http.StatusCreated
	}
	writeJSON(response, status, cancellationOrderConflictDecisionResponse{
		Outcome:        outcome,
		DecisionKind:   decision.Kind.String(),
		DecidedAt:      decision.DecidedAt.UTC().Format(time.RFC3339Nano),
		VoidedRefusals: result.VoidedRefusals(),
	})
}
