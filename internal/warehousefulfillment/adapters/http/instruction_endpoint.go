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

// InstructionService 是指令的提交、撤回、作废、执行与开箱。
type InstructionService interface {
	InstructionExecutor
	InstructionCommands
}

// InstructionExecutor 是加工执行、换标执行与开箱转交的编排。
type InstructionExecutor interface {
	Execute(ctx context.Context, command application.ExecuteInstructionCommand) (application.InstructionResult, error)
	Unbox(ctx context.Context, command application.UnboxCommand) (application.InstructionResult, error)
}

// InstructionExecutionIntake 先认证再读加工或换标执行的载荷。
type InstructionExecutionIntake struct {
	authenticator InternalJobAuthenticator
}

func NewInstructionExecutionIntake(authenticator InternalJobAuthenticator) (*InstructionExecutionIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: instruction execution intake needs an authenticator")
	}
	return &InstructionExecutionIntake{authenticator: authenticator}, nil
}

func (intake *InstructionExecutionIntake) Intake(ctx context.Context, request *http.Request) (application.ExecuteInstructionCommand, error) {
	none := application.ExecuteInstructionCommand{}
	device, scopeKind, reference, err := warehouseWorkScope(request)
	if err != nil {
		return none, err
	}
	tenant, operator, recordedDevice, err := intake.authenticator.AuthenticateInternalJob(ctx, httpapi.BearerToken(request), device, scopeKind, reference)
	if err != nil {
		return none, err
	}
	command, err := registrationjson.InstructionExecutionCommand(requestBody(request))
	if err != nil {
		return none, asMalformed(err)
	}
	if reference != command.Warehouse.String() {
		return none, &malformedRequest{reason: httpapi.HeaderWorkScopeReference + " must equal warehouse"}
	}
	command.Tenant = tenant
	command.Operator = operator
	command.Device = recordedDevice
	return command, nil
}

// UnboxIntake 先认证再读开箱载荷。
type UnboxIntake struct {
	authenticator InternalJobAuthenticator
}

func NewUnboxIntake(authenticator InternalJobAuthenticator) (*UnboxIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: unbox intake needs an authenticator")
	}
	return &UnboxIntake{authenticator: authenticator}, nil
}

func (intake *UnboxIntake) Intake(ctx context.Context, request *http.Request) (application.UnboxCommand, error) {
	none := application.UnboxCommand{}
	device, scopeKind, reference, err := warehouseWorkScope(request)
	if err != nil {
		return none, err
	}
	tenant, operator, recordedDevice, err := intake.authenticator.AuthenticateInternalJob(ctx, httpapi.BearerToken(request), device, scopeKind, reference)
	if err != nil {
		return none, err
	}
	command, err := registrationjson.UnboxCommand(requestBody(request))
	if err != nil {
		return none, asMalformed(err)
	}
	if reference != command.Warehouse.String() {
		return none, &malformedRequest{reason: httpapi.HeaderWorkScopeReference + " must equal warehouse"}
	}
	command.Tenant = tenant
	command.Operator = operator
	command.Device = recordedDevice
	return command, nil
}

type UnconfiguredInstructionExecutionIntake struct{}

func (UnconfiguredInstructionExecutionIntake) Intake(context.Context, *http.Request) (application.ExecuteInstructionCommand, error) {
	return application.ExecuteInstructionCommand{}, ErrAccessChannelNotConfigured
}

type UnconfiguredUnboxIntake struct{}

func (UnconfiguredUnboxIntake) Intake(context.Context, *http.Request) (application.UnboxCommand, error) {
	return application.UnboxCommand{}, ErrAccessChannelNotConfigured
}

func NewExecuteInstructionEndpoint(intake interface {
	Intake(context.Context, *http.Request) (application.ExecuteInstructionCommand, error)
}, executor InstructionExecutor) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.Intake(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := executor.Execute(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "instruction execution did not form an answer", "factId", command.FactID.String())
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeInstructionAnswer(response, request, result)
	})
}

func NewUnboxEndpoint(intake interface {
	Intake(context.Context, *http.Request) (application.UnboxCommand, error)
}, executor InstructionExecutor) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.Intake(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := executor.Unbox(request.Context(), command)
		if err != nil {
			slog.ErrorContext(request.Context(), "unbox did not form an answer", "factId", command.FactID.String())
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeInstructionAnswer(response, request, result)
	})
}

func writeInstructionAnswer(response http.ResponseWriter, request *http.Request, result application.InstructionResult) {
	name := result.Outcome().String()
	if name == "" {
		httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	answer := internalJobAnswer{Outcome: name}
	status := http.StatusOK
	if result.Outcome() == application.InstructionAccepted || result.Outcome() == application.InstructionPosted || result.Outcome() == application.InstructionVoided {
		status = http.StatusCreated
	}
	if result.Outcome() == application.InstructionContentInvalid {
		for _, issue := range result.Issues() {
			answer.Issues = append(answer.Issues, internalJobIssueBody{Line: issue.Line(), Kind: issue.Kind().String()})
		}
	}
	writeJSON(response, status, answer)
}
