package partycommercial_test

import (
	"context"
	"errors"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/partycommercial"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

func TestNetworkServiceFormRequiresANetworkJudgment(t *testing.T) {
	closure := closureWithProduct(t, true)
	view := newEligibility(t, closure)

	eligibility, err := view.AssessNetworkEligibility(t.Context(), reachabilityKey(t))
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if !eligibility.JudgmentRequired() {
		t.Fatal("NETWORK_SERVICE 被译成了不要求——那是把唯一合法形态做成了不适用默认值")
	}
}

func TestAbsentServiceProductIsDependencyUnavailableNotNotRequired(t *testing.T) {
	closure := closureWithProduct(t, false)
	view := newEligibility(t, closure)

	eligibility, err := view.AssessNetworkEligibility(t.Context(), reachabilityKey(t))
	if !errors.Is(err, adapter.ErrServiceProductUnavailable) {
		t.Fatalf("err = %v, want ErrServiceProductUnavailable", err)
	}
	if eligibility.JudgmentRequired() || eligibility.Requirement() != 0 {
		t.Fatal("产品缺席仍交出了一份资格结论——缺席必须是依赖不可用，不是要求或不要求")
	}
}

func TestUnconfiguredClosureIdentityIsDependencyUnavailable(t *testing.T) {
	closure := closureWithProduct(t, true)
	view, err := adapter.NewCommercialEligibility(newClosureStore(closure), nil)
	if err != nil {
		t.Fatalf("new eligibility: %v", err)
	}

	_, err = view.AssessNetworkEligibility(t.Context(), reachabilityKey(t))
	if !errors.Is(err, adapter.ErrServiceProductUnavailable) {
		t.Fatalf("err = %v, want ErrServiceProductUnavailable", err)
	}
}

func TestEveryConstructableServiceProductFormHasAnExplicitTranslation(t *testing.T) {
	live := effectiveProductVersion(t)
	translated := 0
	for value := pcdomain.ServiceProductForm(0); value < 32; value++ {
		product, err := pcdomain.NewServiceProduct(live, value)
		if err != nil {
			continue
		}
		eligibility, translateErr := translateViaClosure(t, product)
		if translateErr != nil {
			t.Fatalf("form %q (%d) has no translation: %v", product.Form(), value, translateErr)
		}
		if product.Form() == pcdomain.NetworkServiceForm && !eligibility.JudgmentRequired() {
			t.Fatal("NETWORK_SERVICE 没有译成要求")
		}
		translated++
	}
	if translated == 0 {
		t.Fatal("没有构造出任何可翻译的服务形态")
	}
}

func TestRoutingApplicabilityUsesTheSameFormTranslation(t *testing.T) {
	closure := closureWithProduct(t, true)
	view := newRouting(t, closure)

	eligibility, err := view.AssessRoutingApplicability(
		t.Context(), routingKey(t), routingResolution(t, closure))
	if err != nil {
		t.Fatalf("assess routing: %v", err)
	}
	if !eligibility.JudgmentRequired() {
		t.Fatal("初始路由适用性把 NETWORK_SERVICE 译成了不要求")
	}

	absentClosure := closureWithProduct(t, false)
	absent := newRouting(t, absentClosure)
	if _, err := absent.AssessRoutingApplicability(
		t.Context(), routingKey(t), routingResolution(t, absentClosure),
	); !errors.Is(err, adapter.ErrServiceProductUnavailable) {
		t.Fatalf("absent product err = %v, want ErrServiceProductUnavailable", err)
	}
}

func TestRoutingApplicabilityMissingClosureIsDependencyUnavailable(t *testing.T) {
	closure := closureWithProduct(t, true)
	view := newRouting(t, closure)

	_, err := view.AssessRoutingApplicability(
		t.Context(), routingKey(t),
		mustNR(t, nrdomain.NewCommercialResolutionReference, "RES-missing"),
	)
	if !errors.Is(err, adapter.ErrServiceProductUnavailable) {
		t.Fatalf("err = %v, want ErrServiceProductUnavailable", err)
	}
}

func TestRoutingApplicabilityTenantMismatchIsNotUnconfigured(t *testing.T) {
	closure := closureWithProduct(t, true)
	view := newRouting(t, closure)
	foreign := routingKey(t)
	foreign.TenantID = mustNR(t, nrdomain.NewTenantID, "tenant-other")

	_, err := view.AssessRoutingApplicability(
		t.Context(), foreign, routingResolution(t, closure))
	if !errors.Is(err, adapter.ErrRoutingClosureTenantMismatch) {
		t.Fatalf("err = %v, want ErrRoutingClosureTenantMismatch", err)
	}
	if errors.Is(err, adapter.ErrServiceProductUnavailable) {
		t.Fatal("租户不一致被折成了服务产品不可用")
	}
}

func translateViaClosure(t *testing.T, product pcdomain.ServiceProduct) (nrdomain.NetworkEligibility, error) {
	t.Helper()
	closure := rehydratedClosure(t, product, true)
	view := newEligibility(t, closure)
	return view.AssessNetworkEligibility(t.Context(), reachabilityKey(t))
}

func newEligibility(t *testing.T, closure pcdomain.CommercialClosure) *adapter.CommercialEligibility {
	t.Helper()
	view, err := adapter.NewCommercialEligibility(
		newClosureStore(closure),
		fixedReachabilityIdentity{id: closure.ResolutionID()},
	)
	if err != nil {
		t.Fatalf("new eligibility: %v", err)
	}
	return view
}

func newRouting(t *testing.T, closure pcdomain.CommercialClosure) *adapter.RoutingApplicability {
	t.Helper()
	view, err := adapter.NewRoutingApplicability(newClosureStore(closure))
	if err != nil {
		t.Fatalf("new routing: %v", err)
	}
	return view
}

func routingResolution(t *testing.T, closure pcdomain.CommercialClosure) nrdomain.CommercialResolutionReference {
	t.Helper()
	return mustNR(t, nrdomain.NewCommercialResolutionReference, closure.ResolutionID().String())
}

type closureStoreDouble struct {
	closure pcdomain.CommercialClosure
}

func newClosureStore(closure pcdomain.CommercialClosure) *closureStoreDouble {
	return &closureStoreDouble{closure: closure}
}

func (double *closureStoreDouble) LoadResolution(
	_ context.Context,
	_ pcdomain.TenantID,
	id pcdomain.ResolutionID,
) (pcdomain.CommercialClosure, bool, error) {
	if double.closure.ResolutionID() != id {
		return pcdomain.CommercialClosure{}, false, nil
	}
	return double.closure, true, nil
}

func (double *closureStoreDouble) Save(_ context.Context, _ pcdomain.CommercialClosure) (pcports.ResolutionSaveOutcome, error) {
	return pcports.ResolutionSaveOutcomeInvalid, errors.New("save is not used by the eligibility adapter")
}

type fixedReachabilityIdentity struct {
	id pcdomain.ResolutionID
}

func (identity fixedReachabilityIdentity) ResolutionID(
	_ context.Context,
	_ nrdomain.ReachabilityJudgmentKey,
) (pcdomain.ResolutionID, bool, error) {
	return identity.id, true, nil
}

func closureWithProduct(t *testing.T, registerProduct bool) pcdomain.CommercialClosure {
	t.Helper()
	version := effectiveProductVersion(t)
	var product pcdomain.ServiceProduct
	if registerProduct {
		built, err := pcdomain.NewServiceProduct(version, pcdomain.NetworkServiceForm)
		if err != nil {
			t.Fatalf("new service product: %v", err)
		}
		product = built
	}
	return rehydratedClosure(t, product, registerProduct)
}

func rehydratedClosure(t *testing.T, product pcdomain.ServiceProduct, withProduct bool) pcdomain.CommercialClosure {
	t.Helper()
	version := effectiveProductVersion(t)
	if withProduct {
		version = product.Version()
	}
	anchor := selectionAnchor(t)
	spec := pcdomain.RehydrateAdoptedBasisSpec{
		Kind:    pcdomain.ServiceProductObject,
		Version: version,
	}
	if withProduct {
		spec.ServiceProduct = product
		spec.HasServiceProduct = true
	}
	closure, err := pcdomain.RehydrateCommercialClosure(pcdomain.RehydrateCommercialClosureSpec{
		Outcome:      pcdomain.UniquelyResolved,
		ResolutionID: mustPC(t, pcdomain.NewResolutionID, "CLO-nr-form-1"),
		Key: pcdomain.ClosureResolutionKey{
			TenantID:             mustPC(t, pcdomain.NewTenantID, "tenant-1"),
			CustomerAccountID:    mustPC(t, pcdomain.NewCustomerAccountID, "customer-1"),
			LegalEntityCandidate: mustPC(t, pcdomain.NewLegalEntityReference, "legal-1"),
			Scope:                mustPC(t, pcdomain.NewCommercialScopeReference, "scope-a"),
			Purpose:              pcdomain.AcceptanceControlPurpose,
			Anchor:               anchor,
			RequiredBases:        []pcdomain.CommercialObjectKind{pcdomain.ServiceProductObject},
		},
		Anchor:       anchor,
		ViewRevision: mustPC(t, pcdomain.NewAuthorityViewRevision, "VIEW-nr-1"),
		Adopted:      []pcdomain.RehydrateAdoptedBasisSpec{spec},
	})
	if err != nil {
		t.Fatalf("rehydrate closure: %v", err)
	}
	return closure
}

func effectiveProductVersion(t *testing.T) pcdomain.CommercialVersion {
	t.Helper()
	at := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	interval, err := pcdomain.NewEffectiveInterval(at, at.Add(90*24*time.Hour))
	if err != nil {
		t.Fatalf("interval: %v", err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		mustPC(t, pcdomain.NewApprovalReference, "approval-product"),
		mustPC(t, pcdomain.NewCommercialSourceReference, "source-product"),
		at.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("approval: %v", err)
	}
	version, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      mustPC(t, pcdomain.NewTenantID, "tenant-1"),
		Kind:          pcdomain.ServiceProductObject,
		ObjectID:      mustPC(t, pcdomain.NewCommercialObjectID, "product-1"),
		Version:       mustPC(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Scope:         mustPC(t, pcdomain.NewCommercialScopeReference, "scope-a"),
		ContentDigest: mustPC(t, pcdomain.NewCommercialContentDigest, "sha256:product-1"),
		Effective:     interval,
		Status:        pcdomain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   at.Add(-time.Hour),
		EffectiveAt:   at,
	})
	if err != nil {
		t.Fatalf("rehydrate product version: %v", err)
	}
	return version
}

func selectionAnchor(t *testing.T) pcdomain.SelectionAnchor {
	t.Helper()
	anchor, err := pcdomain.NewSelectionAnchor(
		time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC),
		mustPC(t, pcdomain.NewAnchorPolicyVersion, "anchor-policy-v1"),
	)
	if err != nil {
		t.Fatalf("anchor: %v", err)
	}
	return anchor
}

