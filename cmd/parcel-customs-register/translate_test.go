package main

import (
	"strings"
	"testing"
)

// 本文件证译装的拒绝面：形状坏、词表外、必填缺——一律在入库前拒并指名差哪格，
// 用例一步不走。绿路径的字段保真在 main_test 经 execute 对替身册断言。

// TestCommandForRejectsUnknownCommand 证命令集封闭：集合外的命令拒收并列出全部十个。
func TestCommandForRejectsUnknownCommand(t *testing.T) {
	if _, err := commandFor("case-closure", []byte(`{}`)); err == nil {
		t.Fatalf("集合外命令要拒（案件关闭是判断链动作，不是配置登记）")
	}
}

// TestCommandForRejectsUnknownFields 证十二个命令全部拒未知字段：打错的键静默丢弃
// 会让操作员以为登进去的比实际多。
func TestCommandForRejectsUnknownFields(t *testing.T) {
	for _, command := range allCommands {
		if _, err := commandFor(command, []byte(`{"typo": 1}`)); err == nil {
			t.Fatalf("%s 未拒未知字段", command)
		}
	}
}

// TestCommandForRejectsBlankIdentifiers 证有构造门的标识在译装处就拒：空白租户、
// 空白单元不该走到受理门才发现。
func TestCommandForRejectsBlankIdentifiers(t *testing.T) {
	cases := map[string]string{
		commandReadinessRegister: `{
			"tenantId": "  ", "unitId": "unit-1",
			"basisRef": "basis-1", "judgedAt": "2026-08-24T01:00:00Z"
		}`,
		commandAuthorityGrant: `{
			"tenantId": "SYN-T1", "unitId": "",
			"authorityRef": "authority-1", "grantedAt": "2026-08-24T01:00:00Z"
		}`,
		commandInterpretationRule: `{
			"tenantId": "SYN-T1", "resultLayer": "RELEASE_RESULT",
			"jurisdictionRef": "SYN-JURIS-DE", "ruleRef": " ",
			"appliesFrom": "2026-08-24T01:00:00Z"
		}`,
		commandGateFinding: `{
			"tenantId": "SYN-T1", "scopeRef": "scope-1", "action": "FINAL_DELIVERY",
			"boundaryRef": "boundary-1", "preconditionRef": "", "state": "MET"
		}`,
		commandCandidatePort: `{
			"tenantId": "SYN-T1", "portRef": "  ",
			"appliesFrom": "2026-08-24T01:00:00Z"
		}`,
		commandDeclarationPath: `{
			"tenantId": "SYN-T1", "pathRef": "SYN-PATH-01", "portRef": "SYN-PORT-01",
			"direction": "IMPORT", "declarationMode": " ",
			"appliesFrom": "2026-08-24T01:00:00Z"
		}`,
	}
	for command, raw := range cases {
		if _, err := commandFor(command, []byte(raw)); err == nil {
			t.Fatalf("%s 未拒空白标识", command)
		}
	}
}

// TestCommandForRejectsVocabularyOutsideTheClosedSets 证枚举词表封闭：层六值、动作
// 四值、义务态三值、前置态三值，词表外的取值在译装处指名拒绝。
func TestCommandForRejectsVocabularyOutsideTheClosedSets(t *testing.T) {
	cases := []struct {
		command string
		raw     string
		want    string
	}{
		{commandInterpretationRule, `{
			"tenantId": "SYN-T1", "resultLayer": "CLEARANCE_DONE", "ruleRef": "rule-1"
		}`, "resultLayer"},
		{commandGateCatalog, `{
			"tenantId": "SYN-T1", "scopeRef": "scope-1", "action": "RECEIVE",
			"boundaryRef": "boundary-1", "registeredAt": "2026-08-24T01:00:00Z"
		}`, "action"},
		{commandObligationItem, `{
			"tenantId": "SYN-T1", "caseRef": "case-1", "obligation": "duty-1",
			"scope": "scope-1", "state": "DONE", "basis": "basis-1",
			"appliesFrom": "2026-08-24T01:00:00Z"
		}`, "state"},
		{commandGateFinding, `{
			"tenantId": "SYN-T1", "scopeRef": "scope-1", "action": "FINAL_DELIVERY",
			"boundaryRef": "boundary-1", "preconditionRef": "pre-1", "state": "UNKNOWN"
		}`, "state"},
		{commandCaseRequirement, `{
			"tenantId": "SYN-T1", "jurisdictionRef": "JURIS/DE", "direction": "TRANSIT",
			"procedureRef": "PROC/EXPORT-STANDARD", "required": true, "basis": "basis-1"
		}`, "direction"},
		{commandDeclarationPath, `{
			"tenantId": "SYN-T1", "pathRef": "SYN-PATH-01", "portRef": "SYN-PORT-01",
			"direction": "TRANSIT", "declarationMode": "SYN-MODE-GENERAL",
			"appliesFrom": "2026-08-24T01:00:00Z"
		}`, "direction"},
	}
	for _, spec := range cases {
		_, err := commandFor(spec.command, []byte(spec.raw))
		if err == nil {
			t.Fatalf("%s 未拒词表外取值", spec.command)
		}
		if !strings.Contains(err.Error(), spec.want) {
			t.Fatalf("%s 的拒绝没指名 %s：%v", spec.command, spec.want, err)
		}
	}
}

