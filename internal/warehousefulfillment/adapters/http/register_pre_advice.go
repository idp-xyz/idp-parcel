package warehousehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

// PreAdviceOnBehalf 是代录三个写口转交的受理编排。事务边界在编排侧给出。
type PreAdviceOnBehalf interface {
	Accept(ctx context.Context, command application.AcceptPreAdviceCommand) (application.AcceptPreAdviceResult, error)
	UpdateNotice(ctx context.Context, command application.UpdatePreAdviceNoticeCommand) (application.AcceptPreAdviceResult, error)
	Withdraw(ctx context.Context, command application.WithdrawPreAdviceCommand) (application.AcceptPreAdviceResult, error)
}

var _ PreAdviceOnBehalf = (*application.AcceptPreAdviceHandler)(nil)

type preAdviceAnswer struct {
	Outcome   string               `json:"outcome"`
	Reason    string               `json:"reason,omitempty"`
	Status    string               `json:"status,omitempty"`
	Issues    []preAdviceIssueBody `json:"issues,omitempty"`
	PreAdvice *preAdviceBody       `json:"preAdvice,omitempty"`
}

// NewAcceptPreAdviceOnBehalfEndpoint 交回代录提交的入口（POST /warehouse-fulfillment-inbound-pre-advice-on-behalf）。
// 新建答 201，其余业务答复答 200。货主客户账户在本租户查不到答 404，不带 outcome。
func NewAcceptPreAdviceOnBehalfEndpoint(intake PreAdviceOnBehalfIntake, preAdvices PreAdviceOnBehalf) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodPost) {
			return
		}
		command, err := intake.IntakePreAdvice(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := preAdvices.Accept(request.Context(), command)
		if errors.Is(err, application.ErrOwnerAccountNotInTenant) {
			writeProblem(response, http.StatusNotFound, codeOwnerAccountNotVisible)
			return
		}
		if err != nil {
			// 失败只记货主与仓。依据、线索、参考号与错误正文不进日志。
			slog.ErrorContext(request.Context(), "inbound pre-advice recorded on behalf did not form an answer",
				"ownerAccount", command.Owner.String(), "warehouseCode", command.Warehouse.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writePreAdviceAnswer(response, result)
	})
}

// NewUpdatePreAdviceNoticeOnBehalfEndpoint 交回代录告知更新的入口（POST /warehouse-fulfillment-inbound-pre-advice-notice-on-behalf）。
// 查不到预报答 200 PRE_ADVICE_NOT_FOUND。这一口不问货主客户账户在不在租户。
func NewUpdatePreAdviceNoticeOnBehalfEndpoint(intake PreAdviceOnBehalfIntake, preAdvices PreAdviceOnBehalf) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodPost) {
			return
		}
		command, err := intake.IntakePreAdviceNotice(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := preAdvices.UpdateNotice(request.Context(), command)
		if err != nil {
			// 告知更新的命令不带仓。失败只记预报标识，不回册补仓，也不记依据、线索、参考号与错误正文。
			slog.ErrorContext(request.Context(), "inbound pre-advice notice recorded on behalf did not form an answer",
				"preAdviceId", command.ID.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writePreAdviceAnswer(response, result)
	})
}

// NewWithdrawPreAdviceOnBehalfEndpoint 交回代录撤回的入口（POST /warehouse-fulfillment-inbound-pre-advice-withdrawal-on-behalf）。
// 查不到预报答 200 PRE_ADVICE_NOT_FOUND。这一口不问货主客户账户在不在租户。
func NewWithdrawPreAdviceOnBehalfEndpoint(intake PreAdviceOnBehalfIntake, preAdvices PreAdviceOnBehalf) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodPost) {
			return
		}
		command, err := intake.IntakePreAdviceWithdrawal(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := preAdvices.Withdraw(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "inbound pre-advice withdrawal recorded on behalf did not form an answer",
				"preAdviceId", command.ID.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writePreAdviceAnswer(response, result)
	})
}

func writePreAdviceAnswer(response http.ResponseWriter, result application.AcceptPreAdviceResult) {
	name := result.Outcome().String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := preAdviceAnswer{Outcome: name}
	status := http.StatusOK
	switch result.Outcome() {
	case application.PreAdviceOutcomeAdvised, application.PreAdviceOutcomeNoticeUpdated,
		application.PreAdviceOutcomeCancelled, application.PreAdviceOutcomeExistingResult:
		advice, present := result.PreAdvice()
		if !present {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		body := preAdviceBodyOf(advice, true)
		answer.PreAdvice = &body
		if result.Outcome() == application.PreAdviceOutcomeAdvised {
			status = http.StatusCreated
		}
	case application.PreAdviceOutcomeWarehouseNotAccepting:
		reason, present := result.NotAcceptingReason()
		if !present || reason.String() == "" {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		answer.Reason = reason.String()
	case application.PreAdviceOutcomeContentInvalid:
		issues := result.Issues()
		answer.Issues = make([]preAdviceIssueBody, 0, len(issues))
		for _, issue := range issues {
			answer.Issues = append(answer.Issues, preAdviceIssueBody{
				Kind: issue.Kind().String(), Line: issue.Line(), Carton: issue.Carton(),
			})
		}
	case application.PreAdviceOutcomeStatusMismatch, application.PreAdviceOutcomeReceiptStarted:
		echoed, present := result.Status()
		if !present || echoed.String() == "" {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		answer.Status = echoed.String()
	}
	writeJSON(response, status, answer)
}
