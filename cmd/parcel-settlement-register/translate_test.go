package main

import (
	"testing"
)

// 本文件证命令表的封闭与译装的拒绝面接进了本口：集合外命令拒、每条命令都拒未知字段、形状坏当场拒——
// 一律在进事务之前。译装本身逐格的保真与拒绝在 `adapters/registrationjson` 的用例里证，这里不重证。

// TestCommandForRejectsUnknownCommand 证命令集封闭：CC 那本的核对命令与不存在的名字都拒。
func TestCommandForRejectsUnknownCommand(t *testing.T) {
	for _, command := range []string{"duty-payment-verification", "funds-mapping", ""} {
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
