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

// WaveService 是放波次与采用分配策略转交的编排。
type WaveService interface {
	Release(ctx context.Context, command application.ReleaseWaveCommand) (application.WaveResult, error)
	Adopt(ctx context.Context, command application.AdoptStrategyCommand) (application.WaveResult, error)
	Pick(ctx context.Context, command application.PickCommand) (application.WaveResult, error)
	Check(ctx context.Context, command application.CheckCommand) (application.WaveResult, error)
	Pack(ctx context.Context, command application.PackCommand) (application.WaveResult, error)
	Confirm(ctx context.Context, command application.ConfirmCommand) (application.WaveResult, error)
	RegisterOwnerLabel(ctx context.Context, command application.RegisterOwnerLabelCommand) (application.WaveResult, error)
	Correct(ctx context.Context, command application.CorrectConfirmationCommand) (application.WaveResult, error)
	ApproveReplenishment(ctx context.Context, command application.ApproveReplenishmentCommand) (application.WaveResult, error)
	Cutoff(ctx context.Context, command application.CutoffCommand) (application.CutoffResult, error)
	Return(ctx context.Context, command application.ReturnCommand) (application.WaveResult, error)
}

// AllocationStrategyIntake 是采用参考分配策略的 Intake。
type AllocationStrategyIntake interface {
	IntakeAllocationStrategy(ctx context.Context, request *http.Request) (application.AdoptStrategyCommand, error)
}

func (intake *OperatorRegistryIntake) IntakeAllocationStrategy(ctx context.Context, request *http.Request) (application.AdoptStrategyCommand, error) {
	return intakeRegistration(ctx, request, intake.authenticator, registrationjson.AllocationStrategyCommand)
}

func (UnconfiguredIntake) IntakeAllocationStrategy(context.Context, *http.Request) (application.AdoptStrategyCommand, error) {
	return application.AdoptStrategyCommand{}, ErrAccessChannelNotConfigured
}

// WaveReleaseAuthenticator 认证放波次。
type WaveReleaseAuthenticator interface {
	AuthenticateWaveRelease(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

type WaveReleaseIntake interface {
	IntakeWaveRelease(ctx context.Context, request *http.Request) (application.ReleaseWaveCommand, error)
}

type OperatorWaveIntake struct {
	authenticator WaveReleaseAuthenticator
}

func NewOperatorWaveIntake(authenticator WaveReleaseAuthenticator) (*OperatorWaveIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: wave intake needs an authenticator")
	}
	return &OperatorWaveIntake{authenticator: authenticator}, nil
}

func (intake *OperatorWaveIntake) IntakeWaveRelease(ctx context.Context, request *http.Request) (application.ReleaseWaveCommand, error) {
	identity, err := intake.authenticator.AuthenticateWaveRelease(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.ReleaseWaveCommand{}, err
	}
	command, err := registrationjson.ReleaseWaveCommand(requestBody(request))
	if err != nil {
		return application.ReleaseWaveCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	return command, nil
}

type UnconfiguredWaveIntake struct{}

func (UnconfiguredWaveIntake) IntakeWaveRelease(context.Context, *http.Request) (application.ReleaseWaveCommand, error) {
	return application.ReleaseWaveCommand{}, ErrAccessChannelNotConfigured
}

func NewAdoptAllocationStrategyEndpoint(intake AllocationStrategyIntake, waves WaveService) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeAllocationStrategy(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := waves.Adopt(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeWaveAnswer(response, request, result)
	})
}

func NewReleaseWaveEndpoint(intake WaveReleaseIntake, waves WaveService) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeWaveRelease(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := waves.Release(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeWaveAnswer(response, request, result)
	})
}

// PickIntake 先认证拣货事实再读载荷。事实种类由装配绑成 PICK。
type PickIntake struct {
	authenticator InternalJobAuthenticator
}

func NewPickIntake(authenticator InternalJobAuthenticator) (*PickIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: pick intake needs an authenticator")
	}
	return &PickIntake{authenticator: authenticator}, nil
}

