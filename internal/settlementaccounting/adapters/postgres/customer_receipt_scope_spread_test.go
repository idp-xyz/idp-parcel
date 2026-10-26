package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件在真实 PostgreSQL 上证一条资金事实映射到几个客户应收作用域时怎样记入入账余额（ADR-0283，隔离合成 S）：只落到
// 一个作用域的照旧全额；落到几个作用域的，每个作用域只记指向它的未撤销核销分配（对账单分配与账户分配），收款没核销的
// 那部分不计入任何作用域，退回没核销的那部分在每个作用域都先扣；映射让事实落进第二个作用域的那一笔事务同时重算它落到
// 的全部作用域。作用域 A 是 acct-1（对账单 stmt-1），作用域 B 是同法人同币种、另一相对方的应收账户 acct-2——同一付款人
// 一笔款付两个账户。

func seedSecondReceivableAccount(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO settlement_accounting.settlement_account
			(tenant_id, account_id, legal_entity_id, counterparty_id, direction, currency,
			 settlement_policy_id, responsibility_basis, registered_at)
		 VALUES ('tenant-receipt', 'acct-2', 'le-1', 'shipper-2', 'RECEIVABLE', 'CNY',
		         'policy-1', 'contract-1', now())`); err != nil {
		t.Fatalf("第二个应收账户：%v", err)
	}
}

// scopeRow 读一个作用域的入账余额与已确认未结；没有登记行时第三值为 false。
func scopeRow(t *testing.T, pool *pgxpool.Pool, account string) (int64, int64, bool) {
	t.Helper()
	var posted, unsettled int64
	err := pool.QueryRow(t.Context(),
		`SELECT posted_minor, unsettled_minor FROM settlement_accounting.operational_balance
		  WHERE tenant_id = 'tenant-receipt' AND account_id = $1`, account).Scan(&posted, &unsettled)
	if err != nil {
		return 0, 0, false
	}
	return posted, unsettled, true
}

func assertScope(t *testing.T, pool *pgxpool.Pool, account string, posted, unsettled int64) {
	t.Helper()
	gotPosted, gotUnsettled, found := scopeRow(t, pool, account)
	if !found {
		t.Fatalf("%s 没有运营余额登记行，要入账 / 已确认未结 = %d / %d", account, posted, unsettled)
	}
	if gotPosted != posted || gotUnsettled != unsettled {
		t.Fatalf("%s 入账 / 已确认未结 = %d / %d，要 %d / %d", account, gotPosted, gotUnsettled, posted, unsettled)
	}
}

// spreadRig 是本文件各用例共用的一套真库编排：每一步一笔事务、时钟每步前移一小时。
type spreadRig struct {
	db      *bentopg.DB
	pool    *pgxpool.Pool
	clock   *movingClock
	handler *application.MapExternalFundsHandler
	tenant  domain.TenantID
}

func newSpreadRig(t *testing.T) spreadRig {
	t.Helper()
	db, pool := receiptPostingDB(t)
	seedCustomerReceiptScope(t, pool)
	seedSecondReceivableAccount(t, pool)
	clock := &movingClock{at: time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)}
	return spreadRig{
		db:    db,
		pool:  pool,
		clock: clock,
		handler: mustFundsHandler(t, mustFundsFacts(t, db), mustFundsMappings(t, db), mustApplications(t, db),
			mustReceiptPostings(t, db), clock),
		tenant: receiptTenant(t),
	}
}

func (rig spreadRig) step(t *testing.T, what string, want application.FundsOutcome,
	run func(context.Context) (application.FundsResult, error)) application.FundsResult {
	t.Helper()
	rig.clock.at = rig.clock.at.Add(time.Hour)
	var result application.FundsResult
	saWithin(t, rig.db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = run(txCtx)
		if err != nil {
			return err
		}
		if result.Outcome() != want {
			t.Errorf("%s = %s，要 %s", what, result.Outcome(), want)
		}
		return nil
	})
	return result
}

func (rig spreadRig) adopt(t *testing.T, fact string, kind domain.FundsFactKind, amount int64) {
	t.Helper()
	rig.step(t, "采用 "+fact, application.FundsFactAdopted, func(txCtx context.Context) (application.FundsResult, error) {
		return rig.handler.AdoptFact(txCtx, receiptFact(rig.tenant, fact, kind, amount, "v1"))
	})
}

func (rig spreadRig) mapTo(t *testing.T, mapping, fact string, kind domain.SettlementTargetKind, target string) application.FundsResult {
	t.Helper()
	return rig.step(t, "映射 "+mapping, application.FundsMapped, func(txCtx context.Context) (application.FundsResult, error) {
		return rig.handler.Map(txCtx, receiptMap(rig.tenant, mapping, fact, kind, target))
	})
}

func (rig spreadRig) apply(t *testing.T, id, fact, mapping string, kind domain.SettlementTargetKind, target string,
	amount int64) application.FundsResult {
	t.Helper()
	return rig.step(t, "核销 "+id, application.SettlementApplied, func(txCtx context.Context) (application.FundsResult, error) {
		return rig.handler.Apply(txCtx, application.ApplySettlementCommand{
			TenantID: rig.tenant, Application: id, Fact: fact, TargetCurrency: "CNY",
			MappingRefs: []string{mapping},
			Allocations: []application.AllocationDirective{{
				Mapping: mapping, TargetKind: kind, Target: target, Direction: domain.AllocationDebit, AmountMinor: amount,
			}},
			Basis: "instruction-1", AppliedAt: rig.clock.at,
		})
	})
}

// assertCatalogued 读资金事实册上这条事实的一行，核它落在几个客户应收作用域上、多少未分配没记进任何作用域。
func (rig spreadRig) assertCatalogued(t *testing.T, fact string, scopes, unscoped int64) {
	t.Helper()
	catalogue, err := adapter.NewFundsApplicationCatalogue(rig.db)
	if err != nil {
		t.Fatalf("构造资金事实册：%v", err)
	}
	rows, err := catalogue.ListExternalFundsFacts(t.Context(), rig.tenant, 50)
	if err != nil {
		t.Fatalf("列资金事实册：%v", err)
	}
	for _, row := range rows {
		if row.Fact != fact {
			continue
		}
		if row.CustomerScopeCount != scopes || row.UnscopedUnappliedMinor != unscoped {
			t.Fatalf("册上 %s 落在 %d 个作用域、未计入任何作用域的未分配 %d，要 %d 与 %d",
				fact, row.CustomerScopeCount, row.UnscopedUnappliedMinor, scopes, unscoped)
		}
		return
	}
	t.Fatalf("资金事实册上没有 %s", fact)
}

func (rig spreadRig) reverse(t *testing.T, id string) application.FundsResult {
	t.Helper()
	return rig.step(t, "撤销 "+id, application.ApplicationReversedOutcome, func(txCtx context.Context) (application.FundsResult, error) {
		return rig.handler.Reverse(txCtx, application.ReverseApplicationCommand{
			TenantID: rig.tenant, Application: id, Basis: "reversal-1", ReversedAt: rig.clock.at,
		})
	})
}

// Covers: 票 code/94 评审手算例与 SA CONTEXT「不同责任法人、结算账户和币种默认不能共用余额」——收款 5000 映到 acct-1 下的
// stmt-1（A），又映到账户 acct-2（B），全部核到 stmt-1：A 入账 0、已确认未结 0，B 入账 0；钱只到了一次，B 不凭空多出 5000。
func TestAReceiptMappedIntoTwoScopesAndAppliedToOneIsNotPostedInTheOther(t *testing.T) {
	rig := newSpreadRig(t)
	seedConfirmedReceivable(t, t.Context(), rig.pool, 5_000)

	rig.adopt(t, "receipt-f2", domain.FundsReceiptConfirmed, 5_000)
	rig.mapTo(t, "map-f2-stmt", "receipt-f2", domain.TargetStatement, "stmt-1")
	rig.mapTo(t, "map-f2-acct", "receipt-f2", domain.TargetAccount, "acct-2")
	rig.apply(t, "app-f2", "receipt-f2", "map-f2-stmt", domain.TargetStatement, "stmt-1", 5_000)

	assertScope(t, rig.pool, "acct-1", 0, 0)
	assertScope(t, rig.pool, "acct-2", 0, 0)
}

// Covers: 单作用域照旧全额、单作用域变多作用域那一笔事务同时重算两个作用域、尚无核销时各记 0、部分核销按分配（对账单分配
// 与账户分配都算指向该作用域）、撤销把那一截还回未分配；读面与答复带出这条事实落在几个作用域上、累计未分配多少。
func TestMappingAReceiptIntoASecondScopeRepostsBothByApplication(t *testing.T) {
	rig := newSpreadRig(t)
	seedConfirmedReceivable(t, t.Context(), rig.pool, 6_000)

	rig.adopt(t, "receipt-1", domain.FundsReceiptConfirmed, 5_000)
	single := rig.mapTo(t, "map-stmt", "receipt-1", domain.TargetStatement, "stmt-1")
	assertScope(t, rig.pool, "acct-1", 5_000, 6_000)
	if posting, known := single.FactScopePosting(); !known || posting.Scopes != 1 || posting.ByApplication() {
		t.Fatalf("单作用域映射的记入方式 = %+v（%v），要 1 个作用域、全额记入", posting, known)
	}
	rig.assertCatalogued(t, "receipt-1", 1, 0)

	spread := rig.mapTo(t, "map-acct-2", "receipt-1", domain.TargetAccount, "acct-2")
	assertScope(t, rig.pool, "acct-1", 0, 6_000)
	assertScope(t, rig.pool, "acct-2", 0, 0)
	posting, known := spread.FactScopePosting()
	if !known || posting.Scopes != 2 || !posting.ByApplication() || posting.Kind != domain.FundsReceiptConfirmed {
		t.Fatalf("第二个作用域映射之后的记入方式 = %+v（%v），要 2 个作用域、按核销记入", posting, known)
	}
	if unapplied, known := spread.FactUnappliedMinor(); !known || unapplied != 5_000 {
		t.Fatalf("映射答复的累计未分配 = %d（%v），要 5000", unapplied, known)
	}
	rig.assertCatalogued(t, "receipt-1", 2, 5_000)

	rig.apply(t, "app-stmt", "receipt-1", "map-stmt", domain.TargetStatement, "stmt-1", 3_000)
	assertScope(t, rig.pool, "acct-1", 0, 3_000)
	assertScope(t, rig.pool, "acct-2", 0, 0)

	toAccount := rig.apply(t, "app-acct", "receipt-1", "map-acct-2", domain.TargetAccount, "acct-2", 1_500)
	assertScope(t, rig.pool, "acct-1", 0, 3_000)
	assertScope(t, rig.pool, "acct-2", 1_500, 0)
	if unapplied, _ := toAccount.FactUnappliedMinor(); unapplied != 500 {
		t.Fatalf("两笔核销之后累计未分配 = %d，要 500", unapplied)
	}
	rig.assertCatalogued(t, "receipt-1", 2, 500)

	rig.reverse(t, "app-acct")
	assertScope(t, rig.pool, "acct-1", 0, 3_000)
	assertScope(t, rig.pool, "acct-2", 0, 0)
	rig.assertCatalogued(t, "receipt-1", 2, 2_000)
}

// Covers: SA CONTEXT「退回与撤销按反向映射离开」在多作用域上的落法（ADR-0283 决定三）——退回 2000 映到 A 与 B：没核销之前
// 两个作用域都先扣 2000（钱从谁头上走的还没分清，哪一边都不放行）；核到账户 acct-2 之后只有 B 扣；撤销那笔核销又回到
// 两边都扣。同一条退回只映到一个作用域时照旧全额离开那一个。
func TestAReturnMappedIntoTwoScopesIsDeductedInEachUntilApplied(t *testing.T) {
	rig := newSpreadRig(t)

	rig.adopt(t, "receipt-a", domain.FundsReceiptConfirmed, 3_000)
	rig.mapTo(t, "map-receipt-a", "receipt-a", domain.TargetStatement, "stmt-1")
	rig.adopt(t, "receipt-b", domain.FundsReceiptConfirmed, 4_000)
	rig.mapTo(t, "map-receipt-b", "receipt-b", domain.TargetAccount, "acct-2")
	assertScope(t, rig.pool, "acct-1", 3_000, 0)
	assertScope(t, rig.pool, "acct-2", 4_000, 0)

	rig.adopt(t, "return-1", domain.FundsReturned, 2_000)
	rig.mapTo(t, "map-return-stmt", "return-1", domain.TargetStatement, "stmt-1")
	assertScope(t, rig.pool, "acct-1", 1_000, 0)
	assertScope(t, rig.pool, "acct-2", 4_000, 0)

	rig.mapTo(t, "map-return-acct", "return-1", domain.TargetAccount, "acct-2")
	assertScope(t, rig.pool, "acct-1", 1_000, 0)
	assertScope(t, rig.pool, "acct-2", 2_000, 0)
	rig.assertCatalogued(t, "return-1", 2, 2_000)

	rig.apply(t, "app-return", "return-1", "map-return-acct", domain.TargetAccount, "acct-2", 2_000)
	assertScope(t, rig.pool, "acct-1", 3_000, 0)
	assertScope(t, rig.pool, "acct-2", 2_000, 0)

	rig.reverse(t, "app-return")
	assertScope(t, rig.pool, "acct-1", 1_000, 0)
	assertScope(t, rig.pool, "acct-2", 2_000, 0)
}
