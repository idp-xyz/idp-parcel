package warehousehttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// OutboundService 是出库请求受理与缺货处置转交的编排。
type OutboundService interface {
	Accept(ctx context.Context, command application.AcceptOutboundCommand) (application.OutboundResult, error)
	RegisterPolicy(ctx context.Context, command application.RegisterShortagePolicyCommand) (application.OutboundResult, error)
	Split(ctx context.Context, command application.SplitOutboundCommand) (application.SplitResult, error)
}

// FollowUpService 是货主补正后代交新提交版本的编排。
type FollowUpService interface {
	Resubmit(ctx context.Context, command application.ResubmitCommand) (application.FollowUpResult, error)
}

// CustomerOutboundAuthenticator 认证货主提交出库请求。
type CustomerOutboundAuthenticator interface {
	AuthenticateOutboundRequestSubmission(ctx context.Context, credential PresentedCustomerCredential, accessRequestID func() (string, error)) (CustomerCommandIdentity, error)
	AuthenticateOutboundRequestSplit(ctx context.Context, credential PresentedCustomerCredential, accessRequestID func() (string, error)) (CustomerCommandIdentity, error)
	AuthenticateOutboundRequestSupplement(ctx context.Context, credential PresentedCustomerCredential, accessRequestID func() (string, error)) (CustomerCommandIdentity, error)
}

// CustomerOutboundPorts 是货主出库口在渠道没配时也能挂上的那一面。
type CustomerOutboundPorts interface {
	IntakeOutboundRequest(ctx context.Context, request *http.Request) (application.AcceptOutboundCommand, error)
	IntakeOutboundSplit(ctx context.Context, request *http.Request) (application.SplitOutboundCommand, error)
	IntakeOutboundSupplement(ctx context.Context, request *http.Request) (application.ResubmitCommand, error)
}

type CustomerOutboundIntake struct {
	authenticator CustomerOutboundAuthenticator
}

func NewCustomerOutboundIntake(authenticator CustomerOutboundAuthenticator) (*CustomerOutboundIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: outbound customer intake needs an authenticator")
	}
	return &CustomerOutboundIntake{authenticator: authenticator}, nil
}

func OpenCustomerOutboundIntake(intake *CustomerOutboundIntake) CustomerOutboundPorts {
	if intake == nil {
		return unconfiguredCustomerOutbound{}
	}
	return intake
}

type unconfiguredCustomerOutbound struct{}

func (unconfiguredCustomerOutbound) IntakeOutboundRequest(context.Context, *http.Request) (application.AcceptOutboundCommand, error) {
	return application.AcceptOutboundCommand{}, ErrAccessChannelNotConfigured
}

func (unconfiguredCustomerOutbound) IntakeOutboundSplit(context.Context, *http.Request) (application.SplitOutboundCommand, error) {
	return application.SplitOutboundCommand{}, ErrAccessChannelNotConfigured
}

func (unconfiguredCustomerOutbound) IntakeOutboundSupplement(context.Context, *http.Request) (application.ResubmitCommand, error) {
	return application.ResubmitCommand{}, ErrAccessChannelNotConfigured
}

func (intake *CustomerOutboundIntake) IntakeOutboundRequest(ctx context.Context, request *http.Request) (application.AcceptOutboundCommand, error) {
	var fields registrationjson.OwnerOutboundFields
	identity, err := intake.authenticator.AuthenticateOutboundRequestSubmission(ctx, presentedCustomerCredential(request), func() (string, error) {
		read, readErr := registrationjson.ReadOwnerOutbound(bodyOrEmpty(request))
		if readErr != nil {
			return "", asMalformed(readErr)
		}
		fields = read
		return read.AccessRequestID, nil
	})
	if err != nil {
		return application.AcceptOutboundCommand{}, err
	}
	return application.AcceptOutboundCommand{
		Tenant: identity.Tenant, Owner: identity.Owner, Warehouse: fields.Warehouse, Origin: domain.OutboundOriginCustomer,
		Key: identity.RequestKey, Dispatch: fields.Dispatch, Service: fields.Service, Recipient: fields.Recipient,
		Address: fields.Address, Lines: fields.Lines,
	}, nil
}

