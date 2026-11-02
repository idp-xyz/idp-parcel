package main

import (
	"testing"

	"go.idp.xyz/idp-parcel/cmd/internal/docnumbering"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func TestSettlementOrchestrationsStayUnconfiguredWhenTheMomentIsNotRegistered(t *testing.T) {
	db := evaluationRequestTestDB(t)
	numbers, err := docnumbering.Build(db, docnumber.SecretTable{})
	if err != nil {
		t.Fatal(err)
	}
	orchestrations, err := buildSettlementOrchestrations(db, numbers.Assigner)
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
	if _, err := buildSettlementOrchestrations(nil, nil); err == nil {
		t.Fatal("db 为 nil 却装配成功")
	}
	// 没装编号组件就装不起来：对账单、争议与审核应付都要在开立事务里取号，等到第一张单开立才发现就太晚了。
	if _, err := buildSettlementOrchestrations(evaluationRequestTestDB(t), nil); err == nil {
		t.Fatal("没有取号编排却装配成功")
	}
}
