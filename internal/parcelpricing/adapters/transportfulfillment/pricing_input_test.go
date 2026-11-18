package transportfulfillment_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/transportfulfillment"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var (
	inputAt   = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	objectRef = "SYN-PKG-01"
)

func TestDeclaredWeightAndPostalRouteResolveWhenThereIsNoMeasurement(t *testing.T) {
	resolver := resolverWith(t, membersOf(t, objectRef), declaredKG(t, "1.25"), destinationPostal(t, "100115"), nil)

	for _, purpose := range []ppdomain.PricingPurpose{ppdomain.PricingPurposeSupplierCost, ppdomain.PricingPurposeCustomerCharge} {
		resolved := resolve(t, resolver, purpose)
		if resolved.Outcome != ppports.PricingInputResolved {
			t.Fatalf("purpose %s outcome %s missing %v", purpose, resolved.Outcome, resolved.Missing)
		}
		weight := resolved.Input.ActualWeight()
		if weight.Value().String() != "1.25" || weight.Unit() != ppdomain.WeightUnitKilogram {
			t.Fatalf("weight = %s %s", weight.Value(), weight.Unit())
		}
		if _, present := resolved.Input.Dimensions(); present {
			t.Fatal("absent declared dimensions were invented")
		}
		route, ok := resolved.Input.PostalRoute()
		if !ok || route.Destination() != "100115" || route.Origin() != "" {
			t.Fatalf("route = %+v present=%v", route, ok)
		}
		subject, ok := resolved.Input.Subject()
		if !ok || subject.Kind() != ppdomain.SubjectAcceptedPackage || subject.Reference() != objectRef {
			t.Fatalf("subject = %+v present=%v", subject, ok)
		}
		if resolved.Input.BusinessAt() != inputAt {
			t.Fatalf("business time = %s", resolved.Input.BusinessAt())
		}
		assertFactVersion(t, resolved.Input, ppdomain.ArtifactDeclaredMeasurement, "declaration@baseline")
		assertFactVersion(t, resolved.Input, ppdomain.ArtifactAddressElements, "address@baseline")
	}
}

// Covers: 实测整份取用。有当前有效实测时，重量与尺寸都出自那一条，申报的重量与尺寸一格都不进快照——哪怕申报
// 也带着尺寸；快照里只有实测事实，计价重量的来源是实测。
func TestAMeasurementSupersedesTheWholeDeclaration(t *testing.T) {
	declared := declaredKGWithDimensions(t, "1.25", "20", "15", "10")
	resolver := resolverWith(t, membersOf(t, objectRef), declared, destinationPostal(t, "100115"), measuredKG(t, "2.50"))

	for _, purpose := range []ppdomain.PricingPurpose{ppdomain.PricingPurposeSupplierCost, ppdomain.PricingPurposeCustomerCharge} {
		resolved := resolve(t, resolver, purpose)
		weight := resolved.Input.ActualWeight()
		if weight.Value().String() != "2.5" || weight.Unit() != ppdomain.WeightUnitKilogram {
			t.Fatalf("%s：重量 = %s %s，want 实测的 2.5 KG", purpose, weight.Value(), weight.Unit())
		}
		dimensions, present := resolved.Input.Dimensions()
		if !present || dimensions.LongestSide().Value().String() != "40" || dimensions.ShortestSide().Value().String() != "25" {
			t.Fatalf("%s：尺寸 = %+v %v，want 实测的 40×30×25", purpose, dimensions, present)
		}
		assertFactVersion(t, resolved.Input, ppdomain.ArtifactActualMeasurement, "SYN-MEASURE-1")
		for _, fact := range resolved.Input.FactReferences() {
			if fact.Reference().Kind() == ppdomain.ArtifactDeclaredMeasurement {
				t.Fatalf("%s：申报顶进了有实测的快照：%s", purpose, fact.Reference().Version())
			}
		}
		if origin, ok := ppdomain.PricingWeightOriginOf(resolved.Input); !ok || origin.Kind != ppdomain.ArtifactActualMeasurement {
			t.Fatalf("%s：计价重量来源 = %+v %v，want 实测", purpose, origin, ok)
		}
	}
}

