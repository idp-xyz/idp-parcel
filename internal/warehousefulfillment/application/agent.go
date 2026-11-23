package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// AgentOutcome 是代理仓库存声明与转出的结果。
type AgentOutcome uint8

const (
	AgentOutcomeInvalid AgentOutcome = iota
	AgentRegistered
	AgentExisting
	AgentConflict
	AgentValid
	AgentUnknown
	AgentSuperseded
	AgentNotFound
	AgentWarehouseNotFound
	AgentNotConfigured
	AgentForwarded
	AgentRejected
	AgentPacked
	AgentConfirmed
	AgentStatusMismatch
	AgentContentInvalid
)

func (outcome AgentOutcome) String() string {
	switch outcome {
	case AgentRegistered:
		return "REGISTERED"
	case AgentExisting:
		return "EXISTING_RESULT"
	case AgentConflict:
		return "CONTENT_CONFLICT"
	case AgentValid:
		return "VALID"
	case AgentUnknown:
		return "UNKNOWN"
	case AgentSuperseded:
		return "SUPERSEDED"
	case AgentNotFound:
		return "NOT_FOUND"
	case AgentWarehouseNotFound:
		return "WAREHOUSE_NOT_FOUND"
	case AgentNotConfigured:
		return "NOT_CONFIGURED"
	case AgentForwarded:
		return "FORWARDED"
	case AgentRejected:
		return "REJECTED"
	case AgentPacked:
		return "PACKED"
	case AgentConfirmed:
		return "CONFIRMED"
	case AgentStatusMismatch:
		return "STATUS_MISMATCH"
	case AgentContentInvalid:
		return "CONTENT_INVALID"
	default:
		return ""
	}
}

type AgentResult struct {
	outcome AgentOutcome
}

func (result AgentResult) Outcome() AgentOutcome { return result.outcome }

// AgentHandler 收下代理仓报来的声明与结果。声明不进库存账，交接不消耗自营库存。
type AgentHandler struct {
	agents   ports.AgentStore
	requests interface {
		LoadByID(context.Context, domain.TenantID, domain.OutboundRequestID) (domain.OutboundRequest, bool, error)
	}
	statuses interface {
		SetRequestStatus(context.Context, domain.TenantID, domain.OutboundRequestID, domain.OutboundStatus) error
	}
	warehouses   ports.WarehouseLookup
	commissioner ports.OutboundCommissioner
	clock        ports.Clock
	// charges 缺席时交接照样成立，也不消耗自营库存。
	charges chargeFormer
}

func NewAgentHandler(
	agents ports.AgentStore,
	requests interface {
		LoadByID(context.Context, domain.TenantID, domain.OutboundRequestID) (domain.OutboundRequest, bool, error)
	},
	statuses interface {
		SetRequestStatus(context.Context, domain.TenantID, domain.OutboundRequestID, domain.OutboundStatus) error
	},
	warehouses ports.WarehouseLookup,
	commissioner ports.OutboundCommissioner,
	clock ports.Clock,
) *AgentHandler {
	return &AgentHandler{agents: agents, requests: requests, statuses: statuses, warehouses: warehouses, commissioner: commissioner, clock: clock}
}

func (handler *AgentHandler) RegisterConnector(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode) (AgentResult, error) {
	revision, found, err := handler.warehouses.LoadLatestWarehouse(ctx, tenant, warehouse)
	if err != nil {
		return AgentResult{}, fmt.Errorf("register agent connector: %w", err)
	}
	if !found {
		return AgentResult{outcome: AgentWarehouseNotFound}, nil
	}
	if revision.Content().Kind() != domain.WarehouseKindAgent {
		return AgentResult{outcome: AgentContentInvalid}, nil
	}
	bound, err := handler.agents.LoadConnector(ctx, tenant, warehouse)
	if err != nil {
		return AgentResult{}, fmt.Errorf("register agent connector: %w", err)
	}
	if bound {
		return AgentResult{outcome: AgentExisting}, nil
	}
	if err := handler.agents.SaveConnector(ctx, tenant, warehouse, handler.clock.Now()); err != nil {
		return AgentResult{}, fmt.Errorf("register agent connector: %w", err)
	}
	return AgentResult{outcome: AgentRegistered}, nil
}

