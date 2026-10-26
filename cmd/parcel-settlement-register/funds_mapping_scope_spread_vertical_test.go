package main

import (
	"strings"
	"testing"
)

// 本文件对真实 PostgreSQL 16 经受控登记口证一条资金事实落到几个客户应收作用域时的记法与锁（ADR-0283，隔离合成 S）：
// 作用域 A 是 SYN-ACCT-RECV-1（对账单 C、已确认应收 10000），作用域 B 是同法人同币种、另一相对方的应收账户
// SYN-ACCT-RECV-2——同一付款人一笔款付两个账户。并发用例照资金侧锁序的做法，用一条自己的事务先占住要排队的那把锁、
// 等两路都在锁上排队后再放开。

const spreadAccount = "SYN-ACCT-RECV-2"

func (rig ceilingRig) seedSecondReceivableAccount(t *testing.T) {
	t.Helper()
	if _, err := rig.pool.Exec(t.Context(),
		`INSERT INTO settlement_accounting.settlement_account
			(tenant_id, account_id, legal_entity_id, counterparty_id, direction, currency,
			 settlement_policy_id, responsibility_basis, registered_at)
		 VALUES ('SYN-T1', 'SYN-ACCT-RECV-2', 'SYN-LE-01', 'SYN-PARTY-SHIPPER-02', 'RECEIVABLE', 'XTS',
		         'SYN-SETTLEMENT-POLICY-1', 'SYN-CONTRACT-01', now())`); err != nil {
		t.Fatalf("登第二个应收账户：%v", err)
	}
}

// postedOf 读一个作用域的入账余额与已确认未结。
func (rig ceilingRig) postedOf(t *testing.T, account string) (int64, int64) {
	t.Helper()
	var posted, unsettled int64
	if err := rig.pool.QueryRow(t.Context(),
		`SELECT posted_minor, unsettled_minor FROM settlement_accounting.operational_balance
		  WHERE tenant_id = $1 AND account_id = $2`, claimTenant, account).Scan(&posted, &unsettled); err != nil {
		t.Fatalf("读 %s 的运营余额：%v", account, err)
	}
	return posted, unsettled
}

func (rig ceilingRig) assertPostedOf(t *testing.T, account string, posted, unsettled int64) {
	t.Helper()
	if gotPosted, gotUnsettled := rig.postedOf(t, account); gotPosted != posted || gotUnsettled != unsettled {
		t.Fatalf("%s 入账 / 已确认未结 = %d / %d，要 %d / %d", account, gotPosted, gotUnsettled, posted, unsettled)
	}
}

func (rig ceilingRig) seedSpreadScopes(t *testing.T) {
	t.Helper()
	rig.seedReceivableScope(t)
	rig.seedSecondReceivableAccount(t)
}

// Covers: 票 code/94 评审手算例走受控登记口——收款 5000 映到对账单 C（A）又映到账户 SYN-ACCT-RECV-2（B），映到 B 那一笔
// 把 A 从全额改为按核销记、两边都是 0，答复说出这条事实落在两个作用域上、5000 未计入任何作用域；全部核到对账单 C 之后
// A 入账 0、已确认未结 5000，B 入账 0，答复说累计未分配 0。
func TestAReceiptMappedIntoTwoScopesThroughTheCommandsIsPostedOnlyWhereApplied(t *testing.T) {
	rig := newCeilingRig(t)
	rig.seedSpreadScopes(t)
	rig.run(t, commandExternalFundsFact, receiptDocument("SYN-FACT-F2", "5000"), exitRegistered, "FUNDS_FACT_ADOPTED")

	single := rig.run(t, commandFundsMapping, claimMappingDocument("SYN-MAPPING-F2-STMT", "SYN-FACT-F2", "STATEMENT", scopeStatement),
		exitRegistered, "FUNDS_MAPPED")
	if strings.Contains(single, "客户应收作用域") {
		t.Fatalf("只落到一个作用域的映射答复 %q 不该说按核销记入", single)
	}
	rig.assertPostedOf(t, scopeAccount, 5000, 10000)

	spread := rig.run(t, commandFundsMapping, claimMappingDocument("SYN-MAPPING-F2-ACCT", "SYN-FACT-F2", "ACCOUNT", spreadAccount),
		exitRegistered, "FUNDS_MAPPED")
	if !strings.Contains(spread, "累计未分配 5000（落在 2 个客户应收作用域上，按核销记入：未分配不计入任何作用域）") {
		t.Fatalf("落到第二个作用域的映射答复 %q 没说出钱停在哪", spread)
	}
	rig.assertPostedOf(t, scopeAccount, 0, 10000)
	rig.assertPostedOf(t, spreadAccount, 0, 0)

	applied := rig.run(t, commandFundsApplication,
		statementApplicationDocument("SYN-APPLICATION-F2", "SYN-FACT-F2", "SYN-MAPPING-F2-STMT", "5000"),
		exitRegistered, "SETTLEMENT_APPLIED")
	if !strings.Contains(applied, "累计未分配 0（落在 2 个客户应收作用域上") {
		t.Fatalf("核销答复 %q 没说事实累计未分配 0 与它落在两个作用域上", applied)
	}
	rig.assertPostedOf(t, scopeAccount, 0, 5000)
	rig.assertPostedOf(t, spreadAccount, 0, 0)
}

