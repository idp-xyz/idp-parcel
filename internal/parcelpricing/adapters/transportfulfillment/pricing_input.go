// Package transportfulfillment 是计价消费运输履约的一侧（ADR-0025）。造计价输入快照的两只编排器都在这里：
// 按包裹的那一只只问小包托运的申报测量与地址要素，SELL 请求点名包裹时直接用它（ADR-0190 决定四）；按发生项的
// 那一只问话的键是运输收费发生项（目录名由此来）。成员恰为一件时整段交给前者；多于一件且在用采购卡是逐委托时，
// 用同一取源拼一份聚合输入，主体是发生项本身。按包裹的那一只不放 adapters/parcelshipment：按发生项那一只组合它，
// 同包才不必为组合导出「从包裹到快照」那一步，实测优先、缺席取申报的取源规则（ADR-0171 决定二）也就只有一个入口。
// 目录名的复核随 ADR-0171 越权风险点 1 归 PP owner。节点实测登记册与当前有效实测的派生在 adapters/nodeoperations，
// 这一包只消费它答好的重量与尺寸。
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
	missingWeightValue    = "parcel-shipment: declared weight value cannot be read as a pricing weight"
	missingLengthValue    = "parcel-shipment: declared dimension values cannot be read as pricing dimensions"
	missingDestination    = "parcel-shipment: destination postal code is absent"
	missingBusinessTime   = "transport-fulfillment: charge occurrence has no business time"
	missingParcel         = "settlement-accounting: the evaluation request names no parcel"
)

// ActualMeasurementLoad 是按包裹问来的当前有效实测。节点的一条测量同时带重量与尺寸，所以 Found 为真时两样都在，
// 解析器整份用它，申报一格都不补；Found 为假时整份走申报。Undetermined 非空时整份输入不可得，文字就是缺的那一件。
type ActualMeasurementLoad struct {
	Found        bool
	Undetermined string
	Weight       ppdomain.Weight
	Dimensions   ppdomain.Dimensions
	Fact         ppdomain.VersionedFactReference
}

// ActualMeasurementView 是 node-operations 当前有效实测在消费侧的形状。
// 多于一条、发生时间并列、一件包裹对上多件实物，都由读口收成 Undetermined，这里不再挑选。
type ActualMeasurementView interface {
	LoadActualMeasurement(
		ctx context.Context,
		tenant ppdomain.TenantID,
		object string,
	) (ActualMeasurementLoad, error)
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
	if query.Sources.ShipmentReferenced() {
		return resolver.resolveShipment(ctx, query)
	}
	if !query.Sources.ParcelReferenced() {
		return unavailable(missingParcel), nil
	}
	return resolver.resolve(ctx, query, query.Sources.Parcel, query.BasisAt)
}

