package nodeoperations

import (
	"context"
	"fmt"

	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// ReevaluateOnMeasurementChange 把一次作业实物上的测量变化译成对关联包裹的销售重评。
// 对不上包裹的变化（见 MeasuredParcel）这次不另起请求。
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
	measured, found, err := MeasuredParcel(ctx, adapter.associations, changed)
	if err != nil || !found {
		return err
	}
	parcel, err := sadomain.NewParcelReference(measured.String())
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
