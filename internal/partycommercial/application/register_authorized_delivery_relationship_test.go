package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func registerAccount(t *testing.T, handler *application.RegisterPartyIdentityHandler, tenant, account, owner string) {
	t.Helper()
	result, err := handler.RegisterCustomerAccount(context.Background(), application.RegisterCustomerAccountCommand{
		Tenant:        identityValue(t, domain.NewTenantID, tenant),
		Account:       identityValue(t, domain.NewCustomerAccountID, account),
		CustomerParty: identityValue(t, domain.NewPartyID, owner),
		Revision:      1,
		Basis:         identityValue(t, domain.NewIdentityBasisReference, "basis-"+account),
		EffectiveFrom: entityEffectiveFrom,
	})
	if err != nil || result.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("register account %s = (%v, %v)", account, result.Outcome(), err)
	}
}

func authorizedDeliveryCommand(t *testing.T, id, counterparty, scope string) application.RegisterPartyRelationshipCommand {
	t.Helper()
	interval, err := domain.NewEffectiveInterval(relationStartsAt, time.Time{})
	if err != nil {
		t.Fatalf("new interval: %v", err)
	}
	return application.RegisterPartyRelationshipCommand{
		Tenant:   identityValue(t, domain.NewTenantID, "tenant-1"),
		ID:       identityValue(t, domain.NewRelationshipID, id),
		Revision: 1,
		Spec: domain.PartyRelationshipSpec{
			Holder:       identityValue(t, domain.NewPartyID, "party-deliverer"),
			Counterparty: identityValue(t, domain.NewPartyID, counterparty),
			Role:         domain.AuthorizedDeliveringPartyRole,
			Scope:        identityValue(t, domain.NewCommercialScopeReference, scope),
			Basis:        identityValue(t, domain.NewRelationshipBasisReference, "mandate-"+id),
			Effective:    interval,
		},
		Approval: &application.RelationshipApproval{
			Reference:  identityValue(t, domain.NewApprovalReference, "approval-"+id),
			ApprovedAt: relationStartsAt,
		},
	}
}

// Covers: 完成判据「登记写面与受控批量口能登记新角色，重放与冲突照既有参与方关系口的代数」的用例半边——授权交付方
// 关系与别的关系走同一个登记用例：落册、同键同内容答重放、同修订异内容答冲突。另加本角色自己的引用门：范围里的
// 客户账户要已登记，且相对方就是它所属的参与方（PC CONTEXT「参与方关系」：相对方是该客户账户所属参与方）。
func TestAuthorizedDeliveryRelationshipRegistersThroughTheRelationshipUseCase(t *testing.T) {
	registry := newFakePartyRegistry()
	handler := application.NewRegisterPartyIdentityHandler(registry, nil)
	ctx := context.Background()

	registerParty(t, handler, "tenant-1", "party-deliverer", "合成交付方")
	registerParty(t, handler, "tenant-1", "party-shipper", "合成货主")
	registerParty(t, handler, "tenant-1", "party-other", "合成另一货主")

	command := authorizedDeliveryCommand(t, "rel-delivery-1", "party-shipper", "customerAccount=account-shipper")
	unregistered, err := handler.RegisterRelationship(ctx, command)
	if err != nil || unregistered.Outcome() != application.PartyIdentityNotAccepted {
		t.Fatalf("scope account unregistered = (%v, %v), want NOT_ACCEPTED", unregistered.Outcome(), err)
	}

	registerAccount(t, handler, "tenant-1", "account-shipper", "party-shipper")

	landed, err := handler.RegisterRelationship(ctx, command)
	if err != nil || landed.Outcome() != application.PartyIdentityRegistered {
		t.Fatalf("authorized delivery relationship = (%v, %v, %v)", landed.Outcome(), landed.Cause(), err)
	}
	stored, found, err := registry.LoadLatestRelationship(ctx,
		identityValue(t, domain.NewTenantID, "tenant-1"),
		identityValue(t, domain.NewRelationshipID, "rel-delivery-1"))
	if err != nil || !found {
		t.Fatalf("load stored relationship: (%v, %v)", found, err)
	}
	if stored.Relationship().Role() != domain.AuthorizedDeliveringPartyRole {
		t.Fatalf("stored role = %s, want AUTHORIZED_DELIVERING_PARTY", stored.Relationship().Role())
	}

	replayed, err := handler.RegisterRelationship(ctx, command)
	if err != nil || replayed.Outcome() != application.PartyIdentityAlreadyRegistered {
		t.Fatalf("replay = (%v, %v), want ALREADY_REGISTERED", replayed.Outcome(), err)
	}

	conflicting := command
	conflicting.Spec.Basis = identityValue(t, domain.NewRelationshipBasisReference, "mandate-amended")
	conflicted, err := handler.RegisterRelationship(ctx, conflicting)
	if err != nil || conflicted.Outcome() != application.PartyIdentityContentConflict {
		t.Fatalf("same revision other content = (%v, %v), want CONTENT_CONFLICT", conflicted.Outcome(), err)
	}

	foreignOwner := authorizedDeliveryCommand(t, "rel-delivery-2", "party-other", "customerAccount=account-shipper")
	refused, err := handler.RegisterRelationship(ctx, foreignOwner)
	if err != nil || refused.Outcome() != application.PartyIdentityNotAccepted {
		t.Fatalf("counterparty is not the account owner = (%v, %v), want NOT_ACCEPTED", refused.Outcome(), err)
	}

	opaque := authorizedDeliveryCommand(t, "rel-delivery-3", "party-shipper", "SYN-SCOPE-01")
	unreadable, err := handler.RegisterRelationship(ctx, opaque)
	if err != nil || unreadable.Outcome() != application.PartyIdentityNotAccepted {
		t.Fatalf("opaque scope = (%v, %v), want NOT_ACCEPTED", unreadable.Outcome(), err)
	}
}
