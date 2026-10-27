package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证认可的协议抵扣的生产装配（隔离合成 S，ADR-0298）：场景十七
// 整条走通、重复到达与撤回重复到达在真库上分得开，两路并发争最后一笔在途款与两路并发
// 撤回同一笔抵扣被分户账锁排先后。

func agreedOffsetInput(offset, instruction, approval, amount string) string {
	return `{
		"tenantId": "SYN-T1", "offsetRef": "` + offset + `", "instructionRef": "` + instruction + `",
		"currency": "EUR", "amountMinor": ` + amount + `, "approvalRef": "` + approval + `",
		"approvedAt": "2026-08-25T00:00:00Z"
	}`
}

func offsetPostingInput(ref, offset, amount string) string {
	return `{
		"tenantId": "SYN-T1", "postingRef": "` + ref + `",
		"fromPosition": "IN_TRANSIT_AT_CHANNEL", "toPosition": "OFFSET_APPLIED",
		"amountMinor": ` + amount + `, "basisKind": "AGREED_OFFSET",
		"basisRef": "` + offset + `", "postedAt": "2026-08-25T01:00:00Z"
	}`
}

func offsetReversalInput(ref, original, amount string) string {
	return `{
		"tenantId": "SYN-T1", "postingRef": "` + ref + `",
		"fromPosition": "OFFSET_APPLIED", "toPosition": "IN_TRANSIT_AT_CHANNEL",
		"amountMinor": ` + amount + `, "basisKind": "CORRECTION",
		"basisRef": "` + original + `", "postedAt": "2026-08-26T01:00:00Z"
	}`
}

