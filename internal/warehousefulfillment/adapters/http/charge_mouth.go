package warehousehttp

import (
	"context"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// ChargeService 登记收费取值、日结仓租，并作废或重述已经形成的发生项。形成本身跟着来源事实走，不在这些口上填写。
type ChargeService interface {
	RegisterUnits(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, weight, volume string) (application.ChargeResult, error)
	RegisterAging(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, basis string) (application.ChargeResult, error)
	RegisterProject(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, name string) (application.ChargeResult, error)
	CloseStorageDay(ctx context.Context, tenant domain.TenantID, owner domain.OwnerAccountID, warehouse domain.WarehouseCode, day time.Time) (application.ChargeResult, error)
	Void(ctx context.Context, tenant domain.TenantID, id string, version int) (application.ChargeResult, error)
	Restate(ctx context.Context, tenant domain.TenantID, id string, version int, draft application.ChargeDraft) (application.ChargeResult, error)
}

// ChargeIntake 是单位、库龄、增值项目、仓租日结与更正的登记 Intake。
type ChargeIntake interface {
	IntakeChargeUnits(ctx context.Context, request *http.Request) (domain.TenantID, application.RegisterChargeUnitsCommand, error)
	IntakeChargeAging(ctx context.Context, request *http.Request) (domain.TenantID, application.RegisterChargeAgingCommand, error)
	IntakeChargeProject(ctx context.Context, request *http.Request) (domain.TenantID, application.RegisterChargeProjectCommand, error)
	IntakeStorageDay(ctx context.Context, request *http.Request) (domain.TenantID, application.CloseStorageDayCommand, error)
	IntakeChargeVoid(ctx context.Context, request *http.Request) (domain.TenantID, application.ChargeCorrectionCommand, error)
	IntakeChargeRestate(ctx context.Context, request *http.Request) (domain.TenantID, application.ChargeCorrectionCommand, error)
}

func NewRegisterChargeUnitsEndpoint(intake ChargeIntake, charges ChargeService) http.Handler {
	return chargeEndpoint(func(request *http.Request) (application.ChargeResult, error) {
		tenant, command, err := intake.IntakeChargeUnits(request.Context(), request)
		if err != nil {
			return application.ChargeResult{}, err
		}
		return charges.RegisterUnits(request.Context(), tenant, command.Warehouse, command.Weight, command.Volume)
	})
}

func NewRegisterChargeAgingEndpoint(intake ChargeIntake, charges ChargeService) http.Handler {
	return chargeEndpoint(func(request *http.Request) (application.ChargeResult, error) {
		tenant, command, err := intake.IntakeChargeAging(request.Context(), request)
		if err != nil {
			return application.ChargeResult{}, err
		}
		return charges.RegisterAging(request.Context(), tenant, command.Warehouse, command.Basis)
	})
}

func NewRegisterChargeProjectEndpoint(intake ChargeIntake, charges ChargeService) http.Handler {
	return chargeEndpoint(func(request *http.Request) (application.ChargeResult, error) {
		tenant, command, err := intake.IntakeChargeProject(request.Context(), request)
		if err != nil {
			return application.ChargeResult{}, err
		}
		return charges.RegisterProject(request.Context(), tenant, command.Warehouse, command.Name)
	})
}

func NewCloseStorageDayEndpoint(intake ChargeIntake, charges ChargeService) http.Handler {
	return chargeEndpoint(func(request *http.Request) (application.ChargeResult, error) {
		tenant, command, err := intake.IntakeStorageDay(request.Context(), request)
		if err != nil {
			return application.ChargeResult{}, err
		}
		return charges.CloseStorageDay(request.Context(), tenant, command.Owner, command.Warehouse, command.Day)
	})
}

func NewVoidChargeOccurrenceEndpoint(intake ChargeIntake, charges ChargeService) http.Handler {
	return chargeEndpoint(func(request *http.Request) (application.ChargeResult, error) {
		tenant, command, err := intake.IntakeChargeVoid(request.Context(), request)
		if err != nil {
			return application.ChargeResult{}, err
		}
		return charges.Void(request.Context(), tenant, command.Occurrence, command.Version)
	})
}

func NewRestateChargeOccurrenceEndpoint(intake ChargeIntake, charges ChargeService) http.Handler {
	return chargeEndpoint(func(request *http.Request) (application.ChargeResult, error) {
		tenant, command, err := intake.IntakeChargeRestate(request.Context(), request)
		if err != nil {
			return application.ChargeResult{}, err
		}
		return charges.Restate(request.Context(), tenant, command.Occurrence, command.Version, command.Draft)
	})
}

func chargeEndpoint(run func(*http.Request) (application.ChargeResult, error)) http.Handler {
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
		name := result.Outcome().String()
		if name == "" {
			httpapi.WriteProblem(response, request, http.StatusInternalServerError, codeUnnamedOutcome)
			return
		}
		status := http.StatusOK
		switch result.Outcome() {
		case application.ChargeFormed, application.ChargeRestated, application.ChargeVoided:
			status = http.StatusCreated
		}
		writeJSON(response, status, chargeAnswer{Outcome: name, OccurrenceID: result.Occurrence(), Version: result.Version()})
	})
}

type chargeAnswer struct {
	Outcome      string `json:"outcome"`
	OccurrenceID string `json:"occurrenceId,omitempty"`
	Version      int    `json:"version,omitempty"`
}
