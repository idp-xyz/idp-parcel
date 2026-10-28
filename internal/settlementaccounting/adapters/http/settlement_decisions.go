package settlementhttp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// 运营确认口与截单发布口（ADR-0339 决定一、二）：操作者渠道「运营决定」能力面上的两种结算决定。两口进的都是 parcel-api 已装配的
// 结算编排（ADR-0170），先过结算时点那一道门，没登记答未配置、不调编排。

const (
	// OperatorDecisionChargeConfirmation 是点名一笔客户费用补拍确认。
	OperatorDecisionChargeConfirmation OperatorDecision = "CHARGE_CONFIRMATION"
	// OperatorDecisionStatementPublication 是截单并发布一期客户对账单。
	OperatorDecisionStatementPublication OperatorDecision = "STATEMENT_PUBLICATION"
)

// ChargeConfirmationIntake 是运营确认口的 Intake。
type ChargeConfirmationIntake interface {
	IntakeChargeConfirmation(ctx context.Context, request *http.Request) (application.ConfirmChargeCommand, error)
}

// StatementPublicationIntake 是截单发布口的 Intake。at 是这次发布落下的时刻，由端点的时钟给。
type StatementPublicationIntake interface {
	IntakeStatementPublication(ctx context.Context, request *http.Request, at time.Time) (application.PublishStatementCommand, error)
}

// ChargeConfirmer 是运营确认口转交的那一口：先问时点、再确认（parcel-api 的 settlementOrchestrations.Confirm）。
type ChargeConfirmer interface {
	Confirm(ctx context.Context, command application.ConfirmChargeCommand) (
		application.SettlementMomentOutcome, application.ConfirmChargeResult, error)
}

// StatementPublisher 是截单发布口转交的那一口（parcel-api 的 settlementOrchestrations.CutOff）。
type StatementPublisher interface {
	CutOff(ctx context.Context, command application.PublishStatementCommand) (
		application.SettlementMomentOutcome, application.StatementResult, error)
}

var (
	_ ChargeConfirmationIntake   = (*OperatorDecisionIntake)(nil)
	_ StatementPublicationIntake = (*OperatorDecisionIntake)(nil)
)

func (intake *OperatorDecisionIntake) IntakeChargeConfirmation(
	ctx context.Context,
	request *http.Request,
) (application.ConfirmChargeCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionChargeConfirmation)
	if err != nil {
		return application.ConfirmChargeCommand{}, err
	}
	payload, err := readSinglePayload(io.LimitReader(request.Body, maxManualRegistrationBytes+1))
	if err != nil {
		return application.ConfirmChargeCommand{}, err
	}
	command, err := registrationjson.ChargeConfirmationFromJSONForTenant(payload, operator.Tenant)
	if err != nil {
		return application.ConfirmChargeCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return command, nil
}

func (intake *OperatorDecisionIntake) IntakeStatementPublication(
	ctx context.Context,
	request *http.Request,
	at time.Time,
) (application.PublishStatementCommand, error) {
	operator, err := intake.authenticator.AuthenticateOperatorDecision(ctx, httpapi.BearerToken(request), OperatorDecisionStatementPublication)
	if err != nil {
		return application.PublishStatementCommand{}, err
	}
	payload, err := readSinglePayload(io.LimitReader(request.Body, maxManualRegistrationBytes+1))
	if err != nil {
		return application.PublishStatementCommand{}, err
	}
	command, err := registrationjson.StatementPublicationFromJSONForTenant(payload, operator.Tenant, at)
	if err != nil {
		return application.PublishStatementCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return command, nil
}

// NewConfirmChargeEndpoint 交回运营确认口。
func NewConfirmChargeEndpoint(intake ChargeConfirmationIntake, confirmer ChargeConfirmer) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeChargeConfirmation(request.Context(), request)
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		moment, result, err := confirmer.Confirm(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeConfirmationAnswer(response, command.ChargeID, moment, result)
	})
}

// NewPublishStatementEndpoint 交回截单发布口。now 给发布时刻。
func NewPublishStatementEndpoint(intake StatementPublicationIntake, publisher StatementPublisher, now func() time.Time) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			writeProblem(response, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeStatementPublication(request.Context(), request, now())
		if err != nil {
			writeCatalogueIntakeProblem(response, err)
			return
		}
		moment, result, err := publisher.CutOff(request.Context(), command)
		if err != nil {
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writePublicationAnswer(response, moment, result)
	})
}

type confirmationResponse struct {
	Outcome               string `json:"outcome"`
	Charge                string `json:"charge,omitempty"`
	UndecidedReason       string `json:"undecidedReason,omitempty"`
	ContinuationReference string `json:"continuationReference,omitempty"`
}

