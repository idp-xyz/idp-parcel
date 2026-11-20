package application

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// AcceptanceOutcome 是接受决定信封落到装箱记录上的答案。
type AcceptanceOutcome uint8

const (
	AcceptanceOutcomeInvalid AcceptanceOutcome = iota
	AcceptanceAccepted
	AcceptanceRejected
	AcceptancePosted
	AcceptanceExisting
	AcceptanceConflict
	AcceptanceNotFound
	AcceptanceStatusMismatch
	AcceptanceContentInvalid
)

func (outcome AcceptanceOutcome) String() string {
	switch outcome {
	case AcceptanceAccepted:
		return "ACCEPTED"
	case AcceptanceRejected:
		return "REJECTED"
	case AcceptancePosted:
		return "POSTED"
	case AcceptanceExisting:
		return "EXISTING_RESULT"
	case AcceptanceConflict:
		return "CONTENT_CONFLICT"
	case AcceptanceNotFound:
		return "WAVE_NOT_FOUND"
	case AcceptanceStatusMismatch:
		return "STATUS_MISMATCH"
	case AcceptanceContentInvalid:
		return "CONTENT_INVALID"
	default:
		return ""
	}
}

// AcceptanceResult 是一次接受决定入账。
type AcceptanceResult struct {
	outcome AcceptanceOutcome
}

func (result AcceptanceResult) Outcome() AcceptanceOutcome { return result.outcome }

// AcceptanceCommand 是一封接受决定信封上仓配要的那几格。不带委托内部状态以外的决定正文。
type AcceptanceCommand struct {
	Tenant           domain.TenantID
	DecisionID       string
	Source           string
	SourceRequestKey string
	State            string
}

type acceptanceStore interface {
	LoadAcceptance(ctx context.Context, tenant domain.TenantID, decisionID string) (source, key, state string, found bool, err error)
	SaveAcceptance(ctx context.Context, tenant domain.TenantID, decisionID, source, key, state string, at time.Time) error
	ParcelsOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) ([]ports.PackParcel, error)
	SetCommissionState(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, state string) error
}

// AcceptanceHandler 按接受决定信封推进装箱记录。来源不是仓配出库的入账收工。不读委托内部状态。
type AcceptanceHandler struct {
	store acceptanceStore
	clock interface{ Now() time.Time }
}

func NewAcceptanceHandler(store acceptanceStore, clock interface{ Now() time.Time }) *AcceptanceHandler {
	return &AcceptanceHandler{store: store, clock: clock}
}

func (handler *AcceptanceHandler) Record(ctx context.Context, command AcceptanceCommand) (AcceptanceResult, error) {
	if command.Tenant.String() == "" || strings.TrimSpace(command.DecisionID) == "" || strings.TrimSpace(command.Source) == "" || strings.TrimSpace(command.SourceRequestKey) == "" {
		return AcceptanceResult{outcome: AcceptanceContentInvalid}, nil
	}
	if command.State != "ACCEPTED" && command.State != "REJECTED" {
		return AcceptanceResult{outcome: AcceptanceContentInvalid}, nil
	}
	source, key, state, found, err := handler.store.LoadAcceptance(ctx, command.Tenant, command.DecisionID)
	if err != nil {
		return AcceptanceResult{}, fmt.Errorf("record acceptance: %w", err)
	}
	if found {
		if source == command.Source && key == command.SourceRequestKey && state == command.State {
			return AcceptanceResult{outcome: AcceptanceExisting}, nil
		}
		return AcceptanceResult{outcome: AcceptanceConflict}, nil
	}
	if command.Source != domain.WarehouseOutboundSource {
		if err := handler.store.SaveAcceptance(ctx, command.Tenant, command.DecisionID, command.Source, command.SourceRequestKey, command.State, handler.clock.Now().UTC()); err != nil {
			return AcceptanceResult{}, fmt.Errorf("record acceptance: %w", err)
		}
		return AcceptanceResult{outcome: AcceptancePosted}, nil
	}
	requestID, sequence, ok := splitCommissionKey(command.SourceRequestKey)
	if !ok {
		return AcceptanceResult{outcome: AcceptanceContentInvalid}, nil
	}
	request, err := domain.NewOutboundRequestID(requestID)
	if err != nil {
		return AcceptanceResult{outcome: AcceptanceContentInvalid}, nil
	}
	parcels, err := handler.store.ParcelsOf(ctx, command.Tenant, request)
	if err != nil {
		return AcceptanceResult{}, fmt.Errorf("record acceptance: %w", err)
	}
	var parcel ports.PackParcel
	seen := false
	for _, item := range parcels {
		if item.Sequence == sequence {
			parcel = item
			seen = true
			break
		}
	}
	if !seen {
		return AcceptanceResult{}, fmt.Errorf("record acceptance: parcel %s/%d is not packed", requestID, sequence)
	}
	if parcel.ShipmentRequestID == "" {
		return AcceptanceResult{}, fmt.Errorf("record acceptance: parcel %s/%d has no shipment", requestID, sequence)
	}
	if parcel.CommissionState != "" && parcel.CommissionState != command.State {
		return AcceptanceResult{outcome: AcceptanceConflict}, nil
	}
	if parcel.CommissionState == "" {
		if err := handler.store.SetCommissionState(ctx, command.Tenant, request, sequence, command.State); err != nil {
			return AcceptanceResult{}, fmt.Errorf("record acceptance: %w", err)
		}
	}
	if err := handler.store.SaveAcceptance(ctx, command.Tenant, command.DecisionID, command.Source, command.SourceRequestKey, command.State, handler.clock.Now().UTC()); err != nil {
		return AcceptanceResult{}, fmt.Errorf("record acceptance: %w", err)
	}
	if command.State == "REJECTED" {
		return AcceptanceResult{outcome: AcceptanceRejected}, nil
	}
	return AcceptanceResult{outcome: AcceptanceAccepted}, nil
}

func splitCommissionKey(key string) (string, int, bool) {
	cut := strings.LastIndex(key, "/")
	if cut <= 0 || cut == len(key)-1 {
		return "", 0, false
	}
	sequence, err := strconv.Atoi(key[cut+1:])
	if err != nil || sequence < 1 {
		return "", 0, false
	}
	return key[:cut], sequence, true
}
