package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 16 证运营余额作用域锁与它护着的几格（隔离合成 S）：并发两笔对账单核销按锁后读到的账判
// 已确认应收够不够；排在锁后的映射用锁后的时刻推进、终值含两笔；并发两份撤销后到的答已撤销；金额本体账不平归冲突格。

const (
	scopeLegalEntity = "SYN-LE-01"
	scopeAccount     = "SYN-ACCT-RECV-1"
	scopeCurrency    = "XTS"
	scopeStatement   = "SYN-STMT-C"
)

// seedReceivableScope 登一个 SYN-T1 的应收作用域：账户、一张对账单 C 与已确认应收 10000。
func (rig ceilingRig) seedReceivableScope(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	for what, statement := range map[string]string{
		"应收账户": `INSERT INTO settlement_accounting.settlement_account
			(tenant_id, account_id, legal_entity_id, counterparty_id, direction, currency,
			 settlement_policy_id, responsibility_basis, registered_at)
		 VALUES ('SYN-T1', 'SYN-ACCT-RECV-1', 'SYN-LE-01', 'SYN-PARTY-SHIPPER-01', 'RECEIVABLE', 'XTS',
		         'SYN-SETTLEMENT-POLICY-1', 'SYN-CONTRACT-01', now())`,
		"对账单 C": `INSERT INTO settlement_accounting.customer_statement
			(tenant_id, statement_number, account_id, period_ref, currency, total_minor,
			 content_digest, lines, adjustment_lines, published_at, recorded_at)
		 VALUES ('SYN-T1', 'SYN-STMT-C', 'SYN-ACCT-RECV-1', 'SYN-PERIOD-1', 'XTS', 10000,
		         'SYN-DIGEST-STMT-C', '[{"charge":"SYN-CHARGE-C-1","amount_minor":10000}]'::jsonb, '[]'::jsonb, now(), now())`,
		"已确认应收": `INSERT INTO settlement_accounting.customer_charge
			(tenant_id, charge_id, fee_item, evaluation_ref, original_currency,
			 original_minor, settlement_currency, settlement_minor,
			 stage, confirmation_basis, formed_at, confirmed_at, recorded_at,
			 responsible_entity, counterparty_ref, charge_direction,
			 settlement_account_id, contract_basis, primary_charging_scope,
			 source_fact_ref)
		 VALUES ('SYN-T1', 'SYN-CHARGE-C-1', 'SYN-FEE-ITEM-FREIGHT-01', 'SYN-EVAL-1', 'XTS', 10000, 'XTS', 10000,
		         'CONFIRMED', 'SYN-BASIS-1', now(), now(), now(),
		         'SYN-LE-01', 'SYN-PARTY-SHIPPER-01', 'RECEIVABLE', 'SYN-ACCT-RECV-1', 'SYN-CONTRACT-01', 'SYN-SCOPE-01',
		         'SYN-SOURCE-FACT-1')`,
	} {
		if _, err := rig.pool.Exec(ctx, statement); err != nil {
			t.Fatalf("登%s：%v", what, err)
		}
	}
}

func (rig ceilingRig) postedAndUnsettled(t *testing.T) (int64, int64) {
	t.Helper()
	var posted, unsettled int64
	if err := rig.pool.QueryRow(t.Context(),
		`SELECT posted_minor, unsettled_minor FROM settlement_accounting.operational_balance
		  WHERE tenant_id = $1 AND account_id = $2`, claimTenant, scopeAccount).Scan(&posted, &unsettled); err != nil {
		t.Fatalf("读运营余额：%v", err)
	}
	return posted, unsettled
}

func statementApplicationDocument(application, fact, mapping, amountMinor string) string {
	return applicationDocument(application, fact, mapping, "STATEMENT", scopeStatement, amountMinor)
}

