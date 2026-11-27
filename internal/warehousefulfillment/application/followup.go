package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// FollowUpOutcome 是资料版本、包裹取消或补正再提交的答案。
type FollowUpOutcome uint8

const (
	FollowUpOutcomeInvalid FollowUpOutcome = iota
	FollowUpAdopted
	FollowUpAwaitingReview
	FollowUpCancelled
	FollowUpPosted
	FollowUpExisting
	FollowUpConflict
	FollowUpRecorded
	FollowUpNotConfigured
	FollowUpStatusMismatch
	FollowUpContentInvalid
)

func (outcome FollowUpOutcome) String() string {
	switch outcome {
	case FollowUpAdopted:
		return "ADOPTED"
	case FollowUpAwaitingReview:
		return "AWAITING_REVIEW"
	case FollowUpCancelled:
		return "CANCELLED"
	case FollowUpPosted:
		return "POSTED"
	case FollowUpExisting:
		return "EXISTING_RESULT"
	case FollowUpConflict:
		return "CONTENT_CONFLICT"
	case FollowUpRecorded:
		return "SUPPLEMENT_RECORDED"
	case FollowUpNotConfigured:
		return "NOT_CONFIGURED"
	case FollowUpStatusMismatch:
		return "STATUS_MISMATCH"
	case FollowUpContentInvalid:
		return "CONTENT_INVALID"
	default:
		return ""
	}
}

type FollowUpResult struct{ outcome FollowUpOutcome }

func (result FollowUpResult) Outcome() FollowUpOutcome { return result.outcome }

type followUpStore interface {
	LoadSourceData(ctx context.Context, tenant domain.TenantID, version string) (source, key, adoption string, found bool, err error)
	SaveSourceData(ctx context.Context, tenant domain.TenantID, version, source, key, adoption string, at time.Time) error
	// HasAwaitingSourceData 回答同一个来源请求键上，除了 except 这一版之外还有没有记成待复核的资料版本。
	HasAwaitingSourceData(ctx context.Context, tenant domain.TenantID, source, key, except string) (bool, error)
	SetCorrection(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, state string) error
	LoadCancellation(ctx context.Context, tenant domain.TenantID, requestKey, parcelID string) (matched bool, found bool, err error)
	SaveCancellation(ctx context.Context, tenant domain.TenantID, requestKey, parcelID string, matched bool, at time.Time) error
	ParcelByDeclared(ctx context.Context, tenant domain.TenantID, declaredParcelID string) (domain.OutboundRequestID, int, bool, error)
	SetCommissionState(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, state string) error
	LoadBasis(ctx context.Context, tenant domain.TenantID, sourceKey string) (version string, found bool, err error)
	ParcelsOf(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID) ([]ports.PackParcel, error)
	ClearAwaiting(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int) error
}

type outboundReviser interface {
	Revise(ctx context.Context, revision ports.CommissionRevision) (ports.CommissionAnswer, error)
}

// FollowUpHandler 消费资料版本与包裹取消，并在货主补正后代交新提交版本。
type FollowUpHandler struct {
	requests commissionRequests
	store    followUpStore
	reviser  outboundReviser
	clock    interface{ Now() time.Time }
}

func NewFollowUpHandler(requests commissionRequests, store followUpStore, reviser outboundReviser, clock interface{ Now() time.Time }) *FollowUpHandler {
	return &FollowUpHandler{requests: requests, store: store, reviser: reviser, clock: clock}
}

type SourceDataCommand struct {
	Tenant           domain.TenantID
	Version          string
	Source           string
	SourceRequestKey string
	Adoption         string
}

