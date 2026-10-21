package transportfulfillment

import (
	"context"
	"errors"
	"fmt"
	"slices"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var (
	// ErrObjectSegmentsNotVisible 表示控制事实那件对象有参与的段读不动——列的那一问报错，或按键取的那一问报错。对象进没进段要
	// 读得出来才说得清，读不动是依赖的事，续办格。按键读得动却答没有是另一格：段册自相矛盾，归 ErrMovementFactRecordInconsistent。
	ErrObjectSegmentsNotVisible = errors.New(
		"visibility exception transportfulfillment adapter: the segments of the controlled object cannot be read")
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

// SegmentMovementFactLister 按段列出声明了它的在册移动事实。由 TF 的 MovementFacts 满足（tfports.MovementFactsOfSegment）。
type SegmentMovementFactLister interface {
	FindBySegment(ctx context.Context, key tfports.FulfillmentSegmentKey) ([]tfports.MovementFactRecord, error)
}

// BackfillMovementFactsAdapter 是控制事实信封上补派生那一路的处理方（ADR-0224）：对象凭这一版`已交接`或这一版场外揽收进了
// 某一段，就按段找回声明了这一段的在册移动事实，凡事实发生那一刻这件对象在段里在控的，就在它身上补一份移动事实派生。段由
// 这一次控制事实首立，还是对象后来加入既有段，同样走这一路：后者也会先后颠倒——后到的对象，控制事实的业务时间可能早于出发。
//
// 补的是正常路径当初因段不在册而没派生的那一份（ADR-0205 决定四允许事实先于段登记）。判据与幂等键都不另立：在控与否取
// ActualFulfillmentSegment.ObjectsUnderControlAt，派生走 deriveMovementFactOnto，与移动事实信封那一路是同一段代码，所以补过的
// 再被正常路径派生、或正常路径派生过的再被补，都只答已有结果。
//
// 只补投影，不补信号、不重问关务：出发的门禁读数是登记那一刻问的，段不在册时它停在未决、没有阻断的那一件可交分诊；补问归补问
// 机制（ADR-0205 越权风险点 2）。只看这一件对象：同段别的对象凭各自的控制事实进段，各由各的那封信补。
type BackfillMovementFactsAdapter struct {
	handovers TransportHandoverFinder
	pickups   OffsitePickupFinder
	segments  ObjectSegmentFinder
	facts     SegmentMovementFactLister
	members   veports.ConsolidationMembershipView
	derive    MovementProjectionHandler
}

func NewBackfillMovementFactsAdapter(
	handovers TransportHandoverFinder,
	pickups OffsitePickupFinder,
	segments ObjectSegmentFinder,
	facts SegmentMovementFactLister,
	members veports.ConsolidationMembershipView,
	derive MovementProjectionHandler,
) (*BackfillMovementFactsAdapter, error) {
	if handovers == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: handover finder is nil")
	}
	if pickups == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: pickup finder is nil")
	}
	if segments == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: segment finder is nil")
	}
	if facts == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: movement fact lister is nil")
	}
	if members == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: consolidation membership view is nil")
	}
	if derive == nil {
		return nil, fmt.Errorf("visibility exception transportfulfillment adapter: projection handler is nil")
	}
	return &BackfillMovementFactsAdapter{
		handovers: handovers, pickups: pickups, segments: segments, facts: facts, members: members, derive: derive,
	}, nil
}

var (
	_ veinbox.RegisteredTransportHandoverHandler = (*BackfillMovementFactsAdapter)(nil)
	_ veinbox.RegisteredOffsitePickupHandler     = (*BackfillMovementFactsAdapter)(nil)
)

// HandleRegisteredTransportHandover 按信封键取回交接；它转出了控制，就在它让对象进的那一段上补派生。
//
// 一封信一笔事务，失败格与移动事实那一路同口径：任一处读不动或派生未决即整封回滚重投；记录与键不符是仓储不变量已破。拒收与
// 待确认不转出控制、不让对象进段，入账收工。进段那一半欠了账（段册当时写不进）时这一版交接在段里没有参与，同样入账——那一格在
// ADR-0224 越权风险点里。
func (adapter *BackfillMovementFactsAdapter) HandleRegisteredTransportHandover(
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
	return adapter.backfillEntered(ctx, handover.TenantID(), handover.Object(),
		func(participation tfdomain.FulfillmentParticipation) bool {
			return participation.EnteredByHandover(handover)
		})
}

