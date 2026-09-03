package domain_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

func rehydrateAssignmentSpec(t *testing.T) domain.RehydrateLoadAssignmentSpec {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	assignment, err := domain.NewLoadAssignmentReference("load-assignment-1")
	if err != nil {
		t.Fatalf("assignment: %v", err)
	}
	schedule, err := domain.NewScheduleReference("schedule-1")
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	version, err := domain.NewLoadAssignmentVersion("v1")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	member, err := domain.NewCarriedObjectReference("parcel-1")
	if err != nil {
		t.Fatalf("member: %v", err)
	}
	return domain.RehydrateLoadAssignmentSpec{
		TenantID:   tenant,
		Assignment: assignment,
		Schedule:   schedule,
		Members:    []domain.CarriedObjectReference{member},
		Version:    version,
		AssignedAt: time.Date(2026, 9, 13, 6, 0, 0, 0, time.UTC),
	}
}

func assignmentVersion(t *testing.T, value string) domain.LoadAssignmentVersion {
	t.Helper()
	version, err := domain.NewLoadAssignmentVersion(value)
	if err != nil {
		t.Fatalf("version %q: %v", value, err)
	}
	return version
}

// 首版装回来既没有前身也没有变化/撤回时刻——首版就是首版。
func TestRehydratingAFirstLoadAssignmentVersion(t *testing.T) {
	assignment, err := domain.RehydrateLoadAssignment(rehydrateAssignmentSpec(t))
	if err != nil {
		t.Fatalf("装回：%v", err)
	}
	if _, has := assignment.Corrects(); has {
		t.Fatal("首版带了前身")
	}
	if _, revised := assignment.RevisedAt(); revised {
		t.Fatal("首版带了变化时刻")
	}
	if _, withdrawn := assignment.Withdrawn(); withdrawn {
		t.Fatal("首版是已撤回的")
	}
}

// 链上耦合逐格：一个版本要么是变化要么是撤回；撤回两件成对；前身引用与「这是后继版本」互为
// 充要。**缺一半就读不出这一版是怎么来的**，所以拒绝而不是补一个占位。
func TestARehydratedLoadAssignmentVersionNeedsACoherentChain(t *testing.T) {
	base := rehydrateAssignmentSpec(t)
	later := base.AssignedAt.Add(2 * time.Hour)

	cases := map[string]func(*domain.RehydrateLoadAssignmentSpec){
		"同时是变化与撤回": func(spec *domain.RehydrateLoadAssignmentSpec) {
			spec.Corrects, spec.RevisedAt = assignmentVersion(t, "v0"), later
			spec.Withdrawn, spec.WithdrawnAt = true, later
		},
		"撤了却没有时刻": func(spec *domain.RehydrateLoadAssignmentSpec) {
			spec.Corrects, spec.Withdrawn = assignmentVersion(t, "v0"), true
		},
		"有撤回时刻却没撤": func(spec *domain.RehydrateLoadAssignmentSpec) {
			spec.Corrects, spec.WithdrawnAt = assignmentVersion(t, "v0"), later
		},
		"后继版本没有前身": func(spec *domain.RehydrateLoadAssignmentSpec) {
			spec.RevisedAt = later
		},
		"首版却回指前身": func(spec *domain.RehydrateLoadAssignmentSpec) {
			spec.Corrects = assignmentVersion(t, "v0")
		},
		"前身指向自己": func(spec *domain.RehydrateLoadAssignmentSpec) {
			spec.Corrects, spec.RevisedAt = spec.Version, later
		},
		"变化早于分配": func(spec *domain.RehydrateLoadAssignmentSpec) {
			spec.Corrects = assignmentVersion(t, "v0")
			spec.RevisedAt = spec.AssignedAt.Add(-time.Hour)
		},
		"撤回早于分配": func(spec *domain.RehydrateLoadAssignmentSpec) {
			spec.Corrects, spec.Withdrawn = assignmentVersion(t, "v0"), true
			spec.WithdrawnAt = spec.AssignedAt.Add(-time.Hour)
		},
		"空成员集": func(spec *domain.RehydrateLoadAssignmentSpec) {
			spec.Members = nil
		},
	}

	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			spec := rehydrateAssignmentSpec(t)
			breakIt(&spec)
			if _, err := domain.RehydrateLoadAssignment(spec); err == nil {
				t.Fatal("坏行被装了回来")
			}
		})
	}
}

// 已撤回的版本装回来仍然撤回着，且不再变化——装回不是重开。
func TestARehydratedWithdrawnAssignmentStaysWithdrawn(t *testing.T) {
	spec := rehydrateAssignmentSpec(t)
	spec.Version = assignmentVersion(t, "v2")
	spec.Corrects = assignmentVersion(t, "v1")
	spec.Withdrawn = true
	spec.WithdrawnAt = spec.AssignedAt.Add(3 * time.Hour)

	assignment, err := domain.RehydrateLoadAssignment(spec)
	if err != nil {
		t.Fatalf("装回：%v", err)
	}
	if _, withdrawn := assignment.Withdrawn(); !withdrawn {
		t.Fatal("已撤回的版本装回来却报不出撤回")
	}
	if _, err := assignment.ReviseMembers(
		spec.Members, assignmentVersion(t, "v3"), spec.WithdrawnAt.Add(time.Hour),
	); err == nil {
		t.Fatal("装回来的已撤回分配还能变化")
	}
}
