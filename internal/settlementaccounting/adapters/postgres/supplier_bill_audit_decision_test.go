package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
	"go.idp.xyz/idp-parcel/migrations"
)

// 本文件对真实 PostgreSQL 证 ADR-0357 的两张新册：供应商账单号册（同一供应商同号只标识一份主张）与审核决定册（同一行上直接给的
// 各段、同一争议的各份再审互不重叠，库上的排他约束兜底），经审核编排走一遍三种结果、并发审同一行、争议的再审与旧载荷的重放，
// 以及迁移 0054 在有旧应付的库上回填。证据只记 `S`。

func rangedAudit(t *testing.T, claim, line, decision string, verdict domain.AuditVerdict, from, to int64, basis, resolves string) application.AuditBillLineCommand {
	t.Helper()
	span, err := domain.NewAmountRange(from, to)
	if err != nil {
		t.Fatal(err)
	}
	command := application.AuditBillLineCommand{
		TenantID: saTenant(t, "tenant-1"), Claim: saValue(t, domain.NewBillClaimID, claim),
		Version: saValue(t, domain.NewBillClaimVersion, "v1"), Line: saValue(t, domain.NewBillLineReference, line),
		Decision: saValue(t, domain.NewAuditDecisionID, decision), Verdict: verdict, Range: span,
	}
	if basis != "" {
		command.Basis = saValue(t, domain.NewAuditBasisReference, basis)
	}
	if resolves != "" {
		command.Resolves = saValue(t, domain.NewAuditDecisionID, resolves)
	}
	return command
}

func (stack numberingStack) audit(t *testing.T, command application.AuditBillLineCommand) (string, application.AuditResult) {
	t.Helper()
	var result application.AuditResult
	outcome := stack.within(t, false, func(ctx context.Context) (string, error) {
		var err error
		result, err = stack.supplier.Audit(ctx, command)
		return result.Outcome().String(), err
	})
	return outcome, result
}

