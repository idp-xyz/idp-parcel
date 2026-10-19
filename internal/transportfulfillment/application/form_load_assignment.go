package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// LoadAssignmentOutcome 是形成一次装载分配的结果代数。
//
// `已在册`独立成格而不是复用`已形成`：同一版本重投交回的是**原版本**，而调用方送来的可能是
// 另一份对象范围——两者答案相同会让「我这一份生效了」与「原来那份仍在」分不开。
//
// `分配已形成`与`已在册`也分开：前者是同一分配下已有别的版本，交回的是**链头**（可能是变化版或撤回版），
// 后者交回的是同一版本本身。调用方拿前者要去变化或撤回的入口，拿后者知道自己这一版早就在了。
//
// `班次不在册`独立成格而不并进`输入未受理`：载荷本身写得对，续办是去登记班次或换一个在册班次，不是改载荷
// （ADR-0029 按恢复动作分格）。他租户名下的同名班次同落这一格——按本租户的键读不回，答复也不透露它在别处存在。
//
// 容量保障各格（ADR-0203 决定一、二）都要对着班次登记的池与在册的预占才判得出，所以都不并进`输入未受理`：
//   - `须引用预占`：班次挂了池而有池没有分配行，续办是先预占、再逐池引用；
//   - `预占不在本班次`：某一行指的池不是本班次的池，或那份预占不在那个池上，续办是改指本班次池上的在册预占；
//   - `单位不符`、`预占尚未生效`、`预占过期`：与预占口同名同义，续办分别是按池的单位重述、按预占的有效区间重述分配
//     时点、另占一份；
//   - `分配超出`：带那一份预占此刻剩下的量，续办是改量、另占一份或先撤别的分配。
type LoadAssignmentOutcome uint8

const (
	LoadAssignmentOutcomeInvalid LoadAssignmentOutcome = iota
	LoadAssignmentFormed
	LoadAssignmentVersionExists
	LoadAssignmentNotAccepted
	LoadAssignmentUndecided
	LoadAssignmentAlreadyFormed
	LoadAssignmentScheduleNotFound
	LoadAssignmentCapacityReservationRequired
	LoadAssignmentReservationNotOnSchedule
	LoadAssignmentUnitMismatch
	LoadAssignmentReservationNotYetEffective
	LoadAssignmentReservationExpired
	LoadAssignmentAllocationExceeded
)

func (outcome LoadAssignmentOutcome) String() string {
	switch outcome {
	case LoadAssignmentFormed:
		return "LOAD_ASSIGNMENT_FORMED"
	case LoadAssignmentVersionExists:
		return "LOAD_ASSIGNMENT_VERSION_EXISTS"
	case LoadAssignmentNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case LoadAssignmentUndecided:
		return "LOAD_ASSIGNMENT_UNDECIDED"
	case LoadAssignmentAlreadyFormed:
		return "LOAD_ASSIGNMENT_ALREADY_FORMED"
	case LoadAssignmentScheduleNotFound:
		return "SCHEDULE_NOT_FOUND"
	case LoadAssignmentCapacityReservationRequired:
		return "CAPACITY_RESERVATION_REQUIRED"
	case LoadAssignmentReservationNotOnSchedule:
		return "RESERVATION_NOT_ON_SCHEDULE"
	case LoadAssignmentUnitMismatch:
		return "UNIT_MISMATCH"
	case LoadAssignmentReservationNotYetEffective:
		return "RESERVATION_NOT_YET_EFFECTIVE"
	case LoadAssignmentReservationExpired:
		return "RESERVATION_EXPIRED"
	case LoadAssignmentAllocationExceeded:
		return "ALLOCATION_EXCEEDED"
	default:
		return ""
	}
}

// FormLoadAssignmentCommand 携带形成一次装载分配所需的全部输入。
//
// 版本由调用方指名而不是这里铸：它是分配链上的身份，变化与撤回各自带来下一个版本号，编排
// 铸号就等于替调用方决定「这算不算同一次分配的下一版」。
//
// Allocations 是逐池的分配量（ADR-0203 决定一、二）：班次挂了容量池时每个池一行，引用那个池上的一份预占；没挂池的
// 班次不要求，带了行反倒是指错了池。
type FormLoadAssignmentCommand struct {
	TenantID    domain.TenantID
	Assignment  string
	Schedule    string
	Members     []string
	Version     string
	AssignedAt  time.Time
	Allocations []LoadAssignmentAllocationLine
}

