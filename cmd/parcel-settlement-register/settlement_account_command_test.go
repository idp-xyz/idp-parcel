package main

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func TestTheAllocationFormCommandTranslatesBeforeTheTransaction(t *testing.T) {
	raw := []byte(`{
		"tenantId": "SYN-T1",
		"ruleVersion": "rule-1/v1",
		"form": "BY_WEIGHT"
	}`)
	dispatch, err := commandFor(commandAllocationForm, raw)
	if err != nil || dispatch == nil {
		t.Fatalf("allocation-form 译装失败：dispatch=%v err=%v", dispatch, err)
	}
	if _, err := commandFor(commandAllocationForm, []byte(`{"tenantId":"SYN-T1","ruleVersion":"rule-1/v1","form":"BY_WEIGHT","basis":1}`)); err == nil {
		t.Fatal("权重被收成分法登记")
	}
	if _, err := commandFor(commandAllocationForm, []byte(`{"tenantId":"SYN-T1","ruleVersion":"rule-1/v1","form":"EQUAL"}`)); err == nil {
		t.Fatal("均摊被收成一套分法")
	}
}

func TestTheBuyEvaluationTriggerCommandTranslatesBeforeTheTransaction(t *testing.T) {
	raw := []byte(`{
		"tenantId": "SYN-T1",
		"occurrenceReason": "BOOKING",
		"moment": "OCCURRENCE_FORMED"
	}`)
	dispatch, err := commandFor(commandBuyEvaluationTrigger, raw)
	if err != nil || dispatch == nil {
		t.Fatalf("buy-evaluation-trigger 译装失败：dispatch=%v err=%v", dispatch, err)
	}
	if _, err := commandFor(commandBuyEvaluationTrigger, []byte(`{"tenantId":"SYN-T1","occurrenceReason":"BOOKING","moment":"SETTLEMENT_PERIOD"}`)); err == nil {
		t.Fatal("结算周期被收成触发时点")
	}
	if _, err := commandFor(commandBuyEvaluationTrigger, []byte(`{"tenantId":"SYN-T1","occurrenceReason":"BOOKING","moment":"OCCURRENCE_FORMED","reasons":["CANCEL"]}`)); err == nil {
		t.Fatal("预列的发生项清单被收成触发登记")
	}
}

func TestTheAccountingConnectorCommandTranslatesBeforeTheTransaction(t *testing.T) {
	raw := []byte(`{
		"tenantId": "SYN-T1",
		"exchangeKind": "SUPPLIER_BILL",
		"counterparty": "SYN-SUPPLIER",
		"form": "CANONICAL"
	}`)
	dispatch, err := commandFor(commandAccountingConnector, raw)
	if err != nil || dispatch == nil {
		t.Fatalf("accounting-connector 译装失败：dispatch=%v err=%v", dispatch, err)
	}
	if _, err := commandFor(commandAccountingConnector, []byte(`{"tenantId":"SYN-T1","exchangeKind":"SUPPLIER_BILL","counterparty":"SYN-SUPPLIER","form":"SAP_IDOC"}`)); err == nil {
		t.Fatal("财务系统报文被收成连接器形态")
	}
	if _, err := commandFor(commandAccountingConnector, []byte(`{"tenantId":"SYN-T1","exchangeKind":"SUPPLIER_BILL","counterparty":"SYN-SUPPLIER","form":"CANONICAL","mapping":{"column":"A"}}`)); err == nil {
		t.Fatal("格式映射被收成连接器登记")
	}
}

func TestTheSettlementAccountCommandTranslatesBeforeTheTransaction(t *testing.T) {
	raw := []byte(`{
		"tenantId": "SYN-T1",
		"accountId": "ACCT-1",
		"legalEntityId": "LE-1",
		"counterpartyId": "CP-1",
		"direction": "RECEIVABLE",
		"currency": "XTS",
		"settlementPolicyId": "settle-1",
		"responsibilityBasis": "CONTRACT-1"
	}`)
	dispatch, err := commandFor(commandSettlementAccount, raw)
	if err != nil || dispatch == nil {
		t.Fatalf("settlement-account 译装失败：dispatch=%v err=%v", dispatch, err)
	}
	if _, err := commandFor(commandSettlementAccount, []byte(`{"reconciliationCycle":"MONTHLY"}`)); err == nil {
		t.Fatal("对账周期仍被收成结算账户命令")
	}
}