// Covers: `AT-SA-234`、`AT-SA-235`（真库半边）——claim-1 的 L1 已匹配 1000：拆成通过、争议、拒绝三段，只有通过的取应付号；
// 争议以再审解除，再审通过的那一段在同一行上形成第二份应付、取下一个号；再审出了争议答未受理；同一段换个身份再提交答先到的那份；
// 旧载荷整行通过撞上已有的段答范围已裁决；读回的决定经重建门回来。
func TestRangedAuditDecisionsAreKeptDisjointOnTheLine(t *testing.T) {
	stack := newNumberingStack(t)
	registerShanghaiNumbering(t, stack)
	mustSaveBill(t, stack.db.Transactor(), t.Context(), stack.receptions, billRecord(t, "tenant-1", "claim-1", "v1", "digest-1"))

	steps := []struct {
		command application.AuditBillLineCommand
		want    string
	}{
		{rangedAudit(t, "claim-1", "L1", "d-pass", domain.AuditPassed, 0, 400, "", ""), "PAYABLE_FORMED"},
		{rangedAudit(t, "claim-1", "L1", "d-dispute", domain.AuditDisputed, 400, 900, "weight-proof", ""), "DISPUTE_RECORDED"},
		{rangedAudit(t, "claim-1", "L1", "d-reject", domain.AuditRejected, 900, 1000, "not-ordered", ""), "REJECTION_RECORDED"},
		{rangedAudit(t, "claim-1", "L1", "x-overlap", domain.AuditPassed, 300, 500, "", ""), "RANGE_ALREADY_DECIDED"},
		{rangedAudit(t, "claim-1", "L1", "x-outside", domain.AuditRejected, 300, 500, "b", "d-dispute"), "SOURCE_NOT_ACCEPTED"},
		{rangedAudit(t, "claim-1", "L1", "d-reject-again", domain.AuditRejected, 900, 1000, "not-ordered", ""), "EXISTING_DECISION"},
		{rangedAudit(t, "claim-1", "L1", "d-repass", domain.AuditPassed, 400, 700, "weight-confirmed", "d-dispute"), "PAYABLE_FORMED"},
		{rangedAudit(t, "claim-1", "L1", "x-reaudit-overlap", domain.AuditRejected, 600, 900, "b", "d-dispute"), "RANGE_ALREADY_DECIDED"},
		{rangedAudit(t, "claim-1", "L1", "d-rereject", domain.AuditRejected, 700, 900, "refused", "d-dispute"), "REJECTION_RECORDED"},
	}
	numbers := []string{}
	for _, step := range steps {
		outcome, result := stack.audit(t, step.command)
		if outcome != step.want {
			t.Fatalf("%s = %s, want %s", step.command.Decision, outcome, step.want)
		}
		if payable, has := result.Payable(); has && outcome == "PAYABLE_FORMED" {
			numbers = append(numbers, documentNumberOf(payable.Numbering))
		}
	}
	if len(numbers) != 2 || numbers[0] != "AP-202611-00001" || numbers[1] != "AP-202611-00002" {
		t.Fatalf("应付号 = %v，只有两段通过取号、争议与拒绝不取", numbers)
	}

	legacy := application.AuditBillLineCommand{
		TenantID: saTenant(t, "tenant-1"), Claim: saValue(t, domain.NewBillClaimID, "claim-1"),
		Version: saValue(t, domain.NewBillClaimVersion, "v1"), Line: saValue(t, domain.NewBillLineReference, "L1"),
		Payable: saValue(t, domain.NewPayableID, "payable-legacy"),
	}
	if outcome, _ := stack.audit(t, legacy); outcome != "RANGE_ALREADY_DECIDED" {
		t.Fatalf("旧载荷整行通过 = %s", outcome)
	}

	records, err := stack.decisions.ListByLine(t.Context(), saTenant(t, "tenant-1"),
		saValue(t, domain.NewBillClaimID, "claim-1"), saValue(t, domain.NewBillLineReference, "L1"))
	if err != nil || len(records) != 5 {
		t.Fatalf("决定 %d 份 err=%v，want 5", len(records), err)
	}
	decisions := make([]domain.BillLineAuditDecision, 0, len(records))
	for _, record := range records {
		decisions = append(decisions, record.Decision)
		if record.Decision.Auditor().String() != "auditor-1" {
			t.Fatalf("决定 %s 没记审核授权方", record.Key.Decision)
		}
	}
	if tally := domain.TallyLineDecisions(1000, decisions); tally != (domain.LineAuditTally{PassedMinor: 700, RejectedMinor: 300}) {
		t.Fatalf("tally = %+v", tally)
	}
	if count := countRows(t, stack, `SELECT count(*) FROM settlement_accounting.audited_payable WHERE claim_id = 'claim-1' AND line_ref = 'L1'`); count != 2 {
		t.Fatalf("同一行的应付 %d 份，want 2", count)
	}
}

// Covers: `AT-SA-234`（真库半边）——两笔审核并发审同一行、范围重叠：一笔形成应付，另一笔答范围已裁决，只取一个号；同一段同一结果
// 并发（两个身份）则一笔形成、另一笔答先到的那份。
func TestConcurrentAuditsOfOneLineFormOnePayable(t *testing.T) {
	stack := newNumberingStack(t)
	registerShanghaiNumbering(t, stack)
	for _, claim := range []string{"claim-1", "claim-2"} {
		mustSaveBill(t, stack.db.Transactor(), t.Context(), stack.receptions, billRecord(t, "tenant-1", claim, "v1", "digest-"+claim))
	}
	race := func(commands ...application.AuditBillLineCommand) []string {
		outcomes := make([]string, len(commands))
		var wait sync.WaitGroup
		for index, command := range commands {
			wait.Add(1)
			go func() {
				defer wait.Done()
				outcomes[index], _ = stack.audit(t, command)
			}()
		}
		wait.Wait()
		sort.Strings(outcomes)
		return outcomes
	}
	if got := race(
		rangedAudit(t, "claim-1", "L1", "race-a", domain.AuditPassed, 0, 1000, "", ""),
		rangedAudit(t, "claim-1", "L1", "race-b", domain.AuditPassed, 0, 600, "", ""),
	); got[0] != "PAYABLE_FORMED" || got[1] != "RANGE_ALREADY_DECIDED" {
		t.Fatalf("重叠并发 = %v", got)
	}
	if got := race(
		rangedAudit(t, "claim-2", "L1", "same-a", domain.AuditPassed, 0, 1000, "", ""),
		rangedAudit(t, "claim-2", "L1", "same-b", domain.AuditPassed, 0, 1000, "", ""),
	); got[0] != "EXISTING_PAYABLE" || got[1] != "PAYABLE_FORMED" {
		t.Fatalf("同段并发 = %v", got)
	}
	if count := countRows(t, stack, `SELECT count(*) FROM settlement_accounting.audited_payable WHERE document_number IS NOT NULL`); count != 2 {
		t.Fatalf("取了号的应付 %d 份，want 2（每行一份）", count)
	}
	if next, _ := stack.audit(t, rangedAudit(t, "claim-1", "L2", "next", domain.AuditPassed, 0, 300, "", "")); next != "PAYABLE_FORMED" {
		t.Fatalf("next = %s", next)
	}
	if number := textRow(t, stack, `SELECT document_number FROM settlement_accounting.audited_payable WHERE payable_id = 'next'`); number != "AP-202611-00003" {
		t.Fatalf("下一个号 = %s（并发那几笔只该取两个号）", number)
	}
}

