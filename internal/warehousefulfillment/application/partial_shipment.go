package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// RecordPartialShipmentConsentCommand 是受理之后货主对一张出库请求另外表示的部分发货意思，由运营凭依据代录。
// 依据是货主那一句话的出处（书面确认的编号之类），同一张请求同一个依据只记一次。
type RecordPartialShipmentConsentCommand struct {
	Tenant   domain.TenantID
	Request  domain.OutboundRequestID
	Rule     domain.PartialShipment
	Basis    string
	Operator domain.Registrant
}

// RecordConsent 记下货主对这一张请求的部分发货意思。它不动请求提交时的内容，也不动预占与任务：
// 只是一句话落册，之后按已拣数量发货或拆出缺货部分时照它判断。已经出库、取消、拒收、拆开或交给别处履行的请求不再收。
func (handler *OutboundHandler) RecordConsent(ctx context.Context, command RecordPartialShipmentConsentCommand) (OutboundResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return OutboundResult{}, fmt.Errorf("record partial shipment consent: %w", err)
	}
	if !configured {
		return OutboundResult{outcome: OutboundWarehouseNotFound}, nil
	}
	if command.Request.String() == "" || !command.Rule.Given() || command.Basis == "" || command.Operator.String() == "" {
		return OutboundResult{outcome: OutboundContentInvalid}, nil
	}
	request, found, err := handler.requests.LoadByID(ctx, command.Tenant, command.Request)
	if err != nil {
		return OutboundResult{}, fmt.Errorf("record partial shipment consent: %w", err)
	}
	if !found {
		return OutboundResult{outcome: OutboundNotFound}, nil
	}
	existing, found, err := handler.requests.LoadConsent(ctx, command.Tenant, command.Request, command.Basis)
	if err != nil {
		return OutboundResult{}, fmt.Errorf("record partial shipment consent: %w", err)
	}
	if found {
		if existing.Rule == command.Rule {
			return OutboundResult{outcome: OutboundExisting, request: request, has: true}, nil
		}
		return OutboundResult{outcome: OutboundConflict, request: request, has: true}, nil
	}
	switch request.Status() {
	case domain.OutboundShipped, domain.OutboundCancelled, domain.OutboundRejected, domain.OutboundSplit,
		domain.OutboundHandedOff, domain.OutboundForwarded, domain.OutboundAwaitingReturn:
		return OutboundResult{outcome: OutboundStatusMismatch, request: request, has: true}, nil
	}
	if err := handler.requests.SaveConsent(ctx, command.Tenant, ports.PartialShipmentConsent{
		Request: command.Request, Basis: command.Basis, Rule: command.Rule,
		Operator: command.Operator.String(), RecordedAt: handler.clock.Now(),
	}); err != nil {
		return OutboundResult{}, fmt.Errorf("record partial shipment consent: %w", err)
	}
	return OutboundResult{outcome: OutboundConsentRecorded, request: request, has: true}, nil
}

// partialShipmentOf 读出一张请求此刻适用的部分发货规则与它出自哪一层，口径是 domain.EffectivePartialShipment。
func partialShipmentOf(ctx context.Context, store ports.OutboundStore, tenant domain.TenantID, request domain.OutboundRequest) (domain.PartialShipment, domain.PartialShipmentSource, error) {
	var consent, owner domain.PartialShipment
	latest, found, err := store.LatestConsent(ctx, tenant, request.ID())
	if err != nil {
		return domain.PartialShipmentUnset, domain.PartialShipmentFromNowhere, err
	}
	if found {
		consent = latest.Rule
	}
	policy, found, err := store.LoadPolicy(ctx, tenant, request.Owner())
	if err != nil {
		return domain.PartialShipmentUnset, domain.PartialShipmentFromNowhere, err
	}
	if found {
		owner = policy.PartialShipment
	}
	rule, source := domain.EffectivePartialShipment(consent, request.PartialShipment(), owner)
	return rule, source, nil
}
