package postgres_test

import (
	"reflect"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证商业版本按身份读回（ADR-0361 决定二）：Find… 是 List… 的投影——同一条语句多两格键过滤，
// 读回的那一行与列表上的那一行逐字段相同；键差一字、别的租户、未登记的类别三者同答不在册。

// Covers: ADR-0361 决定二——按身份读回的服务产品行与目录列表上的同一行逐字段相同（含形态与收尾回显四格）；键不在册答 false 不答错。
func TestFindServiceProductIsTheRowTheCatalogueLists(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	withForm := productVersionInTenant(t, "tenant-1", "product-1", "v1", "digest-p1")
	bare := productVersionInTenant(t, "tenant-1", "product-2", "v1", "digest-p2")
	mustSaveVersion(t, transactor, ctx, repository, withForm)
	mustSaveVersion(t, transactor, ctx, repository, bare)
	mustSaveServiceProduct(t, transactor, ctx, repository, serviceProductOf(t, withForm))

	listed, err := rowsOf(catalogue.ListServiceProducts(ctx, tenant, 10, firstPage(t, ports.ServiceProductPages)))
	if err != nil || len(listed) != 2 {
		t.Fatalf("上列服务产品：%d 行，err = %v", len(listed), err)
	}
	for _, expected := range listed {
		found, present, err := catalogue.FindServiceProduct(ctx, tenant, pcValue(t, domain.NewCommercialObjectID, expected.ObjectID), pcValue(t, domain.NewCommercialVersionLabel, expected.VersionLabel))
		if err != nil || !present {
			t.Fatalf("按身份读回 %s：present=%v err=%v", expected.ObjectID, present, err)
		}
		if !reflect.DeepEqual(found, expected) {
			t.Fatalf("按身份读回与目录行不同：\n读回 %+v\n列表 %+v", found, expected)
		}
	}

	if _, present, err := catalogue.FindServiceProduct(ctx, tenant, withForm.ObjectID(), pcValue(t, domain.NewCommercialVersionLabel, "v2")); err != nil || present {
		t.Fatalf("不在册的版本：present=%v err=%v，want false 且无错", present, err)
	}
}

// Covers: ADR-0361 决定二——供应商协议按身份读回与列表同行；两租户共用同一个对象标识与版本号时各读各的，
// 从未登记的租户与别的类别同键（服务产品 product-1/v1 不是协议）同答不在册。
func TestFindSupplierAgreementIsTenantIsolatedAndKindBound(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()

	mine := effectiveSupplierAgreement(t, "tenant-1", "agreement-1", "v1", "digest-a1")
	theirs := effectiveSupplierAgreement(t, "tenant-2", "agreement-1", "v1", "digest-a2")
	product := productVersionInTenant(t, "tenant-1", "product-1", "v1", "digest-p1")
	mustSaveVersion(t, transactor, ctx, repository, mine)
	mustSaveVersion(t, transactor, ctx, repository, theirs)
	mustSaveVersion(t, transactor, ctx, repository, product)

	for _, tenant := range []string{"tenant-1", "tenant-2"} {
		listed, err := rowsOf(catalogue.ListSupplierAgreements(ctx, pcTenant(t, tenant), 10, firstPage(t, ports.SupplierAgreementPages)))
		if err != nil || len(listed) != 1 {
			t.Fatalf("%s 上列协议：%d 行，err = %v", tenant, len(listed), err)
		}
		found, present, err := catalogue.FindSupplierAgreement(ctx, pcTenant(t, tenant), mine.ObjectID(), mine.Version())
		if err != nil || !present {
			t.Fatalf("%s 按身份读回：present=%v err=%v", tenant, present, err)
		}
		if !reflect.DeepEqual(found, listed[0]) {
			t.Fatalf("%s 按身份读回与目录行不同：\n读回 %+v\n列表 %+v", tenant, found, listed[0])
		}
	}

	if _, present, err := catalogue.FindSupplierAgreement(ctx, pcTenant(t, "tenant-9"), mine.ObjectID(), mine.Version()); err != nil || present {
		t.Fatalf("从未登记的租户：present=%v err=%v，want false 且无错", present, err)
	}
	if _, present, err := catalogue.FindSupplierAgreement(ctx, pcTenant(t, "tenant-1"), product.ObjectID(), product.Version()); err != nil || present {
		t.Fatalf("别的类别同键：present=%v err=%v，want false 且无错（类别在语句的 WHERE 上，不靠对象标识区分）", present, err)
	}
}
