package main

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 证登记争议口与处理争议口的生产装配（ADR-0354 决定一至四；票 home-today-todo/03）：两口按特性登记处那一份
// build 与 mount 装上，进的是 buildSettlementOrchestrations 装的截单编排上的开立与处理。对账单经确认口与发布口真发一张，不直写库。
// 证据只记 `S`。

const (
	disputeClerk   = "SYN-DISPUTE-CLERK"
	disputeFinance = "SYN-DISPUTE-FINANCE"

	statementDisputeFace           = "/settlement-statement-disputes"
	statementDisputeResolutionFace = "/settlement-statement-dispute-resolutions"
)

// disputeNumberPattern 是争议产品默认模板 DSP-{yyyy}{mm}-{seq:5} 渲染出的形状（ADR-0355 决定五），比形状不比年月，理由同
// statementNumberPattern。
var disputeNumberPattern = regexp.MustCompile(`^DSP-\d{6}-\d{5}$`)

// newPublishedStatementFixture 发一张只有一笔 9600 费用的对账单，再给客服与财务各登一种争议授予。单号由服务端取，记在 published 上。
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
		`{"account":"`+decisionsAccount+`","period":"SYN-PERIOD-2026-11","version":"v1",`+
			`"currency":"CNY","charges":["`+decisionsCharge+`"],"lastAttributionDate":"`+last+`","declaredTotal":"9600",`+
			`"cutOffAt":"`+time.Now().UTC().Format(time.RFC3339)+`"}`)
	if status != http.StatusCreated {
		t.Fatalf("发布 = %d %v", status, answer)
	}
	fixture.published, _ = answer["statement"].(string)
	publishedAt, err := time.Parse(time.RFC3339Nano, fixture.text(t,
		`SELECT to_char(published_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') FROM settlement_accounting.customer_statement
		  WHERE tenant_id = $1 AND statement_number = $2`, decisionsTenant, fixture.published))
	if err != nil {
		t.Fatal(err)
	}
	return fixture, publishedAt
}

// raisedAfter 交回一个晚于 at 的整秒时刻（载荷按秒写），并等到服务端此刻过了它：提出时刻不得晚于登记时刻（ADR-0354 决定一），
// 用例里的提出时刻都得是已经过去的一刻。
func raisedAfter(at time.Time) time.Time {
	raised := at.Truncate(time.Second).Add(time.Second)
	time.Sleep(time.Until(raised) + 20*time.Millisecond)
	return raised
}

func (fixture *settlementDecisionsFixture) disputeBodyOf(id, amount string, openedAt time.Time) string {
	return `{"dispute":"` + id + `","statement":"` + fixture.published + `","charge":"` + decisionsCharge + `","disputedAmount":"` + amount +
		`","reason":"客户来电：重量按 1.2 kg 计","openedAt":"` + openedAt.UTC().Format(time.RFC3339) + `"}`
}

