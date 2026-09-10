package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件对交付条件折进 PCC-1 之后的两条路径证编排（票 admin-write-faces/25 ④ 应用两向；ADR-0126 Decision 二、三）：
// 受控批文那一半的对账门对带交付条件的服务产品项开门、把合同层算进合同正文的摘要；载体那一半把两册上的这一节原样
// 折回发布用例的 DELIVERY_CONDITION 通道。不带这一节的产品版本与只带交付条件不带正文的合同项照旧「不在场」。

func productDeliveryDeclaration(t *testing.T, methods ...string) application.DeliveryConditionDeclaration {
	t.Helper()
	return application.DeliveryConditionDeclaration{Terms: deliveryTermsFor(t, methods...)}
}

func contractDeliveryDeclaration(t *testing.T, methods ...string) application.DeliveryConditionDeclaration {
	t.Helper()
	return application.DeliveryConditionDeclaration{Tightens: tightensProduct(t, "product-1", "v1"), Terms: deliveryTermsFor(t, methods...)}
}

func serviceProductContent(t *testing.T, declaration *application.DeliveryConditionDeclaration) domain.PublicationContent {
	t.Helper()
	content := domain.PublicationContent{Kind: domain.ServiceProductObject}
	if declaration != nil {
		content.ServiceProduct = &domain.ServiceProductBody{DeliveryConditions: &domain.DeliveryConditionBody{
			Tightens: declaration.Tightens, Terms: declaration.Terms,
		}}
	}
	return content
}

// Covers: ADR-0126 Decision 二在服务产品册的落法（票 25「本册规范化判断」）——带产品层交付条件的项才开门：旧式 `sha256:`
// 串不等即`未受理`带两串、一个字节不写；算出的串放行且这一节写进交付条件册；不带这一节的壳照今天登记声明的串
// （票 09 那一格不变，seed 两项因此不必换串）。
func TestAServiceProductWithDeliveryConditionsIsReconciledAgainstTheCanonicalDigest(t *testing.T) {
	declaration := productDeliveryDeclaration(t, "method-b", "method-a")

	t.Run("a legacy declared digest is not accepted once the section is present", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.ServiceProductObject, "product-1", "v1"),
			Approval:     publishApproval(t, "product-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{DeliveryConditions: &declaration},
		})
		if err != nil {
			t.Fatalf("Handle：%v——未受理不是 error", err)
		}
		if result.Outcome() != application.CommercialPublicationNotAccepted || !errors.Is(result.RefusalCause(), domain.ErrDeclaredDigestMismatch) {
			t.Fatalf("outcome = %q, cause = %v; want NOT_ACCEPTED / ErrDeclaredDigestMismatch", result.Outcome(), result.RefusalCause())
		}
		declared, computed, ok := result.DigestReconciliation()
		if !ok || declared != "sha256:product-1-v1" || !strings.HasPrefix(computed, "PCC-1:") {
			t.Fatalf("DigestReconciliation = (%q, %q, %v)", declared, computed, ok)
		}
		if len(registry.savedVersions) != 0 || len(registry.savedDeliveryConditions) != 0 || registry.loads != 0 {
			t.Fatal("未受理写了库或读了整册")
		}
	})

	t.Run("the computed digest publishes the section", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         deliveryConditionProductSpec(t, "product-1", "v1", declaration),
			Approval:     publishApproval(t, "product-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{DeliveryConditions: &declaration},
		})
		if err != nil || result.Outcome() != application.CommercialVersionPublishedEffective {
			t.Fatalf("outcome = %q, err = %v; want PUBLISHED_EFFECTIVE", result.Outcome(), err)
		}
		if len(registry.savedDeliveryConditions) != 1 || len(registry.savedDeliveryConditions[0].Methods()) != 2 {
			t.Fatalf("saved delivery conditions = %#v", registry.savedDeliveryConditions)
		}
	})

	t.Run("a shell without the section still publishes on its declared digest", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.ServiceProductObject, "product-1", "v1"),
			Approval:     publishApproval(t, "product-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
		})
		if err != nil || result.Outcome() != application.CommercialVersionPublishedEffective {
			t.Fatalf("outcome = %q, err = %v; want PUBLISHED_EFFECTIVE", result.Outcome(), err)
		}
		if _, _, reconciled := result.DigestReconciliation(); reconciled {
			t.Fatal("a shell without a body has nothing to reconcile")
		}
	})
}

