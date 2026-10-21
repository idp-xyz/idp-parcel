package transportfulfillment

import (
	"context"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// CurrentCarrierPickupFinder 按（租户，载运对象）取该对象那条首次有效收寄链的链尾。由 TF 的
// CarrierFirstEffectivePickups 满足。取消提交边界问的是「此刻在不在册」，所以读链尾；信封那一路读指名那一代，
// 两者不是同一个问题。
type CurrentCarrierPickupFinder interface {
	FindCurrentByObject(
		ctx context.Context,
		tenant tfdomain.TenantID,
		object tfdomain.CarriedObjectReference,
	) (tfports.CarrierFirstEffectivePickupRecord, bool, error)
}

// CarrierPickupBoundaryAdapter 是 psports.CarrierPickupBoundaryView 的实现：取消提交边界上读面单渠道服务的
// 实际承运商首次有效收寄（PS CONTEXT 取消一条）。PS 不判「这条证据算不算收寄」，只把 TF 判过的结果译成三格。
//
// 载运对象取包裹标识原串，与收寄那一路反查包裹用同一个串（JudgeOnCarrierFirstEffectivePickupAdapter）——
// 对象可能是集运单元那一格今天同样不替它猜，查不中即答没有。
type CarrierPickupBoundaryAdapter struct {
	pickups CurrentCarrierPickupFinder
}

func NewCarrierPickupBoundaryAdapter(pickups CurrentCarrierPickupFinder) (*CarrierPickupBoundaryAdapter, error) {
	if pickups == nil {
		return nil, fmt.Errorf("parcel shipment transportfulfillment adapter: carrier pickup finder is nil")
	}
	return &CarrierPickupBoundaryAdapter{pickups: pickups}, nil
}

var _ psports.CarrierPickupBoundaryView = (*CarrierPickupBoundaryAdapter)(nil)

func (adapter *CarrierPickupBoundaryAdapter) FindCurrentCarrierPickup(
	ctx context.Context,
	tenant psdomain.TenantID,
	parcel psdomain.DeclaredParcelID,
) (psports.CarrierPickupBoundary, error) {
	none := psports.CarrierPickupBoundary{}
	tfTenant, err := tfdomain.NewTenantID(tenant.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	object, err := tfdomain.NewCarriedObjectReference(parcel.String())
	if err != nil {
		return none, fmt.Errorf("%w: carried object: %v", ErrUntranslatableAnswer, err)
	}
	record, found, err := adapter.pickups.FindCurrentByObject(ctx, tfTenant, object)
	if err != nil {
		return none, fmt.Errorf("find current carrier first effective pickup: %w", err)
	}
	if !found {
		return psports.CarrierPickupBoundary{State: psports.CarrierPickupBoundaryAbsent}, nil
	}
	pickup := record.Pickup
	if pickup.TenantID() != tfTenant || pickup.Object() != object {
		return none, fmt.Errorf("%w: chain tail of %q belongs to %q/%q",
			ErrCarrierPickupRecordInconsistent, parcel, pickup.TenantID(), pickup.Object())
	}

	switch pickup.Result() {
	case tfdomain.CarrierPickupVoided:
		return psports.CarrierPickupBoundary{State: psports.CarrierPickupBoundaryAbsent}, nil
	case tfdomain.CarrierPickupPending:
		return psports.CarrierPickupBoundary{State: psports.CarrierPickupBoundaryPending}, nil
	case tfdomain.CarrierPickupFormed:
		occurredAt, present := pickup.OccurredAt()
		if !present {
			return none, fmt.Errorf("%w: formed pickup %q carries no occurrence time",
				ErrCarrierPickupRecordInconsistent, pickup.Fact())
		}
		key := tfports.CarrierFirstEffectivePickupKey{TenantID: tfTenant, Fact: pickup.Fact(), Version: pickup.Version()}
		spec, err := firstEffectivePickupSpecFor(key, occurredAt)
		if err != nil {
			return none, err
		}
		return psports.CarrierPickupBoundary{State: psports.CarrierPickupBoundaryFormed, Pickup: spec}, nil
	default:
		return none, fmt.Errorf("%w: chain tail %q is %q", ErrCarrierPickupRecordInconsistent, pickup.Fact(), pickup.Result())
	}
}
