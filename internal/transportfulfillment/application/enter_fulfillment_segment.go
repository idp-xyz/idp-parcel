package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// SegmentEntryRefusal 是进段那一半被领域**正当拒绝**时单独答给调用方的格（票
// tf-segment-lifecycle-closure/03 裁决 3）。
//
// 它与续办引用是两种东西：续办引用说「登记册那一侧读不到或写不进，等它恢复重试同一份」；这一格
// 说「段不收了，重试一万次都一样，去另立新段」。此前段已关闭后新对象凭正当控制事实来到同段，
// 可观察结果只有「交接在册、段里没它」，与正常入段在调用方眼里同形——这一格就是为了让两者分开。
//
// 封闭集合而且今天只有一格：领域的其余拒绝（对象已在段内、拒收与待确认不转出控制）各有自己的
// 可观察形状，不在这里另开格；要加格是一次产品判断，不是补枚举。
type SegmentEntryRefusal uint8

const (
	SegmentEntryRefusalNone SegmentEntryRefusal = iota
	SegmentEntryRefusedSegmentClosed
)

func (refusal SegmentEntryRefusal) String() string {
	switch refusal {
	case SegmentEntryRefusedSegmentClosed:
		return "SEGMENT_CLOSED"
	default:
		return ""
	}
}

// segmentEntry 是进段那一半的全部回答：欠账（续办引用）与正当拒绝各占一格，两格不会同时非空。
type segmentEntry struct {
	continuation string
	refusal      SegmentEntryRefusal
}

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

// enterFulfillmentSegment 让一次控制事实的对象进入实际履约段，交回进段那一半的回答：续办引用
// 空串且拒绝格为空表示这一半没有欠账也没被拒（CONTEXT 生命周期①②）。
//
// **段引用缺席时不进段，也不算失败。** 实际履约段不等同于交接范围、计划段、班次或订舱，段身份
// 由谁铸出至今没有裁决，这里不拿手边任一引用顶替；今天的调用方都不给段号，那不是错。
//
// 下面两处 `err != nil` 交回空串**今天走不到**：两个引用构造器唯一的失败是去空白后为空，而那
// 一种已经被上面那道缺席判据先筛走了。留着它们是构造器合同的一部分（引用日后加了格式规则就会
// 真的失败），但**不要据此以为「引用写坏了」是个可观察的答案格**——票 tf-unwired-seven/08
// 曾按那个前提要求开一格，取证后作废，经过记在该票面。
//
// **进段是派生的一侧，它的失败不得回滚来源登记。** 收寄与交接都是控制事实的保全，接货时间是
// 责任起点锚——一次段登记故障抹不掉一条已经发生的物理事实，所以失败只留续办引用。
//
// **领域拒绝与登记册故障是两种答案。** 领域拒了（交接的三裁决里只有`已交接`转出控制、对象已在
// 段内、段已关闭）是正当的业务结果，不留引用：留了会让调用方反复重试一件本就不该发生的事。
// 只有登记册这一侧读不到或写不进才是欠账。领域拒绝里**只有段已关闭单开一格**答出去
// （SegmentEntryRefusal），理由在那个类型上。
//
// **段首登之后同笔铸实际承运商判断的首版**（CONTEXT「实际履约段成立时即形成首个判断版本」；票
// tf-segment-lifecycle-closure/02）。它挂在这里而不是各调用方，理由与进段本身相同：段只从这一道门
// 成立，首版跟着段走。`judgments` 可缺席，判据同 `segments` 缺席——派生一侧缺席不让来源保全停摆；
// 在场而写不进则是欠账，续办引用与段那一半同一格：没有首版的段在 CONTEXT 里不是一个完整成立的段。
func enterFulfillmentSegment(
	ctx context.Context,
	segments ports.ActualFulfillmentSegmentRegistry,
	judgments ports.ActualCarrierJudgmentRegistry,
	clock ports.Clock,
	tenant domain.TenantID,
	segmentReference string,
	plannedReference string,
	doors segmentEntryDoors,
) segmentEntry {
	if segments == nil || strings.TrimSpace(segmentReference) == "" {
		return segmentEntry{}
	}
	segment, err := domain.NewFulfillmentSegmentReference(segmentReference)
	if err != nil {
		return segmentEntry{}
	}
	var planned domain.PlannedSegmentReference
	if strings.TrimSpace(plannedReference) != "" {
		if planned, err = domain.NewPlannedSegmentReference(plannedReference); err != nil {
			return segmentEntry{}
		}
	}

	// 「这个段是否已成立」每次都问登记册。编排自己记住就是一次竞态：两个对象并发到达时，
	// 各自那份缓存都会说「还没成立」，于是同一个段被立两回。
	key := ports.FulfillmentSegmentKey{TenantID: tenant, Segment: segment}
	existing, found, err := segments.FindByKey(ctx, key)
	if err != nil {
		return segmentEntry{continuation: owed("SEGMENT_REGISTRY_UNAVAILABLE", tenant, segmentReference, doors.object)}
	}
	if found {
		return joinExistingSegment(ctx, segments, clock, key, existing, planned, doors)
	}

	established, err := doors.establish(segment, planned)
	if err != nil {
		return segmentEntry{}
	}
	saved, err := segments.Save(ctx, ports.FulfillmentSegmentRecord{
		Key:        key,
		Segment:    established,
		RecordedAt: clock.Now(),
	})
	if err != nil || saved == ports.SegmentSaveOutcomeInvalid {
		return segmentEntry{continuation: owed("SEGMENT_NOT_ESTABLISHED", tenant, segmentReference, doors.object)}
	}
	// 只有本次真把段立起来的那一方开判断：并发下撞键的一方读回的是赢家的段，首版由赢家开。
	if saved == ports.SegmentSaved {
		return segmentEntry{continuation: openCarrierJudgment(ctx, judgments, clock, key, established, doors.object)}
	}
	return segmentEntry{}
}