// LoadAssignmentAllocationLine 是一行分配量：哪个池上的哪一份预占、分多少、什么单位、依据什么来源。FromMembers 说数量取
// 分配的成员数，只许落在件数维的池上，此时 Quantity 留零。来源种类取 domain.CapacityDemandSourceKind 的封闭词。
type LoadAssignmentAllocationLine struct {
	Pool            string
	Reservation     string
	Quantity        int64
	FromMembers     bool
	Unit            string
	SourceKind      string
	SourceReference string
}

type FormLoadAssignmentResult struct {
	outcome      LoadAssignmentOutcome
	record       ports.LoadAssignmentRecord
	hasRecord    bool
	continuation string

	uncovered  []ports.CapacityPoolRecord
	misplaced  []domain.CapacityAllocation
	allocation domain.CapacityAllocation
	pool       ports.CapacityPoolRecord
	hasPool    bool
	available  int64
}

func (result FormLoadAssignmentResult) Outcome() LoadAssignmentOutcome {
	return result.outcome
}

func (result FormLoadAssignmentResult) Record() (ports.LoadAssignmentRecord, bool) {
	return result.record, result.hasRecord
}

// ContinuationReference 只在`未决`时非空，供调用方续办同一次分配。
func (result FormLoadAssignmentResult) ContinuationReference() string {
	return result.continuation
}

// PoolsWithoutReservation 只在`须引用预占`时非空：本班次上没有分配行的那几个池，按池身份排。
func (result FormLoadAssignmentResult) PoolsWithoutReservation() []ports.CapacityPoolRecord {
	return append([]ports.CapacityPoolRecord(nil), result.uncovered...)
}

// AllocationsNotOnSchedule 只在`预占不在本班次`时非空：指错了的那几行，按池身份排。
func (result FormLoadAssignmentResult) AllocationsNotOnSchedule() []domain.CapacityAllocation {
	return append([]domain.CapacityAllocation(nil), result.misplaced...)
}

// RefusedAllocation 交回逐行判定挡下的那一行与它所在的池（单位不符、尚未生效、过期、分配超出）。池是读到的那一个，
// 那份预占连同它的三量与分配关系在上面，声明方据此看该怎么改。
func (result FormLoadAssignmentResult) RefusedAllocation() (domain.CapacityAllocation, ports.CapacityPoolRecord, bool) {
	return result.allocation, result.pool, result.hasPool
}

// AvailableQuantity 只在`分配超出`时有意义：那一份预占此刻还能分给这次分配的量，单位同池。
func (result FormLoadAssignmentResult) AvailableQuantity() int64 {
	return result.available
}

// FormLoadAssignmentDeps 是本编排的缝。Pools 按班次读登记册上的池连同预占与分配关系，在写事务里锁住各池行：判守恒与
// 落分配量同一笔，同一个池上并发的分配、预占与释放因此串行判。
type FormLoadAssignmentDeps struct {
	Assignments ports.LoadAssignmentRegistry
	Schedules   ports.TransportScheduleReader
	Pools       ports.CapacityPoolsOnSchedules
	Clock       ports.Clock
}

type FormLoadAssignmentHandler struct {
	deps FormLoadAssignmentDeps
}

func NewFormLoadAssignmentHandler(deps FormLoadAssignmentDeps) *FormLoadAssignmentHandler {
	return &FormLoadAssignmentHandler{deps: deps}
}

