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
			result.Outcome() == application.WaveSubmitted {
		status = http.StatusCreated
	}
	writeJSON(response, status, outboundAnswer{Outcome: name})
}
