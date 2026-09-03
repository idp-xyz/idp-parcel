package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证实际履约段的重建门（票 tf-unwired-seven/01，ADR-0028）：它验形状与成对关系，
// **不重走转换门**——不重放 JoinWith*、不重算「谁该在什么时点入场」。同族先例的理由见
// RehydrateCapacityPool 的注释：那些是转换门，依据是调用期的事实而不是行上的事实。

var (
	segmentEnteredAt = time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	segmentEndedAt   = time.Date(2026, 8, 13, 18, 0, 0, 0, time.UTC)
	segmentClosedAt  = time.Date(2026, 8, 13, 19, 0, 0, 0, time.UTC)
)

func segmentValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

// activeParticipationSpec 是一条在场的参与关系：入场三件齐、离场三件全缺。
func activeParticipationSpec(t *testing.T, object string) domain.RehydrateParticipationSpec {
	t.Helper()
	return domain.RehydrateParticipationSpec{
		Object:     segmentValue(t, domain.NewCarriedObjectReference, object),
		Planned:    segmentValue(t, domain.NewPlannedSegmentReference, "planned-"+object),
		EntryKind:  domain.EnteredByOffsitePickup,
		EntryBasis: segmentValue(t, domain.NewParticipationBasisReference, "OFFSITE-PICKUP/v1-"+object),
		EnteredAt:  segmentEnteredAt,
	}
}

func endedParticipationSpec(t *testing.T, object string) domain.RehydrateParticipationSpec {
	t.Helper()
	spec := activeParticipationSpec(t, object)
	spec.EndKind = domain.EndedByEffectiveDelivery
	spec.EndBasis = segmentValue(t, domain.NewParticipationBasisReference, "EFFECTIVE-DELIVERY/v1-"+object)
	spec.EndedAt = segmentEndedAt
	return spec
}

func segmentSpec(t *testing.T, participations ...domain.RehydrateParticipationSpec) domain.RehydrateActualFulfillmentSegmentSpec {
	t.Helper()
	return domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID:       segmentValue(t, domain.NewTenantID, "tenant-1"),
		Segment:        segmentValue(t, domain.NewFulfillmentSegmentReference, "segment-1"),
		Participations: participations,
	}
}

// Covers: CONTEXT「共享实际履约段中的每个载运对象分别成立、结束和更正，不能由整段结果
// 覆盖成员差异」——重建必须逐条把成员差异原样带回来，一个在场一个已离场并存。
func TestRehydratedSegmentKeepsEachMemberSeparate(t *testing.T) {
	rebuilt, err := domain.RehydrateActualFulfillmentSegment(segmentSpec(t,
		activeParticipationSpec(t, "parcel-1"),
		endedParticipationSpec(t, "parcel-2"),
	))
	if err != nil {
		t.Fatalf("重建：%v", err)
	}
	if !rebuilt.Established() || rebuilt.Closed() {
		t.Fatalf("established=%v closed=%v", rebuilt.Established(), rebuilt.Closed())
	}
	if rebuilt.ActiveParticipations() != 1 {
		t.Fatalf("在场参与 = %d, want 1", rebuilt.ActiveParticipations())
	}

	active, found := rebuilt.ParticipationFor(segmentValue(t, domain.NewCarriedObjectReference, "parcel-1"))
	if !found || !active.Active() {
		t.Fatal("parcel-1 应当仍在场")
	}
	if active.EntryKind() != domain.EnteredByOffsitePickup {
		t.Fatalf("入场种类 = %q", active.EntryKind())
	}
	if planned, has := active.PlannedSegment(); !has || planned.String() != "planned-parcel-1" {
		t.Fatalf("计划段没带回来：has=%v", has)
	}

	ended, found := rebuilt.ParticipationFor(segmentValue(t, domain.NewCarriedObjectReference, "parcel-2"))
	if !found || ended.Active() {
		t.Fatal("parcel-2 应当已离场")
	}
	kind, basis, at, done := ended.End()
	if !done || kind != domain.EndedByEffectiveDelivery || basis.String() == "" || !at.Equal(segmentEndedAt) {
		t.Fatalf("离场三件没带回来：done=%v kind=%q at=%v", done, kind, at)
	}
}

// Covers: 领域「计划履约段可缺席——明确允许待路由的产品在没有可行候选时照样实际揽收」。
// 缺席是有依据的缺席，重建必须收下它而不是当成坏行。
func TestRehydrationAcceptsAParticipationWithoutAPlannedSegment(t *testing.T) {
	spec := activeParticipationSpec(t, "parcel-1")
	spec.Planned = domain.PlannedSegmentReference{}

	rebuilt, err := domain.RehydrateActualFulfillmentSegment(segmentSpec(t, spec))
	if err != nil {
		t.Fatalf("无计划段的参与应当收得下：%v", err)
	}
	participation, _ := rebuilt.ParticipationFor(segmentValue(t, domain.NewCarriedObjectReference, "parcel-1"))
	if _, has := participation.PlannedSegment(); has {
		t.Fatal("凭空长出了一个计划段")
	}
}

