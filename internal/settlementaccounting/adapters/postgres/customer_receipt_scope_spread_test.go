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

// 本文件在真实 PostgreSQL 上证一条资金事实怎样记入客户应收作用域的入账余额（ADR-0283，隔离合成 S）：每个作用域取这条
// 事实在它身上可能的最小记入——指向它的未撤销核销分配（对账单分配与账户分配）；收款只落到一个作用域时再加没核销的
// 部分（只有客户映射时就是全额，核到应付上的那部分离开）；退回没核销的部分在每个作用域都先扣。一条收款从一个作用域
// 变成两个的那一笔映射同时重算原作用域与新作用域。作用域 A 是 acct-1（对账单 stmt-1），作用域 B 是同法人同币种、
// 另一相对方的应收账户 acct-2——同一付款人一笔款付两个账户。

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

// Covers: 只有客户映射的单作用域全额、单作用域变多作用域那一笔事务同时重算两个作用域、尚无核销时各记 0、部分核销按分配（对账单分配
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
// 两边都扣。同一条退回只映到一个作用域时全额离开那一个。
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

// Covers: ADR-0283「币种与事实不同的作用域也算一个」——CNY 收款 5000 映到 stmt-1（A）又映到 USD 应收账户：A 从全额改为
// 按核销记（0），USD 账户从这条 CNY 事实上什么都记不到（0）；资金事实册上这条事实落在 2 个作用域上、5000 未计入任何
// 作用域。只数同币种作用域的话 A 会停在 5000。
func TestAScopeInAnotherCurrencyStillCountsAsAScope(t *testing.T) {
	rig := newSpreadRig(t)
	if _, err := rig.pool.Exec(t.Context(),
		`INSERT INTO settlement_accounting.settlement_account
			(tenant_id, account_id, legal_entity_id, counterparty_id, direction, currency,
			 settlement_policy_id, responsibility_basis, registered_at)
		 VALUES ('tenant-receipt', 'acct-usd', 'le-1', 'shipper-3', 'RECEIVABLE', 'USD',
		         'policy-1', 'contract-1', now())`); err != nil {
		t.Fatalf("USD 应收账户：%v", err)
	}

	rig.adopt(t, "receipt-cny", domain.FundsReceiptConfirmed, 5_000)
	rig.mapTo(t, "map-cny-stmt", "receipt-cny", domain.TargetStatement, "stmt-1")
	assertScope(t, rig.pool, "acct-1", 5_000, 0)

	rig.mapTo(t, "map-cny-usd", "receipt-cny", domain.TargetAccount, "acct-usd")
	assertScope(t, rig.pool, "acct-1", 0, 0)
	assertScope(t, rig.pool, "acct-usd", 0, 0)
	rig.assertCatalogued(t, "receipt-cny", 2, 5_000)
}

// Covers: ADR-0283 决定二在非客户目标上的落法——收款 5000 映到 stmt-1（A）与审核应付 payable-x：A 是这条事实唯一的客户
// 应收作用域，没核销的部分推定归它，入账 5000；核 2000 到应付，那一截离开 A（3000）；再核 3000 到应付，A 归 0；撤销
// 后一笔，那一截回到没核销的部分、回到 A。资金事实册上它只落在 1 个作用域上，没有未计入任何作用域的未分配。
func TestAReceiptAppliedToANonCustomerTargetLeavesItsOnlyCustomerScope(t *testing.T) {
	rig := newSpreadRig(t)

	rig.adopt(t, "receipt-x", domain.FundsReceiptConfirmed, 5_000)
	rig.mapTo(t, "map-x-stmt", "receipt-x", domain.TargetStatement, "stmt-1")
	rig.mapTo(t, "map-x-payable", "receipt-x", domain.TargetPayable, "payable-x")
	assertScope(t, rig.pool, "acct-1", 5_000, 0)
	rig.assertCatalogued(t, "receipt-x", 1, 0)

	rig.apply(t, "app-x-1", "receipt-x", "map-x-payable", domain.TargetPayable, "payable-x", 2_000)
	assertScope(t, rig.pool, "acct-1", 3_000, 0)
	rig.apply(t, "app-x-2", "receipt-x", "map-x-payable", domain.TargetPayable, "payable-x", 3_000)
	assertScope(t, rig.pool, "acct-1", 0, 0)

	rig.reverse(t, "app-x-2")
	assertScope(t, rig.pool, "acct-1", 3_000, 0)
	rig.assertCatalogued(t, "receipt-x", 1, 0)
}

// correctAmount 把一条已采用的事实更正为新金额（回指当前链头 v1）。
func (rig spreadRig) correctAmount(t *testing.T, fact string, amount int64) {
	t.Helper()
	rig.step(t, "更正 "+fact, application.FundsFactAdopted, func(txCtx context.Context) (application.FundsResult, error) {
		return rig.handler.CorrectFact(txCtx, application.CorrectFundsFactCommand{
			TenantID: rig.tenant, Fact: fact, Corrects: "v1", Version: "v2",
			AmountMinor: amount, CorrectedAt: rig.clock.at,
		})
	})
}

// postedNet 直接读 acct-1 作用域的收款 P。
func (rig spreadRig) postedNet(t *testing.T) int64 {
	t.Helper()
	scope, err := domain.NewSettlementScope(
		saValue(t, domain.NewLegalEntityReference, "le-1"),
		saValue(t, domain.NewSettlementAccountID, "acct-1"),
		saValue(t, domain.NewCurrencyCode, "CNY"))
	if err != nil {
		t.Fatalf("作用域：%v", err)
	}
	var posted int64
	saWithin(t, rig.db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		posted, _, _, err = mustReceiptPostings(t, rig.db).CustomerDirectionNets(txCtx, rig.tenant, scope)
		return err
	})
	return posted
}

// Covers: 票 code/92 超核情形——外部更正把事实金额改到已核销合计以下时，单作用域的记入仍封顶到事实金额（LEAST），
// 防一个作用域记入多于钱：收款 5000 核销 5000 到 stmt-1 后更正为 3000，P 是 3000 而非核销合计 5000；
// 退回同形，P 是 -3000。
func TestACorrectionBelowTheAppliedTotalCapsTheScopePostingAtTheFactAmount(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind domain.FundsFactKind
		want int64
	}{
		{"receipt", domain.FundsReceiptConfirmed, 3_000},
		{"return", domain.FundsReturned, -3_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := newSpreadRig(t)
			seedConfirmedReceivable(t, t.Context(), rig.pool, 5_000)

			rig.adopt(t, "fact-cap", tc.kind, 5_000)
			rig.mapTo(t, "map-cap", "fact-cap", domain.TargetStatement, "stmt-1")
			rig.apply(t, "app-cap", "fact-cap", "map-cap", domain.TargetStatement, "stmt-1", 5_000)
			rig.correctAmount(t, "fact-cap", 3_000)

			if got := rig.postedNet(t); got != tc.want {
				t.Fatalf("更正到 3000 后 acct-1 的 P = %d，要 %d", got, tc.want)
			}
		})
	}
}