// Covers: 作用域锁护住对账单分配的落行前判——两笔核销（两条事实）各核对账单 C 已确认应收 10000 的 60%，同时到达：
// 后进锁的那一路按锁后的账读到先到那笔，答`未受理`退 1；作用域的已确认未结停在 4000。
func TestConcurrentStatementApplicationsStayWithinTheConfirmedReceivable(t *testing.T) {
	rig := newCeilingRig(t)
	rig.seedReceivableScope(t)
	for _, fact := range []string{"SYN-FACT-S1", "SYN-FACT-S2"} {
		rig.run(t, commandExternalFundsFact, receiptDocument(fact, "6000"), exitRegistered, "FUNDS_FACT_ADOPTED")
		rig.run(t, commandFundsMapping, claimMappingDocument("SYN-MAPPING-"+fact, fact, "STATEMENT", scopeStatement),
			exitRegistered, "FUNDS_MAPPED")
	}

	got := rig.raceUnder(t, lockBalanceScope(scopeLegalEntity, scopeAccount, scopeCurrency), [2]raceCommand{
		{commandFundsApplication, statementApplicationDocument("SYN-APPLICATION-S1", "SYN-FACT-S1", "SYN-MAPPING-SYN-FACT-S1", "6000")},
		{commandFundsApplication, statementApplicationDocument("SYN-APPLICATION-S2", "SYN-FACT-S2", "SYN-MAPPING-SYN-FACT-S2", "6000")},
	})
	applied, refused := 0, 0
	for _, answer := range got {
		switch {
		case answer.code == exitRegistered && strings.Contains(answer.message, "SETTLEMENT_APPLIED"):
			applied++
		case answer.code == exitUsage && strings.Contains(answer.message, "SOURCE_NOT_ACCEPTED"):
			refused++
		default:
			t.Fatalf("意外的答复：%d %q", answer.code, answer.message)
		}
	}
	if applied != 1 || refused != 1 {
		t.Fatalf("已核销 %d 路、未受理 %d 路，要各 1（%+v）", applied, refused, got)
	}
	if posted, unsettled := rig.postedAndUnsettled(t); posted != 6000 || unsettled != 4000 {
		t.Fatalf("入账 / 已确认未结 = %d / %d，要 6000 / 4000", posted, unsettled)
	}
}

