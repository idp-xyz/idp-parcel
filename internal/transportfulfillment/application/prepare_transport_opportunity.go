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

// ErrUnexpectedPoolSave 说明容量池库交回了封闭集合以外的写入结果。
var ErrUnexpectedPoolSave = errors.New("transport fulfillment: unexpected capacity pool save outcome")

// OpportunityOutcome 是运输机会准备的应用处理结果。容量三格是业务负向（ADR-0029）：
// 超额预留带余量（等容量或换班次）、预占过期（重新预占）、守恒撞线（同一范围不得二次
// 转换）——都是已知答案，不是改单也不是等依赖。建池时班次不在册也是一格业务负向：
// 续办是先登班次，与改载荷不同。预占与释放成了运营决定之后（ADR-0203 决定六），池不在册、
// 预占不在册、单位不是池的单位各成一格，续办分别是先登池或改指池、改指预占、按池的单位
// 重述数量，都不并进`未受理`。`未受理`的线上名与 TF 其余运营决定口、登记口同名。
type OpportunityOutcome uint8

const (
	OpportunityOutcomeInvalid OpportunityOutcome = iota
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
	PoolScheduleNotFound
	PoolNotFound
	ReservationNotFound
	ReleaseExistingResult
	ReleaseConflict
	DemandUnitMismatch
)

func (outcome OpportunityOutcome) String() string {
	switch outcome {
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
		return "INPUT_NOT_ACCEPTED"
	case OpportunityUndecided:
		return "OPPORTUNITY_UNDECIDED"
	case PoolScheduleNotFound:
		return "SCHEDULE_NOT_FOUND"
	case PoolNotFound:
		return "POOL_NOT_FOUND"
	case ReservationNotFound:
		return "RESERVATION_NOT_FOUND"
	case ReleaseExistingResult:
		return "EXISTING_RELEASE"
	case ReleaseConflict:
		return "RELEASE_CONFLICT"
	case DemandUnitMismatch:
		return "UNIT_MISMATCH"
	default:
		return ""
	}
}

// OpportunityUndecidedReason 指名提交停在哪一步等谁。
type OpportunityUndecidedReason uint8

const (
	OpportunityUndecidedReasonNone OpportunityUndecidedReason = iota
	PoolStoreUnavailable
	PoolScheduleRegistryUnavailable
)

func (reason OpportunityUndecidedReason) String() string {
	switch reason {
	case PoolStoreUnavailable:
		return "POOL_STORE_UNAVAILABLE"
	case PoolScheduleRegistryUnavailable:
		return "TRANSPORT_SCHEDULE_REGISTRY_UNAVAILABLE"
	default:
		return ""
	}
}

// EstablishPoolCommand 在一个已登记的班次上登一个维度的池（ADR-0201 决定一：容量池走登记册配置写）。池身份不在
// 命令里，由班次与维度派生（ADR-0201 补记「池身份」）；维度取 domain.CapacityDimension 的封闭词。
type EstablishPoolCommand struct {
	TenantID  domain.TenantID
	Schedule  string
	Dimension string
	Unit      string
	Capacity  int64
}

// ReserveCapacityCommand 是一次预占决定（ADR-0203 决定三）：在哪个池、用哪个预占引用、为多少需求量占到何时。需求量
// 带单位与来源——来源种类取 domain.CapacityDemandSourceKind 的封闭词，引用指名运营声明的那一版或准备结果的需求快照。
// At 是判断时点，由声明方给。
type ReserveCapacityCommand struct {
	TenantID        domain.TenantID
	Pool            string
	Reservation     string
	Quantity        int64
	Unit            string
	SourceKind      string
	SourceReference string
	ValidUntil      time.Time
	At              time.Time
}

