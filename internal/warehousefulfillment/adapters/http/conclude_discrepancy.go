package warehousehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// DiscrepancyConclusionAuthenticator 认证代录结论。先授予与准入，后读请求体。
type DiscrepancyConclusionAuthenticator interface {
	AuthenticateDiscrepancyConclusion(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

// CustomerDiscrepancyConclusionAuthenticator 认证货主结论口。accessRequestID 在令牌、册、授予与准入都过了之后才被调用。
type CustomerDiscrepancyConclusionAuthenticator interface {
	AuthenticateReceiptDiscrepancyConclusion(
		ctx context.Context,
		credential PresentedCustomerCredential,
		accessRequestID func() (string, error),
	) (CustomerCommandIdentity, error)
}

// DiscrepancyConclusionIntake 是代录口要的 Intake。
type DiscrepancyConclusionIntake interface {
	IntakeDiscrepancyConclusion(ctx context.Context, request *http.Request) (application.ConcludeDiscrepancyCommand, error)
}

// CustomerDiscrepancyConclusionIntake 是货主口要的 Intake。
type CustomerDiscrepancyConclusionPorts interface {
	IntakeReceiptDiscrepancyConclusion(ctx context.Context, request *http.Request) (application.ConcludeDiscrepancyCommand, error)
}

// DiscrepancyConcluder 是两口共用的编排。
type DiscrepancyConcluder interface {
	Conclude(ctx context.Context, command application.ConcludeDiscrepancyCommand) (application.ConcludeDiscrepancyResult, error)
}

// OperatorDiscrepancyConclusionIntake 是代录结论的运营决定 Intake。
type OperatorDiscrepancyConclusionIntake struct {
	authenticator DiscrepancyConclusionAuthenticator
}

func NewOperatorDiscrepancyConclusionIntake(authenticator DiscrepancyConclusionAuthenticator) (*OperatorDiscrepancyConclusionIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: receipt discrepancy conclusion intake needs an authenticator")
	}
	return &OperatorDiscrepancyConclusionIntake{authenticator: authenticator}, nil
}

func (intake *OperatorDiscrepancyConclusionIntake) IntakeDiscrepancyConclusion(
	ctx context.Context,
	request *http.Request,
) (application.ConcludeDiscrepancyCommand, error) {
	identity, err := intake.authenticator.AuthenticateDiscrepancyConclusion(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.ConcludeDiscrepancyCommand{}, err
	}
	command, err := registrationjson.OnBehalfDiscrepancyConclusionCommand(requestBody(request), identity.Tenant, identity.Registrant)
	if err != nil {
		return application.ConcludeDiscrepancyCommand{}, asMalformed(err)
	}
	return command, nil
}

var _ DiscrepancyConclusionIntake = (*OperatorDiscrepancyConclusionIntake)(nil)

func (UnconfiguredIntake) IntakeDiscrepancyConclusion(
	context.Context, *http.Request,
) (application.ConcludeDiscrepancyCommand, error) {
	return application.ConcludeDiscrepancyCommand{}, ErrAccessChannelNotConfigured
}

// CustomerDiscrepancyConclusionIntake 是货主结论口的首方 Intake。问题详情用 application/problem+json。
type CustomerDiscrepancyConclusionIntake struct {
	authenticator CustomerDiscrepancyConclusionAuthenticator
}

func NewCustomerDiscrepancyConclusionIntake(authenticator CustomerDiscrepancyConclusionAuthenticator) (*CustomerDiscrepancyConclusionIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: receipt discrepancy conclusion customer intake needs an authenticator")
	}
	return &CustomerDiscrepancyConclusionIntake{authenticator: authenticator}, nil
}

func OpenCustomerDiscrepancyConclusionIntake(intake *CustomerDiscrepancyConclusionIntake) CustomerDiscrepancyConclusionPorts {
	if intake == nil {
		return unconfiguredDiscrepancyConclusion{}
	}
	return intake
}

type unconfiguredDiscrepancyConclusion struct{}

func (unconfiguredDiscrepancyConclusion) IntakeReceiptDiscrepancyConclusion(
	context.Context, *http.Request,
) (application.ConcludeDiscrepancyCommand, error) {
	return application.ConcludeDiscrepancyCommand{}, ErrAccessChannelNotConfigured
}

