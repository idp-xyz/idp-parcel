package shipmenthttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// ActiveRejectionIntake 把一次已认证的接入请求翻译成主动拒绝命令。它是接口而非解析代码，
// 理由与 ManualReviewCompletionIntake 相同；本口的翻译还额外携带结构化拒绝原因与证据
// 引用——采信自报的决定人等于让任何调用方替任何角色拒单。授权本身不在这里判：编排去问
// party-commercial（RejectShipmentRequestHandler 的授权先于一切写动作），Intake 只交出
// 「谁在请求」。
type ActiveRejectionIntake interface {
	IntakeActiveRejection(
		ctx context.Context,
		request *http.Request,
	) (application.RejectShipmentRequestCommand, error)
}

// ActiveRejectionHandler 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与
// 映射。真实装配接 RejectShipmentRequestHandler：主动拒绝在自己的命令事务里直接形成决定，
// 不发续办信封（ADR-0086 Decision 三）。
type ActiveRejectionHandler interface {
	Handle(
		ctx context.Context,
		command application.RejectShipmentRequestCommand,
	) (application.RejectShipmentRequestResult, error)
}

// NewRejectShipmentRequestEndpoint 交回主动拒绝的 HTTP 入口（票 09；ADR-0081 的命令面
// 保留条款）。
func NewRejectShipmentRequestEndpoint(
	intake ActiveRejectionIntake,
	handler ActiveRejectionHandler,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakeActiveRejection(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}

		result, err := handler.Handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeActiveRejectionOutcome(response, result)
	})
}

// activeRejectionResponse 是本端点的封闭响应形状。`已形成`与`任务已完结`带决定视图——后者交回
// 的是先到那一个（可能是接受），操作员要知道输给了什么：决定带主动拒绝留痕时再带实际决定方与
// 结构化原因，后到者凭它们自己比出是重放还是输给了别人；未决带原因与续办引用；拒绝成立而随附
// 释放未确定完成时带补偿续办引用，与判断续办分开。
type activeRejectionResponse struct {
	Outcome               string `json:"outcome"`
	RequestState          string `json:"requestState,omitempty"`
	DecisionKind          string `json:"decisionKind,omitempty"`
	Decider               string `json:"decider,omitempty"`
	RejectionReason       string `json:"rejectionReason,omitempty"`
	PendingReason         string `json:"pendingReason,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
	CompensationReference string `json:"compensationReference,omitempty"`
}

func writeActiveRejectionOutcome(
	response http.ResponseWriter,
	result application.RejectShipmentRequestResult,
) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := activeRejectionResponse{Outcome: outcome}
	if state := result.State().String(); state != "" {
		body.RequestState = state
	}
	if decision, present := result.AcceptanceDecision(); present {
		body.DecisionKind, body.Decider, body.RejectionReason = decisionEcho(decision)
	}
	if reason := result.PendingReason().String(); reason != "" {
		body.PendingReason = reason
	}
	if continuation := result.ContinuationReference().String(); continuation != "" {
		body.ContinuationReference = continuation
	}
	if compensation := result.CompensationReference().String(); compensation != "" {
		body.CompensationReference = compensation
	}

	status := http.StatusOK
	if result.Outcome() == application.ActiveRejectionFormed {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

// decisionEcho 是主动拒绝口与授权处置口共用的决定回显：决定种类，以及决定带主动拒绝留痕时的实际决定方与结构化
// 原因。两口撞上同一份在册决定时说的必须是同一句话，分写两份日后多回显一格时必然走散。规则形成的拒绝与接受没有
// 留痕，后两格留空。
func decisionEcho(decision domain.AcceptanceDecision) (kind, decider, reason string) {
	kind = "REJECTED"
	if decision.Accepted() {
		kind = "ACCEPTED"
	}
	if rejection, active := decision.ActiveRejection(); active {
		decider, reason = rejection.Decider().String(), rejection.Reason().String()
	}
	return kind, decider, reason
}