// Covers: 审核决定册与账单号册的库面兜底——没排队的写者撞上同一行已有的段、同一争议已有的再审都答`已有记录`，首尾相接照常落；
// 号册拒首尾空白、控制字符与格式字符；事务外 Save 拒（ErrTransactionRequired）；争议不带依据、
// 再审写成争议、范围倒置由 CHECK 拒；他租户读不到。
func TestTheAuditDecisionRegisterGuardsRangesInTheDatabase(t *testing.T) {
	stack := newNumberingStack(t)
	ctx := t.Context()
	save := func(id string, verdict domain.AuditVerdict, from, to int64, basis, resolves string) ports.AuditDecisionSaveOutcome {
		t.Helper()
		span, _ := domain.NewAmountRange(from, to)
		spec := domain.RehydrateBillLineAuditDecisionSpec{
			Decision: saValue(t, domain.NewAuditDecisionID, id), Claim: saValue(t, domain.NewBillClaimID, "claim-1"),
			Version: saValue(t, domain.NewBillClaimVersion, "v1"), Line: saValue(t, domain.NewBillLineReference, "L1"),
			Verdict: verdict, Span: span, Auditor: saValue(t, domain.NewAuditorReference, "auditor-1"), AuditedAt: numberingNow,
		}
		if basis != "" {
			spec.Basis = saValue(t, domain.NewAuditBasisReference, basis)
		}
		if resolves != "" {
			spec.Resolves = saValue(t, domain.NewAuditDecisionID, resolves)
		}
		decision, err := domain.RehydrateBillLineAuditDecision(spec)
		if err != nil {
			t.Fatal(err)
		}
		var outcome ports.AuditDecisionSaveOutcome
		saWithin(t, stack.db.Transactor(), ctx, func(txCtx context.Context) error {
			var err error
			outcome, err = stack.decisions.Save(txCtx, ports.AuditDecisionRecord{
				Key:           ports.AuditDecisionKey{TenantID: saTenant(t, "tenant-1"), Decision: decision.Decision()},
				ContentDigest: "digest-" + id, Decision: decision, RecordedAt: numberingNow, OperationEvidence: "OPERATOR/syn#a",
			})
			return err
		})
		return outcome
	}
	cases := []struct {
		name string
		got  ports.AuditDecisionSaveOutcome
		want ports.AuditDecisionSaveOutcome
	}{
		{"dispute", save("dispute", domain.AuditDisputed, 0, 500, "b", ""), ports.AuditDecisionSaved},
		{"touching pass", save("pass", domain.AuditPassed, 500, 1000, "", ""), ports.AuditDecisionSaved},
		{"overlapping pass", save("pass-2", domain.AuditPassed, 400, 600, "", ""), ports.AuditDecisionAlreadyRecorded},
		{"same identity", save("dispute", domain.AuditRejected, 2000, 3000, "b", ""), ports.AuditDecisionAlreadyRecorded},
		{"reaudit", save("re-1", domain.AuditPassed, 0, 200, "b", "dispute"), ports.AuditDecisionSaved},
		{"overlapping reaudit", save("re-2", domain.AuditRejected, 100, 300, "b", "dispute"), ports.AuditDecisionAlreadyRecorded},
		{"touching reaudit", save("re-3", domain.AuditRejected, 200, 500, "b", "dispute"), ports.AuditDecisionSaved},
	}
	for _, testCase := range cases {
		if testCase.got != testCase.want {
			t.Fatalf("%s = %d, want %d", testCase.name, testCase.got, testCase.want)
		}
	}
	found, present, err := stack.decisions.FindByKey(ctx, ports.AuditDecisionKey{
		TenantID: saTenant(t, "tenant-1"), Decision: saValue(t, domain.NewAuditDecisionID, "re-3")})
	if resolves, _ := found.Decision.Resolves(); err != nil || !present || resolves.String() != "dispute" ||
		found.Decision.Span().From() != 200 || found.OperationEvidence != "OPERATOR/syn#a" {
		t.Fatalf("读回 = %+v present=%v err=%v", found, present, err)
	}
	if _, present, _ := stack.decisions.FindByKey(ctx, ports.AuditDecisionKey{
		TenantID: saTenant(t, "tenant-b"), Decision: saValue(t, domain.NewAuditDecisionID, "re-3")}); present {
		t.Fatal("他租户读到了本租户的审核决定")
	}
	if outcome, err := stack.decisions.Save(ctx, found); !errors.Is(err, bentopg.ErrTransactionRequired) || outcome != ports.AuditDecisionSaveOutcomeInvalid {
		t.Fatalf("事务外 Save 应答 ErrTransactionRequired，实得：%d %v", outcome, err)
	}

	for name, statement := range map[string]string{
		"dispute without basis": `INSERT INTO settlement_accounting.supplier_bill_audit_decision
			(tenant_id, decision_id, claim_id, line_ref, verdict, range_from_minor, range_to_minor, auditor_ref, audited_at, content_digest, recorded_at)
			VALUES ('tenant-1', 'c1', 'claim-9', 'L1', 'DISPUTED', 0, 10, 'a', now(), 'd', now())`,
		"reaudit as dispute": `INSERT INTO settlement_accounting.supplier_bill_audit_decision
			(tenant_id, decision_id, claim_id, line_ref, verdict, range_from_minor, range_to_minor, basis_ref, resolves_decision, auditor_ref, audited_at, content_digest, recorded_at)
			VALUES ('tenant-1', 'c2', 'claim-9', 'L1', 'DISPUTED', 0, 10, 'b', 'x', 'a', now(), 'd', now())`,
		"inverted range": `INSERT INTO settlement_accounting.supplier_bill_audit_decision
			(tenant_id, decision_id, claim_id, line_ref, verdict, range_from_minor, range_to_minor, auditor_ref, audited_at, content_digest, recorded_at)
			VALUES ('tenant-1', 'c3', 'claim-9', 'L1', 'PASSED', 10, 10, 'a', now(), 'd', now())`,
		"unknown verdict": `INSERT INTO settlement_accounting.supplier_bill_audit_decision
			(tenant_id, decision_id, claim_id, line_ref, verdict, range_from_minor, range_to_minor, basis_ref, auditor_ref, audited_at, content_digest, recorded_at)
			VALUES ('tenant-1', 'c4', 'claim-9', 'L1', 'ESCALATED', 0, 10, 'b', 'a', now(), 'd', now())`,
	} {
		if _, err := stack.pool.Exec(ctx, statement); err == nil {
			t.Fatalf("%s 落了库", name)
		}
	}

	// 账单号册的库面兜底（领域 NewSupplierBillNumber 之外）：首尾空白、控制字符与格式字符落不进；中间的普通空格与小写照收。
	register := func(number string) error {
		_, err := stack.pool.Exec(ctx, `INSERT INTO settlement_accounting.supplier_bill_number
			(tenant_id, supplier_ref, bill_number, claim_id, claim_version, recorded_at)
			VALUES ('tenant-1', 'supplier-1', $1, 'claim-'||md5($1), 'v1', now())`, number)
		return err
	}
	for _, number := range []string{"INV-9 ", " INV-9", "INV-9\u3000", "INV\t9", "INV\u200b9", "\ufeffINV-9", "INV-9\u202e", "INV\U000E00419"} {
		if register(number) == nil {
			t.Fatalf("账单号 %q 落进了号册", number)
		}
	}
	for _, number := range []string{"INV 9", "inv-9", "账单-9"} {
		if err := register(number); err != nil {
			t.Fatalf("账单号 %q 应照收：%v", number, err)
		}
	}
}

