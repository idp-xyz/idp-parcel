package main

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

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
