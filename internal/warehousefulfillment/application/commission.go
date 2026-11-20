package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// CommissionOutcome 是打包之后提交委托的答案。自备面单与自提不形成委托。
type CommissionOutcome uint8

const (
	CommissionOutcomeInvalid CommissionOutcome = iota
	CommissionSubmitted
	CommissionExisting
	CommissionNotConfigured
	CommissionSkipped
	CommissionStatusMismatch
	CommissionNotFound
	CommissionConflict
	CommissionUnresolved
	CommissionPaused
	CommissionOtherAuthority
	CommissionInputRejected
	CommissionIngressConflict
)

func (outcome CommissionOutcome) String() string {
	switch outcome {
	case CommissionSubmitted:
		return ports.CommissionSubmittedWord
	case CommissionExisting:
		return ports.CommissionExistingWord
	case CommissionNotConfigured:
		return ports.CommissionNotConfiguredWord
	case CommissionSkipped:
		return "NO_COMMISSION"
	case CommissionStatusMismatch:
		return "STATUS_MISMATCH"
	case CommissionNotFound:
		return "WAVE_NOT_FOUND"
	case CommissionConflict:
		return "CONTENT_CONFLICT"
	case CommissionUnresolved:
		return ports.CommissionUnresolvedWord
	case CommissionPaused:
		return ports.CommissionPausedWord
	case CommissionOtherAuthority:
		return ports.CommissionOtherAuthorityWord
	case CommissionInputRejected:
		return ports.CommissionInputRejectedWord
	case CommissionIngressConflict:
		return ports.CommissionIngressConflictWord
	default:
		return ""
	}
}

// CommissionResult 是一次代办提交。
type CommissionResult struct {
	outcome CommissionOutcome
}

func (result CommissionResult) Outcome() CommissionOutcome { return result.outcome }

// AfterCommission 把打包的答案换成委托提交的答案。自备面单与自提保持打包的原答案。
func (result WaveResult) AfterCommission(commission CommissionResult) WaveResult {
	switch commission.outcome {
	case CommissionSkipped:
		return result
	case CommissionSubmitted:
		return WaveResult{outcome: WaveSubmitted}
	case CommissionExisting:
		return WaveResult{outcome: WaveExisting}
	case CommissionNotConfigured:
		return WaveResult{outcome: WaveNotConfigured}
	case CommissionStatusMismatch:
		return WaveResult{outcome: WaveStatusMismatch}
	case CommissionNotFound:
		return WaveResult{outcome: WaveNotFound}
	case CommissionConflict, CommissionIngressConflict:
		return WaveResult{outcome: WaveConflict}
	case CommissionUnresolved:
		return WaveResult{outcome: WaveUnresolved}
	case CommissionPaused:
		return WaveResult{outcome: WavePaused}
	case CommissionOtherAuthority:
		return WaveResult{outcome: WaveOtherAuthority}
	case CommissionInputRejected:
		return WaveResult{outcome: WaveInputRejected}
	default:
		return WaveResult{}
	}
}

type commissionRequests interface {
	LoadByID(ctx context.Context, tenant domain.TenantID, id domain.OutboundRequestID) (domain.OutboundRequest, bool, error)
}

type commissionParcels interface {
	ParcelsOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) ([]ports.PackParcel, error)
	MarkShipment(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, shipmentRequestID string) error
}

// CommissionHandler 在出库请求打包之后，为本产品网络的每个包裹提交一份委托。
// 来源请求键是（出库请求，包裹序号）。代办授予没登记时答未配置，不建委托。预取不在这里：放波次只记下选择。
type CommissionHandler struct {
	requests     commissionRequests
	parcels      commissionParcels
	commissioner ports.OutboundCommissioner
	anyWarehouse ports.TenantWarehouseLookup
}

func NewCommissionHandler(
	requests commissionRequests,
	parcels commissionParcels,
	commissioner ports.OutboundCommissioner,
	anyWarehouse ports.TenantWarehouseLookup,
) *CommissionHandler {
	return &CommissionHandler{requests: requests, parcels: parcels, commissioner: commissioner, anyWarehouse: anyWarehouse}
}

