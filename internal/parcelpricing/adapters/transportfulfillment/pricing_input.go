// Package transportfulfillment 是计价消费运输履约的一侧（ADR-0025）。造计价输入快照的两只编排器都在这里：
// 按包裹的那一只只问小包托运的申报测量与地址要素，SELL 请求点名包裹时直接用它（ADR-0190 决定四）；按发生项的
// 那一只问话的键是运输收费发生项（目录名由此来），经运输履约找到成员包裹之后整段交给前者。按包裹的那一只不放
// adapters/parcelshipment：按发生项那一只组合它，同包才不必为组合导出「从包裹到快照」那一步，实测优先、缺席取
// 申报的取源规则（ADR-0171 决定二）也就只有一个入口。目录名的复核随 ADR-0171 越权风险点 1 归 PP owner。节点
// 实测登记册不在这里实现。
package transportfulfillment

import (
	"context"
	"fmt"
	"time"

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
	missingParcel         = "settlement-accounting: the evaluation request names no parcel"
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

// ParcelResolverDeps 是按包裹那一只的提供方口。Measured 可以为 nil：没有实测读口。
type ParcelResolverDeps struct {
	Declared  psports.DeclaredMeasurementView
	Addresses psports.AddressElementsView
	Measured  ActualMeasurementView
}

// ParcelResolver 按一件已受理包裹造快照：实重尺寸实测优先、缺席取申报，邮编路线取小包托运的寄件到收件。它实现
// PricingInputResolver，键是请求点名的包裹，业务时点取请求的计价基准时点。不 import 提供方 application。
type ParcelResolver struct {
	deps ParcelResolverDeps
}

func NewParcelResolver(deps ParcelResolverDeps) (*ParcelResolver, error) {
	if deps.Declared == nil || deps.Addresses == nil {
		return nil, fmt.Errorf("parcel pricing: pricing input resolver is missing a provider port")
	}
	return &ParcelResolver{deps: deps}, nil
}

// ResolvePricingInput 只认包裹引用。发生项在这里不是钥匙：经发生项找包裹是 Resolver 的事，这一只收到只带发生项的
// 请求就是装错了路，如实答不可得。
func (resolver *ParcelResolver) ResolvePricingInput(
	ctx context.Context,
	query ppports.PricingInputQuery,
) (ppports.PricingInputResolution, error) {
	if !query.Sources.ParcelReferenced() {
		return unavailable(missingParcel), nil
	}
	return resolver.resolve(ctx, query, query.Sources.Parcel, query.BasisAt)
}

// resolve 是两只共用的那一段：从一件包裹到一份快照。businessAt 由调用方给——按发生项时是发生项的业务时间，按包裹
// 时是请求的计价基准时点。请求带了结算币种就随快照进去，评价按它输出。
func (resolver *ParcelResolver) resolve(
	ctx context.Context,
	query ppports.PricingInputQuery,
	object string,
	businessAt time.Time,
) (ppports.PricingInputResolution, error) {
	parcel, err := psdomain.NewDeclaredParcelID(object)
	if err != nil {
		return unavailable(missingNotAParcel), nil
	}
	psTenant, err := psdomain.NewTenantID(query.Tenant.String())
	if err != nil {
		return unavailable(missingNotAParcel), nil
	}

	// 计算目的不参与选源：某一目的是否拒用申报仍未决，这里不预设。SELL·客户费用预估那一格 ADR-0190 决定四答的是
	// 「不拒用」，结果与不看目的相同，所以这里不看。
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
		query.Tenant, query.Scope, subject, route, weight, dimensions, businessAt, facts...)
	if err != nil {
		return ppports.PricingInputResolution{}, err
	}
	if settlement, declared := query.SettlementCurrency(); declared {
		if input, err = input.WithSettlementCurrency(settlement); err != nil {
			return ppports.PricingInputResolution{}, err
		}
	}
	return ppports.PricingInputResolution{Outcome: ppports.PricingInputResolved, Input: input}, nil
}

// ResolverDeps 是按发生项那一只的提供方口：成员走运输履约，其余照原样交给按包裹的那一只。Measured 可以为 nil：
// 没有实测读口。
type ResolverDeps struct {
	Members   tfports.ChargeOccurrenceMemberView
	Declared  psports.DeclaredMeasurementView
	Addresses psports.AddressElementsView
	Measured  ActualMeasurementView
}

// Resolver 按运输收费发生项造快照：找到唯一的成员包裹之后，取数整段交给 ParcelResolver。它实现
// PricingInputResolver。不 import 提供方 application。
type Resolver struct {
	members tfports.ChargeOccurrenceMemberView
	parcels *ParcelResolver
}

func NewResolver(deps ResolverDeps) (*Resolver, error) {
	if deps.Members == nil {
		return nil, fmt.Errorf("parcel pricing: pricing input resolver is missing a provider port")
	}
	parcels, err := NewParcelResolver(ParcelResolverDeps{
		Declared:  deps.Declared,
		Addresses: deps.Addresses,
		Measured:  deps.Measured,
	})
	if err != nil {
		return nil, err
	}
	return &Resolver{members: deps.Members, parcels: parcels}, nil
}

// ResolvePricingInput 只认发生项引用。业务时点取运输履约答的发生项业务时间，不取请求上的计价基准时点：前者是提供方
// 一侧的权威，后者是 SA 转抄的同一刻。
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
	members, found, err := resolver.members.LoadMembers(ctx, key)
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
	return resolver.parcels.resolve(ctx, query, members.Members[0].String(), members.OccurredAt)
}

func (resolver *ParcelResolver) weightAndDimensions(
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

func (resolver *ParcelResolver) postalRoute(
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
