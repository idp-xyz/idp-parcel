package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件在真实 PostgreSQL 上证核销与核销撤销在各自的事务里重算入账余额与已确认未结（ADR-0181 决定一，隔离合成 S）：
// 指向对账单的核销分配一落，入账余额与已确认未结等额离开；撤销把它们等额还回来；会让已确认未结为负的那一笔在落行
// 之前拒成`未受理`，什么都不落。两把新锁（事实身份行、运营余额作用域）无事务即拒。

func seedConfirmedReceivable(t *testing.T, ctx context.Context, pool *pgxpool.Pool, amountMinor int64) {
	t.Helper()
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.customer_charge
			(tenant_id, charge_id, fee_item, evaluation_ref, original_currency,
			 original_minor, settlement_currency, settlement_minor,
			 stage, confirmation_basis, formed_at, confirmed_at, recorded_at,
			 responsible_entity, counterparty_ref, charge_direction,
			 settlement_account_id, contract_basis, primary_charging_scope,
			 source_fact_ref)
		 VALUES ('tenant-receipt', 'charge-1', 'BASE_FREIGHT', 'eval-1', 'CNY', $1, 'CNY', $1,
		         'CONFIRMED', 'basis-1', now(), now(), now(),
		         'le-1', 'shipper-1', 'RECEIVABLE', 'acct-1', 'contract-1', 'scope-1', 'fact-1')`, amountMinor); err != nil {
		t.Fatalf("已确认应收：%v", err)
	}
}

func statementApplication(tenant domain.TenantID, id string, amount int64) application.ApplySettlementCommand {
	return application.ApplySettlementCommand{
		TenantID:       tenant,
		Application:    id,
		Fact:           "receipt-1",
		TargetCurrency: "CNY",
		MappingRefs:    []string{"map-receipt"},
		Allocations: []application.AllocationDirective{{
			Mapping: "map-receipt", TargetKind: domain.TargetStatement, Target: "stmt-1",
			Direction: domain.AllocationDebit, AmountMinor: amount,
		}},
		Basis:     "instruction-1",
		AppliedAt: time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC),
	}
}

// Covers: ADR-0181 决定一与 SA CONTEXT「入账余额……按……减去指向该作用域客户应收的核销分配净额重算并推进……已核销
// 分配等额离开已确认未结」——核销与撤销在自己的事务里推进；`AT-SA-113`「核销撤销恢复相应未结金额」；超出已确认应收的
// 对账单分配落行之前答`未受理`。
func TestApplicationsAndReversalsRecomputeThePostedBalanceInTheirOwnTransaction(t *testing.T) {
	db, pool := receiptPostingDB(t)
	ctx := t.Context()
	seedCustomerReceiptScope(t, pool)
	seedConfirmedReceivable(t, ctx, pool, 6_000)

	clock := &movingClock{at: time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)}
	handler := mustFundsHandler(t, mustFundsFacts(t, db), mustFundsMappings(t, db), mustApplications(t, db),
		mustReceiptPostings(t, db), clock)
	tenant := receiptTenant(t)
	within := func(what string, want application.FundsOutcome, run func(context.Context) (application.FundsResult, error)) {
		t.Helper()
		clock.at = clock.at.Add(time.Hour)
		saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
			result, err := run(txCtx)
			if err != nil {
				return err
			}
			if result.Outcome() != want {
				t.Errorf("%s = %s，要 %s", what, result.Outcome(), want)
			}
			return nil
		})
	}

	within("采用", application.FundsFactAdopted, func(txCtx context.Context) (application.FundsResult, error) {
		return handler.AdoptFact(txCtx, receiptFact(tenant, "receipt-1", domain.FundsReceiptConfirmed, 10_000, "v1"))
	})
	within("映射到对账单", application.FundsMapped, func(txCtx context.Context) (application.FundsResult, error) {
		return handler.Map(txCtx, receiptMap(tenant, "map-receipt", "receipt-1", domain.TargetStatement, "stmt-1"))
	})
	assertPosted(t, pool, 10_000, 0, 6_000)

	within("核销 4000", application.SettlementApplied, func(txCtx context.Context) (application.FundsResult, error) {
		return handler.Apply(txCtx, statementApplication(tenant, "app-1", 4_000))
	})
	assertPosted(t, pool, 6_000, 0, 2_000)

	within("超出已确认应收的核销", application.FundsNotAccepted, func(txCtx context.Context) (application.FundsResult, error) {
		return handler.Apply(txCtx, statementApplication(tenant, "app-over", 2_001))
	})
	assertPosted(t, pool, 6_000, 0, 2_000)
	var overRows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM settlement_accounting.settlement_application WHERE tenant_id = 'tenant-receipt' AND application_id = 'app-over'`,
	).Scan(&overRows); err != nil || overRows != 0 {
		t.Fatalf("超出已确认应收的核销落了 %d 行（%v）", overRows, err)
	}

	within("撤销", application.ApplicationReversedOutcome, func(txCtx context.Context) (application.FundsResult, error) {
		return handler.Reverse(txCtx, application.ReverseApplicationCommand{
			TenantID: tenant, Application: "app-1", Basis: "payment-returned-1", ReversedAt: clock.at,
		})
	})
	assertPosted(t, pool, 10_000, 0, 6_000)
}

// Covers: PBC-08——两把新锁无环境事务即拒，不在自动提交里取一把语句结束就放的锁。
func TestTheFactAndBalanceScopeLocksRefuseToRunOutsideATransaction(t *testing.T) {
	db, _ := receiptPostingDB(t)
	ctx := t.Context()
	applications, err := adapter.NewSettlementApplications(db)
	if err != nil {
		t.Fatalf("构造核销库：%v", err)
	}
	postings, err := adapter.NewCustomerReceiptPostings(db)
	if err != nil {
		t.Fatalf("构造重算：%v", err)
	}

	if _, _, err := applications.LockFundsFactApplications(ctx, ports.FundsFactKey{
		TenantID: receiptTenant(t), Fact: saValue(t, domain.NewFundsFactReference, "receipt-1"),
	}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务锁事实：err = %v，要 ErrTransactionRequired", err)
	}
	if err := postings.LockOperationalBalanceScope(ctx, receiptTenant(t), receiptScope(t)); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务锁余额作用域：err = %v，要 ErrTransactionRequired", err)
	}
}
