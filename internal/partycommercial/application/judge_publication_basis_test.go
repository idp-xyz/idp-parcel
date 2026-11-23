package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

func TestPublicationBasisFollowsSendsAndTheTenantRule(t *testing.T) {
	tenant := publicationTenant(t)
	view := &publicationView{digest: "digest-1", quotationID: "QTS-1"}
	query := application.PublicationBasisQuery{Tenant: tenant, PlanID: "PLAN", PlanVersion: "v1", ContentDigest: "digest-1"}

	view.sends = nil
	got, err := application.JudgePublicationBasis(context.Background(), view, query)
	if err != nil || got != application.PublicationBasisNotApplicable {
		t.Fatalf("没有发出 = %s %v", got, err)
	}

	view.sends = []ports.QuotationSendRef{{ID: "QTS-1", Digest: "digest-1"}}
	got, err = application.JudgePublicationBasis(context.Background(), view, query)
	if err != nil || got != application.PublicationBasisUnconfigured {
		t.Fatalf("未登记 = %s %v", got, err)
	}

	view.configured, view.requires = true, true
	got, err = application.JudgePublicationBasis(context.Background(), view, query)
	if err != nil || got != application.PublicationBasisAcceptanceRequired {
		t.Fatalf("要接受 = %s %v", got, err)
	}

	query.Basis = "QTS-1"
	view.accepted = true
	got, err = application.JudgePublicationBasis(context.Background(), view, query)
	if err != nil || got != application.PublicationBasisSatisfied || !got.AllowsPublication() {
		t.Fatalf("对上了 = %s %v", got, err)
	}

	query.Basis = "QTS-OTHER"
	got, err = application.JudgePublicationBasis(context.Background(), view, query)
	if err != nil || got != application.PublicationBasisCitationRejected || got.AllowsPublication() {
		t.Fatalf("对不上 = %s %v", got, err)
	}
}

func publicationTenant(t *testing.T) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	return tenant
}

type publicationView struct {
	sends       []ports.QuotationSendRef
	configured  bool
	requires    bool
	accepted    bool
	digest      string
	quotationID string
}

func (view *publicationView) LoadQuotationPublicationRequirement(context.Context, domain.TenantID) (bool, bool, error) {
	return view.requires, view.configured, nil
}

func (view *publicationView) SaveQuotationPublicationRequirement(context.Context, domain.QuotationPublicationRequirement) (bool, bool, error) {
	return false, true, nil
}

func (view *publicationView) ListQuotationSendsForPlan(context.Context, domain.TenantID, string, string) ([]ports.QuotationSendRef, error) {
	return view.sends, nil
}

func (view *publicationView) AcceptedQuotationReply(context.Context, domain.TenantID, string) (string, bool, bool, error) {
	if !view.accepted {
		return "", false, false, nil
	}
	return view.digest, true, true, nil
}

func (view *publicationView) ContractNamesPricingScope(context.Context, domain.TenantID, string, string) (bool, error) {
	return false, nil
}
