package main

import (
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

// 本文件证翻译层：JSON 逐字段折进领域规格、未知字段拒收、标识构造门在翻译处就拒。
// 内容合法性（九件缺一、恢复四件）不在这里重证——那是领域门的地盘，登记口只保证
// 「给了什么就递什么，没给的绝不代填」。

func TestSuspensionSpecTranslatesAllNineFields(t *testing.T) {
	raw := []byte(`{
		"suspensionId": "suspension-9",
		"triggerSource": "PILOT-RULE/hard-risk-3",
		"basis": "cross-customer isolation risk",
		"evidence": "evidence-pack/incident-9",
		"scope": "pilot-scope/v2",
		"executedBy": "declared-duty-officer",
		"occurredAt": "2026-08-24T01:00:00Z",
		"effectiveAt": "2026-08-24T01:05:00Z",
		"inTransitNote": "in-transit objects continue under domain owners"
	}`)

	spec, err := suspensionSpecFromJSON(raw)
	if err != nil {
		t.Fatalf("翻译暂停输入：%v", err)
	}
	decision, err := domain.RecordSuspension(spec)
	if err != nil {
		t.Fatalf("翻译产物过不了领域门：%v", err)
	}
	if decision.ID().String() != "suspension-9" {
		t.Fatalf("暂停标识 = %q", decision.ID().String())
	}
	if decision.TriggerSource() != "PILOT-RULE/hard-risk-3" ||
		decision.Basis() != "cross-customer isolation risk" ||
		decision.Evidence() != "evidence-pack/incident-9" {
		t.Fatalf("触发来源/依据/证据没有原样递达")
	}
	if decision.Scope().String() != "pilot-scope/v2" {
		t.Fatalf("范围 = %q", decision.Scope().String())
	}
	if decision.ExecutedBy() != "declared-duty-officer" {
		t.Fatalf("执行身份（第②轨） = %q", decision.ExecutedBy())
	}
	if !decision.OccurredAt().Equal(time.Date(2026, 8, 24, 1, 0, 0, 0, time.UTC)) ||
		!decision.EffectiveAt().Equal(time.Date(2026, 8, 24, 1, 5, 0, 0, time.UTC)) {
		t.Fatalf("发生/生效时间没有原样递达")
	}
	if decision.InTransitNote() != "in-transit objects continue under domain owners" {
		t.Fatalf("在途处置说明 = %q", decision.InTransitNote())
	}
}

func TestResumptionSpecCarriesAllFourResumptionItems(t *testing.T) {
	raw := []byte(`{
		"suspensionId": "suspension-9",
		"releaseEvidence": "evidence-pack/release-9",
		"consistencyCheck": "consistency-report/r9",
		"inventory": {
			"takenAt": "2026-08-24T02:00:00Z",
			"entries": [{
				"objectIdentity": "parcel-object/p1",
				"currentFacts": "accepted-fact/p1",
				"currentAuthority": "parcel-product",
				"responsibleParty": "ops-owner-1",
				"nextAction": "resume-normal-processing",
				"reviewBy": "2026-08-25T02:00:00Z"
			}]
		},
		"decidedBy": "pilot-business-owner",
		"decidedAt": "2026-08-24T03:00:00Z",
		"effectiveAt": "2026-08-24T03:05:00Z"
	}`)

	spec, err := resumptionSpecFromJSON(raw)
	if err != nil {
		t.Fatalf("翻译恢复输入：%v", err)
	}
	decision, err := domain.RecordResumption(spec)
	if err != nil {
		t.Fatalf("翻译产物过不了领域门：%v", err)
	}
	if decision.Suspension().String() != "suspension-9" {
		t.Fatalf("被恢复的暂停 = %q", decision.Suspension().String())
	}
	if decision.ReleaseEvidence() != "evidence-pack/release-9" ||
		decision.ConsistencyCheck() != "consistency-report/r9" {
		t.Fatalf("解除证据/一致性核对没有原样递达")
	}
	entries := decision.Inventory().Entries()
	if len(entries) != 1 || entries[0].ObjectIdentity != "parcel-object/p1" ||
		entries[0].NextAction != "resume-normal-processing" {
		t.Fatalf("在途盘点没有原样递达：%+v", entries)
	}
	if !decision.Inventory().TakenAt().Equal(time.Date(2026, 8, 24, 2, 0, 0, 0, time.UTC)) {
		t.Fatalf("盘点时刻 = %s", decision.Inventory().TakenAt())
	}
	if decision.DecidedBy() != "pilot-business-owner" {
		t.Fatalf("决定人（第②轨） = %q", decision.DecidedBy())
	}
}

func TestAuthorityIntervalTranslatesOpenAndClosedIntervals(t *testing.T) {
	open := []byte(`{
		"objectScope": "pilot-members/v1",
		"capability": "shipment-intake",
		"factKind": "production-ownership",
		"authority": "parcel-product",
		"fromAt": "2026-08-24T00:00:00Z"
	}`)
	interval, err := authorityIntervalFromJSON(open)
	if err != nil {
		t.Fatalf("翻译开放区间：%v", err)
	}
	if !interval.To.IsZero() {
		t.Fatalf("开放区间的 To 应为零值，得 %s", interval.To)
	}
	if interval.ObjectScope != "pilot-members/v1" || interval.Authority != "parcel-product" {
		t.Fatalf("区间维度没有原样递达：%+v", interval)
	}

	closed := []byte(`{
		"objectScope": "pilot-members/v1",
		"capability": "shipment-intake",
		"factKind": "production-ownership",
		"authority": "legacy-system",
		"fromAt": "2026-08-01T00:00:00Z",
		"toAt": "2026-08-24T00:00:00Z"
	}`)
	interval, err = authorityIntervalFromJSON(closed)
	if err != nil {
		t.Fatalf("翻译关闭区间：%v", err)
	}
	if !interval.To.Equal(time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("关闭区间的 To = %s", interval.To)
	}
}

// TestTranslateRejectsUnknownFields 证打错字段名不得静默变成「没给」——三种输入
// 一致拒收未知字段。
func TestTranslateRejectsUnknownFields(t *testing.T) {
	cases := map[string]func([]byte) error{
		"authority-interval": func(raw []byte) error {
			_, err := authorityIntervalFromJSON(raw)
			return err
		},
		"suspend": func(raw []byte) error {
			_, err := suspensionSpecFromJSON(raw)
			return err
		},
		"resume": func(raw []byte) error {
			_, err := resumptionSpecFromJSON(raw)
			return err
		},
	}
	for name, translate := range cases {
		if err := translate([]byte(`{"unexpectedField": "x"}`)); err == nil {
			t.Fatalf("%s：未知字段被静默吞掉了", name)
		}
	}
}

// TestTranslateRejectsBlankIdentifiers 证标识构造门在翻译处就拒：空暂停标识、
// 六件不齐的盘点条目都到不了用例。
func TestTranslateRejectsBlankIdentifiers(t *testing.T) {
	_, err := suspensionSpecFromJSON([]byte(`{
		"suspensionId": "  ",
		"triggerSource": "t", "basis": "b", "evidence": "e", "scope": "s",
		"executedBy": "x",
		"occurredAt": "2026-08-24T01:00:00Z", "effectiveAt": "2026-08-24T01:00:00Z",
		"inTransitNote": "n"
	}`))
	if err == nil {
		t.Fatalf("空暂停标识被接受了")
	}

	_, err = resumptionSpecFromJSON([]byte(`{
		"suspensionId": "suspension-9",
		"releaseEvidence": "r", "consistencyCheck": "c",
		"inventory": {
			"takenAt": "2026-08-24T02:00:00Z",
			"entries": [{"objectIdentity": "p1"}]
		},
		"decidedBy": "d",
		"decidedAt": "2026-08-24T03:00:00Z", "effectiveAt": "2026-08-24T03:00:00Z"
	}`))
	if err == nil || !strings.Contains(err.Error(), "invalid inventory") {
		t.Fatalf("六件不齐的盘点条目应被 TakeInventory 拒绝，err = %v", err)
	}
}
