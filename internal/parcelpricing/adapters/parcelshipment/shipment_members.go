package parcelshipment

import (
	"context"
	"fmt"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ShipmentMembers 按一件包裹读它所属的已接受委托，以及这份委托当前仍然有效的成员。
// 取消了的成员不交出去：探逐委托卡的基础运费表用的是此刻仍然有效的那一份清单，不是历史上声明过的全部件。
type ShipmentMembers struct {
	targets       psports.CurrentAcceptedParcelTargetView
	parcels       psports.CurrentDeclaredParcelsView
	cancellations psports.ParcelCancellationView
}

func NewShipmentMembers(
	targets psports.CurrentAcceptedParcelTargetView,
	parcels psports.CurrentDeclaredParcelsView,
	cancellations psports.ParcelCancellationView,
) (*ShipmentMembers, error) {
	if targets == nil || parcels == nil || cancellations == nil {
		return nil, fmt.Errorf("parcel pricing parcelshipment adapter: shipment members view is missing a provider port")
	}
	return &ShipmentMembers{targets: targets, parcels: parcels, cancellations: cancellations}, nil
}

var _ ppports.ShipmentMemberView = (*ShipmentMembers)(nil)

func (adapter *ShipmentMembers) LoadShipmentMembers(
	ctx context.Context,
	tenant ppdomain.TenantID,
	parcel string,
) (ppports.ShipmentMemberPicture, bool, error) {
	none := ppports.ShipmentMemberPicture{}
	psTenant, err := psdomain.NewTenantID(tenant.String())
	if err != nil {
		return none, false, nil
	}
	psParcel, err := psdomain.NewDeclaredParcelID(parcel)
	if err != nil {
		return none, false, nil
	}
	target, found, err := adapter.targets.FindCurrentAcceptedByParcel(ctx, psTenant, psParcel)
	if err != nil || !found {
		return none, found, err
	}
	declared, found, err := adapter.parcels.FindCurrentDeclaredParcels(ctx, psTenant, target.ShipmentRequestID())
	if err != nil || !found {
		return none, found, err
	}
	picture := ppports.ShipmentMemberPicture{Shipment: target.ShipmentRequestID().String()}
	for _, member := range declared {
		_, cancelled, err := adapter.cancellations.FindCancellation(ctx, psTenant, member)
		if err != nil {
			return none, false, err
		}
		if cancelled {
			continue
		}
		picture.Members = append(picture.Members, member.String())
	}
	return picture, true, nil
}
