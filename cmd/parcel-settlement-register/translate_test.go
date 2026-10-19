package main

import (
	"testing"
)

// 本文件证命令表的封闭与译装的拒绝面接进了本口：集合外命令拒、每条命令都拒未知字段、形状坏当场拒——
// 一律在进事务之前。译装本身逐格的保真与拒绝在 `adapters/registrationjson` 的用例里证，这里不重证。

// TestCommandForRejectsUnknownCommand 证命令集封闭：CC 那本的核对命令与不存在的名字都拒。
func TestCommandForRejectsUnknownCommand(t *testing.T) {
	for _, command := range []string{"duty-payment-verification", "funds-apply", ""} {
		if _, err := commandFor(command, []byte(`{}`)); err == nil {
			t.Fatalf("集合外命令 %q 没拒", command)
		}
	}
}

// TestCommandForRejectsUnknownFieldsOnEveryCommand 证命令表上每一条都拒未知字段：打错的键静默丢弃会让操作员
// 以为登进去的比实际多。
func TestCommandForRejectsUnknownFieldsOnEveryCommand(t *testing.T) {
	for _, command := range allCommands {
		if _, err := commandFor(command, []byte(`{"typo": 1}`)); err == nil {
			t.Fatalf("%s 未拒未知字段", command)
		}
	}
}

// TestCommandForRejectsAnEmptyDocumentOnEveryCommand 证空文档拒在译装：租户是构造门上的标识，空载荷连租户都没有。
func TestCommandForRejectsAnEmptyDocumentOnEveryCommand(t *testing.T) {
	for _, command := range allCommands {
		if _, err := commandFor(command, []byte(`{}`)); err == nil {
			t.Fatalf("%s 收下了空文档", command)
		}
	}
}

// TestCommandForAcceptsBothCommands 证两条命令各自译装得过并交回可执行的调用（正例的落库在 vertical_test）。
func TestCommandForAcceptsBothCommands(t *testing.T) {
	if _, err := commandFor(commandExternalFundsFact, []byte(adoptInput)); err != nil {
		t.Fatalf("%s 译装被拒：%v", commandExternalFundsFact, err)
	}
	correction := []byte(`{
		"tenantId": "SYN-T1", "factRef": "SYN-FACT-1", "corrects": "SYN-FACT-1/v1",
		"version": "SYN-FACT-1/v2", "amountMinor": 9000, "correctedAt": "2026-09-02T08:00:00Z"
	}`)
	if _, err := commandFor(commandExternalFundsFactCorrection, correction); err != nil {
		t.Fatalf("%s 译装被拒：%v", commandExternalFundsFactCorrection, err)
	}
}

// TestCommandForAcceptsTheSellCustomerChargeRegisters 证 ADR-0190 两本册的命令接进了命令表，各自译装得过（落库在
// sell_customer_charge_registers_vertical_test）；词表外的时点在事务之前就拒。
func TestCommandForAcceptsTheSellCustomerChargeRegisters(t *testing.T) {
	adoption := []byte(`{"tenantId": "SYN-T1", "accountId": "SYN-ACCT-1", "billingMoment": "NETWORK_INTAKE_ADOPTED", "pricingScopeId": "SYN-SCOPE-SELL-1"}`)
	if _, err := commandFor(commandSellBillingMomentAdoption, adoption); err != nil {
		t.Fatalf("%s 译装被拒：%v", commandSellBillingMomentAdoption, err)
	}
	mapping := []byte(`{"tenantId": "SYN-T1", "pricingChargeCode": "BASE_FREIGHT", "feeItem": "SYN-FEE-FREIGHT", "basisRef": "SYN-CLAUSE-1"}`)
	if _, err := commandFor(commandFeeItemMapping, mapping); err != nil {
		t.Fatalf("%s 译装被拒：%v", commandFeeItemMapping, err)
	}
	if _, err := commandFor(commandSellBillingMomentAdoption, []byte(`{"tenantId": "SYN-T1", "accountId": "SYN-ACCT-1", "billingMoment": "COMMISSION_ACCEPTED", "pricingScopeId": "SYN-SCOPE-SELL-1"}`)); err == nil {
		t.Fatal("词表外的计费时点没在译装拒")
	}
	// 两本册的载荷形状不同，送错命令要拒，不能靠字段名巧合译进另一本。
	if _, err := commandFor(commandFeeItemMapping, adoption); err == nil {
		t.Fatal("采用载荷送进映射命令没拒")
	}
}