// ReleaseCapacityCommand 是一次释放决定：释放引用认领这一次释放（重放的键），其余同预占，来源说的是为什么不再需要这些量。
type ReleaseCapacityCommand struct {
	TenantID        domain.TenantID
	Pool            string
	Reservation     string
	Release         string
	Quantity        int64
	Unit            string
	SourceKind      string
	SourceReference string
	At              time.Time
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
	pool         ports.CapacityPoolRecord
	hasRecord    bool
	reservation  domain.CapacityReservationReference
	release      domain.CapacityReleaseReference
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

func (result OpportunityResult) Pool() (ports.CapacityPoolRecord, bool) {
	return result.pool, result.hasRecord && result.pool.Key.Pool.String() != ""
}

// Reservation 交回这一格说的那一份预占，取自交回的池：新立的、重放认出的、冲突里挡路的、释放所落的那一份。
func (result OpportunityResult) Reservation() (domain.CapacityReservation, bool) {
	record, present := result.Pool()
	if !present || result.reservation.String() == "" {
		return domain.CapacityReservation{}, false
	}
	return record.Pool.ReservationFor(result.reservation)
}

// Release 交回这一格说的那一次释放与它所落的预占，取自交回的池：新落的、重放认出的、冲突里挡路的那一次。
func (result OpportunityResult) Release() (domain.CapacityReservationReference, domain.CapacityRelease, bool) {
	record, present := result.Pool()
	if !present || result.release.String() == "" {
		return domain.CapacityReservationReference{}, domain.CapacityRelease{}, false
	}
	return record.Pool.ReleaseFor(result.release)
}

// AvailableQuantity 在超额预留时报告当时余量——拒绝要带着答案，调用方据此改量或换班次。余量取预占整段里最紧那一刻的
// （ADR-0203 决定四），所以它可能小于判断时点那一刻的余量。
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

// PrepareTransportOpportunityDeps 是本编排的缝。Schedules 只供建池核「班次在册」；Downstream 只有 Consume 用——
// 只装建池一口的装配点（登记写面与 CLI）不接它，也就交不出调 Consume 的入口。
type PrepareTransportOpportunityDeps struct {
	Pools      ports.CapacityPoolStore
	Schedules  ports.TransportScheduleReader
	Downstream ports.CapacityConsumptionHandoff
	Clock      ports.Clock
}

type PrepareTransportOpportunityHandler struct {
	deps PrepareTransportOpportunityDeps
}

func NewPrepareTransportOpportunityHandler(deps PrepareTransportOpportunityDeps) *PrepareTransportOpportunityHandler {
	return &PrepareTransportOpportunityHandler{deps: deps}
}

// EstablishPool 在一个已登记的班次上建立一个维度的容量池：班次存在≠容量可用——池是单独建立的有限量（领域已钉）。
//
// 次序照装载分配核班次的先例：先按池身份看重放与冲突，再核班次在册。已建的池答的是「这个池是什么」，交回的是库里
// 那一个，与这次送来的班次此刻在不在册无关；先核班次会把一次重放改答成去登班次，而登完再投还是那一格。班次不在册
// （含指向他租户的班次——按本租户的键读不回）答`班次不在册`，不建池：只经准备评估物化、没有显式登记的班次不算在册，
// 池不能挂在一个册上没有的班次上（方案 D03-6）。
func (handler *PrepareTransportOpportunityHandler) EstablishPool(
	ctx context.Context,
	command EstablishPoolCommand,
) (OpportunityResult, error) {
	pool, err := capacityPoolFrom(command)
	if err != nil {
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}
	key := ports.CapacityPoolKey{TenantID: command.TenantID, Pool: pool.Pool()}
	_, digest, err := domain.CanonicalizeEstablishPoolPayload(pool)
	if err != nil {
		return OpportunityResult{}, err
	}
	existing, found, err := handler.deps.Pools.FindByKey(ctx, key)
	if err != nil {
		return opportunityUndecided(PoolStoreUnavailable, key.Pool.String()), nil
	}
	if found {
		return poolReplayOrConflict(existing, digest, key.Pool.String()), nil
	}

	_, registered, err := handler.deps.Schedules.FindByKey(ctx, ports.ScheduleKey{
		TenantID: command.TenantID,
		Schedule: pool.Schedule(),
	})
	if err != nil {
		return opportunityUndecided(PoolScheduleRegistryUnavailable, key.Pool.String()), nil
	}
	if !registered {
		return OpportunityResult{outcome: PoolScheduleNotFound}, nil
	}
	return saveCapacityPool(ctx, handler.deps.Pools, handler.deps.Clock, pool, digest)
}

func capacityPoolFrom(command EstablishPoolCommand) (domain.CapacityPool, error) {
	schedule, err := domain.NewScheduleReference(command.Schedule)
	if err != nil {
		return domain.CapacityPool{}, err
	}
	dimension, err := domain.ParseCapacityDimension(command.Dimension)
	if err != nil {
		return domain.CapacityPool{}, err
	}
	unit, err := domain.NewQuantityUnitReference(command.Unit)
	if err != nil {
		return domain.CapacityPool{}, err
	}
	return domain.EstablishCapacityPool(domain.CapacityPoolSpec{
		TenantID:  command.TenantID,
		Schedule:  schedule,
		Dimension: dimension,
		Unit:      unit,
		Capacity:  command.Capacity,
	})
}

// establishMaterializedPool 立一个由周期安排容量声明物化出的池。与 EstablishPool 只差不核班次在册：调用方是登记
// 那一班的同一笔事务，班次刚落册或本来就在。
func establishMaterializedPool(
	ctx context.Context,
	pools ports.CapacityPoolStore,
	clock ports.Clock,
	pool domain.CapacityPool,
) (OpportunityResult, error) {
	key := ports.CapacityPoolKey{TenantID: pool.TenantID(), Pool: pool.Pool()}
	_, digest, err := domain.CanonicalizeEstablishPoolPayload(pool)
	if err != nil {
		return OpportunityResult{}, err
	}
	existing, found, err := pools.FindByKey(ctx, key)
	if err != nil {
		return opportunityUndecided(PoolStoreUnavailable, key.Pool.String()), nil
	}
	if found {
		return poolReplayOrConflict(existing, digest, key.Pool.String()), nil
	}
	return saveCapacityPool(ctx, pools, clock, pool, digest)
}

// poolReplayOrConflict 比已建的池与这次的内容：同即`已存在`，异即`池冲突`。登记口立池而不更正池：同身份上再登一份
// 不同内容，读不出是登错了还是容量变了，后者按 TF CONTEXT 要另走更正（缩容不改写既有预占、形成超配缺口），不该由
// 登记口顺手改上限，所以答冲突而不是像班次那样成下一版（ADR-0201 补记「登记口不改池」）。冲突也交回库里那一个池：
// 登记方要看见挡住他的是什么。两侧摘要都是 TFC-1：池表在迁移 0027 之前无行，没有无版本那一形可比。
func poolReplayOrConflict(existing ports.CapacityPoolRecord, digest string, subject string) OpportunityResult {
	switch domain.CompareStoredDigest(existing.ContentDigest, digest, "") {
	case domain.SamePayload:
		return OpportunityResult{outcome: PoolExistingResult, pool: existing, hasRecord: true}
	case domain.DifferentPayload:
		return OpportunityResult{outcome: PoolConflict, pool: existing, hasRecord: true}
	default:
		return opportunityUndecided(PoolStoreUnavailable, subject)
	}
}

func saveCapacityPool(
	ctx context.Context,
	pools ports.CapacityPoolStore,
	clock ports.Clock,
	pool domain.CapacityPool,
	digest string,
) (OpportunityResult, error) {
	key := ports.CapacityPoolKey{TenantID: pool.TenantID(), Pool: pool.Pool()}
	record := ports.CapacityPoolRecord{Key: key, ContentDigest: digest, Pool: pool, RecordedAt: clock.Now()}
	saved, err := pools.Save(ctx, record)
	if err != nil {
		return opportunityUndecided(PoolStoreUnavailable, key.Pool.String()), nil
	}
	switch saved {
	case ports.PoolSaved:
		return OpportunityResult{outcome: PoolEstablished, pool: record, hasRecord: true}, nil
	case ports.PoolAlreadyEstablished:
		// 并发下另一方先建了同一个池：读回赢家按内容分重放与冲突，不宣称自己建了它。
		winner, found, err := pools.FindByKey(ctx, key)
		if err != nil || !found {
			return opportunityUndecided(PoolStoreUnavailable, key.Pool.String()), nil
		}
		return poolReplayOrConflict(winner, digest, key.Pool.String()), nil
	default:
		return OpportunityResult{}, fmt.Errorf("%w: %d", ErrUnexpectedPoolSave, saved)
	}
}

// Reserve 建立一份预占（ADR-0203 决定三至六）。次序：载荷过构造门 → 读池 → 按预占引用分重放与冲突 → 单位与池比 →
// 硬上限 → 落池。
//
// 重放判在单位与上限之前：同一份预占原样重交答原预占，哪怕此刻池已经占满——它占的就是那一份。冲突与单位不符都交回
// 库里的池（冲突另指挡路的那一份预占），声明方要看见挡住他的是什么。超硬上限答`容量超出`并带预占整段里最紧那一刻的
// 余量；本编排没有任何越过上限的入口，审批也不行（AT-TF-031）。
func (handler *PrepareTransportOpportunityHandler) Reserve(
	ctx context.Context,
	command ReserveCapacityCommand,
) (OpportunityResult, error) {
	reservationRef, err := domain.NewCapacityReservationReference(command.Reservation)
	if err != nil {
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}
	demand, err := capacityDemandFrom(command.Quantity, command.Unit, command.SourceKind, command.SourceReference)
	at := domain.AtRegistryPrecision(command.At)
	if err != nil || at.IsZero() || !domain.AtRegistryPrecision(command.ValidUntil).After(at) {
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}
	record, result := handler.loadPool(ctx, command.TenantID, command.Pool)
	if result != nil {
		return *result, nil
	}
	if existing, exists := record.Pool.ReservationFor(reservationRef); exists {
		outcome := ReservationConflict
		if existing.Declares(demand, command.ValidUntil, command.At) {
			outcome = ReservationExistingResult
		}
		return OpportunityResult{outcome: outcome, pool: record, hasRecord: true, reservation: reservationRef}, nil
	}

	reservedPool, err := record.Pool.Reserve(reservationRef, demand, command.ValidUntil, command.At)
	switch {
	case errors.Is(err, domain.ErrCapacityUnitMismatch):
		return OpportunityResult{outcome: DemandUnitMismatch, pool: record, hasRecord: true}, nil
	case errors.Is(err, domain.ErrCapacityExceeded):
		return OpportunityResult{
			outcome:   CapacityExceededOutcome,
			pool:      record,
			hasRecord: true,
			available: record.Pool.AvailableThrough(command.At, command.ValidUntil),
		}, nil
	case err != nil:
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}
	out, err := handler.replacePool(ctx, record, reservedPool, CapacityReserved, command.Pool)
	out.reservation = reservationRef
	return out, err
}

// Release 显式释放一份预占上未消耗的范围（ADR-0203 决定三、五、六）。次序：载荷过构造门 → 读池 → 按释放引用分重放与
// 冲突 → 预占在册 → 单位 → 过期与守恒 → 落池。释放引用在池内认领：同一引用原样重交答原释放，改了数量、来源、时点或
// 挪到另一份预占上都是冲突——不认领的话，每重交一次就多还一份容量。
func (handler *PrepareTransportOpportunityHandler) Release(
	ctx context.Context,
	command ReleaseCapacityCommand,
) (OpportunityResult, error) {
	reservationRef, err := domain.NewCapacityReservationReference(command.Reservation)
	if err != nil {
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}
	releaseRef, err := domain.NewCapacityReleaseReference(command.Release)
	if err != nil {
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}
	demand, err := capacityDemandFrom(command.Quantity, command.Unit, command.SourceKind, command.SourceReference)
	if err != nil || domain.AtRegistryPrecision(command.At).IsZero() {
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}
	record, result := handler.loadPool(ctx, command.TenantID, command.Pool)
	if result != nil {
		return *result, nil
	}
	if on, existing, taken := record.Pool.ReleaseFor(releaseRef); taken {
		outcome := ReleaseConflict
		if on == reservationRef && existing.Declares(demand, command.At) {
			outcome = ReleaseExistingResult
		}
		return OpportunityResult{outcome: outcome, pool: record, hasRecord: true, reservation: on, release: releaseRef}, nil
	}
	if _, exists := record.Pool.ReservationFor(reservationRef); !exists {
		return OpportunityResult{outcome: ReservationNotFound, pool: record, hasRecord: true}, nil
	}

	releasedPool, err := record.Pool.Release(releaseRef, reservationRef, demand, command.At)
	if errors.Is(err, domain.ErrCapacityUnitMismatch) {
		return OpportunityResult{outcome: DemandUnitMismatch, pool: record, hasRecord: true, reservation: reservationRef}, nil
	}
	if mapped := capacitySentinel(err); mapped != OpportunityOutcomeInvalid {
		// 过期与守恒撞线都交回那一份预占：三量在上面，声明方据此看还能释放多少。
		return OpportunityResult{outcome: mapped, pool: record, hasRecord: true, reservation: reservationRef}, nil
	}
	if err != nil {
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}
	out, err := handler.replacePool(ctx, record, releasedPool, CapacityReleased, command.Pool)
	out.reservation = reservationRef
	out.release = releaseRef
	return out, err
}

// capacityDemandFrom 把命令里的需求量各格过构造门。哪一格不成立都是改载荷，不分格。
func capacityDemandFrom(quantity int64, unit, sourceKind, sourceReference string) (domain.CapacityDemand, error) {
	unitRef, err := domain.NewQuantityUnitReference(unit)
	if err != nil {
		return domain.CapacityDemand{}, err
	}
	kind, err := domain.ParseCapacityDemandSourceKind(sourceKind)
	if err != nil {
		return domain.CapacityDemand{}, err
	}
	reference, err := domain.NewCapacityDemandSourceReference(sourceReference)
	if err != nil {
		return domain.CapacityDemand{}, err
	}
	source, err := domain.NewCapacityDemandSource(kind, reference)
	if err != nil {
		return domain.CapacityDemand{}, err
	}
	return domain.NewCapacityDemand(quantity, unitRef, source)
}

// Consume 依据装载分配确认消耗，成功后把消耗交给装载分配链。
func (handler *PrepareTransportOpportunityHandler) Consume(
	ctx context.Context,
	command ConsumeCapacityCommand,
) (OpportunityResult, error) {
	reservationRef, err := domain.NewCapacityReservationReference(command.Reservation)
	if err != nil {
		return OpportunityResult{outcome: OpportunityNotAccepted}, nil
	}
	record, result := handler.loadPool(ctx, command.TenantID, command.Pool)
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

// loadPool 受理池引用并读回当前池值。池不在册（含他租户的池——按本租户的键读不回）答`池不在册`：续办是先登池或改指
// 在册的池，与改载荷不同。
func (handler *PrepareTransportOpportunityHandler) loadPool(
	ctx context.Context,
	tenant domain.TenantID,
	pool string,
) (ports.CapacityPoolRecord, *OpportunityResult) {
	poolRef, err := domain.NewCapacityPoolReference(pool)
	if err != nil {
		return ports.CapacityPoolRecord{}, &OpportunityResult{outcome: OpportunityNotAccepted}
	}
	record, found, err := handler.deps.Pools.FindByKey(ctx, ports.CapacityPoolKey{TenantID: tenant, Pool: poolRef})
	if err != nil {
		undecided := opportunityUndecided(PoolStoreUnavailable, pool)
		return ports.CapacityPoolRecord{}, &undecided
	}
	if !found {
		return ports.CapacityPoolRecord{}, &OpportunityResult{outcome: PoolNotFound}
	}
	return record, nil
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
// 二次转换、查无预占改指在册的那一份；其余交回零值由调用方落未受理。
func capacitySentinel(err error) OpportunityOutcome {
	switch {
	case errors.Is(err, domain.ErrReservationExpired):
		return ReservationExpiredOutcome
	case errors.Is(err, domain.ErrReservationOverdrawn):
		return ReservationOverdrawnOutcome
	case errors.Is(err, domain.ErrReservationNotFound):
		return ReservationNotFound
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
