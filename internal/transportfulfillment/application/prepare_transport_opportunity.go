package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	// ErrUnexpectedScheduleSave 说明班次库交回了封闭集合以外的写入结果。
	ErrUnexpectedScheduleSave = errors.New("transport fulfillment: unexpected schedule save outcome")
	// ErrUnexpectedPoolSave 说明容量池库交回了封闭集合以外的写入结果。
	ErrUnexpectedPoolSave = errors.New("transport fulfillment: unexpected capacity pool save outcome")
)

// OpportunityOutcome 是运输机会准备的应用处理结果。容量三格是业务负向（ADR-0029）：
// 超额预留带余量（等容量或换班次）、预占过期（重新预占）、守恒撞线（同一范围不得二次
// 转换）——都是已知答案，不是改单也不是等依赖。
type OpportunityOutcome uint8

const (
	OpportunityOutcomeInvalid OpportunityOutcome = iota
	ScheduleEstablished
	ScheduleExistingResult
	ScheduleConflict
	PoolEstablished
	PoolExistingResult
	PoolConflict
	CapacityReserved
	ReservationExistingResult
	ReservationConflict
	CapacityExceededOutcome
	CapacityReleased
	CapacityConsumed
	ReservationExpiredOutcome
	ReservationOverdrawnOutcome
	OpportunityNotAccepted
	OpportunityUndecided
)

func (outcome OpportunityOutcome) String() string {
	switch outcome {
	case ScheduleEstablished:
		return "SCHEDULE_ESTABLISHED"
	case ScheduleExistingResult:
		return "EXISTING_SCHEDULE"
	case ScheduleConflict:
		return "SCHEDULE_CONFLICT"
	case PoolEstablished:
		return "POOL_ESTABLISHED"
	case PoolExistingResult:
		return "EXISTING_POOL"
	case PoolConflict:
		return "POOL_CONFLICT"
	case CapacityReserved:
		return "CAPACITY_RESERVED"
	case ReservationExistingResult:
		return "EXISTING_RESERVATION"
	case ReservationConflict:
		return "RESERVATION_CONFLICT"
	case CapacityExceededOutcome:
		return "CAPACITY_EXCEEDED"
	case CapacityReleased:
		return "CAPACITY_RELEASED"
	case CapacityConsumed:
		return "CAPACITY_CONSUMED"
	case ReservationExpiredOutcome:
		return "RESERVATION_EXPIRED"
	case ReservationOverdrawnOutcome:
		return "RESERVATION_OVERDRAWN"
	case OpportunityNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case OpportunityUndecided:
		return "OPPORTUNITY_UNDECIDED"
	default:
		return ""
	}
}

// OpportunityUndecidedReason 指名提交停在哪一步等谁。
type OpportunityUndecidedReason uint8

const (
	OpportunityUndecidedReasonNone OpportunityUndecidedReason = iota
	ScheduleStoreUnavailable
	PoolStoreUnavailable
)

