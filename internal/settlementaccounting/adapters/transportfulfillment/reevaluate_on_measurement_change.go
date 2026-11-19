package transportfulfillment

import (
	"context"
	"fmt"

	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	sanodeops "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/nodeoperations"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ReevaluateBuyForMeasuredParcel 把一次作业实物上的测量变化译成对载有关联包裹的收费发生项的采购重评。
// 对不上包裹的变化（见 nodeoperations 适配器的 MeasuredParcel）这次不另起请求。
type ReevaluateBuyForMeasuredParcel struct {
	associations noports.CurrentParcelAssociation
	occurrences  tfports.ChargeOccurrencesCarryingObject
	buy          *saapplication.ReevaluateBuyOnMeasurementChangeHandler
}

func NewReevaluateBuyForMeasuredParcel(
	associations noports.CurrentParcelAssociation,
	occurrences tfports.ChargeOccurrencesCarryingObject,
	buy *saapplication.ReevaluateBuyOnMeasurementChangeHandler,
) (*ReevaluateBuyForMeasuredParcel, error) {
	if associations == nil || occurrences == nil || buy == nil {
		return nil, fmt.Errorf("settlement accounting: buy reevaluation for a measured parcel is missing a dependency")
	}
	return &ReevaluateBuyForMeasuredParcel{associations: associations, occurrences: occurrences, buy: buy}, nil
}

func (adapter *ReevaluateBuyForMeasuredParcel) HandleChangedMeasurement(
	ctx context.Context,
	changed sainbox.ChangedMeasurement,
) error {
	measured, found, err := sanodeops.MeasuredParcel(ctx, adapter.associations, changed)
	if err != nil || !found {
		return err
	}
	carried, err := tfdomain.NewCarriedObjectReference(measured.String())
	if err != nil {
		return err
	}
	transportTenant, err := tfdomain.NewTenantID(changed.TenantID)
	if err != nil {
		return err
	}
	buyTenant, err := sadomain.NewTenantID(changed.TenantID)
	if err != nil {
		return err
	}
	keys, err := adapter.occurrences.ListCurrentCarrying(ctx, transportTenant, carried)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if _, err := adapter.buy.Handle(ctx, saapplication.ReevaluateBuyOnMeasurementChangeCommand{
			Tenant:            buyTenant,
			OccurrenceID:      key.Occurrence.String(),
			OccurrenceVersion: key.Validity.String(),
			SourceID:          changed.SourceID,
		}); err != nil {
			return err
		}
	}
	return nil
}
