package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// Covers: ADR-0181 决定六——客户收款映射及其资金退回在同一事务里重算入账余额；
// 授信额度列插入为 0 且之后不覆盖；迟到快照不盖新值；从未推进的作用域仍答余额不可用。
func TestMappingACustomerReceiptAdvancesThePostedBalance(t *testing.T) {
	db, pool := receiptPostingDB(t)
	ctx := t.Context()
	seedCustomerReceiptScope(t, pool)

	postings := mustReceiptPostings(t, db)
	facts := mustFundsFacts(t, db)
	mappings := mustFundsMappings(t, db)
	applications := mustApplications(t, db)
	clock := &movingClock{at: time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)}
	handler := mustFundsHandler(t, facts, mappings, applications, postings, clock)
	tenant := receiptTenant(t)
	balances := mustBalances(t, db)

	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		if _, err := handler.AdoptFact(txCtx, receiptFact(tenant, "receipt-1", domain.FundsReceiptConfirmed, 10_000, "v1")); err != nil {
			return err
		}
		payable, err := handler.Map(txCtx, receiptMap(tenant, "map-payable", "receipt-1", domain.TargetPayable, "payable-1"))
		if err != nil {
			return err
		}
		if payable.Outcome() != application.FundsMapped {
			t.Errorf("应付映射 = %s", payable.Outcome())
		}
		return nil
	})
	if _, err := balances.LoadBalance(ctx, tenant, receiptScope(t)); !errors.Is(err, adapter.ErrBalanceNotRegistered) {
		t.Fatalf("供应商应付映射推进了客户余额：%v", err)
	}

	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		mapped, err := handler.Map(txCtx, receiptMap(tenant, "map-receipt", "receipt-1", domain.TargetStatement, "stmt-1"))
		if err != nil {
			return err
		}
		if mapped.Outcome() != application.FundsMapped {
			t.Errorf("收款映射 = %s", mapped.Outcome())
		}
		return nil
	})
	assertPosted(t, pool, 10_000, 0, 0)

	if _, err := pool.Exec(ctx,
		`UPDATE settlement_accounting.operational_balance
		    SET credit_minor = 7
		  WHERE tenant_id = 'tenant-receipt' AND account_id = 'acct-1'`); err != nil {
		t.Fatalf("放一笔额度列：%v", err)
	}

	clock.at = clock.at.Add(time.Hour)
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		if _, err := handler.AdoptFact(txCtx, receiptFact(tenant, "return-1", domain.FundsReturned, 4_000, "v1")); err != nil {
			return err
		}
		returned, err := handler.Map(txCtx, receiptMap(tenant, "map-return", "return-1", domain.TargetStatement, "stmt-1"))
		if err != nil {
			return err
		}
		if returned.Outcome() != application.FundsMapped {
			t.Errorf("退回映射 = %s", returned.Outcome())
		}
		return nil
	})
	assertPosted(t, pool, 6_000, 7, 0)

	scope := receiptScope(t)
	var outcome ports.OperationalBalanceAdvanceOutcome
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		var err error
		outcome, err = postings.AdvanceOperationalBalance(txCtx, tenant, scope, 1, 0, clock.at.Add(-2*time.Hour))
		return err
	})
	if outcome != ports.OperationalBalanceNotAdvanced {
		t.Fatalf("迟到快照 = %s，want NOT_ADVANCED", outcome)
	}
	assertPosted(t, pool, 6_000, 7, 0)

	other := receiptScopeAccount(t, "acct-2")
	if _, err := balances.LoadBalance(ctx, tenant, other); !errors.Is(err, adapter.ErrBalanceNotRegistered) {
		t.Fatalf("从未推进的作用域：%v", err)
	}
}