func (intake *CustomerOutboundIntake) IntakeOutboundSplit(ctx context.Context, request *http.Request) (application.SplitOutboundCommand, error) {
	var command application.SplitOutboundCommand
	identity, err := intake.authenticator.AuthenticateOutboundRequestSplit(ctx, presentedCustomerCredential(request), func() (string, error) {
		accessID, read, readErr := registrationjson.ReadOwnerOutboundSplit(bodyOrEmpty(request))
		if readErr != nil {
			return "", asMalformed(readErr)
		}
		command = read
		return accessID, nil
	})
	if err != nil {
		return application.SplitOutboundCommand{}, err
	}
	command.Tenant = identity.Tenant
	command.Owner = identity.Owner
	command.Origin = domain.OutboundOriginCustomer
	return command, nil
}

func (intake *CustomerOutboundIntake) IntakeOutboundSupplement(ctx context.Context, request *http.Request) (application.ResubmitCommand, error) {
	var command application.ResubmitCommand
	identity, err := intake.authenticator.AuthenticateOutboundRequestSupplement(ctx, presentedCustomerCredential(request), func() (string, error) {
		accessID, read, readErr := registrationjson.ReadOwnerOutboundSupplement(bodyOrEmpty(request))
		if readErr != nil {
			return "", asMalformed(readErr)
		}
		command = read
		return accessID, nil
	})
	if err != nil {
		return application.ResubmitCommand{}, err
	}
	command.Tenant = identity.Tenant
	command.Owner = identity.Owner
	command.RequestKey = identity.RequestKey.String()
	return command, nil
}

