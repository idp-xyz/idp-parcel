package settlementaccounting_test

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

type accountClock struct{ at time.Time }

func (clock accountClock) Now() time.Time { return clock.at }

func TestTheProductionDirectoryFindsOnlyTheRegisteredReceivableAccount(t *testing.T) {
	db := openSettlementDB(t)
	accounts := mustAccounts(t, db)
	handler := mustAccountHandler(t, accounts)
	directory := mustDirectory(t, accounts)
	ctx := t.Context()

	registerAccount(t, handler, db, "ACCT-R", sadomain.ChargeReceivable, "CNY")
	registerAccount(t, handler, db, "ACCT-P", sadomain.ChargePayable, "CNY")
	registerAccount(t, handler, db, "ACCT-USD", sadomain.ChargeReceivable, "USD")

	found, ok, err := directory.FindSettlementAccount(ctx, scopeIdentity(t), adoptedTerms(t))
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !ok || found.String() != "ACCT-R" {
		t.Fatalf("found %q ok=%v, want ACCT-R", found, ok)
	}

	usd := adoptedCurrency(t, "USD")
	usdAccount, ok, err := directory.FindSettlementAccount(ctx, scopeIdentity(t), usd)
	if err != nil {
		t.Fatalf("find usd: %v", err)
	}
	if !ok || usdAccount.String() != "ACCT-USD" {
		t.Fatalf("usd found %q ok=%v, want ACCT-USD", usdAccount, ok)
	}

	_, ok, err = directory.FindSettlementAccount(ctx, scopeIdentity(t), adoptedPolicy(t, "settlement-policy-absent"))
	if err != nil || ok {
		t.Fatalf("未登记 ok=%v err=%v, want not found", ok, err)
	}
}

func TestAPayableRowDoesNotAnswerTheReceivableDirectory(t *testing.T) {
	db := openSettlementDB(t)
	accounts := mustAccounts(t, db)
	handler := mustAccountHandler(t, accounts)
	directory := mustDirectory(t, accounts)
	registerAccount(t, handler, db, "ACCT-P", sadomain.ChargePayable, "CNY")

	_, ok, err := directory.FindSettlementAccount(t.Context(), scopeIdentity(t), adoptedTerms(t))
	if err != nil || ok {
		t.Fatalf("应付行被目录当成了应收账户 ok=%v err=%v", ok, err)
	}
}

func adoptedCurrency(t *testing.T, currency string) domain.AdoptedSettlementTerms {
	t.Helper()
	return adoptedEcho(t, "settlement-policy-1/v3", currency)
}

func adoptedPolicy(t *testing.T, policy string) domain.AdoptedSettlementTerms {
	t.Helper()
	return adoptedEcho(t, policy, "CNY")
}

func adoptedEcho(t *testing.T, policy, currency string) domain.AdoptedSettlementTerms {
	t.Helper()
	terms, err := domain.NewAdoptedSettlementTerms(domain.AdoptedSettlementTermsSpec{
		Policy:       value(t, domain.NewSettlementPolicyEcho, policy),
		Method:       value(t, domain.NewSettlementMethodEcho, "TERMS"),
		LegalEntity:  value(t, domain.NewSettlementLegalEntityEcho, "legal-1"),
		Counterparty: value(t, domain.NewSettlementCounterpartyEcho, "counterparty-1"),
		Currency:     value(t, domain.NewSettlementCurrencyEcho, currency),
	})
	if err != nil {
		t.Fatalf("terms: %v", err)
	}
	return terms
}

func openSettlementDB(t *testing.T) *bentopg.DB {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	return db
}

func mustAccounts(t *testing.T, db *bentopg.DB) *sapostgres.SettlementAccounts {
	t.Helper()
	accounts, err := sapostgres.NewSettlementAccounts(db)
	if err != nil {
		t.Fatalf("accounts: %v", err)
	}
	return accounts
}

func mustAccountHandler(t *testing.T, accounts *sapostgres.SettlementAccounts) *saapplication.RegisterSettlementAccountHandler {
	t.Helper()
	handler, err := saapplication.NewRegisterSettlementAccountHandler(accounts, accountClock{
		at: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	return handler
}

func mustDirectory(t *testing.T, accounts ports.SettlementAccountRegister) *adapter.RegisteredAccountDirectory {
	t.Helper()
	directory, err := adapter.NewRegisteredAccountDirectory(accounts)
	if err != nil {
		t.Fatalf("directory: %v", err)
	}
	return directory
}

func registerAccount(
	t *testing.T,
	handler *saapplication.RegisterSettlementAccountHandler,
	db *bentopg.DB,
	accountID string,
	direction sadomain.ChargeDirection,
	currency string,
) {
	t.Helper()
	key, err := sadomain.NewSettlementAccountKey(
		parseSA(t, sadomain.NewLegalEntityReference, "legal-1"),
		parseSA(t, sadomain.NewSettlementCounterpartyReference, "counterparty-1"),
		direction,
		parseSA(t, sadomain.NewCurrencyCode, currency),
		parseSA(t, sadomain.NewSettlementPolicyReference, "settlement-policy-1/v3"),
	)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	statement, err := sadomain.NewAccountStatementTerms("MONTHLY", "Asia/Shanghai", "18:00", "NET-30")
	if err != nil {
		t.Fatalf("statement: %v", err)
	}
	account, err := sadomain.NewSettlementAccount(
		parseSA(t, sadomain.NewSettlementAccountID, accountID),
		key,
		sadomain.SettlementCounterpartyReference{},
		false,
		parseSA(t, sadomain.NewResponsibilityBasis, "CONTRACT-1"),
		statement,
	)
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	tenant := parseSA(t, sadomain.NewTenantID, "tenant-1")
	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		effect, err := handler.Register(ctx, saapplication.RegisterSettlementAccountCommand{
			Tenant:  tenant,
			Account: account,
		})
		if err != nil {
			return err
		}
		if effect != ports.SettlementAccountRegistered {
			t.Errorf("effect = %d, want registered", effect)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("register %s: %v", accountID, err)
	}
}

func parseSA[T any](t *testing.T, parse func(string) (T, error), value string) T {
	t.Helper()
	parsed, err := parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