// Covers: 锁后取时刻——一笔映射排在作用域锁后面，占锁那笔事务以更晚的时刻推进了另一笔映射并提交；排队那一笔拿到锁之后
// 用锁后的时刻推进，答已映射，终值含两笔。时刻若在锁前取，它比占锁那笔早，「同键只准往前」会把这一次吞掉。
func TestAMappingQueuedBehindTheScopeLockPostsBothReceipts(t *testing.T) {
	rig := newCeilingRig(t)
	rig.seedReceivableScope(t)
	rig.run(t, commandExternalFundsFact, receiptDocument("SYN-FACT-Q1", "3000"), exitRegistered, "FUNDS_FACT_ADOPTED")
	rig.run(t, commandExternalFundsFact, receiptDocument("SYN-FACT-Q2", "4000"), exitRegistered, "FUNDS_FACT_ADOPTED")
	held, err := registrationjson.FundsMappingFromJSON([]byte(
		claimMappingDocument("SYN-MAPPING-Q2", "SYN-FACT-Q2", "STATEMENT", scopeStatement)))
	if err != nil {
		t.Fatalf("译装占锁那一笔映射：%v", err)
	}
	postings, err := sapostgres.NewCustomerReceiptPostings(rig.db)
	if err != nil {
		t.Fatalf("构造重算口：%v", err)
	}
	scope, err := domain.NewSettlementScope(
		mustBuild(t, domain.NewLegalEntityReference, scopeLegalEntity),
		mustBuild(t, domain.NewSettlementAccountID, scopeAccount),
		mustBuild(t, domain.NewCurrencyCode, scopeCurrency))
	if err != nil {
		t.Fatalf("作用域：%v", err)
	}
	tenant := mustBuild(t, domain.NewTenantID, claimTenant)
	ctx := t.Context()

	locked := make(chan struct{})
	proceed := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(proceed) }) }
	t.Cleanup(release)
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- rig.db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			if err := postings.LockOperationalBalanceScope(txCtx, tenant, scope); err != nil {
				return err
			}
			close(locked)
			<-proceed
			result, err := rig.registrar.funds.Map(txCtx, held)
			if err != nil {
				return err
			}
			if result.Outcome() != application.FundsMapped {
				return fmt.Errorf("占锁那一笔映射答 %s", result.Outcome())
			}
			return nil
		})
	}()
	select {
	case <-locked:
	case err := <-holderDone:
		t.Fatalf("占锁事务没锁上就收了：%v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("占锁事务十秒内没锁上")
	}

	queued := make(chan commandAnswer, 1)
	go func() {
		message, code := execute(ctx, commandFundsMapping,
			[]byte(claimMappingDocument("SYN-MAPPING-Q1", "SYN-FACT-Q1", "STATEMENT", scopeStatement)), rig.registrar)
		queued <- commandAnswer{message: message, code: code}
	}()
	rig.awaitWaiters(t, 1)
	release()
	if err := <-holderDone; err != nil {
		t.Fatalf("占锁事务：%v", err)
	}
	select {
	case answer := <-queued:
		if answer.code != exitRegistered || !strings.Contains(answer.message, "FUNDS_MAPPED") {
			t.Fatalf("排队那一笔映射：%d %q，要退 0 答已映射", answer.code, answer.message)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("放开之后十秒内排队那一笔没答")
	}
	if posted, _ := rig.postedAndUnsettled(t); posted != 7000 {
		t.Fatalf("入账余额 = %d，要两笔合计 7000", posted)
	}
}

// Covers: 并发两份撤销同一笔核销——后到的那份在事实锁后再读一次核销，读到的已是撤销版，答已撤销、退 0，与顺序重放同一
// 结果；不因改写撤销两列落空而答`未受理`。
func TestConcurrentReversalsOfOneApplicationAnswerAlreadyReversed(t *testing.T) {
	rig := newCeilingRig(t)
	rig.seedFact(t)
	rig.run(t, commandFundsApplication, payableApplication("SYN-APPLICATION-REV", "5000"), exitRegistered, "SETTLEMENT_APPLIED")

	document := reversalDocument("SYN-APPLICATION-REV")
	got := rig.raceUnder(t, lockFactRow("SYN-FACT-CEILING-1"),
		[2]raceCommand{{commandFundsApplicationReversal, document}, {commandFundsApplicationReversal, document}})
	answers := map[string]int{}
	for _, answer := range got {
		if answer.code != exitRegistered {
			t.Fatalf("并发撤销有一路没退 0：%d %q", answer.code, answer.message)
		}
		for _, name := range []string{"APPLICATION_REVERSED", "ALREADY_REVERSED"} {
			if strings.Contains(answer.message, name) {
				answers[name]++
			}
		}
	}
	if answers["APPLICATION_REVERSED"] != 1 || answers["ALREADY_REVERSED"] != 1 {
		t.Fatalf("并发撤销的两路 = %+v，要一路已撤销、一路撤销重放", got)
	}
}

// Covers: ADR-0029 在金额本体账不平那一格——那一条的已核销已越过它的金额本体（这里直接在库里垫一笔越额的核销），再交
// 一笔核销整笔回滚，与作用域已确认未结为负同归冲突格 2：续办都是核对指向它的核销、撤掉多出那一笔，不是重试。
func TestAClaimTargetAppliedBeyondItsAmountExitsAsConflict(t *testing.T) {
	rig := newCeilingRig(t)
	seedClaimAmounts(t, rig.db)
	rig.run(t, commandExternalFundsFact, receiptDocument("SYN-FACT-COMP-9", "8000"), exitRegistered, "FUNDS_FACT_ADOPTED")
	rig.run(t, commandFundsMapping, claimMappingDocument("SYN-MAPPING-COMP-9", "SYN-FACT-COMP-9", "CUSTOMER_COMPENSATION", "SYN-COMP-1"),
		exitRegistered, "FUNDS_MAPPED")
	if _, err := rig.pool.Exec(t.Context(),
		`INSERT INTO settlement_accounting.settlement_application
			(tenant_id, application_id, fact_id, currency, fact_minor, applied_minor,
			 allocations, basis, applied_at, content_digest, recorded_at)
		 VALUES ('SYN-T1', 'SYN-APPLICATION-LEGACY', 'SYN-FACT-LEGACY', 'XTS', 9000, 6000,
		         '[{"mapping":"SYN-MAPPING-LEGACY","targetKind":"CUSTOMER_COMPENSATION","target":"SYN-COMP-1","direction":"DEBIT","amountMinor":6000}]'::jsonb,
		         'SYN-BASIS-LEGACY', now(), 'SYN-DIGEST-LEGACY', now())`); err != nil {
		t.Fatalf("垫越额核销：%v", err)
	}
	message := rig.run(t, commandFundsApplication,
		applicationDocument("SYN-APPLICATION-COMP-9", "SYN-FACT-COMP-9", "SYN-MAPPING-COMP-9", "CUSTOMER_COMPENSATION", "SYN-COMP-1", "1"),
		exitConflict, "账不平")
	if strings.Contains(message, "重跑") {
		t.Fatalf("账不平的答复 %q 叫人重跑", message)
	}
	if rows := countRows(t, rig.pool,
		`SELECT count(*) FROM settlement_accounting.settlement_application WHERE tenant_id = $1 AND application_id = $2`,
		claimTenant, "SYN-APPLICATION-COMP-9"); rows != 0 {
		t.Fatalf("账不平时落了 %d 行核销", rows)
	}
}
