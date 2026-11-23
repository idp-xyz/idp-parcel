package parcelshipment

import (
	"context"
	"fmt"
	"strconv"
	"time"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	warehouseapp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

type shipmentBySource interface {
	FindBySourceIdentity(ctx context.Context, identity psdomain.SourceIdentity) (psdomain.ShipmentRequest, bool, error)
}

type withdrawalHandler interface {
	Handle(ctx context.Context, command psapplication.WithdrawShipmentRequestCommand) (psapplication.WithdrawShipmentRequestResult, error)
}

// Releaser 问一份已提交的仓配出库委托能不能撤回。代办没获准时不调用撤回编排，免得没登记的租户留下撤回来源。
type Releaser struct {
	agency   agencyAllowance
	requests shipmentBySource
	withdraw func(context.Context, psapplication.WithdrawShipmentRequestCommand) (string, string, string, error)
	clock    interface{ Now() time.Time }
}

func NewReleaser(
	agency agencyAllowance,
	requests shipmentBySource,
	handler withdrawalHandler,
	clock interface{ Now() time.Time },
) (*Releaser, error) {
	if agency == nil || requests == nil || handler == nil || clock == nil {
		return nil, fmt.Errorf("warehouse fulfillment parcel shipment: releaser needs an agency allowance, a request store, a withdrawal handler and a clock")
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
	}, nil
}

// Release 按出库请求与包裹序号找回那份委托。已撤回或已拒绝视为已经放行。已接受要走包裹取消，那一口还没接，答未配置。
func (releaser *Releaser) Release(ctx context.Context, tenant, owner, shipmentID, request string, sequence int) (string, error) {
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
		return warehouseapp.ShipmentNotConfigured, nil
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
	return releaseWord(outcome, state, pending)
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
