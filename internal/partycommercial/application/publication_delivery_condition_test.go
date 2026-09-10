package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件钉票 party-commercial-context-gaps/11「要建什么」3 的发布通道半边（ADR-0133 决定四）：交付条件随服务产品
// 版本（产品层）或客户合同版本（合同层）发布、走自己的 DELIVERY_CONDITION 通道；层由发布的版本类别定，合同层必须
// 指名所收紧的产品版本、产品层必须不带；三格正文原样到达持久化面，通道不代填任何一种方式或规则。

func deliveryTermsFor(t *testing.T, methods ...string) domain.DeliveryConditionTerms {
	t.Helper()
	terms := domain.DeliveryConditionTerms{
		RecipientScopeRule:  pcValue(t, domain.NewDeliveryRuleReference, "RULE/recipient-scope"),
		ProofOfDeliveryRule: pcValue(t, domain.NewDeliveryRuleReference, "RULE/proof-of-delivery"),
	}
	for _, method := range methods {
		terms.Methods = append(terms.Methods, pcValue(t, domain.NewDeliveryMethodReference, method))
	}
	return terms
}

func tightensProduct(t *testing.T, objectID, version string) *domain.TightenedProductVersion {
	t.Helper()
	target, err := domain.NewTightenedProductVersion(
		pcValue(t, domain.NewCommercialObjectID, objectID),
		pcValue(t, domain.NewCommercialVersionLabel, version),
	)
	if err != nil {
		t.Fatalf("所收紧的产品版本：%v", err)
	}
	return &target
}

// Covers: 产品层随服务产品版本发布——方式集合与两条规则引用原样到达持久化面、拥有对象是已生效的那一版；报告一条
// DELIVERY_CONDITION=SAVED。合同层随客户合同版本发布并指名所收紧的产品版本，原样到达。缺键（nil）就是没这一节。
func TestDeliveryConditionsPublishWithTheirOwningVersion(t *testing.T) {
	t.Run("产品层", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.ServiceProductObject, "product-1", "v1"),
			Approval:     publishApproval(t, "product-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{DeliveryConditions: &application.DeliveryConditionDeclaration{
				Terms: deliveryTermsFor(t, "method-b", "method-a"),
			}},
		})
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if result.Outcome() != application.CommercialVersionPublishedEffective {
			t.Fatalf("outcome = %q, want PUBLISHED_EFFECTIVE", result.Outcome())
		}
		if len(registry.savedDeliveryConditions) != 1 {
			t.Fatalf("交付条件册收到 %d 份, want 1", len(registry.savedDeliveryConditions))
		}
		saved := registry.savedDeliveryConditions[0]
		if saved.Owner().ObjectID().String() != "product-1" || saved.Owner().Kind() != domain.ServiceProductObject ||
			saved.Owner().Status() != domain.CommercialVersionEffective {
			t.Fatalf("拥有对象 = %s/%s %q", saved.Owner().Kind(), saved.Owner().ObjectID(), saved.Owner().Status())
		}
		methods := saved.Methods()
		if len(methods) != 2 || methods[0].String() != "method-a" || methods[1].String() != "method-b" ||
			saved.RecipientScopeRule().String() != "RULE/recipient-scope" || saved.ProofOfDeliveryRule().String() != "RULE/proof-of-delivery" {
			t.Fatalf("正文没有原样到达持久化面：methods=%v rules=%s/%s", methods, saved.RecipientScopeRule(), saved.ProofOfDeliveryRule())
		}
		if _, tightening := saved.Tightens(); tightening {
			t.Fatal("产品层带上了所收紧的产品版本")
		}
		reports := result.Declarations()
		if len(reports) != 1 ||
			reports[0].Channel != application.DeliveryConditionChannel ||
			reports[0].Channel.String() != "DELIVERY_CONDITION" ||
			reports[0].Outcome != ports.DeclarationSaved {
			t.Fatalf("报告 = %#v, want DELIVERY_CONDITION=SAVED 一条", reports)
		}
	})

	t.Run("合同层指名所收紧的产品版本", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.CustomerContractObject, "contract-1", "v1"),
			Approval:     publishApproval(t, "contract-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{DeliveryConditions: &application.DeliveryConditionDeclaration{
				Tightens: tightensProduct(t, "product-1", "v3"),
				Terms:    deliveryTermsFor(t, "method-a"),
			}},
		})
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if len(registry.savedDeliveryConditions) != 1 {
			t.Fatalf("交付条件册收到 %d 份, want 1", len(registry.savedDeliveryConditions))
		}
		saved := registry.savedDeliveryConditions[0]
		target, tightening := saved.Tightens()
		if saved.Owner().Kind() != domain.CustomerContractObject || !tightening ||
			target.ObjectID().String() != "product-1" || target.Version().String() != "v3" || len(saved.Methods()) != 1 {
			t.Fatalf("合同层没有原样到达持久化面：owner=%s tightens=%+v/%v methods=%d", saved.Owner().Kind(), target, tightening, len(saved.Methods()))
		}
		if reports := result.Declarations(); len(reports) != 1 || reports[0].Channel != application.DeliveryConditionChannel {
			t.Fatalf("报告 = %#v, want DELIVERY_CONDITION 一条", reports)
		}
	})

	t.Run("缺键就是没这一节", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.ServiceProductObject, "product-1", "v1"),
			Approval:     publishApproval(t, "product-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
		})
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if len(registry.savedDeliveryConditions) != 0 || len(result.Declarations()) != 0 {
			t.Fatal("没给这一节却登了一份声明")
		}
	})
}

