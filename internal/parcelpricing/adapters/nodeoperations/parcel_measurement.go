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

	// 译不出重量或尺寸有两条不同的排查路：单位不在计价单位表里（去查登记的单位符号），与单位在表内而数值计价读不成
	// 重量或外廓（去查登记的数字）。节点登记册不核单位表，也不核计价能表示的位数，两种都登得进去。
	weightUnitOutsideTable    = "输入不可得：实测重量的单位不在计价单位表里"
	weightValueUnreadable     = "输入不可得：实测重量的数值不成立，计价读不成一个重量"
	dimensionUnitOutsideTable = "输入不可得：实测尺寸的单位不在计价单位表里"
	dimensionValueUnreadable  = "输入不可得：实测尺寸的数值不成立，计价读不成一个外廓"
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
	weight, undetermined := translateWeight(current)
	if undetermined != "" {
		return pptransport.ActualMeasurementLoad{Undetermined: undetermined}, nil
	}
	dimensions, undetermined := translateDimensions(current)
	if undetermined != "" {
		return pptransport.ActualMeasurementLoad{Undetermined: undetermined}, nil
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

// translateWeight 把实测的重量译成计价的重量；译不出来时第二个返回值是照实的原因文字，译得出来时为空。
func translateWeight(measurement nodomain.ActualMeasurement) (ppdomain.Weight, string) {
	value, unitName := measurement.Weight()
	unit, err := ppdomain.NewWeightUnit(unitName)
	if err != nil {
		return ppdomain.Weight{}, weightUnitOutsideTable + "：" + unitName
	}
	weight, err := ppdomain.NewWeightFromString(value, unit)
	if err != nil {
		return ppdomain.Weight{}, weightValueUnreadable
	}
	return weight, ""
}

var _ pptransport.ActualMeasurementView = (*ParcelMeasurementView)(nil)

// translateDimensions 与 translateWeight 同形：尺寸三边共用一个单位，单位与数值各答各的原因。
func translateDimensions(measurement nodomain.ActualMeasurement) (ppdomain.Dimensions, string) {
	length, width, height, unitName := measurement.Dimensions()
	unit, err := ppdomain.NewLengthUnit(unitName)
	if err != nil {
		return ppdomain.Dimensions{}, dimensionUnitOutsideTable + "：" + unitName
	}
	dimensions, err := ppdomain.NewDimensionsFromStrings(length, width, height, unit)
	if err != nil {
		return ppdomain.Dimensions{}, dimensionValueUnreadable
	}
	return dimensions, ""
}