// OutboundOnBehalfAuthenticator 认证出库请求代录。
type OutboundOnBehalfAuthenticator interface {
	AuthenticateOutboundOnBehalf(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

type OutboundOnBehalfIntake interface {
	IntakeOutboundOnBehalf(ctx context.Context, request *http.Request) (application.AcceptOutboundCommand, error)
	IntakeOutboundSplit(ctx context.Context, request *http.Request) (application.SplitOutboundCommand, error)
}

type OperatorOutboundIntake struct {
	authenticator OutboundOnBehalfAuthenticator
}

func NewOperatorOutboundIntake(authenticator OutboundOnBehalfAuthenticator) (*OperatorOutboundIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: outbound intake needs an authenticator")
	}
	return &OperatorOutboundIntake{authenticator: authenticator}, nil
}

func (intake *OperatorOutboundIntake) IntakeOutboundOnBehalf(ctx context.Context, request *http.Request) (application.AcceptOutboundCommand, error) {
	identity, err := intake.authenticator.AuthenticateOutboundOnBehalf(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.AcceptOutboundCommand{}, err
	}
	command, err := registrationjson.OperatorOutboundCommand(requestBody(request))
	if err != nil {
		return application.AcceptOutboundCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	return command, nil
}

func (intake *OperatorOutboundIntake) IntakeOutboundSplit(ctx context.Context, request *http.Request) (application.SplitOutboundCommand, error) {
	identity, err := intake.authenticator.AuthenticateOutboundOnBehalf(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.SplitOutboundCommand{}, err
	}
	command, err := registrationjson.OperatorOutboundSplit(requestBody(request))
	if err != nil {
		return application.SplitOutboundCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	command.Origin = domain.OutboundOriginOperator
	return command, nil
}

type UnconfiguredOutboundIntake struct{}

func (UnconfiguredOutboundIntake) IntakeOutboundOnBehalf(context.Context, *http.Request) (application.AcceptOutboundCommand, error) {
	return application.AcceptOutboundCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredOutboundIntake) IntakeOutboundSplit(context.Context, *http.Request) (application.SplitOutboundCommand, error) {
	return application.SplitOutboundCommand{}, ErrAccessChannelNotConfigured
}

// ShortagePolicyIntake 是缺货处置登记口的 Intake。
type ShortagePolicyIntake interface {
	IntakeShortagePolicy(ctx context.Context, request *http.Request) (application.RegisterShortagePolicyCommand, error)
}

func (intake *OperatorRegistryIntake) IntakeShortagePolicy(ctx context.Context, request *http.Request) (application.RegisterShortagePolicyCommand, error) {
	return intakeRegistration(ctx, request, intake.authenticator, registrationjson.ShortagePolicyCommand)
}

func (UnconfiguredIntake) IntakeShortagePolicy(context.Context, *http.Request) (application.RegisterShortagePolicyCommand, error) {
	return application.RegisterShortagePolicyCommand{}, ErrAccessChannelNotConfigured
}

func NewSubmitOutboundEndpoint(intake CustomerOutboundPorts, service OutboundService) http.Handler {
	return newOutboundEndpoint(func(request *http.Request) (application.OutboundResult, error) {
		command, err := intake.IntakeOutboundRequest(request.Context(), request)
		if err != nil {
			return application.OutboundResult{}, err
		}
		return service.Accept(request.Context(), command)
	})
}

func NewRecordOutboundEndpoint(intake OutboundOnBehalfIntake, service OutboundService) http.Handler {
	return newOutboundEndpoint(func(request *http.Request) (application.OutboundResult, error) {
		command, err := intake.IntakeOutboundOnBehalf(request.Context(), request)
		if err != nil {
			return application.OutboundResult{}, err
		}
		return service.Accept(request.Context(), command)
	})
}

func NewSupplementCustomerOutboundEndpoint(intake CustomerOutboundPorts, follow FollowUpService) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeOutboundSupplement(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := follow.Resubmit(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		name := result.Outcome().String()
		if name == "" {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		status := http.StatusOK
		if result.Outcome() == application.FollowUpRecorded {
			status = http.StatusCreated
		}
		writeJSON(response, status, outboundAnswer{Outcome: name})
	})
}

func NewSplitCustomerOutboundEndpoint(intake CustomerOutboundPorts, service OutboundService) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeOutboundSplit(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := service.Split(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeSplitAnswer(response, request, result)
	})
}

func NewSplitOutboundEndpoint(intake OutboundOnBehalfIntake, service OutboundService) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeOutboundSplit(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := service.Split(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeSplitAnswer(response, request, result)
	})
}

func NewRegisterShortagePolicyEndpoint(intake ShortagePolicyIntake, service OutboundService) http.Handler {
	return newOutboundEndpoint(func(request *http.Request) (application.OutboundResult, error) {
		command, err := intake.IntakeShortagePolicy(request.Context(), request)
		if err != nil {
			return application.OutboundResult{}, err
		}
		return service.RegisterPolicy(request.Context(), command)
	})
}

func newOutboundEndpoint(run func(*http.Request) (application.OutboundResult, error)) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		result, err := run(request)
		if errors.Is(err, application.ErrOwnerAccountNotInTenant) {
			writeProblem(response, http.StatusNotFound, codeOwnerAccountNotVisible)
			return
		}
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		writeOutboundAnswer(response, request, result)
	})
}

func writeOutboundAnswer(response http.ResponseWriter, request *http.Request, result application.OutboundResult) {
	name := result.Outcome().String()
	if name == "" {
		httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	status := http.StatusOK
	switch result.Outcome() {
	case application.OutboundReserved, application.OutboundPartiallyReserved, application.OutboundRejected,
		application.OutboundPendingPolicy, application.OutboundHandedOff, application.OutboundPolicyRegistered,
		application.OutboundSplitResult:
		status = http.StatusCreated
	}
	body := outboundAnswer{Outcome: name}
	if requestSaved, ok := result.Request(); ok {
		body.Status = requestSaved.Status().String()
		if number, numbered := requestSaved.DocumentNumber(); numbered {
			body.DocumentNumber = number.String()
		}
	}
	writeJSON(response, status, body)
}

func writeSplitAnswer(response http.ResponseWriter, request *http.Request, result application.SplitResult) {
	name := result.Outcome().String()
	if name == "" {
		httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	status := http.StatusOK
	if result.Outcome() == application.OutboundSplitResult {
		status = http.StatusCreated
	}
	body := outboundAnswer{Outcome: name}
	if parent, ok := result.Parent(); ok {
		body.Status = parent.Status().String()
	}
	writeJSON(response, status, body)
}

type outboundAnswer struct {
	Outcome        string `json:"outcome"`
	Status         string `json:"status,omitempty"`
	DocumentNumber string `json:"documentNumber,omitempty"`
}
