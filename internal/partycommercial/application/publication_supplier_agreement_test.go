package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件证供应商协议册在应用层的两个方向都接上了（票 admin-write-faces/11）：受控批文那一半的对账门
// 对本册开门（publicationContentOf 一支），载体那一半把正文交回发布用例（declarationsOfContent 一支）。

func supplierAgreementDeclaration(t *testing.T, purchasePlan string) *application.SupplierAgreementBodyDeclaration {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(pubStartsAt, time.Time{})
	if err != nil {
		t.Fatalf("适用区间：%v", err)
	}
	return &application.SupplierAgreementBodyDeclaration{
		Supplier:     pcValue(t, domain.NewPartyID, "supplier-1"),
		LegalEntity:  pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		Scope:        pcValue(t, domain.NewCommercialScopeReference, "scope-procurement"),
		PurchasePlan: pcValue(t, domain.NewPricingPlanReference, purchasePlan),
		Effective:    interval,
	}
}

func supplierAgreementContent(t *testing.T, purchasePlan string) domain.PublicationContent {
	t.Helper()
	declaration := supplierAgreementDeclaration(t, purchasePlan)
	return domain.PublicationContent{Kind: domain.SupplierAgreementObject, SupplierAgreement: &domain.SupplierAgreementBody{
		Supplier:     declaration.Supplier,
		LegalEntity:  declaration.LegalEntity,
		Scope:        declaration.Scope,
		PurchasePlan: declaration.PurchasePlan,
		Effective:    declaration.Effective,
	}}
}

// supplierAgreementSpec 给供应商协议版本壳配上**算出的**内容摘要：本册接进服务端规范化后，对账门要求声明的串
// 与算出的逐字节相等，测试里的壳因此不能再随手写一个（判据同 creditPolicySpec）。
func supplierAgreementSpec(t *testing.T, objectID, label string, body *application.SupplierAgreementBodyDeclaration) domain.CommercialVersionSpec {
	t.Helper()
	spec := publishSpec(t, domain.SupplierAgreementObject, objectID, label)
	canonical, err := domain.CanonicalizePublicationContent(domain.PublicationContent{
		Kind: domain.SupplierAgreementObject,
		SupplierAgreement: &domain.SupplierAgreementBody{
			Supplier:     body.Supplier,
			LegalEntity:  body.LegalEntity,
			Scope:        body.Scope,
			PurchasePlan: body.PurchasePlan,
			Effective:    body.Effective,
		},
	})
	if err != nil {
		t.Fatalf("规范化供应商协议正文：%v", err)
	}
	spec.ContentDigest = canonical.Digest()
	return spec
}

// Covers: ADR-0126 Decision 二 — 供应商协议册接进规范化后对账门对它开门：声明的旧式串与算出的不等即`未受理`，
// 一个字节不写、整册不读，结果带出两个串；壳单独发布（正文缺席）没有可比对象照旧登记。
func TestASupplierAgreementDeclaredDigestIsReconciled(t *testing.T) {
	body := supplierAgreementDeclaration(t, "plan-buy-1")

	t.Run("a mismatching declared digest is not accepted", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.SupplierAgreementObject, "agreement-1", "v1"),
			Approval:     publishApproval(t, "agreement-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{SupplierAgreementBody: body},
		})
		if err != nil {
			t.Fatalf("Handle：%v——未受理不是 error", err)
		}
		if result.Outcome() != application.CommercialPublicationNotAccepted || !errors.Is(result.RefusalCause(), domain.ErrDeclaredDigestMismatch) {
			t.Fatalf("outcome = %q, cause = %v; want NOT_ACCEPTED / ErrDeclaredDigestMismatch", result.Outcome(), result.RefusalCause())
		}
		declared, computed, ok := result.DigestReconciliation()
		if !ok || declared != "sha256:agreement-1-v1" || !strings.HasPrefix(computed, "PCC-1:") {
			t.Fatalf("DigestReconciliation = (%q, %q, %v)", declared, computed, ok)
		}
		if len(registry.savedVersions) != 0 || len(registry.savedSupplier) != 0 || registry.loads != 0 {
			t.Fatalf("未受理写了 %d 版本 / %d 正文、读了 %d 次整册", len(registry.savedVersions), len(registry.savedSupplier), registry.loads)
		}
	})

	t.Run("the computed digest publishes and the body lands", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         supplierAgreementSpec(t, "agreement-1", "v1", body),
			Approval:     publishApproval(t, "agreement-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{SupplierAgreementBody: body},
		})
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.CommercialVersionPublishedEffective || len(registry.savedSupplier) != 1 {
			t.Fatalf("outcome = %q, saved supplier bodies = %d", result.Outcome(), len(registry.savedSupplier))
		}
	})

	t.Run("a shell without its body still publishes", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.SupplierAgreementObject, "agreement-1", "v1"),
			Approval:     publishApproval(t, "agreement-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
		})
		if err != nil || result.Outcome() != application.CommercialVersionPublishedEffective {
			t.Fatalf("outcome = %q, err = %v; want PUBLISHED_EFFECTIVE", result.Outcome(), err)
		}
	})
}

