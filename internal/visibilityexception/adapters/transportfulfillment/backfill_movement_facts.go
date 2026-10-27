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

// BackfillMovementFactsAdapter 是控制事实信封上补派生那一路的处理方（ADR-0224，ADR-0302）：对象的一版`已交接`或一版场外揽收
// 登了，就把这件对象有参与的每一段都扫一遍，按段找回声明了那一段的在册移动事实，凡事实发生那一刻这件对象在段里在控的，就在它
// 身上补一份移动事实派生。段由这一次控制事实首立、对象后来加入既有段，同样走这一路：后者也会先后颠倒——后到的对象，控制事实的
// 业务时间可能早于出发。
//
// 扫每一段而不只看这一次控制事实让对象进的那一段（ADR-0302 停用 ADR-0224 决定三那一句）：控制事实登记时进段那一半欠了账，这一版
// 在段里就没有参与，日后对象不论经哪条路进了那一段（续办、存量迁移、直写段册），那一刻都没有信封唤醒补派生；按对象扫，这一段
// 先于进段登记的出发就由这件对象此后的下一份控制事实补上。
//
// 补的是正常路径当初因段不在册而没派生的那一份（ADR-0205 决定四允许事实先于段登记）。判据与幂等键都不另立：在控与否取
// ActualFulfillmentSegment.ObjectsUnderControlAt，派生走 deriveMovementFactOnto，与移动事实信封那一路是同一段代码，所以补过的
// 再被正常路径派生、或正常路径派生过的再被补，都只答已有结果——扫到早已派生过的段，代价只是几次读。
//
// 只补投影，不补信号、不重问关务：出发的门禁读数是登记那一刻问的，段不在册时它停在未决、没有阻断的那一件可交分诊；补问归补问
// 机制（ADR-0205 越权风险点 2）。只看这一件对象：同段别的对象由各自的控制事实补。
type BackfillMovementFactsAdapter struct {
	handovers TransportHandoverFinder
	pickups   OffsitePickupFinder
	backfill  movementBackfill
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
		handovers: handovers, pickups: pickups,
		backfill: movementBackfill{segments: segments, facts: facts, members: members, derive: derive},
	}, nil
}

var (
	_ veinbox.RegisteredTransportHandoverHandler = (*BackfillMovementFactsAdapter)(nil)
	_ veinbox.RegisteredOffsitePickupHandler     = (*BackfillMovementFactsAdapter)(nil)
)

// HandleRegisteredTransportHandover 按信封键取回交接；它转出了控制，就扫这件对象有参与的每一段补派生。
//
// 一封信一笔事务，失败格与移动事实那一路同口径：任一处读不动或派生未决即整封回滚重投；记录与键不符是仓储不变量已破。拒收与
// 待确认不转出控制、不让对象进段，入账收工，连段都不读。这一版交接进段那一半欠了账（段册当时写不进）时它在段里没有参与，扫到的
// 是对象此前进过的各段；欠着的那一段日后有了这件对象，由它的下一份控制事实扫到。
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
	return adapter.backfill.backfillObject(ctx, handover.TenantID(), handover.Object(), "")
}

// HandleRegisteredOffsitePickup 按信封键取回对象级揽收，扫这件对象有参与的每一段补派生。对象级揽收只为成功收寄形成，没有
// 「不转出控制」的那一格；收寄没带段、对象也从没进过别的段时，扫不到段，入账收工。进段欠账同交接那一格。
func (adapter *BackfillMovementFactsAdapter) HandleRegisteredOffsitePickup(
	ctx context.Context,
	registered veinbox.RegisteredOffsitePickup,
) error {
	record, err := findRegisteredPickup(ctx, adapter.pickups, registered)
	if err != nil {
		return err
	}
	pickup := record.Pickup
	return adapter.backfill.backfillObject(ctx, pickup.TenantID(), pickup.Object(), "")
}