func (intake *PickIntake) Intake(ctx context.Context, request *http.Request) (application.PickCommand, error) {
	tenant, reference, err := admitWarehouseWork(ctx, request, intake.authenticator)
	if err != nil {
		return application.PickCommand{}, err
	}
	command, err := registrationjson.PickCommand(requestBody(request))
	if err != nil {
		return application.PickCommand{}, asMalformed(err)
	}
	if reference != command.Warehouse.String() {
		return application.PickCommand{}, &malformedRequest{reason: httpapi.HeaderWorkScopeReference + " must equal warehouse"}
	}
	command.Tenant = tenant
	return command, nil
}

type UnconfiguredPickIntake struct{}

func (UnconfiguredPickIntake) Intake(context.Context, *http.Request) (application.PickCommand, error) {
	return application.PickCommand{}, ErrAccessChannelNotConfigured
}

func NewRecordPickEndpoint(intake interface {
	Intake(context.Context, *http.Request) (application.PickCommand, error)
}, waves WaveService) http.Handler {
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
		result, err := waves.Pick(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeWaveAnswer(response, request, result)
	})
}

// CheckIntake 先认证出库复核再读载荷。事实种类由装配绑成 OUTBOUND_CHECK。
type CheckIntake struct {
	authenticator InternalJobAuthenticator
}

func NewCheckIntake(authenticator InternalJobAuthenticator) (*CheckIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: outbound check intake needs an authenticator")
	}
	return &CheckIntake{authenticator: authenticator}, nil
}

func (intake *CheckIntake) Intake(ctx context.Context, request *http.Request) (application.CheckCommand, error) {
	tenant, reference, err := admitWarehouseWork(ctx, request, intake.authenticator)
	if err != nil {
		return application.CheckCommand{}, err
	}
	command, err := registrationjson.CheckCommand(requestBody(request))
	if err != nil {
		return application.CheckCommand{}, asMalformed(err)
	}
	if reference != command.Warehouse.String() {
		return application.CheckCommand{}, &malformedRequest{reason: httpapi.HeaderWorkScopeReference + " must equal warehouse"}
	}
	command.Tenant = tenant
	return command, nil
}

type UnconfiguredCheckIntake struct{}

func (UnconfiguredCheckIntake) Intake(context.Context, *http.Request) (application.CheckCommand, error) {
	return application.CheckCommand{}, ErrAccessChannelNotConfigured
}

func NewRecordCheckEndpoint(intake interface {
	Intake(context.Context, *http.Request) (application.CheckCommand, error)
}, waves WaveService) http.Handler {
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
		result, err := waves.Check(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeWaveAnswer(response, request, result)
	})
}

// PackIntake 先认证打包事实再读载荷。事实种类由装配绑成 PACK。包材文本不查册。
type PackIntake struct {
	authenticator InternalJobAuthenticator
}

func NewPackIntake(authenticator InternalJobAuthenticator) (*PackIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: pack intake needs an authenticator")
	}
	return &PackIntake{authenticator: authenticator}, nil
}

func (intake *PackIntake) Intake(ctx context.Context, request *http.Request) (application.PackCommand, error) {
	tenant, reference, err := admitWarehouseWork(ctx, request, intake.authenticator)
	if err != nil {
		return application.PackCommand{}, err
	}
	command, err := registrationjson.PackCommand(requestBody(request))
	if err != nil {
		return application.PackCommand{}, asMalformed(err)
	}
	if reference != command.Warehouse.String() {
		return application.PackCommand{}, &malformedRequest{reason: httpapi.HeaderWorkScopeReference + " must equal warehouse"}
	}
	command.Tenant = tenant
	return command, nil
}

type UnconfiguredPackIntake struct{}

func (UnconfiguredPackIntake) Intake(context.Context, *http.Request) (application.PackCommand, error) {
	return application.PackCommand{}, ErrAccessChannelNotConfigured
}

func NewRecordPackEndpoint(intake interface {
	Intake(context.Context, *http.Request) (application.PackCommand, error)
}, waves WaveService) http.Handler {
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
		result, err := waves.Pack(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeWaveAnswer(response, request, result)
	})
}

