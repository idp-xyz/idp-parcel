package domain

import "time"

// RehydrateMovementFactSpec 是一条实际移动事实某一个版本在库面的样子。
//
// **没有 GateRequired。** 它是判断的输入而不是事实的属性：一条已经成立的出发要么带着放行依据、
// 要么本来就不受门禁管，两者的区别就是 GateClearance 在不在场。把它也装回来等于把判断过程当成
// 事实存着，而下一次规则变了那一格就开始说谎。
type RehydrateMovementFactSpec struct {
	TenantID      TenantID
	Fact          MovementFactReference
	Schedule      ScheduleReference
	Kind          MovementFactKind
	Location      MovementLocationReference
	Source        MovementSourceReference
	Version       MovementFactVersion
	OccurredAt    time.Time
	GateClearance GateClearanceReference

	Corrects    MovementFactVersion
	CorrectedAt time.Time
}

// RehydrateMovementFact 从库面重建一条实际移动事实的一个版本。
func RehydrateMovementFact(spec RehydrateMovementFactSpec) (TransportMovementFact, error) {
	// 本体那一半的判据与构造门同一套，复用它而不是抄一遍——抄一遍就是为同一形状立第二个口径，
	// 构造门改一次判据这里会悄悄漂移。GateRequired 传 false：装回时门禁那一问已经有答案了，
	// 答案就是 GateClearance 在不在场；再问一遍会把一条早已成立的出发判成「被门禁挡住」。
	fact, err := RecordMovementFact(MovementFactSpec{
		TenantID:      spec.TenantID,
		Fact:          spec.Fact,
		Schedule:      spec.Schedule,
		Kind:          spec.Kind,
		Location:      spec.Location,
		Source:        spec.Source,
		Version:       spec.Version,
		OccurredAt:    spec.OccurredAt,
		GateClearance: spec.GateClearance,
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
