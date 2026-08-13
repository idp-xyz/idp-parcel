package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidDispatchTask = errors.New("transport fulfillment: invalid dispatch task")
	// ErrTaskClosed：已终止或已完成的任务不再改约、也不再关第二次——后续要揽派是
	// 新的任务。
	ErrTaskClosed = errors.New("transport fulfillment: the dispatch task is already closed")
)

// DispatchTaskKind 是揽派任务的封闭二值：场外揽收或末端派送。
type DispatchTaskKind uint8

const (
	DispatchTaskKindInvalid DispatchTaskKind = iota
	PickupDispatch
	DeliveryDispatch
)

func (kind DispatchTaskKind) valid() bool {
	return kind == PickupDispatch || kind == DeliveryDispatch
}

func (kind DispatchTaskKind) String() string {
	switch kind {
	case PickupDispatch:
		return "PICKUP"
	case DeliveryDispatch:
		return "DELIVERY"
	default:
		return ""
	}
}

// ServiceConditionReference 指名任务适用的服务条件（产品、合同与授权处置规则的快照）。
type ServiceConditionReference struct{ requiredValue }

func NewServiceConditionReference(value string) (ServiceConditionReference, error) {
	required, err := newRequiredValue("service condition reference", value)
	return ServiceConditionReference{required}, err
}

// TaskClosureBasisReference 指名任务终止或完成的依据。完成的依据是对象级结果——任务
// 汇总只能由对象结果派生，没有依据的关闭与「一次失败自动结束任务」分不开。
type TaskClosureBasisReference struct{ requiredValue }

func NewTaskClosureBasisReference(value string) (TaskClosureBasisReference, error) {
	required, err := newRequiredValue("task closure basis reference", value)
	return TaskClosureBasisReference{required}, err
}

// TaskState 是任务的封闭三态：开放、已终止、已完成。
type TaskState uint8

const (
	TaskStateInvalid TaskState = iota
	TaskOpen
	TaskTerminated
	TaskCompleted
)

func (state TaskState) String() string {
	switch state {
	case TaskOpen:
		return "OPEN"
	case TaskTerminated:
		return "TERMINATED"
	case TaskCompleted:
		return "COMPLETED"
	default:
		return ""
	}
}

// DispatchTaskSpec 是建立一项揽派任务所需的全部输入。
type DispatchTaskSpec struct {
	TenantID   TenantID
	Task       DispatchTaskReference
	Kind       DispatchTaskKind
	Objects    []CarriedObjectReference
	Place      AttemptPlaceReference
	WindowFrom time.Time
	WindowTo   time.Time
	Conditions ServiceConditionReference
	OpenedAt   time.Time
}

// DispatchTask 是在明确地点、时间范围和服务条件下对一个或多个载运对象执行场外揽收
// 或末端派送的工作范围（CONTEXT「揽派任务」）。
//
// 任务表达需要完成什么——它不等于已经到场、取得控制或完成交付：类型上没有任何到场、
// 控制或交付字段，那些在 FulfillmentAttempt、OffsitePickup 与 EffectiveDelivery 上，
// 经 DispatchTaskReference 回指本任务。一次失败尝试不自动结束任务（尝试对象钉了
// 「失败不造段」半边，这里钉「不自动关」半边）：终止与完成都必须显式带依据。
type DispatchTask struct {
	tenantID     TenantID
	task         DispatchTaskReference
	kind         DispatchTaskKind
	objects      []CarriedObjectReference
	place        AttemptPlaceReference
	windowFrom   time.Time
	windowTo     time.Time
	conditions   ServiceConditionReference
	openedAt     time.Time
	reschedules  int
	state        TaskState
	closureBasis TaskClosureBasisReference
	closedAt     time.Time
}

