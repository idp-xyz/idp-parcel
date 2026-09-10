package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件对客户合同册接进服务端规范化后的两条路径证编排（票 admin-write-faces/10；ADR-0126 Decision 二、三）：
// 受控批文那一半的对账门对本册开门，载体那一半把两层声明原样交给发布用例。

func contractBinding(t *testing.T, scope, policy, basis string) domain.FinancialControlBinding {
	t.Helper()
	chargeScope := pcValue(t, domain.NewChargeScopeReference, scope)
	var binding domain.FinancialControlBinding
	var err error
	if policy != "" {
		binding, err = domain.NewAppliedFinancialControl(chargeScope, pcValue(t, domain.NewCommercialObjectID, policy))
	} else {
		binding, err = domain.NewInapplicableFinancialControl(chargeScope, pcValue(t, domain.NewInapplicabilityBasis, basis))
	}
	if err != nil {
		t.Fatalf("约定 %s：%v", scope, err)
	}
	return binding
}

// contractDeclarations 造一份两层齐全的客户合同正文：规则包 + 两行约定（一指名、一不适用）+ 合同级`要求控制`。
func contractDeclarations(t *testing.T, rulePackage string) application.CommercialDeclarations {
	t.Helper()
	return application.CommercialDeclarations{
		ContractContent: &application.ContractContentDeclaration{
			RulePackage: pcValue(t, domain.NewCommercialObjectID, rulePackage),
			Bindings: []domain.FinancialControlBinding{
				contractBinding(t, "charge-prepaid", "control-policy-1", ""),
				contractBinding(t, "charge-cod", "", "COD-NA-01"),
			},
		},
		PreAcceptanceControl: &application.PreAcceptanceControlInstruction{Requirement: domain.PreAcceptanceControlRequired},
	}
}

func customerContractContent(t *testing.T, declarations application.CommercialDeclarations) domain.PublicationContent {
	t.Helper()
	body := &domain.CustomerContractBody{
		RulePackage: declarations.ContractContent.RulePackage,
		Bindings:    declarations.ContractContent.Bindings,
	}
	if declarations.PreAcceptanceControl != nil {
		body.Control = &domain.PreAcceptanceControlBody{
			Requirement: declarations.PreAcceptanceControl.Requirement,
			Basis:       declarations.PreAcceptanceControl.Basis,
		}
	}
	// 第三层（合同层交付条件，票 admin-write-faces/25）在场时一并折进：与另两层同一份正文、同一个摘要。
	if declarations.DeliveryConditions != nil {
		body.DeliveryConditions = &domain.DeliveryConditionBody{
			Tightens: declarations.DeliveryConditions.Tightens,
			Terms:    declarations.DeliveryConditions.Terms,
		}
	}
	return domain.PublicationContent{Kind: domain.CustomerContractObject, CustomerContract: body}
}

// customerContractSpec 给客户合同版本壳配上**算出的**内容摘要：本册接进服务端规范化后，对账门要求声明的串与
// 算出的逐字节相等（判据同 creditPolicySpec）。
func customerContractSpec(t *testing.T, objectID, label string, declarations application.CommercialDeclarations) domain.CommercialVersionSpec {
	t.Helper()
	spec := publishSpec(t, domain.CustomerContractObject, objectID, label)
	canonical, err := domain.CanonicalizePublicationContent(customerContractContent(t, declarations))
	if err != nil {
		t.Fatalf("规范化客户合同正文：%v", err)
	}
	spec.ContentDigest = canonical.Digest()
	return spec
}