// Covers: 通道的门与其余通道同一条纪律——零方式 / 同方式两行 / 挂在接单规则包上 / 合同层不指名所收紧的产品版本 /
// 产品层却带了所收紧的产品版本，都在触碰持久化面之前整项拒且一行不写；册的`内容冲突`折进报告而不是 error（ADR-0031）。
func TestDeliveryConditionsAreGuardedLikeTheOtherChannels(t *testing.T) {
	for name, tc := range map[string]struct {
		kind        domain.CommercialObjectKind
		declaration application.DeliveryConditionDeclaration
		want        error
	}{
		"产品层零方式是缺件":      {domain.ServiceProductObject, application.DeliveryConditionDeclaration{Terms: deliveryTermsFor(t)}, domain.ErrDeliveryConditionNotConfigured},
		"同方式两行":          {domain.ServiceProductObject, application.DeliveryConditionDeclaration{Terms: deliveryTermsFor(t, "method-a", "method-a")}, domain.ErrConflictingDeliveryCondition},
		"挂在接单规则包上":       {domain.AcceptanceRulePackageObject, application.DeliveryConditionDeclaration{Terms: deliveryTermsFor(t, "method-a")}, domain.ErrDeliveryConditionOwner},
		"合同层不指名所收紧的产品版本": {domain.CustomerContractObject, application.DeliveryConditionDeclaration{Terms: deliveryTermsFor(t, "method-a")}, domain.ErrDeliveryConditionNotConfigured},
		"产品层却带了所收紧的产品版本": {domain.ServiceProductObject, application.DeliveryConditionDeclaration{Tightens: tightensProduct(t, "product-1", "v1"), Terms: deliveryTermsFor(t, "method-a")}, domain.ErrDeliveryConditionOwner},
	} {
		t.Run(name, func(t *testing.T) {
			registry := &publicationRegistryDouble{}
			handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
			declaration := tc.declaration
			if _, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
				Spec:         publishSpec(t, tc.kind, "owner-1", "v1"),
				Approval:     publishApproval(t, "owner-1"),
				RoleStanding: domain.ApprovalRoleConfirmed,
				Declarations: application.CommercialDeclarations{DeliveryConditions: &declaration},
			}); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if len(registry.savedVersions) != 0 || len(registry.savedDeliveryConditions) != 0 {
				t.Fatal("拒收的发布写了库")
			}
		})
	}

	t.Run("内容冲突落在报告里", func(t *testing.T) {
		registry := &publicationRegistryDouble{declarationOutcome: ports.DeclarationContentConflict}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.ServiceProductObject, "product-1", "v1"),
			Approval:     publishApproval(t, "product-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: application.CommercialDeclarations{DeliveryConditions: &application.DeliveryConditionDeclaration{
				Terms: deliveryTermsFor(t, "method-a"),
			}},
		})
		if err != nil {
			t.Fatalf("Handle：%v", err)
		}
		if reports := result.Declarations(); len(reports) != 1 || reports[0].Outcome != ports.DeclarationContentConflict {
			t.Fatalf("报告 = %#v, want DELIVERY_CONDITION=CONTENT_CONFLICT", reports)
		}
	})
}
