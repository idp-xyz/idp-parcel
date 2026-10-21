package main

import (
	"context"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对真实 PostgreSQL 16 证运营续办口（ADR-0229 决定六，AT-SA-196；隔离合成 S）：一份停在「计价输入不可得」的 BUY
// 请求——没有哪一类参数登记解得开它——由运营点名那一版发生项、给一个续办引用续办：续办出第二代、回指第一代、带运营
// 依据，同笔交一封请求信封；同一个引用重跑是重放，不出第三代；换一个引用而第二代还没停下，同样什么都不推。点名与方向
// 对不上、没有续办引用，都在事务之前拒。

func evaluationResumeDocument(direction, occurrence, version, parcel, reference string) string {
	return `{"tenantId": "SYN-T1", "direction": "` + direction + `", "occurrence": "` + occurrence + `", "occurrenceVersion": "` +
		version + `", "parcel": "` + parcel + `", "resumeReference": "` + reference + `"}`
}

// Covers: AT-SA-196 运营续办口点名一版发生项续办一代，同一个引用重跑是重放，点名与方向对不上在事务之前拒。
func TestTheOperatorResumesAStalledBuyRequestOnceOnRealPostgres(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registrar, err := buildRegistrar(db)
	if err != nil {
		t.Fatalf("装配登记口：%v", err)
	}
	ctx := t.Context()
	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)

	occurrence, err := domain.NewTransportChargeOccurrence(
		mustDomain(t, domain.NewChargeOccurrenceID, "SYN-OCC-1"), mustDomain(t, domain.NewOccurrenceReasonReference, "ACTUAL_FULFILLMENT"),
		mustDomain(t, domain.NewOccurrenceVersion, "V1"), at)
	if err != nil {
		t.Fatal(err)
	}
	request, err := domain.SubmitEvaluationRequest(domain.EvaluationRequestSpec{
		ID:      mustDomain(t, domain.NewEvaluationRequestID, "EVREQ-SYN-1"),
		Scope:   mustDomain(t, domain.NewPrimaryScopeReference, "SYN-SCOPE-01"),
		Purpose: domain.BuySupplierCost,
		Sources: domain.EligibleSourceReferences{
			Occurrence: occurrence,
			FeeItem:    mustDomain(t, domain.NewFeeItemReference, "SYN-FEE-1"),
			Agreement:  mustDomain(t, domain.NewSupplierAgreementReference, "SYN-AGR/v1"),
		},
		RequestedAt: at,
		RequestedBy: mustDomain(t, domain.NewRequesterReference, "settlement-accounting/buy-evaluation-on-occurrence-formed"),
	})
	if err != nil {
		t.Fatal(err)
	}
	requests, err := sapostgres.NewEvaluationRequests(db)
	if err != nil {
		t.Fatal(err)
	}
	key := ports.EvaluationRequestKey{TenantID: mustDomain(t, domain.NewTenantID, "SYN-T1"), Request: request.ID()}
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := requests.Save(txCtx, ports.EvaluationRequestRecord{Key: key, Request: request, RecordedAt: at}); err != nil {
			return err
		}
		_, err := requests.RecordStop(txCtx, key, domain.StopPricingInputUnavailable, at)
		return err
	}); err != nil {
		t.Fatalf("落停下的第一代：%v", err)
	}

	mustExecute := func(raw string, wantCode int, wantAnswer string) {
		t.Helper()
		message, code := execute(ctx, commandEvaluationResume, []byte(raw), registrar)
		if code != wantCode || !strings.Contains(message, wantAnswer) {
			t.Fatalf("续办退出码 = %d 答复 = %q，要 %d 且含 %s", code, message, wantCode, wantAnswer)
		}
	}
	generations := func() int {
		return countRows(t, pool, `SELECT count(*) FROM settlement_accounting.evaluation_request WHERE tenant_id = 'SYN-T1' AND occurrence_id = 'SYN-OCC-1'`)
	}
	requestEnvelopes := func() int {
		return countRows(t, pool, `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = 'settlement-accounting.evaluation-request.submitted'`)
	}

	mustExecute(evaluationResumeDocument("BUY", "SYN-OCC-1", "V1", "SYN-PARCEL-1", "SYN-TICKET-1"), exitUsage, "译装被拒")
	mustExecute(evaluationResumeDocument("BUY", "SYN-OCC-1", "V1", "", ""), exitUsage, "译装被拒")
	mustExecute(evaluationResumeDocument("BUY", "SYN-OCC-1", "V1", "", "SYN-TICKET-1"), exitRegistered, "已续办 1 件")
	if generations() != 2 || requestEnvelopes() != 1 {
		t.Fatalf("续办之后 %d 代、%d 封请求信封，want 2 / 1", generations(), requestEnvelopes())
	}
	successor, found, err := requests.FindSuccessor(ctx, key)
	if err != nil || !found || successor.Request.Generation() != 2 || successor.Request.ResumeBasis().String() != "OPSRESUME/SYN-TICKET-1" ||
		successor.StopCause != domain.EvaluationStopCauseInvalid {
		t.Fatalf("第二代 = %+v found=%v err=%v", successor, found, err)
	}
	old, _, err := requests.FindByID(ctx, key)
	if err != nil || old.StopCause != domain.StopPricingInputUnavailable {
		t.Fatalf("第一代停因 = %s err=%v，want 原样", old.StopCause, err)
	}

	mustExecute(evaluationResumeDocument("BUY", "SYN-OCC-1", "V1", "", "SYN-TICKET-1"), exitRegistered, "没有在等的停办")
	mustExecute(evaluationResumeDocument("BUY", "SYN-OCC-1", "V1", "", "SYN-TICKET-2"), exitRegistered, "没有在等的停办")
	if generations() != 2 || requestEnvelopes() != 1 {
		t.Fatalf("重跑之后 %d 代、%d 封请求信封，want 仍是 2 / 1", generations(), requestEnvelopes())
	}
}

func mustDomain[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}
