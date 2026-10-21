package transportfulfillment

import (
	"context"
	"errors"
	"fmt"
	"slices"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

var (
	// ErrHandoverSegmentNotVisible 表示交接对象有参与的那几段读不动——列不出来，或按键取不回。对象进没进段要读得出来才说得清，
	// 读不动是依赖的事，续办格。
	ErrHandoverSegmentNotVisible = errors.New(
		"visibility exception transportfulfillment adapter: the segments of the handed-over object cannot be read")
	// ErrMovementFactsOfSegmentNotVisible 表示按段列移动事实那一问读不动。续办格：读得动之后重投，补派生从头再走一遍，已派生过
	// 的答已有结果。
	ErrMovementFactsOfSegmentNotVisible = errors.New(
		"visibility exception transportfulfillment adapter: the movement facts declaring the segment cannot be listed")
)

// ObjectSegmentFinder 答对象在哪些段里有参与，并按键取回段连同全部参与。由 TF 的 FulfillmentSegments 满足。
type ObjectSegmentFinder interface {
	FindSegmentsForObject(
		ctx context.Context,
		tenant tfdomain.TenantID,
		object tfdomain.CarriedObjectReference,
	) ([]tfports.FulfillmentSegmentKey, error)
	FindByKey(ctx context.Context, key tfports.FulfillmentSegmentKey) (tfports.FulfillmentSegmentRecord, bool, error)
}

// BackfillMovementFactsOnHandoverAdapter 是交接信封上补派生那一路的处理方（ADR-0224）：对象凭这一版`已交接`进了某一段，就按段
// 找回声明了这一段的在册移动事实，凡事实发生那一刻这件对象在段里在控的，就在它身上补一份移动事实派生。
//
// 补的是正常路径当初因段不在册而没派生的那一份（ADR-0205 决定四允许事实先于段登记）。判据与幂等键都不另立：在控与否取
// ActualFulfillmentSegment.ObjectsUnderControlAt，派生走 deriveMovementFactOnto，与移动事实信封那一路是同一段代码，所以补过的
// 再被正常路径派生、或正常路径派生过的再被补，都只答已有结果。
//
// 只补投影，不补信号、不重问关务：出发的门禁读数是登记那一刻问的，段不在册时它停在未决、没有阻断的那一件可交分诊；补问归补问
// 机制（ADR-0205 越权风险点 2）。只看这一件对象：同段别的对象凭各自的交接进段，各由各的那封信补。
type BackfillMovementFactsOnHandoverAdapter struct {
	handovers TransportHandoverFinder
	segments  ObjectSegmentFinder
	facts     tfports.MovementFactsOfSegment
	derive    MovementProjectionHandler
}

func NewBackfillMovementFactsOnHandoverAdapter(
	handovers TransportHandoverFinder,
	segments ObjectSegmentFinder,
	facts tfports.MovementFactsOfSegment,
	derive MovementProjectionHandler,
) (*BackfillMovementFactsOnHandoverAdapter, error) {
	if handovers == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: handover finder is nil")
	}
	if segments == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: segment finder is nil")
	}
	if facts == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: movement facts of segment is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: projection handler is nil")
	}
	return &BackfillMovementFactsOnHandoverAdapter{handovers: handovers, segments: segments, facts: facts, derive: derive}, nil
}

var _ veinbox.RegisteredTransportHandoverHandler = (*BackfillMovementFactsOnHandoverAdapter)(nil)

