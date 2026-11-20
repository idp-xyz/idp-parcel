package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// RegisterUnclaimedArrivalOutcome 是一线作业登记待认领到货的结果（UC-WF-005 结果语义契约），平铺。
type RegisterUnclaimedArrivalOutcome uint8

const (
	RegisterUnclaimedOutcomeInvalid RegisterUnclaimedArrivalOutcome = iota
	RegisterUnclaimedRegistered
	RegisterUnclaimedExistingResult
	RegisterUnclaimedContentConflict
	RegisterUnclaimedWarehouseNotFound
	RegisterUnclaimedWarehouseNotAccepting
)

func (outcome RegisterUnclaimedArrivalOutcome) String() string {
	switch outcome {
	case RegisterUnclaimedRegistered:
		return "REGISTERED"
	case RegisterUnclaimedExistingResult:
		return "EXISTING_RESULT"
	case RegisterUnclaimedContentConflict:
		return "CONTENT_CONFLICT"
	case RegisterUnclaimedWarehouseNotFound:
		return "WAREHOUSE_NOT_FOUND"
	case RegisterUnclaimedWarehouseNotAccepting:
		return "WAREHOUSE_NOT_ACCEPTING"
	default:
		return ""
	}
}

// RegisterUnclaimedArrivalResult 携带在册的待认领到货（REGISTERED、EXISTING_RESULT）或仓不受理原因。
type RegisterUnclaimedArrivalResult struct {
	outcome RegisterUnclaimedArrivalOutcome
	arrival domain.RecordedUnclaimedArrival
	has     bool
	reason  WarehouseNotAcceptingReason
}

func (result RegisterUnclaimedArrivalResult) Outcome() RegisterUnclaimedArrivalOutcome {
	return result.outcome
}

func (result RegisterUnclaimedArrivalResult) Arrival() (domain.RecordedUnclaimedArrival, bool) {
	return result.arrival, result.has
}

func (result RegisterUnclaimedArrivalResult) NotAcceptingReason() (WarehouseNotAcceptingReason, bool) {
	return result.reason, result.outcome == RegisterUnclaimedWarehouseNotAccepting
}

// RegisterUnclaimedArrivalCommand 是仓内作业直接登记一件待认领到货（入库到货或退件）。
// 租户、设备与操作者取自信封，不在载荷里。
type RegisterUnclaimedArrivalCommand struct {
	Tenant          domain.TenantID
	FactID          domain.UnclaimedArrivalFactID
	Device          domain.ReceiptDeviceID
	Operator        domain.ReceiptOperatorID
	OccurredAt      time.Time
	DeviceSequence  int64
	Warehouse       domain.WarehouseCode
	Kind            domain.UnclaimedArrivalKind
	Observation     domain.UnclaimedArrivalObservation
	OwnerClue       domain.OwnerAccountID
	PreAdviceClue   domain.PreAdviceID
	StorageLocation domain.UnclaimedArrivalStorageLocation
}

func (command RegisterUnclaimedArrivalCommand) assemblyError() error {
	switch {
	case command.Tenant.String() == "":
		return errors.New("缺租户")
	case command.FactID.String() == "":
		return errors.New("缺事实身份")
	case command.Device.String() == "":
		return errors.New("缺设备")
	case command.Operator.String() == "":
		return errors.New("缺操作者")
	case command.OccurredAt.IsZero():
		return errors.New("缺发生时刻")
	case command.DeviceSequence < 1:
		return errors.New("设备序号须为正")
	case command.Warehouse.String() == "":
		return errors.New("缺仓")
	case command.Kind.String() == "":
		return errors.New("缺到货种类")
	case command.Observation.String() == "":
		return errors.New("缺观察")
	default:
		return nil
	}
}

func (command RegisterUnclaimedArrivalCommand) submission() domain.DirectUnclaimedArrivalSubmission {
	return domain.DirectUnclaimedArrivalSubmission{
		Device:          command.Device,
		Operator:        command.Operator,
		OccurredAt:      command.OccurredAt,
		DeviceSequence:  command.DeviceSequence,
		Warehouse:       command.Warehouse,
		Kind:            command.Kind,
		Observation:     command.Observation,
		OwnerClue:       command.OwnerClue,
		PreAdviceClue:   command.PreAdviceClue,
		StorageLocation: command.StorageLocation,
	}
}

// RegisterUnclaimedArrivalHandler 把仓内作业直接登记的待认领到货落册，不改库存账，不取单据号。
// 事务边界在调用方：HoldFact 与 SaveDirect 在没有环境事务时拒绝。
type RegisterUnclaimedArrivalHandler struct {
	arrivals   ports.UnclaimedArrivalStore
	identities ports.UnclaimedArrivalIdentities
	warehouses ports.WarehouseLookup
	clock      ports.Clock
}

