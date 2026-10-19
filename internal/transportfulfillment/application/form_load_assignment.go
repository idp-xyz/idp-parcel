package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
type LoadAssignmentOutcome uint8

const (
	LoadAssignmentOutcomeInvalid LoadAssignmentOutcome = iota
	LoadAssignmentFormed
	LoadAssignmentVersionExists
	LoadAssignmentNotAccepted
	LoadAssignmentUndecided
	LoadAssignmentAlreadyFormed
	LoadAssignmentScheduleNotFound
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
	default:
		return ""
	}
}

// FormLoadAssignmentCommand 携带形成一次装载分配所需的全部输入。
//
// 版本由调用方指名而不是这里铸：它是分配链上的身份，变化与撤回各自带来下一个版本号，编排
// 铸号就等于替调用方决定「这算不算同一次分配的下一版」。
type FormLoadAssignmentCommand struct {
	TenantID   domain.TenantID
	Assignment string
	Schedule   string
	Members    []string
	Version    string
	AssignedAt time.Time
}

type FormLoadAssignmentResult struct {
	outcome      LoadAssignmentOutcome
	record       ports.LoadAssignmentRecord
	hasRecord    bool
	continuation string
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

type FormLoadAssignmentDeps struct {
	Assignments ports.LoadAssignmentRegistry
	Schedules   ports.TransportScheduleReader
	Clock       ports.Clock
}

type FormLoadAssignmentHandler struct {
	deps FormLoadAssignmentDeps
}

func NewFormLoadAssignmentHandler(deps FormLoadAssignmentDeps) *FormLoadAssignmentHandler {
	return &FormLoadAssignmentHandler{deps: deps}
}

// Form 形成一次装载分配的首版并登记：受理（对象范围等五件由 FormLoadAssignment 构造门把门）
// → 幂等按（租户 + 分配 + 版本）分重放 → 按（租户 + 分配）认出已形成的分配 → 按（租户 + 班次）核班次在册 → 登记。
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
	return domain.LoadAssignmentSpec{
			TenantID:   command.TenantID,
			Assignment: assignment,
			Schedule:   schedule,
			Members:    members,
			Version:    version,
			AssignedAt: command.AssignedAt,
		}, ports.LoadAssignmentKey{
			TenantID:   command.TenantID,
			Assignment: assignment,
			Version:    version,
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
