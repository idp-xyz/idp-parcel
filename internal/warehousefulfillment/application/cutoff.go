package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// CutoffOutcome 是一次出库截单的答案。
type CutoffOutcome uint8

const (
	CutoffOutcomeInvalid CutoffOutcome = iota
	CutoffCancelled
	CutoffPartial
	CutoffReturnRequired
	CutoffNotConfigured
	CutoffExisting
	CutoffConflict
	CutoffStatusMismatch
	CutoffContentInvalid
	CutoffNotFound
	CutoffRefused
)

func (outcome CutoffOutcome) String() string {
	switch outcome {
	case CutoffCancelled:
		return "CANCELLED"
	case CutoffPartial:
		return "PARTIALLY_CANCELLED"
	case CutoffReturnRequired:
		return "RETURN_REQUIRED"
	case CutoffNotConfigured:
		return "NOT_CONFIGURED"
	case CutoffExisting:
		return "EXISTING_RESULT"
	case CutoffConflict:
		return "CONTENT_CONFLICT"
	case CutoffStatusMismatch:
		return "STATUS_MISMATCH"
	case CutoffContentInvalid:
		return "CONTENT_INVALID"
	case CutoffNotFound:
		return "NOT_FOUND"
	case CutoffRefused:
		return "CUTOFF_REFUSED"
	default:
		return ""
	}
}

type CutoffResult struct{ outcome CutoffOutcome }

func (result CutoffResult) Outcome() CutoffOutcome { return result.outcome }

// CutoffCommand 撤回整份出库请求或打包后的一个包裹。已出库确认的不再受理。
type CutoffCommand struct {
	Tenant   domain.TenantID
	Owner    domain.OwnerAccountID
	Operator domain.Registrant
	Request  domain.OutboundRequestID
	Sequence int
	Key      string
	Basis    string
	Origin   domain.OutboundOrigin
}

// 小包托运对一次已提交委托的放行。没授予或还没接上答未配置，不放行答拒绝。
const (
	ShipmentReleased      = "RELEASED"
	ShipmentRefused       = "REFUSED"
	ShipmentNotConfigured = "NOT_CONFIGURED"
)

type shipmentReleaser interface {
	Release(ctx context.Context, tenant, owner, shipmentID, declaredParcel, request string, sequence int) (string, error)
}

func (handler *WaveHandler) Cutoff(ctx context.Context, command CutoffCommand) (CutoffResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return CutoffResult{}, fmt.Errorf("cut off outbound: %w", err)
	}
	if !configured {
		return CutoffResult{outcome: CutoffNotConfigured}, nil
	}
	if command.Request.String() == "" || command.Key == "" || command.Sequence < 0 {
		return CutoffResult{outcome: CutoffContentInvalid}, nil
	}
	if command.Origin == domain.OutboundOriginOperator && command.Basis == "" {
		return CutoffResult{outcome: CutoffContentInvalid}, nil
	}
	request, found, err := handler.requests.LoadByID(ctx, command.Tenant, command.Request)
	if err != nil {
		return CutoffResult{}, fmt.Errorf("cut off outbound: %w", err)
	}
	if !found {
		return CutoffResult{outcome: CutoffNotFound}, nil
	}
	if command.Owner.String() != "" && request.Owner() != command.Owner {
		return CutoffResult{outcome: CutoffContentInvalid}, nil
	}
	storedRequest, storedSequence, stored, err := handler.waves.LoadCutoff(ctx, command.Tenant, command.Key)
	if err != nil {
		return CutoffResult{}, fmt.Errorf("cut off outbound: %w", err)
	}
	if stored {
		if storedRequest == command.Request.String() && storedSequence == command.Sequence {
			return CutoffResult{outcome: CutoffExisting}, nil
		}
		return CutoffResult{outcome: CutoffConflict}, nil
	}
	outcome, status, err := handler.cutOff(ctx, command, request)
	if err != nil {
		return CutoffResult{}, err
	}
	if outcome == CutoffOutcomeInvalid {
		return CutoffResult{outcome: CutoffStatusMismatch}, nil
	}
	if outcome == CutoffRefused || outcome == CutoffNotConfigured || outcome == CutoffStatusMismatch {
		return CutoffResult{outcome: outcome}, nil
	}
	if status != domain.OutboundStatusInvalid {
		if err := handler.waves.SetRequestStatus(ctx, command.Tenant, command.Request, status); err != nil {
			return CutoffResult{}, fmt.Errorf("cut off outbound: %w", err)
		}
	}
	if err := handler.waves.SaveCutoff(ctx, command.Tenant, command.Key, command.Request.String(), command.Sequence, command.Basis, outcome.String(), handler.clock.Now()); err != nil {
		return CutoffResult{}, fmt.Errorf("cut off outbound: %w", err)
	}
	return CutoffResult{outcome: outcome}, nil
}

