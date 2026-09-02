package domain

import "time"

// 实际履约段的重建门（ADR-0028）。构造与重建是两扇门：`EstablishSegmentWith*` 从零创生，
// 每条不变量当场算；从库里读回一个已经成立的段走不了它——那会重放「首个对象立段」的语义，
// 而库里那个段可能早已有多个成员、其中一部分已经离场。
//
// 本门**验形状与成对关系，不重走转换门**。同族先例把理由写得最清楚（`RehydrateCapacityPool`）：
// 转换门的依据是调用期的事实，不是行上的事实，重放它等于拿今天的输入去追认昨天的判断。
// 落到这里就是：不重算「这个对象当时该不该入场」「此刻该不该关段」，只核行上带回来的东西
// 自身立不立得住。
//
// 与之相对，**行间一致性该核**：一个「已关闭且仍有在场参与」的段是领域任何路径都产不出的
// 状态，收下它等于让重建门造出一个构造门造不出的聚合。它与「重放转换门」的分界是——本门只
// 拿行上已有的两个事实作比对，不引入任何调用期输入。

// RehydrateParticipationSpec 是一条参与关系在库面的样子。
//
// `Planned` 可缺席，且那是**有依据的缺席不是数据缺失**：明确允许待路由的产品在没有可行
// 候选时照样实际揽收，计划段此刻不存在。离场三件（种类、依据、时刻）必须同在或同缺——
// `Active()` 按 `EndedAt` 判，半截会重建出一个既非在场又非离场的参与。
type RehydrateParticipationSpec struct {
	Object     CarriedObjectReference
	Planned    PlannedSegmentReference
	EntryKind  ParticipationEntryKind
	EntryBasis ParticipationBasisReference
	EnteredAt  time.Time
	EndKind    ParticipationEndKind
	EndBasis   ParticipationBasisReference
	EndedAt    time.Time
}

// RehydrateActualFulfillmentSegmentSpec 是一个段连同它全部成员在库面的样子。
type RehydrateActualFulfillmentSegmentSpec struct {
	TenantID       TenantID
	Segment        FulfillmentSegmentReference
	Closed         bool
	ClosedAt       time.Time
	Participations []RehydrateParticipationSpec
}

// RehydrateActualFulfillmentSegment 从库面重建一个实际履约段。
func RehydrateActualFulfillmentSegment(spec RehydrateActualFulfillmentSegmentSpec) (ActualFulfillmentSegment, error) {
	if !spec.TenantID.valid() || !spec.Segment.valid() {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	// 空集不是「空段」是坏行：段由首个对象的控制事实成立，没有成员的段从来不曾成立过。
	if len(spec.Participations) == 0 {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}
	if spec.Closed != !spec.ClosedAt.IsZero() {
		return ActualFulfillmentSegment{}, ErrInvalidFulfillmentSegment
	}

	segment := ActualFulfillmentSegment{
		tenantID:       spec.TenantID,
		segment:        spec.Segment,
		closed:         spec.Closed,
		participations: make([]FulfillmentParticipation, 0, len(spec.Participations)),
	}
	if spec.Closed {
		segment.closedAt = spec.ClosedAt.UTC()
	}

	seen := make(map[CarriedObjectReference]struct{}, len(spec.Participations))
	for _, row := range spec.Participations {
		participation, err := rehydrateParticipation(row)
		if err != nil {
			return ActualFulfillmentSegment{}, err
		}
		// 同一对象两条参与是行上就看得出的坏。这不是重放 join——join 判的是「此刻能不能
		// 加进来」，这里判的是「带回来的这批彼此相容不相容」。
		if _, duplicate := seen[participation.object]; duplicate {
			return ActualFulfillmentSegment{}, ErrObjectAlreadyParticipating
		}
		seen[participation.object] = struct{}{}
		segment.participations = append(segment.participations, participation)
	}

	if segment.closed && segment.ActiveParticipations() > 0 {
		return ActualFulfillmentSegment{}, ErrSegmentStillActive
	}
	return segment, nil
}

func rehydrateParticipation(row RehydrateParticipationSpec) (FulfillmentParticipation, error) {
	if !row.Object.valid() || !row.EntryKind.valid() || !row.EntryBasis.valid() || row.EnteredAt.IsZero() {
		return FulfillmentParticipation{}, ErrInvalidFulfillmentSegment
	}

	ended := !row.EndedAt.IsZero()
	if ended != row.EndKind.valid() || ended != row.EndBasis.valid() {
		return FulfillmentParticipation{}, ErrInvalidFulfillmentSegment
	}
	if ended && row.EndedAt.Before(row.EnteredAt) {
		return FulfillmentParticipation{}, ErrInvalidFulfillmentSegment
	}

	participation := FulfillmentParticipation{
		object:     row.Object,
		planned:    row.Planned,
		entryKind:  row.EntryKind,
		entryBasis: row.EntryBasis,
		enteredAt:  row.EnteredAt.UTC(),
	}
	if ended {
		participation.endKind = row.EndKind
		participation.endBasis = row.EndBasis
		participation.endedAt = row.EndedAt.UTC()
	}
	return participation, nil
}

func (kind ParticipationEntryKind) valid() bool {
	return kind == EnteredByOffsitePickup || kind == EnteredByTransportHandover
}

func (kind ParticipationEndKind) valid() bool {
	return kind == EndedByEffectiveDelivery ||
		kind == EndedByNextHandover ||
		kind == EndedByControlTermination
}
