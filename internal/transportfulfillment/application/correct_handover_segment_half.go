package application

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// correctionSegmentHalf 是交接更正落库之后同一笔里的段那一半，交回结束前段参与那一步的答案（只在改走进段时非零）与段那一半
// 的回答。error 只来自结束前段参与那一步：那一步失败整笔不落，理由同首登。
//
// 先问重派生：凭前版入场的参与（含链尾失效的那一条）在原段上长下一版，ADR-0112 那一路一字不改。只有三件同时成立才改走
// 进段（ADR-0307 决定一）：本版转出控制、前版没有转出控制、没有一条参与凭前版入场。那时前版从没让对象进过段，也没结束过
// 对象此前的参与——控制直到这一版才转入接收方（CONTEXT「运输交接与实物控制」），这一版就是对象的这一次交接，段那一半与
// 首登即为`已交接`逐格相同：结束此刻在场的参与、进这一版承继来的段声明。只走重派生的话，对象既不离开先前的段、也进不了
// 新段，派送、追踪与实际履约三路随交接信封各自认错段。
//
// 前版待确认还是拒收不分路：两格都保持此前已确认的控制关系，在段与参与上留下的东西也一样——都没有，或都是一版失效参与。
// 前版没转出控制而重派生找得到参与，是链尾失效的那一种：对象曾凭更早一版`已交接`进段、又被更正撤回。它照 ADR-0112
// 决定四从失效版本长出替代版本、回到原段，不进声明的段，也不结束任何参与——撤回那一次没有把先前段的参与还回去（不回退
// 到更早的控制版本），这一次也就无从结束。
//
// 段登记册缺席时重派生答两格皆空，两路都不走：判不出有没有可重派生的参与，也就说不出这一版是不是对象的又一次交接。
func (handler *RegisterTransportHandoverHandler) correctionSegmentHalf(
	ctx context.Context,
	predecessor ports.TransportHandoverRecord,
	corrected domain.TransportHandover,
) (ParticipationEndOutcome, segmentEntry, error) {
	entry := handler.rederiveParticipation(ctx, corrected.TenantID(), corrected)
	if entry.refusal != SegmentEntryRefusedNoParticipationToRederive ||
		!corrected.TransfersControl() || predecessor.Handover.TransfersControl() {
		return ParticipationEndOutcomeInvalid, entry, nil
	}
	if later := laterControlFacts(ctx, handler.deps.Segments, corrected); later != (segmentEntry{}) {
		return ParticipationEndOutcomeInvalid, later, nil
	}
	return handler.handOverIntoSegment(ctx, predecessor.SegmentDeclaration, corrected)
}

// laterControlFacts 答对象的参与链上有没有晚于这一版业务时间的入场或离场（ADR-0307 决定三）。有，就是对象此后已凭别的
// 控制事实进了段、交出或交付离场：凭这一版去结束「此刻在场」的参与会结束错一条，进声明的段会让对象两段同时在场、或在
// 离场之后重新在场——前者让对象此后每一次交接都在结束参与那一步响亮失败。控制来源链的重算不在更正里做（CONTEXT「运输
// 交接与实物控制」：后续事实不得被删除或倒退），这里只认出来、一笔不写，答单开的那一格。
//
// 看每段里该对象的链尾，在场、已离场、失效都看：失效的链尾记着一次此后的入场，它撤回的是控制，不是那次交接发生过；那一版
// 日后若又被更正回`已交接`，对象会从失效版本回到那一段，这里若放行就又是两段同时在场。与这一版业务时间同一刻的不算更晚：
// 首登即为`已交接`时同一刻的终点与起点同样立得住（结束那道门只拒终点早于起点）。逐段读，读不动是欠账，判据同重派生。
func laterControlFacts(
	ctx context.Context,
	segments ports.ActualFulfillmentSegmentRegistry,
	handover domain.TransportHandover,
) segmentEntry {
	tenant, object := handover.TenantID(), handover.Object()
	at := domain.AtRegistryPrecision(handover.JudgedAt())
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
		tail, present := record.Segment.ParticipationFor(object)
		if !present {
			continue
		}
		_, _, endedAt, ended := tail.End()
		if domain.AtRegistryPrecision(tail.EnteredAt()).After(at) || (ended && domain.AtRegistryPrecision(endedAt).After(at)) {
			return segmentEntry{refusal: SegmentEntryRefusedLaterControlFacts}
		}
	}
	return segmentEntry{}
}
