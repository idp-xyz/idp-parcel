package nodeoperations

import (
	"context"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
)

// MeasuredParcel 从一次实测变化解出它对上的那一件包裹；对不上答 false，这次不另起任何评价请求。
// 只认作业实物上的测量：集运单元上的测量不对应哪一件包裹。作业实物此刻关联着零件或多件包裹同样对不上——
// 不在多件里挑一件猜，也不为零件另起一份请求，下一次关联到恰好一件时第一次评价自己会读到实测。
//
// 销售与采购两路的重评都先问这一句，「恰好一件」这条规则只在这里写一遍；两路各自把解出的包裹译成自己上下文的类型。
func MeasuredParcel(
	ctx context.Context,
	associations noports.CurrentParcelAssociation,
	changed sainbox.ChangedMeasurement,
) (nodomain.ParcelAssociationReference, bool, error) {
	if changed.ObjectKind != nodomain.HandlingUnitObject.String() {
		return nodomain.ParcelAssociationReference{}, false, nil
	}
	tenant, err := nodomain.NewTenantID(changed.TenantID)
	if err != nil {
		return nodomain.ParcelAssociationReference{}, false, err
	}
	unit, err := nodomain.NewHandlingUnitID(changed.ObjectID)
	if err != nil {
		return nodomain.ParcelAssociationReference{}, false, err
	}
	parcels, err := associations.ListParcels(ctx, tenant, unit)
	if err != nil {
		return nodomain.ParcelAssociationReference{}, false, err
	}
	if len(parcels) != 1 {
		return nodomain.ParcelAssociationReference{}, false, nil
	}
	return parcels[0], true, nil
}