// Covers: 离场三件同在或同缺。Active() 按 endedAt 判，库面半截会重建出一个既非在场
// 又非离场的参与——那种东西领域自己造不出来。
func TestRehydrationRefusesAHalfEndedParticipation(t *testing.T) {
	full := endedParticipationSpec(t, "parcel-1")
	for name, mutate := range map[string]func(*domain.RehydrateParticipationSpec){
		"只有种类": func(s *domain.RehydrateParticipationSpec) {
			s.EndBasis, s.EndedAt = domain.ParticipationBasisReference{}, time.Time{}
		},
		"只有依据": func(s *domain.RehydrateParticipationSpec) {
			s.EndKind, s.EndedAt = domain.ParticipationEndKindInvalid, time.Time{}
		},
		"只有时刻": func(s *domain.RehydrateParticipationSpec) {
			s.EndKind, s.EndBasis = domain.ParticipationEndKindInvalid, domain.ParticipationBasisReference{}
		},
		"缺种类": func(s *domain.RehydrateParticipationSpec) { s.EndKind = domain.ParticipationEndKindInvalid },
		"缺依据": func(s *domain.RehydrateParticipationSpec) { s.EndBasis = domain.ParticipationBasisReference{} },
		"缺时刻": func(s *domain.RehydrateParticipationSpec) { s.EndedAt = time.Time{} },
	} {
		t.Run(name, func(t *testing.T) {
			broken := full
			mutate(&broken)
			if _, err := domain.RehydrateActualFulfillmentSegment(segmentSpec(t, broken)); err == nil {
				t.Fatal("半截的离场被收下了")
			}
		})
	}
}

// Covers: 入场三件必备，且入场种类是封闭二值——领域注释「刻意没有第三格：扫描、装载、
// 订舱确认都立不起参与」。重建不得从集外取值。
func TestRehydrationRefusesAnIncompleteOrOutOfSetEntry(t *testing.T) {
	for name, mutate := range map[string]func(*domain.RehydrateParticipationSpec){
		"缺入场种类": func(s *domain.RehydrateParticipationSpec) { s.EntryKind = domain.ParticipationEntryKindInvalid },
		"集外入场种类": func(s *domain.RehydrateParticipationSpec) {
			s.EntryKind = domain.ParticipationEntryKind(99)
		},
		"缺入场依据": func(s *domain.RehydrateParticipationSpec) { s.EntryBasis = domain.ParticipationBasisReference{} },
		"缺入场时刻": func(s *domain.RehydrateParticipationSpec) { s.EnteredAt = time.Time{} },
		"缺对象":   func(s *domain.RehydrateParticipationSpec) { s.Object = domain.CarriedObjectReference{} },
	} {
		t.Run(name, func(t *testing.T) {
			broken := activeParticipationSpec(t, "parcel-1")
			mutate(&broken)
			if _, err := domain.RehydrateActualFulfillmentSegment(segmentSpec(t, broken)); err == nil {
				t.Fatalf("%s 被收下了", name)
			}
		})
	}
}

// Covers: 领域 join「同一实际控制范围不能因伙伴重投、任务重建或批量重试重复建立履约
// 参与」。重建门不重放 join，但同一对象两条参与是行上就看得出的坏，必须拒。
func TestRehydrationRefusesTwoParticipationsForTheSameObject(t *testing.T) {
	if _, err := domain.RehydrateActualFulfillmentSegment(segmentSpec(t,
		activeParticipationSpec(t, "parcel-1"),
		endedParticipationSpec(t, "parcel-1"),
	)); !errors.Is(err, domain.ErrObjectAlreadyParticipating) {
		t.Fatalf("err = %v, want ErrObjectAlreadyParticipating", err)
	}
}

// Covers: 段由首个对象的控制事实成立（CONTEXT 生命周期①）。一个没有任何参与关系的段
// 从来不曾成立过，它不是「空段」而是坏行。
func TestRehydrationRefusesASegmentWithNoParticipation(t *testing.T) {
	if _, err := domain.RehydrateActualFulfillmentSegment(segmentSpec(t)); !errors.Is(err, domain.ErrInvalidFulfillmentSegment) {
		t.Fatalf("err = %v, want ErrInvalidFulfillmentSegment", err)
	}
}