// 场景十七：报告 100、到账 96、认可的协议抵扣 3、短款 1。抵扣先于到账移位落账被拦
// （到账优先），移位后落账；重放答已在册，换标识再落答内容冲突；撤回走冲正，同一笔
// 抵扣冲正第二次答内容冲突；撤回的认可不能再落。账面终态各位置之和等于入账 100。
// Covers: AT-CR-017、AT-CR-024、AT-CR-025（生产装配半边）
func TestScenarioSeventeenOffsetOnRealPostgres(t *testing.T) {
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
	run := func(command, raw string, wantCode int, wantAnswer string) {
		t.Helper()
		message, code := execute(ctx, command, []byte(raw), registrar)
		if code != wantCode {
			t.Fatalf("%s 退出码 = %d（%s），要 %d", command, code, message, wantCode)
		}
		if wantAnswer != "" && !strings.Contains(message, wantAnswer) {
			t.Fatalf("%s 答复 = %q，要含 %s", command, message, wantAnswer)
		}
	}
	rig := concurrencyRig{registrar: registrar}
	rig.seedOneReport(t)
	seedPrincipalFunds(t, pool, "SYN-EV-SYN-FACT-CREDIT", "EUR", 96)
	run(commandFact, concurrencyFact("SYN-FACT-CREDIT", "OPERATOR_BANK_CREDIT", "96"), exitRegistered, "REGISTERED")

	// 登记只钉引用：重放答已在册；改金额与换标识再登记同一份认可都是内容冲突；指令不在册
	// 答依据不在册；币种不是那本账的币种被拒。
	offset := agreedOffsetInput("SYN-OFFSET-1", "SYN-INSTR-1", "SYN-SA-APPROVAL-1", "3")
	run(commandAgreedOffset, offset, exitRegistered, "REGISTERED")
	run(commandAgreedOffset, offset, exitRegistered, "EXISTING")
	run(commandAgreedOffset, agreedOffsetInput("SYN-OFFSET-1", "SYN-INSTR-1", "SYN-SA-APPROVAL-1", "4"),
		exitNeedsReview, "CONTENT_CONFLICT")
	run(commandAgreedOffset, agreedOffsetInput("SYN-OFFSET-1B", "SYN-INSTR-1", "SYN-SA-APPROVAL-1", "3"),
		exitNeedsReview, "CONTENT_CONFLICT")
	run(commandAgreedOffset, agreedOffsetInput("SYN-OFFSET-N", "SYN-INSTR-NEVER", "SYN-SA-APPROVAL-N", "3"),
		exitNeedsReview, "BASIS_MISSING")
	run(commandAgreedOffset, strings.Replace(
		agreedOffsetInput("SYN-OFFSET-USD", "SYN-INSTR-1", "SYN-SA-APPROVAL-USD", "3"), `"EUR"`, `"USD"`, 1),
		exitUsage, "NOT_ACCEPTED")

	// 到账 96 已登记而没移位：抵扣等着，不先吃掉在途款。
	run(commandPosting, offsetPostingInput("SYN-POST-OFFSET", "SYN-OFFSET-1", "3"),
		exitUnderfunded, "UNDERFUNDED")
	run(commandPosting, concurrencyPosting("SYN-POST-CREDIT",
		"IN_TRANSIT_AT_CHANNEL", "AWAITING_ALLOCATION", "96", "SYN-FACT-CREDIT"),
		exitRegistered, "REGISTERED")
	run(commandPosting, offsetPostingInput("SYN-POST-OFFSET", "SYN-OFFSET-1", "3"),
		exitRegistered, "REGISTERED")
	run(commandPosting, offsetPostingInput("SYN-POST-OFFSET", "SYN-OFFSET-1", "3"),
		exitRegistered, "EXISTING")
	run(commandPosting, offsetPostingInput("SYN-POST-OFFSET-AGAIN", "SYN-OFFSET-1", "3"),
		exitNeedsReview, "CONTENT_CONFLICT")
	run(commandPosting, offsetPostingInput("SYN-POST-OFFSET-BIG", "SYN-OFFSET-1", "4"),
		exitUsage, "NOT_ACCEPTED")

	// 余下的 1 落短款；落完在途清零。
	run(commandDiscrepancy, `{
		"tenantId": "SYN-T1", "discrepancyRef": "SYN-DIFF-1", "instructionRef": "SYN-INSTR-1",
		"kind": "SHORTFALL", "currency": "EUR", "amountMinor": 1,
		"basisRef": "SYN-CHANNEL-REPORT-1", "observedAt": "2026-08-25T02:00:00Z"
	}`, exitRegistered, "REGISTERED")
	run(commandPosting, `{
		"tenantId": "SYN-T1", "postingRef": "SYN-POST-SHORT",
		"fromPosition": "IN_TRANSIT_AT_CHANNEL", "toPosition": "SHORTFALL",
		"amountMinor": 1, "basisKind": "DISCREPANCY", "basisRef": "SYN-DIFF-1",
		"postedAt": "2026-08-25T03:00:00Z"
	}`, exitRegistered, "REGISTERED")

	view, err := adapter.NewSubledgerBalanceView(db)
	if err != nil {
		t.Fatalf("构造分户账读口：%v", err)
	}
	tenant, err := domain.NewTenantID("SYN-T1")
	if err != nil {
		t.Fatalf("构造租户标识：%v", err)
	}
	balance, opened, err := view.LoadBalance(ctx, tenant, verticalLedgerKey(t))
	if err != nil || !opened {
		t.Fatalf("读回账面：opened=%v err=%v", opened, err)
	}
	wants := map[domain.FundPosition]int64{
		domain.PositionInTransitAtChannel: 0,
		domain.PositionAwaitingAllocation: 96,
		domain.PositionOffsetApplied:      3,
		domain.PositionShortfall:          1,
	}
	var held int64
	for position, want := range wants {
		if got := balance.At(position); got != want {
			t.Fatalf("%s 余额 = %d，要 %d", position, got, want)
		}
		held += balance.At(position)
	}
	if balance.IntakeTotal() != 100 || held != 100 {
		t.Fatalf("分配守恒破了：账内合计 %d，入账 %d，要都是 100", held, balance.IntakeTotal())
	}

	// 撤回：先冲正短款腾出在途，再冲正抵扣；同一笔抵扣冲正第二次、撤回的认可再落，都是
	// 内容冲突，已抵扣不被多拿。
	run(commandPosting, `{
		"tenantId": "SYN-T1", "postingRef": "SYN-POST-SHORT-FIX",
		"fromPosition": "SHORTFALL", "toPosition": "IN_TRANSIT_AT_CHANNEL",
		"amountMinor": 1, "basisKind": "CORRECTION", "basisRef": "SYN-POST-SHORT",
		"postedAt": "2026-08-26T00:00:00Z"
	}`, exitRegistered, "REGISTERED")
	run(commandPosting, offsetReversalInput("SYN-POST-OFFSET-FIX", "SYN-POST-OFFSET", "3"),
		exitRegistered, "REGISTERED")
	run(commandPosting, offsetReversalInput("SYN-POST-OFFSET-FIX", "SYN-POST-OFFSET", "3"),
		exitRegistered, "EXISTING")
	run(commandPosting, offsetReversalInput("SYN-POST-OFFSET-FIX-AGAIN", "SYN-POST-OFFSET", "3"),
		exitNeedsReview, "CONTENT_CONFLICT")
	run(commandPosting, offsetPostingInput("SYN-POST-OFFSET-RETRY", "SYN-OFFSET-1", "3"),
		exitNeedsReview, "CONTENT_CONFLICT")

	balance, _, err = view.LoadBalance(ctx, tenant, verticalLedgerKey(t))
	if err != nil {
		t.Fatalf("读回账面：%v", err)
	}
	if balance.At(domain.PositionOffsetApplied) != 0 || balance.At(domain.PositionInTransitAtChannel) != 4 ||
		balance.At(domain.PositionShortfall) != 0 || balance.At(domain.PositionAwaitingAllocation) != 96 {
		t.Fatalf("撤回后账面：已抵扣 %d 在途 %d 短款 %d 待清分 %d，要 0 / 4 / 0 / 96",
			balance.At(domain.PositionOffsetApplied), balance.At(domain.PositionInTransitAtChannel),
			balance.At(domain.PositionShortfall), balance.At(domain.PositionAwaitingAllocation))
	}
}

