package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// Covers: CONTEXT「客户服务规则必须按服务产品和客户合同明确适用范围、有效期间及责任方」，
// 以及 ADR-0093 的归族判据在构造门上的落点。
//
// 类别那一格最有后果：挂错类别的规则版本仍是一个合法的商业版本，入册、被解析选中都不会报错，
// 只是解析按错的类别去找——那正是 ADR-0093 否决复用 AuthorizationRuleObject 的理由。构造门若
// 不判类别，这个错要到下游取不到规则依据时才显形，而那时它长得像「这个客户没配规则」。
func TestCustomerServiceRuleVersionRefusesAVersionOfAnotherKind(t *testing.T) {
	responsible := commercialValue(t, domain.NewPartyID, "operator-1")
	scope := commercialValue(t, domain.NewCommercialScopeReference, "scope-a")
	product := commercialValue(t, domain.NewCommercialObjectID, "product-1")

	t.Run("a customer service rule version is accepted", func(t *testing.T) {
		rule, err := domain.NewCustomerServiceRuleVersion(
			effectiveVersionOfKind(t, domain.CustomerServiceRuleObject, "csr-1"),
			domain.CustomerServiceRuleAppliesToServiceProduct(product),
			responsible, scope,
		)
		if err != nil {
			t.Fatalf("new customer service rule version: %v", err)
		}
		if rule.ResponsibleParty() != responsible {
			t.Fatal("责任方没有原样留在规则版本上")
		}
	})

	t.Run("a version of another kind is refused", func(t *testing.T) {
		// 授权规则也是集内成员、也能生效，两者在版本壳上唯一的差别就是类别。
		_, err := domain.NewCustomerServiceRuleVersion(
			effectiveVersionOfKind(t, domain.AuthorizationRuleObject, "auth-1"),
			domain.CustomerServiceRuleAppliesToServiceProduct(product),
			responsible, scope,
		)
		if !errors.Is(err, domain.ErrInvalidCustomerServiceRuleVersion) {
			t.Fatalf("error = %v，别的类别的版本挂成了客户服务规则", err)
		}
	})

	t.Run("an applicability that names nothing is refused", func(t *testing.T) {
		// 零值适用对象过不了门：CONTEXT 要的是「明确适用范围」，而「没说挂在哪」与
		// 「挂在一个尚未指明的对象上」在零值里长得一样。
		_, err := domain.NewCustomerServiceRuleVersion(
			effectiveVersionOfKind(t, domain.CustomerServiceRuleObject, "csr-2"),
			domain.CustomerServiceRuleApplicability{},
			responsible, scope,
		)
		if !errors.Is(err, domain.ErrInvalidCustomerServiceRuleVersion) {
			t.Fatalf("error = %v，没有适用对象的规则版本立住了", err)
		}
	})
}

func effectiveVersionOfKind(
	t *testing.T,
	kind domain.CommercialObjectKind,
	objectID string,
) domain.CommercialVersion {
	t.Helper()
	version := registerable(t, kind, objectID, "v1", "sha256:"+objectID)
	live, err := version.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	return live
}
