package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

var (
	assignedAt     = time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	movementSeenAt = time.Date(2026, 8, 12, 14, 0, 0, 0, time.UTC)
)

func assignmentSpec(t *testing.T) domain.LoadAssignmentSpec {
	t.Helper()
	return domain.LoadAssignmentSpec{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Assignment: mustValue(t, domain.NewLoadAssignmentReference, "assignment-1"),
		Schedule:   mustValue(t, domain.NewScheduleReference, "schedule-1"),
		Members: []domain.CarriedObjectReference{
			mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
			mustValue(t, domain.NewCarriedObjectReference, "parcel-2"),
		},
		Version:    mustValue(t, domain.NewLoadAssignmentVersion, "assignment/v1"),
		AssignedAt: assignedAt,
	}
}

func movementSpec(t *testing.T, kind domain.MovementFactKind) domain.MovementFactSpec {
	t.Helper()
	return domain.MovementFactSpec{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Fact:       mustValue(t, domain.NewMovementFactReference, "movement-1"),
		Schedule:   mustValue(t, domain.NewScheduleReference, "schedule-1"),
		Kind:       kind,
		Location:   mustValue(t, domain.NewMovementLocationReference, "location-hub-1"),
		Source:     mustValue(t, domain.NewMovementSourceReference, "carrier-feed-1"),
		Version:    mustValue(t, domain.NewMovementFactVersion, "movement/v1"),
		OccurredAt: movementSeenAt,
	}
}

// Covers: CONTEXT「装载分配」词条与 208「装载分配形成、变化或撤回时保存版本和对象范围；
// 物理装载、短装、多装或错装作为独立事实参与比较，不修改分配历史」——分配是执行意图：
// 类型上没有已装载/控制/交接字段；变化与撤回都走新版本回指前身，原版本不动。
func TestALoadAssignmentIsIntentNotLoading(t *testing.T) {
	assignmentType := reflect.TypeOf(domain.LoadAssignment{})
	for index := 0; index < assignmentType.NumField(); index++ {
		name := strings.ToLower(assignmentType.Field(index).Name)
		for _, forbidden := range []string{"loaded", "control", "handover", "intake", "custody"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("LoadAssignment 携带 %q——分配就能被读成已装载或已控制", assignmentType.Field(index).Name)
			}
		}
	}

	assignment, err := domain.FormLoadAssignment(assignmentSpec(t))
	if err != nil {
		t.Fatalf("form assignment: %v", err)
	}
	if len(assignment.Members()) != 2 {
		t.Fatalf("members = %d, want 2", len(assignment.Members()))
	}

	revised, err := assignment.ReviseMembers(
		[]domain.CarriedObjectReference{mustValue(t, domain.NewCarriedObjectReference, "parcel-1")},
		mustValue(t, domain.NewLoadAssignmentVersion, "assignment/v2"),
		assignedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("revise members: %v", err)
	}
	predecessor, present := revised.Corrects()
	if !present || predecessor.String() != "assignment/v1" {
		t.Fatalf("corrects = %q present=%v, want v1", predecessor, present)
	}
	if len(assignment.Members()) != 2 || assignment.Version().String() != "assignment/v1" {
		t.Fatal("变化改写了原分配版本")
	}

	t.Run("reusing the original version is an overwrite and is refused", func(t *testing.T) {
		if _, err := assignment.ReviseMembers(
			assignment.Members(),
			assignment.Version(),
			assignedAt.Add(time.Hour),
		); !errors.Is(err, domain.ErrInvalidLoadAssignment) {
			t.Fatalf("error = %v; 沿用原版本号就是覆盖", err)
		}
	})

	t.Run("a withdrawn assignment no longer changes", func(t *testing.T) {
		withdrawn, err := revised.Withdraw(
			mustValue(t, domain.NewLoadAssignmentVersion, "assignment/v3"),
			assignedAt.Add(2*time.Hour),
		)
		if err != nil {
			t.Fatalf("withdraw: %v", err)
		}
		if _, ok := withdrawn.Withdrawn(); !ok {
			t.Fatal("撤回没有登记")
		}
		if _, err := withdrawn.ReviseMembers(
			withdrawn.Members(),
			mustValue(t, domain.NewLoadAssignmentVersion, "assignment/v4"),
			assignedAt.Add(3*time.Hour),
		); !errors.Is(err, domain.ErrLoadAssignmentWithdrawn) {
			t.Fatalf("revise error = %v, want ErrLoadAssignmentWithdrawn", err)
		}
		if _, err := withdrawn.Withdraw(
			mustValue(t, domain.NewLoadAssignmentVersion, "assignment/v5"),
			assignedAt.Add(3*time.Hour),
		); !errors.Is(err, domain.ErrLoadAssignmentWithdrawn) {
			t.Fatalf("withdraw error = %v, want ErrLoadAssignmentWithdrawn", err)
		}
	})

	broken := map[string]func(*domain.LoadAssignmentSpec){
		"no members":       func(spec *domain.LoadAssignmentSpec) { spec.Members = nil },
		"duplicate member": func(spec *domain.LoadAssignmentSpec) { spec.Members = append(spec.Members, spec.Members[0]) },
		"no version":       func(spec *domain.LoadAssignmentSpec) { spec.Version = domain.LoadAssignmentVersion{} },
		"no schedule":      func(spec *domain.LoadAssignmentSpec) { spec.Schedule = domain.ScheduleReference{} },
	}
	for name, breakSpec := range broken {
		t.Run(name, func(t *testing.T) {
			spec := assignmentSpec(t)
			breakSpec(&spec)
			if _, err := domain.FormLoadAssignment(spec); !errors.Is(err, domain.ErrInvalidLoadAssignment) {
				t.Fatalf("error = %v, want ErrInvalidLoadAssignment", err)
			}
		})
	}
}

