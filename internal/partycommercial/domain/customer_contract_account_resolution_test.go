package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件钉 ADR-0177 决定二：闭包按解析键上的客户账户收窄客户合同候选。形状照 ADR-0153 的声明服务产品收窄——正文指名
// 了另一户的落选，正文未登记的照旧参选；收窄之后零候选仍是`无适用依据`、多候选仍是`适用冲突`。

func contractForAccount(t *testing.T, registry *domain.CommercialRegistry, objectID, account string) domain.CommercialVersion {
	t.Helper()
	version := effectiveIn(t, registry, domain.CustomerContractObject, objectID, "v1", "sha256:"+objectID, "scope-a")
	if err := registry.RegisterCustomerContractAccount(version, commercialValue(t, domain.NewCustomerAccountID, account)); err != nil {
		t.Fatalf("register customer contract account: %v", err)
	}
	return version
}

func contractKeyFor(t *testing.T, account string) domain.ResolutionKey {
	t.Helper()
	key := resolutionKey(t, "scope-a", domain.CustomerContractObject)
	key.CustomerAccountID = commercialValue(t, domain.NewCustomerAccountID, account)
	return key
}

// Covers: ADR-0177 决定二与票 07 完成判据一——同一范围两户各一份合同，各户解出自己那一份，不再`适用冲突`。
func TestEachCustomerAccountResolvesItsOwnContractInASharedScope(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	first := contractForAccount(t, registry, "contract-1", "customer-1")
	second := contractForAccount(t, registry, "contract-2", "customer-2")

	for account, want := range map[string]domain.CommercialVersion{"customer-1": first, "customer-2": second} {
		result := domain.ResolveCommercialBasis(registry, contractKeyFor(t, account), nil)
		if result.Outcome() != domain.UniquelyResolved {
			t.Fatalf("%s: outcome = %q, want UNIQUELY_RESOLVED", account, result.Outcome())
		}
		adopted, _ := result.AdoptedVersion()
		if adopted.ObjectID() != want.ObjectID() {
			t.Fatalf("%s 解出了 %s，要它自己那一份 %s", account, adopted.ObjectID(), want.ObjectID())
		}
	}
}

// Covers: ADR-0177 决定二「不等的落选」——范围里只有别户的合同时，这一户答`无适用依据`，不借别户的合同。
func TestAnotherAccountsContractIsNotACandidate(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	contractForAccount(t, registry, "contract-2", "customer-2")

	result := domain.ResolveCommercialBasis(registry, contractKeyFor(t, "customer-1"), nil)
	if result.Outcome() != domain.NoApplicableBasis {
		t.Fatalf("outcome = %q, want NO_APPLICABLE_BASIS", result.Outcome())
	}
}

// Covers: ADR-0177 决定二「同一户在同一范围同时生效两份合同照旧是冲突，不任选」。
func TestTwoContractsOfOneAccountAreStillAConflict(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	contractForAccount(t, registry, "contract-1a", "customer-1")
	contractForAccount(t, registry, "contract-1b", "customer-1")
	contractForAccount(t, registry, "contract-2", "customer-2")

	result := domain.ResolveCommercialBasis(registry, contractKeyFor(t, "customer-1"), nil)
	if result.Outcome() != domain.ApplicabilityConflict {
		t.Fatalf("outcome = %q, want APPLICABILITY_CONFLICT", result.Outcome())
	}
}

// Covers: 收窄与 ADR-0153 同形——正文未登记（登记册里没有这一版的账户）的合同照旧参选，问题留给正文点读口；一户的
// 合同正文在场、另一份只有壳时，两份都对这一户参选，仍是冲突。
func TestAContractWithoutARegisteredBodyStaysACandidate(t *testing.T) {
	t.Run("只有壳的那一份照旧唯一解析", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		effectiveIn(t, registry, domain.CustomerContractObject, "contract-shell", "v1", "sha256:shell", "scope-a")

		result := domain.ResolveCommercialBasis(registry, contractKeyFor(t, "customer-1"), nil)
		if result.Outcome() != domain.UniquelyResolved {
			t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", result.Outcome())
		}
	})
	t.Run("只有壳的与本户的并存仍冲突", func(t *testing.T) {
		registry := domain.NewCommercialRegistry()
		effectiveIn(t, registry, domain.CustomerContractObject, "contract-shell", "v1", "sha256:shell", "scope-a")
		contractForAccount(t, registry, "contract-1", "customer-1")

		result := domain.ResolveCommercialBasis(registry, contractKeyFor(t, "customer-1"), nil)
		if result.Outcome() != domain.ApplicabilityConflict {
			t.Fatalf("outcome = %q, want APPLICABILITY_CONFLICT", result.Outcome())
		}
	})
}

// Covers: 收窄只对客户合同那一类——别的类别不因键上的账户落选。
func TestAccountNarrowingTouchesOnlyCustomerContracts(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	effectiveIn(t, registry, domain.AcceptanceRulePackageObject, "rules-1", "v1", "sha256:rules", "scope-a")
	key := resolutionKey(t, "scope-a", domain.AcceptanceRulePackageObject)
	key.CustomerAccountID = commercialValue(t, domain.NewCustomerAccountID, "customer-9")

	if result := domain.ResolveCommercialBasis(registry, key, nil); result.Outcome() != domain.UniquelyResolved {
		t.Fatalf("outcome = %q, want UNIQUELY_RESOLVED", result.Outcome())
	}
}

// Covers: 登记口只收客户合同、只收非空账户。
func TestRegisteringAContractAccountRefusesOtherKindsAndBlankAccounts(t *testing.T) {
	registry := domain.NewCommercialRegistry()
	rules := effectiveIn(t, registry, domain.AcceptanceRulePackageObject, "rules-1", "v1", "sha256:rules", "scope-a")
	if err := registry.RegisterCustomerContractAccount(rules, commercialValue(t, domain.NewCustomerAccountID, "customer-1")); !errors.Is(err, domain.ErrInvalidCustomerContract) {
		t.Fatalf("规则包也登了账户：err = %v", err)
	}
	contract := effectiveIn(t, registry, domain.CustomerContractObject, "contract-1", "v1", "sha256:c1", "scope-a")
	if err := registry.RegisterCustomerContractAccount(contract, domain.CustomerAccountID{}); !errors.Is(err, domain.ErrInvalidCustomerContract) {
		t.Fatalf("空账户被登了：err = %v", err)
	}
}
