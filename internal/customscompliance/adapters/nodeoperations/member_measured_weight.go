// Package nodeoperations 把节点作业的当前有效实测译成关务读口。取哪一条只问 CurrentParcelMeasurementQuery，这里不另判。
package nodeoperations

import (
	"context"
	"errors"
	"fmt"

	noapplication "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// MemberMeasuredWeights 实现 ports.MemberMeasuredWeightView。数值与单位原样交，不换算、不改写数字。
type MemberMeasuredWeights struct {
	query *noapplication.CurrentParcelMeasurementQuery
}

func NewMemberMeasuredWeights(query *noapplication.CurrentParcelMeasurementQuery) (*MemberMeasuredWeights, error) {
	if query == nil {
		return nil, errors.New("customs compliance: member measured weight view is unwired")
	}
	return &MemberMeasuredWeights{query: query}, nil
}

func (view *MemberMeasuredWeights) LoadMemberMeasuredWeight(
	ctx context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelReference,
) (ports.MeasuredWeightReading, error) {
	nodeTenant, err := nodomain.NewTenantID(tenant.String())
	if err != nil {
		return ports.MeasuredWeightReading{}, err
	}
	// 关务的成员包裹引用与节点作业的包裹关联引用是同一串字面，两边都只要求非空白。
	association, err := nodomain.NewParcelAssociationReference(parcel.String())
	if err != nil {
		return ports.MeasuredWeightReading{}, err
	}
	current, err := view.query.Current(ctx, nodeTenant, association)
	if err != nil {
		return ports.MeasuredWeightReading{}, err
	}
	switch current.Answer() {
	case noapplication.NoCurrentParcelMeasurement:
		return ports.MeasuredWeightReading{Answer: ports.MeasuredWeightNone}, nil
	case noapplication.UndeterminedCurrentParcelMeasurement:
		return ports.MeasuredWeightReading{
			Answer: ports.MeasuredWeightUndetermined,
			Cause:  current.Cause().String(),
		}, nil
	case noapplication.OneCurrentParcelMeasurement:
		measurement := current.Measurement()
		return ports.MeasuredWeightReading{
			Answer:            ports.MeasuredWeightOne,
			GrossWeight:       measurement.Weight().Value(),
			GrossWeightUnit:   measurement.Weight().Unit(),
			MeasurementSource: measurement.Source().SourceID(),
			MeasuredAt:        measurement.Source().OccurredAt(),
		}, nil
	default:
		return ports.MeasuredWeightReading{}, fmt.Errorf("customs compliance: current measurement answer %s", current.Answer())
	}
}
