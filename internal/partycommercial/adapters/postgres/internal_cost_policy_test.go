package postgres_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件对真实 PostgreSQL 16 证内部成本政策点读口（票 routing-first-cut/10，ADR-0148 决定四）：
// 按版本身份点读、方向照登记原样交回、没登记是合法缺席、版本标签区分同一对象的两个版本、租户隔离。

func TestInternalCostPolicyLoadsByVersionKey(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.PriceRuleObject, "policy-internal", "v1", "digest-internal-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	policy := pricePolicyOn(t, version, domain.InternalDirection, domain.InternalDirection,
		domain.PlanBindingConversionNone, "plan-internal")
	mustSavePricePolicy(t, transactor, ctx, repository, policy, domain.InternalDirection, domain.PlanBindingConversionNone)

	loaded, found, err := repository.LoadInternalCostPolicy(ctx, pcTenant(t, "tenant-1"), version.ObjectID(), version.Version())
	if err != nil {
		t.Fatalf("点读：%v", err)
	}
	if !found {
		t.Fatal("登记过的内部政策答了没找到")
	}
	if loaded.Direction() != domain.InternalDirection {
		t.Fatalf("direction = %q", loaded.Direction())
	}
	if loaded.PricingPlan().String() != "plan-internal" {
		t.Fatalf("plan = %q", loaded.PricingPlan())
	}
	if !loaded.Version().SameVersionAs(version) {
		t.Fatal("政策挂回了另一个版本")
	}
}

// 方向照登记原样交回：本口不做方向筛，「引用指错方向」留给消费侧桥拒译。
func TestInternalCostPolicyReturnsTheRegisteredDirectionUnfiltered(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.PriceRuleObject, "policy-sell", "v1", "digest-sell-1")
	mustSaveVersion(t, transactor, ctx, repository, version)
	policy := pricePolicyOn(t, version, domain.SellDirection, domain.SellDirection,
		domain.PlanBindingConversionNone, "plan-sell")
	mustSavePricePolicy(t, transactor, ctx, repository, policy, domain.SellDirection, domain.PlanBindingConversionNone)

	loaded, found, err := repository.LoadInternalCostPolicy(ctx, pcTenant(t, "tenant-1"), version.ObjectID(), version.Version())
	if err != nil {
		t.Fatalf("点读：%v", err)
	}
	if !found || loaded.Direction() != domain.SellDirection {
		t.Fatalf("found = %v direction = %q, want 照登记原样交回 SELL", found, loaded.Direction())
	}
}

func TestInternalCostPolicyVersionLabelsStayApart(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	first := effectiveVersionOfKind(t, domain.PriceRuleObject, "policy-v", "v1", "digest-v1")
	mustSaveVersion(t, transactor, ctx, repository, first)
	mustSavePricePolicy(t, transactor, ctx, repository,
		pricePolicyOn(t, first, domain.InternalDirection, domain.InternalDirection, domain.PlanBindingConversionNone, "plan-v1"),
		domain.InternalDirection, domain.PlanBindingConversionNone)

	second := effectiveVersionOfKind(t, domain.PriceRuleObject, "policy-v", "v2", "digest-v2")
	mustSaveVersion(t, transactor, ctx, repository, second)
	mustSavePricePolicy(t, transactor, ctx, repository,
		pricePolicyOn(t, second, domain.InternalDirection, domain.InternalDirection, domain.PlanBindingConversionNone, "plan-v2"),
		domain.InternalDirection, domain.PlanBindingConversionNone)

	loadedV1, found, err := repository.LoadInternalCostPolicy(ctx, pcTenant(t, "tenant-1"), first.ObjectID(), first.Version())
	if err != nil || !found || loadedV1.PricingPlan().String() != "plan-v1" {
		t.Fatalf("v1 = plan %q found %v err %v, want plan-v1", loadedV1.PricingPlan(), found, err)
	}
	loadedV2, found, err := repository.LoadInternalCostPolicy(ctx, pcTenant(t, "tenant-1"), second.ObjectID(), second.Version())
	if err != nil || !found || loadedV2.PricingPlan().String() != "plan-v2" {
		t.Fatalf("v2 = plan %q found %v err %v, want plan-v2", loadedV2.PricingPlan(), found, err)
	}
}

func TestInternalCostPolicyMissingBodyIsALawfulAbsence(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.PriceRuleObject, "policy-bodiless", "v1", "digest-bodiless")
	mustSaveVersion(t, transactor, ctx, repository, version)

	_, found, err := repository.LoadInternalCostPolicy(ctx, pcTenant(t, "tenant-1"), version.ObjectID(), version.Version())
	if err != nil {
		t.Fatalf("没登正文不该报错：%v", err)
	}
	if found {
		t.Fatal("没登正文的政策答了找到")
	}
}

func TestInternalCostPolicyAbsentVersionIsALawfulAbsence(t *testing.T) {
	repository, _, _ := newPublications(t)
	ctx := t.Context()

	objectID, err := domain.NewCommercialObjectID("policy-never")
	if err != nil {
		t.Fatalf("object id: %v", err)
	}
	label, err := domain.NewCommercialVersionLabel("v1")
	if err != nil {
		t.Fatalf("version label: %v", err)
	}
	_, found, err := repository.LoadInternalCostPolicy(ctx, pcTenant(t, "tenant-1"), objectID, label)
	if err != nil {
		t.Fatalf("没有版本行不该报错：%v", err)
	}
	if found {
		t.Fatal("从没发布的政策答了找到")
	}
}

func TestInternalCostPolicyStaysInsideItsOwnTenant(t *testing.T) {
	repository, transactor, _ := newPublications(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.PriceRuleObject, "policy-tenant", "v1", "digest-tenant")
	mustSaveVersion(t, transactor, ctx, repository, version)
	mustSavePricePolicy(t, transactor, ctx, repository,
		pricePolicyOn(t, version, domain.InternalDirection, domain.InternalDirection, domain.PlanBindingConversionNone, "plan-tenant"),
		domain.InternalDirection, domain.PlanBindingConversionNone)

	_, found, err := repository.LoadInternalCostPolicy(ctx, pcTenant(t, "tenant-2"), version.ObjectID(), version.Version())
	if err != nil {
		t.Fatalf("跨租户点读：%v", err)
	}
	if found {
		t.Fatal("别的租户读到了这份政策正文")
	}
}
