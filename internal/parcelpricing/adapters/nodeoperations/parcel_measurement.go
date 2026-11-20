package nodeoperations

import (
	"context"
	"errors"
	"fmt"
	"time"

	noapplication "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
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

// ParcelMeasurementView 把节点作业按包裹答的当前有效实测译成计价的重量与外廓。
// 取哪一条在 CurrentParcelMeasurementQuery；这里只译单位与数值，并把定不下的三种来由映成计价已经在用的那三句。
type ParcelMeasurementView struct {
	query *noapplication.CurrentParcelMeasurementQuery
}

func NewParcelMeasurementView(query *noapplication.CurrentParcelMeasurementQuery) (*ParcelMeasurementView, error) {
	if query == nil {
		return nil, fmt.Errorf("parcel pricing: actual measurement view is missing the current measurement query")
	}
	return &ParcelMeasurementView{query: query}, nil
}

func (view *ParcelMeasurementView) LoadActualMeasurement(
	ctx context.Context,
	tenant ppdomain.TenantID,
	object string,
) (pptransport.ActualMeasurementLoad, error) {
	return view.loadActualMeasurement(ctx, tenant, object, nil)
}

// LoadActualMeasurementAsOf 与 LoadActualMeasurement 走同一段翻译。发生时间上界交给 CurrentParcelMeasurementQuery。
func (view *ParcelMeasurementView) LoadActualMeasurementAsOf(
	ctx context.Context,
	tenant ppdomain.TenantID,
	object string,
	asOf time.Time,
) (pptransport.ActualMeasurementLoad, error) {
	return view.loadActualMeasurement(ctx, tenant, object, &asOf)
}

func (view *ParcelMeasurementView) loadActualMeasurement(
	ctx context.Context,
	tenant ppdomain.TenantID,
	object string,
	asOf *time.Time,
) (pptransport.ActualMeasurementLoad, error) {
	none := pptransport.ActualMeasurementLoad{}
	nodeTenant, err := nodomain.NewTenantID(tenant.String())
	if err != nil {
		return none, err
	}
	// 空白标识指不到任何一件包裹，答「没有实测」与读口查无关联时一致；别的错不在此列，照原样冒上去。
	parcel, err := nodomain.NewParcelAssociationReference(object)
	if errors.Is(err, nodomain.ErrBlankValue) {
		return none, nil
	}
	if err != nil {
		return none, err
	}
	var result noapplication.CurrentParcelMeasurement
	if asOf == nil {
		result, err = view.query.Current(ctx, nodeTenant, parcel)
	} else {
		result, err = view.query.CurrentAsOf(ctx, nodeTenant, parcel, *asOf)
	}
	if err != nil {
		return none, err
	}
	switch result.Answer() {
	case noapplication.UndeterminedCurrentParcelMeasurement:
		text, err := causeText(result.Cause())
		if err != nil {
			return none, err
		}
		return pptransport.ActualMeasurementLoad{Undetermined: text}, nil
	case noapplication.OneCurrentParcelMeasurement:
	case noapplication.NoCurrentParcelMeasurement:
		return none, nil
	default:
		return none, fmt.Errorf("parcel pricing: current parcel measurement answer %s", result.Answer())
	}
	current := result.Measurement()
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

func causeText(cause noapplication.CurrentParcelMeasurementCause) (string, error) {
	switch cause {
	case noapplication.SeveralAssociatedHandlingUnits:
		return severalUnits, nil
	case noapplication.SeveralStillValidWithoutAdoption:
		return severalMeasurements, nil
	case noapplication.OccurredAtTied:
		return sameOccurredAt, nil
	default:
		return "", fmt.Errorf("parcel pricing: current parcel measurement cause %s", cause)
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
	gross := measurement.Weight()
	unit, err := ppdomain.NewWeightUnit(gross.Unit())
	if err != nil {
		return ppdomain.Weight{}, weightUnitOutsideTable + "：" + gross.Unit()
	}
	weight, err := ppdomain.NewWeightFromString(gross.Value(), unit)
	if err != nil {
		return ppdomain.Weight{}, weightValueUnreadable
	}
	return weight, ""
}

var _ pptransport.ActualMeasurementView = (*ParcelMeasurementView)(nil)

// translateDimensions 与 translateWeight 同形：尺寸三边共用一个单位，单位与数值各答各的原因。
func translateDimensions(measurement nodomain.ActualMeasurement) (ppdomain.Dimensions, string) {
	outer := measurement.Dimensions()
	unit, err := ppdomain.NewLengthUnit(outer.Unit())
	if err != nil {
		return ppdomain.Dimensions{}, dimensionUnitOutsideTable + "：" + outer.Unit()
	}
	dimensions, err := ppdomain.NewDimensionsFromStrings(outer.Length(), outer.Width(), outer.Height(), unit)
	if err != nil {
		return ppdomain.Dimensions{}, dimensionValueUnreadable
	}
	return dimensions, ""
}
