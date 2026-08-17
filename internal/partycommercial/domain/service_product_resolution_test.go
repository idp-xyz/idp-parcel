package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func registerServiceProductIn(
	t *testing.T,
	registry *domain.CommercialRegistry,
	scope, objectID string,
) domain.ServiceProduct {
	t.Helper()
	version := effectiveIn(t, registry, domain.ServiceProductObject, objectID, "v1", "sha256:"+objectID, scope)
	product, err := domain.NewServiceProduct(version, domain.NetworkServiceForm)
	if err != nil {
		t.Fatalf("new service product: %v", err)
	}
	registry.RegisterServiceProduct(product)
	return product
}

// Covers: ADR-0050 — 采用了服务产品时闭包携带整个产品，形态因此可观察；其余依据缺席。
func TestAdoptedServiceProductIsObservableOnTheClosure(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", closureBases...)
	registerServiceProductIn(t, registry, "scope-a", "product-1")

	required := append([]domain.CommercialObjectKind{}, closureBases...)
	required = append(required, domain.ServiceProductObject)

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", required...), nil)
	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", closure.Outcome())
	}

	adopted, ok := closure.AdoptedFor(domain.ServiceProductObject)
	if !ok {
		t.Fatal("closure did not adopt the service product")
	}
	product, present := adopted.ServiceProduct()
	if !present {
		t.Fatal("adopted service product basis has no ServiceProduct — form is still unobservable")
	}
	if product.Form() != domain.NetworkServiceForm {
		t.Fatalf("form = %q, want NETWORK_SERVICE", product.Form())
	}

	contract, ok := closure.AdoptedFor(domain.CustomerContractObject)
	if !ok {
		t.Fatal("closure lost the contract member")
	}
	if _, leaked := contract.ServiceProduct(); leaked {
		t.Fatal("非服务产品成员也带上了服务产品")
	}
}

// Covers: ADR-0050 第三条与第四条 — 只登版本未登产品时 ServiceProduct() 缺席，解析仍是
// 唯一解析，不退化成无适用依据。
func TestServiceProductAbsenceDoesNotDegradeAUniqueResolution(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", closureBases...)
	effectiveIn(t, registry, domain.ServiceProductObject, "product-1", "v1", "sha256:product-1", "scope-a")

	required := append([]domain.CommercialObjectKind{}, closureBases...)
	required = append(required, domain.ServiceProductObject)

	closure := domain.ResolveCommercialClosure(registry, closureKey(t, "scope-a", required...), nil)
	if closure.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED; product absence must not degrade the verdict", closure.Outcome())
	}
	adopted, ok := closure.AdoptedFor(domain.ServiceProductObject)
	if !ok {
		t.Fatal("version-only product was not adopted")
	}
	if _, present := adopted.ServiceProduct(); present {
		t.Fatal("unregistered product object still became observable")
	}
}

// 只改产品、不动版本正文时解析身份仍须变：服务产品参与 ViewRevision 派生（ADR-0050
// Consequence 第三条，对齐 ADR-0044）。AT-PC-024 要求相同输入与相同修订返回原解析——
// 登记产品后修订不再相同，因此换身份不是冲突，而是让消费方看得见形态从缺席变成在场。
func TestRegisteringAServiceProductAdvancesTheViewRevision(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	tenant := commercialValue(t, domain.NewTenantID, "tenant-1")
	scope := commercialValue(t, domain.NewCommercialScopeReference, "scope-a")

	version := effectiveIn(t, registry, domain.ServiceProductObject, "product-1", "v1", "sha256:p1", "scope-a")
	before := registry.ViewRevision(tenant, scope)

	product, err := domain.NewServiceProduct(version, domain.NetworkServiceForm)
	if err != nil {
		t.Fatalf("new service product: %v", err)
	}
	registry.RegisterServiceProduct(product)

	if registry.ViewRevision(tenant, scope) == before {
		t.Fatal("登记服务产品没有推进视图修订——先前解析的失效检测看不见形态从缺席变成在场")
	}
}

// Covers: AT-PC-024 — 版本与产品都已在册时，相同输入与相同修订重复解析返回原身份。
func TestRepeatedProductResolutionIsStableUnderTheSameView(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	seedClosure(t, registry, "scope-a", closureBases...)
	registerServiceProductIn(t, registry, "scope-a", "product-1")

	required := append([]domain.CommercialObjectKind{}, closureBases...)
	required = append(required, domain.ServiceProductObject)
	key := closureKey(t, "scope-a", required...)

	first := domain.ResolveCommercialClosure(registry, key, nil)
	second := domain.ResolveCommercialClosure(registry, key, nil)
	if first.Outcome() != domain.UniquelyResolved || first.ResolutionID() != second.ResolutionID() {
		t.Fatalf("repeated resolution diverged under one view: %q/%q vs %q/%q",
			first.Outcome(), first.ResolutionID(), second.Outcome(), second.ResolutionID())
	}
	firstView, _ := first.ViewRevision()
	secondView, _ := second.ViewRevision()
	if firstView != secondView {
		t.Fatal("an unchanged registry reported two different view revisions")
	}
}
