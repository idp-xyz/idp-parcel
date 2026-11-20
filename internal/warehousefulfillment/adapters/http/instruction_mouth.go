package warehousehttp

import (
	"context"
	"errors"
	"io"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// CustomerInstructionAuthenticator 按口认证加工与换标的货主提交、撤回。
type CustomerInstructionAuthenticator interface {
	AuthenticateProcessingInstructionSubmission(ctx context.Context, credential PresentedCustomerCredential, accessRequestID func() (string, error)) (CustomerCommandIdentity, error)
	AuthenticateProcessingInstructionWithdrawal(ctx context.Context, credential PresentedCustomerCredential, accessRequestID func() (string, error)) (CustomerCommandIdentity, error)
	AuthenticateRelabelInstructionSubmission(ctx context.Context, credential PresentedCustomerCredential, accessRequestID func() (string, error)) (CustomerCommandIdentity, error)
	AuthenticateRelabelInstructionWithdrawal(ctx context.Context, credential PresentedCustomerCredential, accessRequestID func() (string, error)) (CustomerCommandIdentity, error)
}

// CustomerInstructionPorts 是四个货主口的 Intake。未配置时一个字节的业务内容都不读。
type CustomerInstructionPorts interface {
	IntakeProcessingInstruction(ctx context.Context, request *http.Request) (application.AcceptInstructionCommand, error)
	IntakeRelabelInstruction(ctx context.Context, request *http.Request) (application.AcceptInstructionCommand, error)
	IntakeProcessingWithdrawal(ctx context.Context, request *http.Request) (instructionWithdrawal, error)
	IntakeRelabelWithdrawal(ctx context.Context, request *http.Request) (instructionWithdrawal, error)
}

// instructionWithdrawal 是撤回口认证之后的指名。种类由口绑死，不在载荷里。
type instructionWithdrawal struct {
	Tenant domain.TenantID
	Owner  domain.OwnerAccountID
	ID     domain.InstructionID
	Key    domain.RequestKey
}

// CustomerInstructionIntake 是首方客户渠道在这四口的 Intake。
type CustomerInstructionIntake struct {
	authenticator CustomerInstructionAuthenticator
}

func NewCustomerInstructionIntake(authenticator CustomerInstructionAuthenticator) (*CustomerInstructionIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: instruction customer intake needs an authenticator")
	}
	return &CustomerInstructionIntake{authenticator: authenticator}, nil
}

func OpenCustomerInstructionIntake(intake *CustomerInstructionIntake) CustomerInstructionPorts {
	if intake == nil {
		return unconfiguredCustomerInstruction{}
	}
	return intake
}

type unconfiguredCustomerInstruction struct{}

func (unconfiguredCustomerInstruction) IntakeProcessingInstruction(context.Context, *http.Request) (application.AcceptInstructionCommand, error) {
	return application.AcceptInstructionCommand{}, ErrAccessChannelNotConfigured
}
func (unconfiguredCustomerInstruction) IntakeRelabelInstruction(context.Context, *http.Request) (application.AcceptInstructionCommand, error) {
	return application.AcceptInstructionCommand{}, ErrAccessChannelNotConfigured
}
func (unconfiguredCustomerInstruction) IntakeProcessingWithdrawal(context.Context, *http.Request) (instructionWithdrawal, error) {
	return instructionWithdrawal{}, ErrAccessChannelNotConfigured
}
func (unconfiguredCustomerInstruction) IntakeRelabelWithdrawal(context.Context, *http.Request) (instructionWithdrawal, error) {
	return instructionWithdrawal{}, ErrAccessChannelNotConfigured
}

func (intake *CustomerInstructionIntake) IntakeProcessingInstruction(ctx context.Context, request *http.Request) (application.AcceptInstructionCommand, error) {
	return intake.intakeSubmission(ctx, request, true, intake.authenticator.AuthenticateProcessingInstructionSubmission, registrationjson.ReadOwnerProcessingInstruction)
}

func (intake *CustomerInstructionIntake) IntakeRelabelInstruction(ctx context.Context, request *http.Request) (application.AcceptInstructionCommand, error) {
	return intake.intakeSubmission(ctx, request, false, intake.authenticator.AuthenticateRelabelInstructionSubmission, registrationjson.ReadOwnerRelabelInstruction)
}