// movementBackfill 是移动事实补派生的共用一半：按对象列出它有参与的段，按段列出声明了那一段的在册移动事实，凡事实发生那一刻对象
// 在控的就在追踪包裹上补派生。控制事实信封上的补派生（BackfillMovementFactsAdapter，ADR-0224、ADR-0302）与实物识别成功信封上的
// 补派生（BackfillIdentifiedMemberAdapter，ADR-0299）都走它，判据、展开与幂等键因此只有这一处。
type movementBackfill struct {
	segments ObjectSegmentFinder
	facts    SegmentMovementFactLister
	members  veports.ConsolidationMembershipView
	derive   MovementProjectionHandler
}

// backfillObject 按对象扫它有参与的每一段，在每一段上补派生（ADR-0302）。不问对象凭哪一份控制事实进的段：入场依据只回答「凭什么
// 进」，补不补只看事实发生那一刻它在不在控，与正常路径同一条判据。扫的次序不影响结论——各段各补各的事实，同一把幂等键。
//
// onlyParcel 非空时只补到这一件追踪包裹上，展开到的别的成员不动：实物识别成功的那一封信只欠这一件，同袋别的成员早已投过或各有
// 各的信封。为空即对象展开到的每一件都补。
func (core movementBackfill) backfillObject(
	ctx context.Context,
	tenant tfdomain.TenantID,
	object tfdomain.CarriedObjectReference,
	onlyParcel string,
) error {
	keys, err := core.segments.FindSegmentsForObject(ctx, tenant, object)
	if err != nil {
		return fmt.Errorf("%w: object %q: %v", ErrObjectSegmentsNotVisible, object, err)
	}
	for _, key := range keys {
		segment, err := core.segmentOf(ctx, key, tenant)
		if err != nil {
			return err
		}
		if err := core.backfillSegment(ctx, key, segment, object, onlyParcel); err != nil {
			return err
		}
	}
	return nil
}

// segmentOf 按键取回对象有参与的一段。
func (core movementBackfill) segmentOf(
	ctx context.Context,
	key tfports.FulfillmentSegmentKey,
	tenant tfdomain.TenantID,
) (tfdomain.ActualFulfillmentSegment, error) {
	record, found, err := core.segments.FindByKey(ctx, key)
	if err != nil {
		return tfdomain.ActualFulfillmentSegment{}, fmt.Errorf("%w: segment %q: %v", ErrObjectSegmentsNotVisible, key.Segment, err)
	}
	// 段不能被删除（CONTEXT），按对象列得出的段按键却答没有，是段册自相矛盾，不是等谁。
	if !found || key.TenantID != tenant || record.Key != key ||
		record.Segment.Segment() != key.Segment || record.Segment.TenantID() != key.TenantID {
		return tfdomain.ActualFulfillmentSegment{}, fmt.Errorf("%w: segment %q", ErrMovementFactRecordInconsistent, key.Segment)
	}
	return record.Segment, nil
}

// backfillSegment 按段列出声明了它的在册移动事实，逐版在这件对象上补派生；这件对象是集运单元时，补到它在那一版事实发生那一刻的成员
// 身上，展开与正常路径同一处（trackedParcelsAt）。次序取读口交回的次序——同一条事实按登记先后——因为正常路径当初也是按登记先后
// 逐封收到各版的：更正版的替代关系因而落在前版之后，中间各版投影与正常路径同形。
func (core movementBackfill) backfillSegment(
	ctx context.Context,
	key tfports.FulfillmentSegmentKey,
	segment tfdomain.ActualFulfillmentSegment,
	object tfdomain.CarriedObjectReference,
	onlyParcel string,
) error {
	records, err := core.facts.FindBySegment(ctx, key)
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
		parcels, err := trackedParcelsAt(ctx, core.members, key.TenantID,
			[]tfdomain.CarriedObjectReference{object}, record.Fact.OccurredAt())
		if err != nil {
			return err
		}
		for _, parcel := range parcels {
			if onlyParcel != "" && parcel.String() != onlyParcel {
				continue
			}
			if err := deriveMovementFactOnto(ctx, core.derive, record, kind, parcel); err != nil {
				return err
			}
		}
	}
	return nil
}
