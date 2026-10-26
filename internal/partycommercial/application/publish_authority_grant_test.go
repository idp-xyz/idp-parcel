package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

func authorityGrantDeclarationOf(t *testing.T, level string) *application.AuthorityGrantDeclaration {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("interval: %v", err)
	}
	return &application.AuthorityGrantDeclaration{
		Action:      domain.SourceDataAmendmentAction,
		LegalEntity: pcValue(t, domain.NewLegalEntityReference, "legal-1"),
		Level:       pcValue(t, domain.NewAuthorityLevel, level),
		Scope:       pcValue(t, domain.NewCommercialScopeReference, "scope-a"),
		Effective:   interval,
	}
}

// Covers: AT-PC-063 的发布编排那一半——授权规则版本声明的授予随该版本同一次发布登记，原样到达持久化面，
// 并向下游发「授权规则已登记」的交接（等级是租户登记的串，编排不认任何值）；同版本再带取消授权目录整项拒，
// 因为内容摘要盖不住授予。
func TestAnAuthorityGrantPublishesWithItsRuleVersion(t *testing.T) {
	registry := &publicationRegistryDouble{}
	handoff := &operatorRegistrationHandoffDouble{}
	handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, handoff)

	declarations := application.CommercialDeclarations{AuthorityGrant: authorityGrantDeclarationOf(t, "CUSTOMER_SELF_SERVICE")}
	result, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
		Spec:         publishSpec(t, domain.AuthorizationRuleObject, "auth-amend", "v1"),
		Approval:     publishApproval(t, "auth-amend"),
		RoleStanding: domain.ApprovalRoleConfirmed,
		Declarations: declarations,
	})
	if err != nil {
		t.Fatalf("Handle：%v", err)
	}
	if result.Outcome() != application.CommercialVersionPublishedEffective {
		t.Fatalf("outcome = %q, want PUBLISHED_EFFECTIVE", result.Outcome())
	}
	if len(registry.savedGrants) != 1 {
		t.Fatalf("授权册收到 %d 条, want 1", len(registry.savedGrants))
	}
	saved := registry.savedGrants[0]
	if saved.Version().ObjectID().String() != "auth-amend" || saved.Action() != domain.SourceDataAmendmentAction ||
		saved.Level().String() != "CUSTOMER_SELF_SERVICE" || saved.LegalEntity().String() != "legal-1" || saved.Scope().String() != "scope-a" {
		t.Fatalf("授予 = %#v，没有原样到达持久化面", saved)
	}
	reports := result.Declarations()
	if len(reports) != 1 || reports[0].Channel != application.AuthorityGrantChannel ||
		reports[0].Channel.String() != "AUTHORITY_GRANT" || reports[0].Outcome != ports.DeclarationSaved {
		t.Fatalf("报告 = %#v, want AUTHORITY_GRANT=SAVED 一条", reports)
	}
	if len(handoff.intents) != 1 || handoff.intents[0].Kind != ports.AuthorityGrantRegistered {
		t.Fatalf("交接 = %#v, want 一封 AUTHORITY_GRANT", handoff.intents)
	}

	t.Run("同一版再带取消授权目录整项拒", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		rows := cancellationRows(t, "rule-1", "rule-2")
		combined := application.CommercialDeclarations{
			AuthorityGrant:        authorityGrantDeclarationOf(t, "CUSTOMER_SELF_SERVICE"),
			CancellationAuthority: rows,
		}
		_, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         authorizationRuleSpec(t, "auth-both", "v1", rows),
			Approval:     publishApproval(t, "auth-both"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: combined,
		})
		if !errors.Is(err, domain.ErrInvalidAuthorityGrant) {
			t.Fatalf("err = %v, want ErrInvalidAuthorityGrant", err)
		}
		if len(registry.savedGrants) != 0 {
			t.Fatal("被拒的发布写了授予")
		}
	})

	t.Run("挂在非授权规则版本上由领域构造门拒", func(t *testing.T) {
		registry := &publicationRegistryDouble{}
		handler := application.NewPublishCommercialAuthorityHandler(registry, fixedClock{at: pubNow}, &operatorRegistrationHandoffDouble{})
		_, err := handler.Handle(context.Background(), application.PublishCommercialAuthorityCommand{
			Spec:         publishSpec(t, domain.CreditPolicyObject, "credit-1", "v1"),
			Approval:     publishApproval(t, "credit-1"),
			RoleStanding: domain.ApprovalRoleConfirmed,
			Declarations: declarations,
		})
		if !errors.Is(err, domain.ErrInvalidAuthorityGrant) {
			t.Fatalf("err = %v, want ErrInvalidAuthorityGrant", err)
		}
	})
}