// Covers: closed 与 closedAt 成对。半截的关闭同样是领域造不出来的状态。
func TestRehydrationRequiresClosedAndClosedAtTogether(t *testing.T) {
	base := segmentSpec(t, endedParticipationSpec(t, "parcel-1"))

	onlyFlag := base
	onlyFlag.Closed = true
	if _, err := domain.RehydrateActualFulfillmentSegment(onlyFlag); err == nil {
		t.Fatal("只有 closed 标记、没有关闭时刻，被收下了")
	}

	onlyTime := base
	onlyTime.ClosedAt = segmentClosedAt
	if _, err := domain.RehydrateActualFulfillmentSegment(onlyTime); err == nil {
		t.Fatal("只有关闭时刻、没有 closed 标记，被收下了")
	}

	both := base
	both.Closed, both.ClosedAt = true, segmentClosedAt
	rebuilt, err := domain.RehydrateActualFulfillmentSegment(both)
	if err != nil {
		t.Fatalf("成对的关闭应当收得下：%v", err)
	}
	if at, closed := rebuilt.ClosedAt(); !closed || !at.Equal(segmentClosedAt) {
		t.Fatalf("关闭时刻没带回来：closed=%v at=%v", closed, at)
	}
}

// Covers: CONTEXT「一个仍在控制中的对象足以让段继续存在」。
//
// 这一条是**行间的一致性核对，不是重放 CloseSegment**：一个「已关闭且仍有在场参与」的段
// 是领域任何路径都产不出的状态，收下它等于让重建门造出一个构造门造不出的聚合。区别在于
// 本条只读行上已有的两个事实作比对，不重算「此刻该不该关」。
func TestRehydrationRefusesAClosedSegmentThatStillHasActiveParticipations(t *testing.T) {
	spec := segmentSpec(t, activeParticipationSpec(t, "parcel-1"))
	spec.Closed, spec.ClosedAt = true, segmentClosedAt

	if _, err := domain.RehydrateActualFulfillmentSegment(spec); !errors.Is(err, domain.ErrSegmentStillActive) {
		t.Fatalf("err = %v, want ErrSegmentStillActive", err)
	}
}

// Covers: 段的关闭不得早于任何成员的离场。与「已关闭且仍有在场参与」同族：一个在某成员仍受控时
// 就已结束的段，领域任何路径都产不出。同样只比对行上已有的两个时刻，不重放 CloseSegment。
func TestRehydrationRefusesAClosureBeforeAMemberEnded(t *testing.T) {
	spec := segmentSpec(t, endedParticipationSpec(t, "parcel-1"), endedParticipationSpec(t, "parcel-2"))
	spec.Closed, spec.ClosedAt = true, segmentEndedAt.Add(-time.Minute)

	if _, err := domain.RehydrateActualFulfillmentSegment(spec); !errors.Is(err, domain.ErrInvalidFulfillmentSegment) {
		t.Fatalf("err = %v, want ErrInvalidFulfillmentSegment（关闭早于成员离场）", err)
	}

	t.Run("closing at the very moment the last member left is consistent", func(t *testing.T) {
		sameMoment := spec
		sameMoment.ClosedAt = segmentEndedAt
		if _, err := domain.RehydrateActualFulfillmentSegment(sameMoment); err != nil {
			t.Fatalf("与最后一次离场同刻的关闭应当收得下：%v", err)
		}
	})
}

// Covers: 离场不得早于入场——同一行上的两个时刻，行内就比得出来。
func TestRehydrationRefusesAnEndBeforeItsEntry(t *testing.T) {
	broken := endedParticipationSpec(t, "parcel-1")
	broken.EndedAt = segmentEnteredAt.Add(-time.Hour)

	if _, err := domain.RehydrateActualFulfillmentSegment(segmentSpec(t, broken)); err == nil {
		t.Fatal("离场早于入场的参与被收下了")
	}
}

// Covers: ADR-0028「重建不重算判断路径」——重建门交回的段仍是可继续演进的聚合，
// 后续转换走原有转换门。这一条同时钉住重建产物没有被做成一个只读快照。
func TestARehydratedSegmentStillAcceptsItsTransitionDoors(t *testing.T) {
	rebuilt, err := domain.RehydrateActualFulfillmentSegment(segmentSpec(t, activeParticipationSpec(t, "parcel-1")))
	if err != nil {
		t.Fatalf("重建：%v", err)
	}

	object := segmentValue(t, domain.NewCarriedObjectReference, "parcel-1")
	basis := segmentValue(t, domain.NewParticipationBasisReference, "CONTROL-TERMINATION/case-1")
	ended, err := rebuilt.EndParticipationWithTermination(object, basis, segmentEndedAt)
	if err != nil {
		t.Fatalf("重建出的段应当还能走转换门：%v", err)
	}
	closed, err := ended.CloseSegment(segmentClosedAt)
	if err != nil {
		t.Fatalf("全部参与已结束后应当关得上：%v", err)
	}
	if !closed.Closed() {
		t.Fatal("段没有关上")
	}
}
