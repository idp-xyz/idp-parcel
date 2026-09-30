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

	registerAccount(t, handler, db, "ACCT-R", sadomain.ChargeReceivable, "CNY", "legal-1", "counterparty-1", "settlement-policy-1", "CONTRACT-1")
	registerAccount(t, handler, db, "ACCT-P", sadomain.ChargePayable, "CNY", "legal-1", "counterparty-1", "settlement-policy-1", "CONTRACT-1")
	registerAccount(t, handler, db, "ACCT-USD", sadomain.ChargeReceivable, "USD", "legal-1", "counterparty-1", "settlement-policy-1", "CONTRACT-1")

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

	_, ok, err = directory.FindSettlementAccount(ctx, scopeIdentity(t), adoptedPolicy(t, "settlement-policy-absent", "v1"))
	if err != nil || ok {
		t.Fatalf("未登记 ok=%v err=%v, want not found", ok, err)
	}
}

func TestAPayableRowDoesNotAnswerTheReceivableDirectory(t *testing.T) {
	db := openSettlementDB(t)
	accounts := mustAccounts(t, db)
	handler := mustAccountHandler(t, accounts)
	directory := mustDirectory(t, accounts)
	registerAccount(t, handler, db, "ACCT-P", sadomain.ChargePayable, "CNY", "legal-1", "counterparty-1", "settlement-policy-1", "CONTRACT-1")

	_, ok, err := directory.FindSettlementAccount(t.Context(), scopeIdentity(t), adoptedTerms(t))
	if err != nil || ok {
		t.Fatalf("应付行被目录当成了应收账户 ok=%v err=%v", ok, err)
	}
}

func TestAPolicyObjectRegisteredOnceAnswersBothAdoptedVersions(t *testing.T) {
	db := openSettlementDB(t)
	accounts := mustAccounts(t, db)
	handler := mustAccountHandler(t, accounts)
	directory := mustDirectory(t, accounts)
	registerAccount(t, handler, db, "ACCT-R", sadomain.ChargeReceivable, "CNY", "legal-1", "counterparty-1", "settlement-policy-1", "CONTRACT-1")

	for _, version := range []string{"v1", "v2"} {
		found, ok, err := directory.FindSettlementAccount(
			t.Context(), scopeIdentity(t), adoptedVersion(t, "settlement-policy-1", version),
		)
		if err != nil || !ok || found.String() != "ACCT-R" {
			t.Fatalf("%s found %q ok=%v err=%v, want ACCT-R", version, found, ok, err)
		}
	}
}

func TestTheDirectoryKeepsTenantLegalEntityAndCounterpartyApart(t *testing.T) {
	db := openSettlementDB(t)
	accounts := mustAccounts(t, db)
	handler := mustAccountHandler(t, accounts)
	directory := mustDirectory(t, accounts)
	registerAccount(t, handler, db, "ACCT-R", sadomain.ChargeReceivable, "CNY", "legal-1", "counterparty-1", "settlement-policy-1", "CONTRACT-1")

	otherTenant, err := domain.NewSourceIdentity(
		value(t, domain.NewTenantID, "tenant-2"),
		value(t, domain.NewCustomerAccountID, "customer-1"),
		value(t, domain.NewSource, "source-a"),
		value(t, domain.NewSourceRequestKey, "key-1"),
	)
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	cases := []struct {
		name     string
		identity domain.SourceIdentity
		terms    domain.AdoptedSettlementTerms
	}{
		{"另一租户", otherTenant, adoptedTerms(t)},
		{"另一法人", scopeIdentity(t), adoptedParty(t, "legal-2", "counterparty-1")},
		{"另一相对方", scopeIdentity(t), adoptedParty(t, "legal-1", "counterparty-2")},
	}
	for _, tc := range cases {
		_, ok, err := directory.FindSettlementAccount(t.Context(), tc.identity, tc.terms)
		if err != nil || ok {
			t.Fatalf("%s ok=%v err=%v, want not found", tc.name, ok, err)
		}
	}
}