// HandleRegisteredOffsitePickup 按信封键取回对象级揽收，在它让对象进的那一段上补派生。对象级揽收只为成功收寄形成，没有
// 「不转出控制」的那一格；收寄没带段时对象不进段，入账收工。进段欠账同交接那一格。
func (adapter *BackfillMovementFactsAdapter) HandleRegisteredOffsitePickup(
	ctx context.Context,
	registered veinbox.RegisteredOffsitePickup,
) error {
	record, err := findRegisteredPickup(ctx, adapter.pickups, registered)
	if err != nil {
		return err
	}
	pickup := record.Pickup
	return adapter.backfillEntered(ctx, pickup.TenantID(), pickup.Object(),
		func(participation tfdomain.FulfillmentParticipation) bool {
			return participation.EnteredByPickup(pickup)
		})
}

// backfillEntered 找出这一次控制事实让对象进的那一段，在那一段上补派生。对象一程走几段，早先那几段与这一次无关：只认入场依据
// 指的正是这一次的那条参与（EnteredByHandover、EnteredByPickup；更正落下的替代参与凭更正那一版认得出）。一次控制事实至多让对象
// 进一段，认到就停。按什么次序认只影响读几段，不影响认到的是哪一段。
func (adapter *BackfillMovementFactsAdapter) backfillEntered(
	ctx context.Context,
	tenant tfdomain.TenantID,
	object tfdomain.CarriedObjectReference,
	enteredBy func(tfdomain.FulfillmentParticipation) bool,
) error {
	keys, err := adapter.segments.FindSegmentsForObject(ctx, tenant, object)
	if err != nil {
		return fmt.Errorf("%w: object %q: %v", ErrObjectSegmentsNotVisible, object, err)
	}
	for index := len(keys) - 1; index >= 0; index-- {
		segment, entered, err := adapter.segmentEnteredBy(ctx, keys[index], tenant, enteredBy)
		if err != nil {
			return err
		}
		if entered {
			return adapter.backfill(ctx, keys[index], segment, object)
		}
	}
	return nil
}

// segmentEnteredBy 取回段，答这一次控制事实是不是让对象进了它。
func (adapter *BackfillMovementFactsAdapter) segmentEnteredBy(
	ctx context.Context,
	key tfports.FulfillmentSegmentKey,
	tenant tfdomain.TenantID,
	enteredBy func(tfdomain.FulfillmentParticipation) bool,
) (tfdomain.ActualFulfillmentSegment, bool, error) {
	none := tfdomain.ActualFulfillmentSegment{}
	record, found, err := adapter.segments.FindByKey(ctx, key)
	if err != nil {
		return none, false, fmt.Errorf("%w: segment %q: %v", ErrObjectSegmentsNotVisible, key.Segment, err)
	}
	// 段不能被删除（CONTEXT），按对象列得出的段按键却答没有，是段册自相矛盾，不是等谁。
	if !found || key.TenantID != tenant || record.Key != key ||
		record.Segment.Segment() != key.Segment || record.Segment.TenantID() != key.TenantID {
		return none, false, fmt.Errorf("%w: segment %q", ErrMovementFactRecordInconsistent, key.Segment)
	}
	if slices.ContainsFunc(record.Segment.Participations(), enteredBy) {
		return record.Segment, true, nil
	}
	return none, false, nil
}

// backfill 按段列出声明了它的在册移动事实，逐版在这件对象上补派生；这件对象是集运单元时，补到它在那一版事实发生那一刻的成员
// 身上，展开与正常路径同一处（trackedParcelsAt）。次序取读口交回的次序——同一条事实按登记先后——因为正常路径当初也是按登记先后
// 逐封收到各版的：更正版的替代关系因而落在前版之后，中间各版投影与正常路径同形。
func (adapter *BackfillMovementFactsAdapter) backfill(
	ctx context.Context,
	key tfports.FulfillmentSegmentKey,
	segment tfdomain.ActualFulfillmentSegment,
	object tfdomain.CarriedObjectReference,
) error {
	records, err := adapter.facts.FindBySegment(ctx, key)
	if errors.Is(err, tfports.ErrMovementFactRecordUnreadable) {
		// 坏行重投一万次还是坏行；记成未决会让它以「等依赖」之名挂在队头烧失败预算，运维从错的入口进。
		return fmt.Errorf("%w: segment %q: %v", ErrMovementFactRecordInconsistent, key.Segment, err)
	}
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
		parcels, err := trackedParcelsAt(ctx, adapter.members, key.TenantID,
			[]tfdomain.CarriedObjectReference{object}, record.Fact.OccurredAt())
		if err != nil {
			return err
		}
		for _, parcel := range parcels {
			if err := deriveMovementFactOnto(ctx, adapter.derive, record, kind, parcel); err != nil {
				return err
			}
		}
	}
	return nil
}
