package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 证登记争议口与处理争议口的生产装配（ADR-0353 决定一至四；票 home-today-todo/03）：两口按特性登记处那一份
// build 与 mount 装上，进的是 buildSettlementOrchestrations 装的截单编排上的开立与处理。对账单经确认口与发布口真发一张，不直写库。
// 证据只记 `S`。

const (
	disputeClerk   = "SYN-DISPUTE-CLERK"
	disputeFinance = "SYN-DISPUTE-FINANCE"

	statementDisputeFace           = "/settlement-statement-disputes"
	statementDisputeResolutionFace = "/settlement-statement-dispute-resolutions"
	disputedStatement              = "SYN-STMT-DISPUTE-1"
)

// newPublishedStatementFixture 发一张只有一笔 9600 费用的对账单，再给客服与财务各登一种争议授予。
func newPublishedStatementFixture(t *testing.T) (*settlementDecisionsFixture, time.Time) {
	t.Helper()
	fixture := newSettlementDecisionsFixture(t)
	operators, err := aipg.NewOperatorRegistry(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	registerOperatorIn(t, fixture.db, operators, decisionsTenant, disputeClerk, interval, accessidentity.DecisionStatementDisputeRegistration)
	registerOperatorIn(t, fixture.db, operators, decisionsTenant, disputeFinance, interval, accessidentity.DecisionStatementDisputeResolution)
	for _, pattern := range []string{statementDisputeFace, statementDisputeResolutionFace} {
		if fixture.endpoints[pattern] == nil {
			t.Fatalf("%s 没按登记处挂上", pattern)
		}
	}

	fixture.adoptMoment(t, sadomain.SettlementMomentConfirm)
	fixture.adoptMoment(t, sadomain.SettlementMomentCutOff)
	if status, answer := fixture.post(t, chargeConfirmationFace, decisionsClerk, `{"charge":"`+decisionsCharge+`"}`); status != http.StatusCreated {
		t.Fatalf("先确认 = %d %v", status, answer)
	}
	last := time.Now().In(time.FixedZone("CST", 8*3600)).AddDate(0, 0, 1).Format("2006-01-02")
	status, answer := fixture.post(t, statementPublicationFace, decisionsPublisher,
		`{"number":"`+disputedStatement+`","account":"`+decisionsAccount+`","period":"SYN-PERIOD-2026-11","version":"v1",`+
			`"currency":"CNY","charges":["`+decisionsCharge+`"],"lastAttributionDate":"`+last+`","declaredTotal":"9600",`+
			`"cutOffAt":"`+time.Now().UTC().Format(time.RFC3339)+`"}`)
	if status != http.StatusCreated {
		t.Fatalf("发布 = %d %v", status, answer)
	}
	publishedAt, err := time.Parse(time.RFC3339Nano, fixture.text(t,
		`SELECT to_char(published_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM settlement_accounting.customer_statement
		  WHERE tenant_id = $1 AND statement_number = $2`, decisionsTenant, disputedStatement))
	if err != nil {
		t.Fatal(err)
	}
	return fixture, publishedAt
}

func disputeBodyOf(id, amount string, openedAt time.Time) string {
	return `{"dispute":"` + id + `","statement":"` + disputedStatement + `","charge":"` + decisionsCharge + `","disputedAmount":"` + amount +
		`","reason":"客户来电：重量按 1.2 kg 计","openedAt":"` + openedAt.UTC().Format(time.RFC3339) + `"}`
}

// Covers: AT-SA-228（ADR-0353 决定一、三、四）——持登记授予的客服代登记一项争议：答 201；同一份重放答已存在，同标识换了金额答冲突；金额
// 越行、单号不在册、提出早于发布答未受理；行上记下登记人；持处理授予的财务打登记口答未授予；单作废之后新争议答作废那一格，已开的那项
// 照旧在。
func TestTheDisputeRegistrationFaceRegistersAndRecordsTheClerk(t *testing.T) {
	fixture, publishedAt := newPublishedStatementFixture(t)
	openedAt := publishedAt.Add(time.Minute).Truncate(time.Second).Add(time.Second)
	body := disputeBodyOf("SYN-DSP-1", "1600", openedAt)

	if status, answer := fixture.post(t, statementDisputeFace, disputeFinance, body); status != http.StatusForbidden {
		t.Fatalf("持处理授予的人登记 = %d %v，want 403", status, answer)
	}
	status, answer := fixture.post(t, statementDisputeFace, disputeClerk, body)
	if status != http.StatusCreated || answer["outcome"] != "DISPUTE_OPENED" || answer["disputedAmount"] != "1600" ||
		answer["statement"] != disputedStatement || answer["resolution"] != nil {
		t.Fatalf("登记 = %d %v", status, answer)
	}
	if status, answer := fixture.post(t, statementDisputeFace, disputeClerk, body); status != http.StatusOK || answer["outcome"] != "EXISTING_DISPUTE" {
		t.Fatalf("重放 = %d %v", status, answer)
	}
	if status, answer := fixture.post(t, statementDisputeFace, disputeClerk, disputeBodyOf("SYN-DSP-1", "1700", openedAt)); status != http.StatusOK ||
		answer["outcome"] != "DISPUTE_CONFLICT" {
		t.Fatalf("同标识换金额 = %d %v", status, answer)
	}
	for name, raw := range map[string]string{
		"金额越行":   disputeBodyOf("SYN-DSP-2", "9601", openedAt),
		"单号不在册":  strings.Replace(disputeBodyOf("SYN-DSP-3", "100", openedAt), disputedStatement, "SYN-STMT-NONE", 1),
		"提出早于发布": disputeBodyOf("SYN-DSP-4", "100", publishedAt.Add(-time.Hour)),
	} {
		if status, answer := fixture.post(t, statementDisputeFace, disputeClerk, raw); status != http.StatusOK || answer["outcome"] != "SOURCE_NOT_ACCEPTED" {
			t.Fatalf("%s = %d %v", name, status, answer)
		}
	}
	if evidence := fixture.text(t, `SELECT operation_evidence FROM settlement_accounting.statement_dispute
		 WHERE tenant_id = $1 AND dispute_id = 'SYN-DSP-1'`, decisionsTenant); evidence != operatorEvidenceOf(disputeClerk) {
		t.Fatalf("登记人 = %q", evidence)
	}

	// 作废没有生产口，直写作废两列（同作废编排落的那两格）。
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		executor, err := fixture.db.RequireExecutor(ctx)
		if err != nil {
			return err
		}
		_, err = executor.Exec(ctx, `UPDATE settlement_accounting.customer_statement
		    SET void_basis = 'SYN-VOID-1', voided_at = $3 WHERE tenant_id = $1 AND statement_number = $2`,
			decisionsTenant, disputedStatement, publishedAt.Add(2*time.Minute))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if status, answer := fixture.post(t, statementDisputeFace, disputeClerk, disputeBodyOf("SYN-DSP-5", "100", openedAt.Add(5*time.Minute))); status != http.StatusOK ||
		answer["outcome"] != "STATEMENT_VOIDED_TAKES_NO_DISPUTE" {
		t.Fatalf("单已作废 = %d %v", status, answer)
	}
	if count := fixture.text(t, `SELECT count(*)::text FROM settlement_accounting.statement_dispute WHERE tenant_id = $1`, decisionsTenant); count != "1" {
		t.Fatalf("库上争议 %s 项，want 只有作废前那一项", count)
	}
}

// Covers: AT-SA-229（ADR-0353 决定二、四）——持处理授予的财务先处理成待复核（201），再处理成拒绝（201），之后再处理答已处理并交回拒绝那
// 一份（200）；行上记下最后那一次处理人，登记人不变；持登记授予的客服打处理口答未授予；不在册的争议答未受理。
func TestTheDisputeResolutionFaceResolvesOnceFinalAndRecordsTheDecider(t *testing.T) {
	fixture, publishedAt := newPublishedStatementFixture(t)
	openedAt := publishedAt.Add(time.Minute).Truncate(time.Second).Add(time.Second)
	if status, answer := fixture.post(t, statementDisputeFace, disputeClerk, disputeBodyOf("SYN-DSP-1", "1600", openedAt)); status != http.StatusCreated {
		t.Fatalf("先登记 = %d %v", status, answer)
	}
	resolution := func(kind string, at time.Time) string {
		return `{"dispute":"SYN-DSP-1","resolution":"` + kind + `","basis":"核过磅单","resolvedAt":"` + at.UTC().Format(time.RFC3339) + `"}`
	}

	if status, answer := fixture.post(t, statementDisputeResolutionFace, disputeClerk, resolution("REJECTED", openedAt.Add(time.Hour))); status != http.StatusForbidden {
		t.Fatalf("持登记授予的人处理 = %d %v，want 403", status, answer)
	}
	if status, answer := fixture.post(t, statementDisputeResolutionFace, disputeFinance, resolution("PENDING_REVIEW", openedAt.Add(time.Hour))); status != http.StatusCreated ||
		answer["outcome"] != "DISPUTE_RESOLVED" || answer["resolution"] != "PENDING_REVIEW" {
		t.Fatalf("待复核 = %d %v", status, answer)
	}
	if status, answer := fixture.post(t, statementDisputeResolutionFace, disputeFinance, resolution("REJECTED", openedAt.Add(2*time.Hour))); status != http.StatusCreated ||
		answer["resolution"] != "REJECTED" {
		t.Fatalf("拒绝 = %d %v", status, answer)
	}
	if status, answer := fixture.post(t, statementDisputeResolutionFace, disputeFinance, resolution("ACCEPTED", openedAt.Add(3*time.Hour))); status != http.StatusOK ||
		answer["outcome"] != "DISPUTE_ALREADY_RESOLVED" || answer["resolution"] != "REJECTED" {
		t.Fatalf("终局后再处理 = %d %v", status, answer)
	}
	if status, answer := fixture.post(t, statementDisputeResolutionFace, disputeFinance,
		strings.Replace(resolution("REJECTED", openedAt.Add(time.Hour)), "SYN-DSP-1", "SYN-DSP-NONE", 1)); status != http.StatusOK ||
		answer["outcome"] != "SOURCE_NOT_ACCEPTED" {
		t.Fatalf("不在册的争议 = %d %v", status, answer)
	}
	if decider := fixture.text(t, `SELECT resolution_evidence FROM settlement_accounting.statement_dispute
		 WHERE tenant_id = $1 AND dispute_id = 'SYN-DSP-1'`, decisionsTenant); decider != operatorEvidenceOf(disputeFinance) {
		t.Fatalf("处理人 = %q", decider)
	}
	if clerk := fixture.text(t, `SELECT operation_evidence FROM settlement_accounting.statement_dispute
		 WHERE tenant_id = $1 AND dispute_id = 'SYN-DSP-1'`, decisionsTenant); clerk != operatorEvidenceOf(disputeClerk) {
		t.Fatalf("登记人被处理改写成 %q", clerk)
	}
}
