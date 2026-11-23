package parcelshipment

import (
	"context"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ShipmentIntakePictureAdapter 按一件包裹读它所属的已接受委托，以及各件此刻的有效网络收寄。件清单只来自委托的当前声明，不另编。
type ShipmentIntakePictureAdapter struct {
	targets       psports.CurrentAcceptedParcelTargetView
	parcels       psports.CurrentDeclaredParcelsView
	starts        psports.ResponsibilityStartView
	cancellations psports.ParcelCancellationView
	products      psports.AcceptedServiceProductView
}

func NewShipmentIntakePictureAdapter(
	targets psports.CurrentAcceptedParcelTargetView,
	parcels psports.CurrentDeclaredParcelsView,
	starts psports.ResponsibilityStartView,
	cancellations psports.ParcelCancellationView,
	products psports.AcceptedServiceProductView,
) (*ShipmentIntakePictureAdapter, error) {
	if targets == nil || parcels == nil || starts == nil || cancellations == nil || products == nil {
		return nil, fmt.Errorf("settlement accounting parcelshipment adapter: shipment intake picture is missing a provider port")
	}
	return &ShipmentIntakePictureAdapter{
		targets: targets, parcels: parcels, starts: starts, cancellations: cancellations, products: products,
	}, nil
}

func (adapter *ShipmentIntakePictureAdapter) LoadByParcel(
	ctx context.Context,
	tenant sadomain.TenantID,
	parcel sadomain.ParcelReference,
) (saports.ShipmentIntakePicture, bool, error) {
	none := saports.ShipmentIntakePicture{}
	psTenant, err := psdomain.NewTenantID(tenant.String())
	if err != nil {
		return none, false, err
	}
	psParcel, err := psdomain.NewDeclaredParcelID(parcel.String())
	if err != nil {
		return none, false, err
	}
	target, found, err := adapter.targets.FindCurrentAcceptedByParcel(ctx, psTenant, psParcel)
	if err != nil || !found {
		return none, found, err
	}
	declared, found, err := adapter.parcels.FindCurrentDeclaredParcels(ctx, psTenant, target.ShipmentRequestID())
	if err != nil || !found {
		return none, found, err
	}
	picture := saports.ShipmentIntakePicture{Shipment: target.ShipmentRequestID().String()}
	for _, member := range declared {
		intake := saports.ShipmentMemberIntake{Parcel: member.String()}
		cancellation, cancelled, err := adapter.cancellations.FindCancellation(ctx, psTenant, member)
		if err != nil {
			return none, false, err
		}
		_ = cancellation
		intake.Cancelled = cancelled
		if !cancelled {
			record, adopted, err := adapter.starts.FindResponsibilityStart(ctx, psTenant, member)
			if err != nil {
				return none, false, err
			}
			if adopted && !record.Intake.ResponsibilityStart().IsZero() {
				intake.Adopted = true
				intake.ResponsibilityStart = record.Intake.ResponsibilityStart()
				intake.Source = "network-intake/" + member.String() + "/" + record.Key.Kind.String() + "/" + record.Key.Version.String()
			}
		}
		picture.Members = append(picture.Members, intake)
	}
	product, err := adapter.products.LoadAcceptedServiceProduct(ctx, psTenant, psParcel)
	if err != nil {
		return none, false, err
	}
	if product.Declared() {
		picture.ServiceProduct = product.String()
	}
	return picture, true, nil
}
