package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// SupplementOutcome 是「委托待受控补充」落到装箱记录上的答案。
type SupplementOutcome uint8

const (
	SupplementOutcomeInvalid SupplementOutcome = iota
	SupplementAwaiting
	SupplementPosted
	SupplementExisting
	SupplementConflict
	SupplementContentInvalid
)

func (outcome SupplementOutcome) String() string {
	switch outcome {
	case SupplementAwaiting:
		return "AWAITING_SUPPLEMENT"
	case SupplementPosted:
		return "POSTED"
	case SupplementExisting:
		return "EXISTING_RESULT"
	case SupplementConflict:
		return "CONTENT_CONFLICT"
	case SupplementContentInvalid:
		return "CONTENT_INVALID"
	default:
		return ""
	}
}

// SupplementResult 是一次待补充入账。
type SupplementResult struct {
	outcome SupplementOutcome
}

func (result SupplementResult) Outcome() SupplementOutcome { return result.outcome }

// SupplementCommand 是一封待受控补充信封上仓配要的引用。缺口留在小包托运的任务上，这里不复写。
type SupplementCommand struct {
	Tenant            domain.TenantID
	SubmissionVersion string
	Source            string
	SourceRequestKey  string
}

type supplementStore interface {
	LoadSupplement(ctx context.Context, tenant domain.TenantID, version string) (source, key string, found bool, err error)
	SaveSupplement(ctx context.Context, tenant domain.TenantID, version, source, key string, at time.Time) error
	ParcelsOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) ([]ports.PackParcel, error)
	SetCommissionState(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, state string) error
}

// SupplementHandler 按待受控补充信封把仓配出库的包裹标成待货主补正。别的来源入账收工。还不代货主提交新版本。
type SupplementHandler struct {
	store supplementStore
	clock interface{ Now() time.Time }
}

func NewSupplementHandler(store supplementStore, clock interface{ Now() time.Time }) *SupplementHandler {
	return &SupplementHandler{store: store, clock: clock}
}

func (handler *SupplementHandler) Record(ctx context.Context, command SupplementCommand) (SupplementResult, error) {
	if command.Tenant.String() == "" || strings.TrimSpace(command.SubmissionVersion) == "" || strings.TrimSpace(command.Source) == "" || strings.TrimSpace(command.SourceRequestKey) == "" {
		return SupplementResult{outcome: SupplementContentInvalid}, nil
	}
	source, key, found, err := handler.store.LoadSupplement(ctx, command.Tenant, command.SubmissionVersion)
	if err != nil {
		return SupplementResult{}, fmt.Errorf("record supplement: %w", err)
	}
	if found {
		if source == command.Source && key == command.SourceRequestKey {
			return SupplementResult{outcome: SupplementExisting}, nil
		}
		return SupplementResult{outcome: SupplementConflict}, nil
	}
	if command.Source != domain.WarehouseOutboundSource {
		if err := handler.store.SaveSupplement(ctx, command.Tenant, command.SubmissionVersion, command.Source, command.SourceRequestKey, handler.clock.Now().UTC()); err != nil {
			return SupplementResult{}, fmt.Errorf("record supplement: %w", err)
		}
		return SupplementResult{outcome: SupplementPosted}, nil
	}
	requestID, sequence, ok := splitCommissionKey(command.SourceRequestKey)
	if !ok {
		return SupplementResult{outcome: SupplementContentInvalid}, nil
	}
	request, err := domain.NewOutboundRequestID(requestID)
	if err != nil {
		return SupplementResult{outcome: SupplementContentInvalid}, nil
	}
	parcels, err := handler.store.ParcelsOf(ctx, command.Tenant, request)
	if err != nil {
		return SupplementResult{}, fmt.Errorf("record supplement: %w", err)
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
	if !seen || parcel.ShipmentRequestID == "" {
		return SupplementResult{}, fmt.Errorf("record supplement: parcel %s/%d is not submitted", requestID, sequence)
	}
	if parcel.CommissionState != "" && parcel.CommissionState != "AWAITING_SUPPLEMENT" {
		return SupplementResult{outcome: SupplementConflict}, nil
	}
	if parcel.CommissionState == "" {
		if err := handler.store.SetCommissionState(ctx, command.Tenant, request, sequence, "AWAITING_SUPPLEMENT"); err != nil {
			return SupplementResult{}, fmt.Errorf("record supplement: %w", err)
		}
	}
	if err := handler.store.SaveSupplement(ctx, command.Tenant, command.SubmissionVersion, command.Source, command.SourceRequestKey, handler.clock.Now().UTC()); err != nil {
		return SupplementResult{}, fmt.Errorf("record supplement: %w", err)
	}
	return SupplementResult{outcome: SupplementAwaiting}, nil
}