// ConfirmIntake 先认证出库确认或自提交付，再读载荷。事实种类由装配绑上。
type ConfirmIntake struct {
	authenticator InternalJobAuthenticator
	kind          string
}

func NewConfirmIntake(authenticator InternalJobAuthenticator, kind string) (*ConfirmIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: outbound confirmation intake needs an authenticator")
	}
	return &ConfirmIntake{authenticator: authenticator, kind: kind}, nil
}

func (intake *ConfirmIntake) Intake(ctx context.Context, request *http.Request) (application.ConfirmCommand, error) {
	tenant, reference, err := admitWarehouseWork(ctx, request, intake.authenticator)
	if err != nil {
		return application.ConfirmCommand{}, err
	}
	command, err := registrationjson.ConfirmCommand(requestBody(request), intake.kind)
	if err != nil {
		return application.ConfirmCommand{}, asMalformed(err)
	}
	if reference != command.Warehouse.String() {
		return application.ConfirmCommand{}, &malformedRequest{reason: httpapi.HeaderWorkScopeReference + " must equal warehouse"}
	}
	command.Tenant = tenant
	return command, nil
}

type UnconfiguredConfirmIntake struct{}

func (UnconfiguredConfirmIntake) Intake(context.Context, *http.Request) (application.ConfirmCommand, error) {
	return application.ConfirmCommand{}, ErrAccessChannelNotConfigured
}

func NewRecordReturnEndpoint(intake interface {
	Intake(context.Context, *http.Request) (application.ReturnCommand, error)
}, waves WaveService) http.Handler {
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
		result, err := waves.Return(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeWaveAnswer(response, request, result)
	})
}

type ReturnIntake struct {
	authenticator InternalJobAuthenticator
}

func NewReturnIntake(authenticator InternalJobAuthenticator) (*ReturnIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: return intake needs an authenticator")
	}
	return &ReturnIntake{authenticator: authenticator}, nil
}

func (intake *ReturnIntake) Intake(ctx context.Context, request *http.Request) (application.ReturnCommand, error) {
	tenant, reference, err := admitWarehouseWork(ctx, request, intake.authenticator)
	if err != nil {
		return application.ReturnCommand{}, err
	}
	command, err := registrationjson.ReturnCommand(requestBody(request))
	if err != nil {
		return application.ReturnCommand{}, asMalformed(err)
	}
	if reference != command.Warehouse.String() {
		return application.ReturnCommand{}, &malformedRequest{reason: httpapi.HeaderWorkScopeReference + " must equal warehouse"}
	}
	command.Tenant = tenant
	return command, nil
}

type UnconfiguredReturnIntake struct{}

func (UnconfiguredReturnIntake) Intake(context.Context, *http.Request) (application.ReturnCommand, error) {
	return application.ReturnCommand{}, ErrAccessChannelNotConfigured
}

func NewRecordConfirmationEndpoint(intake interface {
	Intake(context.Context, *http.Request) (application.ConfirmCommand, error)
}, waves WaveService) http.Handler {
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
		result, err := waves.Confirm(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeWaveAnswer(response, request, result)
	})
}

func NewRegisterOwnerLabelEndpoint(intake interface {
	IntakeOwnerLabel(context.Context, *http.Request) (application.RegisterOwnerLabelCommand, error)
}, waves WaveService) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		command, err := intake.IntakeOwnerLabel(request.Context(), request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		result, err := waves.RegisterOwnerLabel(request.Context(), command)
		if err != nil {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeNoAnswerFormed)
			return
		}
		writeWaveAnswer(response, request, result)
	})
}

// CorrectionAuthenticator 认证出库确认更正。
type CorrectionAuthenticator interface {
	AuthenticateCorrection(ctx context.Context, bearerToken string) (OperatorIdentity, error)
}

type CorrectionIntake interface {
	IntakeCorrection(ctx context.Context, request *http.Request) (application.CorrectConfirmationCommand, error)
}