// Covers: AT-SA-228（ADR-0354 决定一、三、四）——持登记授予的客服代登记一项争议：答 201；同一份重放答已存在，同标识换了金额答冲突；金额
// 越行、单号不在册、提出早于发布、提出晚于登记答未受理；行上记下登记人；持处理授予的财务打登记口答未授予；单作废之后新争议答作废那一格，已开的那项
// 照旧在。
func TestTheDisputeRegistrationFaceRegistersAndRecordsTheClerk(t *testing.T) {
	fixture, publishedAt := newPublishedStatementFixture(t)
	openedAt := raisedAfter(publishedAt)
	body := fixture.disputeBodyOf("SYN-DSP-1", "1600", openedAt)

	if status, answer := fixture.post(t, statementDisputeFace, disputeFinance, body); status != http.StatusForbidden {
		t.Fatalf("持处理授予的人登记 = %d %v，want 403", status, answer)
	}
	status, answer := fixture.post(t, statementDisputeFace, disputeClerk, body)
	number, _ := answer["documentNumber"].(string)
	if status != http.StatusCreated || answer["outcome"] != "DISPUTE_OPENED" || answer["disputedAmount"] != "1600" ||
		answer["statement"] != fixture.published || answer["resolution"] != nil || answer["dispute"] != "SYN-DSP-1" ||
		!disputeNumberPattern.MatchString(number) || !strings.HasSuffix(number, "-00001") || answer["documentNumberAbsence"] != nil {
		t.Fatalf("登记 = %d %v（争议号由服务端在登记事务里取，争议标识仍是载荷里那一个）", status, answer)
	}
	if status, answer := fixture.post(t, statementDisputeFace, disputeClerk, body); status != http.StatusOK || answer["outcome"] != "EXISTING_DISPUTE" ||
		answer["documentNumber"] != number {
		t.Fatalf("重放 = %d %v，要答原来那个争议号", status, answer)
	}
	if status, answer := fixture.post(t, statementDisputeFace, disputeClerk, fixture.disputeBodyOf("SYN-DSP-1", "1700", openedAt)); status != http.StatusOK ||
		answer["outcome"] != "DISPUTE_CONFLICT" {
		t.Fatalf("同标识换金额 = %d %v", status, answer)
	}
	for name, raw := range map[string]string{
		"金额越行":   fixture.disputeBodyOf("SYN-DSP-2", "9601", openedAt),
		"单号不在册":  strings.Replace(fixture.disputeBodyOf("SYN-DSP-3", "100", openedAt), fixture.published, "SYN-STMT-NONE", 1),
		"提出早于发布": fixture.disputeBodyOf("SYN-DSP-4", "100", publishedAt.Add(-time.Hour)),
		"提出晚于登记": fixture.disputeBodyOf("SYN-DSP-6", "100", time.Now().Add(time.Hour)),
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
			decisionsTenant, fixture.published, openedAt)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if status, answer := fixture.post(t, statementDisputeFace, disputeClerk, fixture.disputeBodyOf("SYN-DSP-5", "100", raisedAfter(openedAt))); status != http.StatusOK ||
		answer["outcome"] != "STATEMENT_VOIDED_TAKES_NO_DISPUTE" {
		t.Fatalf("单已作废 = %d %v", status, answer)
	}
	if count := fixture.text(t, `SELECT count(*)::text FROM settlement_accounting.statement_dispute WHERE tenant_id = $1`, decisionsTenant); count != "1" {
		t.Fatalf("库上争议 %s 项，want 只有作废前那一项", count)
	}
	if stored := fixture.text(t, `SELECT document_number FROM settlement_accounting.statement_dispute
		 WHERE tenant_id = $1 AND dispute_id = 'SYN-DSP-1'`, decisionsTenant); stored != number {
		t.Fatalf("库上争议号 = %q，want %s（未受理、冲突、作废那几次都没取号）", stored, number)
	}
}