func reachabilityKey(t *testing.T) nrdomain.ReachabilityJudgmentKey {
	t.Helper()
	asOf, err := nrdomain.NewJudgmentAsOf(
		mustNR(t, nrdomain.NewAsOfSemantic, "acceptance-as-of"),
		time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC),
		mustNR(t, nrdomain.NewAsOfStrategyVersion, "asof-v1"),
	)
	if err != nil {
		t.Fatalf("asOf: %v", err)
	}
	return nrdomain.ReachabilityJudgmentKey{
		TenantID:          mustNR(t, nrdomain.NewTenantID, "tenant-1"),
		CustomerAccountID: mustNR(t, nrdomain.NewCustomerAccountID, "customer-1"),
		ShipmentRequestID: mustNR(t, nrdomain.NewShipmentRequestID, "shipment-1"),
		SubmissionVersion: mustNR(t, nrdomain.NewSubmissionVersionID, "sub-v1"),
		DeclaredParcelID:  mustNR(t, nrdomain.NewDeclaredParcelID, "parcel-1"),
		ServicePurpose:    mustNR(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"),
		AsOf:              asOf,
	}
}

func routingKey(t *testing.T) nrdomain.InitialRouteJudgmentKey {
	t.Helper()
	return nrdomain.InitialRouteJudgmentKey{
		TenantID:           mustNR(t, nrdomain.NewTenantID, "tenant-1"),
		CustomerAccountID:  mustNR(t, nrdomain.NewCustomerAccountID, "customer-1"),
		ShipmentRequestID:  mustNR(t, nrdomain.NewShipmentRequestID, "shipment-1"),
		AcceptanceBaseline: mustNR(t, nrdomain.NewAcceptanceBaselineReference, "baseline-1"),
		DeclaredParcelID:   mustNR(t, nrdomain.NewDeclaredParcelID, "parcel-1"),
		ServicePurpose:     mustNR(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"),
	}
}

func mustPC[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	got, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return got
}

func mustNR[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	got, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return got
}