// Covers: 当前有效实测定不下（复测之后没有采用形态，或两条发生时间相同）时整份输入不可得，不退回申报。
func TestAnUndeterminedMeasurementDoesNotFallBackToTheDeclaration(t *testing.T) {
	deps := adapter.ResolverDeps{
		Members:   &memberDouble{members: membersOf(t, objectRef), found: true},
		Declared:  declaredDouble{resolution: declaredKGWithDimensions(t, "1.25", "20", "15", "10")},
		Addresses: addressDouble{elements: destinationPostal(t, "100115")},
		Measured:  measurementDouble{load: adapter.ActualMeasurementLoad{Undetermined: "输入不可得：实测多于一条"}},
	}
	resolver, err := adapter.NewResolver(deps)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolver.ResolvePricingInput(context.Background(), inputQuery(t, ppdomain.PricingPurposeCustomerCharge))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Outcome != ppports.PricingInputUnavailable || !names(resolved.Missing, "实测多于一条") {
		t.Fatalf("outcome=%s missing=%v，want 输入不可得", resolved.Outcome, resolved.Missing)
	}
}

// Covers: 没有实测时两样都取申报；客户申报里的尺寸不是必填，申报了就带上。
func TestWithoutAMeasurementTheDeclaredDimensionsAreUsed(t *testing.T) {
	declared := declaredKGWithDimensions(t, "1.25", "20", "15", "10")
	resolved := resolve(t, resolverWith(t, membersOf(t, objectRef), declared, destinationPostal(t, "100115"), nil),
		ppdomain.PricingPurposeCustomerCharge)
	if weight := resolved.Input.ActualWeight(); weight.Value().String() != "1.25" {
		t.Fatalf("重量 = %s，want 申报的 1.25", weight.Value())
	}
	if dimensions, present := resolved.Input.Dimensions(); !present || dimensions.LongestSide().Value().String() != "20" {
		t.Fatalf("尺寸 = %+v %v，want 申报的 20×15×10", dimensions, present)
	}
	if origin, ok := ppdomain.PricingWeightOriginOf(resolved.Input); !ok || origin.Kind != ppdomain.ArtifactDeclaredMeasurement {
		t.Fatalf("计价重量来源 = %+v %v，want 申报", origin, ok)
	}
}