// writeConfirmationAnswer 与本包其余写口同一条判据（writeFundsAnswer 头注）：状态码只答有没有形成答案，业务判别在 outcome。
// 时点门没过时 outcome 就是时点门的答案，编排没被调；时点册读不动、确认停在依赖上、确认信封没交出去都是没形成答案。确认条件
// 或确认事实没登是租户取值缺席，那是答案（200，带未决原因与续办引用，补登之后再来）。
func writeConfirmationAnswer(
	response http.ResponseWriter,
	charge string,
	moment application.SettlementMomentOutcome,
	result application.ConfirmChargeResult,
) {
	switch moment {
	case application.SettlementMomentAdmitted:
	case application.SettlementMomentUnconfigured, application.SettlementMomentNotAccepted:
		writeJSON(response, http.StatusOK, confirmationResponse{Outcome: moment.String(), Charge: charge})
		return
	default:
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	name := result.Outcome().String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	if result.Outcome() == application.ConfirmUndecided && !businessUndecidedConfirmation(result.UndecidedReason()) {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	if result.ConfirmationHandoffReference() != "" {
		writeProblem(response, http.StatusInternalServerError, codeHandoffNotSent)
		return
	}
	status := http.StatusOK
	if result.Outcome() == application.ChargeConfirmedOutcome {
		status = http.StatusCreated
	}
	writeJSON(response, status, confirmationResponse{
		Outcome: name, Charge: charge,
		UndecidedReason: result.UndecidedReason().String(), ContinuationReference: result.ContinuationReference(),
	})
}

// businessUndecidedConfirmation 点名确认里「形成了答案」的未决原因：确认条件目录或确认事实册没登。其余都是依赖读不动。
func businessUndecidedConfirmation(reason application.ConfirmUndecidedReason) bool {
	return reason == application.ConditionUnconfigured || reason == application.ConfirmationFactsUnconfigured
}

type publicationResponse struct {
	Outcome               string                   `json:"outcome"`
	Statement             string                   `json:"statement,omitempty"`
	Total                 string                   `json:"total,omitempty"`
	Lines                 []string                 `json:"lines,omitempty"`
	Exclusions            []statementExclusionBody `json:"exclusions,omitempty"`
	ContinuationReference string                   `json:"continuationReference,omitempty"`
}

type statementExclusionBody struct {
	Charge          string   `json:"charge"`
	Cause           string   `json:"cause"`
	AttributionDate string   `json:"attributionDate,omitempty"`
	BilledOn        string   `json:"billedOn,omitempty"`
	Missing         []string `json:"missing,omitempty"`
}

// writePublicationAnswer 判据同 writeConfirmationAnswer。截单的未决原因全是册读不动，没有业务那一格；截单未配置是另一个具名格
// （STATEMENT_CUT_UNCONFIGURED），带逐件没入本期的缘由与缺哪一册，照答 200。发布了的给单号、合计与入本期的费用，单子全貌在
// /settlement-statements 读口上。
func writePublicationAnswer(
	response http.ResponseWriter,
	moment application.SettlementMomentOutcome,
	result application.StatementResult,
) {
	switch moment {
	case application.SettlementMomentAdmitted:
	case application.SettlementMomentUnconfigured, application.SettlementMomentNotAccepted:
		writeJSON(response, http.StatusOK, publicationResponse{Outcome: moment.String()})
		return
	default:
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	name := result.Outcome().String()
	if name == "" {
		writeProblem(response, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	if result.Outcome() == application.StatementUndecided {
		writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
		return
	}
	if result.StatementHandoffReference() != "" {
		writeProblem(response, http.StatusInternalServerError, codeHandoffNotSent)
		return
	}
	body := publicationResponse{Outcome: name, ContinuationReference: result.ContinuationReference()}
	if record, has := result.Statement(); has {
		statement := record.Statement
		body.Statement = statement.Number().String()
		body.Total = minorAmount(statement.TotalMinor())
		for _, line := range statement.Lines() {
			body.Lines = append(body.Lines, line.Charge.String())
		}
	}
	for _, exclusion := range result.Exclusions() {
		entry := statementExclusionBody{
			Charge: exclusion.Charge.String(), Cause: exclusion.Cause.String(), BilledOn: exclusion.BilledOn.String(),
		}
		// 零值归属日印出来是 0000-00-00，不是「没有」。
		if exclusion.AttributionDate.Valid() {
			entry.AttributionDate = exclusion.AttributionDate.String()
		}
		for _, register := range exclusion.Missing {
			entry.Missing = append(entry.Missing, register.String())
		}
		body.Exclusions = append(body.Exclusions, entry)
	}
	status := http.StatusOK
	if result.Outcome() == application.StatementPublished {
		status = http.StatusCreated
	}
	writeJSON(response, status, body)
}
