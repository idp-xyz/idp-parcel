package transportfulfillment_test

import (
	"context"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/transportfulfillment"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 单件仍走包裹主体，摘要与按今天的构造器另造的那一份相同。请求上的聚合方式与另一时刻的计价基准都不进入这一支。
func TestOneMemberKeepsThePackageSubjectAndTheSameDigest(t *testing.T) {
	resolver := resolverWith(t, membersOf(t, objectRef), declaredKG(t, "1.25"), destinationPostal(t, "100115"), nil)
	basis := inputAt.Add(3 * time.Hour)
	plain := resolveAt(t, resolver, ppdomain.AggregationMode(""), basis)
	marked := resolveAt(t, resolver, ppdomain.AggregationPerShipment, basis)
	for _, resolved := range []ppports.PricingInputResolution{plain, marked} {
		subject, ok := resolved.Input.Subject()
		if !ok || subject.Kind() != ppdomain.SubjectAcceptedPackage || subject.Reference() != objectRef {
			t.Fatalf("主体 = %+v %v", subject, ok)
		}
		if !resolved.Input.BusinessAt().Equal(inputAt) {
			t.Fatalf("业务时点 = %s，want 发生时刻 %s", resolved.Input.BusinessAt(), inputAt)
		}
		if _, present := resolved.Input.Members(); present {
			t.Fatal("单件长出了成员清单")
		}
	}
	hand := packageSnapshot(t)
	plan := singlePiecePlan(t)
	if pptest.Evaluate(t, "from-resolver", plan, plain.Input).SemanticDigest() !=
		pptest.Evaluate(t, "from-constructors", plan, hand).SemanticDigest() {
		t.Fatal("单件摘要与包裹主体构造器算出的那一份不同")
	}
}

func TestSeveralMembersOnAPerShipmentCardFormOneOccurrenceSubject(t *testing.T) {
	resolver := multiResolver(t, map[string]parcelFacts{
		"SYN-PKG-01": {weight: "2", length: "30", width: "20", height: "10", postal: "018956"},
		"SYN-PKG-02": {weight: "1", length: "50", width: "40", height: "30", postal: "018956"},
	})
	resolved := resolveAt(t, resolver, ppdomain.AggregationPerShipment, inputAt.Add(time.Hour))
	subject, ok := resolved.Input.Subject()
	if !ok || subject.Kind() != ppdomain.SubjectTransportChargeOccurrence || subject.Reference() != "SYN-OCC-01" {
		t.Fatalf("主体 = %+v %v", subject, ok)
	}
	if !resolved.Input.BusinessAt().Equal(inputAt) {
		t.Fatalf("业务时点 = %s，want 发生时刻", resolved.Input.BusinessAt())
	}
	route, present := resolved.Input.PostalRoute()
	if !present || route.Destination() != "018956" {
		t.Fatalf("路线 = %+v %v", route, present)
	}
	manifest, present := resolved.Input.Members()
	if !present || manifest.Count() != 2 || manifest.TotalActualWeight().Value().String() != "3" {
		t.Fatalf("清单 = %+v %v", manifest, present)
	}
	pieces, present := manifest.Pieces()
	if !present || len(pieces) != 2 {
		t.Fatal("逐件实重没有写上")
	}
	for _, piece := range pieces {
		origin, recorded := piece.MeasurementOrigin()
		if !recorded || origin != ppdomain.PieceMeasurementDeclared {
			t.Fatalf("%s 取源 = %s %v", piece.ID(), origin, recorded)
		}
	}
}

func TestSeveralMembersStayUnavailableOnAPerPackageCard(t *testing.T) {
	resolver := multiResolver(t, map[string]parcelFacts{
		"SYN-PKG-01": {weight: "2", postal: "018956"},
		"SYN-PKG-02": {weight: "1", postal: "018956"},
	})
	for _, aggregation := range []ppdomain.AggregationMode{"", ppdomain.AggregationPerPackage, ppdomain.AggregationPerMasterDocument} {
		resolved, err := resolver.ResolvePricingInput(context.Background(), queryWith(t, aggregation, inputAt))
		if err != nil {
			t.Fatal(err)
		}
		if resolved.Outcome != ppports.PricingInputUnavailable || !names(resolved.Missing, "more than one") {
			t.Fatalf("%s outcome=%s missing=%v", aggregation, resolved.Outcome, resolved.Missing)
		}
	}
}

func TestDivergentPostalRoutesLeaveTheSnapshotWithoutARoute(t *testing.T) {
	resolver := multiResolver(t, map[string]parcelFacts{
		"SYN-PKG-01": {weight: "2", postal: "018956"},
		"SYN-PKG-02": {weight: "1", postal: "238001"},
	})
	resolved := resolveAt(t, resolver, ppdomain.AggregationPerShipment, inputAt)
	if _, present := resolved.Input.PostalRoute(); present || resolved.Input.Zone() != "" {
		t.Fatal("邮编不一致却挑了一条路线或写了分区")
	}
	subject, _ := resolved.Input.Subject()
	if subject.Kind() != ppdomain.SubjectTransportChargeOccurrence {
		t.Fatalf("主体 = %s", subject.Kind())
	}
}

func TestAConsolidationUnitAmongMembersStaysUnavailable(t *testing.T) {
	resolver := multiResolver(t, map[string]parcelFacts{
		"SYN-PKG-01":  {weight: "2", postal: "018956"},
		"SYN-UNIT-01": {},
	})
	resolved, err := resolver.ResolvePricingInput(context.Background(), queryWith(t, ppdomain.AggregationPerShipment, inputAt))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Outcome != ppports.PricingInputUnavailable || !names(resolved.Missing, "consolidation unit") {
		t.Fatalf("outcome=%s missing=%v", resolved.Outcome, resolved.Missing)
	}
}

type parcelFacts struct {
	weight, length, width, height, postal string
}

type declaredByParcel struct {
	by map[string]psdomain.DeclaredMeasurementResolution
}

func (double declaredByParcel) LoadDeclaredMeasurement(_ context.Context, _ psdomain.TenantID, parcel psdomain.DeclaredParcelID) (psdomain.DeclaredMeasurementResolution, error) {
	resolution, ok := double.by[parcel.String()]
	if !ok {
		return psdomain.NoDeclaredMeasurementResolution(), nil
	}
	return resolution, nil
}

type addressByParcel struct {
	by map[string]psdomain.ShipmentAddressElements
}

func (double addressByParcel) LoadAddressElements(_ context.Context, _ psdomain.TenantID, parcel psdomain.DeclaredParcelID) (psdomain.ShipmentAddressElements, error) {
	elements, ok := double.by[parcel.String()]
	if !ok {
		return psdomain.NoShipmentAddressElements(), nil
	}
	return elements, nil
}

func multiResolver(t *testing.T, facts map[string]parcelFacts) *adapter.Resolver {
	t.Helper()
	declared := map[string]psdomain.DeclaredMeasurementResolution{}
	addresses := map[string]psdomain.ShipmentAddressElements{}
	var members []tfdomain.CarriedObjectReference
	for id, fact := range facts {
		reference, err := tfdomain.NewCarriedObjectReference(id)
		if err != nil {
			t.Fatal(err)
		}
		members = append(members, reference)
		if fact.weight == "" {
			continue
		}
		if fact.length == "" {
			declared[id] = declaredKG(t, fact.weight)
		} else {
			declared[id] = declaredKGWithDimensions(t, fact.weight, fact.length, fact.width, fact.height)
		}
		if fact.postal != "" {
			addresses[id] = destinationPostal(t, fact.postal)
		}
	}
	resolver, err := adapter.NewResolver(adapter.ResolverDeps{
		Members:   &memberDouble{members: tfports.ChargeOccurrenceMembers{Members: members, OccurredAt: inputAt}, found: true},
		Declared:  declaredByParcel{by: declared},
		Addresses: addressByParcel{by: addresses},
	})
	if err != nil {
		t.Fatal(err)
	}
	return resolver
}

func resolveAt(t *testing.T, resolver *adapter.Resolver, aggregation ppdomain.AggregationMode, basis time.Time) ppports.PricingInputResolution {
	t.Helper()
	resolved, err := resolver.ResolvePricingInput(context.Background(), queryWith(t, aggregation, basis))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Outcome != ppports.PricingInputResolved {
		t.Fatalf("outcome %s missing %v", resolved.Outcome, resolved.Missing)
	}
	return resolved
}

func queryWith(t *testing.T, aggregation ppdomain.AggregationMode, basis time.Time) ppports.PricingInputQuery {
	t.Helper()
	query := inputQuery(t, ppdomain.PricingPurposeSupplierCost)
	query.Aggregation = aggregation
	query.BasisAt = basis
	return query
}

func packageSnapshot(t *testing.T) ppdomain.PricingInputSnapshot {
	t.Helper()
	subject, err := ppdomain.NewAcceptedPackageSubject(mustPackage(t, objectRef))
	if err != nil {
		t.Fatal(err)
	}
	route, err := ppdomain.NewPostalRoute("", "100115")
	if err != nil {
		t.Fatal(err)
	}
	weight, err := ppdomain.NewWeightFromString("1.25", ppdomain.WeightUnitKilogram)
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := ppdomain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := ppdomain.NewPricingScopeID("SYN-SCOPE")
	if err != nil {
		t.Fatal(err)
	}
	input, err := ppdomain.NewPostalPricingInputSnapshot(tenant, scope, subject, route, weight, nil, inputAt,
		factOf(t, ppdomain.ArtifactDeclaredMeasurement, objectRef, "declaration@baseline"),
		factOf(t, ppdomain.ArtifactAddressElements, objectRef, "address@baseline"))
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func factOf(t *testing.T, kind ppdomain.ArtifactKind, id, version string) ppdomain.VersionedFactReference {
	t.Helper()
	reference, err := ppdomain.NewVersionReferenceIdentity(kind, id, version)
	if err != nil {
		t.Fatal(err)
	}
	fact, err := ppdomain.NewVersionedFactReference(reference)
	if err != nil {
		t.Fatal(err)
	}
	return fact
}

func mustPackage(t *testing.T, id string) ppdomain.PackageID {
	t.Helper()
	packageID, err := ppdomain.NewPackageID(id)
	if err != nil {
		t.Fatal(err)
	}
	return packageID
}

func singlePiecePlan(t *testing.T) ppdomain.PricingPlanVersion {
	t.Helper()
	return pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, "SYN-PLAN-ONE", "v1"),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, "SYN-TABLE-ONE", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, "SYN-WEIGHT-ONE", "v1"),
		Scope:                 "SYN-SCOPE",
		Direction:             ppdomain.PricingDirectionBuy,
		Purpose:               ppdomain.PricingPurposeSupplierCost,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "CNY",
		Period:                pptest.Period{StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		RateEntryID:           "SYN-RATE-ONE",
		RateZone:              "Z1",
		MinimumKilograms:      "0",
		MaximumKilograms:      "30",
		RateAmount:            "10",
		WeightRounding:        ppdomain.RoundingNone,
		WeightStepKilograms:   "1",
	})
}
