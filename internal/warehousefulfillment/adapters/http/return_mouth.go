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

// ReturnService 是退货预报、到仓与处置的编排。
type ReturnService interface {
	Accept(ctx context.Context, command application.AcceptReturnCommand) (application.ReturnResult, error)
	Receive(ctx context.Context, command application.ReceiveReturnCommand) (application.ReturnResult, error)
	Dispose(ctx context.Context, command application.DisposeReturnCommand) (application.ReturnResult, error)
}

// CustomerReturnAuthenticator 认证货主提交退货预报与回库处置。
type CustomerReturnAuthenticator interface {
	AuthenticateReturnPreAdvice(ctx context.Context, credential PresentedCustomerCredential, accessRequestID func() (string, error)) (CustomerCommandIdentity, error)
	AuthenticateReturnDisposition(ctx context.Context, credential PresentedCustomerCredential, accessRequestID func() (string, error)) (CustomerCommandIdentity, error)
}

// CustomerReturnPorts 是货主退货口在渠道没配时也能挂上的那一面。
type CustomerReturnPorts interface {
	IntakeReturnPreAdvice(ctx context.Context, request *http.Request) (application.AcceptReturnCommand, error)
	IntakeReturnDisposition(ctx context.Context, request *http.Request) (application.DisposeReturnCommand, error)
}

type CustomerReturnIntake struct {
	authenticator CustomerReturnAuthenticator
}

func NewCustomerReturnIntake(authenticator CustomerReturnAuthenticator) (*CustomerReturnIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: return customer intake needs an authenticator")
	}
	return &CustomerReturnIntake{authenticator: authenticator}, nil
}

func OpenCustomerReturnIntake(intake *CustomerReturnIntake) CustomerReturnPorts {
	if intake == nil {
		return unconfiguredCustomerReturn{}
	}
	return intake
}

type unconfiguredCustomerReturn struct{}

func (unconfiguredCustomerReturn) IntakeReturnPreAdvice(context.Context, *http.Request) (application.AcceptReturnCommand, error) {
	return application.AcceptReturnCommand{}, ErrAccessChannelNotConfigured
}

func (unconfiguredCustomerReturn) IntakeReturnDisposition(context.Context, *http.Request) (application.DisposeReturnCommand, error) {
	return application.DisposeReturnCommand{}, ErrAccessChannelNotConfigured
}

func (intake *CustomerReturnIntake) IntakeReturnPreAdvice(ctx context.Context, request *http.Request) (application.AcceptReturnCommand, error) {
	var command application.AcceptReturnCommand
	identity, err := intake.authenticator.AuthenticateReturnPreAdvice(ctx, presentedCustomerCredential(request), func() (string, error) {
		accessID, read, readErr := registrationjson.ReadOwnerReturnAdvice(bodyOrEmpty(request))
		if readErr != nil {
			return "", asMalformed(readErr)
		}
		command = read
		return accessID, nil
	})
	if err != nil {
		return application.AcceptReturnCommand{}, err
	}
	command.Tenant = identity.Tenant
	command.Owner = identity.Owner
	command.Key = identity.RequestKey.String()
	command.Origin = domain.ReturnOriginCustomer
	return command, nil
}

func (intake *CustomerReturnIntake) IntakeReturnDisposition(ctx context.Context, request *http.Request) (application.DisposeReturnCommand, error) {
	var command application.DisposeReturnCommand
	identity, err := intake.authenticator.AuthenticateReturnDisposition(ctx, presentedCustomerCredential(request), func() (string, error) {
		accessID, read, readErr := registrationjson.ReadOwnerReturnDisposition(bodyOrEmpty(request))
		if readErr != nil {
			return "", asMalformed(readErr)
		}
		command = read
		return accessID, nil
	})
	if err != nil {
		return application.DisposeReturnCommand{}, err
	}
	decision, err := domain.NewReturnDecisionID(identity.RequestKey.String())
	if err != nil {
		return application.DisposeReturnCommand{}, &malformedRequest{reason: "accessRequestId is required"}
	}
	command.Tenant = identity.Tenant
	command.Owner = identity.Owner
	command.Decision = decision
	command.Origin = domain.ReturnOriginCustomer
	return command, nil
}