func (intake *CustomerInstructionIntake) intakeSubmission(
	ctx context.Context,
	request *http.Request,
	processing bool,
	authenticate func(context.Context, PresentedCustomerCredential, func() (string, error)) (CustomerCommandIdentity, error),
	read func(io.Reader) (registrationjson.OwnerInstructionFields, error),
) (application.AcceptInstructionCommand, error) {
	var fields registrationjson.OwnerInstructionFields
	identity, err := authenticate(ctx, presentedCustomerCredential(request), func() (string, error) {
		readFields, readErr := read(bodyOrEmpty(request))
		if readErr != nil {
			return "", asMalformed(readErr)
		}
		fields = readFields
		return readFields.AccessRequestID, nil
	})
	if err != nil {
		return application.AcceptInstructionCommand{}, err
	}
	kind := domain.InstructionRelabel
	if processing {
		kind = domain.InstructionProcess
		if fields.Disassemble {
			kind = domain.InstructionDisassemble
		}
	}
	return application.AcceptInstructionCommand{
		Tenant: identity.Tenant, Owner: identity.Owner, Warehouse: fields.Warehouse, Kind: kind,
		Origin: domain.InstructionOriginCustomer, Goods: fields.Goods, Unit: fields.Unit, Quantity: fields.Quantity,
		FromAlias: fields.FromAlias, ToAlias: fields.ToAlias, Batch: fields.Batch, Expiry: fields.Expiry,
		Reference: fields.Reference, RequestKey: identity.RequestKey, NamedRevision: fields.NamedRevision,
	}, nil
}

func (intake *CustomerInstructionIntake) IntakeProcessingWithdrawal(ctx context.Context, request *http.Request) (instructionWithdrawal, error) {
	return intake.intakeWithdrawal(ctx, request, intake.authenticator.AuthenticateProcessingInstructionWithdrawal)
}

func (intake *CustomerInstructionIntake) IntakeRelabelWithdrawal(ctx context.Context, request *http.Request) (instructionWithdrawal, error) {
	return intake.intakeWithdrawal(ctx, request, intake.authenticator.AuthenticateRelabelInstructionWithdrawal)
}

func (intake *CustomerInstructionIntake) intakeWithdrawal(
	ctx context.Context,
	request *http.Request,
	authenticate func(context.Context, PresentedCustomerCredential, func() (string, error)) (CustomerCommandIdentity, error),
) (instructionWithdrawal, error) {
	var fields registrationjson.OwnerInstructionWithdrawalFields
	identity, err := authenticate(ctx, presentedCustomerCredential(request), func() (string, error) {
		read, readErr := registrationjson.ReadOwnerInstructionWithdrawal(bodyOrEmpty(request))
		if readErr != nil {
			return "", asMalformed(readErr)
		}
		fields = read
		return read.AccessRequestID, nil
	})
	if err != nil {
		return instructionWithdrawal{}, err
	}
	return instructionWithdrawal{Tenant: identity.Tenant, Owner: identity.Owner, ID: fields.ID, Key: identity.RequestKey}, nil
}

// InstructionCommands 是提交、撤回与作废转交的编排。执行与开箱不走这里。
type InstructionCommands interface {
	Accept(ctx context.Context, command application.AcceptInstructionCommand) (application.InstructionResult, error)
	Withdraw(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, id domain.InstructionID, key domain.RequestKey, allowed []domain.InstructionKind) (application.InstructionResult, error)
	Void(ctx context.Context, tenant domain.TenantID, id domain.InstructionID, operator domain.Registrant, reason string, allowed []domain.InstructionKind) (application.InstructionResult, error)
}

func NewSubmitProcessingInstructionEndpoint(intake CustomerInstructionPorts, commands InstructionCommands) http.Handler {
	return newCustomerInstructionEndpoint(func(ctx context.Context, request *http.Request) (application.InstructionResult, error) {
		command, err := intake.IntakeProcessingInstruction(ctx, request)
		if err != nil {
			return application.InstructionResult{}, err
		}
		return commands.Accept(ctx, command)
	})
}