// Covers: 合同层是合同正文的第三层——带正文的合同项摘要盖住它：与不带这一层的同一份正文不共一个串；按三层算出的串放行且三个
// 通道各写各的；只带交付条件不带 contractContent 的项没有可比对象，照今天登记声明的串（不改受控批文既有字段语义）。
func TestACustomerContractDeliveryConditionsAreCoveredByTheContractDigest(t *testing.T) {
	declarations := contractDeclarations(t, "rules-1")
	tightened := contractDeliveryDeclaration(t, "method-a")
	declarations.DeliveryConditions = &tightened

	t.Run("the digest covers the third layer", func(t *testing.T) {
		with := customerContractSpec(t, "contract-1", "v1", declarations)
		without := customerContractSpec(t, "contract-1", "v1", contractDeclarations(t, "rules-1"))
		if with.ContentDigest == without.ContentDigest {
			t.Fatal("declaring a contract layer must change the contract digest")
		}
	})

	t.Run("the computed digest publishes all three layers", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         customerContractSpec(t, "contract-1", "v1", declarations),
			Approval:     publishApproval(t, "contract-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: declarations,
		})
		if err != nil || result.Outcome() != application.CommercialVersionPublishedEffective {
			t.Fatalf("outcome = %q, err = %v; want PUBLISHED_EFFECTIVE", result.Outcome(), err)
		}
		if strings.Join(registry.declarationLog, ",") != "pre-acceptance-control,contract-content,delivery-conditions" {
			t.Fatalf("declaration log = %v, want all three layers", registry.declarationLog)
		}
		if len(registry.savedDeliveryConditions) != 1 {
			t.Fatalf("saved delivery conditions = %#v", registry.savedDeliveryConditions)
		}
		if target, tightening := registry.savedDeliveryConditions[0].Tightens(); !tightening || target.ObjectID().String() != "product-1" {
			t.Fatalf("contract layer must name the tightened product version, got %+v / %v", target, tightening)
		}
	})

	t.Run("a legacy declared digest with the third layer is not accepted", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         customerContractSpec(t, "contract-1", "v1", contractDeclarations(t, "rules-1")),
			Approval:     publishApproval(t, "contract-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: declarations,
		})
		if err != nil {
			t.Fatalf("Handle：%v——未受理不是 error", err)
		}
		if result.Outcome() != application.CommercialPublicationNotAccepted || !errors.Is(result.RefusalCause(), domain.ErrDeclaredDigestMismatch) {
			t.Fatalf("outcome = %q, cause = %v; a digest computed without the third layer must not pass", result.Outcome(), result.RefusalCause())
		}
	})

	t.Run("delivery conditions without the contract body have nothing to reconcile", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.CustomerContractObject, "contract-1", "v1"),
			Approval:     publishApproval(t, "contract-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{DeliveryConditions: &tightened},
		})
		if err != nil || result.Outcome() != application.CommercialVersionPublishedEffective {
			t.Fatalf("outcome = %q, err = %v; want PUBLISHED_EFFECTIVE", result.Outcome(), err)
		}
		if strings.Join(registry.declarationLog, ",") != "delivery-conditions" {
			t.Fatalf("declaration log = %v, want the contract layer alone", registry.declarationLog)
		}
	})
}

// Covers: ADR-0126 Decision 三——载体上的交付条件发布时折回发布用例的 DELIVERY_CONDITION 通道（declarationsOfContent 是
// publicationContentOf 的反向）：服务产品载体带这一节 → 册上收到产品层且入册摘要就是载体的摘要；客户合同载体带第三层 →
// 三个通道各写各的、合同层指名所收紧的产品版本；载体上没有这一节就不交这一通道。
func TestDeliveryConditionsOnADraftPublishThroughTheExistingUseCase(t *testing.T) {
	publishDraft := func(t *testing.T, kind domain.CommercialObjectKind, objectID string, content domain.PublicationContent) (*publicationRegistryDouble, domain.PublicationDraft) {
		t.Helper()
		drafts := newDraftRegistryDouble()
		shell := draftShell(t, kind, objectID, "v1")
		submitted, err := application.NewSubmitPublicationDraftHandler(drafts, fixedClock{at: draftSubmittedAt}).Handle(context.Background(),
			application.SubmitPublicationDraftCommand{
				Shell:     shell,
				Content:   content,
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

	t.Run("service product with the product layer", func(t *testing.T) {
		declaration := productDeliveryDeclaration(t, "method-b", "method-a")
		registry, stored := publishDraft(t, domain.ServiceProductObject, "product-1", serviceProductContent(t, &declaration))
		if strings.Join(registry.declarationLog, ",") != "delivery-conditions" {
			t.Fatalf("declaration log = %v, want the product layer alone", registry.declarationLog)
		}
		if len(registry.savedVersions) != 1 || registry.savedVersions[0].ContentDigest() != stored.Canonical().Digest() {
			t.Fatalf("入册摘要与载体摘要不一致：%#v vs %s", registry.savedVersions, stored.Canonical().Digest())
		}
		saved := registry.savedDeliveryConditions[0]
		if methods := saved.Methods(); len(methods) != 2 || methods[0].String() != "method-a" || methods[1].String() != "method-b" {
			t.Fatalf("saved methods = %v", methods)
		}
		if _, tightening := saved.Tightens(); tightening {
			t.Fatal("a product layer must not tighten anything")
		}
	})

	t.Run("service product without the section declares nothing", func(t *testing.T) {
		registry, _ := publishDraft(t, domain.ServiceProductObject, "product-1", serviceProductContent(t, nil))
		if len(registry.declarationLog) != 0 || len(registry.savedDeliveryConditions) != 0 {
			t.Fatalf("a shell-only draft declared %v", registry.declarationLog)
		}
	})

	t.Run("customer contract with the third layer", func(t *testing.T) {
		declarations := contractDeclarations(t, "rules-1")
		tightened := contractDeliveryDeclaration(t, "method-a")
		declarations.DeliveryConditions = &tightened
		registry, stored := publishDraft(t, domain.CustomerContractObject, "contract-1", customerContractContent(t, declarations))
		if strings.Join(registry.declarationLog, ",") != "pre-acceptance-control,contract-content,delivery-conditions" {
			t.Fatalf("declaration log = %v, want all three layers", registry.declarationLog)
		}
		if registry.savedVersions[0].ContentDigest() != stored.Canonical().Digest() {
			t.Fatalf("入册摘要与载体摘要不一致：%s vs %s", registry.savedVersions[0].ContentDigest(), stored.Canonical().Digest())
		}
		if target, tightening := registry.savedDeliveryConditions[0].Tightens(); !tightening || target.Version().String() != "v1" {
			t.Fatalf("contract layer on the draft lost its tightened product version: %+v / %v", target, tightening)
		}
	})
}
