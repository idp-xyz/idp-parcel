package nodeoperations

import (
	"context"
	"fmt"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// ReevaluateOnMeasurementChange 把一次作业实物上的测量变化译成对关联包裹的销售重评。
// 集运单元上的测量、对不上恰好一件包裹的实物，这次不另起请求。
type ReevaluateOnMeasurementChange struct {
	associations noports.CurrentParcelAssociation
	sell         *saapplication.ReevaluateSellOnMeasurementChangeHandler
}

func NewReevaluateOnMeasurementChange(
	associations noports.CurrentParcelAssociation,
	sell *saapplication.ReevaluateSellOnMeasurementChangeHandler,
) (*ReevaluateOnMeasurementChange, error) {
	if associations == nil || sell == nil {
		return nil, fmt.Errorf("settlement accounting: measurement change reevaluation is missing a dependency")
	}
	return &ReevaluateOnMeasurementChange{associations: associations, sell: sell}, nil
}

func (adapter *ReevaluateOnMeasurementChange) HandleChangedMeasurement(
	ctx context.Context,
	changed sainbox.ChangedMeasurement,
) error {
	if changed.ObjectKind != nodomain.HandlingUnitObject.String() {
		return nil
	}
	tenant, err := nodomain.NewTenantID(changed.TenantID)
	if err != nil {
		return err
	}
	unit, err := nodomain.NewHandlingUnitID(changed.ObjectID)
	if err != nil {
		return err
	}
	parcels, err := adapter.associations.ListParcels(ctx, tenant, unit)
	if err != nil {
		return err
	}
	if len(parcels) != 1 {
		return nil
	}
	parcel, err := sadomain.NewParcelReference(parcels[0].String())
	if err != nil {
		return err
	}
	sellTenant, err := sadomain.NewTenantID(changed.TenantID)
	if err != nil {
		return err
	}
	_, err = adapter.sell.Handle(ctx, saapplication.ReevaluateSellOnMeasurementChangeCommand{
		Tenant: sellTenant, Parcel: parcel, SourceID: changed.SourceID,
	})
	return err
}