func (handler *AgentHandler) RecordStatement(ctx context.Context, tenant domain.TenantID, statement ports.AgentStatement) (AgentResult, error) {
	if statement.Source == "" || statement.StatedAt.IsZero() || statement.ValidUntil.IsZero() || statement.Quantity < 0 {
		return AgentResult{outcome: AgentContentInvalid}, nil
	}
	if _, found, err := handler.warehouses.LoadLatestWarehouse(ctx, tenant, statement.Warehouse); err != nil {
		return AgentResult{}, fmt.Errorf("record agent statement: %w", err)
	} else if !found {
		return AgentResult{outcome: AgentWarehouseNotFound}, nil
	}
	existing, found, err := handler.agents.LoadStatement(ctx, tenant, statement.Source, statement.StatedAt)
	if err != nil {
		return AgentResult{}, fmt.Errorf("record agent statement: %w", err)
	}
	if found {
		if sameStatement(existing, statement) {
			return AgentResult{outcome: AgentExisting}, nil
		}
		return AgentResult{outcome: AgentConflict}, nil
	}
	if err := handler.agents.SaveStatement(ctx, tenant, statement); err != nil {
		return AgentResult{}, fmt.Errorf("record agent statement: %w", err)
	}
	return AgentResult{outcome: handler.freshness(ctx, tenant, statement)}, nil
}

func (handler *AgentHandler) ReadStatement(ctx context.Context, tenant domain.TenantID, statement ports.AgentStatement) (AgentResult, error) {
	if !statement.StatedAt.IsZero() {
		stored, found, err := handler.agents.LoadStatement(ctx, tenant, statement.Source, statement.StatedAt)
		if err != nil {
			return AgentResult{}, fmt.Errorf("read agent statement: %w", err)
		}
		if !found {
			return AgentResult{outcome: AgentNotFound}, nil
		}
		later, err := handler.agents.HasLaterStatement(ctx, tenant, stored)
		if err != nil {
			return AgentResult{}, fmt.Errorf("read agent statement: %w", err)
		}
		if later {
			return AgentResult{outcome: AgentSuperseded}, nil
		}
		return AgentResult{outcome: handler.freshness(ctx, tenant, stored)}, nil
	}
	latest, found, err := handler.agents.LatestStatement(ctx, tenant, statement)
	if err != nil {
		return AgentResult{}, fmt.Errorf("read agent statement: %w", err)
	}
	if !found {
		return AgentResult{outcome: AgentNotFound}, nil
	}
	return AgentResult{outcome: handler.freshness(ctx, tenant, latest)}, nil
}

func (handler *AgentHandler) Forward(ctx context.Context, tenant domain.TenantID, requestID domain.OutboundRequestID) (AgentResult, error) {
	request, found, err := handler.requests.LoadByID(ctx, tenant, requestID)
	if err != nil {
		return AgentResult{}, fmt.Errorf("forward agent outbound: %w", err)
	}
	if !found {
		return AgentResult{outcome: AgentNotFound}, nil
	}
	if request.Status() == domain.OutboundForwarded {
		return AgentResult{outcome: AgentExisting}, nil
	}
	if request.Status() != domain.OutboundHandedOff {
		return AgentResult{outcome: AgentStatusMismatch}, nil
	}
	bound, err := handler.agents.LoadConnector(ctx, tenant, request.Warehouse())
	if err != nil {
		return AgentResult{}, fmt.Errorf("forward agent outbound: %w", err)
	}
	if !bound {
		return AgentResult{outcome: AgentNotConfigured}, nil
	}
	if err := handler.agents.SaveResult(ctx, tenant, ports.AgentResult{
		ID: "forward:" + requestID.String(), Request: requestID, Warehouse: request.Warehouse(), Kind: "FORWARDED", Reported: handler.clock.Now(),
	}); err != nil {
		return AgentResult{}, fmt.Errorf("forward agent outbound: %w", err)
	}
	if err := handler.statuses.SetRequestStatus(ctx, tenant, requestID, domain.OutboundForwarded); err != nil {
		return AgentResult{}, fmt.Errorf("forward agent outbound: %w", err)
	}
	return AgentResult{outcome: AgentForwarded}, nil
}

