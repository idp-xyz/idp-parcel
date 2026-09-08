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

// 本文件证结算政策册在应用层的两个方向都接上了（票 admin-write-faces/15）：受控批文那一半的对账门对本册开门
// （publicationContentOf 一支），载体那一半把正文交回发布用例（declarationsOfContent 一支）——六维原样往返，不代填、
// 不归并任何一维（ADR-0044）。

func settlementPolicyDeclaration(t *testing.T, method domain.SettlementMethod, chargeScope, currency string) *application.SettlementPolicyBodyDeclaration {
	t.Helper()
	return &application.SettlementPolicyBodyDeclaration{
		Method:        method,
		Applicability: settlementApplicability(t, chargeScope, currency),
	}
}

func settlementPolicyContent(t *testing.T, declaration *application.SettlementPolicyBodyDeclaration) domain.PublicationContent {
	t.Helper()
	return domain.PublicationContent{Kind: domain.SettlementPolicyObject, SettlementPolicy: &domain.SettlementPolicyBody{
		Method:        declaration.Method,
		Applicability: declaration.Applicability,
	}}
}

// settlementPolicySpec 给结算政策版本壳配上**算出的**内容摘要：本册接进服务端规范化后，对账门要求声明的串与算出的
// 逐字节相等，测试里的壳因此不能再随手写一个（判据同 creditPolicySpec）。
func settlementPolicySpec(t *testing.T, objectID, label string, body *application.SettlementPolicyBodyDeclaration) domain.CommercialVersionSpec {
	t.Helper()
	spec := publishSpec(t, domain.SettlementPolicyObject, objectID, label)
	canonical, err := domain.CanonicalizePublicationContent(settlementPolicyContent(t, body))
	if err != nil {
		t.Fatalf("规范化结算政策正文：%v", err)
	}
	spec.ContentDigest = canonical.Digest()
	return spec
}

// Covers: ADR-0126 Decision 二 — 结算政策册接进规范化后对账门对它开门：声明的旧式串与算出的不等即`未受理`，一个字节
// 不写、整册不读，结果带出两个串；算出的串放行且六维原样落册；壳单独发布（正文缺席）没有可比对象照旧登记。
func TestASettlementPolicyDeclaredDigestIsReconciled(t *testing.T) {
	body := settlementPolicyDeclaration(t, domain.TermsMethod, "charge-terms", "CNY")

	t.Run("a mismatching declared digest is not accepted", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.SettlementPolicyObject, "settlement-1", "v1"),
			Approval:     publishApproval(t, "settlement-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{SettlementPolicyBody: body},
		})
		if err != nil {
			t.Fatalf("Handle：%v——未受理不是 error", err)
		}
		if result.Outcome() != application.CommercialPublicationNotAccepted || !errors.Is(result.RefusalCause(), domain.ErrDeclaredDigestMismatch) {
			t.Fatalf("outcome = %q, cause = %v; want NOT_ACCEPTED / ErrDeclaredDigestMismatch", result.Outcome(), result.RefusalCause())
		}
		declared, computed, ok := result.DigestReconciliation()
		if !ok || declared != "sha256:settlement-1-v1" || !strings.HasPrefix(computed, "PCC-1:") {
			t.Fatalf("DigestReconciliation = (%q, %q, %v)", declared, computed, ok)
		}
		if len(registry.savedVersions) != 0 || len(registry.savedSettlement) != 0 || registry.loads != 0 {
			t.Fatalf("未受理写了 %d 版本 / %d 正文、读了 %d 次整册", len(registry.savedVersions), len(registry.savedSettlement), registry.loads)
		}
	})

	t.Run("the computed digest publishes and the six dimensions land untouched", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         settlementPolicySpec(t, "settlement-1", "v1", body),
			Approval:     publishApproval(t, "settlement-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{SettlementPolicyBody: body},
		})
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.CommercialVersionPublishedEffective || len(registry.savedSettlement) != 1 {
			t.Fatalf("outcome = %q, saved settlement bodies = %d", result.Outcome(), len(registry.savedSettlement))
		}
		saved := registry.savedSettlement[0]
		if saved.Method() != domain.TermsMethod || saved.Applicability() != body.Applicability {
			t.Fatalf("registered body = %#v", saved)
		}
	})

	t.Run("a shell without its body still publishes", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.SettlementPolicyObject, "settlement-1", "v1"),
			Approval:     publishApproval(t, "settlement-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
		})
		if err != nil || result.Outcome() != application.CommercialVersionPublishedEffective {
			t.Fatalf("outcome = %q, err = %v; want PUBLISHED_EFFECTIVE", result.Outcome(), err)
		}
	})
}

// Covers: ADR-0126 Decision 三 — 结算政策载体走完录入 → 批准 → 发布：正文随版本同笔登记到结算政策册（方式与六维
// 就是录入时那一份，合同维仍是同一个两段式串）、报告一条 SETTLEMENT_POLICY_BODY=SAVED、入册摘要就是载体的摘要。
func TestASettlementPolicyDraftPublishesItsBodyThroughTheExistingUseCase(t *testing.T) {
	drafts := newDraftRegistryDouble()
	shell := draftShell(t, domain.SettlementPolicyObject, "settlement-1", "v1")
	declaration := settlementPolicyDeclaration(t, domain.PrepaidMethod, "charge-prepaid", "CNY")
	submitted, err := application.NewSubmitPublicationDraftHandler(drafts, fixedClock{at: draftSubmittedAt}).Handle(context.Background(),
		application.SubmitPublicationDraftCommand{
			Shell:     shell,
			Content:   settlementPolicyContent(t, declaration),
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
	if len(registry.savedVersions) != 1 || len(registry.savedSettlement) != 1 {
		t.Fatalf("saved %d versions / %d settlement bodies, want 1 / 1", len(registry.savedVersions), len(registry.savedSettlement))
	}
	saved := registry.savedSettlement[0]
	if saved.Method() != domain.PrepaidMethod || saved.Applicability() != declaration.Applicability {
		t.Fatalf("registered body = %#v", saved)
	}
	if saved.Applicability().Contract().String() != "contract-1/v1" {
		t.Fatalf("contract label = %q, want the qualified label the draft carried", saved.Applicability().Contract())
	}
	reports := publication.Declarations()
	if len(reports) != 1 || reports[0].Channel != application.SettlementPolicyBodyChannel || reports[0].Outcome != ports.DeclarationSaved {
		t.Fatalf("报告 = %#v, want SETTLEMENT_POLICY_BODY=SAVED 一条", reports)
	}
	stored, _, _ := drafts.LoadDraft(context.Background(), shell.TenantID, shell.Kind, shell.ObjectID, shell.Version)
	if registry.savedVersions[0].ContentDigest() != stored.Canonical().Digest() {
		t.Fatalf("入册摘要 %s ≠ 载体摘要 %s", registry.savedVersions[0].ContentDigest(), stored.Canonical().Digest())
	}
}
