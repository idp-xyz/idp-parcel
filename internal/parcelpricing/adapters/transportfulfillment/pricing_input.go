// Package transportfulfillment 是计价消费运输履约的一侧（ADR-0025）。这一只编排器同时问
// 小包托运的申报测量与地址要素：问话的键是运输收费发生项，所以目录落在 transportfulfillment，
// 不另拆三只只为通过「adapters/<提供方>」那条路径。节点实测登记册不在这里实现。
package transportfulfillment

import (
	"context"
	"fmt"

	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

const (
	missingMembers        = "transport-fulfillment: charge occurrence members were not found"
	missingMemberCount    = "transport-fulfillment: charge occurrence has no carried object"
	missingSingleMember   = "transport-fulfillment: charge occurrence has more than one carried object"
	missingNotAParcel     = "parcel-shipment: carried object is not a declared parcel (consolidation unit is not an evaluation subject)"
	missingDeclaredWeight = "parcel-shipment: declared weight is absent"
	missingWeightUnit     = "parcel-shipment: declared weight unit is outside the pricing unit table"
	missingLengthUnit     = "parcel-shipment: declared dimension unit is outside the pricing unit table"
	missingDestination    = "parcel-shipment: destination postal code is absent"
	missingBusinessTime   = "transport-fulfillment: charge occurrence has no business time"
)

// ActualMeasurement 是节点仍有效实测交来的原始量。本卡不造那本登记册；读口缺席时解析器改走申报。
type ActualMeasurement struct {
	Weight     ppdomain.Weight
	Dimensions *ppdomain.Dimensions
	Fact       ppdomain.VersionedFactReference
}

// ActualMeasurementView 是将来 node-operations 实测只读口在消费侧的形状。今天没有实现。
// found=false 表示这个对象没有实测，解析器改走申报。多于一条实测的选择不在这里。
type ActualMeasurementView interface {
	LoadActualMeasurement(
		ctx context.Context,
		tenant ppdomain.TenantID,
		object string,
	) (ActualMeasurement, bool, error)
}

// ResolverDeps 是三只提供方口。Measured 可以为 nil：没有实测读口。
type ResolverDeps struct {
	Members   tfports.ChargeOccurrenceMemberView
	Declared  psports.DeclaredMeasurementView
	Addresses psports.AddressElementsView
	Measured  ActualMeasurementView
}

// Resolver 实现 PricingInputResolver。它不 import 提供方 application。
type Resolver struct {
	deps ResolverDeps
}

func NewResolver(deps ResolverDeps) (*Resolver, error) {
	if deps.Members == nil || deps.Declared == nil || deps.Addresses == nil {
		return nil, fmt.Errorf("parcel pricing: pricing input resolver is missing a provider port")
	}
	return &Resolver{deps: deps}, nil
}

func (resolver *Resolver) ResolvePricingInput(
	ctx context.Context,
	query ppports.PricingInputQuery,
) (ppports.PricingInputResolution, error) {
	if !query.Sources.OccurrenceReferenced() {
		return unavailable(missingMembers), nil
	}
	key, err := occurrenceKey(query)
	if err != nil {
		return unavailable(missingMembers), nil
	}
	members, found, err := resolver.deps.Members.LoadMembers(ctx, key)
	if err != nil {
		return ppports.PricingInputResolution{}, err
	}
	if !found {
		return unavailable(missingMembers), nil
	}
	if members.OccurredAt.IsZero() {
		return unavailable(missingBusinessTime), nil
	}
	switch len(members.Members) {
	case 0:
		return unavailable(missingMemberCount), nil
	case 1:
	default:
		return unavailable(missingSingleMember), nil
	}
	object := members.Members[0].String()
	parcel, err := psdomain.NewDeclaredParcelID(object)
	if err != nil {
		return unavailable(missingNotAParcel), nil
	}
	psTenant, err := psdomain.NewTenantID(query.Tenant.String())
	if err != nil {
		return unavailable(missingNotAParcel), nil
	}

	// 计算目的不参与选源：某一目的是否拒用申报仍未决，这里不预设。
	_ = query.Purpose
	weight, dimensions, facts, missing, err := resolver.weightAndDimensions(ctx, query.Tenant, psTenant, parcel, object)
	if err != nil {
		return ppports.PricingInputResolution{}, err
	}
	if names(missing, missingNotAParcel) {
		return unavailable(missingNotAParcel), nil
	}
	route, addressFact, routeMissing, err := resolver.postalRoute(ctx, psTenant, parcel)
	if err != nil {
		return ppports.PricingInputResolution{}, err
	}
	missing = append(missing, routeMissing...)
	if len(missing) > 0 {
		return unavailable(missing...), nil
	}
	facts = append(facts, addressFact)

	subject, err := packageSubject(object)
	if err != nil {
		return unavailable(missingNotAParcel), nil
	}
	input, err := ppdomain.NewPostalPricingInputSnapshot(
		query.Tenant, query.Scope, subject, route, weight, dimensions, members.OccurredAt, facts...)
	if err != nil {
		return ppports.PricingInputResolution{}, err
	}
	return ppports.PricingInputResolution{Outcome: ppports.PricingInputResolved, Input: input}, nil
}

func (resolver *Resolver) weightAndDimensions(
	ctx context.Context,
	tenant ppdomain.TenantID,
	psTenant psdomain.TenantID,
	parcel psdomain.DeclaredParcelID,
	object string,
) (ppdomain.Weight, *ppdomain.Dimensions, []ppdomain.VersionedFactReference, []string, error) {
	if resolver.deps.Measured != nil {
		measured, found, err := resolver.deps.Measured.LoadActualMeasurement(ctx, tenant, object)
		if err != nil {
			return ppdomain.Weight{}, nil, nil, nil, err
		}
		if found {
			// 实测在场时申报不得顶替，尺寸缺了也不回退到申报。
			return measured.Weight, measured.Dimensions, []ppdomain.VersionedFactReference{measured.Fact}, nil, nil
		}
	}
	resolution, err := resolver.deps.Declared.LoadDeclaredMeasurement(ctx, psTenant, parcel)
	if err != nil {
		return ppdomain.Weight{}, nil, nil, nil, err
	}
	switch resolution.Outcome() {
	case psdomain.NoDeclaredMeasurement:
		return ppdomain.Weight{}, nil, nil, []string{missingNotAParcel}, nil
	case psdomain.DeclaredMeasurementUndetermined, psdomain.DeclaredMeasurementNotDeclared:
		return ppdomain.Weight{}, nil, nil, []string{missingDeclaredWeight}, nil
	}
	measurement, present := resolution.Measurement()
	if !present {
		return ppdomain.Weight{}, nil, nil, []string{missingDeclaredWeight}, nil
	}
	weight, ok := translateWeight(measurement.Weight())
	if !ok {
		return ppdomain.Weight{}, nil, nil, []string{missingWeightUnit + ": " + measurement.Weight().Unit().String()}, nil
	}
	var dimensions *ppdomain.Dimensions
	if declared, hasDimensions := measurement.Dimensions(); hasDimensions {
		translated, ok := translateDimensions(declared)
		if !ok {
			return ppdomain.Weight{}, nil, nil, []string{missingLengthUnit + ": " + declared.Unit().String()}, nil
		}
		dimensions = &translated
	}
	fact, err := declaredFact(parcel.String(), resolution)
	if err != nil {
		return ppdomain.Weight{}, nil, nil, nil, err
	}
	return weight, dimensions, []ppdomain.VersionedFactReference{fact}, nil, nil
}

func (resolver *Resolver) postalRoute(
	ctx context.Context,
	tenant psdomain.TenantID,
	parcel psdomain.DeclaredParcelID,
) (ppdomain.PostalRoute, ppdomain.VersionedFactReference, []string, error) {
	none := ppdomain.VersionedFactReference{}
	elements, err := resolver.deps.Addresses.LoadAddressElements(ctx, tenant, parcel)
	if err != nil {
		return ppdomain.PostalRoute{}, none, nil, err
	}
	destination := elements.Destination()
	if destination.Outcome() == psdomain.NoAddressElements {
		return ppdomain.PostalRoute{}, none, []string{missingNotAParcel}, nil
	}
	values, present := destination.Elements()
	postal, declared := "", false
	if present {
		postal, declared = values.PostalCode()
	}
	if !declared || postal == "" {
		return ppdomain.PostalRoute{}, none, []string{missingDestination}, nil
	}
	origin := ""
	if originValues, originPresent := elements.Origin().Elements(); originPresent {
		if code, originDeclared := originValues.PostalCode(); originDeclared {
			origin = code
		}
	}
	route, err := ppdomain.NewPostalRoute(origin, postal)
	if err != nil && origin != "" {
		origin = ""
		route, err = ppdomain.NewPostalRoute(origin, postal)
	}
	if err != nil {
		return ppdomain.PostalRoute{}, none, []string{missingDestination}, nil
	}
	fact, err := addressFact(parcel.String(), destination)
	if err != nil {
		return ppdomain.PostalRoute{}, none, nil, err
	}
	return route, fact, nil, nil
}

func occurrenceKey(query ppports.PricingInputQuery) (tfports.ChargeOccurrenceKey, error) {
	tenant, err := tfdomain.NewTenantID(query.Tenant.String())
	if err != nil {
		return tfports.ChargeOccurrenceKey{}, err
	}
	occurrence, err := tfdomain.NewChargeOccurrenceReference(query.Sources.Occurrence)
	if err != nil {
		return tfports.ChargeOccurrenceKey{}, err
	}
	validity, err := tfdomain.NewOccurrenceValidityVersion(query.Sources.OccurrenceVersion)
	if err != nil {
		return tfports.ChargeOccurrenceKey{}, err
	}
	return tfports.ChargeOccurrenceKey{TenantID: tenant, Occurrence: occurrence, Validity: validity}, nil
}

func packageSubject(object string) (ppdomain.EvaluationSubject, error) {
	packageID, err := ppdomain.NewPackageID(object)
	if err != nil {
		return ppdomain.EvaluationSubject{}, err
	}
	return ppdomain.NewAcceptedPackageSubject(packageID)
}

func addressFact(parcel string, resolution psdomain.AddressElementsResolution) (ppdomain.VersionedFactReference, error) {
	version := "address"
	if anchor, anchored := resolution.Anchor(); anchored {
		if adopted, ok := anchor.AdoptedVersion(); ok {
			version = "address@" + adopted.String()
		} else if anchor.OnAcceptanceBaseline() {
			version = "address@baseline"
		}
	}
	reference, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactAddressElements, parcel, version)
	if err != nil {
		return ppdomain.VersionedFactReference{}, err
	}
	return ppdomain.NewVersionedFactReference(reference)
}