func (handler *WaveHandler) cutOff(ctx context.Context, command CutoffCommand, request domain.OutboundRequest) (CutoffOutcome, domain.OutboundStatus, error) {
	var none domain.OutboundStatus
	if request.Status() == domain.OutboundShipped || request.Status() == domain.OutboundCancelled {
		return CutoffStatusMismatch, none, nil
	}
	tasks, err := handler.waves.TasksOf(ctx, command.Tenant, command.Request)
	if err != nil {
		return CutoffOutcomeInvalid, none, fmt.Errorf("cut off outbound: %w", err)
	}
	if command.Sequence == 0 {
		if request.Status() == domain.OutboundPacked || request.Status() == domain.OutboundPartiallyCancelled {
			return CutoffStatusMismatch, none, nil
		}
		picked := false
		for _, task := range tasks {
			if task.Picked > 0 {
				picked = true
			}
		}
		if picked || request.Status() == domain.OutboundPicked || request.Status() == domain.OutboundChecked {
			if err := handler.queueReturns(ctx, command.Tenant, command.Request, 0, tasks); err != nil {
				return CutoffOutcomeInvalid, none, err
			}
			if err := handler.waves.ReleaseReservations(ctx, command.Tenant, command.Request); err != nil {
				return CutoffOutcomeInvalid, none, fmt.Errorf("cut off outbound: %w", err)
			}
			if err := handler.waves.CancelOpenTasks(ctx, command.Tenant, command.Request); err != nil {
				return CutoffOutcomeInvalid, none, fmt.Errorf("cut off outbound: %w", err)
			}
			return CutoffReturnRequired, domain.OutboundAwaitingReturn, nil
		}
		switch request.Status() {
		case domain.OutboundReserved, domain.OutboundPartiallyReserved, domain.OutboundPendingPolicy, domain.OutboundAllocated:
		default:
			return CutoffStatusMismatch, none, nil
		}
		if err := handler.waves.ReleaseReservations(ctx, command.Tenant, command.Request); err != nil {
			return CutoffOutcomeInvalid, none, fmt.Errorf("cut off outbound: %w", err)
		}
		if err := handler.waves.CancelOpenTasks(ctx, command.Tenant, command.Request); err != nil {
			return CutoffOutcomeInvalid, none, fmt.Errorf("cut off outbound: %w", err)
		}
		return CutoffCancelled, domain.OutboundCancelled, nil
	}
	if request.Status() != domain.OutboundPacked && request.Status() != domain.OutboundPartiallyCancelled && request.Status() != domain.OutboundAwaitingReturn {
		return CutoffStatusMismatch, none, nil
	}
	parcels, err := handler.waves.ParcelsOf(ctx, command.Tenant, command.Request)
	if err != nil {
		return CutoffOutcomeInvalid, none, fmt.Errorf("cut off outbound: %w", err)
	}
	var parcel ports.PackParcel
	seen := false
	for _, item := range parcels {
		if item.Sequence == command.Sequence {
			parcel = item
			seen = true
		}
	}
	if !seen {
		return CutoffStatusMismatch, none, nil
	}
	if parcel.ShipmentRequestID != "" {
		if handler.releaser == nil {
			return CutoffNotConfigured, none, nil
		}
		word, err := handler.releaser.Release(ctx, command.Tenant.String(), request.Owner().String(), parcel.ShipmentRequestID, parcel.DeclaredParcelID, command.Request.String(), command.Sequence)
		if err != nil {
			return CutoffOutcomeInvalid, none, fmt.Errorf("cut off outbound: %w", err)
		}
		switch word {
		case ShipmentReleased:
		case ShipmentNotConfigured:
			return CutoffNotConfigured, none, nil
		case ShipmentRefused:
			return CutoffRefused, none, nil
		default:
			return CutoffOutcomeInvalid, none, fmt.Errorf("cut off outbound: unknown release %s", word)
		}
	}
	confirmed, err := handler.waves.ParcelConfirmed(ctx, command.Tenant, command.Request, command.Sequence)
	if err != nil {
		return CutoffOutcomeInvalid, none, fmt.Errorf("cut off outbound: %w", err)
	}
	if confirmed {
		return CutoffStatusMismatch, none, nil
	}
	if err := handler.queueReturns(ctx, command.Tenant, command.Request, command.Sequence, tasksForParcel(tasks, parcel)); err != nil {
		return CutoffOutcomeInvalid, none, err
	}
	next, err := handler.waves.MarkParcelCutoff(ctx, command.Tenant, command.Request, command.Sequence)
	if err != nil {
		return CutoffOutcomeInvalid, none, fmt.Errorf("cut off outbound: %w", err)
	}
	if next == domain.OutboundCancelled {
		return CutoffCancelled, none, nil
	}
	return CutoffPartial, none, nil
}

func tasksForParcel(tasks []ports.PickTask, parcel ports.PackParcel) []ports.PickTask {
	matched := make([]ports.PickTask, 0, 1)
	for _, task := range tasks {
		if task.Goods == parcel.Goods && task.Picked > 0 {
			matched = append(matched, task)
		}
	}
	return matched
}

func (handler *WaveHandler) queueReturns(ctx context.Context, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, tasks []ports.PickTask) error {
	return queueReturnTasks(ctx, handler.waves, tenant, request, sequence, tasks)
}

// queueReturnTasks 为已拣出的货生成回库任务。货还在待出库，完成回库才移回可分配库区。
func queueReturnTasks(ctx context.Context, store interface {
	SaveReturnTask(ctx context.Context, tenant domain.TenantID, task ports.ReturnTask) error
}, tenant domain.TenantID, request domain.OutboundRequestID, sequence int, tasks []ports.PickTask) error {
	for _, task := range tasks {
		if task.Picked <= 0 || task.Staging.String() == "" {
			continue
		}
		id := request.String() + "/" + task.ID.String()
		if sequence > 0 {
			id = request.String() + "/" + fmt.Sprint(sequence) + "/" + task.ID.String()
		}
		if err := store.SaveReturnTask(ctx, tenant, ports.ReturnTask{
			ID: id, Request: request, Sequence: sequence, Location: task.Staging, Goods: task.Goods,
			Unit: task.Unit, State: task.State, Inbound: task.Inbound, Quantity: task.Picked,
		}); err != nil {
			return fmt.Errorf("queue return: %w", err)
		}
	}
	return nil
}