func (handler *FollowUpHandler) RecordSourceData(ctx context.Context, command SourceDataCommand) (FollowUpResult, error) {
	if command.Tenant.String() == "" || command.Version == "" || command.Source == "" || command.SourceRequestKey == "" ||
		(command.Adoption != "ADOPTED" && command.Adoption != "AWAITING_REVIEW") {
		return FollowUpResult{outcome: FollowUpContentInvalid}, nil
	}
	source, key, adoption, found, err := handler.store.LoadSourceData(ctx, command.Tenant, command.Version)
	if err != nil {
		return FollowUpResult{}, err
	}
	if found {
		if source == command.Source && key == command.SourceRequestKey && adoption == command.Adoption {
			return FollowUpResult{outcome: FollowUpExisting}, nil
		}
		return FollowUpResult{outcome: FollowUpConflict}, nil
	}
	if command.Source == domain.WarehouseOutboundSource {
		requestID, sequence, ok := splitCommissionKey(command.SourceRequestKey)
		if !ok {
			return FollowUpResult{outcome: FollowUpContentInvalid}, nil
		}
		request, err := domain.NewOutboundRequestID(requestID)
		if err != nil {
			return FollowUpResult{outcome: FollowUpContentInvalid}, nil
		}
		// 一个包裹可以同时有两份修订在途（件数与测量各一份）。这一份已采用时，另一份还在待复核的话，包裹仍是待复核：
		// 两样都采用了才算更正到位，后到的一份「已采用」不该把先到的那份「待复核」盖掉。
		state := command.Adoption
		if state == "ADOPTED" {
			held, err := handler.store.HasAwaitingSourceData(ctx, command.Tenant, command.Source, command.SourceRequestKey, command.Version)
			if err != nil {
				return FollowUpResult{}, err
			}
			if held {
				state = "AWAITING_REVIEW"
			}
		}
		if err := handler.store.SetCorrection(ctx, command.Tenant, request, sequence, state); err != nil {
			return FollowUpResult{}, err
		}
	}
	if err := handler.store.SaveSourceData(ctx, command.Tenant, command.Version, command.Source, command.SourceRequestKey, command.Adoption, handler.clock.Now().UTC()); err != nil {
		return FollowUpResult{}, err
	}
	if command.Source != domain.WarehouseOutboundSource {
		return FollowUpResult{outcome: FollowUpPosted}, nil
	}
	if command.Adoption == "ADOPTED" {
		return FollowUpResult{outcome: FollowUpAdopted}, nil
	}
	return FollowUpResult{outcome: FollowUpAwaitingReview}, nil
}

type CancellationCommand struct {
	Tenant     domain.TenantID
	RequestKey string
	ParcelID   string
}

func (handler *FollowUpHandler) RecordCancellation(ctx context.Context, command CancellationCommand) (FollowUpResult, error) {
	if command.Tenant.String() == "" || command.RequestKey == "" || command.ParcelID == "" {
		return FollowUpResult{outcome: FollowUpContentInvalid}, nil
	}
	matched, found, err := handler.store.LoadCancellation(ctx, command.Tenant, command.RequestKey, command.ParcelID)
	if err != nil {
		return FollowUpResult{}, err
	}
	if found {
		return FollowUpResult{outcome: FollowUpExisting}, nil
	}
	request, sequence, parcelFound, err := handler.store.ParcelByDeclared(ctx, command.Tenant, command.ParcelID)
	if err != nil {
		return FollowUpResult{}, err
	}
	if parcelFound {
		if err := handler.store.SetCommissionState(ctx, command.Tenant, request, sequence, "CANCELLED"); err != nil {
			return FollowUpResult{}, err
		}
	}
	if err := handler.store.SaveCancellation(ctx, command.Tenant, command.RequestKey, command.ParcelID, parcelFound, handler.clock.Now().UTC()); err != nil {
		return FollowUpResult{}, err
	}
	if !parcelFound {
		return FollowUpResult{outcome: FollowUpPosted}, nil
	}
	_ = matched
	return FollowUpResult{outcome: FollowUpCancelled}, nil
}

