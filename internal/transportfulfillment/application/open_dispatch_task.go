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

// DispatchTaskOutcome 是建立一项揽派任务的结果代数。
//
// `已建立`与`已在册`分开：后者是同一任务被重投，交回原任务而不顶替——一次重投不该改写工作
// 范围。`输入未受理`与`未决`同样分开：前者重试一万次都是同一格，后者原样重试就可能过。
type DispatchTaskOutcome uint8

const (
	DispatchTaskOutcomeInvalid DispatchTaskOutcome = iota
	DispatchTaskOpened
	DispatchTaskAlreadyRegistered
	DispatchTaskNotAccepted
	DispatchTaskUndecided
)

func (outcome DispatchTaskOutcome) String() string {
	switch outcome {
	case DispatchTaskOpened:
		return "DISPATCH_TASK_OPENED"
	case DispatchTaskAlreadyRegistered:
		return "DISPATCH_TASK_ALREADY_OPEN"
	case DispatchTaskNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case DispatchTaskUndecided:
		return "DISPATCH_TASK_UNDECIDED"
	default:
		return ""
	}
}

// OpenDispatchTaskCommand 携带建立一项揽派任务的全部输入。
//
// 业务发生时间（OpenedAt）由调用方给，登记时刻另取时钟：任务是何时成立的，不由写库那一刻
// 决定。这两个时间分开是本上下文的通例，不是本用例的特例。
type OpenDispatchTaskCommand struct {
	TenantID   domain.TenantID
	Task       string
	Kind       domain.DispatchTaskKind
	Objects    []string
	Place      string
	WindowFrom time.Time
	WindowTo   time.Time
	Conditions string
	OpenedAt   time.Time
}

type OpenDispatchTaskResult struct {
	outcome      DispatchTaskOutcome
	record       ports.DispatchTaskRecord
	hasRecord    bool
	continuation string
}

func (result OpenDispatchTaskResult) Outcome() DispatchTaskOutcome {
	return result.outcome
}

func (result OpenDispatchTaskResult) Record() (ports.DispatchTaskRecord, bool) {
	return result.record, result.hasRecord
}

// ContinuationReference 只在`未决`时非空，供调用方续办同一次建立。
func (result OpenDispatchTaskResult) ContinuationReference() string {
	return result.continuation
}

type OpenDispatchTaskDeps struct {
	Tasks ports.DispatchTaskRegistry
	Clock ports.Clock
}

type OpenDispatchTaskHandler struct {
	deps OpenDispatchTaskDeps
}

func NewOpenDispatchTaskHandler(deps OpenDispatchTaskDeps) *OpenDispatchTaskHandler {
	return &OpenDispatchTaskHandler{deps: deps}
}

// Open 建立一项揽派任务：受理（工作范围七件由 OpenDispatchTask 构造门把门）→ 幂等按
// （租户 + 任务）分重放 → 提交。
//
// 任务表达需要完成什么，**不表达已经到场、取得控制或完成交付**——那些事实在履约尝试、场外
// 揽收与有效交付上，各自经任务引用回指本任务。所以这里没有任何到场或控制入参，领域类型上
// 也没有那些字段。
func (handler *OpenDispatchTaskHandler) Open(
	ctx context.Context,
	command OpenDispatchTaskCommand,
) (OpenDispatchTaskResult, error) {
	spec, taskRef, accepted := dispatchTaskSpecFrom(command)
	if !accepted {
		return OpenDispatchTaskResult{outcome: DispatchTaskNotAccepted}, nil
	}
	task, err := domain.OpenDispatchTask(spec)
	if err != nil {
		return OpenDispatchTaskResult{outcome: DispatchTaskNotAccepted}, nil
	}

	key := ports.DispatchTaskKey{TenantID: command.TenantID, Task: taskRef}
	existing, found, err := handler.deps.Tasks.FindByKey(ctx, key)
	if err != nil {
		return dispatchTaskUndecided(command.Task), nil
	}
	if found {
		// 同一任务被重投：交回原任务，不顶替。改工作范围不是重投能表达的事。
		return OpenDispatchTaskResult{
			outcome:   DispatchTaskAlreadyRegistered,
			record:    existing,
			hasRecord: true,
		}, nil
	}

	record := ports.DispatchTaskRecord{Key: key, Task: task, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Tasks.Save(ctx, record)
	if err != nil {
		return dispatchTaskUndecided(command.Task), nil
	}
	if saved == ports.DispatchTaskAlreadyOpen {
		// 并发下另一方先提交：读回赢家而不是宣称自己建立了它。
		winner, found, err := handler.deps.Tasks.FindByKey(ctx, key)
		if err != nil || !found {
			return dispatchTaskUndecided(command.Task), nil
		}
		return OpenDispatchTaskResult{
			outcome:   DispatchTaskAlreadyRegistered,
			record:    winner,
			hasRecord: true,
		}, nil
	}
	return OpenDispatchTaskResult{outcome: DispatchTaskOpened, record: record, hasRecord: true}, nil
}

// dispatchTaskSpecFrom 逐件过构造器。任一件写坏就当场不受理，不留给领域构造门用一个笼统的
// 「任务不成立」回答——那一格是给「这一件确实缺了」用的。
func dispatchTaskSpecFrom(command OpenDispatchTaskCommand) (domain.DispatchTaskSpec, domain.DispatchTaskReference, bool) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return domain.DispatchTaskSpec{}, domain.DispatchTaskReference{}, false
	}
	task, err := domain.NewDispatchTaskReference(command.Task)
	if err != nil {
		return domain.DispatchTaskSpec{}, domain.DispatchTaskReference{}, false
	}
	place, err := domain.NewAttemptPlaceReference(command.Place)
	if err != nil {
		return domain.DispatchTaskSpec{}, domain.DispatchTaskReference{}, false
	}
	conditions, err := domain.NewServiceConditionReference(command.Conditions)
	if err != nil {
		return domain.DispatchTaskSpec{}, domain.DispatchTaskReference{}, false
	}
	objects := make([]domain.CarriedObjectReference, 0, len(command.Objects))
	for _, raw := range command.Objects {
		object, err := domain.NewCarriedObjectReference(raw)
		if err != nil {
			return domain.DispatchTaskSpec{}, domain.DispatchTaskReference{}, false
		}
		objects = append(objects, object)
	}
	return domain.DispatchTaskSpec{
		TenantID:   command.TenantID,
		Task:       task,
		Kind:       command.Kind,
		Objects:    objects,
		Place:      place,
		WindowFrom: command.WindowFrom,
		WindowTo:   command.WindowTo,
		Conditions: conditions,
		OpenedAt:   command.OpenedAt,
	}, task, true
}

func dispatchTaskUndecided(task string) OpenDispatchTaskResult {
	digest := sha256.Sum256([]byte(strings.Join([]string{"DISPATCH_TASK_REGISTRY_UNAVAILABLE", task}, "\x00")))
	return OpenDispatchTaskResult{
		outcome:      DispatchTaskUndecided,
		continuation: "CONT-" + hex.EncodeToString(digest[:8]),
	}
}