// ReturnOnBehalfAuthenticator 认证退货代录。预报与处置共用退货代录这一种授予。
type ReturnOnBehalfAuthenticator interface {
	AuthenticateReturnOnBehalf(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

type ReturnOnBehalfIntake interface {
	IntakeReturnPreAdvice(ctx context.Context, request *http.Request) (application.AcceptReturnCommand, error)
	IntakeReturnDisposition(ctx context.Context, request *http.Request) (application.DisposeReturnCommand, error)
}

type OperatorReturnIntake struct {
	authenticator ReturnOnBehalfAuthenticator
}

func NewOperatorReturnIntake(authenticator ReturnOnBehalfAuthenticator) (*OperatorReturnIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: return intake needs an authenticator")
	}
	return &OperatorReturnIntake{authenticator: authenticator}, nil
}

func (intake *OperatorReturnIntake) IntakeReturnPreAdvice(ctx context.Context, request *http.Request) (application.AcceptReturnCommand, error) {
	identity, err := intake.authenticator.AuthenticateReturnOnBehalf(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.AcceptReturnCommand{}, err
	}
	command, err := registrationjson.OperatorReturnAdvice(requestBody(request))
	if err != nil {
		return application.AcceptReturnCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	command.Origin = domain.ReturnOriginOperator
	return command, nil
}

func (intake *OperatorReturnIntake) IntakeReturnDisposition(ctx context.Context, request *http.Request) (application.DisposeReturnCommand, error) {
	identity, err := intake.authenticator.AuthenticateReturnOnBehalf(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.DisposeReturnCommand{}, err
	}
	command, err := registrationjson.OperatorReturnDisposition(requestBody(request))
	if err != nil {
		return application.DisposeReturnCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	command.Origin = domain.ReturnOriginOperator
	return command, nil
}

type UnconfiguredReturnOnBehalfIntake struct{}

func (UnconfiguredReturnOnBehalfIntake) IntakeReturnPreAdvice(context.Context, *http.Request) (application.AcceptReturnCommand, error) {
	return application.AcceptReturnCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredReturnOnBehalfIntake) IntakeReturnDisposition(context.Context, *http.Request) (application.DisposeReturnCommand, error) {
	return application.DisposeReturnCommand{}, ErrAccessChannelNotConfigured
}

// ReturnReceiptIntake 是退货入库作业事实的 Intake。先认证再读载荷。
type ReturnReceiptIntake struct {
	authenticator InternalJobAuthenticator
}

func NewReturnReceiptIntake(authenticator InternalJobAuthenticator) (*ReturnReceiptIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: return receipt intake needs an authenticator")
	}
	return &ReturnReceiptIntake{authenticator: authenticator}, nil
}

func (intake *ReturnReceiptIntake) Intake(ctx context.Context, request *http.Request) (application.ReceiveReturnCommand, error) {
	device, scopeKind, reference, err := warehouseWorkScope(request)
	if err != nil {
		return application.ReceiveReturnCommand{}, err
	}
	tenant, operator, recordedDevice, err := intake.authenticator.AuthenticateInternalJob(ctx, httpapi.BearerToken(request), device, scopeKind, reference)
	if err != nil {
		return application.ReceiveReturnCommand{}, err
	}
	command, err := registrationjson.ReturnReceiptCommand(requestBody(request))
	if err != nil {
		return application.ReceiveReturnCommand{}, asMalformed(err)
	}
	if reference != command.Warehouse.String() {
		return application.ReceiveReturnCommand{}, &malformedRequest{reason: httpapi.HeaderWorkScopeReference + " must equal warehouse"}
	}
	command.Tenant = tenant
	command.Operator = operator
	command.Device = recordedDevice
	return command, nil
}

type UnconfiguredReturnReceiptIntake struct{}

func (UnconfiguredReturnReceiptIntake) Intake(context.Context, *http.Request) (application.ReceiveReturnCommand, error) {
	return application.ReceiveReturnCommand{}, ErrAccessChannelNotConfigured
}

func NewAcceptCustomerReturnEndpoint(intake CustomerReturnPorts, returns ReturnService) http.Handler {
	return returnEndpoint(func(request *http.Request) (application.ReturnResult, error) {
		command, err := intake.IntakeReturnPreAdvice(request.Context(), request)
		if err != nil {
			return application.ReturnResult{}, err
		}
		return returns.Accept(request.Context(), command)
	})
}

func NewAcceptReturnEndpoint(intake ReturnOnBehalfIntake, returns ReturnService) http.Handler {
	return returnEndpoint(func(request *http.Request) (application.ReturnResult, error) {
		command, err := intake.IntakeReturnPreAdvice(request.Context(), request)
		if err != nil {
			return application.ReturnResult{}, err
		}
		return returns.Accept(request.Context(), command)
	})
}

func NewDisposeCustomerReturnEndpoint(intake CustomerReturnPorts, returns ReturnService) http.Handler {
	return returnEndpoint(func(request *http.Request) (application.ReturnResult, error) {
		command, err := intake.IntakeReturnDisposition(request.Context(), request)
		if err != nil {
			return application.ReturnResult{}, err
		}
		return returns.Dispose(request.Context(), command)
	})
}

func NewDisposeReturnEndpoint(intake ReturnOnBehalfIntake, returns ReturnService) http.Handler {
	return returnEndpoint(func(request *http.Request) (application.ReturnResult, error) {
		command, err := intake.IntakeReturnDisposition(request.Context(), request)
		if err != nil {
			return application.ReturnResult{}, err
		}
		return returns.Dispose(request.Context(), command)
	})
}

func NewReceiveReturnEndpoint(intake interface {
	Intake(context.Context, *http.Request) (application.ReceiveReturnCommand, error)
}, returns ReturnService) http.Handler {
	return returnEndpoint(func(request *http.Request) (application.ReturnResult, error) {
		command, err := intake.Intake(request.Context(), request)
		if err != nil {
			return application.ReturnResult{}, err
		}
		return returns.Receive(request.Context(), command)
	})
}

func returnEndpoint(run func(*http.Request) (application.ReturnResult, error)) http.Handler {
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
		writeReturnAnswer(response, request, result)
	})
}

func writeReturnAnswer(response http.ResponseWriter, request *http.Request, result application.ReturnResult) {
	name := result.Outcome().String()
	if name == "" {
		httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	status := http.StatusOK
	switch result.Outcome() {
	case application.ReturnAccepted, application.ReturnCounted, application.ReturnUnclaimed, application.ReturnDisposed:
		status = http.StatusCreated
	}
	body := returnAnswer{Outcome: name, Arrival: result.Arrival()}
	if reason, ok := result.NotAcceptingReason(); ok {
		body.Reason = reason.String()
	}
	if advice, ok := result.Advice(); ok {
		body.DocumentNumber = advice.Number
	}
	writeJSON(response, status, body)
}

type returnAnswer struct {
	Outcome        string `json:"outcome"`
	Reason         string `json:"reason,omitempty"`
	Arrival        string `json:"arrivalId,omitempty"`
	DocumentNumber string `json:"documentNumber,omitempty"`
}