// raceBehindTheLock 让两条命令同时撞同一本账：屏障连接先占住分户账的事务锁，等看见两路
// 都等在咨询锁上才放——证的是两路确实被锁挡在读回账面之前，而不是碰巧先后跑。
func raceBehindTheLock(t *testing.T, rig concurrencyRig, first, second string) [2]int {
	t.Helper()
	barrier := rig.newConn(t)
	watcher := rig.newConn(t)
	tx, err := barrier.Begin(t.Context())
	if err != nil {
		t.Fatalf("开屏障事务：%v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(t.Context(),
		`SELECT pg_advisory_xact_lock(hashtext('collection_remittance.subledger_posting.ledger'),
		        hashtext('SYN-T1' || chr(31) || 'SYN-CUST-1' || chr(31) || 'SYN-ENTITY-1' || chr(31) || 'EUR' || chr(31) || 'SYN-CHANNEL-1'))`,
	); err != nil {
		t.Fatalf("屏障取锁：%v", err)
	}

	var codes [2]int
	var group sync.WaitGroup
	for index, raw := range []string{first, second} {
		group.Add(1)
		go func() {
			defer group.Done()
			_, codes[index] = execute(context.Background(), commandPosting, []byte(raw), rig.registrar)
		}()
	}
	sawBoth := waitForLockWaiters(watcher, "advisory", 2, 10*time.Second)
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("放屏障：%v", err)
	}
	group.Wait()
	if !sawBoth {
		t.Fatal("两路记账没有都等在分户账锁上：读回账面之前没有锁")
	}
	return codes
}

func countCodes(codes [2]int) map[int]int {
	counted := map[int]int{}
	for _, code := range codes {
		counted[code]++
	}
	return counted
}

// 同一指令两条认可抵扣各 60、在途只有 100 的一半以上：两路并发落账争最后的在途款，先拿到
// 锁的落账，后到的读到落过的账面答余额不足；已抵扣不超过在途款。
// Covers: AT-CR-025（并发半边）
func TestConcurrentOffsetsOfOneInstructionCannotOverdrawInTransit(t *testing.T) {
	rig := newConcurrencyRig(t)
	rig.seedOneReport(t)
	rig.must(t, commandAgreedOffset, agreedOffsetInput("SYN-OFFSET-A", "SYN-INSTR-1", "SYN-SA-APPROVAL-A", "60"), exitRegistered)
	rig.must(t, commandAgreedOffset, agreedOffsetInput("SYN-OFFSET-B", "SYN-INSTR-1", "SYN-SA-APPROVAL-B", "60"), exitRegistered)

	codes := raceBehindTheLock(t, rig,
		offsetPostingInput("SYN-POST-OFFSET-A", "SYN-OFFSET-A", "60"),
		offsetPostingInput("SYN-POST-OFFSET-B", "SYN-OFFSET-B", "60"))
	if counted := countCodes(codes); counted[exitRegistered] != 1 || counted[exitUnderfunded] != 1 {
		t.Fatalf("两路抵扣退出码 = %v，要一路 0（落账）、一路 4（余额不足）", codes)
	}

	var inTransit, applied int64
	if err := rig.newConn(t).QueryRow(t.Context(),
		`SELECT COALESCE(SUM(CASE WHEN to_position = 'IN_TRANSIT_AT_CHANNEL' THEN amount_minor ELSE 0 END)
		              - SUM(CASE WHEN from_position = 'IN_TRANSIT_AT_CHANNEL' THEN amount_minor ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN to_position = 'OFFSET_APPLIED' THEN amount_minor ELSE 0 END)
		              - SUM(CASE WHEN from_position = 'OFFSET_APPLIED' THEN amount_minor ELSE 0 END), 0)
		   FROM collection_remittance.subledger_posting WHERE tenant_id = 'SYN-T1'`,
	).Scan(&inTransit, &applied); err != nil {
		t.Fatalf("读账面：%v", err)
	}
	if inTransit != 40 || applied != 60 {
		t.Fatalf("渠道在途 %d 已抵扣 %d，要 40 与 60——透支了或两笔都落了", inTransit, applied)
	}
}