// Covers: `AT-SA-233`（真库半边）——账单号随接收写进账单号册，主张读回带号（不论哪一版带来）；两笔并发接收同一供应商同一号、不同主张：
// 一笔接收、另一笔答账单号冲突，册上只有一份主张；新版本换了供应商，带不带号都答主张供应商冲突；另一家供应商同号照常；没排队的
// 写者撞号交回错误、接收行不落。
func TestASupplierBillNumberIsRegisteredOncePerSupplier(t *testing.T) {
	stack := newNumberingStack(t)
	ctx := t.Context()
	instances := registeredPayableInstances{}
	receiver := application.NewReceiveSupplierBillHandler(application.ReceiveSupplierBillDeps{
		Receptions: stack.receptions, Costs: zeroCosts{}, Successions: zeroCosts{}, Authority: instances,
		Downstream: quietBillHandoff{}, Clock: stack.clock,
	})
	command := func(claim, version, supplier, number string) application.ReceiveSupplierBillCommand {
		spec := domain.SupplierBillClaimSpec{
			Claim: saValue(t, domain.NewBillClaimID, claim), Version: saValue(t, domain.NewBillClaimVersion, version),
			Supplier: saValue(t, domain.NewSupplierPartyReference, supplier), LegalEntity: saValue(t, domain.NewLegalEntityReference, "entity-1"),
			Period: saValue(t, domain.NewBillingPeriodReference, "2026-11"), Currency: saValue(t, domain.NewCurrencyCode, "CNY"),
			Lines:      []domain.BillLine{{Line: saValue(t, domain.NewBillLineReference, "L1"), FeeItem: saValue(t, domain.NewFeeItemReference, "fee-a"), ClaimedMinor: 700}},
			ReceivedAt: numberingNow.Add(-time24h),
		}
		if number != "" {
			spec.BillNumber = saValue(t, domain.NewSupplierBillNumber, number)
		}
		return application.ReceiveSupplierBillCommand{
			TenantID: saTenant(t, "tenant-1"), Claim: spec,
			Directives: []application.LineMatchDirective{{
				Line: saValue(t, domain.NewBillLineReference, "L1"), Classification: domain.NoMatchingOccurrence,
				Basis: saValue(t, domain.NewMatchBasisReference, "search-1"),
			}},
		}
	}
	receive := func(command application.ReceiveSupplierBillCommand) (string, application.ReceiveSupplierBillResult) {
		var result application.ReceiveSupplierBillResult
		outcome := stack.within(t, false, func(ctx context.Context) (string, error) {
			var err error
			result, err = receiver.Handle(ctx, command)
			return result.Outcome().String(), err
		})
		return outcome, result
	}

	outcomes := make([]string, 2)
	var wait sync.WaitGroup
	for index, claim := range []string{"claim-a", "claim-b"} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			outcomes[index], _ = receive(command(claim, "v1", "supplier-1", "INV-001"))
		}()
	}
	wait.Wait()
	sort.Strings(outcomes)
	if outcomes[0] != "BILL_RECEIVED" || outcomes[1] != "SUPPLIER_BILL_NUMBER_CONFLICT" {
		t.Fatalf("并发同号 = %v", outcomes)
	}
	if count := countRows(t, stack, `SELECT count(*) FROM settlement_accounting.supplier_bill_reception`); count != 1 {
		t.Fatalf("接收行 %d，同一张账单只该落一份", count)
	}
	holder := textRow(t, stack, `SELECT claim_id || '|' || supplier_ref || '|' || claim_version FROM settlement_accounting.supplier_bill_number WHERE bill_number = 'INV-001'`)
	if holder != "claim-a|supplier-1|v1" && holder != "claim-b|supplier-1|v1" {
		t.Fatalf("账单号册 = %s", holder)
	}
	winner := holder[:len("claim-a")]

	if outcome, _ := receive(command(winner, "v2", "supplier-1", "")); outcome != "BILL_RECEIVED" {
		t.Fatalf("不带号的新版本 = %s", outcome)
	}
	found, present, err := stack.receptions.FindByKey(ctx, ports.BillReceptionKey{
		TenantID: saTenant(t, "tenant-1"), Claim: saValue(t, domain.NewBillClaimID, winner), Version: saValue(t, domain.NewBillClaimVersion, "v2")})
	if number, has := found.Claim.BillNumber(); err != nil || !present || !has || number.String() != "INV-001" {
		t.Fatalf("v2 读回的号 = %v %v（号标识主张，不论哪一版带来）", number, has)
	}
	// 同一份主张的新版本换了供应商，带不带号都答主张供应商冲突、带出原先的供应商。不先这样挡，带号的那一笔会撞上号册的分配方、
	// 落成一致性错误（未形成答案），不带号的那一笔会在号册分配方之外另起一个供应商。
	for _, number := range []string{"INV-001", ""} {
		outcome, switched := receive(command(winner, "v3", "supplier-2", number))
		if original, _ := switched.SupplierConflict(); outcome != "CLAIM_SUPPLIER_CONFLICT" || original.String() != "supplier-1" {
			t.Fatalf("换供应商的新版本（号 %q）= %s %q", number, outcome, original)
		}
	}
	if count := countRows(t, stack, `SELECT count(*) FROM settlement_accounting.supplier_bill_reception WHERE supplier_ref = 'supplier-2' AND claim_id = '`+winner+`'`); count != 0 {
		t.Fatal("换供应商的新版本落了库")
	}
	if outcome, _ := receive(command("claim-c", "v1", "supplier-2", "INV-001")); outcome != "BILL_RECEIVED" {
		t.Fatalf("另一家供应商同号 = %s", outcome)
	}

	// 没排队的写者：直接经接收库写另一份主张、同一号。号册先写、核不过交回错误，接收行不落。
	stray, err := domain.ReceiveSupplierBillClaim(command("claim-d", "v1", "supplier-1", "INV-001").Claim)
	if err != nil {
		t.Fatal(err)
	}
	strayErr := stack.db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := stack.receptions.Save(txCtx, ports.BillReceptionRecord{
			Key:   ports.BillReceptionKey{TenantID: saTenant(t, "tenant-1"), Claim: stray.Claim(), Version: stray.Version()},
			Claim: stray, ContentDigest: "stray", RecordedAt: numberingNow,
		})
		return err
	})
	if strayErr == nil {
		t.Fatal("没排队的写者撞号却写成了")
	}
	if count := countRows(t, stack, `SELECT count(*) FROM settlement_accounting.supplier_bill_reception WHERE claim_id = 'claim-d'`); count != 0 {
		t.Fatal("撞号的接收行落了库")
	}
}