// Covers: ADR-0126 Decision 三 — 供应商协议载体走完录入 → 批准 → 发布：正文随版本同笔登记到供应商协议册、
// 报告一条 SUPPLIER_AGREEMENT_BODY=SAVED、入册摘要就是载体的摘要。
func TestASupplierAgreementDraftPublishesItsBodyThroughTheExistingUseCase(t *testing.T) {
	drafts := newDraftRegistryDouble()
	shell := draftShell(t, domain.SupplierAgreementObject, "agreement-1", "v1")
	submitted, err := application.NewSubmitPublicationDraftHandler(drafts, fixedClock{at: draftSubmittedAt}).Handle(context.Background(),
		application.SubmitPublicationDraftCommand{
			Shell:     shell,
			Content:   supplierAgreementContent(t, "plan-buy-1"),
			Submitter: pcValue(t, domain.NewOperatorSubjectReference, "op-submitter"),
		})
	if err != nil || submitted.Outcome() != application.PublicationDraftSubmitted {
		t.Fatalf("录入：%q, %v", submitted.Outcome(), err)
	}
	rules := approvalDutyRuleDouble{rule: dutyRule(t, true, ""), found: true}
	approved, err := application.NewApprovePublicationDraftHandler(drafts, rules, fixedClock{at: draftApprovedAt}).Handle(context.Background(),
		application.ApprovePublicationDraftCommand{
			Tenant: shell.TenantID, Kind: shell.Kind, ObjectID: shell.ObjectID, Version: shell.Version,
			Approver: operatorSubject(t, "op-approver"),
		})
	if err != nil || approved.Outcome() != application.PublicationDraftApproved {
		t.Fatalf("批准：%q, %v", approved.Outcome(), err)
	}

	registry := &publicationRegistryDouble{}
	publisher := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: draftPublishedAt}, &operatorRegistrationHandoffDouble{})
	result, err := application.NewPublishPublicationDraftHandler(drafts, publisher, fixedClock{at: draftPublishedAt}).Handle(context.Background(),
		application.PublishPublicationDraftCommand{Tenant: shell.TenantID, Kind: shell.Kind, ObjectID: shell.ObjectID, Version: shell.Version})
	if err != nil {
		t.Fatalf("发布：%v", err)
	}
	if result.Outcome() != application.PublicationDraftPublished {
		t.Fatalf("outcome = %q, want DRAFT_PUBLISHED", result.Outcome())
	}
	publication, ok := result.Publication()
	if !ok || publication.Outcome() != application.CommercialVersionPublishedEffective {
		t.Fatalf("publication = %q, %v; want PUBLISHED_EFFECTIVE", publication.Outcome(), ok)
	}
	if len(registry.savedVersions) != 1 || len(registry.savedSupplier) != 1 {
		t.Fatalf("saved %d versions / %d supplier bodies, want 1 / 1", len(registry.savedVersions), len(registry.savedSupplier))
	}
	saved := registry.savedSupplier[0]
	if saved.PurchasePricingPlan().String() != "plan-buy-1" || saved.Supplier().String() != "supplier-1" || saved.Direction() != domain.BuyDirection {
		t.Fatalf("registered body = %#v", saved)
	}
	reports := publication.Declarations()
	if len(reports) != 1 || reports[0].Channel != application.SupplierAgreementBodyChannel || reports[0].Outcome != ports.DeclarationSaved {
		t.Fatalf("报告 = %#v, want SUPPLIER_AGREEMENT_BODY=SAVED 一条", reports)
	}
	stored, _, _ := drafts.LoadDraft(context.Background(), shell.TenantID, shell.Kind, shell.ObjectID, shell.Version)
	if registry.savedVersions[0].ContentDigest() != stored.Canonical().Digest() {
		t.Fatalf("入册摘要 %s ≠ 载体摘要 %s", registry.savedVersions[0].ContentDigest(), stored.Canonical().Digest())
	}
}