func NewSubmitRelabelInstructionEndpoint(intake CustomerInstructionPorts, commands InstructionCommands) http.Handler {
	return newCustomerInstructionEndpoint(func(ctx context.Context, request *http.Request) (application.InstructionResult, error) {
		command, err := intake.IntakeRelabelInstruction(ctx, request)
		if err != nil {
			return application.InstructionResult{}, err
		}
		return commands.Accept(ctx, command)
	})
}

func NewWithdrawProcessingInstructionEndpoint(intake CustomerInstructionPorts, commands InstructionCommands) http.Handler {
	return newCustomerInstructionEndpoint(func(ctx context.Context, request *http.Request) (application.InstructionResult, error) {
		withdrawal, err := intake.IntakeProcessingWithdrawal(ctx, request)
		if err != nil {
			return application.InstructionResult{}, err
		}
		return commands.Withdraw(ctx, withdrawal.Tenant, withdrawal.Owner, withdrawal.ID, withdrawal.Key, processingKinds())
	})
}

func NewWithdrawRelabelInstructionEndpoint(intake CustomerInstructionPorts, commands InstructionCommands) http.Handler {
	return newCustomerInstructionEndpoint(func(ctx context.Context, request *http.Request) (application.InstructionResult, error) {
		withdrawal, err := intake.IntakeRelabelWithdrawal(ctx, request)
		if err != nil {
			return application.InstructionResult{}, err
		}
		return commands.Withdraw(ctx, withdrawal.Tenant, withdrawal.Owner, withdrawal.ID, withdrawal.Key, []domain.InstructionKind{domain.InstructionRelabel})
	})
}

func newCustomerInstructionEndpoint(run func(context.Context, *http.Request) (application.InstructionResult, error)) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if refuseCustomerMethod(response, request, http.MethodPost) {
			return
		}
		result, err := run(request.Context(), request)
		if err != nil {
			if customerIntakeError(err) {
				answerCustomerIntake(response, request, err)
				return
			}
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed, "")
			return
		}
		writeInstructionMouth(response, request, result, true)
	})
}

// OperatorInstructionAuthenticator 认证加工或换标的运营决定。发起与作废共用。
type OperatorInstructionAuthenticator interface {
	AuthenticateInstruction(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

// OperatorInstructionIntake 先认证再读发起或作废的载荷。
type OperatorInstructionIntake struct {
	authenticator OperatorInstructionAuthenticator
	processing    bool
}

func NewOperatorInstructionIntake(authenticator OperatorInstructionAuthenticator, processing bool) (*OperatorInstructionIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: instruction operator intake needs an authenticator")
	}
	return &OperatorInstructionIntake{authenticator: authenticator, processing: processing}, nil
}

func (intake *OperatorInstructionIntake) IntakeInstruction(ctx context.Context, request *http.Request) (application.AcceptInstructionCommand, error) {
	identity, err := intake.authenticator.AuthenticateInstruction(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.AcceptInstructionCommand{}, err
	}
	var fields registrationjson.OperatorInstructionFields
	var readErr error
	if intake.processing {
		fields, readErr = registrationjson.ReadOperatorProcessingInstruction(requestBody(request))
	} else {
		fields, readErr = registrationjson.ReadOperatorRelabelInstruction(requestBody(request))
	}
	if readErr != nil {
		return application.AcceptInstructionCommand{}, asMalformed(readErr)
	}
	kind := domain.InstructionRelabel
	if intake.processing {
		kind = domain.InstructionProcess
		if fields.Disassemble {
			kind = domain.InstructionDisassemble
		}
	}
	return application.AcceptInstructionCommand{
		Tenant: identity.Tenant, Owner: fields.Owner, Warehouse: fields.Warehouse, Kind: kind,
		Origin: domain.InstructionOriginOperator, Goods: fields.Goods, Unit: fields.Unit, Quantity: fields.Quantity,
		FromAlias: fields.FromAlias, ToAlias: fields.ToAlias, Batch: fields.Batch, Expiry: fields.Expiry,
		Reference: fields.Reference, DecisionID: fields.DecisionID, Operator: identity.Registrant,
		Reason: fields.Reason, Basis: fields.Basis, NamedRevision: fields.NamedRevision,
	}, nil
}

func (intake *OperatorInstructionIntake) IntakeVoid(ctx context.Context, request *http.Request) (operatorInstructionVoid, error) {
	identity, err := intake.authenticator.AuthenticateInstruction(ctx, httpapi.BearerToken(request))
	if err != nil {
		return operatorInstructionVoid{}, err
	}
	fields, err := registrationjson.ReadOperatorInstructionVoid(requestBody(request))
	if err != nil {
		return operatorInstructionVoid{}, asMalformed(err)
	}
	return operatorInstructionVoid{Tenant: identity.Tenant, Operator: identity.Registrant, ID: fields.ID, Reason: fields.Reason, Processing: intake.processing}, nil
}

type operatorInstructionVoid struct {
	Tenant     domain.TenantID
	Operator   domain.Registrant
	ID         domain.InstructionID
	Reason     string
	Processing bool
}

type UnconfiguredOperatorInstructionIntake struct{}

func (UnconfiguredOperatorInstructionIntake) IntakeInstruction(context.Context, *http.Request) (application.AcceptInstructionCommand, error) {
	return application.AcceptInstructionCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredOperatorInstructionIntake) IntakeVoid(context.Context, *http.Request) (operatorInstructionVoid, error) {
	return operatorInstructionVoid{}, ErrAccessChannelNotConfigured
}

func NewInitiateInstructionEndpoint(intake interface {
	IntakeInstruction(context.Context, *http.Request) (application.AcceptInstructionCommand, error)
}, commands InstructionCommands) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeInstruction(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := commands.Accept(request.Context(), command)
		if errors.Is(err, application.ErrOwnerAccountNotInTenant) {
			writeProblem(response, http.StatusNotFound, codeOwnerAccountNotVisible)
			return
		}
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeInstructionMouth(response, request, result, false)
	})
}

