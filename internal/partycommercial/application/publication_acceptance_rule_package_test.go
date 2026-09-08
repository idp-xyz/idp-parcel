package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件对接单规则包册接进服务端规范化后的两条路径证编排（票 admin-write-faces/12；ADR-0126 Decision 二、三）：
// 受控批文那一半的对账门对带正文的项开门，载体那一半把正文与全部声明节原样交给发布用例、各写各的通道。

// rulePackageDeclarations 造一份七节齐全的接单规则包声明：正文（五维 + 一条规则）、一条时点锚、接受内容、收寄资格、
// 一条终局规则 + 面单有效期、未封闭一格的资料修订允许。
func rulePackageDeclarations(t *testing.T) application.CommercialDeclarations {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(pubStartsAt, time.Time{})
	if err != nil {
		t.Fatalf("适用期间：%v", err)
	}
	applicability, err := domain.NewRulePackageApplicability(
		pcValue(t, domain.NewCommercialObjectID, "product-1"),
		pcValue(t, domain.NewCommercialObjectID, "contract-1"),
		pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		pcValue(t, domain.NewCommercialScopeReference, "scope-1"),
		interval,
	)
	if err != nil {
		t.Fatalf("五维适用性：%v", err)
	}
	rule, err := domain.NewAssembledRule(domain.MinimumIngressIdentityRules, pcValue(t, domain.NewRuleReference, "RULE/ingress-identity"))
	if err != nil {
		t.Fatalf("装配规则：%v", err)
	}
	asOf, err := domain.NewAsOfPolicy(domain.NetworkReachabilityJudgment,
		pcValue(t, domain.NewAsOfSemanticsReference, "AT_ACCEPTANCE"),
		pcValue(t, domain.NewAsOfPolicyVersion, "asof-policy/v1"))
	if err != nil {
		t.Fatalf("时点锚：%v", err)
	}
	validity, err := domain.NewLabelValidityDeclaration(domain.ChannelResultObservedAnchor, 84*time.Hour)
	if err != nil {
		t.Fatalf("面单有效期：%v", err)
	}
	return application.CommercialDeclarations{
		RulePackageBody: &application.RulePackageBodyDeclaration{Applicability: applicability, Rules: []domain.AssembledRule{rule}},
		AsOfPolicies:    []domain.AsOfPolicy{asOf},
		AcceptanceContent: &application.AcceptanceContentDeclaration{
			ApplicableGroups: []domain.AcceptanceCheckGroupType{domain.RequiredDocumentCheckGroup},
			ManualReview:     domain.ManualReviewNotRequired,
		},
		IntakeQualification: &application.IntakeQualificationDeclaration{
			Sources:        []domain.DeclaredIntakeSource{domain.DeclaredNodeIntake},
			Qualifications: []domain.RuleReference{pcValue(t, domain.NewRuleReference, "INTAKE-QUAL/realname")},
		},
		FinalRules: []domain.FinalizationDeclaration{{
			Outcome:   domain.DeclaredEffectiveDelivery,
			FinalKind: pcValue(t, domain.NewRuleReference, "FINAL/effective-delivery"),
		}},
		FinalRuleValidity: &validity,
		SourceDataAmendment: &application.SourceDataAmendmentDeclaration{
			Closed: false,
			Rules: []domain.SourceDataAmendmentRule{{
				DataGroup: pcValue(t, domain.NewSourceDataGroupReference, "consignee.address"),
				Stage:     domain.DeclaredAcceptedNotYetReceived,
				Intent:    domain.DeclaredCorrectionIntent,
				Allowance: domain.AmendmentAllowed,
			}},
		},
	}
}

// acceptanceRulePackageContent 把声明输入面折成本册的正文输入面——与应用层 publicationContentOf 那一支同一套对应，
// 这里手写一遍是为了让测试独立于被测的那一段。
func acceptanceRulePackageContent(t *testing.T, declarations application.CommercialDeclarations) domain.PublicationContent {
	t.Helper()
	body := &domain.AcceptanceRulePackageBody{
		Applicability:     declarations.RulePackageBody.Applicability,
		Rules:             declarations.RulePackageBody.Rules,
		AsOfPolicies:      declarations.AsOfPolicies,
		FinalRules:        declarations.FinalRules,
		FinalRuleValidity: declarations.FinalRuleValidity,
	}
	if declarations.AcceptanceContent != nil {
		body.AcceptanceContent = &domain.AcceptanceContentBody{
			ApplicableGroups: declarations.AcceptanceContent.ApplicableGroups,
			ManualReview:     declarations.AcceptanceContent.ManualReview,
		}
	}
	if declarations.IntakeQualification != nil {
		body.IntakeQualification = &domain.IntakeQualificationBody{
			Sources:        declarations.IntakeQualification.Sources,
			Qualifications: declarations.IntakeQualification.Qualifications,
		}
	}
	if declarations.SourceDataAmendment != nil {
		body.SourceDataAmendment = &domain.SourceDataAmendmentBody{
			Closed: declarations.SourceDataAmendment.Closed,
			Rules:  declarations.SourceDataAmendment.Rules,
		}
	}
	return domain.PublicationContent{Kind: domain.AcceptanceRulePackageObject, AcceptanceRulePackage: body}
}

