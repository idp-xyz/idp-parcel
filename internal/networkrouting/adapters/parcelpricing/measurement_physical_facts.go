package parcelpricing

import (
	"context"
	"fmt"
	"time"

	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	pptransport "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/transportfulfillment"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// actualMeasurementAsOf 是计价按包裹读当前有效实测的带时点读法。取哪一条、单位怎么译只在那一只上，
// 路由适配器不读节点作业的登记册，也不把那套判断再写一遍。
type actualMeasurementAsOf interface {
	LoadActualMeasurementAsOf(
		ctx context.Context,
		tenant ppdomain.TenantID,
		object string,
		asOf time.Time,
	) (pptransport.ActualMeasurementLoad, error)
}

// MeasurementPhysicalFacts 把那只读法的三格译成路由的物理事实。包裹标识用判断键的声明包裹：
// 与计价传入的 object 是同一个值，路由出价与计价取源才是同一件包裹。
type MeasurementPhysicalFacts struct {
	measurements actualMeasurementAsOf
}

func NewMeasurementPhysicalFacts(measurements actualMeasurementAsOf) (*MeasurementPhysicalFacts, error) {
	if measurements == nil {
		return nil, fmt.Errorf("network routing: physical parcel facts need the actual measurement read")
	}
	return &MeasurementPhysicalFacts{measurements: measurements}, nil
}

var _ PhysicalParcelFactsSource = (*MeasurementPhysicalFacts)(nil)

func (facts *MeasurementPhysicalFacts) PhysicalParcelFactsFor(
	ctx context.Context,
	key nrdomain.InitialRouteJudgmentKey,
	asOf time.Time,
) (PhysicalParcelFacts, error) {
	tenant, err := ppdomain.NewTenantID(key.TenantID.String())
	if err != nil {
		return PhysicalParcelFacts{}, fmt.Errorf("network routing: physical parcel facts tenant: %w", err)
	}
	load, err := facts.measurements.LoadActualMeasurementAsOf(ctx, tenant, key.DeclaredParcelID.String(), asOf)
	if err != nil {
		return PhysicalParcelFacts{}, err
	}
	if load.Found {
		// 取得了却组不成一份物理事实时不折成「没取得」：那一折会退回客户声明，把读口的坏结果当成没称过。
		return ObtainedPhysicalFacts(load.Weight, load.Dimensions, []ppdomain.VersionedFactReference{load.Fact})
	}
	if load.Undetermined != "" {
		return UnresolvedPhysicalFacts(), nil
	}
	return NoPhysicalFactsObtained(), nil
}
