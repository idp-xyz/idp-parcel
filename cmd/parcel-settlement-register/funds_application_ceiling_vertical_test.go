package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证一笔资金事实的核销合计不超过它当前有效那一版的金额（隔离合成 S）：顺序交、并发交都只
// 让合计停在事实金额以内，超出的那一笔答分配失衡、什么都不落；并发两份同键答一份已核销、一份已存在。并发用例用一条
// 自己的事务先占住事实身份行，让两路都在锁上排队后再放开——排队的那一刻由 pg_stat_activity 认，不靠睡眠。

type ceilingRig struct {
	pool      *pgxpool.Pool
	db        *bentopg.DB
	registrar registrar
}

// newCeilingRig 另开一只宽池：pgtest 的池只有两条连接，占锁的事务、两路命令与看锁的那一句要四条。
func newCeilingRig(t *testing.T) ceilingRig {
	t.Helper()
	base := pgtest.Pool(t)
	config := base.Config().Copy()
	config.MaxConns = 8
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatalf("开宽池：%v", err)
	}
	t.Cleanup(pool.Close)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registrar, err := buildRegistrar(db)
	if err != nil {
		t.Fatalf("装配登记口：%v", err)
	}
	return ceilingRig{pool: pool, db: db, registrar: registrar}
}

func (rig ceilingRig) run(t *testing.T, command, raw string, wantCode int, wantAnswer string) string {
	t.Helper()
	message, code := execute(t.Context(), command, []byte(raw), rig.registrar)
	if code != wantCode || !strings.Contains(message, wantAnswer) {
		t.Fatalf("%s 退出码 = %d、答复 = %q，要 %d 与 %s", command, code, message, wantCode, wantAnswer)
	}
	return message
}

// payableApplication 是对 SYN-FACT-CEILING-1 的一笔核销，全额分给同一张审核应付——应付这一格没有按目标的上限，
// 拦得住的只有事实那一级。
func payableApplication(application, amountMinor string) string {
	return applicationDocument(application, "SYN-FACT-CEILING-1", "SYN-MAPPING-PAYABLE-1", "PAYABLE", "SYN-PAYABLE-1", amountMinor)
}

func (rig ceilingRig) seedFact(t *testing.T) {
	t.Helper()
	rig.run(t, commandExternalFundsFact, receiptDocument("SYN-FACT-CEILING-1", "5000"), exitRegistered, "FUNDS_FACT_ADOPTED")
	rig.run(t, commandFundsMapping, claimMappingDocument("SYN-MAPPING-PAYABLE-1", "SYN-FACT-CEILING-1", "PAYABLE", "SYN-PAYABLE-1"),
		exitRegistered, "FUNDS_MAPPED")
}

func (rig ceilingRig) appliedTotal(t *testing.T) int64 {
	t.Helper()
	var total int64
	if err := rig.pool.QueryRow(t.Context(),
		`SELECT COALESCE(SUM(applied_minor), 0) FROM settlement_accounting.settlement_application
		  WHERE tenant_id = $1 AND fact_id = $2 AND reversal_basis IS NULL`,
		claimTenant, "SYN-FACT-CEILING-1").Scan(&total); err != nil {
		t.Fatalf("合计已核销：%v", err)
	}
	return total
}

