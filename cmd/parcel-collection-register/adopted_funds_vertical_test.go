package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// 本文件在真实 PostgreSQL 上证运营企业真实到账凭 settlement-accounting 已采用事实登记的整条链（ADR-0274、ADR-0330，
// 隔离合成 S）：走 buildRegistrar 的生产装配，结算一侧用与其采用口、映射口落下的同形的行（support 文件）。

func arrivalInput(fact, instruction, funds, amount string) string {
	return `{
		"tenantId": "SYN-T1", "factRef": "` + fact + `", "instructionRef": "` + instruction + `",
		"sourceLayer": "OPERATOR_BANK_CREDIT", "evidenceRef": "` + funds + `",
		"currency": "EUR", "amountMinor": ` + amount + `, "occurredAt": "2026-08-24T01:00:00Z"
	}`
}

func instructionInput(ref, amount string) string {
	return `{
		"tenantId": "SYN-T1", "instructionRef": "` + ref + `",
		"parcelRef": "SYN-PARCEL-` + ref + `", "requirementRef": "SYN-COD-REQ-V1",
		"customerRef": "SYN-CUST-1", "legalEntityRef": "SYN-ENTITY-1",
		"currency": "EUR", "channelRef": "SYN-CHANNEL-1",
		"amountMinor": ` + amount + `, "instructedAt": "2026-08-24T00:30:00Z"
	}`
}

func reportInput(fact, instruction, amount string) string {
	return `{
		"tenantId": "SYN-T1", "factRef": "` + fact + `", "instructionRef": "` + instruction + `",
		"sourceLayer": "CHANNEL_COLLECTION_REPORT", "evidenceRef": "SYN-EV-` + fact + `",
		"currency": "EUR", "amountMinor": ` + amount + `, "occurredAt": "2026-08-24T01:00:00Z"
	}`
}

func moveInput(posting, from, to, amount, kind, basis string) string {
	return `{
		"tenantId": "SYN-T1", "postingRef": "` + posting + `",
		"fromPosition": "` + from + `", "toPosition": "` + to + `",
		"amountMinor": ` + amount + `, "basisKind": "` + kind + `",
		"basisRef": "` + basis + `", "postedAt": "2026-08-24T02:00:00Z"
	}`
}

func openLedgerInput() string {
	return `{
		"tenantId": "SYN-T1", "customerRef": "SYN-CUST-1", "legalEntityRef": "SYN-ENTITY-1",
		"currency": "EUR", "channelRef": "SYN-CHANNEL-1",
		"custodyBasisRef": "SYN-COD-V1", "openedAt": "2026-08-24T00:00:00Z"
	}`
}

func scalar[T any](t *testing.T, rig concurrencyRig, query string, into *T) {
	t.Helper()
	if err := rig.pool.QueryRow(t.Context(), query).Scan(into); err != nil {
		t.Fatalf("%s：%v", query, err)
	}
}