// acceptanceRulePackageSpec 给接单规则包版本壳配上**算出的**内容摘要：本册接进服务端规范化后，对账门要求声明的串与
// 算出的逐字节相等（判据同 customerContractSpec）。
func acceptanceRulePackageSpec(t *testing.T, objectID, label string, declarations application.CommercialDeclarations) domain.CommercialVersionSpec {
	t.Helper()
	spec := publishSpec(t, domain.AcceptanceRulePackageObject, objectID, label)
	canonical, err := domain.CanonicalizePublicationContent(acceptanceRulePackageContent(t, declarations))
	if err != nil {
		t.Fatalf("规范化接单规则包正文：%v", err)
	}
	spec.ContentDigest = canonical.Digest()
	return spec
}

const fullRulePackageDeclarationLog = "as-of,acceptance-content,intake-qualification,final-rule,rule-package-body,source-data-amendment"

// Covers: ADR-0126 Decision 二在本册的落法——受控批文的接单规则包项带正文时，声明的摘要必须与按正文 + 全部声明节算出的
// 相等：相等即发布并把各节各写各的通道（含终局规则父行上的有效期）；旧式 `sha256:` 串不等即`未受理`带两串、一个字节不写
// 整册不读；只带声明不带正文的项（今天 CLI 批与 seed 里常见的只声明时点锚那种）没有可比对象，照今天登记声明的串。
func TestAnAcceptanceRulePackageBodyIsReconciledAgainstTheCanonicalDigest(t *testing.T) {
	declarations := rulePackageDeclarations(t)

	t.Run("the computed digest publishes every section", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         acceptanceRulePackageSpec(t, "rules-1", "v1", declarations),
			Approval:     publishApproval(t, "rules-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: declarations,
		})
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.CommercialVersionPublishedEffective {
			t.Fatalf("outcome = %q, want PUBLISHED_EFFECTIVE", result.Outcome())
		}
		if got := strings.Join(registry.declarationLog, ","); got != fullRulePackageDeclarationLog {
			t.Fatalf("declaration log = %v, want every section", registry.declarationLog)
		}
		if len(registry.savedFinalRules) != 1 {
			t.Fatalf("终局规则写入 %d 份，want 1", len(registry.savedFinalRules))
		}
		if validity, declared := registry.savedFinalRules[0].Validity(); !declared || validity.Duration() != 84*time.Hour {
			t.Fatalf("有效期 = %#v, %v；want 84h 在场", validity, declared)
		}
		if len(registry.savedSourceDataAmendments) != 1 || registry.savedSourceDataAmendments[0].Closed() {
			t.Fatalf("资料修订允许 = %#v，want 一份未封闭", registry.savedSourceDataAmendments)
		}
	})

	t.Run("a legacy declared digest is not accepted", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.AcceptanceRulePackageObject, "rules-1", "v1"),
			Approval:     publishApproval(t, "rules-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: declarations,
		})
		if err != nil {
			t.Fatalf("Handle：%v——未受理不是 error", err)
		}
		if result.Outcome() != application.CommercialPublicationNotAccepted || !errors.Is(result.RefusalCause(), domain.ErrDeclaredDigestMismatch) {
			t.Fatalf("outcome = %q, cause = %v; want NOT_ACCEPTED / ErrDeclaredDigestMismatch", result.Outcome(), result.RefusalCause())
		}
		declared, computed, ok := result.DigestReconciliation()
		if !ok || declared != "sha256:rules-1-v1" || !strings.HasPrefix(computed, "PCC-1:") {
			t.Fatalf("DigestReconciliation = (%q, %q, %v)", declared, computed, ok)
		}
		if len(registry.savedVersions) != 0 || len(registry.declarationLog) != 0 || registry.loads != 0 {
			t.Fatal("未受理写了库或读了整册")
		}
	})

	t.Run("declarations without the body have nothing to reconcile", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.AcceptanceRulePackageObject, "rules-1", "v1"),
			Approval:     publishApproval(t, "rules-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{
				AsOfPolicies:        declarations.AsOfPolicies,
				SourceDataAmendment: declarations.SourceDataAmendment,
			},
		})
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.CommercialVersionPublishedEffective || strings.Join(registry.declarationLog, ",") != "as-of,source-data-amendment" {
			t.Fatalf("outcome = %q, log = %v", result.Outcome(), registry.declarationLog)
		}
	})

	t.Run("a body that would not publish is not accepted before the register is read", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		// 只给有效期不给终局行：发布用例本来在 declarationWrites 整项拒（ADR-0119 Decision 五）；本册接进规范化后同一件事在
		// 对账门就答`未受理`带成因，不再是一个 error。
		orphanValidity := declarations
		orphanValidity.FinalRules = nil
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.AcceptanceRulePackageObject, "rules-1", "v1"),
			Approval:     publishApproval(t, "rules-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: orphanValidity,
		})
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.CommercialPublicationNotAccepted || !errors.Is(result.RefusalCause(), domain.ErrFinalContentNotConfigured) {
			t.Fatalf("outcome = %q, cause = %v", result.Outcome(), result.RefusalCause())
		}
		if _, _, ok := result.DigestReconciliation(); ok || registry.loads != 0 {
			t.Fatal("折不成文档时没有算出的串可交，也不该读整册")
		}
	})
}