// Covers: 迁移 settlement_accounting/0054 施加在有旧应付的库上——旧应付回填为通过整行的决定（标识、摘要、审核方照搬，版本留空），
// 回填之后同一行能有第二份应付；账单号册不从「供应商/账单号」形的旧主张标识拆号回填；旧载荷的重放照旧认得回填的那份。
func TestTheAuditDecisionMigrationBackfillsExistingPayables(t *testing.T) {
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, pgtest.FreshDatabase(t))
	if err != nil {
		t.Fatalf("连空库：%v", err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	if _, err := conn.Exec(ctx, `CREATE SCHEMA settlement_accounting`); err != nil {
		t.Fatalf("建 schema：%v", err)
	}
	assets, err := migrations.SettlementAccounting()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	const target = "0054_supplier_bill_number_and_audit_decisions"
	var backfill *migrations.Asset
	for index := range assets {
		if assets[index].Name == target {
			backfill = &assets[index]
			break
		}
		if _, err := conn.Exec(ctx, assets[index].SQL); err != nil {
			t.Fatalf("施加 %s：%v", assets[index].Name, err)
		}
	}
	if backfill == nil {
		t.Fatalf("没有找到迁移 %s", target)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO settlement_accounting.supplier_bill_reception
			(tenant_id, claim_id, claim_version, supplier_ref, legal_entity, period_ref, currency, content_digest, claim, matches,
			 audit_authority_configured, recorded_at)
		VALUES ('SYN-TENANT-01', 'SYN-PARTY-1/INV-2610-001', 'v1', 'SYN-PARTY-1', 'SYN-LE-01', '2026-10', 'CNY', 'rd',
		        '{"lines":[{"line":"L1","fee_item":"F","claimed_minor":8500}],"received_at":"2026-10-28T02:00:00Z"}',
		        '[{"line":"L1","classification":"MATCHED","expected":"SYN-COST-1","claimed_minor":8500,"expected_minor":8500,"matched_at":"2026-10-28T02:00:00Z"}]',
		        true, now());
		INSERT INTO settlement_accounting.audited_payable
			(tenant_id, payable_id, claim_id, line_ref, expected_version, legal_entity, account_id, currency, amount_minor, auditor_ref,
			 audited_at, content_digest, recorded_at, operation_evidence, document_number)
		VALUES ('SYN-TENANT-01', 'PAYABLE-OLD', 'SYN-PARTY-1/INV-2610-001', 'L1', 'SYN-COST-1', 'SYN-LE-01', 'SYN-ACC-2', 'CNY', 8500,
		        'SYN-AUDITOR-1', now(), 'SAC-1:pd', now(), 'OPERATOR/syn#old', 'AP-202611-00001')`); err != nil {
		t.Fatalf("落旧形状的行：%v", err)
	}
	if _, err := conn.Exec(ctx, backfill.SQL); err != nil {
		t.Fatalf("施加 %s：%v", target, err)
	}

	var row string
	if err := conn.QueryRow(ctx, `SELECT decision_id || '|' || claim_id || '|' || line_ref || '|' || verdict || '|' ||
			range_from_minor || '|' || range_to_minor || '|' || coalesce(claim_version, '-') || '|' || coalesce(basis_ref, '-') || '|' ||
			auditor_ref || '|' || content_digest || '|' || coalesce(operation_evidence, '-')
		  FROM settlement_accounting.supplier_bill_audit_decision`).Scan(&row); err != nil {
		t.Fatal(err)
	}
	if want := "PAYABLE-OLD|SYN-PARTY-1/INV-2610-001|L1|PASSED|0|8500|-|-|SYN-AUDITOR-1|SAC-1:pd|OPERATOR/syn#old"; row != want {
		t.Fatalf("回填 = %s\nwant  %s", row, want)
	}
	var numbers int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM settlement_accounting.supplier_bill_number`).Scan(&numbers); err != nil || numbers != 0 {
		t.Fatalf("账单号册 %d 行 err=%v，旧主张标识不拆号回填", numbers, err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO settlement_accounting.audited_payable
			(tenant_id, payable_id, claim_id, line_ref, expected_version, legal_entity, account_id, currency, amount_minor, auditor_ref,
			 audited_at, content_digest, recorded_at)
		VALUES ('SYN-TENANT-01', 'PAYABLE-SECOND', 'SYN-PARTY-1/INV-2610-001', 'L1', 'SYN-COST-1', 'SYN-LE-01', 'SYN-ACC-2', 'CNY', 1,
		        'SYN-AUDITOR-1', now(), 'x', now())`); err != nil {
		t.Fatalf("撤下按行唯一之后同一行第二份应付落不进：%v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO settlement_accounting.supplier_bill_audit_decision
			(tenant_id, decision_id, claim_id, line_ref, verdict, range_from_minor, range_to_minor, auditor_ref, audited_at, content_digest, recorded_at)
		VALUES ('SYN-TENANT-01', 'OVERLAP', 'SYN-PARTY-1/INV-2610-001', 'L1', 'PASSED', 8000, 9000, 'a', now(), 'd', now())`); err == nil {
		t.Fatal("与回填那一段重叠的决定落了库")
	}
}

// Covers: `AT-SA-234`（旧载荷照收的真库半边）——迁移之前形成的应付已回填进决定册：旧载荷以同一应付标识重放答原应付与原号，不取新号；
// 换一个应付标识再整行通过答先到的那份。
func TestALegacyAuditReplaysAgainstABackfilledPayable(t *testing.T) {
	stack := newNumberingStack(t)
	registerShanghaiNumbering(t, stack)
	mustSaveBill(t, stack.db.Transactor(), t.Context(), stack.receptions, billRecord(t, "tenant-1", "claim-1", "v1", "digest-1"))
	legacy := func(payable string) application.AuditBillLineCommand {
		return application.AuditBillLineCommand{
			TenantID: saTenant(t, "tenant-1"), Claim: saValue(t, domain.NewBillClaimID, "claim-1"),
			Version: saValue(t, domain.NewBillClaimVersion, "v1"), Line: saValue(t, domain.NewBillLineReference, "L1"),
			Payable: saValue(t, domain.NewPayableID, payable),
		}
	}
	first, formed := stack.audit(t, legacy("payable-1"))
	payable, _ := formed.Payable()
	if first != "PAYABLE_FORMED" {
		t.Fatalf("first = %s", first)
	}
	// 把它做成迁移之前的样子：决定册里那一行换成回填的形状（版本留空），摘要照旧。
	if _, err := stack.pool.Exec(t.Context(), `UPDATE settlement_accounting.supplier_bill_audit_decision SET claim_version = NULL WHERE decision_id = 'payable-1'`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"payable-1", "payable-2"} {
		outcome, result := stack.audit(t, legacy(id))
		replayed, _ := result.Payable()
		if outcome != "EXISTING_PAYABLE" || replayed.Key.Payable.String() != "payable-1" || replayed.Numbering != payable.Numbering {
			t.Fatalf("%s 重放 = %s %+v", id, outcome, replayed)
		}
	}
	if count := countRows(t, stack, `SELECT count(*) FROM settlement_accounting.audited_payable`); count != 1 {
		t.Fatalf("应付 %d 份，want 1", count)
	}
}

func countRows(t *testing.T, stack numberingStack, query string) int {
	t.Helper()
	var count int
	if err := stack.pool.QueryRow(t.Context(), query).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func textRow(t *testing.T, stack numberingStack, query string) string {
	t.Helper()
	var value string
	if err := stack.pool.QueryRow(t.Context(), query).Scan(&value); err != nil {
		t.Fatal(fmt.Errorf("%s: %w", query, err))
	}
	return value
}

const time24h = 24 * time.Hour

// registerShanghaiNumbering 给 tenant-1 登单据编号时区：应付账户没登对账安排，日期按它出（ADR-0355 决定七）。
func registerShanghaiNumbering(t *testing.T, stack numberingStack) {
	t.Helper()
	saWithin(t, stack.db.Transactor(), t.Context(), func(txCtx context.Context) error {
		_, err := stack.registrar.RegisterTimeZone(txCtx, docnumber.RegisterTimeZoneCommand{
			Tenant: "tenant-1", TimeZone: "Asia/Shanghai", EffectiveFrom: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
		})
		return err
	})
}

// zeroCosts 是一本空的预期成本册：本文件的账单只给「没有对应发生项」的裁决，不点名预期成本。
type zeroCosts struct{}

func (zeroCosts) LoadExpectedCost(context.Context, domain.TenantID, domain.SupplierCostVersionID) (domain.SupplierExpectedCost, bool, error) {
	return domain.SupplierExpectedCost{}, false, nil
}

func (zeroCosts) LoadExpectedCostSuccessor(context.Context, domain.TenantID, domain.SupplierCostVersionID) (domain.SupplierCostVersionID, bool, error) {
	return domain.SupplierCostVersionID{}, false, nil
}