// Covers: AT-CR-020、AT-CR-026、AT-CR-027（生产装配半边）——一笔 1000.00 的回款拆给两条指令：
//
//	先在结算采用 → 还没认作代收本金时到账答依据不在册（迟到的到账等采用与认作，不自铸）→ 认作之后 600.00 + 400.00 登记；
//	再多一分、同一指令换事实标识再登都答内容冲突；已登记的重放答已在册；渠道报告入在途后 A 移位，
//	结算把事实更正到 900.00 之后 B 的移位答内容冲突、待清分仍是 600.00，更正回 1000.00 后 B 可移位。
//
// 每一步的账面都核：入账合计 = 账内各位置之和，移位只在放行时发生。
func TestAnArrivalIsRegisteredAndMovedOnlyWhileTheAdoptedPrincipalFactCoversIt(t *testing.T) {
	rig := newConcurrencyRig(t)
	pool := rig.pool
	must := func(command, raw string, wantCode int, wantAnswer string) {
		t.Helper()
		message, code := execute(t.Context(), command, []byte(raw), rig.registrar)
		if code != wantCode {
			t.Fatalf("%s 退出码 = %d（%s），要 %d", command, code, message, wantCode)
		}
		if wantAnswer != "" && !strings.Contains(message, wantAnswer) {
			t.Fatalf("%s 答复 = %q，要含 %s", command, message, wantAnswer)
		}
	}
	must(commandSubledger, openLedgerInput(), exitRegistered, "REGISTERED")
	must(commandInstruction, instructionInput("SYN-INSTR-1", "60000"), exitRegistered, "REGISTERED")
	must(commandInstruction, instructionInput("SYN-INSTR-2", "40000"), exitRegistered, "REGISTERED")
	must(commandInstruction, instructionInput("SYN-INSTR-3", "50"), exitRegistered, "REGISTERED")

	// 结算还没有这条事实：到账答依据不在册，什么都不落。
	must(commandFact, arrivalInput("SYN-FACT-A", "SYN-INSTR-1", "SYN-BANK-RECEIPT-1", "60000"), exitNeedsReview, "BASIS_MISSING")
	// 采用了、还没认作代收本金：同一个答案——这条回款可能是运营企业自己的收款。
	seedAdoptedFunds(t, pool, "SYN-BANK-RECEIPT-1", "EUR", 100000)
	must(commandFact, arrivalInput("SYN-FACT-A", "SYN-INSTR-1", "SYN-BANK-RECEIPT-1", "60000"), exitNeedsReview, "BASIS_MISSING")
	var facts int
	scalar(t, rig, `SELECT count(*) FROM collection_remittance.collection_fact`, &facts)
	if facts != 0 {
		t.Fatalf("依据不在册仍落了 %d 条事实", facts)
	}

	setAsideAsPrincipal(t, pool, "SYN-BANK-RECEIPT-1")
	must(commandFact, arrivalInput("SYN-FACT-A", "SYN-INSTR-1", "SYN-BANK-RECEIPT-1", "60000"), exitRegistered, "REGISTERED")
	must(commandFact, arrivalInput("SYN-FACT-A", "SYN-INSTR-1", "SYN-BANK-RECEIPT-1", "60000"), exitRegistered, "EXISTING")
	must(commandFact, arrivalInput("SYN-FACT-A", "SYN-INSTR-1", "SYN-BANK-RECEIPT-1", "59999"), exitNeedsReview, "CONTENT_CONFLICT")
	must(commandFact, arrivalInput("SYN-FACT-A2", "SYN-INSTR-1", "SYN-BANK-RECEIPT-1", "60000"), exitNeedsReview, "CONTENT_CONFLICT")
	must(commandFact, arrivalInput("SYN-FACT-B", "SYN-INSTR-2", "SYN-BANK-RECEIPT-1", "40000"), exitRegistered, "REGISTERED")
	must(commandFact, arrivalInput("SYN-FACT-C", "SYN-INSTR-3", "SYN-BANK-RECEIPT-1", "1"), exitNeedsReview, "CONTENT_CONFLICT")

	var referenced, total int64
	scalar(t, rig, `SELECT count(*) FROM collection_remittance.collection_fact
		WHERE source_layer = 'OPERATOR_BANK_CREDIT' AND evidence_ref = 'SYN-BANK-RECEIPT-1'`, &referenced)
	scalar(t, rig, `SELECT COALESCE(SUM(amount_minor), 0)::bigint FROM collection_remittance.collection_fact
		WHERE source_layer = 'OPERATOR_BANK_CREDIT' AND evidence_ref = 'SYN-BANK-RECEIPT-1'`, &total)
	if referenced != 2 || total != 100000 {
		t.Fatalf("引用这条事实的到账 %d 条、合计 %d，要 2 条、合计 100000", referenced, total)
	}
	var pinned string
	scalar(t, rig, `SELECT funds_fact_version FROM collection_remittance.collection_fact WHERE fact_ref = 'SYN-FACT-A'`, &pinned)
	if pinned != "SYN-BANK-RECEIPT-1/v1" {
		t.Fatalf("钉的版本 = %q，要登记时读到的链头 SYN-BANK-RECEIPT-1/v1", pinned)
	}

	must(commandFact, reportInput("SYN-FACT-REPORT-1", "SYN-INSTR-1", "60000"), exitRegistered, "REGISTERED")
	must(commandFact, reportInput("SYN-FACT-REPORT-2", "SYN-INSTR-2", "40000"), exitRegistered, "REGISTERED")
	must(commandPosting, moveInput("SYN-POST-R1", "EXTERNAL_SOURCE", "IN_TRANSIT_AT_CHANNEL", "60000",
		"COLLECTION_FACT", "SYN-FACT-REPORT-1"), exitRegistered, "REGISTERED")
	must(commandPosting, moveInput("SYN-POST-R2", "EXTERNAL_SOURCE", "IN_TRANSIT_AT_CHANNEL", "40000",
		"COLLECTION_FACT", "SYN-FACT-REPORT-2"), exitRegistered, "REGISTERED")
	must(commandPosting, moveInput("SYN-POST-MOVE-A", "IN_TRANSIT_AT_CHANNEL", "AWAITING_ALLOCATION", "60000",
		"COLLECTION_FACT", "SYN-FACT-A"), exitRegistered, "REGISTERED")

	// 结算把事实更正到 900.00（照采）：已引用 1000.00 不再被撑住，B 不能移位。
	correctAdoptedFunds(t, pool, "SYN-BANK-RECEIPT-1", "SYN-BANK-RECEIPT-1/v1", "SYN-BANK-RECEIPT-1/v2", 90000)
	must(commandPosting, moveInput("SYN-POST-MOVE-B", "IN_TRANSIT_AT_CHANNEL", "AWAITING_ALLOCATION", "40000",
		"COLLECTION_FACT", "SYN-FACT-B"), exitNeedsReview, "CONTENT_CONFLICT")
	must(commandPosting, moveInput("SYN-POST-MOVE-A", "IN_TRANSIT_AT_CHANNEL", "AWAITING_ALLOCATION", "60000",
		"COLLECTION_FACT", "SYN-FACT-A"), exitRegistered, "EXISTING")
	var awaiting int64
	scalar(t, rig, awaitingAllocationSQL, &awaiting)
	if awaiting != 60000 {
		t.Fatalf("B 被拒之后待清分 = %d，要 60000", awaiting)
	}

	// 更正回 1000.00：B 可以移位；账内各位置之和等于入账之和。
	correctAdoptedFunds(t, pool, "SYN-BANK-RECEIPT-1", "SYN-BANK-RECEIPT-1/v2", "SYN-BANK-RECEIPT-1/v3", 100000)
	must(commandPosting, moveInput("SYN-POST-MOVE-B", "IN_TRANSIT_AT_CHANNEL", "AWAITING_ALLOCATION", "40000",
		"COLLECTION_FACT", "SYN-FACT-B"), exitRegistered, "REGISTERED")
	scalar(t, rig, awaitingAllocationSQL, &awaiting)
	var inTransit, intake int64
	scalar(t, rig, `SELECT COALESCE(SUM(CASE WHEN to_position = 'IN_TRANSIT_AT_CHANNEL' THEN amount_minor ELSE 0 END)
	                       - SUM(CASE WHEN from_position = 'IN_TRANSIT_AT_CHANNEL' THEN amount_minor ELSE 0 END), 0)
	                  FROM collection_remittance.subledger_posting`, &inTransit)
	scalar(t, rig, `SELECT COALESCE(SUM(amount_minor), 0) FROM collection_remittance.subledger_posting
	                 WHERE from_position = 'EXTERNAL_SOURCE'`, &intake)
	if awaiting != 100000 || inTransit != 0 || intake != 100000 || awaiting+inTransit != intake {
		t.Fatalf("终态：待清分 %d、在途 %d、入账 %d，要 100000/0/100000 且账平衡", awaiting, inTransit, intake)
	}
}