// openCarrierJudgment 在段首登之后开判断历史：首版待确认（无合格证据），业务时间取段成立时刻——首个
// 对象的控制起点（CONTEXT 生命周期「实际承运商判断」首条；ADR-0103 决定二）。交回续办引用，空串即无欠账。
//
// 成立事实随带的合格证据不在这里收：今天两条立段来源（收寄、`已交接`交接）都不携带「接收方就是承运
// 主体」这一判断——收寄的执行方是运输方引用不是承运主体身份，交接的接收方可以是节点也可以是承运方——
// 替它们推一步就是 CONTEXT 明禁的推断。证据从「形成实际承运商判断」用例进来。
func openCarrierJudgment(
	ctx context.Context,
	judgments ports.ActualCarrierJudgmentRegistry,
	clock ports.Clock,
	key ports.FulfillmentSegmentKey,
	established domain.ActualFulfillmentSegment,
	object domain.CarriedObjectReference,
) string {
	if judgments == nil {
		return ""
	}
	judgment, err := domain.OpenActualCarrierJudgment(domain.OpenActualCarrierJudgmentSpec{
		TenantID:      key.TenantID,
		Segment:       key.Segment,
		EstablishedAt: segmentEstablishedAt(established),
		FormedAt:      clock.Now(),
	})
	if err != nil {
		return owed("CARRIER_JUDGMENT_NOT_OPENED", key.TenantID, key.Segment.String(), object)
	}
	outcome, err := judgments.Open(ctx, ports.ActualCarrierJudgmentRecord{
		Key:        ports.ActualCarrierJudgmentKey{TenantID: key.TenantID, Segment: key.Segment},
		Judgment:   judgment,
		RecordedAt: clock.Now(),
	})
	if err != nil || outcome == ports.JudgmentOpenOutcomeInvalid {
		return owed("CARRIER_JUDGMENT_NOT_OPENED", key.TenantID, key.Segment.String(), object)
	}
	return ""
}

// segmentEstablishedAt 是段成立的业务时刻：成员里最早的控制起点。段本身不存这个时刻——它由首个对象的
// 控制事实派生（CONTEXT 生命周期①），这里按成员算回来，不给段聚合加一格（票面「不改 actual_fulfillment_
// segment.go 任何导出签名」）。
func segmentEstablishedAt(segment domain.ActualFulfillmentSegment) time.Time {
	var earliest time.Time
	for _, participation := range segment.Participations() {
		if earliest.IsZero() || participation.EnteredAt().Before(earliest) {
			earliest = participation.EnteredAt()
		}
	}
	return earliest
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
) segmentEntry {
	joined, err := doors.join(existing.Segment, planned)
	if err != nil {
		// 段已关闭是领域拒绝里唯一答出去的一格：「不再接受新对象」是一个已经做出的决定，调用方
		// 该做的是另立新段，而它从「交接在册、段里没它」里读不出这一点。其余拒绝照旧不出声。
		if errors.Is(err, domain.ErrSegmentClosed) {
			return segmentEntry{refusal: SegmentEntryRefusedSegmentClosed}
		}
		return segmentEntry{}
	}
	participation, present := joined.ParticipationFor(doors.object)
	if !present {
		return segmentEntry{}
	}
	// `已在段内`是业务答案不是欠账：同一控制范围不因伙伴重投、任务重建或批量重试重复建立参与。
	outcome, err := segments.Join(ctx, key, participation, clock.Now())
	if err != nil || outcome == ports.SegmentJoinOutcomeInvalid {
		return segmentEntry{continuation: owed("OBJECT_NOT_JOINED", key.TenantID, key.Segment.String(), doors.object)}
	}
	return segmentEntry{}
}

// owed 造一个续办引用：只有登记册这一侧读不到或写不进才走到这里。
func owed(
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