type OperatorCorrectionIntake struct {
	authenticator CorrectionAuthenticator
}

func NewOperatorCorrectionIntake(authenticator CorrectionAuthenticator) (*OperatorCorrectionIntake, error) {
	if authenticator == nil {
		return nil, errors.New("warehouse fulfillment http: correction intake needs an authenticator")
	}
	return &OperatorCorrectionIntake{authenticator: authenticator}, nil
}

func (intake *OperatorCorrectionIntake) IntakeCorrection(ctx context.Context, request *http.Request) (application.CorrectConfirmationCommand, error) {
	identity, err := intake.authenticator.AuthenticateCorrection(ctx, httpapi.BearerToken(request))
	if err != nil {
		return application.CorrectConfirmationCommand{}, err
	}
	command, err := registrationjson.CorrectConfirmationCommand(requestBody(request))
	if err != nil {
		return application.CorrectConfirmationCommand{}, asMalformed(err)
	}
	command.Tenant = identity.Tenant
	command.Operator = identity.Registrant
	return command, nil
}

type UnconfiguredCorrectionIntake struct{}

func (UnconfiguredCorrectionIntake) IntakeCorrection(context.Context, *http.Request) (application.CorrectConfirmationCommand, error) {
	return application.CorrectConfirmationCommand{}, ErrAccessChannelNotConfigured
}

func NewCorrectConfirmationEndpoint(intake CorrectionIntake, waves WaveService) http.Handler {
	return confirmationDecisionEndpoint(func(request *http.Request) (application.WaveResult, error) {
		command, err := intake.IntakeCorrection(request.Context(), request)
		if err != nil {
			return application.WaveResult{}, err
		}
		return waves.Correct(request.Context(), command)
	})
}

func NewApproveReplenishmentEndpoint(intake StockApprovalIntake, waves WaveService) http.Handler {
	return confirmationDecisionEndpoint(func(request *http.Request) (application.WaveResult, error) {
		command, err := intake.IntakeApproveReplenishment(request.Context(), request)
		if err != nil {
			return application.WaveResult{}, err
		}
		return waves.ApproveReplenishment(request.Context(), command)
	})
}

func confirmationDecisionEndpoint(run func(*http.Request) (application.WaveResult, error)) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.Header().Set("Allow", http.MethodPost)
			httpapi.WriteProblem(response, request, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		result, err := run(request)
		if err != nil {
			writeReceiptProblem(response, request, err)
			return
		}
		writeWaveAnswer(response, request, result)
	})
}

// admitWarehouseWork 先认证再读载荷。返回的参照留给调用方去对载荷里的仓。
func admitWarehouseWork(ctx context.Context, request *http.Request, authenticator InternalJobAuthenticator) (domain.TenantID, string, error) {
	device, scopeKind, reference, err := warehouseWorkScope(request)
	if err != nil {
		return domain.TenantID{}, "", err
	}
	tenant, _, _, err := authenticator.AuthenticateInternalJob(ctx, httpapi.BearerToken(request), device, scopeKind, reference)
	if err != nil {
		return domain.TenantID{}, "", err
	}
	return tenant, reference, nil
}

func writeWaveAnswer(response http.ResponseWriter, request *http.Request, result application.WaveResult) {
	name := result.Outcome().String()
	if name == "" {
		httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
		return
	}
	status := http.StatusOK
	if result.Outcome() == application.WaveAllocated || result.Outcome() == application.WaveStrategyRegistered ||
		result.Outcome() == application.WavePicked || result.Outcome() == application.WaveChecked || result.Outcome() == application.WavePacked ||
		result.Outcome() == application.WaveConfirmed || result.Outcome() == application.WaveRecorded ||
		result.Outcome() == application.WaveCorrected || result.Outcome() == application.WavePending ||
		result.Outcome() == application.WavePosted ||
		result.Outcome() == application.WaveSubmitted {
		status = http.StatusCreated
	}
	writeJSON(response, status, outboundAnswer{Outcome: name})
}