// Form 形成一次装载分配的首版并登记：受理（对象范围、分配量各行等由 FormLoadAssignment 构造门把门）
// → 幂等按（租户 + 分配 + 版本）分重放 → 按（租户 + 分配）认出已形成的分配 → 按（租户 + 班次）核班次在册
// → 容量保障（ADR-0203 决定一、二）→ 登记。
//
// **分配是执行意图，不是已经发生的装载。** 它不证明物理装载完成，也不证明控制转移；实际装载、
// 短装、多装或错装是与分配版本比较的独立事实，不修改分配历史。所以这里没有任何已装载入参，
// 领域类型上也没有那些字段。
func (handler *FormLoadAssignmentHandler) Form(
	ctx context.Context,
	command FormLoadAssignmentCommand,
) (FormLoadAssignmentResult, error) {
	spec, key, accepted := loadAssignmentSpecFrom(command)
	if !accepted {
		return FormLoadAssignmentResult{outcome: LoadAssignmentNotAccepted}, nil
	}
	assignment, err := domain.FormLoadAssignment(spec)
	if err != nil {
		return FormLoadAssignmentResult{outcome: LoadAssignmentNotAccepted}, nil
	}

	existing, found, err := handler.deps.Assignments.FindByKey(ctx, key)
	if err != nil {
		return loadAssignmentUndecided(command), nil
	}
	if found {
		// 同一版本重投：交回原版本，不顶替。先按键查再看链头，顺序不能反：链头已长出变化或撤回之后，
		// 原样重放首版仍该答`已在册`，而不是被链头那一格吞掉。
		return FormLoadAssignmentResult{
			outcome:   LoadAssignmentVersionExists,
			record:    existing,
			hasRecord: true,
		}, nil
	}
	// 本口只形成一次分配的首版。换一个版本号不是变化：变化要回指前版、沿用分配时刻（0009「assigned_at 在整条链上
	// 不变」），走领域 ReviseMembers 的入口；撤回之后「后续要装载只能是新的分配」。在这里落库就是在同一分配下另起一根链，
	// 还会占掉变化入口要用的版本号。
	head, formed, err := handler.deps.Assignments.FindChainHead(ctx, key.TenantID, key.Assignment)
	if err != nil {
		return loadAssignmentUndecided(command), nil
	}
	if formed {
		return loadAssignmentAlreadyFormed(head), nil
	}
	// 班次核对排在重放与链头之后：那两格答的是「这次分配是谁」，交回的是库里那一版，与这次送来的班次无关——同一版本
	// 重投带了别的班次照样是重放，已形成的分配照样该去变化或撤回的入口。先核班次会把它们改答成去登班次，而登完再投
	// 还是那两格。
	_, registered, err := handler.deps.Schedules.FindByKey(ctx, ports.ScheduleKey{
		TenantID: key.TenantID,
		Schedule: assignment.Schedule(),
	})
	if err != nil {
		return loadAssignmentUndecided(command), nil
	}
	if !registered {
		return FormLoadAssignmentResult{outcome: LoadAssignmentScheduleNotFound}, nil
	}
	// 容量保障排在班次在册之后：池挂在班次上，班次不在册就无池可判；排在重放与链头之后，理由同班次核对。
	if refused, decided := handler.assureCapacity(ctx, command, assignment); decided {
		return refused, nil
	}

	record := ports.LoadAssignmentRecord{
		Key:        key,
		Assignment: assignment,
		RecordedAt: handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Assignments.Save(ctx, record)
	if err != nil {
		return loadAssignmentUndecided(command), nil
	}
	if saved == ports.LoadAssignmentVersionAlreadyRegistered {
		// 并发下另一方先登记：读回赢家而不是宣称自己形成了它。撞上的可能是同一版本，也可能是同一分配别的版本号的
		// 首版（0021 的首版唯一索引）——后者按本次键读不回，要按分配读链头。
		winner, found, err := handler.deps.Assignments.FindByKey(ctx, key)
		if err != nil {
			return loadAssignmentUndecided(command), nil
		}
		if found {
			return FormLoadAssignmentResult{
				outcome:   LoadAssignmentVersionExists,
				record:    winner,
				hasRecord: true,
			}, nil
		}
		head, formed, err := handler.deps.Assignments.FindChainHead(ctx, key.TenantID, key.Assignment)
		if err != nil || !formed {
			return loadAssignmentUndecided(command), nil
		}
		return loadAssignmentAlreadyFormed(head), nil
	}
	return FormLoadAssignmentResult{outcome: LoadAssignmentFormed, record: record, hasRecord: true}, nil
}

// assureCapacity 判容量保障（ADR-0203 决定一、二）。班次只要挂了池，就要对每个池有一行、每行引用那个池上一份当前有效的
// 预占、分配量守恒；班次没挂池且没有分配行照旧形成。decided 为真时交回的就是答复，不登记。
//
// 次序：指错了的行 → 缺行的池 → 逐行（按池身份）判单位、有效区间与守恒。先挑指错的行：指向别的班次的那一行改对了，
// 缺的那个池多半也就有了；先答缺池，声明方补一行之后还得再撞一次指错。分配时点取分配时刻——业务时间由提交方给
// （ADR-0023），不取服务端时钟。
//
// 分配不调 Consume、不换存池：分配量随版本落库，预占上的分配关系读池时从各分配当前那一版派生。
func (handler *FormLoadAssignmentHandler) assureCapacity(
	ctx context.Context,
	command FormLoadAssignmentCommand,
	assignment domain.LoadAssignment,
) (FormLoadAssignmentResult, bool) {
	records, err := handler.deps.Pools.ListPoolsOnSchedules(ctx, assignment.TenantID(), []domain.ScheduleReference{assignment.Schedule()})
	if err != nil {
		return loadAssignmentUndecided(command), true
	}
	pools := make(map[domain.CapacityPoolReference]ports.CapacityPoolRecord, len(records))
	for _, record := range records {
		pools[record.Pool.Pool()] = record
	}

	allocations := assignment.Allocations()
	var misplaced []domain.CapacityAllocation
	covered := make(map[domain.CapacityPoolReference]bool, len(allocations))
	for _, allocation := range allocations {
		record, onSchedule := pools[allocation.Pool()]
		if onSchedule && record.Pool.Schedule() == assignment.Schedule() {
			if _, held := record.Pool.ReservationFor(allocation.Reservation()); held {
				covered[allocation.Pool()] = true
				continue
			}
		}
		misplaced = append(misplaced, allocation)
	}
	if len(misplaced) > 0 {
		return FormLoadAssignmentResult{outcome: LoadAssignmentReservationNotOnSchedule, misplaced: misplaced}, true
	}
	var uncovered []ports.CapacityPoolRecord
	for _, record := range records {
		if !covered[record.Pool.Pool()] {
			uncovered = append(uncovered, record)
		}
	}
	if len(uncovered) > 0 {
		return FormLoadAssignmentResult{outcome: LoadAssignmentCapacityReservationRequired, uncovered: uncovered}, true
	}

	for _, allocation := range allocations {
		record := pools[allocation.Pool()]
		err := record.Pool.AdmitAllocation(allocation, assignment.Assignment(), assignment.AssignedAt())
		if err == nil {
			continue
		}
		refused := FormLoadAssignmentResult{allocation: allocation, pool: record, hasPool: true}
		switch {
		case errors.Is(err, domain.ErrCapacityUnitMismatch):
			refused.outcome = LoadAssignmentUnitMismatch
		case errors.Is(err, domain.ErrReservationNotYetEffective):
			refused.outcome = LoadAssignmentReservationNotYetEffective
		case errors.Is(err, domain.ErrReservationExpired):
			refused.outcome = LoadAssignmentReservationExpired
		case errors.Is(err, domain.ErrAllocationExceeded):
			refused.outcome = LoadAssignmentAllocationExceeded
			refused.available, _ = record.Pool.AllocationHeadroom(allocation.Reservation(), assignment.Assignment())
		default:
			// 池与预占都已核过在册，剩下的只有构造门该挡住的坏值；按载荷不成立答，不落成未决让人重试。
			return FormLoadAssignmentResult{outcome: LoadAssignmentNotAccepted}, true
		}
		return refused, true
	}
	return FormLoadAssignmentResult{}, false
}

// loadAssignmentSpecFrom 逐件过构造器。任一件写坏就当场不受理，不留给领域构造门用一个笼统的
// 「分配不成立」回答——那一格是给「这一件确实缺了」用的。
func loadAssignmentSpecFrom(
	command FormLoadAssignmentCommand,
) (domain.LoadAssignmentSpec, ports.LoadAssignmentKey, bool) {
	none := ports.LoadAssignmentKey{}
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return domain.LoadAssignmentSpec{}, none, false
	}
	assignment, err := domain.NewLoadAssignmentReference(command.Assignment)
	if err != nil {
		return domain.LoadAssignmentSpec{}, none, false
	}
	schedule, err := domain.NewScheduleReference(command.Schedule)
	if err != nil {
		return domain.LoadAssignmentSpec{}, none, false
	}
	version, err := domain.NewLoadAssignmentVersion(command.Version)
	if err != nil {
		return domain.LoadAssignmentSpec{}, none, false
	}
	members := make([]domain.CarriedObjectReference, 0, len(command.Members))
	for _, raw := range command.Members {
		member, err := domain.NewCarriedObjectReference(raw)
		if err != nil {
			return domain.LoadAssignmentSpec{}, none, false
		}
		members = append(members, member)
	}
	allocations := make([]domain.CapacityAllocationDeclaration, 0, len(command.Allocations))
	for _, line := range command.Allocations {
		declaration, accepted := allocationDeclarationFrom(line)
		if !accepted {
			return domain.LoadAssignmentSpec{}, none, false
		}
		allocations = append(allocations, declaration)
	}
	return domain.LoadAssignmentSpec{
			TenantID:    command.TenantID,
			Assignment:  assignment,
			Schedule:    schedule,
			Members:     members,
			Version:     version,
			AssignedAt:  command.AssignedAt,
			Allocations: allocations,
		}, ports.LoadAssignmentKey{
			TenantID:   command.TenantID,
			Assignment: assignment,
			Version:    version,
		}, true
}

