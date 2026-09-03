package domain

import "time"

// RehydrateLoadAssignmentSpec 是装载分配某一个版本连同它的对象范围在库面的样子。
//
// 它与 LoadAssignmentSpec 分开而不是复用：构造门只接受**首版**（没有前身、既未变化也未撤回），
// 而库面装回来的可能是链上任何一版。合成一个入参就等于让「形成」也能表达「已撤回」。
type RehydrateLoadAssignmentSpec struct {
	TenantID   TenantID
	Assignment LoadAssignmentReference
	Schedule   ScheduleReference
	Members    []CarriedObjectReference
	Version    LoadAssignmentVersion
	AssignedAt time.Time

	Corrects    LoadAssignmentVersion
	RevisedAt   time.Time
	Withdrawn   bool
	WithdrawnAt time.Time
}

// RehydrateLoadAssignment 从库面重建装载分配的一个版本。
//
// **装回不是重开**：已撤回的版本装回来仍然是撤回的，再变化或再撤回照旧被领域挡住。
func RehydrateLoadAssignment(spec RehydrateLoadAssignmentSpec) (LoadAssignment, error) {
	// 首版那一半的判据与构造门同一套，复用它而不是抄一遍——抄一遍就是为同一形状立第二个
	// 口径，构造门改一次判据这里会悄悄漂移。
	assignment, err := FormLoadAssignment(LoadAssignmentSpec{
		TenantID:   spec.TenantID,
		Assignment: spec.Assignment,
		Schedule:   spec.Schedule,
		Members:    spec.Members,
		Version:    spec.Version,
		AssignedAt: spec.AssignedAt,
	})
	if err != nil {
		return LoadAssignment{}, err
	}

	revised := !spec.RevisedAt.IsZero()
	// 一个版本要么是变化要么是撤回，不能两者都是：领域两条路各自产出一版，没有哪条同时填。
	if revised && spec.Withdrawn {
		return LoadAssignment{}, ErrInvalidLoadAssignment
	}
	// 撤回两件成对；前版引用与「这是后继版本」互为充要——缺一半就读不出这一版是怎么来的。
	if spec.Withdrawn != !spec.WithdrawnAt.IsZero() {
		return LoadAssignment{}, ErrInvalidLoadAssignment
	}
	if spec.Corrects.valid() != (revised || spec.Withdrawn) {
		return LoadAssignment{}, ErrInvalidLoadAssignment
	}
	if spec.Corrects.valid() && spec.Corrects == spec.Version {
		return LoadAssignment{}, ErrInvalidLoadAssignment
	}
	if revised && spec.RevisedAt.Before(spec.AssignedAt) {
		return LoadAssignment{}, ErrInvalidLoadAssignment
	}
	if spec.Withdrawn && spec.WithdrawnAt.Before(spec.AssignedAt) {
		return LoadAssignment{}, ErrInvalidLoadAssignment
	}

	assignment.corrects = spec.Corrects
	if revised {
		assignment.revisedAt = spec.RevisedAt.UTC()
	}
	if spec.Withdrawn {
		assignment.withdrawn = true
		assignment.withdrawnAt = spec.WithdrawnAt.UTC()
	}
	return assignment, nil
}
