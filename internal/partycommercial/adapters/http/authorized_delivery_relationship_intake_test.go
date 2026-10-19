package commercialhttp_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Covers: 登记写面认授权交付方这一种角色（ADR-0212 决定一）：角色名过翻译，范围串原样进命令，由用例按领域门判它的写法；
// 落册、重放与冲突走同一个参与方关系登记用例，代数不另起。
func TestOperatorRegistryIntakeTranslatesAnAuthorizedDeliveryRelationship(t *testing.T) {
	intake := operatorIntake(t)
	command, err := intake.IntakePartyRelationshipRegistration(context.Background(),
		identityRequest("/commercial-party-relationship-registrations", `{"relationships":[{
		"relationshipId":"SYN-REL-AUTHORIZED-DELIVERY-01",
		"revision":1,
		"holder":"SYN-PARTY-DELIVERER-01",
		"counterparty":"SYN-PARTY-SHIPPER-01",
		"role":"AUTHORIZED_DELIVERING_PARTY",
		"scope":"customerAccount=SYN-ACCOUNT-01&node=SYN-NODE-SHA-HUB",
		"basis":"SYN-DELIVERY-MANDATE-01",
		"effectiveStartsAt":"2026-01-05T00:00:00Z"
	}]}`))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if command.Spec.Role != domain.AuthorizedDeliveringPartyRole {
		t.Fatalf("Role = %s, want AUTHORIZED_DELIVERING_PARTY", command.Spec.Role)
	}
	if got := command.Spec.Scope.String(); got != "customerAccount=SYN-ACCOUNT-01&node=SYN-NODE-SHA-HUB" {
		t.Fatalf("Scope = %q", got)
	}
}