// Covers: CONTEXT「出发、移动、到达……属于实际事实」与执行准备判断「不得共用一个可覆盖
// 状态」——事实记录不等于交接、不结束控制（类型上无控制/交接字段）；迟到与更正形成新
// 版本回指前身，原记录不改写。
func TestMovementFactsRecordWithoutEndingControl(t *testing.T) {
	factType := reflect.TypeOf(domain.TransportMovementFact{})
	for index := 0; index < factType.NumField(); index++ {
		name := strings.ToLower(factType.Field(index).Name)
		for _, forbidden := range []string{"control", "handover", "intake", "custody", "delivered"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("TransportMovementFact 携带 %q——移动事实就能被读成交接或控制变化", factType.Field(index).Name)
			}
		}
	}

	fact, err := domain.RecordMovementFact(movementSpec(t, domain.InTransitFact))
	if err != nil {
		t.Fatalf("record movement: %v", err)
	}
	if !fact.OccurredAt().Equal(movementSeenAt) {
		t.Fatalf("occurred at = %s", fact.OccurredAt())
	}

	corrected, err := fact.Correct(
		mustValue(t, domain.NewMovementLocationReference, "location-hub-2"),
		movementSeenAt.Add(-30*time.Minute),
		mustValue(t, domain.NewMovementSourceReference, "carrier-feed-2"),
		mustValue(t, domain.NewMovementFactVersion, "movement/v2"),
		movementSeenAt.Add(2*time.Hour),
	)
	if err != nil {
		t.Fatalf("correct movement: %v", err)
	}
	predecessor, present := corrected.Corrects()
	if !present || predecessor.String() != "movement/v1" {
		t.Fatalf("corrects = %q present=%v, want v1", predecessor, present)
	}
	if corrected.Location().String() != "location-hub-2" {
		t.Fatal("更正没有换上新位置")
	}
	if fact.Location().String() != "location-hub-1" || fact.Version().String() != "movement/v1" {
		t.Fatal("更正改写了原事实记录")
	}

	t.Run("reusing the original version is an overwrite and is refused", func(t *testing.T) {
		if _, err := fact.Correct(
			fact.Location(),
			fact.OccurredAt(),
			fact.Source(),
			fact.Version(),
			movementSeenAt.Add(time.Hour),
		); !errors.Is(err, domain.ErrInvalidMovementFact) {
			t.Fatalf("error = %v; 沿用原版本号就是覆盖", err)
		}
	})

	t.Run("the movement kind set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, kind := range []domain.MovementFactKind{
			domain.DepartureFact, domain.InTransitFact, domain.ArrivalFact,
		} {
			label := kind.String()
			if label == "" {
				t.Fatalf("kind %d has no label", kind)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 3 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if domain.MovementFactKind(len(labels)+1).String() != "" {
			t.Fatal("第四个事实取值带了标签——封闭集合被悄悄放开")
		}
	})
}

// Covers: CONTEXT「本上下文只对受明确监管门禁约束的装载出发……等动作执行限制」——受
// 门禁约束的出发没有放行依据立不成（可阻断），放行依据随事实保全；门禁只约束出发，
// 移动与到达挂放行依据被拒。
func TestAGatedDepartureIsBlockedWithoutClearance(t *testing.T) {
	spec := movementSpec(t, domain.DepartureFact)
	spec.GateRequired = true

	t.Run("without clearance the departure cannot form", func(t *testing.T) {
		if _, err := domain.RecordMovementFact(spec); !errors.Is(err, domain.ErrDepartureGateBlocked) {
			t.Fatalf("error = %v, want ErrDepartureGateBlocked", err)
		}
	})

	t.Run("with clearance the departure records its basis", func(t *testing.T) {
		cleared := spec
		cleared.GateClearance = mustValue(t, domain.NewGateClearanceReference, "GUARDED-ACTION/loading-departure-1")
		departure, err := domain.RecordMovementFact(cleared)
		if err != nil {
			t.Fatalf("record gated departure: %v", err)
		}
		clearance, present := departure.GateClearance()
		if !present || clearance.String() != "GUARDED-ACTION/loading-departure-1" {
			t.Fatal("放行依据没有随出发事实保全")
		}
	})

	t.Run("an ungated departure needs no clearance", func(t *testing.T) {
		if _, err := domain.RecordMovementFact(movementSpec(t, domain.DepartureFact)); err != nil {
			t.Fatalf("record ungated departure: %v", err)
		}
	})

	for name, kind := range map[string]domain.MovementFactKind{
		"in-transit": domain.InTransitFact,
		"arrival":    domain.ArrivalFact,
	} {
		t.Run(name+" cannot carry a gate", func(t *testing.T) {
			gated := movementSpec(t, kind)
			gated.GateRequired = true
			if _, err := domain.RecordMovementFact(gated); !errors.Is(err, domain.ErrInvalidMovementFact) {
				t.Fatalf("error = %v; 门禁只约束装载出发", err)
			}
			withClearance := movementSpec(t, kind)
			withClearance.GateClearance = mustValue(t, domain.NewGateClearanceReference, "GUARDED-ACTION/x")
			if _, err := domain.RecordMovementFact(withClearance); !errors.Is(err, domain.ErrInvalidMovementFact) {
				t.Fatalf("error = %v; 移动/到达挂上了放行依据", err)
			}
		})
	}
}