func names(missing []string, wanted string) bool {
	for _, item := range missing {
		if item == wanted {
			return true
		}
	}
	return false
}

func declaredFact(parcel string, resolution psdomain.DeclaredMeasurementResolution) (ppdomain.VersionedFactReference, error) {
	version := "declaration"
	if anchor, anchored := resolution.Anchor(); anchored {
		if adopted, ok := anchor.AdoptedVersion(); ok {
			version = "declaration@" + adopted.String()
		} else if anchor.OnAcceptanceBaseline() {
			version = "declaration@baseline"
		}
	}
	reference, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactDeclaredMeasurement, parcel, version)
	if err != nil {
		return ppdomain.VersionedFactReference{}, err
	}
	return ppdomain.NewVersionedFactReference(reference)
}

func translateWeight(weight psdomain.DeclaredWeight) (ppdomain.Weight, bool) {
	unit, ok := weightUnit(weight.Unit().String())
	if !ok {
		return ppdomain.Weight{}, false
	}
	value, err := ppdomain.ParseDecimal(weight.Value().String())
	if err != nil {
		return ppdomain.Weight{}, false
	}
	translated, err := ppdomain.NewWeight(value, unit)
	if err != nil {
		return ppdomain.Weight{}, false
	}
	return translated, true
}

func translateDimensions(dimensions psdomain.DeclaredDimensions) (ppdomain.Dimensions, bool) {
	unit, ok := lengthUnit(dimensions.Unit().String())
	if !ok {
		return ppdomain.Dimensions{}, false
	}
	length, err := ppdomain.ParseDecimal(dimensions.Length().String())
	if err != nil {
		return ppdomain.Dimensions{}, false
	}
	width, err := ppdomain.ParseDecimal(dimensions.Width().String())
	if err != nil {
		return ppdomain.Dimensions{}, false
	}
	height, err := ppdomain.ParseDecimal(dimensions.Height().String())
	if err != nil {
		return ppdomain.Dimensions{}, false
	}
	translated, err := ppdomain.NewDimensions(length, width, height, unit)
	if err != nil {
		return ppdomain.Dimensions{}, false
	}
	return translated, true
}

func weightUnit(raw string) (ppdomain.WeightUnit, bool) {
	switch raw {
	case "KG", "G", "LB", "OZ":
		return ppdomain.WeightUnit(raw), true
	default:
		return "", false
	}
}

func lengthUnit(raw string) (ppdomain.LengthUnit, bool) {
	switch raw {
	case "CM", "IN":
		return ppdomain.LengthUnit(raw), true
	default:
		return "", false
	}
}

func unavailable(missing ...string) ppports.PricingInputResolution {
	return ppports.PricingInputResolution{
		Outcome: ppports.PricingInputUnavailable,
		Missing: append([]string(nil), missing...),
	}
}