// resolveShipment 按请求钉下的成员清单逐件取实重与尺寸：有实测用实测，否则用申报，并在这一件上标明取的是哪一种。
// 邮编路线取这些件上已经有的那一套；一件都没有，输入不可得，不另造一条路线，也不编一份件清单。
func (resolver *ParcelResolver) resolveShipment(
	ctx context.Context,
	query ppports.PricingInputQuery,
) (ppports.PricingInputResolution, error) {
	psTenant, err := psdomain.NewTenantID(query.Tenant.String())
	if err != nil {
		return unavailable(missingNotAParcel), nil
	}
	pieces := make([]ppdomain.MemberPiece, 0, len(query.Sources.Members))
	ids := make([]ppdomain.PackageID, 0, len(query.Sources.Members))
	var facts []ppdomain.VersionedFactReference
	var missing []string
	var route ppdomain.PostalRoute
	var routeFact ppdomain.VersionedFactReference
	haveRoute := false
	for _, member := range query.Sources.Members {
		parcel, err := psdomain.NewDeclaredParcelID(member)
		if err != nil {
			return unavailable(missingNotAParcel), nil
		}
		packageID, err := ppdomain.NewPackageID(member)
		if err != nil {
			return unavailable(missingNotAParcel), nil
		}
		weight, dimensions, pieceFacts, pieceMissing, err := resolver.weightAndDimensions(ctx, query.Tenant, psTenant, parcel, member)
		if err != nil {
			return ppports.PricingInputResolution{}, err
		}
		if len(pieceMissing) > 0 {
			for _, item := range pieceMissing {
				missing = append(missing, member+": "+item)
			}
			continue
		}
		origin := ppdomain.PieceMeasurementDeclared
		if len(pieceFacts) > 0 && pieceFacts[0].Reference().Kind() == ppdomain.ArtifactActualMeasurement {
			origin = ppdomain.PieceMeasurementMeasured
		}
		piece, err := ppdomain.NewMemberPiece(packageID, weight, dimensions)
		if err != nil {
			return unavailable(missingWeightValue), nil
		}
		piece, err = piece.WithMeasurementOrigin(origin)
		if err != nil {
			return unavailable(missingWeightValue), nil
		}
		pieces = append(pieces, piece)
		ids = append(ids, packageID)
		facts = append(facts, pieceFacts...)
		if !haveRoute {
			candidate, addressFact, routeMissing, err := resolver.postalRoute(ctx, psTenant, parcel)
			if err != nil {
				return ppports.PricingInputResolution{}, err
			}
			if len(routeMissing) == 0 {
				route, routeFact, haveRoute = candidate, addressFact, true
			}
		}
	}
	if len(missing) > 0 {
		return unavailable(missing...), nil
	}
	if !haveRoute {
		return unavailable(missingDestination), nil
	}
	facts = append(facts, routeFact)
	if len(pieces) == 0 {
		return unavailable(missingParcel), nil
	}
	total, err := sumPieceWeights(pieces)
	if err != nil {
		return unavailable(err.Error()), nil
	}
	manifest, err := ppdomain.NewMemberManifest(ids, total, nil)
	if err != nil {
		return unavailable(missingWeightValue), nil
	}
	manifest, err = manifest.WithPieces(pieces)
	if err != nil {
		return unavailable(missingWeightValue), nil
	}
	subject, err := ppdomain.NewShipmentSubject(query.Sources.Shipment)
	if err != nil {
		return unavailable(missingParcel), nil
	}
	input, err := ppdomain.NewPostalAggregatePricingInputSnapshot(
		query.Tenant, query.Scope, subject, route, manifest, query.BasisAt, facts...)
	if err != nil {
		return unavailable(missingWeightValue), nil
	}
	if settlement, declared := query.SettlementCurrency(); declared {
		if input, err = input.WithSettlementCurrency(settlement); err != nil {
			return ppports.PricingInputResolution{}, err
		}
	}
	return ppports.PricingInputResolution{Outcome: ppports.PricingInputResolved, Input: input}, nil
}

func sumPieceWeights(pieces []ppdomain.MemberPiece) (ppdomain.Weight, error) {
	total := pieces[0].ActualWeight().Value()
	unit := pieces[0].ActualWeight().Unit()
	for _, piece := range pieces[1:] {
		weight := piece.ActualWeight()
		if weight.Unit() != unit {
			return ppdomain.Weight{}, fmt.Errorf("parcel-shipment: member weights are not in one unit")
		}
		summed, err := total.Add(weight.Value())
		if err != nil {
			return ppdomain.Weight{}, err
		}
		total = summed
	}
	return ppdomain.NewWeight(total, unit)
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

// Resolver 按运输收费发生项造快照。成员恰为一件时取数整段交给 ParcelResolver；多于一件时只在逐委托的采购卡上
// 另立聚合主体。它实现 PricingInputResolver。不 import 提供方 application。
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
		return resolver.parcels.resolve(ctx, query, members.Members[0].String(), members.OccurredAt)
	default:
		if query.Aggregation != ppdomain.AggregationPerShipment {
			return unavailable(missingSingleMember), nil
		}
		return resolver.resolveOccurrenceAggregate(ctx, query, members)
	}
}