const awaitingAllocationSQL = `SELECT COALESCE(SUM(CASE WHEN to_position = 'AWAITING_ALLOCATION' THEN amount_minor ELSE 0 END)
	                                  - SUM(CASE WHEN from_position = 'AWAITING_ALLOCATION' THEN amount_minor ELSE 0 END), 0)
	                              FROM collection_remittance.subledger_posting`

// Covers: AT-CR-026（并发半边）——六路并发各为不同指令登记一条 300.00 的到账，都引用同一条 1000.00 的已采用事实：本上下文
// 的事实锁把它们排成先后，恰好三路登记（合计 900.00）、三路答内容冲突，册上合计从不超过事实金额。没有这把锁，各路读到同一份
// 已有合计、各自判够，六条全落，合计 1800.00。
func TestConcurrentArrivalsOfOneAdoptedFactNeverExceedIt(t *testing.T) {
	rig := newConcurrencyRig(t)
	pool := rig.pool
	if _, code := execute(t.Context(), commandSubledger, []byte(openLedgerInput()), rig.registrar); code != exitRegistered {
		t.Fatal("开账失败")
	}
	const contenders = 6
	for index := 0; index < contenders; index++ {
		ref := "SYN-INSTR-" + string(rune('A'+index))
		if _, code := execute(t.Context(), commandInstruction, []byte(instructionInput(ref, "30000")), rig.registrar); code != exitRegistered {
			t.Fatalf("登记指令 %s 失败", ref)
		}
	}
	seedPrincipalFunds(t, pool, "SYN-BANK-RECEIPT-RACE", "EUR", 100000)

	codes := make([]int, contenders)
	start := make(chan struct{})
	var group sync.WaitGroup
	for index := 0; index < contenders; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			ref := string(rune('A' + index))
			_, codes[index] = execute(context.Background(), commandFact,
				[]byte(arrivalInput("SYN-FACT-"+ref, "SYN-INSTR-"+ref, "SYN-BANK-RECEIPT-RACE", "30000")), rig.registrar)
		}()
	}
	close(start)
	group.Wait()

	registered, conflicts := 0, 0
	for _, code := range codes {
		switch code {
		case exitRegistered:
			registered++
		case exitNeedsReview:
			conflicts++
		}
	}
	var total int64
	scalar(t, rig, `SELECT COALESCE(SUM(amount_minor), 0)::bigint FROM collection_remittance.collection_fact
	                 WHERE source_layer = 'OPERATOR_BANK_CREDIT'`, &total)
	if registered != 3 || conflicts != 3 || total != 90000 {
		t.Fatalf("六路并发退出码 = %v，册上合计 %d；要恰好三路登记、三路内容冲突、合计 90000", codes, total)
	}
}