// Covers: UC-SA-005 金额守恒与 `AT-SA-109`「付款金额超出明确范围 → 只核销合格范围，超额保持未分配」在事实这一级——
// 5000 的收款换三个核销引用各交 5000，只有第一笔落下，其余答分配失衡；答复里的未分配是这一步之后该事实的累计未分配；
// 撤销之后答复不再把原核销金额说成在核，额度回来、可以再核。
func TestAFundsFactIsNotAppliedBeyondItsAmountAcrossApplications(t *testing.T) {
	rig := newCeilingRig(t)
	rig.seedFact(t)

	first := rig.run(t, commandFundsApplication, payableApplication("SYN-APPLICATION-C1", "5000"), exitRegistered, "SETTLEMENT_APPLIED")
	if !strings.Contains(first, "累计未分配 0") {
		t.Fatalf("核满之后的答复 %q 没说事实累计未分配 0", first)
	}
	rig.run(t, commandFundsApplication, payableApplication("SYN-APPLICATION-C2", "5000"), exitUsage, "APPLICATION_IMBALANCE")
	rig.run(t, commandFundsApplication, payableApplication("SYN-APPLICATION-C3", "1"), exitUsage, "APPLICATION_IMBALANCE")
	if total := rig.appliedTotal(t); total != 5000 {
		t.Fatalf("事实上的已核销合计 = %d，要 5000", total)
	}
	replay := rig.run(t, commandFundsApplication, payableApplication("SYN-APPLICATION-C1", "5000"), exitRegistered, "EXISTING_APPLICATION")
	if !strings.Contains(replay, "累计未分配 0") {
		t.Fatalf("重放答复 %q 没说事实累计未分配 0", replay)
	}

	reversed := rig.run(t, commandFundsApplicationReversal, reversalDocument("SYN-APPLICATION-C1"), exitRegistered, "APPLICATION_REVERSED")
	if !strings.Contains(reversed, "累计未分配 5000") || strings.Contains(reversed, "核销 5000") {
		t.Fatalf("撤销答复 %q 要说事实累计未分配 5000，且不再把原核销金额说成在核", reversed)
	}
	rig.run(t, commandFundsApplication, payableApplication("SYN-APPLICATION-C2", "5000"), exitRegistered, "SETTLEMENT_APPLIED")
	if total := rig.appliedTotal(t); total != 5000 {
		t.Fatalf("撤销后再核的已核销合计 = %d，要 5000", total)
	}
}

type commandAnswer struct {
	message string
	code    int
}

// raceTwo 先用一条自己的事务锁住事实身份行，再让两路核销并发跑；两路都在锁上排队之后才放开，于是它们一定先后进锁。
func (rig ceilingRig) raceTwo(t *testing.T, first, second string) [2]commandAnswer {
	t.Helper()
	return rig.raceUnder(t, lockFactRow("SYN-FACT-CEILING-1"),
		[2]raceCommand{{commandFundsApplication, first}, {commandFundsApplication, second}})
}

type raceCommand struct {
	command string
	raw     string
}

// holderLock 是占锁事务里执行的那一句：拿住两路要排队的那把锁。
type holderLock struct {
	sql  string
	args []any
}

func lockFactRow(fact string) holderLock {
	return holderLock{
		sql:  `SELECT 1 FROM settlement_accounting.external_funds_fact WHERE tenant_id = $1 AND fact_id = $2 FOR UPDATE`,
		args: []any{claimTenant, fact},
	}
}

// lockBalanceScope 与 CustomerReceiptPostings.LockOperationalBalanceScope 取同一把作用域咨询锁。
func lockBalanceScope(legalEntity, account, currency string) holderLock {
	return holderLock{
		sql: `SELECT pg_advisory_xact_lock(hashtext('settlement_accounting.operational_balance.scope'),
		        hashtext($1 || chr(31) || $2 || chr(31) || $3 || chr(31) || $4))`,
		args: []any{claimTenant, legalEntity, account, currency},
	}
}

