package settlementhttp

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 登记争议口与处理争议口（ADR-0354 决定一、二）：操作者渠道「运营决定」能力面上的两种客户对账单争议决定，进的是 parcel-api 已装配的
// 截单编排上的 OpenDispute 与 ResolveDispute（ADR-0170 决定一）。争议不过结算时点那一道门：门管的是确认与截单什么时候能动，客户
// 什么时候提出异议不由它定。提交操作者记成操作证据，只进审计（决定四）。

const (
	// OperatorDecisionStatementDisputeRegistration 是照客户来电或来信代登记一项争议。
	OperatorDecisionStatementDisputeRegistration OperatorDecision = "STATEMENT_DISPUTE_REGISTRATION"
	// OperatorDecisionStatementDisputeResolution 是核实后对一项争议形成处理。
	OperatorDecisionStatementDisputeResolution OperatorDecision = "STATEMENT_DISPUTE_RESOLUTION"
)

// StatementDisputeRegistrationIntake 是登记争议口的 Intake。
type StatementDisputeRegistrationIntake interface {
	IntakeStatementDisputeRegistration(ctx context.Context, request *http.Request) (application.OpenDisputeCommand, error)
}

// StatementDisputeResolutionIntake 是处理争议口的 Intake。
type StatementDisputeResolutionIntake interface {
	IntakeStatementDisputeResolution(ctx context.Context, request *http.Request) (application.ResolveDisputeCommand, error)
}

// StatementDisputeOpener 是登记争议口转交的那一口（parcel-api 的 settlementOrchestrations.OpenDispute，包一笔事务）。
type StatementDisputeOpener interface {
	OpenDispute(ctx context.Context, command application.OpenDisputeCommand) (application.StatementResult, error)
}

// StatementDisputeResolver 是处理争议口转交的那一口（settlementOrchestrations.ResolveDispute）。
type StatementDisputeResolver interface {
	ResolveDispute(ctx context.Context, command application.ResolveDisputeCommand) (application.StatementResult, error)
}

var (
	_ StatementDisputeRegistrationIntake = (*OperatorDecisionIntake)(nil)
	_ StatementDisputeResolutionIntake   = (*OperatorDecisionIntake)(nil)
)

func (intake *OperatorDecisionIntake) IntakeStatementDisputeRegistration(
	ctx context.Context,
	request *http.Request,
) (application.OpenDisputeCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionStatementDisputeRegistration)
	if err != nil {
		return application.OpenDisputeCommand{}, err
	}
	payload, err := readSinglePayload(io.LimitReader(request.Body, maxManualRegistrationBytes+1))
	if err != nil {
		return application.OpenDisputeCommand{}, err
	}
	command, err := registrationjson.StatementDisputeFromJSONForTenant(payload, operator.Tenant)
	if err != nil {
		return application.OpenDisputeCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	command.OperationEvidence = operatorEvidence(operator)
	return command, nil
}

func (intake *OperatorDecisionIntake) IntakeStatementDisputeResolution(
	ctx context.Context,
	request *http.Request,
) (application.ResolveDisputeCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionStatementDisputeResolution)
	if err != nil {
		return application.ResolveDisputeCommand{}, err
	}
	payload, err := readSinglePayload(io.LimitReader(request.Body, maxManualRegistrationBytes+1))
	if err != nil {
		return application.ResolveDisputeCommand{}, err
	}
	command, err := registrationjson.DisputeResolutionFromJSONForTenant(payload, operator.Tenant)
	if err != nil {
		return application.ResolveDisputeCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	command.OperationEvidence = operatorEvidence(operator)
	return command, nil
}

// NewRegisterStatementDisputeEndpoint 交回登记争议口（POST /settlement-statement-disputes）。
func NewRegisterStatementDisputeEndpoint(intake StatementDisputeRegistrationIntake, opener StatementDisputeOpener) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeStatementDisputeRegistration(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		result, err := opener.OpenDispute(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeDisputeAnswer(response, result, application.DisputeOpened)
	})
}

// NewResolveStatementDisputeEndpoint 交回处理争议口（POST /settlement-statement-dispute-resolutions）。
func NewResolveStatementDisputeEndpoint(intake StatementDisputeResolutionIntake, resolver StatementDisputeResolver) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeStatementDisputeResolution(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		result, err := resolver.ResolveDispute(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeDisputeAnswer(response, result, application.DisputeResolvedOutcome)
	})
}

// disputeResponse 透出答复连同那一项争议此刻的样子：重放、冲突与「已经处理过了」都带上库里那一份，操作者看得到自己撞上的是什么。
// 处理三件同在或同缺，未处理时不出现，不代填「待处理」。
//
// documentNumber 是争议号，登记时由服务端按编号规则取（ADR-0355）；没有争议号时不出这一格（不答空串、不拿争议标识顶），
// 改出 documentNumberAbsence 说为什么没有。documentType 只在撞号时出现，指名规则册上该改哪一类单据的编号规则。
type disputeResponse struct {
	Outcome               string `json:"outcome"`
	Dispute               string `json:"dispute,omitempty"`
	DocumentNumber        string `json:"documentNumber,omitempty"`
	DocumentNumberAbsence string `json:"documentNumberAbsence,omitempty"`
	DocumentType          string `json:"documentType,omitempty"`
	Statement             string `json:"statement,omitempty"`
	Charge                string `json:"charge,omitempty"`
	DisputedAmount        string `json:"disputedAmount,omitempty"`
	Reason                string `json:"reason,omitempty"`
	OpenedAt              string `json:"openedAt,omitempty"`
	Resolution            string `json:"resolution,omitempty"`
	Basis                 string `json:"basis,omitempty"`
	ResolvedAt            string `json:"resolvedAt,omitempty"`
}

// writeDisputeAnswer 与本包其余写口同一条判据（writeFundsAnswer 头注）：状态码只答有没有形成答案，业务判别在 outcome。formed 是
// 这一口「这次落了新东西」的那一格，答 201；重放、冲突、已处理、未受理、单已作废都是答案，答 200；册读不动是没形成答案。
func writeDisputeAnswer(response http.ResponseWriter, result application.StatementResult, formed application.StatementOutcome) {
	name := result.Outcome().String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	if result.Outcome() == application.StatementUndecided {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	body := disputeResponse{Outcome: name}
	if result.Outcome() == application.StatementNumberingRuleCollision {
		body.DocumentType = string(domain.StatementDisputeDocumentNumberType.Code)
	}
	if record, has := result.Dispute(); has {
		fillDispute(&body, record)
	}
	status := http.StatusOK
	if result.Outcome() == formed {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}

func fillDispute(body *disputeResponse, record ports.DisputeRecord) {
	dispute := record.Dispute
	body.Dispute = dispute.Dispute().String()
	body.DocumentNumber, body.DocumentNumberAbsence = numberingWords(record.Numbering)
	body.Statement = dispute.Statement().String()
	body.Charge = dispute.Charge().String()
	body.DisputedAmount = minorAmount(dispute.DisputedMinor())
	body.Reason = dispute.Reason().String()
	body.OpenedAt = rfc3339(dispute.OpenedAt())
	if kind, basis, resolvedAt, resolved := dispute.Resolution(); resolved {
		body.Resolution = kind.String()
		body.Basis = basis.String()
		body.ResolvedAt = rfc3339(resolvedAt)
	}
}
