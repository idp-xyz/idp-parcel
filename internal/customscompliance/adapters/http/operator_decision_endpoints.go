package customshttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// CaseEstablisher 是建案口转交的编排（application.EstablishCaseForParcelsHandler）。
type CaseEstablisher interface {
	Handle(ctx context.Context, command application.EstablishCaseForParcelsCommand) (application.EstablishCaseResult, error)
}

// SubmissionAuthorizationConfirmer 是授权确认口转交的编排（application.ConfirmSubmissionAuthorizationHandler）。
type SubmissionAuthorizationConfirmer interface {
	Handle(ctx context.Context, command application.ConfirmSubmissionAuthorizationCommand) (application.AuthorizationConfirmationResult, error)
}

// DeclarationSubmitter 是提交执行口转交的编排（application.SubmitDeclarationHandler）。
type DeclarationSubmitter interface {
	Handle(ctx context.Context, command application.SubmitDeclarationCommand) (application.SubmitDeclarationResult, error)
}

// NewEstablishCaseEndpoint 交回建案口（`POST /customs-cases`，ADR-0227 决定二）。
func NewEstablishCaseEndpoint(intake CaseEstablishmentIntake, handler CaseEstablisher) http.Handler {
	return decisionEndpoint(intake.IntakeCaseEstablishment, handler.Handle, writeCaseEstablishment)
}

// NewConfirmSubmissionAuthorizationEndpoint 交回授权确认口（`POST /customs-submission-authorizations`，ADR-0227 决定三）。
func NewConfirmSubmissionAuthorizationEndpoint(intake SubmissionAuthorizationIntake, handler SubmissionAuthorizationConfirmer) http.Handler {
	return decisionEndpoint(intake.IntakeSubmissionAuthorization, handler.Handle, writeAuthorizationConfirmation)
}

// NewSubmitDeclarationEndpoint 交回提交执行口（`POST /customs-declaration-submissions`，ADR-0227 决定四）。
func NewSubmitDeclarationEndpoint(intake DeclarationSubmissionIntake, handler DeclarationSubmitter) http.Handler {
	return decisionEndpoint(intake.IntakeDeclarationSubmission, handler.Handle, writeDeclarationSubmission)
}

// decisionEndpoint 是三口共用的那一段：只收 POST，Intake 的错按渠道各格答，编排交回 error 就是没形成答案（500），其余由各口
// 自己的写法把业务答案逐名转写进响应体（ADR-0022）。
func decisionEndpoint[C, R any](
	intake func(context.Context, *http.Request) (C, error),
	handle func(context.Context, C) (R, error),
	write func(http.ResponseWriter, R),
) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake(request.Context(), request)
		if err != nil {
			writeRegistrationIntakeProblem(response, err)
			return
		}
		result, err := handle(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		write(response, result)
	})
}

// caseEstablishmentResponse 是建案口的封闭响应形状。未决原因、未接受包裹、不适用依据与续办引用各只在对应格出现。
type caseEstablishmentResponse struct {
	Outcome           string   `json:"outcome"`
	CaseID            string   `json:"caseId,omitempty"`
	Basis             string   `json:"basis,omitempty"`
	UndecidedReason   string   `json:"undecidedReason,omitempty"`
	UnacceptedParcels []string `json:"unacceptedParcels,omitempty"`
	HandoffReference  string   `json:"handoffReference,omitempty"`
}

// writeCaseEstablishment 逐名转写。未决也是形成了的答案（200 带原因）：规则未配置要租户去登，依赖读不动要运维去救，响应体
// 里的原因分得开在等谁——与外部结果接收口同一写法。
func writeCaseEstablishment(response http.ResponseWriter, result application.EstablishCaseResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := caseEstablishmentResponse{
		Outcome:           outcome,
		Basis:             result.Basis(),
		UndecidedReason:   result.UndecidedReason().String(),
		UnacceptedParcels: result.UnacceptedParcels(),
		HandoffReference:  result.HandoffReference(),
	}
	if customsCase, present := result.Case(); present {
		body.CaseID = customsCase.ID().String()
	}
	status := http.StatusOK
	if result.Outcome() == application.CaseEstablished {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

// authorizationConfirmationResponse 是授权确认口的封闭响应形状：在册那一份授权的依据（「所凭授予@确认人」）与授予时刻，
// 不再有效时另带失效原因与时刻——已有授权可能是另一位确认人确认的，依据照册上原样回。
type authorizationConfirmationResponse struct {
	Outcome         string `json:"outcome"`
	UnitID          string `json:"unitId,omitempty"`
	Authority       string `json:"authority,omitempty"`
	GrantedAt       string `json:"grantedAt,omitempty"`
	RevokedBy       string `json:"revokedBy,omitempty"`
	RevokedAt       string `json:"revokedAt,omitempty"`
	UndecidedReason string `json:"undecidedReason,omitempty"`
}

func writeAuthorizationConfirmation(response http.ResponseWriter, result application.AuthorizationConfirmationResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := authorizationConfirmationResponse{Outcome: outcome, UndecidedReason: result.UndecidedReason().String()}
	if authorization, present := result.Authorization(); present {
		body.UnitID = authorization.Unit().String()
		body.Authority = authorization.Authority().String()
		body.GrantedAt = authorization.GrantedAt().UTC().Format(time.RFC3339Nano)
		if cause, at, revoked := authorization.Revocation(); revoked {
			body.RevokedBy = cause
			body.RevokedAt = at.UTC().Format(time.RFC3339Nano)
		}
	}
	status := http.StatusOK
	if result.Outcome() == application.SubmissionAuthorizationConfirmed {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

// declarationSubmissionResponse 是提交执行口的封闭响应形状：提交版本、它快照下来的授权依据与当前尝试。尝试结果原名过线——
// 经本口形成的首次尝试恒为 REGISTERED_PENDING_SEND，那时没有发送时间可报，这一格就不出现。
type declarationSubmissionResponse struct {
	Outcome               string `json:"outcome"`
	UndecidedReason       string `json:"undecidedReason,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
	HandoffReference      string `json:"handoffReference,omitempty"`
	VersionID             string `json:"versionId,omitempty"`
	Authority             string `json:"authority,omitempty"`
	ReadinessBasis        string `json:"readinessBasis,omitempty"`
	AttemptSequence       int    `json:"attemptSequence,omitempty"`
	AttemptResult         string `json:"attemptResult,omitempty"`
	AttemptTarget         string `json:"attemptTarget,omitempty"`
	AttemptSentAt         string `json:"attemptSentAt,omitempty"`
}

func writeDeclarationSubmission(response http.ResponseWriter, result application.SubmitDeclarationResult) {
	outcome := result.Outcome().String()
	if outcome == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	body := declarationSubmissionResponse{
		Outcome:               outcome,
		UndecidedReason:       result.UndecidedReason().String(),
		ContinuationReference: result.ContinuationReference(),
		HandoffReference:      result.SubmissionHandoffReference(),
	}
	if record, present := result.Record(); present {
		body.VersionID = record.Version.ID().String()
		body.Authority = record.Version.Authority().String()
		body.ReadinessBasis = record.Version.ReadinessBasis().String()
		body.AttemptSequence = record.Attempt.Sequence()
		body.AttemptResult = record.Attempt.Result().String()
		body.AttemptTarget = record.Attempt.Target()
		if sentAt := record.Attempt.SentAt(); !sentAt.IsZero() {
			body.AttemptSentAt = sentAt.UTC().Format(time.RFC3339Nano)
		}
	}
	status := http.StatusOK
	if result.Outcome() == application.DeclarationSubmitted {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