// raceUnder 用一条自己的事务先拿住 lock，再让两路命令并发跑，等两路都在锁上排队（由 pg_stat_activity 认）才提交放开，
// 于是两路一定先后进锁。断言失败时占锁事务回滚，排队的两路照样放出来。
func (rig ceilingRig) raceUnder(t *testing.T, lock holderLock, commands [2]raceCommand) [2]commandAnswer {
	t.Helper()
	ctx := t.Context()
	holder, err := rig.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("开占锁事务：%v", err)
	}
	defer func() { _ = holder.Rollback(context.Background()) }()
	if _, err := holder.Exec(ctx, lock.sql, lock.args...); err != nil {
		t.Fatalf("占锁：%v", err)
	}

	answers := make(chan [2]any, 2)
	for index, command := range commands {
		go func(index int, command raceCommand) {
			message, code := execute(ctx, command.command, []byte(command.raw), rig.registrar)
			answers <- [2]any{index, commandAnswer{message: message, code: code}}
		}(index, command)
	}
	rig.awaitWaiters(t, 2)
	if err := holder.Commit(ctx); err != nil {
		t.Fatalf("放开占锁事务：%v", err)
	}

	var got [2]commandAnswer
	for range 2 {
		select {
		case answer := <-answers:
			got[answer[0].(int)] = answer[1].(commandAnswer)
		case <-time.After(10 * time.Second):
			t.Fatal("放开之后十秒内两路没有都答")
		}
	}
	return got
}

// awaitWaiters 等到本库至少有 want 路在锁上排队；十秒内等不到就是要证的那把锁没有取。
func (rig ceilingRig) awaitWaiters(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := rig.pool.QueryRow(t.Context(),
			`SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND datname = current_database()`,
		).Scan(&waiting); err != nil {
			t.Fatalf("看锁：%v", err)
		}
		if waiting >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("十秒内只有 %d 路在锁上排队，要 %d 路——要证的那把锁没有取", waiting, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Covers: 并发超额——同一笔 5000 的收款，两路各交一份 5000 的核销（不同引用）同时到达：后进锁的那一路读到先到那笔的
// 已核销，答分配失衡退 1，合计停在 5000。
func TestConcurrentApplicationsOfOneFactStayWithinItsAmount(t *testing.T) {
	rig := newCeilingRig(t)
	rig.seedFact(t)

	got := rig.raceTwo(t, payableApplication("SYN-APPLICATION-R1", "5000"), payableApplication("SYN-APPLICATION-R2", "5000"))
	applied, imbalanced := 0, 0
	for _, answer := range got {
		switch {
		case answer.code == exitRegistered && strings.Contains(answer.message, "SETTLEMENT_APPLIED"):
			applied++
		case answer.code == exitUsage && strings.Contains(answer.message, "APPLICATION_IMBALANCE"):
			imbalanced++
		default:
			t.Fatalf("意外的答复：%d %q", answer.code, answer.message)
		}
	}
	if applied != 1 || imbalanced != 1 {
		t.Fatalf("已核销 %d 路、失衡 %d 路，要各 1（%+v）", applied, imbalanced, got)
	}
	if total := rig.appliedTotal(t); total != 5000 {
		t.Fatalf("并发之后已核销合计 = %d，要 5000", total)
	}
}

// Covers: 并发同键——同一份核销两路同时到达，与顺序重放同一结果：一路已核销、一路已存在，都退 0，只落一行；不被事实
// 上限判成失衡。
func TestConcurrentReplaysOfOneApplicationAnswerExistingNotImbalance(t *testing.T) {
	rig := newCeilingRig(t)
	rig.seedFact(t)

	document := payableApplication("SYN-APPLICATION-SAME", "5000")
	got := rig.raceTwo(t, document, document)
	answers := map[string]int{}
	for _, answer := range got {
		if answer.code != exitRegistered {
			t.Fatalf("同键并发有一路没退 0：%d %q", answer.code, answer.message)
		}
		for _, name := range []string{"SETTLEMENT_APPLIED", "EXISTING_APPLICATION"} {
			if strings.Contains(answer.message, name) {
				answers[name]++
			}
		}
	}
	if answers["SETTLEMENT_APPLIED"] != 1 || answers["EXISTING_APPLICATION"] != 1 {
		t.Fatalf("同键并发的两路 = %+v，要一路已核销、一路已存在", got)
	}
	if rows := countRows(t, rig.pool,
		`SELECT count(*) FROM settlement_accounting.settlement_application WHERE tenant_id = $1`, claimTenant); rows != 1 {
		t.Fatalf("同键并发落了 %d 行", rows)
	}
}