// resolveOccurrenceAggregate 把已经形成、带多件已受理包裹的发生项收成一份聚合输入。主体是发生项本身。
// 业务时点用发生项的发生时刻：请求上的计价基准时点是结算转抄的同一刻，不是这一路的权威。
// 各件邮编路线必须一致才写进快照；不一致不挑第一件，分区与路线都空着，评价待判断。
func (resolver *Resolver) resolveOccurrenceAggregate(
	ctx context.Context,
	query ppports.PricingInputQuery,
	members tfports.ChargeOccurrenceMembers,
) (ppports.PricingInputResolution, error) {
	psTenant, err := psdomain.NewTenantID(query.Tenant.String())
	if err != nil {
		return unavailable(missingNotAParcel), nil
	}
	pieces := make([]ppdomain.MemberPiece, 0, len(members.Members))
	ids := make([]ppdomain.PackageID, 0, len(members.Members))
	var facts []ppdomain.VersionedFactReference
	var missing []string
	var agreed ppdomain.PostalRoute
	routesDiffer := false
	haveRoute := false
	for _, member := range members.Members {
		object := member.String()
		parcel, err := psdomain.NewDeclaredParcelID(object)
		if err != nil {
			return unavailable(missingNotAParcel), nil
		}
		packageID, err := ppdomain.NewPackageID(object)
		if err != nil {
			return unavailable(missingNotAParcel), nil
		}
		weight, dimensions, pieceFacts, pieceMissing, err := resolver.parcels.weightAndDimensions(ctx, query.Tenant, psTenant, parcel, object)
		if err != nil {
			return ppports.PricingInputResolution{}, err
		}
		if names(pieceMissing, missingNotAParcel) {
			return unavailable(missingNotAParcel), nil
		}
		if len(pieceMissing) > 0 {
			for _, item := range pieceMissing {
				missing = append(missing, object+": "+item)
			}
			continue
		}
		origin := ppdomain.PieceMeasurementDeclared
		if len(pieceFacts) > 0 && pieceFacts[0].Reference().Kind() == ppdomain.ArtifactActualMeasurement {
			origin = ppdomain.PieceMeasurementMeasured
		}
		piece, err := ppdomain.NewMemberPiece(packageID, weight, dimensions)
		if err != nil {
			return unavailable(missingWeightValue), nil
		}
		piece, err = piece.WithMeasurementOrigin(origin)
		if err != nil {
			return unavailable(missingWeightValue), nil
		}
		route, addressFact, routeMissing, err := resolver.parcels.postalRoute(ctx, psTenant, parcel)
		if err != nil {
			return ppports.PricingInputResolution{}, err
		}
		if names(routeMissing, missingNotAParcel) {
			return unavailable(missingNotAParcel), nil
		}
		if len(routeMissing) > 0 {
			for _, item := range routeMissing {
				missing = append(missing, object+": "+item)
			}
			continue
		}
		pieces = append(pieces, piece)
		ids = append(ids, packageID)
		facts = append(facts, pieceFacts...)
		facts = append(facts, addressFact)
		if !haveRoute {
			agreed, haveRoute = route, true
			continue
		}
		if route.Origin() != agreed.Origin() || route.Destination() != agreed.Destination() {
			routesDiffer = true
		}
	}
	if len(missing) > 0 {
		return unavailable(missing...), nil
	}
	if len(pieces) == 0 {
		return unavailable(missingMemberCount), nil
	}
	total, err := sumPieceWeights(pieces)
	if err != nil {
		return unavailable(err.Error()), nil
	}
	manifest, err := ppdomain.NewMemberManifest(ids, total, nil)
	if err != nil {
		return unavailable(missingWeightValue), nil
	}
	manifest, err = manifest.WithPieces(pieces)
	if err != nil {
		return unavailable(missingWeightValue), nil
	}
	subject, err := ppdomain.NewTransportChargeOccurrenceSubject(query.Sources.Occurrence)
	if err != nil {
		return unavailable(missingMembers), nil
	}
	// 调用方没有分区。空分区只对这个主体合法：一致的路线随后补上，不一致则留给评价答待判断。
	input, err := ppdomain.NewAggregatePricingInputSnapshot(
		query.Tenant, query.Scope, subject, "", manifest, members.OccurredAt, facts...)
	if err != nil {
		return unavailable(missingWeightValue), nil
	}
	if haveRoute && !routesDiffer {
		if input, err = input.WithPostalRoute(agreed); err != nil {
			return unavailable(missingDestination), nil
		}
	}
	if settlement, declared := query.SettlementCurrency(); declared {
		if input, err = input.WithSettlementCurrency(settlement); err != nil {
			return ppports.PricingInputResolution{}, err
		}
	}
	return ppports.PricingInputResolution{Outcome: ppports.PricingInputResolved, Input: input}, nil
}