func TestANegativeUnsettledRollsTheMappingBack(t *testing.T) {
	db, pool := receiptPostingDB(t)
	ctx := t.Context()
	seedCustomerReceiptScope(t, pool)
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.settlement_application
			(tenant_id, application_id, fact_id, currency, fact_minor, applied_minor,
			 allocations, basis, applied_at, content_digest, recorded_at)
		 VALUES ('tenant-receipt', 'app-1', 'receipt-2', 'CNY', 100, 50,
		         '[{"mapping":"m","targetKind":"STATEMENT","target":"stmt-1","direction":"DEBIT","amountMinor":50}]'::jsonb,
		         'basis-1', now(), 'digest', now())`); err != nil {
		t.Fatalf("预置核销：%v", err)
	}

	postings := mustReceiptPostings(t, db)
	handler := mustFundsHandler(t, mustFundsFacts(t, db), mustFundsMappings(t, db), mustApplications(t, db), postings,
		&movingClock{at: time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)})
	tenant := receiptTenant(t)

	err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := handler.AdoptFact(txCtx, receiptFact(tenant, "receipt-2", domain.FundsReceiptConfirmed, 100, "v1")); err != nil {
			return err
		}
		_, err := handler.Map(txCtx, receiptMap(tenant, "map-2", "receipt-2", domain.TargetStatement, "stmt-1"))
		return err
	})
	if !errors.Is(err, domain.ErrNegativeConfirmedUnsettled) {
		t.Fatalf("err = %v，want 已确认未结为负", err)
	}
	var mappings int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM settlement_accounting.funds_mapping WHERE tenant_id = 'tenant-receipt'`).Scan(&mappings); err != nil {
		t.Fatalf("数映射：%v", err)
	}
	if mappings != 0 {
		t.Fatalf("推进失败后映射仍在：%d", mappings)
	}
	if _, err := mustBalances(t, db).LoadBalance(ctx, tenant, receiptScope(t)); !errors.Is(err, adapter.ErrBalanceNotRegistered) {
		t.Fatalf("推进失败后仍有余额：%v", err)
	}
}

func TestMappingAReceiptToTheReceivableAccountWritesBalanceAndStanding(t *testing.T) {
	db, pool := receiptPostingDB(t)
	ctx := t.Context()
	seedCustomerReceiptScope(t, pool)
	clock := &movingClock{at: time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)}
	postings := mustReceiptPostings(t, db)
	handler := mustFundsHandler(t, mustFundsFacts(t, db), mustFundsMappings(t, db), mustApplications(t, db), postings, clock)
	tenant := receiptTenant(t)

	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		if _, err := handler.AdoptFact(txCtx, receiptFact(tenant, "receipt-acct", domain.FundsReceiptConfirmed, 10_000, "v1")); err != nil {
			return err
		}
		mapped, err := handler.Map(txCtx, receiptMap(tenant, "map-acct", "receipt-acct", domain.TargetAccount, "acct-1"))
		if err != nil {
			return err
		}
		if mapped.Outcome() != application.FundsMapped {
			t.Errorf("账户映射 = %s", mapped.Outcome())
		}
		return nil
	})
	assertPosted(t, pool, 10_000, 0, 0)
	assertStanding(t, pool, 0, false)

	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.customer_charge
			(tenant_id, charge_id, fee_item, evaluation_ref, original_currency,
			 original_minor, settlement_currency, settlement_minor,
			 stage, confirmation_basis, formed_at, confirmed_at, recorded_at,
			 responsible_entity, counterparty_ref, charge_direction,
			 settlement_account_id, contract_basis, primary_charging_scope,
			 source_fact_ref)
		 VALUES ('tenant-receipt', 'charge-1', 'BASE_FREIGHT', 'eval-1', 'CNY', 100, 'CNY', 100,
		         'CONFIRMED', 'basis-1', now(), now(), now(),
		         'le-1', 'shipper-1', 'RECEIVABLE', 'acct-1', 'contract-1', 'scope-1', 'fact-1')`); err != nil {
		t.Fatalf("预置缺到期日的已确认费用：%v", err)
	}
	clock.at = clock.at.Add(time.Hour)
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		if _, err := handler.AdoptFact(txCtx, receiptFact(tenant, "receipt-acct-2", domain.FundsReceiptConfirmed, 1_000, "v1")); err != nil {
			return err
		}
		_, err := handler.Map(txCtx, receiptMap(tenant, "map-acct-2", "receipt-acct-2", domain.TargetAccount, "acct-1"))
		return err
	})
	assertPosted(t, pool, 11_000, 0, 100)
	assertStanding(t, pool, 0, false)
	var asOf time.Time
	if err := pool.QueryRow(ctx,
		`SELECT as_of FROM settlement_accounting.credit_standing
		  WHERE tenant_id = 'tenant-receipt' AND account_id = 'acct-1'`).Scan(&asOf); err != nil {
		t.Fatalf("读信用状况时点：%v", err)
	}
	if !asOf.Equal(time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("缺到期日仍改写了信用状况时点：%s", asOf)
	}
}

func TestAPayableAccountMappingDoesNotStand(t *testing.T) {
	db, pool := receiptPostingDB(t)
	ctx := t.Context()
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.settlement_account
			(tenant_id, account_id, legal_entity_id, counterparty_id, direction, currency,
			 settlement_policy_id, responsibility_basis, registered_at)
		 VALUES ('tenant-receipt', 'acct-pay', 'le-1', 'shipper-1', 'PAYABLE', 'CNY',
		         'policy-1', 'contract-1', now())`); err != nil {
		t.Fatalf("应付账户：%v", err)
	}
	handler := mustFundsHandler(t, mustFundsFacts(t, db), mustFundsMappings(t, db), mustApplications(t, db),
		mustReceiptPostings(t, db), &movingClock{at: time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)})
	tenant := receiptTenant(t)
	err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := handler.AdoptFact(txCtx, receiptFact(tenant, "receipt-pay", domain.FundsReceiptConfirmed, 100, "v1")); err != nil {
			return err
		}
		_, err := handler.Map(txCtx, receiptMap(tenant, "map-pay", "receipt-pay", domain.TargetAccount, "acct-pay"))
		return err
	})
	if !errors.Is(err, domain.ErrNotReceivableAccount) {
		t.Fatalf("err = %v，want 不是应收账户", err)
	}
}

