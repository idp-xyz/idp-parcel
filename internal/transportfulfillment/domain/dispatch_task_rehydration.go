package domain

import "time"

// RehydrateDispatchTaskSpec 是一项揽派任务连同它的对象范围在库面的样子。
//
// 它与 DispatchTaskSpec 分开而不是复用：构造门只接受**新建**的任务（状态必为开放、改约次数为
// 零、没有关闭三件），而库面装回来的可能是任何一个已经走过的状态。合成一个入参就得给构造门
// 加一堆只有装回时才有意义的字段，那等于让「新建」也能表达「已关闭」。
type RehydrateDispatchTaskSpec struct {
	TenantID   TenantID
	Task       DispatchTaskReference
	Kind       DispatchTaskKind
	Objects    []CarriedObjectReference
	Place      AttemptPlaceReference
	WindowFrom time.Time
	WindowTo   time.Time
	Conditions ServiceConditionReference
	OpenedAt   time.Time

	Reschedules  int
	State        TaskState
	ClosureBasis TaskClosureBasisReference
	ClosedAt     time.Time
}

// RehydrateDispatchTask 从库面重建一项揽派任务。
//
// 逐格完备性在这里复验一遍，坏行在这里暴露而不是流到判断里。**装回不是重开**：已关闭的任务
// 装回来仍然关着，改约与再次关闭照旧被领域挡住。
func RehydrateDispatchTask(spec RehydrateDispatchTaskSpec) (DispatchTask, error) {
	if !spec.TenantID.valid() ||
		!spec.Task.valid() ||
		!spec.Kind.valid() ||
		!spec.Place.valid() ||
		!spec.Conditions.valid() ||
		spec.WindowFrom.IsZero() ||
		spec.WindowTo.IsZero() ||
		!spec.WindowTo.After(spec.WindowFrom) ||
		spec.OpenedAt.IsZero() ||
		spec.Reschedules < 0 {
		return DispatchTask{}, ErrInvalidDispatchTask
	}
	// 空集不是「空任务」是坏行：任务由工作范围成立，没有对象的任务从来不曾成立过。
	if len(spec.Objects) == 0 {
		return DispatchTask{}, ErrInvalidDispatchTask
	}
	seen := make(map[CarriedObjectReference]struct{}, len(spec.Objects))
	objects := make([]CarriedObjectReference, 0, len(spec.Objects))
	for _, object := range spec.Objects {
		if !object.valid() {
			return DispatchTask{}, ErrInvalidDispatchTask
		}
		// 同一对象两条是行上就看得出的坏。这不是重放构造门——构造门判的是「此刻能不能建」，
		// 这里判的是「库里这几行本身立不立得住」。
		if _, exists := seen[object]; exists {
			return DispatchTask{}, ErrInvalidDispatchTask
		}
		seen[object] = struct{}{}
		objects = append(objects, object)
	}

	closed := spec.State == TaskTerminated || spec.State == TaskCompleted
	switch spec.State {
	case TaskOpen, TaskTerminated, TaskCompleted:
	default:
		return DispatchTask{}, ErrInvalidDispatchTask
	}
	// 关闭三件与状态成组。半截会装回一个「关了但说不出依据」的任务，而**没有依据的关闭与
	// 「一次失败尝试自动结束任务」在库里分不开**——后者正是 CONTEXT 明禁的那条。
	if closed != spec.ClosureBasis.valid() || closed != !spec.ClosedAt.IsZero() {
		return DispatchTask{}, ErrInvalidDispatchTask
	}
	if closed && spec.ClosedAt.Before(spec.OpenedAt) {
		return DispatchTask{}, ErrInvalidDispatchTask
	}

	task := DispatchTask{
		tenantID:    spec.TenantID,
		task:        spec.Task,
		kind:        spec.Kind,
		objects:     objects,
		place:       spec.Place,
		windowFrom:  spec.WindowFrom.UTC(),
		windowTo:    spec.WindowTo.UTC(),
		conditions:  spec.Conditions,
		openedAt:    spec.OpenedAt.UTC(),
		reschedules: spec.Reschedules,
		state:       spec.State,
	}
	if closed {
		task.closureBasis = spec.ClosureBasis
		task.closedAt = spec.ClosedAt.UTC()
	}
	return task, nil
}