// Covers: ADR-0126 Decision 三在本册的落法——载体上的接单规则包正文发布时折回发布用例的各声明通道（正文一通道、每节
// 各一通道，有效期随终局规则同一通道），入册摘要就是载体上算出的那一个；只带正文的载体只写正文那一通道，缺席的节不代填。
func TestAnAcceptanceRulePackageDraftPublishesEverySectionThroughTheExistingUseCase(t *testing.T) {
	publishDraft := func(t *testing.T, declarations application.CommercialDeclarations) (*publicationRegistryDouble, domain.PublicationDraft) {
		t.Helper()
		drafts := newDraftRegistryDouble()
		shell := draftShell(t, domain.AcceptanceRulePackageObject, "rules-1", "v1")
		submitted, err := application.NewSubmitPublicationDraftHandler(drafts, fixedClock{at: draftSubmittedAt}).Handle(context.Background(),
			application.SubmitPublicationDraftCommand{
				Shell:     shell,
				Content:   acceptanceRulePackageContent(t, declarations),
				Submitter: pcValue(t, domain.NewOperatorSubjectReference, "op-submitter"),
			})
		if err != nil || submitted.Outcome() != application.PublicationDraftSubmitted {
			t.Fatalf("录入 = %q, %v (%v)", submitted.Outcome(), err, submitted.RefusalCause())
		}
		approved, err := application.NewApprovePublicationDraftHandler(drafts, approvalDutyRuleDouble{rule: dutyRule(t, true, ""), found: true}, fixedClock{at: draftApprovedAt}).
			Handle(context.Background(), application.ApprovePublicationDraftCommand{
				Tenant: shell.TenantID, Kind: shell.Kind, ObjectID: shell.ObjectID, Version: shell.Version,
				Approver: operatorSubject(t, "op-approver"),
			})
		if err != nil || approved.Outcome() != application.PublicationDraftApproved {
			t.Fatalf("批准 = %q, %v", approved.Outcome(), err)
		}
		registry := &publicationRegistryDouble{}
		publisher := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: draftPublishedAt}, &operatorRegistrationHandoffDouble{})
		result, err := application.NewPublishPublicationDraftHandler(drafts, publisher, fixedClock{at: draftPublishedAt}).
			Handle(context.Background(), application.PublishPublicationDraftCommand{
				Tenant: shell.TenantID, Kind: shell.Kind, ObjectID: shell.ObjectID, Version: shell.Version,
			})
		if err != nil || result.Outcome() != application.PublicationDraftPublished {
			t.Fatalf("发布 = %q, %v", result.Outcome(), err)
		}
		stored, _, _ := drafts.LoadDraft(context.Background(), shell.TenantID, shell.Kind, shell.ObjectID, shell.Version)
		return registry, stored
	}

	t.Run("every section", func(t *testing.T) {
		registry, stored := publishDraft(t, rulePackageDeclarations(t))
		if got := strings.Join(registry.declarationLog, ","); got != fullRulePackageDeclarationLog {
			t.Fatalf("declaration log = %v", registry.declarationLog)
		}
		if len(registry.savedVersions) != 1 || registry.savedVersions[0].ContentDigest() != stored.Canonical().Digest() {
			t.Fatalf("入册摘要与载体摘要不一致：%#v vs %s", registry.savedVersions, stored.Canonical().Digest())
		}
		if validity, declared := registry.savedFinalRules[0].Validity(); !declared || validity.Anchor() != domain.ChannelResultObservedAnchor {
			t.Fatalf("有效期 = %#v, %v", validity, declared)
		}
		if len(registry.savedIntake) != 1 || !registry.savedIntake[0].Allows(domain.DeclaredNodeIntake) || registry.savedIntake[0].Allows(domain.DeclaredOffsitePickup) {
			t.Fatalf("收寄资格 = %#v", registry.savedIntake)
		}
		if stored.Status() != domain.PublicationDraftPublished {
			t.Fatalf("draft status = %s, want PUBLISHED", stored.Status())
		}
	})

	t.Run("body only", func(t *testing.T) {
		declarations := application.CommercialDeclarations{RulePackageBody: rulePackageDeclarations(t).RulePackageBody}
		registry, _ := publishDraft(t, declarations)
		if strings.Join(registry.declarationLog, ",") != "rule-package-body" {
			t.Fatalf("declaration log = %v, want the body alone", registry.declarationLog)
		}
	})
}
