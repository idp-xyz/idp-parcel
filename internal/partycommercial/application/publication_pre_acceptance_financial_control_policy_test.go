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

// 本文件证接受前财务控制策略册在应用层的两个方向都接上了（票 admin-write-faces/13）：受控批文那一半的对账门对本册开门
// （publicationContentOf 一支），载体那一半把正文交回发布用例（declarationsOfContent 一支）——共同通过条件与控制项原样
// 往返，行序由判断顺序归一，不代填任何一项（ADR-0115）。

func controlPolicyContent(t *testing.T, declaration *application.PreAcceptanceFinancialControlPolicyBodyDeclaration) domain.PublicationContent {
	t.Helper()
	return domain.PublicationContent{
		Kind: domain.PreAcceptanceFinancialControlPolicyObject,
		PreAcceptanceFinancialControlPolicy: &domain.PreAcceptanceFinancialControlPolicyBody{
			JointPass: declaration.JointPass,
			Items:     declaration.Items,
		},
	}
}

// controlPolicySpec 给策略版本壳配上**算出的**内容摘要：本册接进服务端规范化后，对账门要求声明的串与算出的逐字节相等，
// 测试里的壳因此不能再随手写一个（判据同 creditPolicySpec）。
func controlPolicySpec(t *testing.T, objectID, label string, body *application.PreAcceptanceFinancialControlPolicyBodyDeclaration) domain.CommercialVersionSpec {
	t.Helper()
	spec := publishSpec(t, domain.PreAcceptanceFinancialControlPolicyObject, objectID, label)
	canonical, err := domain.CanonicalizePublicationContent(controlPolicyContent(t, body))
	if err != nil {
		t.Fatalf("规范化策略正文：%v", err)
	}
	spec.ContentDigest = canonical.Digest()
	return spec
}

// Covers: ADR-0126 Decision 二 — 策略册接进规范化后对账门对它开门：声明的旧式串与算出的不等即`未受理`，一个字节不写、
// 整册不读，结果带出两个串；算出的串放行且控制项按判断顺序落册；壳单独发布（正文缺席）没有可比对象照旧登记；正文过不了
// 门（零项）同归`未受理`带成因，不再是 error。
func TestAPreAcceptanceFinancialControlPolicyDeclaredDigestIsReconciled(t *testing.T) {
	body := controlPolicyBody(t,
		controlItemOf(t, domain.CreditCheckControl, "charge-scope-a", 2, domain.AuthorizedDispositionOnControlFailure),
		controlItemOf(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure),
	)

	t.Run("a mismatching declared digest is not accepted", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.PreAcceptanceFinancialControlPolicyObject, "fcp-1", "v1"),
			Approval:     publishApproval(t, "fcp-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{PreAcceptanceFinancialControlPolicyBody: body},
		})
		if err != nil {
			t.Fatalf("Handle：%v——未受理不是 error", err)
		}
		if result.Outcome() != application.CommercialPublicationNotAccepted || !errors.Is(result.RefusalCause(), domain.ErrDeclaredDigestMismatch) {
			t.Fatalf("outcome = %q, cause = %v; want NOT_ACCEPTED / ErrDeclaredDigestMismatch", result.Outcome(), result.RefusalCause())
		}
		declared, computed, ok := result.DigestReconciliation()
		if !ok || declared != "sha256:fcp-1-v1" || !strings.HasPrefix(computed, "PCC-1:") {
			t.Fatalf("DigestReconciliation = (%q, %q, %v)", declared, computed, ok)
		}
		if len(registry.savedVersions) != 0 || len(registry.savedControlPolicies) != 0 || registry.loads != 0 {
			t.Fatalf("未受理写了 %d 版本 / %d 正文、读了 %d 次整册", len(registry.savedVersions), len(registry.savedControlPolicies), registry.loads)
		}
	})

	t.Run("the computed digest publishes and the controls land in evaluation order", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         controlPolicySpec(t, "fcp-1", "v1", body),
			Approval:     publishApproval(t, "fcp-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{PreAcceptanceFinancialControlPolicyBody: body},
		})
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.CommercialVersionPublishedEffective || len(registry.savedControlPolicies) != 1 {
			t.Fatalf("outcome = %q, saved policies = %d", result.Outcome(), len(registry.savedControlPolicies))
		}
		items := registry.savedControlPolicies[0].Items()
		if len(items) != 2 || items[0].EvaluationOrder() != 1 || items[1].EvaluationOrder() != 2 {
			t.Fatalf("items = %#v", items)
		}
	})

	t.Run("a shell without its body still publishes", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.PreAcceptanceFinancialControlPolicyObject, "fcp-1", "v1"),
			Approval:     publishApproval(t, "fcp-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
		})
		if err != nil || result.Outcome() != application.CommercialVersionPublishedEffective {
			t.Fatalf("outcome = %q, err = %v; want PUBLISHED_EFFECTIVE", result.Outcome(), err)
		}
	})

	t.Run("a body that fails its own gate is not accepted before any write", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.PreAcceptanceFinancialControlPolicyObject, "fcp-1", "v1"),
			Approval:     publishApproval(t, "fcp-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{PreAcceptanceFinancialControlPolicyBody: controlPolicyBody(t)},
		})
		if err != nil {
			t.Fatalf("Handle：%v——未受理不是 error", err)
		}
		if result.Outcome() != application.CommercialPublicationNotAccepted || !errors.Is(result.RefusalCause(), domain.ErrInvalidPreAcceptanceFinancialControlPolicy) {
			t.Fatalf("outcome = %q, cause = %v; want NOT_ACCEPTED / ErrInvalidPreAcceptanceFinancialControlPolicy", result.Outcome(), result.RefusalCause())
		}
		if _, _, ok := result.DigestReconciliation(); ok {
			t.Fatal("折不成文档就没有算出的串可比")
		}
		if len(registry.savedVersions) != 0 || len(registry.savedControlPolicies) != 0 || registry.loads != 0 {
			t.Fatal("零项的策略正文写了库或读了整册")
		}
	})
}

