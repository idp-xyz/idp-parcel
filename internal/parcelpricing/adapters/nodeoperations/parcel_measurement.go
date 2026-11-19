package nodeoperations

import (
	"context"
	"fmt"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pptransport "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/transportfulfillment"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

const (
	severalMeasurements = "输入不可得：实测多于一条"
	sameOccurredAt      = "输入不可得：两条实测的发生时间相同，定不下"
	severalUnits        = "输入不可得：这件包裹关联着多件作业实物，定不下"
)

// ParcelMeasurementView 按包裹读当前有效实测，译成计价输入用的重量与尺寸。
// 包裹先对上此刻关联的作业实物；没有实物就没有实测。一条测量同时带重量与尺寸，取到当前有效的那一条就两样都交回。
type ParcelMeasurementView struct {
	associations parcelUnits
	measurements stillValidMeasurements
	adoptions    selectionAdoptions
}

type parcelUnits interface {
	ListHandlingUnits(context.Context, nodomain.TenantID, nodomain.ParcelAssociationReference) ([]nodomain.HandlingUnitID, error)
}

type stillValidMeasurements interface {
	ListStillValid(context.Context, nodomain.TenantID, nodomain.MeasurementObjectKind, string) ([]noports.ActualMeasurementRecord, error)
}

type selectionAdoptions interface {
	FindAdoption(context.Context, nodomain.TenantID) (noports.MeasurementSelectionAdoption, bool, error)
}

func NewParcelMeasurementView(
	associations parcelUnits,
	measurements stillValidMeasurements,
	adoptions selectionAdoptions,
) (*ParcelMeasurementView, error) {
	if associations == nil || measurements == nil || adoptions == nil {
		return nil, fmt.Errorf("parcel pricing: actual measurement view is missing a provider port")
	}
	return &ParcelMeasurementView{
		associations: associations,
		measurements: measurements,
		adoptions:    adoptions,
	}, nil
}

func (view *ParcelMeasurementView) LoadActualMeasurement(
	ctx context.Context,
	tenant ppdomain.TenantID,
	object string,
) (pptransport.ActualMeasurementLoad, error) {
	none := pptransport.ActualMeasurementLoad{}
	nodeTenant, err := nodomain.NewTenantID(tenant.String())
	if err != nil {
		return none, err
	}
	parcel, err := nodomain.NewParcelAssociationReference(object)
	if err != nil {
		return none, nil
	}
	units, err := view.associations.ListHandlingUnits(ctx, nodeTenant, parcel)
	if err != nil {
		return none, err
	}
	switch len(units) {
	case 0:
		return none, nil
	case 1:
	default:
		return pptransport.ActualMeasurementLoad{Undetermined: severalUnits}, nil
	}
	records, err := view.measurements.ListStillValid(ctx, nodeTenant, nodomain.HandlingUnitObject, units[0].String())
	if err != nil {
		return none, err
	}
	adoption, adopted, err := view.adoptions.FindAdoption(ctx, nodeTenant)
	if err != nil {
		return none, err
	}
	useForm := adopted && adoption.Form == nodomain.LatestStillValidByOccurredAt
	measurements := make([]nodomain.ActualMeasurement, 0, len(records))
	for _, record := range records {
		measurements = append(measurements, record.Measurement)
	}
	current, answer, cause := nodomain.SelectCurrentMeasurement(measurements, useForm)
	switch answer {
	case nodomain.CurrentMeasurementUndetermined:
		return pptransport.ActualMeasurementLoad{Undetermined: causeText(cause)}, nil
	case nodomain.CurrentMeasurementOne:
	default:
		return none, nil
	}
	weight, ok := translateWeight(current)
	if !ok {
		value, unit := current.Weight()
		return pptransport.ActualMeasurementLoad{
			Undetermined: "parcel-pricing: measured weight unit is outside the pricing unit table: " + unit + " " + value,
		}, nil
	}
	dimensions, ok := translateDimensions(current)
	if !ok {
		_, _, _, unit := current.Dimensions()
		return pptransport.ActualMeasurementLoad{
			Undetermined: "parcel-pricing: measured dimension unit is outside the pricing unit table: " + unit,
		}, nil
	}
	fact, err := measurementFact(object, current.Source().SourceID())
	if err != nil {
		return none, err
	}
	return pptransport.ActualMeasurementLoad{Found: true, Weight: weight, Dimensions: dimensions, Fact: fact}, nil
}

func causeText(cause nodomain.CurrentMeasurementCause) string {
	switch cause {
	case nodomain.SameOccurredAt:
		return sameOccurredAt
	default:
		return severalMeasurements
	}
}

// measurementFact 造一条实测事实引用。版本是那次测量的来源身份：重量与尺寸出自同一条测量，一条事实就够。
func measurementFact(parcel, sourceID string) (ppdomain.VersionedFactReference, error) {
	reference, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactActualMeasurement, parcel, sourceID)
	if err != nil {
		return ppdomain.VersionedFactReference{}, err
	}
	return ppdomain.NewVersionedFactReference(reference)
}

func translateWeight(measurement nodomain.ActualMeasurement) (ppdomain.Weight, bool) {
	value, unitName := measurement.Weight()
	unit, err := ppdomain.NewWeightUnit(unitName)
	if err != nil {
		return ppdomain.Weight{}, false
	}
	weight, err := ppdomain.NewWeightFromString(value, unit)
	if err != nil {
		return ppdomain.Weight{}, false
	}
	return weight, true
}

var _ pptransport.ActualMeasurementView = (*ParcelMeasurementView)(nil)

func translateDimensions(measurement nodomain.ActualMeasurement) (ppdomain.Dimensions, bool) {
	length, width, height, unitName := measurement.Dimensions()
	unit, err := ppdomain.NewLengthUnit(unitName)
	if err != nil {
		return ppdomain.Dimensions{}, false
	}
	dimensions, err := ppdomain.NewDimensionsFromStrings(length, width, height, unit)
	if err != nil {
		return ppdomain.Dimensions{}, false
	}
	return dimensions, true
}