// Covers: AT-CR-026（并发半边）——同一份到账并发到达两次：先到的登记，后到的在锁后读到先到那条，答已在册而不是内容冲突
// （同一条到账的重放不能被「同一指令已有到账」的门当成第二条）；册上只有一条。
func TestTheSameArrivalArrivingTwiceConcurrentlyIsRegisteredOnce(t *testing.T) {
	rig := newConcurrencyRig(t)
	if _, code := execute(t.Context(), commandSubledger, []byte(openLedgerInput()), rig.registrar); code != exitRegistered {
		t.Fatal("开账失败")
	}
	if _, code := execute(t.Context(), commandInstruction, []byte(instructionInput("SYN-INSTR-1", "60000")), rig.registrar); code != exitRegistered {
		t.Fatal("登记指令失败")
	}
	seedPrincipalFunds(t, rig.pool, "SYN-BANK-RECEIPT-1", "EUR", 100000)

	codes := make([]int, 2)
	messages := make([]string, 2)
	start := make(chan struct{})
	var group sync.WaitGroup
	for index := range codes {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			messages[index], codes[index] = execute(context.Background(), commandFact,
				[]byte(arrivalInput("SYN-FACT-A", "SYN-INSTR-1", "SYN-BANK-RECEIPT-1", "60000")), rig.registrar)
		}()
	}
	close(start)
	group.Wait()

	for index, code := range codes {
		if code != exitRegistered {
			t.Fatalf("第 %d 路退出码 = %d（%s），要 0（登记或已在册）", index, code, messages[index])
		}
	}
	var facts int
	scalar(t, rig, `SELECT count(*) FROM collection_remittance.collection_fact WHERE fact_ref = 'SYN-FACT-A'`, &facts)
	if facts != 1 {
		t.Fatalf("册上 %d 条，要 1", facts)
	}
}