// HandleRegisteredTransportHandover 按信封四维取回交接；它转出了控制，就在它让对象进的那一段上补派生。
//
// 一封信一笔事务，失败格与移动事实那一路同口径：任一处读不动或派生未决即整封回滚重投；记录与键不符是仓储不变量已破。拒收与
// 待确认不转出控制、不让对象进段，入账收工。进段那一半欠了账（段册当时写不进）时这一版交接在段里没有参与，同样入账——那一格在
// ADR-0224 越权风险点里。
func (adapter *BackfillMovementFactsOnHandoverAdapter) HandleRegisteredTransportHandover(
	ctx context.Context,
	registered veinbox.RegisteredTransportHandover,
) error {
	record, err := findRegisteredHandover(ctx, adapter.handovers, registered)
	if err != nil {
		return err
	}
	handover := record.Handover
	if !handover.TransfersControl() {
		return nil
	}
	keys, err := adapter.segments.FindSegmentsForObject(ctx, handover.TenantID(), handover.Object())
	if err != nil {
		return fmt.Errorf("%w: object %q: %v", ErrHandoverSegmentNotVisible, handover.Object(), err)
	}
	for _, key := range keys {
		segment, entered, err := adapter.segmentEnteredBy(ctx, key, handover)
		if err != nil {
			return err
		}
		if !entered {
			continue
		}
		if err := adapter.backfill(ctx, key, segment, handover.Object()); err != nil {
			return err
		}
	}
	return nil
}

// segmentEnteredBy 取回段，答这一版交接是不是让对象进了它。对象一程走几段，早先那几段与这一版无关；只认入场依据指的正是这一版
// 的那条参与（EnteredByHandover，ADR-0221），更正落下的替代参与也凭更正那一版认得出。
func (adapter *BackfillMovementFactsOnHandoverAdapter) segmentEnteredBy(
	ctx context.Context,
	key tfports.FulfillmentSegmentKey,
	handover tfdomain.TransportHandover,
) (tfdomain.ActualFulfillmentSegment, bool, error) {
	none := tfdomain.ActualFulfillmentSegment{}
	record, found, err := adapter.segments.FindByKey(ctx, key)
	if err != nil {
		return none, false, fmt.Errorf("%w: segment %q: %v", ErrHandoverSegmentNotVisible, key.Segment, err)
	}
	// 段不能被删除（CONTEXT），按对象列得出的段按键却取不回，是段册自相矛盾，不是等谁。
	if !found || key.TenantID != handover.TenantID() || record.Key != key ||
		record.Segment.Segment() != key.Segment || record.Segment.TenantID() != key.TenantID {
		return none, false, fmt.Errorf("%w: segment %q", ErrMovementFactRecordInconsistent, key.Segment)
	}
	for _, participation := range record.Segment.Participations() {
		if participation.EnteredByHandover(handover) {
			return record.Segment, true, nil
		}
	}
	return none, false, nil
}

// backfill 按段列出声明了它的在册移动事实，逐版在这件对象上补派生。次序取列出的次序（同一条事实按登记先后），与正常路径当初
// 收信封的次序同形，更正版的替代关系因而落在前版之后。
func (adapter *BackfillMovementFactsOnHandoverAdapter) backfill(
	ctx context.Context,
	key tfports.FulfillmentSegmentKey,
	segment tfdomain.ActualFulfillmentSegment,
	object tfdomain.CarriedObjectReference,
) error {
	records, err := adapter.facts.FindBySegment(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: segment %q: %v", ErrMovementFactsOfSegmentNotVisible, key.Segment, err)
	}
	for _, record := range records {
		declared, has := record.Fact.Segment()
		if !movementFactRecordAgrees(record, record.Key) || record.Key.TenantID != key.TenantID || !has || declared != key.Segment {
			return fmt.Errorf("%w: fact %q version %q listed for segment %q",
				ErrMovementFactRecordInconsistent, record.Key.Fact, record.Key.Version, key.Segment)
		}
		if !slices.Contains(segment.ObjectsUnderControlAt(record.Fact.OccurredAt()), object) {
			continue
		}
		kind, err := kindForMovementFact(record.Fact.Kind())
		if err != nil {
			return err
		}
		if err := deriveMovementFactOnto(ctx, adapter.derive, record, kind, object); err != nil {
			return err
		}
	}
	return nil
}