// Covers: ADR-0126 Decision 三 — 策略载体走完录入 → 批准 → 发布：正文随版本同笔登记到策略册（共同通过条件与控制项就是
// 录入时那一份，行序按判断顺序）、报告一条 PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY_BODY=SAVED、入册摘要就是载体的摘要。
func TestAPreAcceptanceFinancialControlPolicyDraftPublishesItsBodyThroughTheExistingUseCase(t *testing.T) {
	drafts := newDraftRegistryDouble()
	shell := draftShell(t, domain.PreAcceptanceFinancialControlPolicyObject, "fcp-1", "v1")
	declaration := controlPolicyBody(t,
		controlItemOf(t, domain.CreditCheckControl, "charge-scope-b", 2, domain.AuthorizedDispositionOnControlFailure),
		controlItemOf(t, domain.PrepaidFreezeControl, "charge-scope-a", 1, domain.RejectOnControlFailure),
	)
	submitted, err := application.NewSubmitPublicationDraftHandler(drafts, fixedClock{at: draftSubmittedAt}).Handle(context.Background(),
		application.SubmitPublicationDraftCommand{
			Shell:     shell,
			Content:   controlPolicyContent(t, declaration),
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
	if len(registry.savedVersions) != 1 || len(registry.savedControlPolicies) != 1 {
		t.Fatalf("saved %d versions / %d policies, want 1 / 1", len(registry.savedVersions), len(registry.savedControlPolicies))
	}
	saved := registry.savedControlPolicies[0]
	if saved.JointPassCondition() != domain.AllControlsPass {
		t.Fatalf("joint pass = %v", saved.JointPassCondition())
	}
	items := saved.Items()
	if len(items) != 2 || items[0].Kind() != domain.PrepaidFreezeControl || items[0].Scope().String() != "charge-scope-a" ||
		items[1].Kind() != domain.CreditCheckControl || items[1].Scope().String() != "charge-scope-b" ||
		items[1].FailureDisposition() != domain.AuthorizedDispositionOnControlFailure || items[1].Responsibility().String() != "customer-1" {
		t.Fatalf("registered items = %#v", items)
	}
	reports := publication.Declarations()
	if len(reports) != 1 || reports[0].Channel != application.PreAcceptanceFinancialControlPolicyBodyChannel || reports[0].Outcome != ports.DeclarationSaved {
		t.Fatalf("报告 = %#v, want PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY_BODY=SAVED 一条", reports)
	}
	stored, _, _ := drafts.LoadDraft(context.Background(), shell.TenantID, shell.Kind, shell.ObjectID, shell.Version)
	if registry.savedVersions[0].ContentDigest() != stored.Canonical().Digest() {
		t.Fatalf("入册摘要 %s ≠ 载体摘要 %s", registry.savedVersions[0].ContentDigest(), stored.Canonical().Digest())
	}
}
