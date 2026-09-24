package shipmenthttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

// AuthorizedDispositionIntake 把一次已认证的接入请求翻译成授权处置命令。它是接口而非解析代码，
// 理由与 ActiveRejectionIntake 相同；本口的翻译携带处置人、去向、结构化原因与证据引用——采信
// 自报的处置人等于让任何调用方替任何角色决定一份受限委托的去向。授权本身不在这里判：编排去问
// party-commercial（DisposeShipmentRequestHandler 的授权先于一切写动作），Intake 只交出「谁在请求」。
type AuthorizedDispositionIntake interface {
	IntakeAuthorizedDisposition(
		ctx context.Context,
		request *http.Request,
	) (application.DisposeShipmentRequestCommand, error)
}

// AuthorizedDispositionHandler 是本适配器转交的应用编排。适配器不判断任何业务结果，只转交与映射。
// 真实装配接 DisposeShipmentRequestHandler：两个去向都在自己的命令事务里收口，不发续办信封
// （ADR-0132 决定二）。
type AuthorizedDispositionHandler interface {
	Handle(
		ctx context.Context,
		command application.DisposeShipmentRequestCommand,
	) (application.DisposeShipmentRequestResult, error)
}

// NewDisposeShipmentRequestEndpoint 交回授权处置的 HTTP 入口（票 sa-preacceptance-policy-view/04；
// ADR-0081 的命令面保留条款、ADR-0132）。
func NewDisposeShipmentRequestEndpoint(
	intake AuthorizedDispositionIntake,
	handler AuthorizedDispositionHandler,
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}

		command, err := intake.IntakeAuthorizedDisposition(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}

		result, err := handler.Handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeAuthorizedDispositionOutcome(response, result)
	})
}

// authorizedDispositionResponse 是本端点的封闭响应形状。`已记录`带去向与之后的委托状态；撞上既有
// 处置时带先到那一份的去向；`任务已完结`带既有决定视图——处置人要知道输给了什么；`版本已换代`带
// 当前版本供重读队列；处置成立而随附释放未确定完成时带补偿续办引用。
type authorizedDispositionResponse struct {
	Outcome               string `json:"outcome"`
	RequestState          string `json:"requestState,omitempty"`
	DispositionChoice     string `json:"dispositionChoice,omitempty"`
	DecisionKind          string `json:"decisionKind,omitempty"`
	CurrentVersion        string `json:"currentVersion,omitempty"`
	CompensationReference string `json:"compensationReference,omitempty"`
}

func writeAuthorizedDispositionOutcome(
	response http.ResponseWriter,
	result application.DisposeShipmentRequestResult,
) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}

	body := authorizedDispositionResponse{Outcome: outcome}
	if state := result.State().String(); state != "" {
		body.RequestState = state
	}
	if disposition, present := result.Disposition(); present {
		body.DispositionChoice = disposition.Choice().String()
	}
	if decision, present := result.AcceptanceDecision(); present {
		if decision.Accepted() {
			body.DecisionKind = "ACCEPTED"
		} else {
			body.DecisionKind = "REJECTED"
		}
	}
	if version := result.CurrentVersion().String(); version != "" {
		body.CurrentVersion = version
	}
	if compensation := result.CompensationReference().String(); compensation != "" {
		body.CompensationReference = compensation
	}

	status := http.StatusOK
	if result.Outcome() == application.AuthorizedDispositionRecorded {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