func NewVoidInstructionEndpoint(intake interface {
	IntakeVoid(context.Context, *http.Request) (operatorInstructionVoid, error)
}, commands InstructionCommands) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeVoid(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		allowed := []domain.InstructionKind{domain.InstructionRelabel}
		if command.Processing {
			allowed = processingKinds()
		}
		result, err := commands.Void(request.Context(), command.Tenant, command.ID, command.Operator, command.Reason, allowed)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeInstructionMouth(response, request, result, false)
	})
}

func customerIntakeError(err error) bool {
	return errors.Is(err, ErrAccessChannelNotConfigured) ||
		errors.Is(err, ErrCustomerClientCredentialRejected) ||
		errors.Is(err, ErrCustomerClientNotGranted) ||
		errors.Is(err, ErrOutsideAdmissionScope) ||
		errors.Is(err, ErrIdentityDependencyUnavailable) ||
		errors.Is(err, ErrMalformedRequest)
}

func processingKinds() []domain.InstructionKind {
	return []domain.InstructionKind{domain.InstructionProcess, domain.InstructionDisassemble}
}

func writeInstructionMouth(response http.ResponseWriter, request *http.Request, result application.InstructionResult, customer bool) {
	name := result.Outcome().String()
	if name == "" {
		if customer {
			writeCustomerProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome, "")
			return
		}
		httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := instructionMouthAnswer{Outcome: name}
	if instruction, ok := result.Instruction(); ok {
		answer.InstructionID = instruction.ID().String()
	}
	status := http.StatusOK
	if result.Outcome() == application.InstructionAccepted || result.Outcome() == application.InstructionVoided {
		status = http.StatusCreated
	}
	if result.Outcome() == application.InstructionContentInvalid {
		for _, issue := range result.Issues() {
			answer.Issues = append(answer.Issues, internalJobIssueBody{Line: issue.Line(), Kind: issue.Kind().String()})
		}
	}
	writeJSON(response, status, answer)
}

type instructionMouthAnswer struct {
	Outcome       string                 `json:"outcome"`
	InstructionID string                 `json:"instructionId,omitempty"`
	Issues        []internalJobIssueBody `json:"issues,omitempty"`
}
