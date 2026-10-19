package domain

import "time"

// RehydrateMovementFactSpec 是一条实际移动事实某一个版本在库面的样子。
//
// 设备自报的「要不要门禁」不在这里：它不是事实的属性，也不决定是否设门（ADR-0205 决定三）。出发要装回的是登记那一刻
// 问来的门禁读数，读数由它自己的重建门装好再交进来。
type RehydrateMovementFactSpec struct {
	TenantID      TenantID
	Fact          MovementFactReference
	Schedule      ScheduleReference
	Kind          MovementFactKind
	Location      MovementLocationReference
	Source        MovementSourceReference
	Version       MovementFactVersion
	OccurredAt    time.Time
	Segment       FulfillmentSegmentReference
	Resource      TransportResourceReference
	GateClearance GateClearanceReference
	GateReading   DepartureGateReading

	Corrects    MovementFactVersion
	CorrectedAt time.Time
}

// RehydrateMovementFact 从库面重建一条实际移动事实的一个版本。
func RehydrateMovementFact(spec RehydrateMovementFactSpec) (TransportMovementFact, error) {
	// 本体那一半的判据与构造门同一套，复用它而不是抄一遍——抄一遍就是为同一形状立第二个口径，
	// 构造门改一次判据这里会悄悄漂移。
	fact, err := RecordMovementFact(MovementFactSpec{
		TenantID:      spec.TenantID,
		Fact:          spec.Fact,
		Schedule:      spec.Schedule,
		Kind:          spec.Kind,
		Location:      spec.Location,
		Source:        spec.Source,
		Version:       spec.Version,
		OccurredAt:    spec.OccurredAt,
		Segment:       spec.Segment,
		Resource:      spec.Resource,
		GateClearance: spec.GateClearance,
		GateReading:   spec.GateReading,
	})
	if err != nil {
		return TransportMovementFact{}, err
	}

	corrected := !spec.CorrectedAt.IsZero()
	// 更正两件成对；前版引用指向自己是一条读不动的链；更正的是对同一件已发生的事的记述，
	// 不是把它挪到更早。
	if spec.Corrects.valid() != corrected {
		return TransportMovementFact{}, ErrInvalidMovementFact
	}
	if corrected {
		if spec.Corrects == spec.Version {
			return TransportMovementFact{}, ErrInvalidMovementFact
		}
		if spec.CorrectedAt.Before(spec.OccurredAt) {
			return TransportMovementFact{}, ErrInvalidMovementFact
		}
		fact.corrects = spec.Corrects
		fact.correctedAt = spec.CorrectedAt.UTC()
	}
	return fact, nil
}