// Covers: ADR-0126 Decision 二在本册的落法——受控批文的客户合同项带正文时，声明的摘要必须与按两层算出的相等：
// 相等即发布并把两层各写各的通道；旧式 `sha256:` 串不等即`未受理`带两串、一个字节不写整册不读；只带合同级
// 声明不带正文的项没有可比对象，照今天登记声明的串（不改批文既有语义）。
func TestACustomerContractBodyIsReconciledAgainstTheCanonicalDigest(t *testing.T) {
	declarations := contractDeclarations(t, "rules-1")

	t.Run("the computed digest publishes both layers", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         customerContractSpec(t, "contract-1", "v1", declarations),
			Approval:     publishApproval(t, "contract-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: declarations,
		})
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.CommercialVersionPublishedEffective {
			t.Fatalf("outcome = %q, want PUBLISHED_EFFECTIVE", result.Outcome())
		}
		if len(registry.savedContract) != 1 || registry.savedContract[0].AcceptanceRulePackage().String() != "rules-1" ||
			len(registry.savedContract[0].Bindings()) != 2 {
			t.Fatalf("saved contract content = %#v", registry.savedContract)
		}
		if strings.Join(registry.declarationLog, ",") != "pre-acceptance-control,contract-content" {
			t.Fatalf("declaration log = %v, want both layers", registry.declarationLog)
		}
	})

	t.Run("a legacy declared digest is not accepted", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.CustomerContractObject, "contract-1", "v1"),
			Approval:     publishApproval(t, "contract-1"),
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
		if !ok || declared != "sha256:contract-1-v1" || !strings.HasPrefix(computed, "PCC-1:") {
			t.Fatalf("DigestReconciliation = (%q, %q, %v)", declared, computed, ok)
		}
		if len(registry.savedVersions) != 0 || len(registry.savedContract) != 0 || len(registry.declarationLog) != 0 || registry.loads != 0 {
			t.Fatal("未受理写了库或读了整册")
		}
	})

	t.Run("a contract-level declaration without the body has nothing to reconcile", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.CustomerContractObject, "contract-1", "v1"),
			Approval:     publishApproval(t, "contract-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{PreAcceptanceControl: declarations.PreAcceptanceControl},
		})
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.CommercialVersionPublishedEffective || strings.Join(registry.declarationLog, ",") != "pre-acceptance-control" {
			t.Fatalf("outcome = %q, log = %v", result.Outcome(), registry.declarationLog)
		}
	})
}

// Covers: ADR-0126 Decision 三在本册的落法——载体上的客户合同正文发布时折回发布用例的两个声明通道
// （contractContent 与 preAcceptanceControl 各写各的），入册摘要就是载体上算出的那一个；合同级声明缺席的载体只写
// 正文那一层，不代填「要不要」。
func TestACustomerContractDraftPublishesBothLayersThroughTheExistingUseCase(t *testing.T) {
	publishDraft := func(t *testing.T, declarations application.CommercialDeclarations) (*publicationRegistryDouble, domain.PublicationDraft) {
		t.Helper()
		drafts := newDraftRegistryDouble()
		shell := draftShell(t, domain.CustomerContractObject, "contract-1", "v1")
		submitted, err := application.NewSubmitPublicationDraftHandler(drafts, fixedClock{at: draftSubmittedAt}).Handle(context.Background(),
			application.SubmitPublicationDraftCommand{
				Shell:     shell,
				Content:   customerContractContent(t, declarations),
				Submitter: pcValue(t, domain.NewOperatorSubjectReference, "op-submitter"),
			})
		if err != nil || submitted.Outcome() != application.PublicationDraftSubmitted {
			t.Fatalf("录入 = %q, %v", submitted.Outcome(), err)
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

	t.Run("both layers", func(t *testing.T) {
		registry, stored := publishDraft(t, contractDeclarations(t, "rules-1"))
		if strings.Join(registry.declarationLog, ",") != "pre-acceptance-control,contract-content" {
			t.Fatalf("declaration log = %v", registry.declarationLog)
		}
		if len(registry.savedVersions) != 1 || registry.savedVersions[0].ContentDigest() != stored.Canonical().Digest() {
			t.Fatalf("入册摘要与载体摘要不一致：%#v vs %s", registry.savedVersions, stored.Canonical().Digest())
		}
		saved := registry.savedContract[0]
		cod, found := saved.FinancialControlFor(pcValue(t, domain.NewChargeScopeReference, "charge-cod"))
		if !found || !cod.ExplicitlyInapplicable() || cod.InapplicabilityBasis().String() != "COD-NA-01" {
			t.Fatalf("charge-cod binding = %#v, %v", cod, found)
		}
		if stored.Status() != domain.PublicationDraftPublished {
			t.Fatalf("draft status = %s, want PUBLISHED", stored.Status())
		}
	})

	t.Run("content only", func(t *testing.T) {
		declarations := contractDeclarations(t, "rules-1")
		declarations.PreAcceptanceControl = nil
		registry, _ := publishDraft(t, declarations)
		if strings.Join(registry.declarationLog, ",") != "contract-content" {
			t.Fatalf("declaration log = %v, want the content layer alone", registry.declarationLog)
		}
	})
}