func mustDecimal(t *testing.T, raw string) ppdomain.Decimal {
	t.Helper()
	value, err := ppdomain.ParseDecimal(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func declaredKGWithDimensions(t *testing.T, weight, length, width, height string) psdomain.DeclaredMeasurementResolution {
	t.Helper()
	value := func(raw string) psdomain.MeasurementValue {
		parsed, err := psdomain.NewMeasurementValue(raw)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	unit := func(raw string) psdomain.MeasurementUnitReference {
		reference, err := psdomain.NewMeasurementUnitReference(raw)
		if err != nil {
			t.Fatal(err)
		}
		return reference
	}
	declaredWeight, err := psdomain.NewDeclaredWeight(value(weight), unit("KG"))
	if err != nil {
		t.Fatal(err)
	}
	dimensions, err := psdomain.NewDeclaredDimensions(value(length), value(width), value(height), unit("CM"))
	if err != nil {
		t.Fatal(err)
	}
	measurement, err := psdomain.NewDeclaredMeasurement(declaredWeight, dimensions)
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := psdomain.DeclaredMeasurementOnBaseline(measurement)
	if err != nil {
		t.Fatal(err)
	}
	return resolution
}

func TestAMissingDeclarationDoesNotInventAWeight(t *testing.T) {
	resolver := resolverWith(t, membersOf(t, objectRef), psdomain.DeclaredMeasurementNotDeclaredResolution(), destinationPostal(t, "100115"), nil)

	resolved, err := resolver.ResolvePricingInput(context.Background(), inputQuery(t, ppdomain.PricingPurposeSupplierCost))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Outcome != ppports.PricingInputUnavailable {
		t.Fatalf("outcome = %s", resolved.Outcome)
	}
	if !names(resolved.Missing, "declared weight is absent") {
		t.Fatalf("missing = %v", resolved.Missing)
	}
}

func TestAConsolidationUnitIsUnavailable(t *testing.T) {
	resolver := resolverWith(t, membersOf(t, "SYN-UNIT-01"), psdomain.NoDeclaredMeasurementResolution(), psdomain.NoShipmentAddressElements(), nil)

	resolved, err := resolver.ResolvePricingInput(context.Background(), inputQuery(t, ppdomain.PricingPurposeSupplierCost))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Outcome != ppports.PricingInputUnavailable || !names(resolved.Missing, "consolidation unit") {
		t.Fatalf("outcome=%s missing=%v", resolved.Outcome, resolved.Missing)
	}
}

func TestAnUnknownDeclaredUnitIsUnavailable(t *testing.T) {
	resolver := resolverWith(t, membersOf(t, objectRef), declaredUnit(t, "1.25", "TON"), destinationPostal(t, "100115"), nil)

	resolved, err := resolver.ResolvePricingInput(context.Background(), inputQuery(t, ppdomain.PricingPurposeSupplierCost))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Outcome != ppports.PricingInputUnavailable || !names(resolved.Missing, "TON") {
		t.Fatalf("outcome=%s missing=%v", resolved.Outcome, resolved.Missing)
	}
}

// 正式 SELL·客户费用评价按原词认单位，小写 kg 与 TON 同为表外。受理前试算（parcelshipment 适配器的 measurementOf）对同一份
// 申报同样当没有测量：两条路共用计价的封闭单位表，谁折叠谁不折叠一变，就会出现受理前冻得出、受理后评不出的那一份。
func TestALowercaseDeclaredUnitIsOutsideTheTableForTheCustomerCharge(t *testing.T) {
	resolver := resolverWith(t, membersOf(t, objectRef), declaredUnit(t, "1.25", "kg"), destinationPostal(t, "100115"), nil)

	resolved, err := resolver.ResolvePricingInput(context.Background(), inputQuery(t, ppdomain.PricingPurposeCustomerCharge))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Outcome != ppports.PricingInputUnavailable || !names(resolved.Missing, "kg") {
		t.Fatalf("outcome=%s missing=%v, 想要小写 kg 当表外单位停", resolved.Outcome, resolved.Missing)
	}
}

func TestAMissingDestinationPostalCodeIsUnavailable(t *testing.T) {
	resolver := resolverWith(t, membersOf(t, objectRef), declaredKG(t, "1.25"), psdomain.NewShipmentAddressElements(
		psdomain.AddressElementsNotProvidedResolution(),
		psdomain.AddressElementsNotProvidedResolution(),
	), nil)

	resolved, err := resolver.ResolvePricingInput(context.Background(), inputQuery(t, ppdomain.PricingPurposeSupplierCost))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Outcome != ppports.PricingInputUnavailable || !names(resolved.Missing, "destination postal code") {
		t.Fatalf("outcome=%s missing=%v", resolved.Outcome, resolved.Missing)
	}
}

func TestSeveralMembersAreUnavailable(t *testing.T) {
	members := membersOf(t, objectRef)
	second, err := tfdomain.NewCarriedObjectReference("SYN-PKG-02")
	if err != nil {
		t.Fatal(err)
	}
	members.Members = append(members.Members, second)
	resolver := resolverWith(t, members, declaredKG(t, "1.25"), destinationPostal(t, "100115"), nil)

	resolved, err := resolver.ResolvePricingInput(context.Background(), inputQuery(t, ppdomain.PricingPurposeSupplierCost))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Outcome != ppports.PricingInputUnavailable || !names(resolved.Missing, "more than one") {
		t.Fatalf("outcome=%s missing=%v", resolved.Outcome, resolved.Missing)
	}
}

func TestAMissingOccurrenceIsUnavailable(t *testing.T) {
	resolver, err := adapter.NewResolver(adapter.ResolverDeps{
		Members:   &memberDouble{found: false},
		Declared:  declaredDouble{resolution: declaredKG(t, "1.25")},
		Addresses: addressDouble{elements: destinationPostal(t, "100115")},
	})
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := resolver.ResolvePricingInput(context.Background(), inputQuery(t, ppdomain.PricingPurposeSupplierCost))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Outcome != ppports.PricingInputUnavailable || !names(resolved.Missing, "were not found") {
		t.Fatalf("outcome=%s missing=%v", resolved.Outcome, resolved.Missing)
	}
}

func TestAProviderReadErrorIsReturned(t *testing.T) {
	boom := errors.New("member view down")
	resolver, err := adapter.NewResolver(adapter.ResolverDeps{
		Members:   &memberDouble{err: boom, found: true},
		Declared:  declaredDouble{resolution: declaredKG(t, "1.25")},
		Addresses: addressDouble{elements: destinationPostal(t, "100115")},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolver.ResolvePricingInput(context.Background(), inputQuery(t, ppdomain.PricingPurposeSupplierCost))
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

type memberDouble struct {
	members tfports.ChargeOccurrenceMembers
	found   bool
	err     error
}

func (double *memberDouble) LoadMembers(context.Context, tfports.ChargeOccurrenceKey) (tfports.ChargeOccurrenceMembers, bool, error) {
	return double.members, double.found, double.err
}

type declaredDouble struct {
	resolution psdomain.DeclaredMeasurementResolution
	err        error
}

func (double declaredDouble) LoadDeclaredMeasurement(context.Context, psdomain.TenantID, psdomain.DeclaredParcelID) (psdomain.DeclaredMeasurementResolution, error) {
	return double.resolution, double.err
}

type addressDouble struct {
	elements psdomain.ShipmentAddressElements
	err      error
}

func (double addressDouble) LoadAddressElements(context.Context, psdomain.TenantID, psdomain.DeclaredParcelID) (psdomain.ShipmentAddressElements, error) {
	return double.elements, double.err
}

type measurementDouble struct {
	load adapter.ActualMeasurementLoad
}

func (double measurementDouble) LoadActualMeasurement(context.Context, ppdomain.TenantID, string) (adapter.ActualMeasurementLoad, error) {
	return double.load, nil
}

func resolverWith(
	t *testing.T,
	members tfports.ChargeOccurrenceMembers,
	declared psdomain.DeclaredMeasurementResolution,
	addresses psdomain.ShipmentAddressElements,
	measured *adapter.ActualMeasurementLoad,
) *adapter.Resolver {
	t.Helper()
	deps := adapter.ResolverDeps{
		Members:   &memberDouble{members: members, found: true},
		Declared:  declaredDouble{resolution: declared},
		Addresses: addressDouble{elements: addresses},
	}
	if measured != nil {
		deps.Measured = measurementDouble{load: *measured}
	}
	resolver, err := adapter.NewResolver(deps)
	if err != nil {
		t.Fatal(err)
	}
	return resolver
}

func resolve(t *testing.T, resolver *adapter.Resolver, purpose ppdomain.PricingPurpose) ppports.PricingInputResolution {
	t.Helper()
	resolved, err := resolver.ResolvePricingInput(context.Background(), inputQuery(t, purpose))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Outcome != ppports.PricingInputResolved {
		t.Fatalf("outcome %s missing %v", resolved.Outcome, resolved.Missing)
	}
	return resolved
}

func inputQuery(t *testing.T, purpose ppdomain.PricingPurpose) ppports.PricingInputQuery {
	t.Helper()
	tenant, err := ppdomain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := ppdomain.NewPricingScopeID("SYN-SCOPE")
	if err != nil {
		t.Fatal(err)
	}
	return ppports.PricingInputQuery{
		Tenant:  tenant,
		Scope:   scope,
		Purpose: purpose,
		BasisAt: inputAt,
		Sources: ppports.EligibleSourceReferences{
			Occurrence:        "SYN-OCC-01",
			OccurrenceVersion: "SYN-VER-01",
			FeeItem:           "SYN-FEE",
			SupplierAgreement: "SYN-AGR",
		},
	}
}

func membersOf(t *testing.T, object string) tfports.ChargeOccurrenceMembers {
	t.Helper()
	reference, err := tfdomain.NewCarriedObjectReference(object)
	if err != nil {
		t.Fatal(err)
	}
	return tfports.ChargeOccurrenceMembers{Members: []tfdomain.CarriedObjectReference{reference}, OccurredAt: inputAt}
}

func declaredKG(t *testing.T, raw string) psdomain.DeclaredMeasurementResolution {
	t.Helper()
	return declaredUnit(t, raw, "KG")
}

func declaredUnit(t *testing.T, raw, unit string) psdomain.DeclaredMeasurementResolution {
	t.Helper()
	value, err := psdomain.NewMeasurementValue(raw)
	if err != nil {
		t.Fatal(err)
	}
	unitRef, err := psdomain.NewMeasurementUnitReference(unit)
	if err != nil {
		t.Fatal(err)
	}
	weight, err := psdomain.NewDeclaredWeight(value, unitRef)
	if err != nil {
		t.Fatal(err)
	}
	measurement, err := psdomain.NewDeclaredMeasurement(weight, psdomain.DeclaredDimensions{})
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := psdomain.DeclaredMeasurementOnBaseline(measurement)
	if err != nil {
		t.Fatal(err)
	}
	return resolution
}

func destinationPostal(t *testing.T, postal string) psdomain.ShipmentAddressElements {
	t.Helper()
	entry, err := psdomain.NewCanonicalContentEntry(
		psdomain.AddressElementEntryName(psdomain.DeliveryPlaceDataGroup(), psdomain.PostalCodeElement),
		postal,
	)
	if err != nil {
		t.Fatal(err)
	}
	elements := psdomain.AddressElementsOf(psdomain.DeliveryPlaceDataGroup(), []psdomain.CanonicalContentEntry{entry})
	destination, err := psdomain.AddressElementsOnBaseline(elements)
	if err != nil {
		t.Fatal(err)
	}
	return psdomain.NewShipmentAddressElements(psdomain.AddressElementsNotProvidedResolution(), destination)
}

// measuredKG 是一条当前有效实测：毛重由用例给，尺寸 40×30×25 CM，事实版本是那次测量的来源身份。
func measuredKG(t *testing.T, raw string) *adapter.ActualMeasurementLoad {
	t.Helper()
	weight, err := ppdomain.NewWeightFromString(raw, ppdomain.WeightUnitKilogram)
	if err != nil {
		t.Fatal(err)
	}
	sides, err := ppdomain.NewDimensions(mustDecimal(t, "40"), mustDecimal(t, "30"), mustDecimal(t, "25"), ppdomain.LengthUnitCentimeter)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactActualMeasurement, objectRef, "SYN-MEASURE-1")
	if err != nil {
		t.Fatal(err)
	}
	fact, err := ppdomain.NewVersionedFactReference(reference)
	if err != nil {
		t.Fatal(err)
	}
	return &adapter.ActualMeasurementLoad{Found: true, Weight: weight, Dimensions: sides, Fact: fact}
}

func assertFactVersion(t *testing.T, input ppdomain.PricingInputSnapshot, kind ppdomain.ArtifactKind, version string) {
	t.Helper()
	for _, fact := range input.FactReferences() {
		reference := fact.Reference()
		if reference.Kind() == kind && reference.Version() == version {
			return
		}
	}
	t.Fatalf("snapshot has no %s version %s", kind, version)
}

func names(missing []string, fragment string) bool {
	for _, item := range missing {
		if strings.Contains(item, fragment) {
			return true
		}
	}
	return false
}
