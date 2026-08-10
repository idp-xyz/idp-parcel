package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func contractVersion(t *testing.T, objectID string) domain.CommercialVersion {
	t.Helper()
	live, err := registerable(t, domain.CustomerContractObject, objectID, "v1", "sha256:"+objectID).
		TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	return live
}

func appliedControl(t *testing.T, scope string) domain.FinancialControlBinding {
	t.Helper()
	binding, err := domain.NewAppliedFinancialControl(
		commercialValue(t, domain.NewChargeScopeReference, scope),
		commercialValue(t, domain.NewCommercialObjectID, "policy-"+scope),
	)
	if err != nil {
		t.Fatalf("new applied financial control: %v", err)
	}
	return binding
}

func inapplicableControl(t *testing.T, scope, basis string) domain.FinancialControlBinding {
	t.Helper()
	binding, err := domain.NewInapplicableFinancialControl(
		commercialValue(t, domain.NewChargeScopeReference, scope),
		commercialValue(t, domain.NewInapplicabilityBasis, basis),
	)
	if err != nil {
		t.Fatalf("new inapplicable financial control: %v", err)
	}
	return binding
}

func contractContent(t *testing.T, bindings ...domain.FinancialControlBinding) domain.CustomerContract {
	t.Helper()
	contract, err := domain.NewCustomerContract(
		contractVersion(t, "contract-1"),
		commercialValue(t, domain.NewCommercialObjectID, "rules-1"),
		bindings,
	)
	if err != nil {
		t.Fatalf("new customer contract: %v", err)
	}
	return contract
}

// Covers: CONTEXT「每个用于新委托接受的客户合同版本必须明确引用适用接单规则包和接受前
// 财务控制策略」。
func TestContractNamesItsRulePackageAndControlBindings(t *testing.T) {
	contract := contractContent(t, appliedControl(t, "charge-express"))

	if contract.AcceptanceRulePackage().String() != "rules-1" {
		t.Fatalf("rule package = %q, want rules-1", contract.AcceptanceRulePackage())
	}
	binding, present := contract.FinancialControlFor(commercialValue(t, domain.NewChargeScopeReference, "charge-express"))
	if !present || !binding.Applies() {
		t.Fatalf("control binding = %#v present = %v", binding, present)
	}
	policy, named := binding.Policy()
	if !named || policy.String() != "policy-charge-express" {
		t.Fatal("an applied control does not name its policy")
	}
}

// Covers: CONTEXT「规则或策略缺失不得被解释为允许接受」— 未绑定的范围既不是「适用」
// 也不是「明确无控制」，查询必须报告缺席而不是给出一个放行答案。
func TestAnUnboundScopeIsAbsentRatherThanPermissive(t *testing.T) {
	contract := contractContent(t, appliedControl(t, "charge-express"))

	binding, present := contract.FinancialControlFor(commercialValue(t, domain.NewChargeScopeReference, "charge-economy"))
	if present {
		t.Fatal("an unbound scope reported a binding")
	}
	if binding.Applies() {
		t.Fatal("the zero binding reads as an applicable control")
	}
	if binding.ExplicitlyInapplicable() {
		t.Fatal("the zero binding reads as an explicit no-control basis, which would permit acceptance")
	}
}

// Covers: CONTEXT「确实不适用的控制必须按明确范围记录不适用依据」— 无控制必须是一个
// 带依据的明确声明，这正是 settlement-accounting 不得用零金额冻结冒充的那个依据。
func TestNoControlMustBeAnExplicitBasisNotAnOmission(t *testing.T) {
	contract := contractContent(t, inapplicableControl(t, "charge-economy", "CONTRACT_STATES_NO_PRE_ACCEPTANCE_CONTROL"))

	binding, present := contract.FinancialControlFor(commercialValue(t, domain.NewChargeScopeReference, "charge-economy"))
	if !present || !binding.ExplicitlyInapplicable() {
		t.Fatalf("binding = %#v present = %v", binding, present)
	}
	if binding.Applies() {
		t.Fatal("an inapplicable control also reported as applicable")
	}
	if _, named := binding.Policy(); named {
		t.Fatal("an inapplicable control named a policy")
	}
	if binding.InapplicabilityBasis().String() == "" {
		t.Fatal("an inapplicable control carries no basis, making it indistinguishable from an omission")
	}

	if _, err := domain.NewInapplicableFinancialControl(
		commercialValue(t, domain.NewChargeScopeReference, "charge-x"),
		domain.InapplicabilityBasis{},
	); !errors.Is(err, domain.ErrInvalidFinancialControlBinding) {
		t.Fatal("an inapplicability without a basis was constructed")
	}
}

// Covers: CONTEXT — 同一费用范围不得既绑定策略又声明不适用，那样两种读法都成立。
func TestOneScopeCannotBeBothAppliedAndInapplicable(t *testing.T) {
	_, err := domain.NewCustomerContract(
		contractVersion(t, "contract-1"),
		commercialValue(t, domain.NewCommercialObjectID, "rules-1"),
		[]domain.FinancialControlBinding{
			appliedControl(t, "charge-express"),
			inapplicableControl(t, "charge-express", "CONTRACT_STATES_NO_PRE_ACCEPTANCE_CONTROL"),
		},
	)
	if !errors.Is(err, domain.ErrConflictingFinancialControlBinding) {
		t.Fatalf("error = %v, want ErrConflictingFinancialControlBinding", err)
	}
}

// Covers: CONTEXT — 合同必须引用接单规则包；缺失即建不成内容，不能留给接受判断去发现。
func TestContractWithoutARulePackageCannotBeBuilt(t *testing.T) {
	if _, err := domain.NewCustomerContract(
		contractVersion(t, "contract-1"),
		domain.CommercialObjectID{},
		[]domain.FinancialControlBinding{appliedControl(t, "charge-express")},
	); !errors.Is(err, domain.ErrInvalidCustomerContract) {
		t.Fatalf("error = %v, want ErrInvalidCustomerContract", err)
	}
}

// Covers: CONTEXT 商业版本共同不变量 — 合同内容挂在一个当前可用的客户合同版本上。
func TestContractContentNeedsAUsableContractVersion(t *testing.T) {
	bindings := []domain.FinancialControlBinding{appliedControl(t, "charge-express")}

	t.Run("refuses a draft", func(t *testing.T) {
		draft := commercialDraft(t, domain.CustomerContractObject, "contract-x", "v1", "sha256:x")
		if _, err := domain.NewCustomerContract(draft, commercialValue(t, domain.NewCommercialObjectID, "rules-1"), bindings); !errors.Is(err, domain.ErrInvalidCustomerContract) {
			t.Fatalf("error = %v, want ErrInvalidCustomerContract", err)
		}
	})

	t.Run("refuses another object kind", func(t *testing.T) {
		policy := registerable(t, domain.SettlementPolicyObject, "policy-9", "v1", "sha256:p9")
		live, err := policy.TakeEffect(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatalf("take effect: %v", err)
		}
		if _, err := domain.NewCustomerContract(live, commercialValue(t, domain.NewCommercialObjectID, "rules-1"), bindings); !errors.Is(err, domain.ErrInvalidCustomerContract) {
			t.Fatalf("error = %v, want ErrInvalidCustomerContract", err)
		}
	})
}