func (intake *CustomerDiscrepancyConclusionIntake) IntakeReceiptDiscrepancyConclusion(
	ctx context.Context,
	request *http.Request,
) (application.ConcludeDiscrepancyCommand, error) {
	var fields registrationjson.OwnerDiscrepancyConclusionFields
	identity, err := intake.authenticator.AuthenticateReceiptDiscrepancyConclusion(ctx, presentedCustomerCredential(request), func() (string, error) {
		read, readErr := registrationjson.ReadOwnerDiscrepancyConclusion(bodyOrEmpty(request))
		if readErr != nil {
			return "", asMalformed(readErr)
		}
		fields = read
		return read.AccessRequestID, nil
	})
	if err != nil {
		return application.ConcludeDiscrepancyCommand{}, err
	}
	return application.ConcludeDiscrepancyCommand{
		Tenant: identity.Tenant, Owner: identity.Owner, Discrepancy: fields.Discrepancy, Verdict: fields.Verdict,
		Note: fields.Note, Path: domain.SubmissionByOwner, RequestKey: identity.RequestKey,
	}, nil
}

var _ CustomerDiscrepancyConclusionPorts = (*CustomerDiscrepancyConclusionIntake)(nil)
var _ DiscrepancyConcluder = (*application.ConcludeDiscrepancyHandler)(nil)

// NewConcludeDiscrepancyOnBehalfEndpoint 交回代录口（POST /warehouse-fulfillment-receipt-discrepancy-conclusion-on-behalf）。
// 记下结论答 201，其余业务结果答 200。问题体用运营决定口已有的形状。依据不进日志。
func NewConcludeDiscrepancyOnBehalfEndpoint(intake DiscrepancyConclusionIntake, concluder DiscrepancyConcluder) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseMethod(response, request, http.MethodPost) {
			return
		}
		command, err := intake.IntakeDiscrepancyConclusion(request.Context(), request)
		if err != nil {
			writeIntakeProblem(response, err)
			return
		}
		result, err := concluder.Conclude(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "receipt discrepancy conclusion did not form an answer", "discrepancyId", command.Discrepancy.String())
			writeProblem(response, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeConclusionAnswer(response, request, result, true, false)
	})
}

// NewConcludeDiscrepancyEndpoint 交回货主口（POST /v1/warehouse/receipt-discrepancy-conclusions）。
// 问题详情用首方族的 application/problem+json。记下结论答 201，其余业务结果答 200。回显不带录入的操作者。
func NewConcludeDiscrepancyEndpoint(intake CustomerDiscrepancyConclusionPorts, concluder DiscrepancyConcluder) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseCustomerMethod(response, request, http.MethodPost) {
			return
		}
		command, err := intake.IntakeReceiptDiscrepancyConclusion(request.Context(), request)
		if err != nil {
			answerCustomerIntake(response, request, err)
			return
		}
		result, err := concluder.Conclude(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "receipt discrepancy conclusion did not form an answer", "discrepancyId", command.Discrepancy.String())
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		writeConclusionAnswer(response, request, result, false, true)
	})
}

func writeConclusionAnswer(
	response http.ResponseWriter,
	request *http.Request,
	result application.ConcludeDiscrepancyResult,
	withOperator bool,
	customer bool,
) {
	name := result.Outcome().String()
	if name == "" {
		writeConclusionFailure(response, request, codeUnnamedOutcome, customer)
		return
	}
	answer := concludeDiscrepancyAnswer{Outcome: name}
	status := http.StatusOK
	switch result.Outcome() {
	case application.ConcludeDiscrepancyRecorded,
		application.ConcludeDiscrepancyExistingResult,
		application.ConcludeDiscrepancyHeld:
		current, present := result.Current()
		if !present {
			writeConclusionFailure(response, request, codeNoAnswerFormed, customer)
			return
		}
		body := discrepancyConclusionBodyOf(current, withOperator)
		answer.Current = &body
		if result.Outcome() == application.ConcludeDiscrepancyRecorded {
			status = http.StatusCreated
		}
	}
	writeJSON(response, status, answer)
}

func writeConclusionFailure(response http.ResponseWriter, request *http.Request, code string, customer bool) {
	if customer {
		writeCustomerProblem(response, request, http.StatusInternalServerError, code, "")
		return
	}
	writeProblem(response, http.StatusInternalServerError, code)
}

type concludeDiscrepancyAnswer struct {
	Outcome string                     `json:"outcome"`
	Current *discrepancyConclusionBody `json:"current,omitempty"`
}
