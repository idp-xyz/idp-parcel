package main

import (
	"testing"

	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func TestSettlementOrchestrationsStayUnconfiguredWhenTheMomentIsNotRegistered(t *testing.T) {
	db := evaluationRequestTestDB(t)
	orchestrations, err := buildSettlementOrchestrations(db)
	if err != nil {
		t.Fatalf("装配结算编排：%v", err)
	}
	if orchestrations.confirm == nil || orchestrations.cutoff == nil || orchestrations.adjust == nil ||
		orchestrations.allocate == nil || orchestrations.supplier == nil || orchestrations.claims == nil {
		t.Fatal("七个入口里有构造结果是空的")
	}
	tenant, err := sadomain.NewTenantID("SYN-TENANT-MOMENT")
	if err != nil {
		t.Fatal(err)
	}
	outcome, _, err := orchestrations.Confirm(t.Context(), saapplication.ConfirmChargeCommand{TenantID: tenant, ChargeID: "SYN-CHARGE"})
	if err != nil {
		t.Fatalf("确认触发：%v", err)
	}
	if outcome != saapplication.SettlementMomentUnconfigured {
		t.Fatalf("确认 outcome = %s", outcome)
	}
	outcome, _, err = orchestrations.CutOff(t.Context(), saapplication.PublishStatementCommand{TenantID: tenant, Period: "SYN-PERIOD"})
	if err != nil {
		t.Fatalf("截单触发：%v", err)
	}
	if outcome != saapplication.SettlementMomentUnconfigured {
		t.Fatalf("截单 outcome = %s", outcome)
	}
}

func TestSettlementOrchestrationsRefuseANilDB(t *testing.T) {
	if _, err := buildSettlementOrchestrations(nil); err == nil {
		t.Fatal("db 为 nil 却装配成功")
	}
}