// Covers: 两笔映射把同一条事实同时映到 A 与 B——都在事实身份行上排队，后进锁的那一笔看得见先到那一条映射，两个作用域
// 都按核销记（此时各 0）；不排队的话各自只看见自己那一条、各按全额记 5000。
func TestConcurrentMappingsOfOneFactIntoTwoScopesQueueOnTheFact(t *testing.T) {
	rig := newCeilingRig(t)
	rig.seedSpreadScopes(t)
	rig.run(t, commandExternalFundsFact, receiptDocument("SYN-FACT-RACE", "5000"), exitRegistered, "FUNDS_FACT_ADOPTED")

	got := rig.raceUnder(t, lockFactRow("SYN-FACT-RACE"), [2]raceCommand{
		{commandFundsMapping, claimMappingDocument("SYN-MAPPING-RACE-STMT", "SYN-FACT-RACE", "STATEMENT", scopeStatement)},
		{commandFundsMapping, claimMappingDocument("SYN-MAPPING-RACE-ACCT", "SYN-FACT-RACE", "ACCOUNT", spreadAccount)},
	})
	for _, answer := range got {
		if answer.code != exitRegistered || !strings.Contains(answer.message, "FUNDS_MAPPED") {
			t.Fatalf("并发映射有一路没映上：%d %q", answer.code, answer.message)
		}
	}
	rig.assertPostedOf(t, scopeAccount, 0, 10000)
	rig.assertPostedOf(t, spreadAccount, 0, 0)
}

// Covers: 映射与核销同一条事实并发——映到 B 的那一笔与全额核到对账单 C 的那一笔在事实身份行上排队，先后哪一种都落到
// 同一个终值：A 入账 0、已确认未结 5000，B 入账 0。
func TestAMappingAndAnApplicationOfOneFactQueueOnTheFact(t *testing.T) {
	rig := newCeilingRig(t)
	rig.seedSpreadScopes(t)
	rig.run(t, commandExternalFundsFact, receiptDocument("SYN-FACT-MIX", "5000"), exitRegistered, "FUNDS_FACT_ADOPTED")
	rig.run(t, commandFundsMapping, claimMappingDocument("SYN-MAPPING-MIX-STMT", "SYN-FACT-MIX", "STATEMENT", scopeStatement),
		exitRegistered, "FUNDS_MAPPED")

	got := rig.raceUnder(t, lockFactRow("SYN-FACT-MIX"), [2]raceCommand{
		{commandFundsMapping, claimMappingDocument("SYN-MAPPING-MIX-ACCT", "SYN-FACT-MIX", "ACCOUNT", spreadAccount)},
		{commandFundsApplication, statementApplicationDocument("SYN-APPLICATION-MIX", "SYN-FACT-MIX", "SYN-MAPPING-MIX-STMT", "5000")},
	})
	if got[0].code != exitRegistered || !strings.Contains(got[0].message, "FUNDS_MAPPED") ||
		got[1].code != exitRegistered || !strings.Contains(got[1].message, "SETTLEMENT_APPLIED") {
		t.Fatalf("映射与核销并发：%+v，要一路已映射、一路已核销", got)
	}
	rig.assertPostedOf(t, scopeAccount, 0, 5000)
	rig.assertPostedOf(t, spreadAccount, 0, 0)
}

// Covers: 作用域锁按键序取——两条事实各已落在 A、B 之一，再交叉映到另一个：两笔事实锁不同、并发进来，都要锁 A 与 B。
// 先占住 B 的作用域锁让两路都排上队：按键序时一路握 A 等 B、另一路等 A，放开后先后过去；先锁映射目标的话一路握 A 等 B、
// 另一路握 B 等 A，库的死锁检测会拒掉一路。两路都落下之后两条事实都按核销记，A、B 都是 0。
func TestCrossMappingsOfTwoFactsTakeTheScopeLocksInKeyOrder(t *testing.T) {
	rig := newCeilingRig(t)
	rig.seedSpreadScopes(t)
	rig.run(t, commandExternalFundsFact, receiptDocument("SYN-FACT-X1", "3000"), exitRegistered, "FUNDS_FACT_ADOPTED")
	rig.run(t, commandFundsMapping, claimMappingDocument("SYN-MAPPING-X1-STMT", "SYN-FACT-X1", "STATEMENT", scopeStatement),
		exitRegistered, "FUNDS_MAPPED")
	rig.run(t, commandExternalFundsFact, receiptDocument("SYN-FACT-X2", "4000"), exitRegistered, "FUNDS_FACT_ADOPTED")
	rig.run(t, commandFundsMapping, claimMappingDocument("SYN-MAPPING-X2-ACCT", "SYN-FACT-X2", "ACCOUNT", spreadAccount),
		exitRegistered, "FUNDS_MAPPED")
	rig.assertPostedOf(t, scopeAccount, 3000, 10000)
	rig.assertPostedOf(t, spreadAccount, 4000, 0)

	got := rig.raceUnder(t, lockBalanceScope(scopeLegalEntity, spreadAccount, scopeCurrency), [2]raceCommand{
		{commandFundsMapping, claimMappingDocument("SYN-MAPPING-X1-ACCT", "SYN-FACT-X1", "ACCOUNT", spreadAccount)},
		{commandFundsMapping, claimMappingDocument("SYN-MAPPING-X2-STMT", "SYN-FACT-X2", "STATEMENT", scopeStatement)},
	})
	for _, answer := range got {
		if answer.code != exitRegistered || !strings.Contains(answer.message, "FUNDS_MAPPED") {
			t.Fatalf("交叉映射有一路没映上（锁序不定会在这里撞死锁）：%d %q", answer.code, answer.message)
		}
	}
	rig.assertPostedOf(t, scopeAccount, 0, 10000)
	rig.assertPostedOf(t, spreadAccount, 0, 0)
}
