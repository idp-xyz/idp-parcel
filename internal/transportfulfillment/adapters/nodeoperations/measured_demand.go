package nodeoperations

import (
	"context"
	"fmt"

	noapplication "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// MeasuredDemands 实现 tfports.MeasuredDemandSource。取哪一条只问 CurrentParcelMeasurementQuery，数值与单位原样交，不换算。
type MeasuredDemands struct {
	query *noapplication.CurrentParcelMeasurementQuery
}

func NewMeasuredDemands(query *noapplication.CurrentParcelMeasurementQuery) (*MeasuredDemands, error) {
	if query == nil {
		return nil, fmt.Errorf("transport fulfillment nodeoperations adapter: current parcel measurement query is nil")
	}
	return &MeasuredDemands{query: query}, nil
}

func (source *MeasuredDemands) LoadMeasuredDemand(
	ctx context.Context,
	tenant tfdomain.TenantID,
	object tfdomain.CarriedObjectReference,
) (tfports.MeasuredDemandReading, error) {
	nodeTenant, err := nodomain.NewTenantID(tenant.String())
	if err != nil {
		return tfports.MeasuredDemandReading{}, fmt.Errorf("%w: tenant: %v", tfports.ErrMeasuredDemandUntranslatable, err)
	}
	parcel, err := nodomain.NewParcelAssociationReference(object.String())
	if err != nil {
		return tfports.MeasuredDemandReading{}, fmt.Errorf("%w: parcel: %v", tfports.ErrMeasuredDemandUntranslatable, err)
	}
	current, err := source.query.Current(ctx, nodeTenant, parcel)
	if err != nil {
		return tfports.MeasuredDemandReading{}, err
	}
	switch current.Answer() {
	case noapplication.NoCurrentParcelMeasurement:
		return tfports.MeasuredDemandReading{Answer: tfports.MeasuredDemandNone}, nil
	case noapplication.UndeterminedCurrentParcelMeasurement:
		return tfports.MeasuredDemandReading{Answer: tfports.MeasuredDemandUndetermined}, nil
	case noapplication.OneCurrentParcelMeasurement:
		return measuredReadingOf(current.Measurement())
	default:
		return tfports.MeasuredDemandReading{}, fmt.Errorf(
			"transport fulfillment nodeoperations adapter: current measurement answered %s", current.Answer())
	}
}

// measuredReadingOf 把一条实测译成需求量快照。重量、三边与单位照抄；体积由运输准备自己的 MeasuredDemand 用三边乘出。
// 译不成包住不重投的哨兵：缺的重量或边重问不会长出来。
func measuredReadingOf(measurement nodomain.ActualMeasurement) (tfports.MeasuredDemandReading, error) {
	weight := measurement.Weight()
	dimensions := measurement.Dimensions()
	snapshot, err := tfdomain.MeasuredDemand(tfdomain.MeasuredDemandSpec{
		Anchor:        measurement.Source().SourceID(),
		WeightValue:   weight.Value(),
		WeightUnit:    weight.Unit(),
		Length:        dimensions.Length(),
		Width:         dimensions.Width(),
		Height:        dimensions.Height(),
		DimensionUnit: dimensions.Unit(),
	})
	if err != nil {
		return tfports.MeasuredDemandReading{}, fmt.Errorf("%w: %v", tfports.ErrMeasuredDemandUntranslatable, err)
	}
	return tfports.MeasuredDemandReading{
		Answer:     tfports.MeasuredDemandOne,
		Snapshot:   snapshot,
		OccurredAt: measurement.Source().OccurredAt(),
	}, nil
}
