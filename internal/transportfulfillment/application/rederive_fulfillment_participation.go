package application

import (
	"context"
	"errors"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// rederiveFulfillmentParticipation 让一次来源更正在段上重派生该对象的参与关系（ADR-0112 决定二）：段由
// 登记册按对象反查，逐段交给领域的重派生门，替代版本经 `Supersede` 落库。交回段那一半的回答，形状与
// enterFulfillmentSegment 同一个——续办引用与拒绝格互斥，两格都空即这一半没有欠账也没被拒。
//
// 两条来源（揽收更正、交接更正）共用这一道门，与它们共用 enterFulfillmentSegment 同形：收寄与交接的差别
// 全部收在调用方交来的 rederive 函数里（各自的领域门），其余——要不要重派生、段在哪、失败算不算欠账——
// 两条来源逐字相同。
//
// **更正命令不带段号，段是派生知道的事。** 更正的输入是「证据说了什么」；被更正的对象可能早已离场，
// 所以问的是 FindSegmentsForObject 而不是 FindActiveSegments。反查到几个段是正常的（对象一程走几段），
// 哪一段的当前参与指着被更正的版本由领域门判：答无可替代的段跳过，命中一段即落笔并停——同一个来源版本
// 只会是一个段里一条参与的入场依据。全部跳过才是 NO_PARTICIPATION_TO_REDERIVE。
//
// **重派生是派生的一侧，它的失败不得回滚来源更正。** 与进段同一条纪律：登记册读不到或写不进只留续办引用；
// 领域正当拒绝里只有两格答出去（无可替代、撤回控制），其余（更正后的起点晚于继承的终点）照旧不出声——
// 那一格今天没有裁决，票 tf-segment-lifecycle-closure/10 如实记为未决。
//
// `segments` 缺席时不重派生也不算失败——派生一侧缺席不让来源保全停摆，判据同 enterFulfillmentSegment。
func rederiveFulfillmentParticipation(
	ctx context.Context,
	segments ports.ActualFulfillmentSegmentRegistry,
	clock ports.Clock,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
	rederive func(domain.ActualFulfillmentSegment) (domain.ActualFulfillmentSegment, error),
) segmentEntry {
	if segments == nil {
		return segmentEntry{}
	}
	keys, err := segments.FindSegmentsForObject(ctx, tenant, object)
	if err != nil {
		return segmentEntry{continuation: owed("SEGMENT_REGISTRY_UNAVAILABLE", tenant, "", object)}
	}
	for _, key := range keys {
		record, found, err := segments.FindByKey(ctx, key)
		if err != nil {
			return segmentEntry{continuation: owed("SEGMENT_REGISTRY_UNAVAILABLE", tenant, key.Segment.String(), object)}
		}
		if !found {
			continue
		}
		rederived, err := rederive(record.Segment)
		switch {
		case errors.Is(err, domain.ErrNoParticipationToRederive):
			continue
		case errors.Is(err, domain.ErrCorrectionWithdrawsControl):
			return segmentEntry{refusal: SegmentEntryRefusedCorrectionWithdrawsControl}
		case err != nil:
			return segmentEntry{}
		}
		participation, present := rederived.ParticipationFor(object)
		if !present {
			return segmentEntry{}
		}
		// `已替代`是业务答案不是欠账：同一更正重放、或另一方先把同一前版替代掉了，链尾都已在册。
		outcome, err := segments.Supersede(ctx, key, participation, clock.Now())
		if err != nil || outcome == ports.ParticipationSupersedeOutcomeInvalid {
			return segmentEntry{continuation: owed("PARTICIPATION_NOT_REDERIVED", tenant, key.Segment.String(), object)}
		}
		return segmentEntry{}
	}
	return segmentEntry{refusal: SegmentEntryRefusedNoParticipationToRederive}
}
