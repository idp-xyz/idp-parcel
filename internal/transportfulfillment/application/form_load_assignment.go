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
type LoadAssignmentOutcome uint8

const (
	LoadAssignmentOutcomeInvalid LoadAssignmentOutcome = iota
	LoadAssignmentFormed
	LoadAssignmentVersionExists
	LoadAssignmentNotAccepted
	LoadAssignmentUndecided
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
	Clock       ports.Clock
}

type FormLoadAssignmentHandler struct {
	deps FormLoadAssignmentDeps
}

func NewFormLoadAssignmentHandler(deps FormLoadAssignmentDeps) *FormLoadAssignmentHandler {
	return &FormLoadAssignmentHandler{deps: deps}
}

// Form 形成一次装载分配并登记该版本：受理（对象范围等五件由 FormLoadAssignment 构造门把门）
// → 幂等按（租户 + 分配 + 版本）分重放 → 登记。
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
		// 同一版本重投：交回原版本，不顶替。换对象范围要换版本号，那是变化不是重投。
		return FormLoadAssignmentResult{
			outcome:   LoadAssignmentVersionExists,
			record:    existing,
			hasRecord: true,
		}, nil
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
		// 并发下另一方先登记：读回赢家而不是宣称自己形成了它。
		winner, found, err := handler.deps.Assignments.FindByKey(ctx, key)
		if err != nil || !found {
			return loadAssignmentUndecided(command), nil
		}
		return FormLoadAssignmentResult{
			outcome:   LoadAssignmentVersionExists,
			record:    winner,
			hasRecord: true,
		}, nil
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