// TestCommandForRejectsHandedToMismatch 证承接配对在译装处双向守住：承接项必须指名
// 接收责任方，非承接项带承接对象会被适配器静默丢弃——静默丢弃就是要拒的理由。
func TestCommandForRejectsHandedToMismatch(t *testing.T) {
	handedOverWithoutReceiver := `{
		"tenantId": "SYN-T1", "caseRef": "case-1", "obligation": "duty-1",
		"scope": "scope-1", "state": "HANDED_OVER", "basis": "basis-1",
		"appliesFrom": "2026-08-24T01:00:00Z"
	}`
	if _, err := commandFor(commandObligationItem, []byte(handedOverWithoutReceiver)); err == nil {
		t.Fatalf("承接项缺接收责任方要拒（CONTEXT 硬句 219）")
	}
	concludedWithReceiver := `{
		"tenantId": "SYN-T1", "caseRef": "case-1", "obligation": "duty-1",
		"scope": "scope-1", "state": "CONCLUDED", "basis": "basis-1",
		"handedTo": "someone", "appliesFrom": "2026-08-24T01:00:00Z"
	}`
	if _, err := commandFor(commandObligationItem, []byte(concludedWithReceiver)); err == nil {
		t.Fatalf("非承接项带承接对象要拒，不得静默丢弃")
	}
}

// TestCommandForRejectsAbsentRegistrationInstant 证目录登记时刻必填：两本目录的
// registeredAt 在领域与库上都没有零值门（timestamptz 装得下 0001 年），缺格只能在
// 译装处拦，否则「没给」会静默变成一个错的事实。
func TestCommandForRejectsAbsentRegistrationInstant(t *testing.T) {
	cases := map[string]string{
		commandObligationCatalog: `{"tenantId": "SYN-T1", "caseRef": "case-1"}`,
		commandGateCatalog: `{
			"tenantId": "SYN-T1", "scopeRef": "scope-1",
			"action": "OUTBOUND_RELEASE", "boundaryRef": "boundary-1"
		}`,
	}
	for command, raw := range cases {
		_, err := commandFor(command, []byte(raw))
		if err == nil {
			t.Fatalf("%s 未拒缺席的 registeredAt", command)
		}
		if !strings.Contains(err.Error(), "registeredAt") {
			t.Fatalf("%s 的拒绝没指名 registeredAt：%v", command, err)
		}
	}
}

// TestCommandForRejectsAbsentAppliesFrom 证版本键上的生效起点必填：解释规则与口岸/
// 路径两目录同款——起点在键上且领域与库都没有零值门（timestamptz 装得下 0001 年），
// 缺格只能在译装处拦——静默落成 0001 年的版本边界正是「缺格变成错事实」。
func TestCommandForRejectsAbsentAppliesFrom(t *testing.T) {
	cases := map[string]string{
		commandInterpretationRule: `{
			"tenantId": "SYN-T1", "resultLayer": "RELEASE_RESULT",
			"jurisdictionRef": "SYN-JURIS-DE", "ruleRef": "SYN-RULE-1"
		}`,
		commandCandidatePort: `{"tenantId": "SYN-T1", "portRef": "SYN-PORT-01"}`,
		commandDeclarationPath: `{
			"tenantId": "SYN-T1", "pathRef": "SYN-PATH-01", "portRef": "SYN-PORT-01",
			"direction": "IMPORT", "declarationMode": "SYN-MODE-GENERAL"
		}`,
	}
	for command, raw := range cases {
		_, err := commandFor(command, []byte(raw))
		if err == nil {
			t.Fatalf("%s 未拒缺席的 appliesFrom——生效起点没有默认值", command)
		}
		if !strings.Contains(err.Error(), "appliesFrom") {
			t.Fatalf("%s 的拒绝没指名 appliesFrom：%v", command, err)
		}
	}
}

// TestCommandForRejectsBlankObligationContent 证义务项的内容格逐格必填：obligation/
// scope/basis 空白在译装处拒（库的 not_blank CHECK 会拦，但那时答案已滑到未决 3，
// 缺格是用法错误该答 1）。
func TestCommandForRejectsBlankObligationContent(t *testing.T) {
	blankScope := `{
		"tenantId": "SYN-T1", "caseRef": "case-1", "obligation": "duty-1",
		"scope": " ", "state": "CONCLUDED", "basis": "basis-1",
		"appliesFrom": "2026-08-24T01:00:00Z"
	}`
	if _, err := commandFor(commandObligationItem, []byte(blankScope)); err == nil {
		t.Fatalf("空白 scope 要在译装处拒")
	}
}

// TestCommandForRejectsAbsentRequiredFlag 证建案要求的判断格必须显式给：required 缺席
// 时 Go 的零值是 false，静默落成「不要求建案」正是「缺格变成错事实」那类洞，译装以
// 指针分辨「没给」与「给了 false」并拒前者。
func TestCommandForRejectsAbsentRequiredFlag(t *testing.T) {
	absent := `{
		"tenantId": "SYN-T1", "jurisdictionRef": "JURIS/DE", "direction": "EXPORT",
		"procedureRef": "PROC/EXPORT-STANDARD", "basis": "CONTRACT/NO-CASE-V1"
	}`
	_, err := commandFor(commandCaseRequirement, []byte(absent))
	if err == nil {
		t.Fatalf("缺席的 required 要拒，不得静默变成「不要求」")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Fatalf("拒绝没指名 required：%v", err)
	}
}