// Covers: AT-SA-229（ADR-0354 决定二、四）——持处理授予的财务先处理成待复核（201），再处理成拒绝（201），之后再处理答已处理并交回拒绝那
// 一份（200）；行上记下最后那一次处理人，登记人不变；持登记授予的客服打处理口答未授予；不在册的争议答未受理。
func TestTheDisputeResolutionFaceResolvesOnceFinalAndRecordsTheDecider(t *testing.T) {
	fixture, publishedAt := newPublishedStatementFixture(t)
	openedAt := raisedAfter(publishedAt)
	if status, answer := fixture.post(t, statementDisputeFace, disputeClerk, fixture.disputeBodyOf("SYN-DSP-1", "1600", openedAt)); status != http.StatusCreated {
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

// Covers: AT-SA-228（ADR-0354 决定一）——同一编号、金额不同的两份登记并发到达：恰一份开立（201），另一份按内容判答冲突，不答已存在、
// 也不带回赢家的金额；同一编号同内容的两份并发，输的那份答已存在。逐轮换编号，跑几轮让输家走到落库撞主键那一支。
func TestConcurrentRegistrationsOfOneDisputeAreJudgedByContent(t *testing.T) {
	fixture, publishedAt := newPublishedStatementFixture(t)
	openedAt := raisedAfter(publishedAt)
	race := func(id string, amounts [2]string) [2]map[string]any {
		var answers [2]map[string]any
		var statuses [2]int
		var start, done sync.WaitGroup
		start.Add(1)
		for index := range amounts {
			done.Add(1)
			go func(index int) {
				defer done.Done()
				start.Wait()
				statuses[index], answers[index] = fixture.post(t, statementDisputeFace, disputeClerk, fixture.disputeBodyOf(id, amounts[index], openedAt))
			}(index)
		}
		start.Done()
		done.Wait()
		for index := range answers {
			answers[index]["status"] = statuses[index]
		}
		return answers
	}
	for round := 0; round < 8; round++ {
		answers := race(fmt.Sprintf("SYN-DSP-RACE-%d", round), [2]string{"100", "200"})
		opened, conflicts := 0, 0
		for _, answer := range answers {
			switch answer["outcome"] {
			case "DISPUTE_OPENED":
				opened++
			case "DISPUTE_CONFLICT":
				conflicts++
			default:
				t.Fatalf("第 %d 轮不同金额：答 %v（输家不得答已存在、不得带回赢家的金额）", round, answers)
			}
		}
		if opened != 1 || conflicts != 1 {
			t.Fatalf("第 %d 轮不同金额：%v", round, answers)
		}

		same := race(fmt.Sprintf("SYN-DSP-SAME-%d", round), [2]string{"300", "300"})
		if outcomes := []any{same[0]["outcome"], same[1]["outcome"]}; !(outcomes[0] == "DISPUTE_OPENED" && outcomes[1] == "EXISTING_DISPUTE") &&
			!(outcomes[1] == "DISPUTE_OPENED" && outcomes[0] == "EXISTING_DISPUTE") {
			t.Fatalf("第 %d 轮同内容：%v", round, same)
		}
		if same[0]["documentNumber"] != same[1]["documentNumber"] {
			t.Fatalf("第 %d 轮同内容两份答了两个争议号：%v", round, same)
		}
	}
	// 每轮恰开立两项（不同金额一项、同内容一项），争议号逐项不重、一个空号也没有：输家在登记锁上排队，没先取号再撞主键。
	if count, distinct, highest := fixture.text(t, `SELECT count(*)::text FROM settlement_accounting.statement_dispute WHERE tenant_id = $1`, decisionsTenant),
		fixture.text(t, `SELECT count(DISTINCT document_number)::text FROM settlement_accounting.statement_dispute WHERE tenant_id = $1`, decisionsTenant),
		fixture.text(t, `SELECT max(right(document_number, 5)) FROM settlement_accounting.statement_dispute WHERE tenant_id = $1`, decisionsTenant); count != "16" || distinct != "16" || highest != "00016" {
		t.Fatalf("争议 %s 项、争议号 %s 个、最大流水 %s，want 16、16、00016", count, distinct, highest)
	}
}

// Covers: AT-SA-228（ADR-0354 决定三）——单作废之后：提出时刻不晚于作废的照收（客户作废前来电、客服作废后才登记；这里取恰在作废那一刻），作废前已开的那项
// 仍能处理到终局。
func TestADisputeRaisedBeforeTheVoidIsStillRegisteredAndOpenOnesStillResolve(t *testing.T) {
	fixture, publishedAt := newPublishedStatementFixture(t)
	openedAt := raisedAfter(publishedAt)
	if status, answer := fixture.post(t, statementDisputeFace, disputeClerk, fixture.disputeBodyOf("SYN-DSP-EARLY", "100", openedAt)); status != http.StatusCreated {
		t.Fatalf("作废前登记 = %d %v", status, answer)
	}
	// 作废时刻取恰在早先那一项提出的那一刻：提出时刻都要不晚于登记时刻（服务端此刻），作废之后提出的那一项也得在此刻之前。
	voidAt := openedAt
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		executor, err := fixture.db.RequireExecutor(ctx)
		if err != nil {
			return err
		}
		_, err = executor.Exec(ctx, `UPDATE settlement_accounting.customer_statement
		    SET void_basis = 'SYN-VOID-1', voided_at = $3 WHERE tenant_id = $1 AND statement_number = $2`,
			decisionsTenant, fixture.published, voidAt)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if status, answer := fixture.post(t, statementDisputeFace, disputeClerk, fixture.disputeBodyOf("SYN-DSP-LATE-ENTRY", "100", voidAt)); status != http.StatusCreated ||
		answer["outcome"] != "DISPUTE_OPENED" {
		t.Fatalf("作废前提出、作废后登记 = %d %v", status, answer)
	}
	if status, answer := fixture.post(t, statementDisputeFace, disputeClerk, fixture.disputeBodyOf("SYN-DSP-AFTER", "100", raisedAfter(voidAt))); status != http.StatusOK ||
		answer["outcome"] != "STATEMENT_VOIDED_TAKES_NO_DISPUTE" {
		t.Fatalf("作废后提出 = %d %v", status, answer)
	}
	resolve := `{"dispute":"SYN-DSP-EARLY","resolution":"REJECTED","basis":"核过磅单","resolvedAt":"` + voidAt.Add(time.Hour).UTC().Format(time.RFC3339) + `"}`
	if status, answer := fixture.post(t, statementDisputeResolutionFace, disputeFinance, resolve); status != http.StatusCreated || answer["resolution"] != "REJECTED" {
		t.Fatalf("作废前已开的那项处理到终局 = %d %v", status, answer)
	}
}