func TestRegisteringTheSameAccountReplaysAndADifferentBodyConflicts(t *testing.T) {
	db := openSettlementDB(t)
	accounts := mustAccounts(t, db)
	handler := mustAccountHandler(t, accounts)

	registerAccount(t, handler, db, "ACCT-R", sadomain.ChargeReceivable, "CNY", "legal-1", "counterparty-1", "settlement-policy-1", "CONTRACT-1")
	replay := registerAccountEffect(t, handler, db, "ACCT-R", sadomain.ChargeReceivable, "CNY", "legal-1", "counterparty-1", "settlement-policy-1", "CONTRACT-1")
	if replay != ports.SettlementAccountReplay {
		t.Fatalf("同一份再登 effect=%d, want replay", replay)
	}
	conflict := registerAccountEffect(t, handler, db, "ACCT-R", sadomain.ChargeReceivable, "CNY", "legal-1", "counterparty-1", "settlement-policy-1", "CONTRACT-2")
	if conflict != ports.SettlementAccountConflict {
		t.Fatalf("同一标识不同依据 effect=%d, want conflict", conflict)
	}
	otherID := registerAccountEffect(t, handler, db, "ACCT-OTHER", sadomain.ChargeReceivable, "CNY", "legal-1", "counterparty-1", "settlement-policy-1", "CONTRACT-1")
	if otherID != ports.SettlementAccountConflict {
		t.Fatalf("同一组挂到另一个标识 effect=%d, want conflict", otherID)
	}
}

func adoptedCurrency(t *testing.T, currency string) domain.AdoptedSettlementTerms {
	t.Helper()
	return adoptedEcho(t, "settlement-policy-1", "v3", "legal-1", "counterparty-1", currency)
}

func adoptedPolicy(t *testing.T, object, version string) domain.AdoptedSettlementTerms {
	t.Helper()
	return adoptedEcho(t, object, version, "legal-1", "counterparty-1", "CNY")
}

func adoptedVersion(t *testing.T, object, version string) domain.AdoptedSettlementTerms {
	t.Helper()
	return adoptedPolicy(t, object, version)
}

func adoptedParty(t *testing.T, legalEntity, counterparty string) domain.AdoptedSettlementTerms {
	t.Helper()
	return adoptedEcho(t, "settlement-policy-1", "v3", legalEntity, counterparty, "CNY")
}

func adoptedEcho(t *testing.T, object, version, legalEntity, counterparty, currency string) domain.AdoptedSettlementTerms {
	t.Helper()
	policy, err := domain.NewSettlementPolicyEcho(object, version)
	if err != nil {
		t.Fatalf("policy echo: %v", err)
	}
	terms, err := domain.NewAdoptedSettlementTerms(domain.AdoptedSettlementTermsSpec{
		Policy:       policy,
		Method:       value(t, domain.NewSettlementMethodEcho, "TERMS"),
		LegalEntity:  value(t, domain.NewSettlementLegalEntityEcho, legalEntity),
		Counterparty: value(t, domain.NewSettlementCounterpartyEcho, counterparty),
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

func mustDirectory(t *testing.T, accounts ports.SettlementAccountView) *adapter.RegisteredAccountDirectory {
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
	currency, legalEntity, counterparty, policy, basis string,
) {
	t.Helper()
	effect := registerAccountEffect(t, handler, db, accountID, direction, currency, legalEntity, counterparty, policy, basis)
	if effect != ports.SettlementAccountRegistered {
		t.Fatalf("register %s effect = %d, want registered", accountID, effect)
	}
}

func registerAccountEffect(
	t *testing.T,
	handler *saapplication.RegisterSettlementAccountHandler,
	db *bentopg.DB,
	accountID string,
	direction sadomain.ChargeDirection,
	currency, legalEntity, counterparty, policy, basis string,
) ports.SettlementAccountRegistrationEffect {
	t.Helper()
	key, err := sadomain.NewSettlementAccountKey(
		parseSA(t, sadomain.NewLegalEntityReference, legalEntity),
		parseSA(t, sadomain.NewSettlementCounterpartyReference, counterparty),
		direction,
		parseSA(t, sadomain.NewCurrencyCode, currency),
		parseSA(t, sadomain.NewSettlementPolicyReference, policy),
	)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	account, err := sadomain.NewSettlementAccount(
		parseSA(t, sadomain.NewSettlementAccountID, accountID),
		key,
		sadomain.SettlementCounterpartyReference{},
		false,
		parseSA(t, sadomain.NewResponsibilityBasis, basis),
	)
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	tenant := parseSA(t, sadomain.NewTenantID, "tenant-1")
	var effect ports.SettlementAccountRegistrationEffect
	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		var registerErr error
		effect, registerErr = handler.Register(ctx, saapplication.RegisterSettlementAccountCommand{
			Tenant:  tenant,
			Account: account,
		})
		return registerErr
	})
	if err != nil {
		t.Fatalf("register %s: %v", accountID, err)
	}
	return effect
}

func parseSA[T any](t *testing.T, parse func(string) (T, error), value string) T {
	t.Helper()
	parsed, err := parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
