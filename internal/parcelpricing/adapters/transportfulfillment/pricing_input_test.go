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

func TestAMeasurementSupersedesTheDeclaration(t *testing.T) {
	measured := measuredKG(t, "2.50")
	resolver := resolverWith(t, membersOf(t, objectRef), declaredKG(t, "1.25"), destinationPostal(t, "100115"), measured)

	resolved := resolve(t, resolver, ppdomain.PricingPurposeSupplierCost)
	weight := resolved.Input.ActualWeight()
	if weight.Value().String() != "2.5" || weight.Unit() != ppdomain.WeightUnitKilogram {
		t.Fatalf("weight = %s %s", weight.Value(), weight.Unit())
	}
	if _, present := resolved.Input.Dimensions(); present {
		t.Fatal("declaration dimensions replaced a measurement that had none")
	}
	assertFactVersion(t, resolved.Input, ppdomain.ArtifactDeclaredMeasurement, "measurement")
	for _, fact := range resolved.Input.FactReferences() {
		if strings.Contains(fact.Reference().Version(), "declaration") {
			t.Fatalf("declaration replaced the measurement: %s", fact.Reference().Version())
		}
	}
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
	measured adapter.ActualMeasurement
	found    bool
}

func (double measurementDouble) LoadActualMeasurement(context.Context, ppdomain.TenantID, string) (adapter.ActualMeasurement, bool, error) {
	return double.measured, double.found, nil
}

func resolverWith(
	t *testing.T,
	members tfports.ChargeOccurrenceMembers,
	declared psdomain.DeclaredMeasurementResolution,
	addresses psdomain.ShipmentAddressElements,
	measured *adapter.ActualMeasurement,
) *adapter.Resolver {
	t.Helper()
	deps := adapter.ResolverDeps{
		Members:   &memberDouble{members: members, found: true},
		Declared:  declaredDouble{resolution: declared},
		Addresses: addressDouble{elements: addresses},
	}
	if measured != nil {
		deps.Measured = measurementDouble{measured: *measured, found: true}
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

func measuredKG(t *testing.T, raw string) *adapter.ActualMeasurement {
	t.Helper()
	weight, err := ppdomain.NewWeightFromString(raw, ppdomain.WeightUnitKilogram)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactDeclaredMeasurement, objectRef, "measurement")
	if err != nil {
		t.Fatal(err)
	}
	fact, err := ppdomain.NewVersionedFactReference(reference)
	if err != nil {
		t.Fatal(err)
	}
	return &adapter.ActualMeasurement{Weight: weight, Fact: fact}
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
