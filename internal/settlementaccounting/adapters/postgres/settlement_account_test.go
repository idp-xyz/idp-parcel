package postgres_test

import (
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 登记面写入必须在事务里。写口在事务外被调是装配缺陷，按框架合同拒。
func TestSavingASettlementAccountRequiresATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	accounts, err := adapter.NewSettlementAccounts(db)
	if err != nil {
		t.Fatalf("构造结算账户登记册：%v", err)
	}
	account := settlementAccountOutsideATransaction(t)

	if _, err := accounts.Save(t.Context(), settlementTenant(t), account, time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 Save 应答 ErrTransactionRequired，实得：%v", err)
	}
}

func settlementTenant(t *testing.T) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	return tenant
}

func settlementAccountOutsideATransaction(t *testing.T) domain.SettlementAccount {
	t.Helper()
	key, err := domain.NewSettlementAccountKey(
		mustAccountRef(t, domain.NewLegalEntityReference, "LE-1"),
		mustAccountRef(t, domain.NewSettlementCounterpartyReference, "CP-1"),
		domain.ChargeReceivable,
		mustAccountRef(t, domain.NewCurrencyCode, "CNY"),
		mustAccountRef(t, domain.NewSettlementPolicyReference, "POL-1"),
	)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	account, err := domain.NewSettlementAccount(
		mustAccountRef(t, domain.NewSettlementAccountID, "ACCT-1"),
		key,
		domain.SettlementCounterpartyReference{},
		false,
		mustAccountRef(t, domain.NewResponsibilityBasis, "CONTRACT-1"),
	)
	if err != nil {
		t.Fatalf("account: %v", err)
	}
	return account
}

func mustAccountRef[T any](t *testing.T, parse func(string) (T, error), value string) T {
	t.Helper()
	parsed, err := parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
