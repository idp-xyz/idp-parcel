package warehousehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

// OperatorPreAdviceCloseIntake 是入库预报完结的运营决定 Intake。先认证（授予与准入范围），后读请求体。
type OperatorPreAdviceCloseIntake struct {
	authenticator PreAdviceCloseAuthenticator
}

func NewOperatorPreAdviceCloseIntake(authenticator PreAdviceCloseAuthenticator) (*OperatorPreAdviceCloseIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: inbound pre-advice close intake needs an authenticator")
	}
	return &OperatorPreAdviceCloseIntake{authenticator: authenticator}, nil
}

func (intake *OperatorPreAdviceCloseIntake) IntakeClosePreAdvice(
	ctx context.Context,
	request *http.Request,
) (application.ClosePreAdviceCommand, error) {
	identity, err := intake.authenticator.AuthenticatePreAdviceClose(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.ClosePreAdviceCommand{}, err
	}
	command, err := registrationjson.ClosePreAdviceCommand(requestBody(request), identity.Tenant, identity.Registrant)
	if err != nil {
		return application.ClosePreAdviceCommand{}, asMalformed(err)
	}
	return command, nil
}

var (
	_ ClosePreAdviceIntake = (*OperatorPreAdviceCloseIntake)(nil)
	_ PreAdviceCloser      = (*application.ClosePreAdviceHandler)(nil)
)

// NewClosePreAdviceEndpoint 交回完结的入口（POST /warehouse-fulfillment-inbound-pre-advice-closures）。
// 新建意义的完结答 201，其余业务结果答 200。问题体用运营决定口已有的形状，这一行不是作业事实。
func NewClosePreAdviceEndpoint(intake ClosePreAdviceIntake, closer PreAdviceCloser) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodPost) {
			return
		}
		command, err := intake.IntakeClosePreAdvice(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := closer.ClosePreAdvice(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "inbound pre-advice close did not form an answer", "preAdviceId", command.ID.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeCloseAnswer(response, result)
	})
}

func writeCloseAnswer(response http.ResponseWriter, result application.ClosePreAdviceResult) {
	name := result.Outcome().String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := closePreAdviceAnswer{Outcome: name}
	status := http.StatusOK
	switch result.Outcome() {
	case application.ClosePreAdviceClosed, application.ClosePreAdviceExistingResult:
		closure, present := result.Closure()
		if !present {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		answer.Closure = closureBodyOf(&closure, nil, false, true)
		if result.Outcome() == application.ClosePreAdviceClosed {
			status = http.StatusCreated
		}
	case application.ClosePreAdviceStatusMismatch:
		echoed, present := result.Status()
		if !present || echoed.String() == "" {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		answer.Status = echoed.String()
	}
	writeJSON(response, status, answer)
}

type closePreAdviceAnswer struct {
	Outcome string                `json:"outcome"`
	Status  string                `json:"status,omitempty"`
	Closure *preAdviceClosureBody `json:"closure,omitempty"`
}