type movingClock struct{ at time.Time }

func (clock *movingClock) Now() time.Time { return clock.at }

type nopHandoff struct{}

func (nopHandoff) HandOffExternalFundsFact(context.Context, ports.ExternalFundsFactIntent) error {
	return nil
}

func (nopHandoff) HandOffSettlementApplication(context.Context, ports.SettlementApplicationIntent) error {
	return nil
}

func receiptPostingDB(t *testing.T) (*bentopg.DB, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return db, pool
}

func seedCustomerReceiptScope(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := t.Context()
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.settlement_account
			(tenant_id, account_id, legal_entity_id, counterparty_id, direction, currency,
			 settlement_policy_id, responsibility_basis, registered_at)
		 VALUES ('tenant-receipt', 'acct-1', 'le-1', 'shipper-1', 'RECEIVABLE', 'CNY',
		         'policy-1', 'contract-1', now())`); err != nil {
		t.Fatalf("结算账户：%v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO settlement_accounting.customer_statement
			(tenant_id, statement_number, account_id, period_ref, currency, total_minor,
			 content_digest, lines, adjustment_lines, published_at, recorded_at)
		 VALUES ('tenant-receipt', 'stmt-1', 'acct-1', 'period-1', 'CNY', 10000,
		         'digest', '[{"charge":"c-1","amount_minor":10000}]'::jsonb, '[]'::jsonb, now(), now())`); err != nil {
		t.Fatalf("对账单：%v", err)
	}
}

func mustReceiptPostings(t *testing.T, db *bentopg.DB) *adapter.CustomerReceiptPostings {
	t.Helper()
	postings, err := adapter.NewCustomerReceiptPostings(db)
	if err != nil {
		t.Fatalf("构造重算：%v", err)
	}
	return postings
}

func mustFundsFacts(t *testing.T, db *bentopg.DB) *adapter.ExternalFundsFacts {
	t.Helper()
	facts, err := adapter.NewExternalFundsFacts(db)
	if err != nil {
		t.Fatal(err)
	}
	return facts
}

func mustFundsMappings(t *testing.T, db *bentopg.DB) *adapter.FundsMappings {
	t.Helper()
	mappings, err := adapter.NewFundsMappings(db)
	if err != nil {
		t.Fatal(err)
	}
	return mappings
}

func mustApplications(t *testing.T, db *bentopg.DB) *adapter.SettlementApplications {
	t.Helper()
	applications, err := adapter.NewSettlementApplications(db)
	if err != nil {
		t.Fatal(err)
	}
	return applications
}