func OpenDispatchTask(spec DispatchTaskSpec) (DispatchTask, error) {
	if !spec.TenantID.valid() ||
		!spec.Task.valid() ||
		!spec.Kind.valid() ||
		len(spec.Objects) == 0 ||
		!spec.Place.valid() ||
		spec.WindowFrom.IsZero() ||
		spec.WindowTo.IsZero() ||
		!spec.WindowTo.After(spec.WindowFrom) ||
		!spec.Conditions.valid() ||
		spec.OpenedAt.IsZero() {
		return DispatchTask{}, ErrInvalidDispatchTask
	}
	seen := make(map[CarriedObjectReference]struct{}, len(spec.Objects))
	for _, object := range spec.Objects {
		if !object.valid() {
			return DispatchTask{}, ErrInvalidDispatchTask
		}
		if _, exists := seen[object]; exists {
			return DispatchTask{}, ErrInvalidDispatchTask
		}
		seen[object] = struct{}{}
	}
	return DispatchTask{
		tenantID:   spec.TenantID,
		task:       spec.Task,
		kind:       spec.Kind,
		objects:    append([]CarriedObjectReference(nil), spec.Objects...),
		place:      spec.Place,
		windowFrom: spec.WindowFrom.UTC(),
		windowTo:   spec.WindowTo.UTC(),
		conditions: spec.Conditions,
		openedAt:   spec.OpenedAt.UTC(),
		state:      TaskOpen,
	}, nil
}

func (task DispatchTask) TenantID() TenantID {
	return task.tenantID
}

func (task DispatchTask) Task() DispatchTaskReference {
	return task.task
}

func (task DispatchTask) Kind() DispatchTaskKind {
	return task.kind
}

func (task DispatchTask) Objects() []CarriedObjectReference {
	return append([]CarriedObjectReference(nil), task.objects...)
}

func (task DispatchTask) Place() AttemptPlaceReference {
	return task.place
}

func (task DispatchTask) Window() (time.Time, time.Time) {
	return task.windowFrom, task.windowTo
}

func (task DispatchTask) Conditions() ServiceConditionReference {
	return task.conditions
}

func (task DispatchTask) OpenedAt() time.Time {
	return task.openedAt
}

// Reschedules 报告改约次数——改约可多次，每次都不改任务身份。
func (task DispatchTask) Reschedules() int {
	return task.reschedules
}

func (task DispatchTask) State() TaskState {
	return task.state
}

// Closure 报告终止/完成的依据与时刻，只在已关闭任务上给出。
func (task DispatchTask) Closure() (TaskClosureBasisReference, time.Time, bool) {
	if task.closedAt.IsZero() {
		return TaskClosureBasisReference{}, time.Time{}, false
	}
	return task.closureBasis, task.closedAt, true
}

// Reschedule 更换时间窗口：任务身份不变（改约或重派形成的是新**尝试**，不是新任务），
// 可多次；已关闭任务改不了约。
func (task DispatchTask) Reschedule(windowFrom, windowTo, at time.Time) (DispatchTask, error) {
	if task.state != TaskOpen {
		return DispatchTask{}, ErrTaskClosed
	}
	if windowFrom.IsZero() || windowTo.IsZero() || !windowTo.After(windowFrom) ||
		at.IsZero() || at.Before(task.openedAt) {
		return DispatchTask{}, ErrInvalidDispatchTask
	}
	rescheduled := task
	rescheduled.objects = append([]CarriedObjectReference(nil), task.objects...)
	rescheduled.windowFrom = windowFrom.UTC()
	rescheduled.windowTo = windowTo.UTC()
	rescheduled.reschedules = task.reschedules + 1
	return rescheduled, nil
}

// Terminate 显式终止任务。依据必备——是否终止由适用产品、合同和授权处置规则决定，
// 一次失败尝试自动关任务正是被禁止的那条路。
func (task DispatchTask) Terminate(basis TaskClosureBasisReference, at time.Time) (DispatchTask, error) {
	return task.close(TaskTerminated, basis, at)
}

// Complete 显式完成任务。依据必备且指向对象级结果——任务汇总只能由对象结果派生，
// 完成不是从尝试状态自动长出来的。
func (task DispatchTask) Complete(basis TaskClosureBasisReference, at time.Time) (DispatchTask, error) {
	return task.close(TaskCompleted, basis, at)
}

func (task DispatchTask) close(state TaskState, basis TaskClosureBasisReference, at time.Time) (DispatchTask, error) {
	if task.state != TaskOpen {
		return DispatchTask{}, ErrTaskClosed
	}
	if !basis.valid() || at.IsZero() || at.Before(task.openedAt) {
		return DispatchTask{}, ErrInvalidDispatchTask
	}
	closed := task
	closed.objects = append([]CarriedObjectReference(nil), task.objects...)
	closed.state = state
	closed.closureBasis = basis
	closed.closedAt = at.UTC()
	return closed, nil
}