type ResubmitCommand struct {
	Tenant     domain.TenantID
	Owner      domain.OwnerAccountID
	Request    domain.OutboundRequestID
	Sequence   int
	RequestKey string
}

func (handler *FollowUpHandler) Resubmit(ctx context.Context, command ResubmitCommand) (FollowUpResult, error) {
	if handler.reviser == nil || command.Tenant.String() == "" || command.Owner.String() == "" || command.Request.String() == "" || command.Sequence < 1 || strings.TrimSpace(command.RequestKey) == "" {
		return FollowUpResult{outcome: FollowUpContentInvalid}, nil
	}
	request, found, err := handler.requests.LoadByID(ctx, command.Tenant, command.Request)
	if err != nil {
		return FollowUpResult{}, err
	}
	if !found || request.Owner() != command.Owner {
		return FollowUpResult{outcome: FollowUpContentInvalid}, nil
	}
	parcels, err := handler.store.ParcelsOf(ctx, command.Tenant, command.Request)
	if err != nil {
		return FollowUpResult{}, err
	}
	var parcel ports.PackParcel
	seen := false
	for _, item := range parcels {
		if item.Sequence == command.Sequence {
			parcel = item
			seen = true
		}
	}
	if !seen || parcel.CommissionState != "AWAITING_SUPPLEMENT" || parcel.ShipmentRequestID == "" || parcel.DeclaredParcelID == "" {
		return FollowUpResult{outcome: FollowUpStatusMismatch}, nil
	}
	basis, basisFound, err := handler.store.LoadBasis(ctx, command.Tenant, command.Request.String()+"/"+strconvItoa(command.Sequence))
	if err != nil {
		return FollowUpResult{}, err
	}
	if !basisFound {
		return FollowUpResult{outcome: FollowUpStatusMismatch}, nil
	}
	answer, err := handler.reviser.Revise(ctx, ports.CommissionRevision{
		CommissionSubmission: ports.CommissionSubmission{
			Tenant: command.Tenant.String(), Owner: command.Owner.String(), Request: command.Request.String(), Sequence: command.Sequence,
			Service: request.Service(), Recipient: request.Recipient(), Address: request.Address(),
			Parcel: ports.CommissionParcel{
				Sequence: parcel.Sequence, Weight: parcel.Weight, WeightUnit: parcel.WeightUnit,
				Length: parcel.Length, Width: parcel.Width, Height: parcel.Height, Dimension: parcel.Dimension,
				Goods: parcel.Goods.String(), Unit: parcel.Unit.String(), Quantity: parcel.Quantity,
			},
		},
		RequestKey: command.RequestKey, BasisVersion: basis, ShipmentRequest: parcel.ShipmentRequestID, DeclaredParcelID: parcel.DeclaredParcelID,
	})
	if err != nil {
		return FollowUpResult{}, err
	}
	switch answer.Outcome {
	case "SUPPLEMENT_RECORDED":
		if err := handler.store.ClearAwaiting(ctx, command.Tenant, command.Request, command.Sequence); err != nil {
			return FollowUpResult{}, err
		}
		return FollowUpResult{outcome: FollowUpRecorded}, nil
	case "SUPPLEMENT_ALREADY_HANDLED":
		return FollowUpResult{outcome: FollowUpExisting}, nil
	case ports.CommissionNotConfiguredWord:
		return FollowUpResult{outcome: FollowUpNotConfigured}, nil
	case "SUPPLEMENT_BASIS_STALE", "SUPPLEMENT_SOURCE_CONFLICT", "SUPPLEMENT_BOUNDARY_CHANGED", "SUPPLEMENT_DECISION_ALREADY_FORMED":
		return FollowUpResult{outcome: FollowUpConflict}, nil
	default:
		return FollowUpResult{}, fmt.Errorf("resubmit outbound supplement: %s", answer.Outcome)
	}
}

func strconvItoa(value int) string {
	return fmt.Sprintf("%d", value)
}