// 同一笔抵扣被两条撤回并发冲正（换了记账标识）：已抵扣里再放一份别的指令的抵扣，使余额
// 够扣第二次。锁排先后之后，后到的那一路读到先到的冲正，答内容冲突而不是再从已抵扣里多拿。
// Covers: AT-CR-025（并发半边）
func TestConcurrentReversalsOfOneOffsetPostingCannotTakeTwice(t *testing.T) {
	rig := newConcurrencyRig(t)
	rig.seedOneReport(t)
	rig.must(t, commandInstruction, `{
		"tenantId": "SYN-T1", "instructionRef": "SYN-INSTR-2",
		"parcelRef": "SYN-PARCEL-2", "requirementRef": "SYN-COD-REQ-V1",
		"customerRef": "SYN-CUST-1", "legalEntityRef": "SYN-ENTITY-1",
		"currency": "EUR", "channelRef": "SYN-CHANNEL-1",
		"amountMinor": 100, "instructedAt": "2026-08-24T00:30:00Z"
	}`, exitRegistered)
	rig.must(t, commandFact, `{
		"tenantId": "SYN-T1", "factRef": "SYN-FACT-REPORT-2", "instructionRef": "SYN-INSTR-2",
		"sourceLayer": "CHANNEL_COLLECTION_REPORT", "evidenceRef": "SYN-EV-2",
		"currency": "EUR", "amountMinor": 100, "occurredAt": "2026-08-24T01:00:00Z"
	}`, exitRegistered)
	rig.must(t, commandPosting, concurrencyPosting("SYN-POST-REPORT-2",
		"EXTERNAL_SOURCE", "IN_TRANSIT_AT_CHANNEL", "100", "SYN-FACT-REPORT-2"), exitRegistered)
	for _, instruction := range []string{"1", "2"} {
		rig.must(t, commandAgreedOffset,
			agreedOffsetInput("SYN-OFFSET-"+instruction, "SYN-INSTR-"+instruction, "SYN-SA-APPROVAL-"+instruction, "3"),
			exitRegistered)
		rig.must(t, commandPosting,
			offsetPostingInput("SYN-POST-OFFSET-"+instruction, "SYN-OFFSET-"+instruction, "3"), exitRegistered)
	}

	codes := raceBehindTheLock(t, rig,
		offsetReversalInput("SYN-POST-FIX-A", "SYN-POST-OFFSET-1", "3"),
		offsetReversalInput("SYN-POST-FIX-B", "SYN-POST-OFFSET-1", "3"))
	if counted := countCodes(codes); counted[exitRegistered] != 1 || counted[exitNeedsReview] != 1 {
		t.Fatalf("两路冲正退出码 = %v，要一路 0（落账）、一路 2（内容冲突）——余额够扣也不能冲正两次", codes)
	}

	var applied int64
	if err := rig.newConn(t).QueryRow(t.Context(),
		`SELECT COALESCE(SUM(CASE WHEN to_position = 'OFFSET_APPLIED' THEN amount_minor ELSE 0 END)
		              - SUM(CASE WHEN from_position = 'OFFSET_APPLIED' THEN amount_minor ELSE 0 END), 0)
		   FROM collection_remittance.subledger_posting WHERE tenant_id = 'SYN-T1'`,
	).Scan(&applied); err != nil {
		t.Fatalf("读已抵扣：%v", err)
	}
	if applied != 3 {
		t.Fatalf("已抵扣 = %d，要 3——另一条指令的抵扣被多冲走了", applied)
	}
}

// 同一条命令（同记账标识）并发到达两次：先到的落账，后到的等到锁时读到的是落过的账面，
// 答已在册，而不是在来源被移空的账面上答余额不足。
// Covers: AT-CR-025（并发半边）
func TestTheSameOffsetPostingArrivingTwiceConcurrentlyAnswersExisting(t *testing.T) {
	rig := newConcurrencyRig(t)
	rig.seedOneReport(t)
	rig.must(t, commandAgreedOffset, agreedOffsetInput("SYN-OFFSET-1", "SYN-INSTR-1", "SYN-SA-APPROVAL-1", "100"), exitRegistered)

	same := offsetPostingInput("SYN-POST-OFFSET", "SYN-OFFSET-1", "100")
	codes := raceBehindTheLock(t, rig, same, same)
	if counted := countCodes(codes); counted[exitRegistered] != 2 {
		t.Fatalf("同一条抵扣并发两次的退出码 = %v，要都是 0（一路落账、一路已在册）", codes)
	}
	var postings int
	if err := rig.newConn(t).QueryRow(t.Context(),
		`SELECT count(*) FROM collection_remittance.subledger_posting
		  WHERE tenant_id = 'SYN-T1' AND basis_kind = 'AGREED_OFFSET'`,
	).Scan(&postings); err != nil {
		t.Fatalf("读记账笔数：%v", err)
	}
	if postings != 1 {
		t.Fatalf("抵扣记账 %d 笔，要 1", postings)
	}
}