func (reason OpportunityUndecidedReason) String() string {
	switch reason {
	case ScheduleStoreUnavailable:
		return "SCHEDULE_STORE_UNAVAILABLE"
	case PoolStoreUnavailable:
		return "POOL_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

type EstablishScheduleCommand struct {
	TenantID  domain.TenantID
	Schedule  string
	Direction string
	DepartsAt time.Time
}

type EstablishPoolCommand struct {
	TenantID domain.TenantID
	Pool     string
	Schedule string
	Unit     string
	Capacity int64
}

type ReserveCapacityCommand struct {
	TenantID    domain.TenantID
	Pool        string
	Reservation string
	Quantity    int64
	ValidUntil  time.Time
	At          time.Time
}

type ReleaseCapacityCommand struct {
	TenantID    domain.TenantID
	Pool        string
	Reservation string
	Quantity    int64
	At          time.Time
}

type ConsumeCapacityCommand struct {
	TenantID    domain.TenantID
	Pool        string
	Reservation string
	Quantity    int64
	Assignment  string
	At          time.Time
}

type OpportunityResult struct {
	outcome      OpportunityOutcome
	reason       OpportunityUndecidedReason
	schedule     ports.ScheduleRecord
	pool         ports.CapacityPoolRecord
	hasRecord    bool
	available    int64
	continuation string
	handoff      string
}

func (result OpportunityResult) Outcome() OpportunityOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result OpportunityResult) UndecidedReason() OpportunityUndecidedReason {
	return result.reason
}

func (result OpportunityResult) Schedule() (ports.ScheduleRecord, bool) {
	return result.schedule, result.hasRecord && result.schedule.Key.Schedule.String() != ""
}

func (result OpportunityResult) Pool() (ports.CapacityPoolRecord, bool) {
	return result.pool, result.hasRecord && result.pool.Key.Pool.String() != ""
}

// AvailableQuantity 在超额预留时报告当时余量——拒绝要带着答案，调用方据此改量或换班次。
func (result OpportunityResult) AvailableQuantity() int64 {
	return result.available
}

func (result OpportunityResult) ContinuationReference() string {
	return result.continuation
}

// ConsumptionHandoffReference 非空说明消耗已提交但意图还没交出去，重放会重发同一份。
func (result OpportunityResult) ConsumptionHandoffReference() string {
	return result.handoff
}

type PrepareTransportOpportunityDeps struct {
	Schedules  ports.TransportScheduleStore
	Pools      ports.CapacityPoolStore
	Downstream ports.CapacityConsumptionHandoff
	Clock      ports.Clock
}

type PrepareTransportOpportunityHandler struct {
	deps PrepareTransportOpportunityDeps
}

func NewPrepareTransportOpportunityHandler(deps PrepareTransportOpportunityDeps) *PrepareTransportOpportunityHandler {
	return &PrepareTransportOpportunityHandler{deps: deps}
}

// EstablishSchedule 建立班次：纯执行载体（无容量/价格字段，领域已钉），幂等按
// （租户+班次标识）分重放/冲突。
func (handler *PrepareTransportOpportunityHandler) EstablishSchedule(
	ctx context.Context,
	command EstablishScheduleCommand,
) (OpportunityResult, error) {
	scheduleRef, err := domain.NewScheduleReference(command.Schedule)
	if err != nil {
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}
	schedule, err := domain.FormTransportSchedule(domain.TransportScheduleSpec{
		TenantID:  command.TenantID,
		Schedule:  scheduleRef,
		Direction: command.Direction,
		DepartsAt: command.DepartsAt,
	})
	if err != nil {
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}

	key := ports.ScheduleKey{TenantID: command.TenantID, Schedule: scheduleRef}
	digest := opportunityDigest(command.Direction, command.DepartsAt.UTC().Format(time.RFC3339Nano))
	existing, found, err := handler.deps.Schedules.FindByKey(ctx, key)
	if err != nil {
		return opportunityUndecided(ScheduleStoreUnavailable, command.Schedule), nil
	}
	if found {
		if existing.ContentDigest != digest {
			return OpportunityResult{outcome: ScheduleConflict}, nil
		}
		return OpportunityResult{outcome: ScheduleExistingResult, schedule: existing, hasRecord: true}, nil
	}

	record := ports.ScheduleRecord{Key: key, ContentDigest: digest, Schedule: schedule, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Schedules.Save(ctx, record)
	if err != nil {
		return opportunityUndecided(ScheduleStoreUnavailable, command.Schedule), nil
	}
	switch saved {
	case ports.ScheduleSaved:
		return OpportunityResult{outcome: ScheduleEstablished, schedule: record, hasRecord: true}, nil
	case ports.ScheduleAlreadyEstablished:
		winner, found, err := handler.deps.Schedules.FindByKey(ctx, key)
		if err != nil || !found {
			return opportunityUndecided(ScheduleStoreUnavailable, command.Schedule), nil
		}
		return OpportunityResult{outcome: ScheduleExistingResult, schedule: winner, hasRecord: true}, nil
	default:
		return OpportunityResult{}, fmt.Errorf("%w: %d", ErrUnexpectedScheduleSave, saved)
	}
}

// EstablishPool 建立容量池：班次存在≠容量可用——池是单独建立的有限量（领域已钉），
// 幂等分重放/冲突。
func (handler *PrepareTransportOpportunityHandler) EstablishPool(
	ctx context.Context,
	command EstablishPoolCommand,
) (OpportunityResult, error) {
	spec := domain.CapacityPoolSpec{TenantID: command.TenantID, Capacity: command.Capacity}
	var err error
	if spec.Pool, err = domain.NewCapacityPoolReference(command.Pool); err != nil {
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}
	if spec.Schedule, err = domain.NewScheduleReference(command.Schedule); err != nil {
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}
	if spec.Unit, err = domain.NewQuantityUnitReference(command.Unit); err != nil {
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}
	pool, err := domain.EstablishCapacityPool(spec)
	if err != nil {
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}

	key := ports.CapacityPoolKey{TenantID: command.TenantID, Pool: pool.Pool()}
	digest := opportunityDigest(command.Schedule, command.Unit, fmt.Sprintf("%d", command.Capacity))
	existing, found, err := handler.deps.Pools.FindByKey(ctx, key)
	if err != nil {
		return opportunityUndecided(PoolStoreUnavailable, command.Pool), nil
	}
	if found {
		if existing.ContentDigest != digest {
			return OpportunityResult{outcome: PoolConflict}, nil
		}
		return OpportunityResult{outcome: PoolExistingResult, pool: existing, hasRecord: true}, nil
	}

	record := ports.CapacityPoolRecord{Key: key, ContentDigest: digest, Pool: pool, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Pools.Save(ctx, record)
	if err != nil {
		return opportunityUndecided(PoolStoreUnavailable, command.Pool), nil
	}
	switch saved {
	case ports.PoolSaved:
		return OpportunityResult{outcome: PoolEstablished, pool: record, hasRecord: true}, nil
	case ports.PoolAlreadyEstablished:
		winner, found, err := handler.deps.Pools.FindByKey(ctx, key)
		if err != nil || !found {
			return opportunityUndecided(PoolStoreUnavailable, command.Pool), nil
		}
		return OpportunityResult{outcome: PoolExistingResult, pool: winner, hasRecord: true}, nil
	default:
		return OpportunityResult{}, fmt.Errorf("%w: %d", ErrUnexpectedPoolSave, saved)
	}
}

// Reserve 建立预占：同引用同量重放返原、同引用异量冲突；超硬上限 → CAPACITY_EXCEEDED
// 业务负向带当时余量（拒绝带答案，不能以审批绕过）。
func (handler *PrepareTransportOpportunityHandler) Reserve(
	ctx context.Context,
	command ReserveCapacityCommand,
) (OpportunityResult, error) {
	record, reservationRef, result := handler.loadPool(ctx, command.TenantID, command.Pool, command.Reservation)
	if result != nil {
		return *result, nil
	}
	if existing, exists := record.Pool.ReservationFor(reservationRef); exists {
		reserved, _, _ := existing.Quantities()
		if reserved == command.Quantity {
			// 同引用同量：预占重放，返回原池不重扣。
			return OpportunityResult{outcome: ReservationExistingResult, pool: record, hasRecord: true}, nil
		}
		return OpportunityResult{outcome: ReservationConflict}, nil
	}

	reservedPool, err := record.Pool.Reserve(reservationRef, command.Quantity, command.ValidUntil, command.At)
	if errors.Is(err, domain.ErrCapacityExceeded) {
		return OpportunityResult{
			outcome:   CapacityExceededOutcome,
			available: record.Pool.AvailableAt(command.At),
		}, nil
	}
	if err != nil {
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}
	return handler.replacePool(ctx, record, reservedPool, CapacityReserved, command.Pool)
}

// Release 显式释放未消耗范围。过期与守恒的领域哨兵各归业务负向一格。
func (handler *PrepareTransportOpportunityHandler) Release(
	ctx context.Context,
	command ReleaseCapacityCommand,
) (OpportunityResult, error) {
	record, reservationRef, result := handler.loadPool(ctx, command.TenantID, command.Pool, command.Reservation)
	if result != nil {
		return *result, nil
	}
	releasedPool, err := record.Pool.Release(reservationRef, command.Quantity, command.At)
	if mapped := capacitySentinel(err); mapped != OpportunityOutcomeInvalid {
		return OpportunityResult{outcome: mapped}, nil
	}
	if err != nil {
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}
	return handler.replacePool(ctx, record, releasedPool, CapacityReleased, command.Pool)
}

// Consume 依据装载分配确认消耗，成功后把消耗交给装载分配链。
func (handler *PrepareTransportOpportunityHandler) Consume(
	ctx context.Context,
	command ConsumeCapacityCommand,
) (OpportunityResult, error) {
	record, reservationRef, result := handler.loadPool(ctx, command.TenantID, command.Pool, command.Reservation)
	if result != nil {
		return *result, nil
	}
	assignment, err := domain.NewLoadAssignmentReference(command.Assignment)
	if err != nil {
		// 没有装载分配依据的消耗无从谈起（领域硬句）。
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}
	consumedPool, err := record.Pool.Consume(reservationRef, command.Quantity, assignment, command.At)
	if mapped := capacitySentinel(err); mapped != OpportunityOutcomeInvalid {
		return OpportunityResult{outcome: mapped}, nil
	}
	if err != nil {
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}
	out, err := handler.replacePool(ctx, record, consumedPool, CapacityConsumed, command.Pool)
	if err != nil || out.outcome != CapacityConsumed {
		return out, err
	}
	if handoffErr := handler.deps.Downstream.HandOffCapacityConsumption(ctx, ports.CapacityConsumptionIntent{
		Pool:        out.pool,
		Reservation: reservationRef,
		Assignment:  assignment,
		Quantity:    command.Quantity,
	}); handoffErr != nil {
		// 投递失败不翻结果：消耗已经落定，留续办引用重放时重发同一份。
		out.handoff = opportunityContinuation("CAPACITY_CONSUMPTION_HANDOFF", command.Pool, command.Reservation)
	}
	return out, nil
}

// loadPool 受理池与预占引用并读回当前池值。
func (handler *PrepareTransportOpportunityHandler) loadPool(
	ctx context.Context,
	tenant domain.TenantID,
	pool string,
	reservation string,
) (ports.CapacityPoolRecord, domain.CapacityReservationReference, *OpportunityResult) {
	notAccepted := &OpportunityResult{outcome: OpportunityNotAccepted}
	poolRef, err := domain.NewCapacityPoolReference(pool)
	if err != nil {
		return ports.CapacityPoolRecord{}, domain.CapacityReservationReference{}, notAccepted
	}
	reservationRef, err := domain.NewCapacityReservationReference(reservation)
	if err != nil {
		return ports.CapacityPoolRecord{}, domain.CapacityReservationReference{}, notAccepted
	}
	record, found, err := handler.deps.Pools.FindByKey(ctx, ports.CapacityPoolKey{TenantID: tenant, Pool: poolRef})
	if err != nil {
		undecided := opportunityUndecided(PoolStoreUnavailable, pool)
		return ports.CapacityPoolRecord{}, domain.CapacityReservationReference{}, &undecided
	}
	if !found {
		return ports.CapacityPoolRecord{}, domain.CapacityReservationReference{}, notAccepted
	}
	return record, reservationRef, nil
}

// replacePool 把转换后的池换值入库。
func (handler *PrepareTransportOpportunityHandler) replacePool(
	ctx context.Context,
	record ports.CapacityPoolRecord,
	pool domain.CapacityPool,
	outcome OpportunityOutcome,
	poolName string,
) (OpportunityResult, error) {
	record.Pool = pool
	record.RecordedAt = handler.deps.Clock.Now()
	ok, err := handler.deps.Pools.Replace(ctx, record)
	if err != nil {
		return opportunityUndecided(PoolStoreUnavailable, poolName), nil
	}
	if !ok {
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}
	return OpportunityResult{outcome: outcome, pool: record, hasRecord: true}, nil
}

// capacitySentinel 把容量转换的领域哨兵透出为业务负向格：过期重新预占、守恒撞线不得
// 二次转换；查无预占是提交矛盾，交回零值由调用方落未受理。
func capacitySentinel(err error) OpportunityOutcome {
	switch {
	case errors.Is(err, domain.ErrReservationExpired):
		return ReservationExpiredOutcome
	case errors.Is(err, domain.ErrReservationOverdrawn):
		return ReservationOverdrawnOutcome
	default:
		return OpportunityOutcomeInvalid
	}
}

func opportunityUndecided(reason OpportunityUndecidedReason, subject string) OpportunityResult {
	return OpportunityResult{
		outcome:      OpportunityUndecided,
		reason:       reason,
		continuation: opportunityContinuation(reason.String(), subject),
	}
}

func opportunityContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

func opportunityDigest(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}
