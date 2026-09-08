package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件证授权规则册在应用层的两个方向都接上了（票 admin-write-faces/17）：受控批文那一半的对账门对本册开门
// （publicationContentOf 一支——正文就是取消授权目录），载体那一半把目录交回发布用例（declarationsOfContent 一支）。

func cancellationRows(t *testing.T, customerRule, operationsRule string) []domain.CancellationAuthorityDeclaration {
	t.Helper()
	rows := []domain.CancellationAuthorityDeclaration{
		{Party: domain.DeclaredCustomerCancellation, Rule: pcValue(t, domain.NewRuleReference, customerRule)},
	}
	if operationsRule != "" {
		rows = append(rows, domain.CancellationAuthorityDeclaration{
			Party: domain.DeclaredOperationsCancellation, Rule: pcValue(t, domain.NewRuleReference, operationsRule),
		})
	}
	return rows
}

func authorizationRuleContent(rows []domain.CancellationAuthorityDeclaration) domain.PublicationContent {
	return domain.PublicationContent{
		Kind:              domain.AuthorizationRuleObject,
		AuthorizationRule: &domain.AuthorizationRuleBody{CancellationAuthority: rows},
	}
}

// authorizationRuleSpec 给授权规则版本壳配上**算出的**内容摘要：本册接进服务端规范化后，对账门要求声明的串与算出的
// 逐字节相等，测试里的壳因此不能再随手写一个（判据同 supplierAgreementSpec）。
func authorizationRuleSpec(t *testing.T, objectID, label string, rows []domain.CancellationAuthorityDeclaration) domain.CommercialVersionSpec {
	t.Helper()
	spec := publishSpec(t, domain.AuthorizationRuleObject, objectID, label)
	canonical, err := domain.CanonicalizePublicationContent(authorizationRuleContent(rows))
	if err != nil {
		t.Fatalf("规范化授权规则正文：%v", err)
	}
	spec.ContentDigest = canonical.Digest()
	return spec
}

// Covers: ADR-0126 Decision 二 — 授权规则册接进规范化后对账门对它开门：声明的旧式串与算出的不等即`未受理`，一个字节
// 不写、整册不读，结果带出两个串；算出的串照常发布并登记目录；壳单独发布（零行）没有可比对象照旧登记。
func TestAnAuthorizationRuleDeclaredDigestIsReconciled(t *testing.T) {
	rows := cancellationRows(t, "CANCEL/customer-before-intake", "CANCEL/operations-any-time")

	t.Run("a mismatching declared digest is not accepted", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.AuthorizationRuleObject, "authz-1", "v1"),
			Approval:     publishApproval(t, "authz-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{CancellationAuthority: rows},
		})
		if err != nil {
			t.Fatalf("Handle：%v——未受理不是 error", err)
		}
		if result.Outcome() != application.CommercialPublicationNotAccepted || !errors.Is(result.RefusalCause(), domain.ErrDeclaredDigestMismatch) {
			t.Fatalf("outcome = %q, cause = %v; want NOT_ACCEPTED / ErrDeclaredDigestMismatch", result.Outcome(), result.RefusalCause())
		}
		declared, computed, ok := result.DigestReconciliation()
		if !ok || declared != "sha256:authz-1-v1" || !strings.HasPrefix(computed, "PCC-1:") {
			t.Fatalf("DigestReconciliation = (%q, %q, %v)", declared, computed, ok)
		}
		if len(registry.savedVersions) != 0 || registry.loads != 0 {
			t.Fatalf("未受理写了 %d 版本、读了 %d 次整册", len(registry.savedVersions), registry.loads)
		}
	})

	t.Run("the computed digest publishes and the catalogue lands", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         authorizationRuleSpec(t, "authz-1", "v1", rows),
			Approval:     publishApproval(t, "authz-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{CancellationAuthority: rows},
		})
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.CommercialVersionPublishedEffective || len(registry.savedVersions) != 1 {
			t.Fatalf("outcome = %q, saved versions = %d", result.Outcome(), len(registry.savedVersions))
		}
		reports := result.Declarations()
		if len(reports) != 1 || reports[0].Channel != application.CancellationAuthorityChannel || reports[0].Outcome != ports.DeclarationSaved {
			t.Fatalf("报告 = %#v, want CANCELLATION_AUTHORITY=SAVED 一条", reports)
		}
	})

	t.Run("a shell without its catalogue still publishes", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.AuthorizationRuleObject, "authz-1", "v1"),
			Approval:     publishApproval(t, "authz-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
		})
		if err != nil || result.Outcome() != application.CommercialVersionPublishedEffective {
			t.Fatalf("outcome = %q, err = %v; want PUBLISHED_EFFECTIVE", result.Outcome(), err)
		}
	})

	t.Run("the same party twice is refused by the catalogue gate before any write", func(t *testing.T) {
		duplicated := append(cancellationRows(t, "CANCEL/customer-a", ""), cancellationRows(t, "CANCEL/customer-b", "")...)
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.AuthorizationRuleObject, "authz-1", "v1"),
			Approval:     publishApproval(t, "authz-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{CancellationAuthority: duplicated},
		})
		if err != nil {
			t.Fatalf("Handle：%v——未受理不是 error", err)
		}
		if result.Outcome() != application.CommercialPublicationNotAccepted || !errors.Is(result.RefusalCause(), domain.ErrConflictingCancellationAuthority) {
			t.Fatalf("outcome = %q, cause = %v; want NOT_ACCEPTED / ErrConflictingCancellationAuthority", result.Outcome(), result.RefusalCause())
		}
		if len(registry.savedVersions) != 0 {
			t.Fatal("同一请求方两行写了库")
		}
	})
}

// Covers: ADR-0126 Decision 三 — 授权规则载体走完录入 → 批准 → 发布：目录随版本同笔登记、报告一条
// CANCELLATION_AUTHORITY=SAVED、入册摘要就是载体的摘要。
func TestAnAuthorizationRuleDraftPublishesItsCatalogueThroughTheExistingUseCase(t *testing.T) {
	drafts := newDraftRegistryDouble()
	shell := draftShell(t, domain.AuthorizationRuleObject, "authz-1", "v1")
	rows := cancellationRows(t, "CANCEL/customer-before-intake", "")
	submitted, err := application.NewSubmitPublicationDraftHandler(drafts, fixedClock{at: draftSubmittedAt}).Handle(context.Background(),
		application.SubmitPublicationDraftCommand{
			Shell:     shell,
			Content:   authorizationRuleContent(rows),
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
	reports := publication.Declarations()
	if len(reports) != 1 || reports[0].Channel != application.CancellationAuthorityChannel || reports[0].Outcome != ports.DeclarationSaved {
		t.Fatalf("报告 = %#v, want CANCELLATION_AUTHORITY=SAVED 一条", reports)
	}
	stored, _, _ := drafts.LoadDraft(context.Background(), shell.TenantID, shell.Kind, shell.ObjectID, shell.Version)
	if len(registry.savedVersions) != 1 || registry.savedVersions[0].ContentDigest() != stored.Canonical().Digest() {
		t.Fatalf("入册摘要 ≠ 载体摘要（saved %d）", len(registry.savedVersions))
	}
}
