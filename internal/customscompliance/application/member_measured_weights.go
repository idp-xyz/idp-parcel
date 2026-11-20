package application

import (
	"context"
	"sort"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// loadMemberMeasuredWeights 按成员读当前有效实测，供首次提交与原案内更正共用。
// 有则带上毛重、来源身份与发生时间；没有则这件不带——客户声明的重量在资料快照那一侧，这里不退回去。
// 定不下不挑一条，读不动也不当成没有：提交版本不可覆盖，固定了就改不回。
// 内容指纹在调用前已经按调用方交来的内容定了，这里读到的毛重不回写进指纹，复测之后的重放仍是同一份内容。
func loadMemberMeasuredWeights(
	ctx context.Context,
	view ports.MemberMeasuredWeightView,
	tenant domain.TenantID,
	unit domain.DeclarationUnit,
) ([]domain.MemberMeasuredWeight, SubmitDeclarationResult, bool) {
	members := unit.Members()
	sort.Slice(members, func(i, j int) bool { return members[i].String() < members[j].String() })
	weights := make([]domain.MemberMeasuredWeight, 0)
	for _, member := range members {
		reading, err := view.LoadMemberMeasuredWeight(ctx, tenant, member)
		if err != nil {
			return nil, measurementUndecided(MemberMeasurementUnavailable, unit.ID().String(), member.String(), ""), true
		}
		switch reading.Answer {
		case ports.MeasuredWeightNone:
			continue
		case ports.MeasuredWeightUndetermined:
			return nil, measurementUndecided(MemberMeasurementUndetermined, unit.ID().String(), member.String(), reading.Cause), true
		case ports.MeasuredWeightOne:
			weight, err := domain.NewMemberMeasuredWeight(
				member, reading.GrossWeight, reading.GrossWeightUnit, reading.MeasurementSource, reading.MeasuredAt,
			)
			if err != nil {
				// 读口交回的一条缺了数值、单位、来源或时刻，不能当成称到了，也不能挑别的。
				return nil, measurementUndecided(MemberMeasurementUnavailable, unit.ID().String(), member.String(), ""), true
			}
			weights = append(weights, weight)
		default:
			return nil, measurementUndecided(MemberMeasurementUnavailable, unit.ID().String(), member.String(), ""), true
		}
	}
	return weights, SubmitDeclarationResult{}, false
}

func measurementUndecided(reason DeclarationUndecidedReason, unitID, parcel, cause string) SubmitDeclarationResult {
	parts := []string{reason.String(), unitID, parcel}
	if cause != "" {
		parts = append(parts, cause)
	}
	return SubmitDeclarationResult{
		outcome:      DeclarationUndecided,
		reason:       reason,
		continuation: declarationContinuation(parts...),
	}
}