func mustBalances(t *testing.T, db *bentopg.DB) *adapter.OperationalBalances {
	t.Helper()
	freezes, err := adapter.NewFreezeLedgers(db)
	if err != nil {
		t.Fatal(err)
	}
	balances, err := adapter.NewOperationalBalances(db, freezes)
	if err != nil {
		t.Fatal(err)
	}
	return balances
}

func mustFundsHandler(
	t *testing.T,
	facts *adapter.ExternalFundsFacts,
	mappings *adapter.FundsMappings,
	applications *adapter.SettlementApplications,
	postings *adapter.CustomerReceiptPostings,
	clock ports.Clock,
) *application.MapExternalFundsHandler {
	t.Helper()
	handler, err := application.NewMapExternalFundsHandler(application.MapExternalFundsDeps{
		Facts:        facts,
		Mappings:     mappings,
		Applications: applications,
		Downstream:   nopHandoff{},
		FactHandoff:  nopHandoff{},
		Clock:        clock,
		Books:        postings,
		Balances:     postings,
		Standings:    postings,
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func receiptTenant(t *testing.T) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-receipt")
	if err != nil {
		t.Fatal(err)
	}
	return tenant
}

func receiptScope(t *testing.T) domain.SettlementScope {
	t.Helper()
	return receiptScopeAccount(t, "acct-1")
}

func receiptScopeAccount(t *testing.T, account string) domain.SettlementScope {
	t.Helper()
	scope, err := domain.NewSettlementScope(
		mustValue(t, domain.NewLegalEntityReference, "le-1"),
		mustValue(t, domain.NewSettlementAccountID, account),
		mustValue(t, domain.NewCurrencyCode, "CNY"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func mustValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func receiptFact(tenant domain.TenantID, id string, kind domain.FundsFactKind, amount int64, version string) application.AdoptFundsFactCommand {
	return application.AdoptFundsFactCommand{
		TenantID:    tenant,
		Fact:        id,
		Source:      "bank-1",
		Kind:        kind,
		Currency:    "CNY",
		AmountMinor: amount,
		Version:     version,
		OccurredAt:  time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
}

func receiptMap(tenant domain.TenantID, mapping, fact string, kind domain.SettlementTargetKind, target string) application.MapFundsCommand {
	return application.MapFundsCommand{
		TenantID:   tenant,
		Mapping:    mapping,
		Fact:       fact,
		TargetKind: kind,
		Target:     target,
		Basis:      "instruction-1",
		MappedAt:   time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC),
	}
}

func assertPosted(t *testing.T, pool *pgxpool.Pool, posted, credit, unsettled int64) {
	t.Helper()
	var gotPosted, gotCredit, gotUnsettled int64
	err := pool.QueryRow(t.Context(),
		`SELECT posted_minor, credit_minor, unsettled_minor
		   FROM settlement_accounting.operational_balance
		  WHERE tenant_id = 'tenant-receipt' AND account_id = 'acct-1'`).Scan(&gotPosted, &gotCredit, &gotUnsettled)
	if err != nil {
		t.Fatalf("读余额：%v", err)
	}
	if gotPosted != posted || gotCredit != credit || gotUnsettled != unsettled {
		t.Fatalf("入账/额度/未结 = %d/%d/%d，want %d/%d/%d", gotPosted, gotCredit, gotUnsettled, posted, credit, unsettled)
	}
}

func assertStanding(t *testing.T, pool *pgxpool.Pool, limit int64, overdue bool) {
	t.Helper()
	var gotLimit int64
	var gotOverdue bool
	err := pool.QueryRow(t.Context(),
		`SELECT limit_minor, overdue
		   FROM settlement_accounting.credit_standing
		  WHERE tenant_id = 'tenant-receipt' AND account_id = 'acct-1'`).Scan(&gotLimit, &gotOverdue)
	if err != nil {
		t.Fatalf("读信用状况：%v", err)
	}
	if gotLimit != limit || gotOverdue != overdue {
		t.Fatalf("额度/逾期 = %d/%v，want %d/%v", gotLimit, gotOverdue, limit, overdue)
	}
}
