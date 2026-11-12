package postgres_test

import (
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 16 证 ports.AcceptedServiceProductView 与提交版本申报读口带出的服务产品（ADR-0382 决定三）：已接受委托
// 申报了产品，成员都答那个产品；没申报、仅已提交、他租户与从未声明的包裹都答没有产品；受理前按提交版本读的申报同样带出产品。
// 夹具走真接受（Decide → Save），合成 SYN- 值，证据只记 S。

var _ ports.AcceptedServiceProductView = (*adapter.ShipmentRequests)(nil)

func submittedWithProduct(t *testing.T, key, requestID, product string) domain.ShipmentRequest {
	t.Helper()
	spec := submittedShipmentRequestSpec(t, key, requestID)
	if product != "" {
		spec.RequestedProduct = domain.RequestedServiceProductOf([]domain.CanonicalContentEntry{
			mustEntry(t, domain.RequestedServiceProductEntryName, product),
		})
	}
	request, err := domain.SubmitShipmentRequest(spec)
	if err != nil {
		t.Fatalf("建单：%v", err)
	}
	return request
}

func loadAcceptedProduct(t *testing.T, view ports.AcceptedServiceProductView, tenant, parcel string) domain.DeclaredServiceProduct {
	t.Helper()
	product, err := view.LoadAcceptedServiceProduct(t.Context(), mustBuild(t, domain.NewTenantID, tenant), mustBuild(t, domain.NewDeclaredParcelID, parcel))
	if err != nil {
		t.Fatalf("取已接受委托申报的产品（%s / %s）：%v", tenant, parcel, err)
	}
	return product
}

func TestTheAcceptedServiceProductIsTheOneTheAcceptedRequestDeclared(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()
	mustInsert(t, transactor, ctx, repository, submittedWithProduct(t, "req-key-1", "request-1", "SYN-PROD-CN-SG-FBA"))
	accepted, err := mustFind(t, repository, ctx, "req-key-1").Decide(acceptanceDecisionSpec(t))
	if err != nil {
		t.Fatalf("形成接受：%v", err)
	}
	mustSave(t, transactor, ctx, repository, requestIdentity(t, "req-key-1"), accepted)

	for _, parcel := range []string{"parcel-1", "parcel-2"} {
		if got := loadAcceptedProduct(t, repository, "tenant-1", parcel); got.String() != "SYN-PROD-CN-SG-FBA" {
			t.Fatalf("%s 的产品 = %q，want SYN-PROD-CN-SG-FBA", parcel, got.String())
		}
	}
	for name, item := range map[string][2]string{
		"从未声明过的包裹":   {"tenant-1", "parcel-9"},
		"他租户问本租户的成员": {"tenant-b", "parcel-1"},
	} {
		if got := loadAcceptedProduct(t, repository, item[0], item[1]); got.Declared() {
			t.Fatalf("%s 答了产品 %q", name, got.String())
		}
	}
}

func TestAnUndeclaredOrMerelySubmittedRequestHasNoAcceptedServiceProduct(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()
	mustInsert(t, transactor, ctx, repository, submittedWithProduct(t, "req-key-1", "request-1", ""))
	accepted, err := mustFind(t, repository, ctx, "req-key-1").Decide(acceptanceDecisionSpec(t))
	if err != nil {
		t.Fatalf("形成接受：%v", err)
	}
	mustSave(t, transactor, ctx, repository, requestIdentity(t, "req-key-1"), accepted)
	if got := loadAcceptedProduct(t, repository, "tenant-1", "parcel-1"); got.Declared() {
		t.Fatalf("没申报产品的已接受委托答了 %q", got.String())
	}

	submitted, submittedTransactor, _ := newShipmentRequests(t)
	mustInsert(t, submittedTransactor, ctx, submitted, submittedWithProduct(t, "req-key-2", "request-2", "SYN-PROD-CN-SG-FBA"))
	if got := loadAcceptedProduct(t, submitted, "tenant-1", "parcel-1"); got.Declared() {
		t.Fatalf("仅已提交的委托答了 %q", got.String())
	}
}

func TestTheSubmissionVersionDeclarationCarriesTheDeclaredServiceProduct(t *testing.T) {
	repository, transactor, _ := newShipmentRequests(t)
	ctx := t.Context()
	mustInsert(t, transactor, ctx, repository, submittedWithProduct(t, "req-key-1", "request-1", "SYN-PROD-CN-SG-FBA"))
	mustInsert(t, transactor, ctx, repository, submittedWithProduct(t, "req-key-2", "request-2", ""))

	for key, want := range map[string]string{"req-key-1": "SYN-PROD-CN-SG-FBA", "req-key-2": ""} {
		request := mustFind(t, repository, ctx, key)
		declaration, found, err := repository.LoadSubmissionVersionDeclaration(ctx, mustBuild(t, domain.NewTenantID, "tenant-1"),
			request.ShipmentRequestID(), request.CurrentSubmissionVersion().VersionID())
		if err != nil || !found {
			t.Fatalf("%s：found=%v err=%v", key, found, err)
		}
		if got := declaration.RequestedServiceProduct(); got.String() != want || got.Declared() != (want != "") {
			t.Fatalf("%s 的申报产品 = %q，want %q", key, got.String(), want)
		}
	}
}