func TestAccountAnswerKeepsReplayRegisteredAndConflictApart(t *testing.T) {
	registered, code := accountAnswer(commandSettlementAccount, ports.SettlementAccountRegistered, "ACCT-1")
	if code != exitRegistered || !strings.Contains(registered, "ACCT-1") {
		t.Fatalf("registered = %q code=%d", registered, code)
	}
	replay, code := accountAnswer(commandSettlementAccount, ports.SettlementAccountReplay, "ACCT-1")
	if code != exitRegistered || !strings.Contains(replay, "已存在") {
		t.Fatalf("replay = %q code=%d", replay, code)
	}
	conflict, code := accountAnswer(commandSettlementAccount, ports.SettlementAccountConflict, "ACCT-1")
	if code != exitConflict || !strings.Contains(conflict, "冲突") {
		t.Fatalf("conflict = %q code=%d", conflict, code)
	}
}

func TestTheCatalogueCommandsTranslateBeforeTheTransaction(t *testing.T) {
	raw := []byte(`{
		"tenantId": "SYN-T1",
		"supplierId": "SUP-1",
		"legalEntityId": "LE-1",
		"auditorId": "AUDITOR-1"
	}`)
	dispatch, err := commandFor(commandSupplierAuditAuthority, raw)
	if err != nil || dispatch == nil {
		t.Fatalf("supplier-audit-authority 译装失败：%v", err)
	}
	if _, err := commandFor(commandSupplierPayableAccount, []byte(`{"accountId":"ACCT-1","extra":1}`)); err == nil {
		t.Fatal("未知字段仍被收成应付账户查问")
	}
	registered, code := catalogueAnswer(commandClaimAmountRule, ports.CatalogueRegistered, "rule-1/v1")
	if code != exitRegistered || !strings.Contains(registered, "rule-1/v1") {
		t.Fatalf("registered = %q code=%d", registered, code)
	}
	conflict, code := catalogueAnswer(commandClaimAmountRule, ports.CatalogueConflict, "rule-1/v1")
	if code != exitConflict || !strings.Contains(conflict, "冲突") {
		t.Fatalf("conflict = %q code=%d", conflict, code)
	}

	grammar := []byte(`{
		"tenantId": "SYN-T1",
		"subjectKind": "CLAIM_RULE",
		"subjectRef": "claim-rule/v2",
		"limitMinor": 5000,
		"ratioBasisPoints": 8000,
		"deductibleMinor": 1000
	}`)
	if _, err := commandFor(commandAmountGrammar, grammar); err != nil {
		t.Fatalf("amount-grammar 译装失败：%v", err)
	}
	if _, err := commandFor(commandAmountGrammar, []byte(`{"tenantId":"SYN-T1","subjectKind":"CLAIM_RULE","subjectRef":"claim-rule/v2","limitMinor":1,"ratioBasisPoints":10001,"deductibleMinor":0}`)); err == nil {
		t.Fatal("超出万分比 10000 的比例被收成文法登记")
	}
	ceiling := []byte(`{
		"tenantId": "SYN-T1",
		"supplierId": "SUP-1",
		"legalEntityId": "LE-1",
		"currency": "USD",
		"ceilingMinor": 12000
	}`)
	if _, err := commandFor(commandAuditEscalationCeiling, ceiling); err != nil {
		t.Fatalf("audit-escalation-ceiling 译装失败：%v", err)
	}
	if _, err := commandFor(commandAuditEscalationCeiling, []byte(`{"tenantId":"SYN-T1","supplierId":"SUP-1","legalEntityId":"LE-1","currency":"USD","ceilingMinor":-1}`)); err == nil {
		t.Fatal("负上限被收成越权升级登记")
	}
}
