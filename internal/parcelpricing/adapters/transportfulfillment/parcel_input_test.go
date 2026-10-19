package transportfulfillment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/transportfulfillment"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件钉按包裹的那一只（ADR-0190 决定四）：SELL 请求点名包裹，快照按包裹建——实重尺寸照 ADR-0171 决定二实测优先、
// 缺席取申报并标 `declaration`，邮编路线取寄件到收件，业务时点取请求的计价基准时点，结算币种取请求上快照的那一格。
// 按发生项那一只组合它，那一只的行为由 pricing_input_test.go 原样钉着。

// sellBasis 与 inputAt 有意不同：按包裹建的快照，业务时点只能从请求来。
var sellBasis = time.Date(2026, 10, 2, 8, 15, 0, 0, time.UTC)

func parcelResolverWith(
	t *testing.T,
	declared psdomain.DeclaredMeasurementResolution,
	addresses psdomain.ShipmentAddressElements,
	measured *adapter.ActualMeasurement,
) *adapter.ParcelResolver {
	t.Helper()
	deps := adapter.ParcelResolverDeps{
		Declared:  declaredDouble{resolution: declared},
		Addresses: addressDouble{elements: addresses},
	}
	if measured != nil {
		deps.Measured = measurementDouble{measured: *measured, found: true}
	}
	resolver, err := adapter.NewParcelResolver(deps)
	if err != nil {
		t.Fatal(err)
	}
	return resolver
}

func parcelQuery(t *testing.T, settlement string) ppports.PricingInputQuery {
	t.Helper()
	query := inputQuery(t, ppdomain.PricingPurposeCustomerCharge)
	query.BasisAt = sellBasis
	query.Sources = ppports.EligibleSourceReferences{Parcel: objectRef}
	if settlement != "" {
		currency, err := ppdomain.NewCurrency(settlement)
		if err != nil {
			t.Fatal(err)
		}
		query.Settlement = currency
	}
	return query
}

func TestAParcelResolvesFromItsDeclarationAtTheRequestBasisTimeInTheSettlementCurrency(t *testing.T) {
	resolver := parcelResolverWith(t, declaredKG(t, "1.25"), destinationPostal(t, "018956"), nil)

	resolved, err := resolver.ResolvePricingInput(context.Background(), parcelQuery(t, "CNY"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Outcome != ppports.PricingInputResolved {
		t.Fatalf("outcome %s missing %v", resolved.Outcome, resolved.Missing)
	}
	input := resolved.Input
	if weight := input.ActualWeight(); weight.Value().String() != "1.25" || weight.Unit() != ppdomain.WeightUnitKilogram {
		t.Fatalf("weight = %s %s", weight.Value(), weight.Unit())
	}
	subject, ok := input.Subject()
	if !ok || subject.Kind() != ppdomain.SubjectAcceptedPackage || subject.Reference() != objectRef {
		t.Fatalf("subject = %+v present=%v", subject, ok)
	}
	if route, ok := input.PostalRoute(); !ok || route.Destination() != "018956" {
		t.Fatalf("route = %+v present=%v", route, ok)
	}
	if !input.BusinessAt().Equal(sellBasis) {
		t.Fatalf("business time = %s, want the request's pricing basis time %s", input.BusinessAt(), sellBasis)
	}
	if settlement, declared := input.SettlementCurrency(); !declared || settlement.String() != "CNY" {
		t.Fatalf("settlement currency = %q declared=%v, want the snapshotted CNY", settlement, declared)
	}
	assertFactVersion(t, input, ppdomain.ArtifactDeclaredMeasurement, "declaration@baseline")
	assertFactVersion(t, input, ppdomain.ArtifactAddressElements, "address@baseline")
}

func TestAMeasurementSupersedesTheDeclarationOfAParcel(t *testing.T) {
	resolver := parcelResolverWith(t, declaredKG(t, "1.25"), destinationPostal(t, "018956"), measuredKG(t, "2.50"))

	resolved, err := resolver.ResolvePricingInput(context.Background(), parcelQuery(t, "CNY"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Outcome != ppports.PricingInputResolved {
		t.Fatalf("outcome %s missing %v", resolved.Outcome, resolved.Missing)
	}
	if weight := resolved.Input.ActualWeight(); weight.Value().String() != "2.5" {
		t.Fatalf("weight = %s, want the measurement", weight.Value())
	}
	assertFactVersion(t, resolved.Input, ppdomain.ArtifactDeclaredMeasurement, "measurement")
}

func TestAParcelOutsideAnAcceptedRequestIsUnavailable(t *testing.T) {
	resolver := parcelResolverWith(t, psdomain.NoDeclaredMeasurementResolution(), psdomain.NoShipmentAddressElements(), nil)

	resolved, err := resolver.ResolvePricingInput(context.Background(), parcelQuery(t, "CNY"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Outcome != ppports.PricingInputUnavailable || !names(resolved.Missing, "not a declared parcel") {
		t.Fatalf("outcome=%s missing=%v", resolved.Outcome, resolved.Missing)
	}
}

// 按包裹那一只不经发生项找包裹：只带发生项的请求到了这里是装错了路，如实答不可得，不去问运输履约。
func TestTheParcelResolverDoesNotKeyOnAnOccurrence(t *testing.T) {
	resolver := parcelResolverWith(t, declaredKG(t, "1.25"), destinationPostal(t, "018956"), nil)

	resolved, err := resolver.ResolvePricingInput(context.Background(), inputQuery(t, ppdomain.PricingPurposeSupplierCost))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Outcome != ppports.PricingInputUnavailable || !names(resolved.Missing, "names no parcel") {
		t.Fatalf("outcome=%s missing=%v", resolved.Outcome, resolved.Missing)
	}
}

func TestAParcelProviderReadErrorIsReturned(t *testing.T) {
	boom := errors.New("shipment requests down")
	resolver, err := adapter.NewParcelResolver(adapter.ParcelResolverDeps{
		Declared:  declaredDouble{err: boom},
		Addresses: addressDouble{elements: destinationPostal(t, "018956")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.ResolvePricingInput(context.Background(), parcelQuery(t, "")); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

// BUY 请求不带结算币种，按发生项那一只造出的快照也就不声明它——评价照旧按卡币种输出。
func TestTheOccurrenceResolverDeclaresNoSettlementCurrencyForARequestWithout(t *testing.T) {
	resolver := resolverWith(t, membersOf(t, objectRef), declaredKG(t, "1.25"), destinationPostal(t, "100115"), nil)

	resolved := resolve(t, resolver, ppdomain.PricingPurposeSupplierCost)
	if _, declared := resolved.Input.SettlementCurrency(); declared {
		t.Fatal("a settlement currency was invented for a request that carried none")
	}
}

func TestTheParcelResolverRefusesMissingProviderPorts(t *testing.T) {
	for name, deps := range map[string]adapter.ParcelResolverDeps{
		"申报测量": {Addresses: addressDouble{}},
		"地址要素": {Declared: declaredDouble{}},
	} {
		if _, err := adapter.NewParcelResolver(deps); err == nil {
			t.Fatalf("%s 缺席却构造成功", name)
		}
	}
	if _, err := adapter.NewResolver(adapter.ResolverDeps{Declared: declaredDouble{}, Addresses: addressDouble{}}); err == nil {
		t.Fatal("成员读口缺席却构造成功")
	}
}
