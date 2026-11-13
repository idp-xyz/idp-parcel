package visibilityhttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
)

// 本文件是异常案件的三个运营决定口（UC-VE-005 的案件生命周期半边）：接单、以明确结论关闭、把重复案件并进主案件。

// CaseManager 是三口转交的编排（application.ManageCaseHandler）。
type CaseManager interface {
	TakeUp(ctx context.Context, command application.TakeUpCaseCommand) (application.ManageCaseResult, error)
	Close(ctx context.Context, command application.CloseCaseCommand) (application.ManageCaseResult, error)
	Merge(ctx context.Context, command application.MergeCaseCommand) (application.ManageCaseResult, error)
}

type CaseDecisionIntake interface {
	IntakeCaseTakeUp(ctx context.Context, request *http.Request) (application.TakeUpCaseCommand, error)
	IntakeCaseClosure(ctx context.Context, request *http.Request) (application.CloseCaseCommand, error)
	IntakeCaseMerge(ctx context.Context, request *http.Request) (application.MergeCaseCommand, error)
}

var _ CaseDecisionIntake = (*OperatorDecisionIntake)(nil)

func NewTakeUpExceptionCaseEndpoint(intake CaseDecisionIntake, manager CaseManager) http.Handler {
	return caseDecisionEndpoint(intake.IntakeCaseTakeUp, manager.TakeUp)
}

func NewCloseExceptionCaseEndpoint(intake CaseDecisionIntake, manager CaseManager) http.Handler {
	return caseDecisionEndpoint(intake.IntakeCaseClosure, manager.Close)
}

func NewMergeExceptionCaseEndpoint(intake CaseDecisionIntake, manager CaseManager) http.Handler {
	return caseDecisionEndpoint(intake.IntakeCaseMerge, manager.Merge)
}

func caseDecisionEndpoint[C any](
	intake func(context.Context, *http.Request) (C, error),
	handle func(context.Context, C) (application.ManageCaseResult, error),
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake(request.Context(), request)
		if err != nil {
			writeCaseDecisionIntakeProblem(response, err)
			return
		}
		result, err := handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeCaseDecision(response, result)
	})
}

// writeCaseDecisionIntakeProblem 把 Intake 的各格映成状态码：未配置、未授予与不在准入范围 403，令牌不过 401，坏载荷 400，
// 身份依赖读不动 503，其余是 Intake 故障。
func writeCaseDecisionIntakeProblem(response http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrAccessChannelNotConfigured):
		writeProblem(response, http.StatusForbidden, codeAccessChannelNotConfigured)
	case errors.Is(err, ErrOperatorCredentialRejected):
		writeProblem(response, http.StatusUnauthorized, codeOperatorCredentialRejected)
	case errors.Is(err, ErrOperatorNotGranted):
		writeProblem(response, http.StatusForbidden, codeOperatorNotGranted)
	case errors.Is(err, ErrOutsideAdmissionScope):
		writeProblem(response, http.StatusForbidden, codeOutsideAdmissionScope)
	case errors.Is(err, ErrIdentityDependencyUnavailable):
		writeProblem(response, http.StatusServiceUnavailable, codeIdentityDependencyUnavailable)
	case errors.Is(err, ErrMalformedRequest):
		writeProblem(response, http.StatusBadRequest, codeMalformedRequest)
	default:
		writeProblem(response, http.StatusInternalServerError, codeIntakeFailed)
	}
}

// caseDecisionResponse 是三口的封闭响应形状。案件的现状只在答复带着案件时交回：形成了推进，或案件已在这次决定想去的
// 那一格之后；不在册与主案件不合格不带任何案件内容。
type caseDecisionResponse struct {
	Outcome         string `json:"outcome"`
	CaseID          string `json:"caseId,omitempty"`
	Phase           string `json:"phase,omitempty"`
	FirstResponseAt string `json:"firstResponseAt,omitempty"`
	MergedInto      string `json:"mergedInto,omitempty"`
}

// writeCaseDecision 逐名转写。接单、关闭或归并真落了才 201；其余业务答案 200。未决只在案件册读写不动时出现，是没形成
// 答案——答 5xx，不让操作者以为这次决定已经有了着落。
func writeCaseDecision(response http.ResponseWriter, result application.ManageCaseResult) {
	if result.Outcome() == application.CaseUndecided {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := caseDecisionResponse{Outcome: outcome}
	if exceptionCase, present := result.Case(); present {
		body.CaseID = exceptionCase.ID().String()
		body.Phase = exceptionCase.Phase().String()
		if at := exceptionCase.FirstResponseAt(); !at.IsZero() {
			body.FirstResponseAt = at.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		if main, merged := exceptionCase.MergedInto(); merged {
			body.MergedInto = main.String()
		}
	}
	status := http.StatusOK
	switch result.Outcome() {
	case application.CaseTakenUp, application.CaseClosedWithConclusion, application.CaseMerged:
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