// allocationDeclarationFrom 把一行分配量的各格过构造器。数量成不成立（取成员数时须留零、声明时须为正）与成员数只许
// 落在件数维的池上由领域构造门判，这里不另立一份口径。
func allocationDeclarationFrom(line LoadAssignmentAllocationLine) (domain.CapacityAllocationDeclaration, bool) {
	pool, err := domain.NewCapacityPoolReference(line.Pool)
	if err != nil {
		return domain.CapacityAllocationDeclaration{}, false
	}
	reservation, err := domain.NewCapacityReservationReference(line.Reservation)
	if err != nil {
		return domain.CapacityAllocationDeclaration{}, false
	}
	unit, err := domain.NewQuantityUnitReference(line.Unit)
	if err != nil {
		return domain.CapacityAllocationDeclaration{}, false
	}
	kind, err := domain.ParseCapacityDemandSourceKind(line.SourceKind)
	if err != nil {
		return domain.CapacityAllocationDeclaration{}, false
	}
	reference, err := domain.NewCapacityDemandSourceReference(line.SourceReference)
	if err != nil {
		return domain.CapacityAllocationDeclaration{}, false
	}
	source, err := domain.NewCapacityDemandSource(kind, reference)
	if err != nil {
		return domain.CapacityAllocationDeclaration{}, false
	}
	return domain.CapacityAllocationDeclaration{
		Pool:        pool,
		Reservation: reservation,
		Quantity:    line.Quantity,
		FromMembers: line.FromMembers,
		Unit:        unit,
		Source:      source,
	}, true
}

func loadAssignmentAlreadyFormed(head ports.LoadAssignmentRecord) FormLoadAssignmentResult {
	return FormLoadAssignmentResult{outcome: LoadAssignmentAlreadyFormed, record: head, hasRecord: true}
}

func loadAssignmentUndecided(command FormLoadAssignmentCommand) FormLoadAssignmentResult {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		"LOAD_ASSIGNMENT_REGISTRY_UNAVAILABLE",
		command.TenantID.String(),
		command.Assignment,
		command.Version,
	}, "\x00")))
	return FormLoadAssignmentResult{
		outcome:      LoadAssignmentUndecided,
		continuation: "CONT-" + hex.EncodeToString(digest[:8]),
	}
}