func (resolver *ParcelResolver) weightAndDimensions(
	ctx context.Context,
	tenant ppdomain.TenantID,
	psTenant psdomain.TenantID,
	parcel psdomain.DeclaredParcelID,
	object string,
) (ppdomain.Weight, *ppdomain.Dimensions, []ppdomain.VersionedFactReference, []string, error) {
	// 实测整份取用：节点的一条测量同时带重量与尺寸，有当前有效实测时两样都用它，申报不得顶替、也不补哪一格；
	// 没有实测，两样都取申报。快照里因此只会有实测事实或申报事实其中一类，计价重量出自哪一边看事实种类就分得出。
	if resolver.deps.Measured != nil {
		measured, err := resolver.deps.Measured.LoadActualMeasurement(ctx, tenant, object)
		if err != nil {
			return ppdomain.Weight{}, nil, nil, nil, err
		}
		if measured.Undetermined != "" {
			return ppdomain.Weight{}, nil, nil, []string{measured.Undetermined}, nil
		}
		if measured.Found {
			dimensions := measured.Dimensions
			return measured.Weight, &dimensions, []ppdomain.VersionedFactReference{measured.Fact}, nil, nil
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
	weight, untranslatable := translateWeight(measurement.Weight())
	if untranslatable != "" {
		return ppdomain.Weight{}, nil, nil, []string{untranslatable}, nil
	}
	// 客户申报里的尺寸不是必填：没申报就没有尺寸，不在这里造。
	var dimensions *ppdomain.Dimensions
	if declared, hasDimensions := measurement.Dimensions(); hasDimensions {
		translated, untranslatable := translateDimensions(declared)
		if untranslatable != "" {
			return ppdomain.Weight{}, nil, nil, []string{untranslatable}, nil
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
	route, formed := ppdomain.PostalRouteFromDeclared(origin, postal)
	if !formed {
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

// translateWeight 把申报的重量译成计价的重量；译不出来时第二个返回值是照实的缺项文字，译得出来时为空。单位不在计价
// 单位表里与数值计价读不成重量是两条不同的排查路，各给各的话。
func translateWeight(weight psdomain.DeclaredWeight) (ppdomain.Weight, string) {
	unitName := weight.Unit().String()
	unit, err := ppdomain.NewWeightUnit(unitName)
	if err != nil {
		return ppdomain.Weight{}, missingWeightUnit + ": " + unitName
	}
	translated, err := ppdomain.NewWeightFromString(weight.Value().String(), unit)
	if err != nil {
		return ppdomain.Weight{}, missingWeightValue
	}
	return translated, ""
}

// translateDimensions 与 translateWeight 同形：三边共用一个单位，单位与数值各答各的话。
func translateDimensions(dimensions psdomain.DeclaredDimensions) (ppdomain.Dimensions, string) {
	unitName := dimensions.Unit().String()
	unit, err := ppdomain.NewLengthUnit(unitName)
	if err != nil {
		return ppdomain.Dimensions{}, missingLengthUnit + ": " + unitName
	}
	translated, err := ppdomain.NewDimensionsFromStrings(
		dimensions.Length().String(), dimensions.Width().String(), dimensions.Height().String(), unit)
	if err != nil {
		return ppdomain.Dimensions{}, missingLengthValue
	}
	return translated, ""
}

func unavailable(missing ...string) ppports.PricingInputResolution {
	return ppports.PricingInputResolution{
		Outcome: ppports.PricingInputUnavailable,
		Missing: append([]string(nil), missing...),
	}
}
