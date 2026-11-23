package parcelshipment

import (
	"context"
	"fmt"
	"strconv"
	"time"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	warehouseapp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

type shipmentBySource interface {
	FindBySourceIdentity(ctx context.Context, identity psdomain.SourceIdentity) (psdomain.ShipmentRequest, bool, error)
}

type withdrawalHandler interface {
	Handle(ctx context.Context, command psapplication.WithdrawShipmentRequestCommand) (psapplication.WithdrawShipmentRequestResult, error)
}

type parcelCanceller interface {
	Handle(ctx context.Context, command psapplication.CancelParcelCommand) (psapplication.CancelParcelResult, error)
}

// Releaser 问一份仓配出库委托能不能放开。代办没获准时不调用撤回或取消，免得没登记的租户留下来源。
// 已提交走撤回。已接受走现有的包裹取消，不另造一套取消规则。
type Releaser struct {
	agency   agencyAllowance
	requests shipmentBySource
	withdraw func(context.Context, psapplication.WithdrawShipmentRequestCommand) (string, string, string, error)
	cancel   func(context.Context, psapplication.CancelParcelCommand) (string, string, string, error)
	clock    interface{ Now() time.Time }
}

func NewReleaser(
	agency agencyAllowance,
	requests shipmentBySource,
	handler withdrawalHandler,
	canceller parcelCanceller,
	clock interface{ Now() time.Time },
) (*Releaser, error) {
	if agency == nil || requests == nil || handler == nil || canceller == nil || clock == nil {
		return nil, fmt.Errorf("warehouse fulfillment parcel shipment: releaser needs an agency allowance, a request store, a withdrawal handler, a parcel canceller and a clock")
	}
	return &Releaser{
		agency: agency, requests: requests, clock: clock,
		withdraw: func(ctx context.Context, command psapplication.WithdrawShipmentRequestCommand) (string, string, string, error) {
			result, err := handler.Handle(ctx, command)
			if err != nil {
				return "", "", "", err
			}
			return result.Outcome().String(), result.State().String(), result.PendingReason().String(), nil
		},
		cancel: func(ctx context.Context, command psapplication.CancelParcelCommand) (string, string, string, error) {
			result, err := canceller.Handle(ctx, command)
			if err != nil {
				return "", "", "", err
			}
			kind := ""
			if record, ok := result.Record(); ok {
				kind = record.Kind.String()
			}
			return result.Outcome().String(), kind, result.UndecidedReason().String(), nil
		},
	}, nil
}

// Release 按出库请求与包裹序号找回那份委托。已撤回或已拒绝视为已经放行。
// 已接受交给包裹取消：取消成立才放行，不成立或授权没登记都不放行。
func (releaser *Releaser) Release(ctx context.Context, tenant, owner, shipmentID, declaredParcel, request string, sequence int) (string, error) {
	if sequence < 1 || shipmentID == "" || request == "" {
		return "", fmt.Errorf("release shipment: parcel identity is incomplete")
	}
	granted, err := releaser.agency.Allow(ctx, tenant, owner)
	if err != nil {
		return "", err
	}
	if !granted {
		return warehouseapp.ShipmentNotConfigured, nil
	}
	identity, err := warehouseOutboundIdentity(tenant, owner, request, sequence)
	if err != nil {
		return "", err
	}
	found, ok, err := releaser.requests.FindBySourceIdentity(ctx, identity)
	if err != nil {
		return "", err
	}
	if !ok || found.ShipmentRequestID().String() != shipmentID {
		return "", fmt.Errorf("release shipment: %w", psapplication.ErrShipmentRequestNotVisible)
	}
	switch found.State() {
	case psdomain.ShipmentRequestWithdrawn, psdomain.ShipmentRequestRejected:
		return warehouseapp.ShipmentReleased, nil
	case psdomain.ShipmentRequestAccepted:
		return releaser.cancelAccepted(ctx, identity, found, declaredParcel)
	case psdomain.ShipmentRequestSubmitted:
	default:
		return "", fmt.Errorf("release shipment: state %s", found.State())
	}
	command, err := releaser.command(identity, found)
	if err != nil {
		return "", err
	}
	outcome, state, pending, err := releaser.withdraw(ctx, command)
	if err != nil {
		return "", err
	}
	if outcome == psapplication.WithdrawalDecisionAlreadyFormed.String() && state == psdomain.ShipmentRequestAccepted.String() {
		return releaser.cancelAccepted(ctx, identity, found, declaredParcel)
	}
	return releaseWord(outcome, state, pending)
}

// cancelAccepted 以货主本人的名义把这一件交给现有的包裹取消。请求时刻用委托的提交时刻，同一份截单重放才对得上已有取消。
func (releaser *Releaser) cancelAccepted(ctx context.Context, identity psdomain.SourceIdentity, request psdomain.ShipmentRequest, declaredParcel string) (string, error) {
	if releaser.cancel == nil {
		return warehouseapp.ShipmentNotConfigured, nil
	}
	if declaredParcel == "" {
		return "", fmt.Errorf("release shipment: declared parcel is missing")
	}
	parcel, err := psdomain.NewDeclaredParcelID(declaredParcel)
	if err != nil {
		return "", err
	}
	requester, err := psdomain.NewCustomerAccountCancellationRequester(identity.CustomerAccountID())
	if err != nil {
		return "", err
	}
	reason, err := psdomain.NewCancellationReasonReference(psdomain.OutboundCutoffReason)
	if err != nil {
		return "", err
	}
	command := psapplication.CancelParcelCommand{
		Identity: identity, ShipmentRequestID: request.ShipmentRequestID(), CustomerAccount: identity.CustomerAccountID(),
		Parcel: parcel, Requester: requester, Reason: reason, RequestedAt: request.SubmittedAt().UTC(),
	}
	outcome, kind, undecided, err := releaser.cancel(ctx, command)
	if err != nil {
		return "", err
	}
	return cancellationWord(outcome, kind, undecided)
}

func cancellationWord(outcome, recordKind, undecided string) (string, error) {
	switch outcome {
	case psapplication.ParcelCancelled.String():
		return warehouseapp.ShipmentReleased, nil
	case psapplication.CancellationRefused.String(), psapplication.DispositionPending.String(), psapplication.CancellationFinalOutcomeStands.String():
		return warehouseapp.ShipmentRefused, nil
	case psapplication.CancellationExistingResult.String():
		switch recordKind {
		case psports.RecordParcelCancelled.String():
			return warehouseapp.ShipmentReleased, nil
		case psports.RecordCancellationRefused.String(), psports.RecordDispositionPending.String():
			return warehouseapp.ShipmentRefused, nil
		default:
			return "", fmt.Errorf("release shipment: existing cancellation %s", recordKind)
		}
	case psapplication.CancellationUndecided.String():
		if undecided == psapplication.CancelAuthorityUnconfigured.String() {
			return warehouseapp.ShipmentNotConfigured, nil
		}
		return "", fmt.Errorf("release shipment: %s", undecided)
	case psapplication.CancellationRequestEnded.String():
		return warehouseapp.ShipmentReleased, nil
	default:
		return "", fmt.Errorf("release shipment: %s", outcome)
	}
}

func (releaser *Releaser) command(identity psdomain.SourceIdentity, request psdomain.ShipmentRequest) (psapplication.WithdrawShipmentRequestCommand, error) {
	var none psapplication.WithdrawShipmentRequestCommand
	key, err := psdomain.NewSourceRequestKey("warehouse-cutoff/" + request.ShipmentRequestID().String())
	if err != nil {
		return none, err
	}
	withdrawalIdentity, err := psdomain.NewSourceIdentity(identity.TenantID(), identity.CustomerAccountID(), identity.Source(), key)
	if err != nil {
		return none, err
	}
	reason, err := psdomain.NewWithdrawalReasonReference(psdomain.OutboundCutoffReason)
	if err != nil {
		return none, err
	}
	digest, err := psdomain.CanonicalizeCustomerWithdrawal(psdomain.CustomerWithdrawalSpec{
		OriginalAccessRequestID: identity.RequestKey().String(),
		ShipmentRequestID:       request.ShipmentRequestID().String(),
		Reason:                  reason,
	})
	if err != nil {
		return none, err
	}
	requester, err := psdomain.NewWithdrawalRequesterReference(psdomain.WarehouseFulfillmentRequester)
	if err != nil {
		return none, err
	}
	now := releaser.clock.Now().UTC()
	return psapplication.WithdrawShipmentRequestCommand{
		Identity: identity, WithdrawalIdentity: withdrawalIdentity, PayloadDigest: digest,
		OccurredAt: now, ReceivedAt: now, ShipmentRequestID: request.ShipmentRequestID(),
		SubmissionVersion: request.CurrentSubmissionVersion().VersionID(), Requester: requester, Reason: reason,
	}, nil
}

func warehouseOutboundIdentity(tenantID, ownerID, request string, sequence int) (psdomain.SourceIdentity, error) {
	var none psdomain.SourceIdentity
	tenant, err := psdomain.NewTenantID(tenantID)
	if err != nil {
		return none, err
	}
	owner, err := psdomain.NewCustomerAccountID(ownerID)
	if err != nil {
		return none, err
	}
	source, err := psdomain.NewSource(psdomain.SourceWarehouseOutbound)
	if err != nil {
		return none, err
	}
	key, err := psdomain.NewSourceRequestKey(request + "/" + strconv.Itoa(sequence))
	if err != nil {
		return none, err
	}
	return psdomain.NewSourceIdentity(tenant, owner, source, key)
}

func releaseWord(outcome, state, pending string) (string, error) {
	switch outcome {
	case psapplication.WithdrawalFormed.String():
		return warehouseapp.ShipmentReleased, nil
	case psapplication.WithdrawalNotAuthorized.String():
		return warehouseapp.ShipmentRefused, nil
	case psapplication.WithdrawalDecisionAlreadyFormed.String():
		switch state {
		case psdomain.ShipmentRequestWithdrawn.String(), psdomain.ShipmentRequestRejected.String():
			return warehouseapp.ShipmentReleased, nil
		case psdomain.ShipmentRequestAccepted.String():
			return warehouseapp.ShipmentNotConfigured, nil
		default:
			return "", fmt.Errorf("release shipment: decided state %s", state)
		}
	case psapplication.WithdrawalUndecided.String():
		if pending == psapplication.WithdrawalAuthorityRulesNotConfigured.String() {
			return warehouseapp.ShipmentNotConfigured, nil
		}
		return "", fmt.Errorf("release shipment: %s", pending)
	default:
		return "", fmt.Errorf("release shipment: %s", outcome)
	}
}
