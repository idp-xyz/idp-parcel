package main

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 证请求评价编排的组合根（票 sa-cc/08 判据 1「cmd/ 有非测试装配点」）：生产装配下四口全是
// 真适配器，一次请求走完「铸 ID → 登记 → 同事务经 Outbox 发信封」，重放答`已存在`交回原 ID 且信封不翻倍。测试
// 输入全是隔离合成串，只记 `S`。

func evaluationRequestTestDB(t *testing.T) *bentopg.DB {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return db
}

func evaluationRequestTestCommand(t *testing.T, tenant string) saapplication.RequestBuyEvaluationCommand {
	t.Helper()
	occurrence, err := sadomain.NewTransportChargeOccurrence(
		mustValue(t, sadomain.NewChargeOccurrenceID, "SYN-OCC-SACC08-1"),
		mustValue(t, sadomain.NewOccurrenceReasonReference, "BOOKING"),
		mustValue(t, sadomain.NewOccurrenceVersion, "v1"),
		time.Date(2026, 9, 11, 8, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("发生项引用：%v", err)
	}
	return saapplication.RequestBuyEvaluationCommand{
		TenantID:    mustValue(t, sadomain.NewTenantID, tenant),
		Scope:       mustValue(t, sadomain.NewPrimaryScopeReference, "SYN-SCOPE-SACC08"),
		Occurrence:  occurrence,
		FeeItem:     mustValue(t, sadomain.NewFeeItemReference, "SYN-FEE-SACC08"),
		Agreement:   mustValue(t, sadomain.NewSupplierAgreementReference, "SYN-AGREEMENT-SACC08"),
		RequestedBy: mustValue(t, sadomain.NewRequesterReference, "SYN-SETTLEMENT-JOB"),
	}
}

// Covers: 判据 1「cmd/ 有非测试装配点」与判据 2 在真装配下的形：请求 → 登记册一行 + Outbox 一封；重放 →`已存在`
// 交回原 ID、信封仍一封。
func TestTheProductionEvaluationRequestOrchestrationRegistersAndHandsOffOnce(t *testing.T) {
	db := evaluationRequestTestDB(t)
	orchestration, err := buildEvaluationRequestOrchestration(db)
	if err != nil {
		t.Fatalf("装配请求评价编排：%v", err)
	}
	command := evaluationRequestTestCommand(t, "SYN-TENANT-SACC08")

	requested, err := orchestration.Request(t.Context(), command)
	if err != nil {
		t.Fatalf("请求：%v", err)
	}
	if requested.Outcome() != saapplication.EvaluationRequested {
		t.Fatalf("outcome = %q, want EVALUATION_REQUESTED", requested.Outcome())
	}
	record, ok := requested.Request()
	if !ok {
		t.Fatal("已请求却没交回登记记录")
	}
	if requested.EvaluationRequestHandoffReference() != "" {
		t.Fatalf("真装配下交接不该失败，续办引用 = %q", requested.EvaluationRequestHandoffReference())
	}

	registry, err := sapostgres.NewEvaluationRequests(db)
	if err != nil {
		t.Fatalf("构造登记册读口：%v", err)
	}
	stored, found, err := registry.FindByID(t.Context(), record.Key)
	if err != nil || !found {
		t.Fatalf("登记册按 ID 读回：found=%v err=%v", found, err)
	}
	if stored.Request.Sources().FeeItem != command.FeeItem {
		t.Fatalf("登记下的费用项目 = %q", stored.Request.Sources().FeeItem.String())
	}
	eventID := command.TenantID.String() + "/evaluation-request/" + record.Key.Request.String() + "/submitted"
	if count := outboxRowsFor(t, db, eventID); count != 1 {
		t.Fatalf("信封行数 = %d, want 1", count)
	}

	replay, err := orchestration.Request(t.Context(), command)
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if replay.Outcome() != saapplication.EvaluationRequestExisting {
		t.Fatalf("重放 outcome = %q, want EXISTING_EVALUATION_REQUEST", replay.Outcome())
	}
	existing, _ := replay.Request()
	if existing.Key.Request != record.Key.Request {
		t.Fatalf("重放交回的 ID = %q, want 原 ID %q", existing.Key.Request.String(), record.Key.Request.String())
	}
	if count := outboxRowsFor(t, db, eventID); count != 1 {
		t.Fatalf("重放后信封行数 = %d, want 1——重发的必须是同一份", count)
	}
}

func TestTheEvaluationRequestOrchestrationRefusesANilDB(t *testing.T) {
	if _, err := buildEvaluationRequestOrchestration(nil); err == nil {
		t.Fatal("db 为 nil 却装配成功")
	}
}

func outboxRowsFor(t *testing.T, db *bentopg.DB, eventID string) int {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var count int
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`, eventID,
	).Scan(&count); err != nil {
		t.Fatalf("统计 outbox：%v", err)
	}
	return count
}