func NewRegisterUnclaimedArrivalHandler(
	arrivals ports.UnclaimedArrivalStore,
	identities ports.UnclaimedArrivalIdentities,
	warehouses ports.WarehouseLookup,
	clock ports.Clock,
) *RegisterUnclaimedArrivalHandler {
	return &RegisterUnclaimedArrivalHandler{
		arrivals:   arrivals,
		identities: identities,
		warehouses: warehouses,
		clock:      clock,
	}
}

// Register 登记一件待认领到货。判定次序（UC-WF-005）：
//  1. 锁事实身份并按事实身份认重放：内容相同答 EXISTING_RESULT，内容不同答 CONTENT_CONFLICT、在册的不变；
//  2. 核指名的仓在本租户已登记且是自营仓：没登记（含无仓租户）答 WAREHOUSE_NOT_FOUND，代理仓答 WAREHOUSE_NOT_ACCEPTING/AGENT，
//     停用的自营仓照记；未通过的不占事实身份；
//  3. 签发到货标识、落册进入 UNCLAIMED，答 REGISTERED。不改库存账，不取单据号。
func (handler *RegisterUnclaimedArrivalHandler) Register(
	ctx context.Context,
	command RegisterUnclaimedArrivalCommand,
) (RegisterUnclaimedArrivalResult, error) {
	if err := command.assemblyError(); err != nil {
		return RegisterUnclaimedArrivalResult{}, fmt.Errorf("register unclaimed arrival: %w", err)
	}
	if err := handler.arrivals.HoldFact(ctx, command.Tenant, command.FactID); err != nil {
		return RegisterUnclaimedArrivalResult{}, fmt.Errorf("register unclaimed arrival: %w", err)
	}
	existing, found, err := handler.arrivals.LoadByFact(ctx, command.Tenant, command.FactID)
	if err != nil {
		return RegisterUnclaimedArrivalResult{}, fmt.Errorf("register unclaimed arrival: %w", err)
	}
	if found {
		if existing.SameDirectContent(command.submission()) {
			return RegisterUnclaimedArrivalResult{
				outcome: RegisterUnclaimedExistingResult, arrival: existing, has: true,
			}, nil
		}
		return RegisterUnclaimedArrivalResult{outcome: RegisterUnclaimedContentConflict}, nil
	}

	warehouse, found, err := handler.warehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return RegisterUnclaimedArrivalResult{}, fmt.Errorf("register unclaimed arrival: %w", err)
	}
	if !found {
		return RegisterUnclaimedArrivalResult{outcome: RegisterUnclaimedWarehouseNotFound}, nil
	}
	if warehouse.Content().Kind() != domain.WarehouseKindSelfOperated {
		return RegisterUnclaimedArrivalResult{
			outcome: RegisterUnclaimedWarehouseNotAccepting,
			reason:  WarehouseNotAcceptingAgent,
		}, nil
	}

	arrivalID, err := handler.identities.NextUnclaimedArrivalID()
	if err != nil {
		return RegisterUnclaimedArrivalResult{}, fmt.Errorf("register unclaimed arrival: %w", err)
	}
	arrival, err := domain.NewRecordedUnclaimedArrival(domain.RecordedUnclaimedArrivalSpec{
		Tenant:          command.Tenant,
		ID:              arrivalID,
		FactID:          command.FactID,
		Origin:          domain.UnclaimedArrivalOriginWorkFact,
		Kind:            command.Kind,
		Status:          domain.UnclaimedArrivalStatusUnclaimed,
		Warehouse:       command.Warehouse,
		Device:          command.Device,
		Operator:        command.Operator,
		OccurredAt:      command.OccurredAt,
		DeviceSequence:  command.DeviceSequence,
		ReceivedAt:      handler.clock.Now(),
		Observation:     command.Observation,
		OwnerClue:       command.OwnerClue,
		PreAdviceClue:   command.PreAdviceClue,
		StorageLocation: command.StorageLocation,
	})
	if err != nil {
		return RegisterUnclaimedArrivalResult{}, fmt.Errorf("register unclaimed arrival: %w", err)
	}
	inserted, err := handler.arrivals.SaveDirect(ctx, arrival)
	if err != nil {
		return RegisterUnclaimedArrivalResult{}, fmt.Errorf("register unclaimed arrival: %w", err)
	}
	if !inserted {
		return RegisterUnclaimedArrivalResult{}, errors.New("register unclaimed arrival: 事实身份在锁内仍被占")
	}
	return RegisterUnclaimedArrivalResult{
		outcome: RegisterUnclaimedRegistered, arrival: arrival, has: true,
	}, nil
}