func (handler *AgentHandler) RecordResult(ctx context.Context, tenant domain.TenantID, result ports.AgentResult) (AgentResult, error) {
	if result.ID == "" || result.Kind == "" {
		return AgentResult{outcome: AgentContentInvalid}, nil
	}
	existing, found, err := handler.agents.LoadResult(ctx, tenant, result.ID)
	if err != nil {
		return AgentResult{}, fmt.Errorf("record agent result: %w", err)
	}
	if found {
		if existing.Kind == result.Kind && existing.Request == result.Request {
			return AgentResult{outcome: AgentExisting}, nil
		}
		return AgentResult{outcome: AgentConflict}, nil
	}
	request, found, err := handler.requests.LoadByID(ctx, tenant, result.Request)
	if err != nil {
		return AgentResult{}, fmt.Errorf("record agent result: %w", err)
	}
	if !found || request.Status() != domain.OutboundForwarded {
		return AgentResult{outcome: AgentStatusMismatch}, nil
	}
	result.Warehouse = request.Warehouse()
	if result.Reported.IsZero() {
		result.Reported = handler.clock.Now()
	}
	if err := handler.agents.SaveResult(ctx, tenant, result); err != nil {
		return AgentResult{}, fmt.Errorf("record agent result: %w", err)
	}
	switch result.Kind {
	case "REJECTED", "SHORT":
		if err := handler.statuses.SetRequestStatus(ctx, tenant, result.Request, domain.OutboundRejected); err != nil {
			return AgentResult{}, fmt.Errorf("record agent result: %w", err)
		}
		return AgentResult{outcome: AgentRejected}, nil
	case "PACKED":
		return handler.pack(ctx, tenant, request, result)
	case "HANDED_OVER":
		if err := handler.statuses.SetRequestStatus(ctx, tenant, result.Request, domain.OutboundShipped); err != nil {
			return AgentResult{}, fmt.Errorf("record agent result: %w", err)
		}
		if err := handler.formReportedCharge(ctx, tenant, request, result); err != nil {
			return AgentResult{}, err
		}
		return AgentResult{outcome: AgentConfirmed}, nil
	default:
		return AgentResult{outcome: AgentContentInvalid}, nil
	}
}

func (handler *AgentHandler) pack(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequest, result ports.AgentResult) (AgentResult, error) {
	goods, unit, quantity, found, err := handler.agents.FirstLine(ctx, tenant, request.ID())
	if err != nil {
		return AgentResult{}, fmt.Errorf("record agent result: %w", err)
	}
	if !found {
		return AgentResult{outcome: AgentContentInvalid}, nil
	}
	if err := handler.agents.SaveReportedParcel(ctx, tenant, request.ID(), result, goods, unit, quantity); err != nil {
		return AgentResult{}, fmt.Errorf("record agent result: %w", err)
	}
	if request.Dispatch() != domain.DispatchProductNetwork {
		return AgentResult{outcome: AgentPacked}, nil
	}
	if handler.commissioner == nil {
		return AgentResult{outcome: AgentNotConfigured}, nil
	}
	answer, err := handler.commissioner.Submit(ctx, ports.CommissionSubmission{
		Tenant: tenant.String(), Owner: request.Owner().String(), Request: request.ID().String(), Sequence: 1,
		Service: request.Service(), Recipient: request.Recipient(), Address: request.Address(),
		Parcel: ports.CommissionParcel{
			Sequence: 1, Weight: result.Weight, WeightUnit: result.WeightUnit, Length: result.Length, Width: result.Width,
			Height: result.Height, Dimension: result.Dimension, Goods: goods.String(), Unit: unit.String(), Quantity: quantity,
		},
	})
	if err != nil {
		return AgentResult{}, fmt.Errorf("record agent result: %w", err)
	}
	if answer.Outcome == ports.CommissionNotConfiguredWord {
		return AgentResult{outcome: AgentNotConfigured}, nil
	}
	if answer.Outcome != ports.CommissionSubmittedWord && answer.Outcome != ports.CommissionExistingWord {
		return AgentResult{outcome: AgentContentInvalid}, nil
	}
	if err := handler.agents.MarkReportedShipment(ctx, tenant, request.ID(), answer.ShipmentRequestID, answer.DeclaredParcelID); err != nil {
		return AgentResult{}, fmt.Errorf("record agent result: %w", err)
	}
	return AgentResult{outcome: AgentPacked}, nil
}

func (handler *AgentHandler) freshness(_ context.Context, _ domain.TenantID, statement ports.AgentStatement) AgentOutcome {
	if !handler.clock.Now().Before(statement.ValidUntil) {
		return AgentUnknown
	}
	return AgentValid
}

func sameStatement(left, right ports.AgentStatement) bool {
	return left.Warehouse == right.Warehouse && left.Owner == right.Owner && left.Goods == right.Goods && left.Unit == right.Unit &&
		left.Quantity == right.Quantity && left.ValidUntil.Equal(right.ValidUntil)
}