func (handler *CommissionHandler) Submit(ctx context.Context, tenant domain.TenantID, requestID domain.OutboundRequestID) (CommissionResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, tenant)
	if err != nil {
		return CommissionResult{}, fmt.Errorf("submit outbound commission: %w", err)
	}
	if !configured {
		return CommissionResult{outcome: CommissionNotConfigured}, nil
	}
	request, found, err := handler.requests.LoadByID(ctx, tenant, requestID)
	if err != nil {
		return CommissionResult{}, fmt.Errorf("submit outbound commission: %w", err)
	}
	if !found {
		return CommissionResult{outcome: CommissionNotFound}, nil
	}
	if request.Dispatch() != domain.DispatchProductNetwork {
		return CommissionResult{outcome: CommissionSkipped}, nil
	}
	if request.Status() != domain.OutboundPacked {
		return CommissionResult{outcome: CommissionStatusMismatch}, nil
	}
	parcels, err := handler.parcels.ParcelsOf(ctx, tenant, requestID)
	if err != nil {
		return CommissionResult{}, fmt.Errorf("submit outbound commission: %w", err)
	}
	if len(parcels) == 0 {
		return CommissionResult{outcome: CommissionStatusMismatch}, nil
	}
	pending := false
	for _, parcel := range parcels {
		if parcel.ShipmentRequestID == "" {
			pending = true
			break
		}
	}
	if !pending {
		return CommissionResult{outcome: CommissionExisting}, nil
	}
	for _, parcel := range parcels {
		if parcel.ShipmentRequestID != "" {
			continue
		}
		answer, err := handler.commissioner.Submit(ctx, ports.CommissionSubmission{
			Tenant: tenant.String(), Owner: request.Owner().String(), Request: requestID.String(), Sequence: parcel.Sequence,
			Service: request.Service(), Recipient: request.Recipient(), Address: request.Address(),
			Parcel: ports.CommissionParcel{
				Sequence: parcel.Sequence, Weight: parcel.Weight, WeightUnit: parcel.WeightUnit,
				Length: parcel.Length, Width: parcel.Width, Height: parcel.Height, Dimension: parcel.Dimension,
				Goods: parcel.Goods.String(), Unit: parcel.Unit.String(), Quantity: parcel.Quantity,
			},
		})
		if err != nil {
			return CommissionResult{}, fmt.Errorf("submit outbound commission: %w", err)
		}
		outcome := commissionOutcomeOf(answer.Outcome)
		if outcome == CommissionOutcomeInvalid {
			return CommissionResult{}, fmt.Errorf("submit outbound commission: unnamed outcome %q", answer.Outcome)
		}
		if outcome != CommissionSubmitted && outcome != CommissionExisting {
			return CommissionResult{outcome: outcome}, nil
		}
		if answer.ShipmentRequestID == "" {
			return CommissionResult{}, fmt.Errorf("submit outbound commission: %s named no shipment request", answer.Outcome)
		}
		if err := handler.parcels.MarkShipment(ctx, tenant, requestID, parcel.Sequence, answer.ShipmentRequestID); err != nil {
			return CommissionResult{}, fmt.Errorf("submit outbound commission: %w", err)
		}
	}
	return CommissionResult{outcome: CommissionSubmitted}, nil
}

func commissionOutcomeOf(word string) CommissionOutcome {
	switch word {
	case ports.CommissionSubmittedWord:
		return CommissionSubmitted
	case ports.CommissionExistingWord:
		return CommissionExisting
	case ports.CommissionNotConfiguredWord:
		return CommissionNotConfigured
	case ports.CommissionUnresolvedWord:
		return CommissionUnresolved
	case ports.CommissionPausedWord:
		return CommissionPaused
	case ports.CommissionOtherAuthorityWord:
		return CommissionOtherAuthority
	case ports.CommissionInputRejectedWord:
		return CommissionInputRejected
	case ports.CommissionIngressConflictWord:
		return CommissionIngressConflict
	default:
		return CommissionOutcomeInvalid
	}
}