// Covers: AT-CR-026（并发半边）——两路各为不同指令登记 600.00 的到账，引用同一条 1000.00 的已采用事实，容不下两条。屏障连接先占住
// 这条事实在本上下文的事务锁，等看见两路都等在咨询锁上才放——证的是两路确实被锁挡在读合计之前，而不是碰巧先后跑。放开后先拿到
// 锁的登记，另一路读到的合计已含它，答内容冲突；册上合计不超过事实金额。
func TestTwoArrivalsThatCannotBothFitAreQueuedOnTheFundsLock(t *testing.T) {
	rig := newConcurrencyRig(t)
	if _, code := execute(t.Context(), commandSubledger, []byte(openLedgerInput()), rig.registrar); code != exitRegistered {
		t.Fatal("开账失败")
	}
	for _, ref := range []string{"SYN-INSTR-A", "SYN-INSTR-B"} {
		if _, code := execute(t.Context(), commandInstruction, []byte(instructionInput(ref, "60000")), rig.registrar); code != exitRegistered {
			t.Fatalf("登记指令 %s 失败", ref)
		}
	}
	seedPrincipalFunds(t, rig.pool, "SYN-BANK-RECEIPT-RACE", "EUR", 100000)

	barrier := rig.newConn(t)
	watcher := rig.newConn(t)
	tx, err := barrier.Begin(t.Context())
	if err != nil {
		t.Fatalf("开屏障事务：%v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(t.Context(),
		`SELECT pg_advisory_xact_lock(hashtext('collection_remittance.collection_fact.adopted_funds'),
		        hashtext('SYN-T1' || chr(31) || 'SYN-BANK-RECEIPT-RACE'))`,
	); err != nil {
		t.Fatalf("屏障取锁：%v", err)
	}

	codes := make([]int, 2)
	var group sync.WaitGroup
	for index, ref := range []string{"A", "B"} {
		group.Add(1)
		go func() {
			defer group.Done()
			_, codes[index] = execute(context.Background(), commandFact,
				[]byte(arrivalInput("SYN-FACT-"+ref, "SYN-INSTR-"+ref, "SYN-BANK-RECEIPT-RACE", "60000")), rig.registrar)
		}()
	}
	sawBoth := waitForLockWaiters(watcher, "advisory", 2, 10*time.Second)
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("放屏障：%v", err)
	}
	group.Wait()
	if !sawBoth {
		t.Fatal("两路登记没有都等在事实锁上：读合计之前没有锁")
	}

	registered, conflicts := 0, 0
	for _, code := range codes {
		switch code {
		case exitRegistered:
			registered++
		case exitNeedsReview:
			conflicts++
		}
	}
	var total int64
	scalar(t, rig, `SELECT COALESCE(SUM(amount_minor), 0)::bigint FROM collection_remittance.collection_fact
	                 WHERE source_layer = 'OPERATOR_BANK_CREDIT'`, &total)
	if registered != 1 || conflicts != 1 || total != 60000 {
		t.Fatalf("两路登记退出码 = %v，册上合计 %d；要一路 0、一路 2、合计 60000", codes, total)
	}
}
