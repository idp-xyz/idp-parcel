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
	MarkShipment(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, shipmentRequestID, declaredParcelID string) error
	LoadPrefetch(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) (ports.PrefetchRecord, bool, error)
	SetCorrection(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, state string) error
}

// CommissionHandler 在出库请求打包之后提交委托。没标平台备货时每个包裹一份，来源请求键是（出库请求，包裹序号）。
// 标了平台备货则等全部包裹都已打包，按（出库请求，提交轮次）交一份。代办授予没登记时答未配置，不建委托。
// 预取过的包裹序号 1 沿用放波次交出去的那份委托，不再另交；打包称重与估算不同时提出声明测量修订，未采用前停在待复核。
type CommissionHandler struct {
	requests     commissionRequests
	parcels      commissionParcels
	commissioner ports.OutboundCommissioner
	amender      ports.MeasurementAmender
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

// WithMeasurementAmender 接上预取包裹打包称重不同时的资料修订口。没接时遇差异标待复核，不出库确认。
func (handler *CommissionHandler) WithMeasurementAmender(amender ports.MeasurementAmender) *CommissionHandler {
	handler.amender = amender
	return handler
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
	if request.PlatformStocking() {
		return handler.submitPlatform(ctx, tenant, request, requestID, parcels)
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
	prefetched, foundPrefetch, err := handler.parcels.LoadPrefetch(ctx, tenant, requestID)
	if err != nil {
		return CommissionResult{}, fmt.Errorf("submit outbound commission: %w", err)
	}
	for _, parcel := range parcels {
		if parcel.ShipmentRequestID != "" {
			continue
		}
		if foundPrefetch && parcel.Sequence == 1 {
			if err := handler.parcels.MarkShipment(ctx, tenant, requestID, parcel.Sequence, prefetched.ShipmentID, prefetched.DeclaredParcelID); err != nil {
				return CommissionResult{}, fmt.Errorf("submit outbound commission: %w", err)
			}
			if !samePrefetchMeasurement(prefetched, parcel) {
				if err := handler.revisePrefetchMeasurement(ctx, tenant, request, requestID, prefetched, parcel); err != nil {
					return CommissionResult{}, fmt.Errorf("submit outbound commission: %w", err)
				}
			}
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
		if err := handler.parcels.MarkShipment(ctx, tenant, requestID, parcel.Sequence, answer.ShipmentRequestID, answer.DeclaredParcelID); err != nil {
			return CommissionResult{}, fmt.Errorf("submit outbound commission: %w", err)
		}
	}
	return CommissionResult{outcome: CommissionSubmitted}, nil
}

func (handler *CommissionHandler) submitPlatform(
	ctx context.Context,
	tenant domain.TenantID,
	request domain.OutboundRequest,
	requestID domain.OutboundRequestID,
	parcels []ports.PackParcel,
) (CommissionResult, error) {
	pending := 0
	packed := make([]ports.CommissionParcel, 0, len(parcels))
	for _, parcel := range parcels {
		if parcel.ShipmentRequestID == "" {
			pending++
		}
		packed = append(packed, commissionParcelOf(parcel))
	}
	if pending == 0 {
		return CommissionResult{outcome: CommissionExisting}, nil
	}
	if pending != len(parcels) {
		return CommissionResult{}, fmt.Errorf("submit outbound commission: platform stocking left some parcels unmarked")
	}
	answer, err := handler.commissioner.Submit(ctx, ports.CommissionSubmission{
		Tenant: tenant.String(), Owner: request.Owner().String(), Request: requestID.String(), Round: 1,
		Service: request.Service(), Recipient: request.Recipient(), Address: request.Address(), Parcels: packed,
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
	members := answer.Members
	if len(members) == 0 && len(parcels) == 1 {
		members = []ports.CommissionMember{{Sequence: parcels[0].Sequence, DeclaredParcelID: answer.DeclaredParcelID}}
	}
	if len(members) != len(parcels) {
		return CommissionResult{}, fmt.Errorf("submit outbound commission: platform stocking named %d parcels, packed %d", len(members), len(parcels))
	}
	for _, member := range members {
		if err := handler.parcels.MarkShipment(ctx, tenant, requestID, member.Sequence, answer.ShipmentRequestID, member.DeclaredParcelID); err != nil {
			return CommissionResult{}, fmt.Errorf("submit outbound commission: %w", err)
		}
	}
	return CommissionResult{outcome: CommissionSubmitted}, nil
}

func commissionParcelOf(parcel ports.PackParcel) ports.CommissionParcel {
	return ports.CommissionParcel{
		Sequence: parcel.Sequence, Weight: parcel.Weight, WeightUnit: parcel.WeightUnit,
		Length: parcel.Length, Width: parcel.Width, Height: parcel.Height, Dimension: parcel.Dimension,
		Goods: parcel.Goods.String(), Unit: parcel.Unit.String(), Quantity: parcel.Quantity,
	}
}

func (handler *CommissionHandler) revisePrefetchMeasurement(
	ctx context.Context,
	tenant domain.TenantID,
	request domain.OutboundRequest,
	requestID domain.OutboundRequestID,
	prefetched ports.PrefetchRecord,
	parcel ports.PackParcel,
) error {
	// 没接资料修订口或预取没留下包裹标识时，称重差异不能假装已采用，停在待复核挡住在线出库确认。
	state := ports.MeasurementAwaitingReviewWord
	if handler.amender != nil && prefetched.DeclaredParcelID != "" {
		answer, err := handler.amender.AmendMeasurement(ctx, ports.MeasurementAmendment{
			Tenant: tenant.String(), Owner: request.Owner().String(), Request: requestID.String(), Sequence: parcel.Sequence,
			ShipmentRequest: prefetched.ShipmentID, DeclaredParcelID: prefetched.DeclaredParcelID,
			Weight: parcel.Weight, WeightUnit: parcel.WeightUnit,
			Length: parcel.Length, Width: parcel.Width, Height: parcel.Height, Dimension: parcel.Dimension,
		})
		if err != nil {
			return err
		}
		if answer.Outcome == ports.MeasurementAdoptedWord {
			state = ports.MeasurementAdoptedWord
		}
	}
	return handler.parcels.SetCorrection(ctx, tenant, requestID, parcel.Sequence, state)
}

func samePrefetchMeasurement(prefetched ports.PrefetchRecord, parcel ports.PackParcel) bool {
	if prefetched.Weight == "" {
		return true
	}
	return prefetched.WeightUnit == parcel.WeightUnit &&
		prefetched.Dimension == parcel.Dimension &&
		sameMeasureText(prefetched.Weight, parcel.Weight) &&
		sameMeasureText(prefetched.Length, parcel.Length) &&
		sameMeasureText(prefetched.Width, parcel.Width) &&
		sameMeasureText(prefetched.Height, parcel.Height)
}

func sameMeasureText(left, right string) bool {
	if left == right {
		return true
	}
	a, errA := domain.NewMeasure(left)
	b, errB := domain.NewMeasure(right)
	if errA != nil || errB != nil {
		return false
	}
	return a == b
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
