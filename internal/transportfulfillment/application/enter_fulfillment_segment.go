package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// segmentEntryDoors 是一次控制事实进段时两条路各自要走的领域门：段还不存在走 establish，
// 已存在走 join。收寄与交接的差别全部收在这两个函数里——其余（要不要进段、段是否已成立、
// 失败算不算欠账）两条来源逐字相同。
//
// 分成两个函数而不是一个「进段」门，是因为领域本来就分成两个：段由**首个**对象的控制事实
// 成立，后续对象是加入既有段。合成一个会把这条边界藏进实现。
type segmentEntryDoors struct {
	object    domain.CarriedObjectReference
	establish func(domain.FulfillmentSegmentReference, domain.PlannedSegmentReference) (domain.ActualFulfillmentSegment, error)
	join      func(domain.ActualFulfillmentSegment, domain.PlannedSegmentReference) (domain.ActualFulfillmentSegment, error)
}

// enterFulfillmentSegment 让一次控制事实的对象进入实际履约段，交回续办引用；空串表示这一半
// 没有欠账（CONTEXT 生命周期①②）。
//
// **段引用缺席时不进段，也不算失败。** 实际履约段不等同于交接范围、计划段、班次或订舱，段身份
// 由谁铸出至今没有裁决，这里不拿手边任一引用顶替；今天的调用方都不给段号，那不是错。
//
// **进段是派生的一侧，它的失败不得回滚来源登记。** 收寄与交接都是控制事实的保全，接货时间是
// 责任起点锚——一次段登记故障抹不掉一条已经发生的物理事实，所以失败只留续办引用。
//
// **领域拒绝与登记册故障是两种答案。** 领域拒了（交接的三裁决里只有`已交接`转出控制、对象已在
// 段内、段已关闭）是正当的业务结果，不留引用：留了会让调用方反复重试一件本就不该发生的事。
// 只有登记册这一侧读不到或写不进才是欠账。
func enterFulfillmentSegment(
	ctx context.Context,
	segments ports.ActualFulfillmentSegmentRegistry,
	clock ports.Clock,
	tenant domain.TenantID,
	segmentReference string,
	plannedReference string,
	doors segmentEntryDoors,
) string {
	if segments == nil || strings.TrimSpace(segmentReference) == "" {
		return ""
	}
	segment, err := domain.NewFulfillmentSegmentReference(segmentReference)
	if err != nil {
		return ""
	}
	var planned domain.PlannedSegmentReference
	if strings.TrimSpace(plannedReference) != "" {
		if planned, err = domain.NewPlannedSegmentReference(plannedReference); err != nil {
			return ""
		}
	}

	// 「这个段是否已成立」每次都问登记册。编排自己记住就是一次竞态：两个对象并发到达时，
	// 各自那份缓存都会说「还没成立」，于是同一个段被立两回。
	key := ports.FulfillmentSegmentKey{TenantID: tenant, Segment: segment}
	existing, found, err := segments.FindByKey(ctx, key)
	if err != nil {
		return segmentEntryContinuation("SEGMENT_REGISTRY_UNAVAILABLE", tenant, segmentReference, doors.object)
	}
	if found {
		return joinExistingSegment(ctx, segments, clock, key, existing, planned, doors)
	}

	established, err := doors.establish(segment, planned)
	if err != nil {
		return ""
	}
	saved, err := segments.Save(ctx, ports.FulfillmentSegmentRecord{
		Key:        key,
		Segment:    established,
		RecordedAt: clock.Now(),
	})
	if err != nil || saved == ports.SegmentSaveOutcomeInvalid {
		return segmentEntryContinuation("SEGMENT_NOT_ESTABLISHED", tenant, segmentReference, doors.object)
	}
	return ""
}

// joinExistingSegment 让后续对象加入既有段并形成自己的参与起点，不动其他对象的起点。
// 参与关系整体从领域取出后原样交给窄写口——拆开来传就等于让适配器重新组装一遍领域已经
// 判完的东西。
func joinExistingSegment(
	ctx context.Context,
	segments ports.ActualFulfillmentSegmentRegistry,
	clock ports.Clock,
	key ports.FulfillmentSegmentKey,
	existing ports.FulfillmentSegmentRecord,
	planned domain.PlannedSegmentReference,
	doors segmentEntryDoors,
) string {
	joined, err := doors.join(existing.Segment, planned)
	if err != nil {
		return ""
	}
	participation, present := joined.ParticipationFor(doors.object)
	if !present {
		return ""
	}
	// `已在段内`是业务答案不是欠账：同一控制范围不因伙伴重投、任务重建或批量重试重复建立参与。
	outcome, err := segments.Join(ctx, key, participation, clock.Now())
	if err != nil || outcome == ports.SegmentJoinOutcomeInvalid {
		return segmentEntryContinuation("OBJECT_NOT_JOINED", key.TenantID, key.Segment.String(), doors.object)
	}
	return ""
}

func segmentEntryContinuation(
	cause string,
	tenant domain.TenantID,
	segmentReference string,
	object domain.CarriedObjectReference,
) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		cause,
		tenant.String(),
		segmentReference,
		object.String(),
	}, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}
